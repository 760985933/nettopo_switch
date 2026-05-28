package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// HasLegacySessions 检查是否存在 model_provider = "Local" 的旧会话
func (a *App) HasLegacySessions() (bool, error) {
	sessions, err := a.ListCodexSessions()
	if err != nil {
		return false, err
	}
	for _, s := range sessions {
		if s.ModelProvider == "Local" {
			return true, nil
		}
	}
	return false, nil
}

// CountLegacySessions 返回旧格式会话数量
func (a *App) CountLegacySessions() (int, error) {
	sessions, err := a.ListCodexSessions()
	if err != nil {
		return 0, err
	}
	count := 0
	for _, s := range sessions {
		if s.ModelProvider == "Local" {
			count++
		}
	}
	return count, nil
}

// ListCodexSessionProviders 返回所有会话中不同的 model_provider 列表
func (a *App) ListCodexSessionProviders() ([]string, error) {
	sessions, err := a.ListCodexSessions()
	if err != nil {
		return nil, err
	}
	seen := make(map[string]struct{})
	providers := make([]string, 0)
	for _, s := range sessions {
		if s.ModelProvider == "" {
			continue
		}
		if _, ok := seen[s.ModelProvider]; ok {
			continue
		}
		seen[s.ModelProvider] = struct{}{}
		providers = append(providers, s.ModelProvider)
	}
	sort.Strings(providers)
	return providers, nil
}

// findSessionFile 在 sessions 和 archived_sessions 目录中按 ID 查找文件路径
func (a *App) findSessionFile(id string) (string, bool, error) {
	matchFilename := func(name string) bool {
		trimmed := strings.TrimSuffix(name, ".jsonl")
		return trimmed == id || strings.HasSuffix(trimmed, "-"+id)
	}

	if dir, err := codexSessionsDir(); err == nil {
		var found string
		_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() || !strings.HasSuffix(info.Name(), ".jsonl") {
				return nil
			}
			if matchFilename(info.Name()) {
				found = path
				return filepath.SkipDir
			}
			return nil
		})
		if found != "" {
			return found, false, nil
		}
	}

	if dir, err := codexArchivedSessionsDir(); err == nil {
		entries, err := os.ReadDir(dir)
		if err == nil {
			for _, entry := range entries {
				if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") {
					continue
				}
				if matchFilename(entry.Name()) {
					return filepath.Join(dir, entry.Name()), true, nil
				}
			}
		}
	}

	return "", false, fmt.Errorf("未找到会话: %s", id)
}

// DeleteCodexSession 永久删除指定会话文件
func (a *App) DeleteCodexSession(id string) error {
	path, _, err := a.findSessionFile(id)
	if err != nil {
		return err
	}
	return os.Remove(path)
}

// ArchiveCodexSession 将会话移入归档（或从归档移回）
func (a *App) ArchiveCodexSession(id string) (*CodexSession, error) {
	path, isArchived, err := a.findSessionFile(id)
	if err != nil {
		return nil, err
	}

	if isArchived {
		destDir, err := codexSessionsDir()
		if err != nil {
			return nil, err
		}
		today := time.Now().Format("2006/01/02")
		destDir = filepath.Join(destDir, today)
		if err := os.MkdirAll(destDir, 0o755); err != nil {
			return nil, err
		}
		dest := filepath.Join(destDir, filepath.Base(path))
		if err := os.Rename(path, dest); err != nil {
			return nil, err
		}
		session, _, err := parseSessionFile(dest)
		if err != nil {
			return nil, err
		}
		session.IsArchived = false
		return session, nil
	}

	destDir, err := codexArchivedSessionsDir()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return nil, err
	}
	dest := filepath.Join(destDir, filepath.Base(path))
	if err := os.Rename(path, dest); err != nil {
		return nil, err
	}
	session, _, err := parseSessionFile(dest)
	if err != nil {
		return nil, err
	}
	session.IsArchived = true
	return session, nil
}
