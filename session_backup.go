package main

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// ---------- backup utility ----------

// createTarBackup creates a simple tar-like backup of JSONL files from the given dirs.
// Format: name|size\ncontent ... CHECKSUM|hex\n
func createTarBackup(tarPath string, dirs []string) error {
	hasFiles := false
	for _, dir := range dirs {
		if entries, err := os.ReadDir(dir); err == nil && len(entries) > 0 {
			hasFiles = true
			break
		}
	}
	if !hasFiles {
		return os.WriteFile(tarPath, []byte{}, 0o600)
	}

	var buf bytes.Buffer
	for _, dir := range dirs {
		_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() || !strings.HasSuffix(info.Name(), ".jsonl") {
				return nil
			}
			data, readErr := os.ReadFile(path)
			if readErr != nil {
				return nil
			}
			relPath, _ := filepath.Rel(filepath.Dir(dir), path)
			if _, writeErr := fmt.Fprintf(&buf, "%s|%d\n", relPath, len(data)); writeErr != nil {
				return writeErr
			}
			if _, writeErr := buf.Write(data); writeErr != nil {
				return writeErr
			}
			return nil
		})
	}

	hash := sha256.Sum256(buf.Bytes())
	if _, err := fmt.Fprintf(&buf, "CHECKSUM|%x\n", hash); err != nil {
		return err
	}

	return os.WriteFile(tarPath, buf.Bytes(), 0o600)
}

// ---------- Wails bindings ----------

// ListCodexSessionBackups 列出可用的会话迁移备份文件
func (a *App) ListCodexSessionBackups() ([]string, error) {
	backupDir, err := codexBackupDir()
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(backupDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, err
	}

	backups := make([]string, 0)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if strings.HasPrefix(entry.Name(), "sessions_backup_") && strings.HasSuffix(entry.Name(), ".tar") {
			backups = append(backups, filepath.Join(backupDir, entry.Name()))
		}
	}

	sort.Slice(backups, func(i, j int) bool {
		return backups[i] > backups[j]
	})

	return backups, nil
}

// RestoreCodexSessions 从备份文件恢复会话到原始位置
func (a *App) RestoreCodexSessions(backupPath string) (*MigrationResult, error) {
	result := &MigrationResult{}

	if _, err := os.Stat(backupPath); err != nil {
		result.Error = fmt.Sprintf("备份文件不存在: %s", backupPath)
		return result, fmt.Errorf("备份文件不存在: %s", backupPath)
	}

	backupDir, err := codexBackupDir()
	if err != nil {
		result.Error = err.Error()
		return result, err
	}
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		result.Error = err.Error()
		return result, err
	}
	preRestoreBackup := filepath.Join(backupDir,
		fmt.Sprintf("pre_restore_%s.tar", time.Now().Format("20060102_150405")))

	var sessionDirs []string
	if dir, err := codexSessionsDir(); err == nil {
		sessionDirs = append(sessionDirs, dir)
	}
	if dir, err := codexArchivedSessionsDir(); err == nil {
		sessionDirs = append(sessionDirs, dir)
	}
	if err := createTarBackup(preRestoreBackup, sessionDirs); err != nil {
		result.Error = fmt.Sprintf("恢复前备份失败: %v", err)
		return result, err
	}

	raw, err := os.ReadFile(backupPath)
	if err != nil {
		result.Error = fmt.Sprintf("读取备份文件失败: %v", err)
		return result, err
	}

	home, err := os.UserHomeDir()
	if err != nil {
		result.Error = err.Error()
		return result, err
	}
	codexRoot := filepath.Join(home, ".codex")

	data := string(raw)
	lines := strings.Split(data, "\n")

	var i int
	for i < len(lines) {
		line := lines[i]
		if strings.HasPrefix(line, "CHECKSUM|") {
			break
		}
		if !strings.Contains(line, "|") {
			i++
			continue
		}

		parts := strings.SplitN(line, "|", 2)
		if len(parts) != 2 {
			i++
			continue
		}
		relPath := parts[0]
		size := 0
		if _, err := fmt.Sscanf(parts[1], "%d", &size); err != nil || size <= 0 {
			i++
			continue
		}

		if i+1 >= len(lines) {
			break
		}
		i++
		content := ""
		if i < len(lines) {
			content = strings.Join(lines[i:], "\n")
			if len(content) > size {
				content = content[:size]
			}
			i += strings.Count(content, "\n")
		}

		targetPath := filepath.Join(codexRoot, relPath)
		if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
			continue
		}
		if err := os.WriteFile(targetPath, []byte(content), 0o600); err != nil {
			continue
		}

		result.MigratedCount++
		i++
	}

	result.BackupPath = preRestoreBackup
	return result, nil
}

// DeleteCodexSessionBackup 删除指定的会话备份文件
func (a *App) DeleteCodexSessionBackup(backupPath string) (string, error) {
	if err := os.Remove(backupPath); err != nil {
		return "", err
	}
	return backupPath, nil
}
