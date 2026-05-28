package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	globalStateFileBasename     = ".codex-global-state.json"
	globalStateBackupFileBasename = ".codex-global-state.json.bak"
)

func globalStatePath(codexHome string) string {
	return filepath.Join(codexHome, globalStateFileBasename)
}

func globalStateBackupPath(codexHome string) string {
	return filepath.Join(codexHome, globalStateBackupFileBasename)
}

// ---------- path normalization ----------

// normalizeComparablePath returns a lowercased, backslash-normalized path for comparison.
func normalizeComparablePath(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}

	// Remove \\?\UNC\ prefix → \\
	if strings.HasPrefix(trimmed, `\\?\UNC\`) {
		trimmed = `\\` + trimmed[8:]
	} else if strings.HasPrefix(trimmed, `\\?\`) {
		trimmed = trimmed[4:]
	}

	trimmed = strings.ReplaceAll(trimmed, "/", `\`)
	trimmed = strings.TrimRight(trimmed, `\`)

	// Drive letter normalization: C: → C:\
	if len(trimmed) == 2 && trimmed[1] == ':' {
		trimmed += `\`
	}

	return strings.ToLower(trimmed)
}

// ---------- thread cwd stats ----------

func readThreadCwdStats(codexHome string) ([]threadCwdInfo, error) {
	dbPath := stateDBPath(codexHome)
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		return nil, nil
	}

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	if !tableHasColumn(db, "threads", "cwd") {
		return nil, nil
	}

	// Determine available timestamp column
	var timeExpr string
	if tableHasColumn(db, "threads", "updated_at_ms") {
		if tableHasColumn(db, "threads", "updated_at") {
			timeExpr = "COALESCE(MAX(updated_at_ms), MAX(updated_at) * 1000, 0)"
		} else {
			timeExpr = "COALESCE(MAX(updated_at_ms), 0)"
		}
	} else if tableHasColumn(db, "threads", "updated_at") {
		timeExpr = "COALESCE(MAX(updated_at) * 1000, 0)"
	} else {
		timeExpr = "0"
	}

	rows, err := db.Query(fmt.Sprintf(`
		SELECT cwd, COUNT(*) AS count, %s AS updated_at_ms
		FROM threads
		WHERE cwd IS NOT NULL AND cwd <> ''
		GROUP BY cwd
		ORDER BY count DESC, updated_at_ms DESC, cwd
	`, timeExpr))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []threadCwdInfo
	for rows.Next() {
		var cwd string
		var count int
		var updatedAtMs int64
		if err := rows.Scan(&cwd, &count, &updatedAtMs); err != nil {
			continue
		}
		norm := normalizeComparablePath(cwd)
		if norm == "" {
			continue
		}
		result = append(result, threadCwdInfo{
			Cwd:           cwd,
			NormalizedCwd: norm,
			Count:         count,
			UpdatedAtMs:   updatedAtMs,
		})
	}
	return result, rows.Err()
}

// resolveStoredPath finds the most common path variant for a given stored path.
func resolveStoredPath(value string, cwdStats []threadCwdInfo) string {
	comparable := normalizeComparablePath(value)
	if comparable == "" {
		return value
	}

	var matches []threadCwdInfo
	for _, stat := range cwdStats {
		if stat.NormalizedCwd == comparable {
			matches = append(matches, stat)
		}
	}
	if len(matches) == 0 {
		return toDesktopWorkspacePath(value)
	}

	sort.Slice(matches, func(i, j int) bool {
		if matches[i].Count != matches[j].Count {
			return matches[i].Count > matches[j].Count
		}
		if matches[i].UpdatedAtMs != matches[j].UpdatedAtMs {
			return matches[i].UpdatedAtMs > matches[j].UpdatedAtMs
		}
		return matches[i].Cwd < matches[j].Cwd
	})

	return toDesktopWorkspacePath(matches[0].Cwd)
}

// ---------- global state file operations ----------

func toPathArray(value any) []string {
	switch v := value.(type) {
	case []any:
		result := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
				result = append(result, s)
			}
		}
		return result
	case []string:
		result := make([]string, 0, len(v))
		for _, s := range v {
			if strings.TrimSpace(s) != "" {
				result = append(result, s)
			}
		}
		return result
	case string:
		if strings.TrimSpace(v) != "" {
			return []string{v}
		}
	}
	return nil
}

func dedupePathSlice(paths []string) []string {
	seen := make(map[string]bool, len(paths))
	result := make([]string, 0, len(paths))
	for _, p := range paths {
		comparable := normalizeComparablePath(p)
		if comparable == "" || seen[comparable] {
			continue
		}
		seen[comparable] = true
		result = append(result, p)
	}
	return result
}

func readWorkspaceRootsFromGlobalState(state map[string]any) []string {
	savedRoots := toPathArray(state["electron-saved-workspace-roots"])
	projectOrder := toPathArray(state["project-order"])
	activeRoots := toPathArray(state["active-workspace-roots"])

	var combined []string
	if len(projectOrder) > 0 {
		combined = append(projectOrder, savedRoots...)
		combined = append(combined, activeRoots...)
	} else {
		combined = append(savedRoots, activeRoots...)
	}

	resolved := make([]string, len(combined))
	for i, p := range combined {
		resolved[i] = toDesktopWorkspacePath(p)
	}
	return dedupePathSlice(resolved)
}

// syncWorkspaceRoots reads .codex-global-state.json, resolves stale paths, and writes back.
func syncWorkspaceRoots(codexHome string, cwdStats []threadCwdInfo) (updated bool, updatedRoots int, savedRootCount int, _ error) {
	filePath := globalStatePath(codexHome)
	backupPath := globalStateBackupPath(codexHome)

	originalBytes, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, 0, 0, nil
		}
		return false, 0, 0, err
	}

	var state map[string]any
	if err := json.Unmarshal(originalBytes, &state); err != nil {
		return false, 0, 0, err
	}

	if len(cwdStats) == 0 {
		var statErr error
		cwdStats, statErr = readThreadCwdStats(codexHome)
		if statErr != nil {
			cwdStats = nil
		}
	}

	existingSavedRoots := toPathArray(state["electron-saved-workspace-roots"])
	existingProjectOrder := toPathArray(state["project-order"])
	existingActiveRoots := toPathArray(state["active-workspace-roots"])

	// Build next paths by resolving each stored path
	var combined []string
	if len(existingProjectOrder) > 0 {
		combined = append(existingProjectOrder, existingSavedRoots...)
		combined = append(combined, existingActiveRoots...)
	} else {
		combined = append(existingSavedRoots, existingActiveRoots...)
	}

	nextSavedRootsRaw := make([]string, len(combined))
	for i, p := range combined {
		nextSavedRootsRaw[i] = resolveStoredPath(p, cwdStats)
	}
	nextSavedRoots := dedupePathSlice(nextSavedRootsRaw)

	var projectOrderCombined []string
	if len(existingProjectOrder) > 0 {
		projectOrderCombined = append(existingProjectOrder, existingSavedRoots...)
	} else {
		projectOrderCombined = append([]string{}, nextSavedRoots...)
	}
	nextProjectOrderRaw := make([]string, len(projectOrderCombined))
	for i, p := range projectOrderCombined {
		nextProjectOrderRaw[i] = resolveStoredPath(p, cwdStats)
	}
	nextProjectOrder := dedupePathSlice(nextProjectOrderRaw)

	nextActiveRootsRaw := make([]string, len(existingActiveRoots))
	for i, p := range existingActiveRoots {
		nextActiveRootsRaw[i] = resolveStoredPath(p, cwdStats)
	}
	nextActiveRoots := dedupePathSlice(nextActiveRootsRaw)

	// Resolve labels keys
	if labels, ok := state["electron-workspace-root-labels"].(map[string]any); ok {
		newLabels := make(map[string]any, len(labels))
		for key, val := range labels {
			resolved := resolveStoredPath(key, cwdStats)
			if _, exists := newLabels[resolved]; !exists || resolved == key {
				newLabels[resolved] = val
			}
		}
		state["electron-workspace-root-labels"] = newLabels
	}

	// Resolve open-in-target perPath keys
	if openTargets, ok := state["open-in-target-preferences"].(map[string]any); ok {
		if perPath, ok := openTargets["perPath"].(map[string]any); ok {
			newPerPath := make(map[string]any, len(perPath))
			for key, val := range perPath {
				resolved := resolveStoredPath(key, cwdStats)
				if _, exists := newPerPath[resolved]; !exists || resolved == key {
					newPerPath[resolved] = val
				}
			}
			openTargets["perPath"] = newPerPath
		}
		state["open-in-target-preferences"] = openTargets
	}

	// Determine active-workspace-roots value (preserve original type)
	origActiveVal := state["active-workspace-roots"]
	if _, isSlice := origActiveVal.([]any); isSlice {
		state["active-workspace-roots"] = toAnySlice(nextActiveRoots)
	} else if _, isStrSlice := origActiveVal.([]string); isStrSlice {
		state["active-workspace-roots"] = nextActiveRoots
	} else if len(nextActiveRoots) > 0 {
		state["active-workspace-roots"] = nextActiveRoots[0]
	}

	state["electron-saved-workspace-roots"] = toAnySlice(nextSavedRoots)
	state["project-order"] = toAnySlice(nextProjectOrder)

	nextBytes, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return false, 0, 0, err
	}
	nextText := string(nextBytes) + "\n"

	savedRootsChanged := !stringSlicesEqual(existingSavedRoots, nextSavedRoots)
	projectOrderChanged := !stringSlicesEqual(existingProjectOrder, nextProjectOrder)
	activeRootsChanged := fmt.Sprint(origActiveVal) != fmt.Sprint(state["active-workspace-roots"])

	backupMissing := false
	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		backupMissing = true
	}

	changed := savedRootsChanged || projectOrderChanged || activeRootsChanged || backupMissing
	if changed {
		if err := os.WriteFile(filePath, []byte(nextText), 0o600); err != nil {
			return false, 0, 0, err
		}
		if err := os.WriteFile(backupPath, []byte(nextText), 0o600); err != nil {
			// Non-fatal: backup write failure
		}
	}

	changedCount := 0
	for i := 0; i < maxInt(len(existingSavedRoots), len(nextSavedRoots)); i++ {
		var a, b string
		if i < len(existingSavedRoots) {
			a = existingSavedRoots[i]
		}
		if i < len(nextSavedRoots) {
			b = nextSavedRoots[i]
		}
		if a != b {
			changedCount++
		}
	}

	return changed, changedCount, len(nextSavedRoots), nil
}

// ---------- project thread visibility diagnostic ----------

// readProjectThreadVisibility returns per-workspace-root session visibility info.
func readProjectThreadVisibility(codexHome string) ([]ProjectThreadInfo, error) {
	const pageSize = 50

	filePath := globalStatePath(codexHome)
	stateBytes, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var state map[string]any
	if err := json.Unmarshal(stateBytes, &state); err != nil {
		return nil, err
	}

	roots := readWorkspaceRootsFromGlobalState(state)
	if len(roots) == 0 {
		return nil, nil
	}

	dbPath := stateDBPath(codexHome)
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		result := make([]ProjectThreadInfo, len(roots))
		for i, root := range roots {
			result[i] = ProjectThreadInfo{Root: root}
		}
		return result, nil
	}

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	if !tableHasColumn(db, "threads", "cwd") {
		return nil, nil
	}

	sourceFilter := ""
	if tableHasColumn(db, "threads", "source") {
		sourceFilter = "AND source IN ('cli', 'vscode')"
	}
	archivedFilter := ""
	if tableHasColumn(db, "threads", "archived") {
		archivedFilter = "AND archived = 0"
	}
	firstUserFilter := ""
	if tableHasColumn(db, "threads", "first_user_message") {
		firstUserFilter = "AND first_user_message <> ''"
	}

	var timeExpr string
	if tableHasColumn(db, "threads", "updated_at_ms") {
		timeExpr = "COALESCE(updated_at_ms, 0)"
	} else if tableHasColumn(db, "threads", "updated_at") {
		timeExpr = "COALESCE(updated_at * 1000, 0)"
	} else if tableHasColumn(db, "threads", "created_at_ms") {
		timeExpr = "COALESCE(created_at_ms, 0)"
	} else if tableHasColumn(db, "threads", "created_at") {
		timeExpr = "COALESCE(created_at * 1000, 0)"
	} else {
		timeExpr = "0"
	}

	providerExpr := "'' AS model_provider"
	if tableHasColumn(db, "threads", "model_provider") {
		providerExpr = "model_provider"
	}

	query := fmt.Sprintf(`
		SELECT id, cwd, %s, %s AS sort_ts
		FROM threads
		WHERE cwd IS NOT NULL AND cwd <> ''
			%s %s %s
		ORDER BY sort_ts DESC, id DESC
	`, providerExpr, timeExpr, archivedFilter, firstUserFilter, sourceFilter)

	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type rankedRow struct {
		ID          string
		Cwd         string
		Provider    string
		SortTs      int64
		Rank        int
		DesktopCwd  string
	}
	var rankedRows []rankedRow
	rank := 0
	for rows.Next() {
		var id, cwd, provider string
		var sortTs int64
		if err := rows.Scan(&id, &cwd, &provider, &sortTs); err != nil {
			continue
		}
		rank++
		rankedRows = append(rankedRows, rankedRow{
			ID:         id,
			Cwd:        cwd,
			Provider:   provider,
			SortTs:     sortTs,
			Rank:       rank,
			DesktopCwd: toDesktopWorkspacePath(cwd),
		})
	}

	result := make([]ProjectThreadInfo, 0, len(roots))
	for _, root := range roots {
		normalizedRoot := normalizeComparablePath(root)
		exactRoot := toDesktopWorkspacePath(root)

		var matchingRows []rankedRow
		for _, row := range rankedRows {
			if normalizeComparablePath(row.Cwd) == normalizedRoot || normalizeComparablePath(row.DesktopCwd) == normalizedRoot {
				matchingRows = append(matchingRows, row)
			}
		}

		ranks := make([]int, len(matchingRows))
		providerCounts := make(ProviderCounts)
		exactCwdMatches := 0
		verbatimCwdRows := 0

		for i, row := range matchingRows {
			ranks[i] = row.Rank
			p := row.Provider
			if p == "" {
				p = "(missing)"
			}
			providerCounts[p]++
			if row.Cwd == exactRoot || row.DesktopCwd == exactRoot {
				exactCwdMatches++
			}
			if strings.HasPrefix(row.Cwd, `\\?\`) {
				verbatimCwdRows++
			}
		}

		topRank := 0
		firstPageCount := 0
		for _, r := range ranks {
			if topRank == 0 || r < topRank {
				topRank = r
			}
			if r <= pageSize {
				firstPageCount++
			}
		}

		result = append(result, ProjectThreadInfo{
			Root:               exactRoot,
			InteractiveThreads: len(matchingRows),
			FirstPageThreads:   firstPageCount,
			ExactCwdMatches:    exactCwdMatches,
			VerbatimCwdRows:    verbatimCwdRows,
			TopRank:            topRank,
			Ranks:              ranks,
			RankPreview:        formatRankPreview(ranks),
			ProviderCounts:     providerCounts,
		})
	}

	return result, nil
}

func formatRankPreview(ranks []int) string {
	maxCount := 12
	if len(ranks) == 0 {
		return "(none)"
	}
	parts := make([]string, 0, maxCount)
	for i, r := range ranks {
		if i >= maxCount {
			break
		}
		parts = append(parts, fmt.Sprintf("%d", r))
	}
	preview := strings.Join(parts, ", ")
	if len(ranks) > maxCount {
		preview += fmt.Sprintf(" (+%d more)", len(ranks)-maxCount)
	}
	return preview
}

// ---------- helpers ----------

func toAnySlice(strs []string) []any {
	result := make([]any, len(strs))
	for i, s := range strs {
		result[i] = s
	}
	return result
}

func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
