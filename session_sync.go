package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	toml "github.com/pelletier/go-toml/v2"
)

const (
	defaultProvider   = "openai"
	syncBackupSubDir  = "provider-sync"
)

// ---------- status / diagnostic ----------

// GetSyncStatus returns a full diagnostic snapshot of Codex session state.
func (a *App) GetSyncStatus() (*SyncStatusResult, error) {
	codexHome, err := codexHomeDir()
	if err != nil {
		return nil, err
	}

	configPath := filepath.Join(codexHome, "config.toml")
	configText, err := os.ReadFile(configPath)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}

	currentProvider, implicit := readCurrentProviderFromConfig(configText)
	configuredProviders := listConfiguredProvidersFromConfig(configText)

	collected, err := collectSessionChanges(codexHome, "__status_only__")
	if err != nil {
		return nil, err
	}

	sqliteCounts, sqliteUnreadable, sqliteErr := readSQLiteProviderCounts(codexHome)

	var repairStats *SyncRepairStats
	var projectVisibility []ProjectThreadInfo

	if !sqliteUnreadable && sqliteCounts != nil {
		repairStats, _ = readSQLiteRepairStats(codexHome, collected.UserEventThreadIDs, collected.ThreadCwdByID)
		projectVisibility, _ = readProjectThreadVisibility(codexHome)
	}

	backupRoot := filepath.Join(codexHome, "backups_state", syncBackupSubDir)
	backupCount := countSyncBackups(backupRoot)

	lockedPaths := collected.LockedPaths
	if lockedPaths == nil {
		lockedPaths = []string{}
	}

	result := &SyncStatusResult{
		CodexHome:               codexHome,
		CurrentProvider:         currentProvider,
		CurrentProviderImplicit:  implicit,
		ConfiguredProviders:     configuredProviders,
		RolloutCounts: SyncRolloutInfo{
			Sessions:         collected.ProviderCounts["sessions"],
			ArchivedSessions: collected.ProviderCounts["archived_sessions"],
		},
		LockedRolloutFiles: lockedPaths,
		SQLiteCounts:       sqliteCounts,
		SQLiteUnreadable:   sqliteUnreadable,
		SQLiteError:        errString(sqliteErr),
		SQLiteRepairStats:  repairStats,
		ProjectThreadVisibility: projectVisibility,
		BackupRoot:             backupRoot,
		BackupCount:            backupCount,
	}

	// Build encrypted content warning
	if collected.EncryptedContentCounts != nil {
		ec := &SyncRolloutInfo{
			Sessions:         collected.EncryptedContentCounts["sessions"],
			ArchivedSessions: collected.EncryptedContentCounts["archived_sessions"],
		}
		result.EncryptedContentCounts = ec
		result.EncryptedContentWarning = buildEncryptedWarning(ec, currentProvider)
	}

	return result, nil
}

// ---------- sync ----------

// RunSync executes a full provider sync: rollout files, SQLite, global state.
func (a *App) RunSync(targetProvider string) (*SyncResult, error) {
	if strings.TrimSpace(targetProvider) == "" {
		targetProvider = defaultProvider
	}

	codexHome, err := codexHomeDir()
	if err != nil {
		return nil, err
	}

	configPath := filepath.Join(codexHome, "config.toml")
	configText, err := os.ReadFile(configPath)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}

	current, _ := readCurrentProviderFromConfig(configText)
	if current == "" {
		current = defaultProvider
	}

	// 1. Scan rollout files
	collected, err := collectSessionChanges(codexHome, targetProvider)
	if err != nil {
		return nil, err
	}

	// 2. Split locked vs writable
	writable, locked := splitLockedChanges(collected.Changes)

	// 3. Check SQLite is writable
	_, err = assertSQLiteWritable(codexHome)
	if err != nil {
		return nil, err
	}

	// 4. Create backup
	backupDir, err := createSyncBackup(codexHome)
	if err != nil {
		return nil, fmt.Errorf("backup failed: %w", err)
	}

	backupStart := time.Now()

	// 5. Apply rollout changes and SQLite sync atomically
	appliedCount := 0
	appliedPaths := make([]string, 0)
	skippedPaths := make([]string, 0)

	cwdStats, _ := readThreadCwdStats(codexHome)

	sqliteResult, sqliteErr := syncSQLite(codexHome, targetProvider, sqliteSyncOptions{
		UserEventThreadIDs: collected.UserEventThreadIDs,
		ThreadCwdByID:      collected.ThreadCwdByID,
	}, func(sr sqliteSyncResult) error {
		// Apply rollout changes inside the SQLite transaction callback
		if len(writable) > 0 {
			appliedCount, appliedPaths, skippedPaths = applySessionChanges(writable)
		}
		return nil
	})

	if sqliteErr != nil {
		// Restore rollout files that were changed
		type restoreEntry struct {
			Path              string
			OriginalFirstLine string
			OriginalSeparator string
			OriginalMtimeMs   int64
		}
		entries := make([]restoreEntry, 0)
		for _, c := range writable {
			for _, p := range appliedPaths {
				if p == c.Path {
					entries = append(entries, restoreEntry{
						Path:              c.Path,
						OriginalFirstLine: c.OriginalFirstLine,
						OriginalSeparator: c.OriginalSeparator,
						OriginalMtimeMs:   c.OriginalMtimeMs,
					})
					break
				}
			}
		}
		for _, e := range entries {
			change := syncRolloutChange{
				Path:              e.Path,
				UpdatedFirstLine:  e.OriginalFirstLine,
				OriginalSeparator: e.OriginalSeparator,
				OriginalMtimeMs:   e.OriginalMtimeMs,
			}
			_ = rewriteFirstLinePrechecked(change)
		}
		return nil, sqliteErr
	}

	// 6. Sync global state workspace roots
	wsUpdated, wsChangedCount, wsSavedCount, wsErr := syncWorkspaceRoots(codexHome, cwdStats)
	if wsErr != nil {
		a.appendLog("warn", "sync", "全局状态同步失败: "+wsErr.Error(), "")
	}
	_ = wsUpdated

	lockedPaths := lockedFilePaths(locked)
	skippedLocked := append(lockedPaths, skippedPaths...)

	var encWarning string
	if collected.EncryptedContentCounts != nil {
		ec := &SyncRolloutInfo{
			Sessions:         collected.EncryptedContentCounts["sessions"],
			ArchivedSessions: collected.EncryptedContentCounts["archived_sessions"],
		}
		encWarning = buildEncryptedWarning(ec, targetProvider)
	}

	backupDurationMs := time.Since(backupStart).Milliseconds()

	result := &SyncResult{
		CodexHome:                 codexHome,
		TargetProvider:            targetProvider,
		PreviousProvider:          current,
		BackupDir:                 backupDir,
		BackupDurationMs:          backupDurationMs,
		ChangedSessionFiles:       appliedCount,
		SkippedLockedFiles:        skippedLocked,
		SQLiteRowsUpdated:         sqliteResult.TotalUpdated,
		SQLiteProviderRowsUpdated: sqliteResult.ProviderRowsUpdated,
		SQLiteUserEventRowsUpdated: sqliteResult.UserEventRowsUpdated,
		SQLiteCwdRowsUpdated:      sqliteResult.CwdRowsUpdated,
		UpdatedWorkspaceRoots:     wsChangedCount,
		SavedWorkspaceRootCount:   wsSavedCount,
		SQLitePresent:              sqliteResult.DatabasePresent,
		EncryptedContentWarning:   encWarning,
	}

	a.appendLog("info", "sync", fmt.Sprintf(
		"同步完成: provider=%s, rollout=%d, sqlite=%d, user_event=%d, cwd=%d, workspace_roots=%d",
		targetProvider, appliedCount, sqliteResult.ProviderRowsUpdated,
		sqliteResult.UserEventRowsUpdated, sqliteResult.CwdRowsUpdated, wsChangedCount,
	), "")

	return result, nil
}

// ---------- backward-compatible wrappers ----------

// MigrateCodexProviders 一键迁移所有会话的 model_provider（增强版）
// from: 旧 provider 名称（如 "Local"）
// to:   新 provider 名称（如 "openai"）
func (a *App) MigrateCodexProviders(from, to string) (*MigrationResult, error) {
	result := &MigrationResult{}

	if from == "" || to == "" {
		result.Error = "from 和 to 不能为空"
		return result, fmt.Errorf("from 和 to 不能为空")
	}
	if from == to {
		result.Error = "from 和 to 不能相同"
		return result, fmt.Errorf("from 和 to 不能相同")
	}

	// 1. Backup sessions directories (preserve existing behavior)
	backupDir, err := codexBackupDir()
	if err != nil {
		result.Error = err.Error()
		return result, err
	}
	if err = os.MkdirAll(backupDir, 0o755); err != nil {
		result.Error = err.Error()
		return result, err
	}
	backupPath := filepath.Join(backupDir,
		fmt.Sprintf("sessions_backup_%s.tar", time.Now().Format("20060102_150405")))

	var sessionDirs []string
	if d, dErr := codexSessionsDir(); dErr == nil {
		sessionDirs = append(sessionDirs, d)
	}
	if d, dErr := codexArchivedSessionsDir(); dErr == nil {
		sessionDirs = append(sessionDirs, d)
	}

	if err = createTarBackup(backupPath, sessionDirs); err != nil {
		result.Error = fmt.Sprintf("备份失败: %v", err)
		return result, err
	}
	result.BackupPath = backupPath

	// 2. Run enhanced sync
	syncResult, err := a.RunSync(to)
	if err != nil {
		result.Error = fmt.Sprintf("同步失败: %v", err)
		return result, err
	}

	result.MigratedCount = syncResult.ChangedSessionFiles + syncResult.SQLiteRowsUpdated
	return result, nil
}

// MigrateSingleCodexSession 迁移单个会话的 model_provider（增强版）
func (a *App) MigrateSingleCodexSession(id, to string) (*CodexSession, error) {
	path, _, err := a.findSessionFile(id)
	if err != nil {
		return nil, err
	}

	session, _, err := parseSessionFile(path)
	if err != nil {
		return nil, err
	}

	from := session.ModelProvider
	if from == to {
		return nil, fmt.Errorf("该会话的 provider 已经是 %s", to)
	}

	// Use the enhanced rollout file rewrite
	record, err := readFirstLineRecord(path)
	if err != nil {
		return nil, err
	}

	meta := parseSessionMetaFromLine(record.FirstLine)
	if meta == nil || meta.Payload == nil {
		return nil, fmt.Errorf("无法解析会话元信息")
	}

	snap, err := getFileSnapshot(path)
	if err != nil {
		return nil, err
	}

	meta.Payload.ModelProvider = to
	updatedLine, err := jsonMarshal(meta)
	if err != nil {
		return nil, err
	}

	change := syncRolloutChange{
		Path:              path,
		ThreadID:          meta.Payload.ID,
		OriginalFirstLine: record.FirstLine,
		OriginalSeparator: record.Separator,
		OriginalOffset:    record.Offset,
		OriginalSize:      snap.Size,
		OriginalMtimeMs:   snap.MtimeMs,
		OriginalProvider:  from,
		UpdatedFirstLine:  updatedLine,
	}

	ok, err := tryRewriteCollectedFirstLine(change)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("迁移失败：无法更新会话文件（文件可能被占用或已变更）")
	}

	// Repair SQLite: model_provider + has_user_event + cwd for this session
	codexHome, _ := codexHomeDir()
	hasUE := false
	if ue, ueErr := fileHasUserEvent(path, record.FirstLine, record.Offset); ueErr == nil && ue {
		hasUE = true
	}
	cwd := ""
	if meta.Payload.CWD != "" {
		cwd = meta.Payload.CWD
	}

	userEventMap := map[string]bool{}
	if hasUE {
		userEventMap[id] = true
	}
	cwdMap := map[string]string{}
	if cwd != "" {
		cwdMap[id] = cwd
	}

	sqliteResult, sqliteErr := syncSQLite(codexHome, to, sqliteSyncOptions{
		UserEventThreadIDs: userEventMap,
		ThreadCwdByID:      cwdMap,
	}, nil)

	if sqliteErr != nil {
		a.appendLog("warn", "app", fmt.Sprintf("会话 %s SQLite 修复失败: %v", id, sqliteErr), "")
	} else if sqliteResult.TotalUpdated > 0 {
		a.appendLog("info", "app", fmt.Sprintf("会话 %s SQLite 修复 %d 条记录", id, sqliteResult.TotalUpdated), "")
	}

	// Re-parse and return
	updated, _, err := parseSessionFile(path)
	if err != nil {
		return nil, err
	}
	return updated, nil
}

// ---------- config reading helpers ----------

func codexHomeDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".codex"), nil
}

func readCurrentProviderFromConfig(configBytes []byte) (string, bool) {
	if len(configBytes) == 0 {
		return defaultProvider, true
	}
	var doc map[string]any
	if err := toml.Unmarshal(configBytes, &doc); err != nil {
		return defaultProvider, true
	}
	if provider, ok := doc["model_provider"].(string); ok && strings.TrimSpace(provider) != "" {
		return strings.TrimSpace(provider), false
	}
	return defaultProvider, true
}

func listConfiguredProvidersFromConfig(configBytes []byte) []string {
	if len(configBytes) == 0 {
		return nil
	}
	var doc map[string]any
	if err := toml.Unmarshal(configBytes, &doc); err != nil {
		return nil
	}

	seen := make(map[string]bool)
	seen["openai"] = true // built-in
	providers := []string{"openai"}

	if mp, ok := doc["model_providers"].(map[string]any); ok {
		for id := range mp {
			if !seen[id] {
				seen[id] = true
				providers = append(providers, id)
			}
		}
	}

	sort.Strings(providers[1:]) // keep openai first
	return providers
}

// ---------- sync backup ----------

func countSyncBackups(root string) int {
	entries, err := os.ReadDir(root)
	if err != nil {
		return 0
	}
	count := 0
	for _, e := range entries {
		if e.IsDir() {
			count++
		}
	}
	return count
}

func createSyncBackup(codexHome string) (string, error) {
	root := filepath.Join(codexHome, "backups_state", syncBackupSubDir)
	ts := time.Now().UTC().Format("20060102T150405Z")
	dir := filepath.Join(root, ts)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}

	// Copy config.toml
	configPath := filepath.Join(codexHome, "config.toml")
	if configBytes, err := os.ReadFile(configPath); err == nil {
		_ = os.WriteFile(filepath.Join(dir, "config.toml"), configBytes, 0o600)
	}

	// Copy state_5.sqlite
	dbPath := stateDBPath(codexHome)
	if dbBytes, err := os.ReadFile(dbPath); err == nil {
		_ = os.WriteFile(filepath.Join(dir, dbFileBasename), dbBytes, 0o600)
	}

	// Copy .codex-global-state.json
	gsPath := globalStatePath(codexHome)
	if gsBytes, err := os.ReadFile(gsPath); err == nil {
		_ = os.WriteFile(filepath.Join(dir, globalStateFileBasename), gsBytes, 0o600)
	}

	return dir, nil
}

func buildEncryptedWarning(counts *SyncRolloutInfo, targetProvider string) string {
	if counts == nil {
		return ""
	}
	type pair struct {
		provider string
		count    int
	}
	var risky []pair
	for provider, count := range counts.Sessions {
		if count > 0 && provider != targetProvider {
			risky = append(risky, pair{provider, count})
		}
	}
	for provider, count := range counts.ArchivedSessions {
		if count > 0 && provider != targetProvider {
			found := false
			for _, r := range risky {
				if r.provider == provider {
					found = true
					break
				}
			}
			if !found {
				risky = append(risky, pair{provider, count})
			}
		}
	}
	if len(risky) == 0 {
		return ""
	}
	var providers []string
	for _, r := range risky {
		providers = append(providers, r.provider)
	}
	sort.Strings(providers)
	total := 0
	for _, c := range counts.Sessions {
		total += c
	}
	for _, c := range counts.ArchivedSessions {
		total += c
	}
	return fmt.Sprintf(
		"Encrypted content warning: %d rollout file(s) contain encrypted_content from provider(s) %s. Visibility metadata can be synchronized, but continuing or compacting those histories may fail with invalid_encrypted_content.",
		total, strings.Join(providers, ", "),
	)
}

func lockedFilePaths(locked []syncRolloutChange) []string {
	if len(locked) == 0 {
		return nil
	}
	seen := make(map[string]bool)
	var result []string
	for _, c := range locked {
		if !seen[c.Path] {
			seen[c.Path] = true
			result = append(result, c.Path)
		}
	}
	sort.Strings(result)
	return result
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func jsonMarshal(v any) (string, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(data), nil
}
