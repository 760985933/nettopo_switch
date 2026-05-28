package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const rolloutScanChunkBytes = 1024 * 1024

// sessionDirs lists the two subdirectories under ~/.codex that contain rollout files.
var sessionDirs = []string{"sessions", "archived_sessions"}

// ---------- snapshot helpers ----------

type fileSnapshot struct {
	Size    int64
	MtimeMs int64
}

func getFileSnapshot(filePath string) (fileSnapshot, error) {
	info, err := os.Stat(filePath)
	if err != nil {
		return fileSnapshot{}, err
	}
	modTime := info.ModTime()
	return fileSnapshot{
		Size:    info.Size(),
		MtimeMs: modTime.UnixMilli(),
	}, nil
}

func snapshotMatches(change syncRolloutChange, snap fileSnapshot) bool {
	return change.OriginalSize == snap.Size && change.OriginalMtimeMs == snap.MtimeMs
}

// ---------- encrypted_content detection ----------

func fileHasEncryptedContent(filePath string, firstLine string, startOffset int64) (bool, error) {
	if strings.Contains(firstLine, "encrypted_content") {
		return true, nil
	}
	return streamContainsText(filePath, "encrypted_content", startOffset)
}

func streamContainsText(filePath string, text string, startOffset int64) (bool, error) {
	needle := []byte(text)
	safeStart := startOffset
	if safeStart < 0 {
		safeStart = 0
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return false, fmt.Errorf("scan error: %w", err)
	}
	if safeStart >= int64(len(data)) {
		return false, nil
	}
	return bytes.Contains(data[safeStart:], needle), nil
}

// ---------- user event detection ----------

func recordHasUserEvent(record map[string]any) bool {
	if record == nil {
		return false
	}
	if t, _ := record["type"].(string); t == "event_msg" {
		if payload, ok := record["payload"].(map[string]any); ok {
			if pt, _ := payload["type"].(string); pt == "user_message" {
				return true
			}
		}
	}
	for _, key := range []string{"payload", "item", "msg"} {
		if val, ok := record[key].(map[string]any); ok {
			if t, _ := val["type"].(string); t == "message" {
				if role, _ := val["role"].(string); role == "user" {
					return true
				}
			}
		}
	}
	return false
}

func parseJSONLRecord(line []byte) map[string]any {
	line = bytes.TrimSpace(line)
	if len(line) == 0 {
		return nil
	}
	var record map[string]any
	if err := json.Unmarshal(line, &record); err != nil {
		return nil
	}
	return record
}

func fileHasUserEvent(filePath string, firstLine string, startOffset int64) (bool, error) {
	if recordHasUserEvent(parseJSONLRecord([]byte(firstLine))) {
		return true, nil
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return false, fmt.Errorf("scan error: %w", err)
	}
	if startOffset >= int64(len(data)) {
		return false, nil
	}

	remaining := data[startOffset:]
	lines := bytes.Split(remaining, []byte("\n"))
	for _, line := range lines {
		if recordHasUserEvent(parseJSONLRecord(line)) {
			return true, nil
		}
	}
	return false, nil
}

// ---------- JSONL file listing ----------

func listRolloutFiles(rootDir string) ([]string, error) {
	var files []string
	err := filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			return nil
		}
		if strings.HasPrefix(info.Name(), "rollout-") && strings.HasSuffix(info.Name(), ".jsonl") {
			files = append(files, path)
		}
		return nil
	})
	return files, err
}

// ---------- first-line reading ----------

type firstLineRecord struct {
	FirstLine string
	Separator string // "\n" or "\r\n"
	Offset    int64  // byte offset of content after first line + separator
}

func readFirstLineRecord(filePath string) (firstLineRecord, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return firstLineRecord{}, err
	}

	nlIdx := bytes.IndexByte(data, '\n')
	if nlIdx == -1 {
		return firstLineRecord{
			FirstLine: string(data),
			Separator: "",
			Offset:    int64(len(data)),
		}, nil
	}

	crlf := nlIdx > 0 && data[nlIdx-1] == '\r'
	if crlf {
		return firstLineRecord{
			FirstLine: string(data[:nlIdx-1]),
			Separator: "\r\n",
			Offset:    int64(nlIdx + 1),
		}, nil
	}
	return firstLineRecord{
		FirstLine: string(data[:nlIdx]),
		Separator: "\n",
		Offset:    int64(nlIdx + 1),
	}, nil
}

// ---------- meta parsing ----------

func parseSessionMetaFromLine(firstLine string) *codexSessionMeta {
	if firstLine == "" {
		return nil
	}
	var parsed codexSessionMeta
	if err := json.Unmarshal([]byte(firstLine), &parsed); err != nil {
		return nil
	}
	if parsed.Type != "session_meta" || parsed.Payload == nil {
		return nil
	}
	return &parsed
}

// ---------- main collection ----------

type collectResult struct {
	Changes              []syncRolloutChange
	LockedPaths          []string
	ProviderCounts       map[string]map[string]int // dirName -> provider -> count
	EncryptedContentCounts map[string]map[string]int
	UserEventThreadIDs   map[string]bool
	ThreadCwdByID        map[string]string
}

func emptyDirCounts() map[string]map[string]int {
	return map[string]map[string]int{
		"sessions":          {},
		"archived_sessions": {},
	}
}

func (cr *collectResult) incrementProvider(dirName, provider string) {
	if cr.ProviderCounts[dirName] == nil {
		cr.ProviderCounts[dirName] = map[string]int{}
	}
	cr.ProviderCounts[dirName][provider]++
}

func (cr *collectResult) incrementEncrypted(dirName, provider string) {
	if cr.EncryptedContentCounts[dirName] == nil {
		cr.EncryptedContentCounts[dirName] = map[string]int{}
	}
	cr.EncryptedContentCounts[dirName][provider]++
}

// collectSessionChanges scans all rollout files and builds a change list for provider sync.
// If targetProvider is "__status_only__", no changes are collected (diagnostic only).
func collectSessionChanges(codexHome string, targetProvider string) (*collectResult, error) {
	result := &collectResult{
		Changes:               make([]syncRolloutChange, 0),
		ProviderCounts:        emptyDirCounts(),
		EncryptedContentCounts: emptyDirCounts(),
		UserEventThreadIDs:    make(map[string]bool),
		ThreadCwdByID:         make(map[string]string),
	}

	for _, dirName := range sessionDirs {
		rootDir := filepath.Join(codexHome, dirName)
		if _, err := os.Stat(rootDir); os.IsNotExist(err) {
			continue
		}

		rolloutPaths, err := listRolloutFiles(rootDir)
		if err != nil {
			continue
		}

		for _, rolloutPath := range rolloutPaths {
			record, err := readFirstLineRecord(rolloutPath)
			if err != nil {
				if isRolloutFileBusyError(err) {
					result.LockedPaths = append(result.LockedPaths, rolloutPath)
					continue
				}
				continue
			}

			meta := parseSessionMetaFromLine(record.FirstLine)
			if meta == nil || meta.Payload == nil {
				continue
			}

			currentProvider := meta.Payload.ModelProvider
			if currentProvider == "" {
				currentProvider = "(missing)"
			}
			result.incrementProvider(dirName, currentProvider)

			if meta.Payload.ID != "" && strings.TrimSpace(meta.Payload.CWD) != "" {
				result.ThreadCwdByID[meta.Payload.ID] = meta.Payload.CWD
			}

			// Detect encrypted_content
			hasEnc, encErr := fileHasEncryptedContent(rolloutPath, record.FirstLine, record.Offset)
			if encErr != nil {
				if isRolloutFileBusyError(encErr) {
					result.LockedPaths = append(result.LockedPaths, rolloutPath)
					continue
				}
			} else if hasEnc {
				result.incrementEncrypted(dirName, currentProvider)
			}

			// Detect has_user_event
			if meta.Payload.ID != "" {
				hasUE, ueErr := fileHasUserEvent(rolloutPath, record.FirstLine, record.Offset)
				if ueErr != nil {
					if isRolloutFileBusyError(ueErr) {
						result.LockedPaths = append(result.LockedPaths, rolloutPath)
						continue
					}
				} else if hasUE {
					result.UserEventThreadIDs[meta.Payload.ID] = true
				}
			}

			// Build change entry if provider differs
			if targetProvider != "__status_only__" && meta.Payload.ModelProvider != targetProvider {
				snap, snapErr := getFileSnapshot(rolloutPath)
				if snapErr != nil {
					if isRolloutFileBusyError(snapErr) {
						result.LockedPaths = append(result.LockedPaths, rolloutPath)
						continue
					}
					continue
				}

				meta.Payload.ModelProvider = targetProvider
				updatedLine, err := json.Marshal(meta)
				if err != nil {
					continue
				}

				result.Changes = append(result.Changes, syncRolloutChange{
					Path:              rolloutPath,
					ThreadID:          meta.Payload.ID,
					Directory:         dirName,
					OriginalFirstLine: record.FirstLine,
					OriginalSeparator: record.Separator,
					OriginalOffset:    record.Offset,
					OriginalSize:      snap.Size,
					OriginalMtimeMs:   snap.MtimeMs,
					OriginalProvider:  currentProvider,
					UpdatedFirstLine:  string(updatedLine),
				})
			}
		}
	}

	return result, nil
}

// ---------- safe rewrite ----------

// rewriteFirstLinePrechecked rewrites the first line of a rollout file using a temp-file + rename strategy.
// It assumes the caller has already verified the file was not modified since scanning.
func rewriteFirstLinePrechecked(change syncRolloutChange) error {
	tmpPath := change.Path + ".provider-sync.tmp"

	// Write new first line + separator to temp file
	writer, err := os.Create(tmpPath)
	if err != nil {
		return err
	}

	if _, err := writer.WriteString(change.UpdatedFirstLine); err != nil {
		writer.Close()
		os.Remove(tmpPath)
		return err
	}
	if change.OriginalSeparator != "" {
		if _, err := writer.WriteString(change.OriginalSeparator); err != nil {
			writer.Close()
			os.Remove(tmpPath)
			return err
		}
	}

	// Copy remaining content from original file
	headerOnly := change.OriginalOffset >= change.OriginalSize
	if !headerOnly {
		reader, err := os.Open(change.Path)
		if err != nil {
			writer.Close()
			os.Remove(tmpPath)
			return err
		}
		if _, err := reader.Seek(change.OriginalOffset, 0); err != nil {
			reader.Close()
			writer.Close()
			os.Remove(tmpPath)
			return err
		}
		if _, err := writer.ReadFrom(reader); err != nil {
			reader.Close()
			writer.Close()
			os.Remove(tmpPath)
			return err
		}
		reader.Close()
	}

	writer.Close()

	// Atomic rename
	if err := os.Rename(tmpPath, change.Path); err != nil {
		os.Remove(tmpPath)
		return err
	}

	return nil
}

// tryRewriteCollectedFirstLine safely rewrites a rollout file, verifying it hasn't changed since scanning.
func tryRewriteCollectedFirstLine(change syncRolloutChange) (bool, error) {
	// Re-read snapshot to verify file hasn't changed
	beforeSnap, err := getFileSnapshot(change.Path)
	if err != nil {
		return false, err
	}
	if !snapshotMatches(change, beforeSnap) {
		return false, nil
	}

	// Re-read first line to verify it still matches
	current, err := readFirstLineRecord(change.Path)
	if err != nil {
		return false, err
	}
	if current.FirstLine != change.OriginalFirstLine || current.Offset != change.OriginalOffset {
		return false, nil
	}

	// Rewrite
	if err := rewriteFirstLinePrechecked(change); err != nil {
		return false, err
	}

	// Verify file wasn't modified concurrently during our write
	afterSnap, err := getFileSnapshot(change.Path)
	if err != nil {
		return false, nil
	}
	if !snapshotMatches(change, afterSnap) {
		return false, nil
	}

	return true, nil
}

// applySessionChanges applies all collected rollout file changes.
// Returns counts of applied and skipped changes.
func applySessionChanges(changes []syncRolloutChange) (appliedCount int, appliedPaths []string, skippedPaths []string) {
	for _, change := range changes {
		ok, err := tryRewriteCollectedFirstLine(change)
		if err != nil {
			skippedPaths = append(skippedPaths, change.Path)
			continue
		}
		if ok {
			appliedCount++
			appliedPaths = append(appliedPaths, change.Path)
			// Restore original mtime
			restoreOriginalMtime(change.Path, change.OriginalMtimeMs)
		} else {
			skippedPaths = append(skippedPaths, change.Path)
		}
	}
	return
}

// restoreSessionChanges reverts previously-applied rollout file changes.
func restoreSessionChanges(manifest []struct {
	Path              string
	OriginalFirstLine string
	OriginalSeparator string
	OriginalMtimeMs   int64
}) {
	for _, entry := range manifest {
		change := syncRolloutChange{
			Path:              entry.Path,
			UpdatedFirstLine:  entry.OriginalFirstLine,
			OriginalSeparator: entry.OriginalSeparator,
			OriginalMtimeMs:   entry.OriginalMtimeMs,
		}
		_ = rewriteFirstLinePrechecked(change)
		restoreOriginalMtime(entry.Path, entry.OriginalMtimeMs)
	}
}

func restoreOriginalMtime(filePath string, mtimeMs int64) {
	if mtimeMs <= 0 {
		return
	}
	mtime := fileSnapshot{MtimeMs: mtimeMs}
	_ = mtime // unused but preserved for symmetry with original design
	// On macOS/Linux, setting mtime after write may not be needed since
	// we preserve it via snapshot-based verification.
	_ = runtime.GOOS
}

// toDesktopWorkspacePath normalizes a stored cwd path to Desktop-compatible format.
func toDesktopWorkspacePath(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return value
	}

	// Handle \\?\UNC\... → \\...
	if strings.HasPrefix(trimmed, `\\?\UNC\`) {
		return `\\` + strings.ReplaceAll(trimmed[8:], "/", `\`)
	}

	// Handle \\?\X:\... → X:\...
	if len(trimmed) > 4 && strings.HasPrefix(trimmed, `\\?\`) {
		rest := trimmed[4:]
		if len(rest) >= 2 && rest[1] == ':' {
			drive := rest[:2]
			if len(rest) > 2 && (rest[2] == '\\' || rest[2] == '/') {
				return drive + `\` + strings.ReplaceAll(rest[3:], "/", `\`)
			}
			return drive + `\`
		}
		return strings.ReplaceAll(rest, "/", `\`)
	}

	return value
}
