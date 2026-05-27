package main

import (
	"bytes"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// ---------- data types ----------

// CodexSession 表示一个 Codex 会话的元信息
type CodexSession struct {
	ID            string `json:"id"`
	Title         string `json:"title"`
	Model         string `json:"model"`
	ModelProvider string `json:"modelProvider"`
	MessageCount  int    `json:"messageCount"`
	CreatedAt     string `json:"createdAt"`
	IsArchived    bool   `json:"isArchived"`
	CWD           string `json:"cwd"`
	filePath      string `json:"-"`
}

// SessionMessage 表示单条对话消息
type SessionMessage struct {
	Role      string `json:"role"`
	Content   string `json:"content"`
	Timestamp string `json:"timestamp"`
}

// SessionDetail 包含会话元信息和完整消息列表
type SessionDetail struct {
	Session  CodexSession     `json:"session"`
	Messages []SessionMessage `json:"messages"`
}

// MigrationResult 表示迁移操作的结果
type MigrationResult struct {
	MigratedCount int    `json:"migratedCount"`
	BackupPath    string `json:"backupPath"`
	Error         string `json:"error,omitempty"`
}

// ---------- paths ----------

func codexSessionsDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".codex", "sessions"), nil
}

func codexArchivedSessionsDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".codex", "archived_sessions"), nil
}

// ---------- JSONL parsing ----------

// codexSessionMeta 是 JSONL 中 type="session_meta" 行的结构
type codexSessionMeta struct {
	Type    string            `json:"type"`
	Payload *codexMetaPayload `json:"payload"`
}

type codexMetaPayload struct {
	ID            string `json:"id"`
	Timestamp     string `json:"timestamp"`
	ModelProvider string `json:"model_provider"`
	CWD           string `json:"cwd"`
}

// codexLine 是 JSONL 中后续行的通用结构
type codexLine struct {
	Type      string          `json:"type"`
	Timestamp string          `json:"timestamp"`
	Payload   json.RawMessage `json:"payload"`
}

// responsePayload 是 type="response_item" 行的 payload 结构
type responsePayload struct {
	Type    string          `json:"type"`
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

// eventPayload 是 type="event_msg" 行的 payload 结构
type eventPayload struct {
	Type    string `json:"type"`
	Message string `json:"message,omitempty"`
}

func parseSessionFile(path string) (*CodexSession, []SessionMessage, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}

	linesList := bytes.Split(raw, []byte("\n"))

	var meta *codexSessionMeta
	messages := make([]SessionMessage, 0)
	firstUserText := ""
	var lastRole, lastContent string

	for _, line := range linesList {
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}

		// 尝试解析为通用行结构，先判断 type
		var cl codexLine
		if err := json.Unmarshal(line, &cl); err != nil {
			continue
		}

		switch cl.Type {
		case "session_meta":
			if err := json.Unmarshal(line, &meta); err != nil {
				continue
			}

		case "response_item":
			var rp responsePayload
			if err := json.Unmarshal(cl.Payload, &rp); err != nil {
				continue
			}
			if rp.Type != "message" {
				continue
			}
			if rp.Role != "user" && rp.Role != "assistant" {
				continue
			}
			text := extractPartsText(rp.Content)
			if text == "" {
				continue
			}
			// 跳过环境上下文块
			if strings.Contains(text, "<environment_context>") {
				continue
			}
			// 去重：跳过与上一条相同角色和内容的消息
			if text == lastContent && rp.Role == lastRole {
				continue
			}
			messages = append(messages, SessionMessage{Role: rp.Role, Content: text, Timestamp: cl.Timestamp})
			lastRole, lastContent = rp.Role, text
			if rp.Role == "user" && firstUserText == "" {
				firstUserText = text
			}

		case "event_msg":
			var ep eventPayload
			if err := json.Unmarshal(cl.Payload, &ep); err != nil {
				continue
			}
			switch ep.Type {
			case "user_message":
				if ep.Message == "" {
					continue
				}
				if ep.Message == lastContent && "user" == lastRole {
					continue
				}
				messages = append(messages, SessionMessage{Role: "user", Content: ep.Message, Timestamp: cl.Timestamp})
				lastRole, lastContent = "user", ep.Message
				if firstUserText == "" {
					firstUserText = ep.Message
				}
			case "agent_message":
				if ep.Message == "" {
					continue
				}
				if ep.Message == lastContent && "assistant" == lastRole {
					continue
				}
				messages = append(messages, SessionMessage{Role: "assistant", Content: ep.Message, Timestamp: cl.Timestamp})
				lastRole, lastContent = "assistant", ep.Message
			}
		}
	}

	if meta == nil || meta.Payload == nil {
		return nil, nil, fmt.Errorf("无效的会话文件: 缺少 session_meta")
	}

	payload := meta.Payload

	// 生成标题：使用第一条用户消息
	title := payload.ID
	if firstUserText != "" {
		title = firstUserText
	}

	session := &CodexSession{
		ID:            payload.ID,
		Title:         title,
		Model:         payload.ModelProvider,
		ModelProvider: payload.ModelProvider,
		MessageCount:  len(messages),
		CreatedAt:     payload.Timestamp,
		IsArchived:    strings.Contains(path, "archived"),
		CWD:           payload.CWD,
		filePath:      path,
	}

	return session, messages, nil
}

// extractPartsText 从 response_item/message 的 content 数组中提取纯文本
// content 格式: [{"type":"input_text","text":"..."}, {"type":"output_text","text":"..."}]
func extractPartsText(raw json.RawMessage) string {
	var parts []map[string]any
	if err := json.Unmarshal(raw, &parts); err != nil {
		// 尝试直接解析为字符串
		var s string
		if err := json.Unmarshal(raw, &s); err == nil {
			return s
		}
		return ""
	}

	var b strings.Builder
	for _, part := range parts {
		partType, _ := part["type"].(string)
		if !strings.HasSuffix(partType, "_text") {
			continue
		}
		if text, ok := part["text"].(string); ok && text != "" {
			if b.Len() > 0 {
				b.WriteString("\n")
			}
			b.WriteString(text)
		}
	}
	return b.String()
}

// ---------- Wails bindings ----------

// loadModelMap 从 SQLite state_*.sqlite 中查询所有线程的 model 名称
// 返回 map[threadID]modelName
func loadModelMap() map[string]string {
	modelMap := make(map[string]string)
	home, err := os.UserHomeDir()
	if err != nil {
		return modelMap
	}
	codexDir := filepath.Join(home, ".codex")
	entries, err := os.ReadDir(codexDir)
	if err != nil {
		return modelMap
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), "state_") || !strings.HasSuffix(entry.Name(), ".sqlite") {
			continue
		}
		dbPath := filepath.Join(codexDir, entry.Name())
		db, err := sql.Open("sqlite3", dbPath)
		if err != nil {
			continue
		}
		rows, err := db.Query("SELECT id, model FROM threads WHERE model IS NOT NULL AND model != ''")
		if err != nil {
			db.Close()
			continue
		}
		for rows.Next() {
			var id, model string
			if err := rows.Scan(&id, &model); err == nil && model != "" {
				modelMap[id] = model
			}
		}
		if err := rows.Err(); err != nil {
			log.Printf("读取会话模型数据失败: %v", err)
		}
		rows.Close()
		db.Close()
	}
	return modelMap
}

// ListCodexSessions 扫描 sessions 和 archived_sessions 目录，返回所有会话列表
func (a *App) ListCodexSessions() ([]CodexSession, error) {
	sessions := make([]CodexSession, 0)

	// 预加载 SQLite 中的模型名
	modelMap := loadModelMap()

	// 扫描 sessions 目录（嵌套子目录，递归搜索）
	if dir, err := codexSessionsDir(); err == nil {
		_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() || !strings.HasSuffix(info.Name(), ".jsonl") {
				return nil
			}
			session, _, err := parseSessionFile(path)
			if err != nil {
				return nil
			}
			// 从 SQLite 中查找真正的模型名
			if m, ok := modelMap[session.ID]; ok {
				session.Model = m
			}
			sessions = append(sessions, *session)
			return nil
		})
	}

	// 扫描 archived_sessions 目录（扁平结构）
	if dir, err := codexArchivedSessionsDir(); err == nil {
		entries, err := os.ReadDir(dir)
		if err != nil {
			if !os.IsNotExist(err) {
				return nil, err
			}
		} else {
			for _, entry := range entries {
				if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") {
					continue
				}
				path := filepath.Join(dir, entry.Name())
				session, _, err := parseSessionFile(path)
				if err != nil {
					continue
				}
				// 从 SQLite 中查找真正的模型名
				if m, ok := modelMap[session.ID]; ok {
					session.Model = m
				}
				sessions = append(sessions, *session)
			}
		}
	}

	// 按创建时间降序排列
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].CreatedAt > sessions[j].CreatedAt
	})

	return sessions, nil
}

// walkJSONLFiles 递归遍历目录下所有 .jsonl 文件
func walkJSONLFiles(root string, fn func(path string)) error {
	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			return nil
		}
		if strings.HasSuffix(info.Name(), ".jsonl") {
			fn(path)
		}
		return nil
	})
}

// findSessionInDir 在目录中递归查找指定 ID 的会话
func findSessionInDir(root, id string) (*SessionDetail, error) {
	var result *SessionDetail
	walkErr := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() || !strings.HasSuffix(info.Name(), ".jsonl") {
			return nil
		}
		// 精确匹配 ID（支持 <uuid>.jsonl 和 rollout-<ts>-<uuid>.jsonl）
		trimmed := strings.TrimSuffix(info.Name(), ".jsonl")
		if trimmed != id && !strings.HasSuffix(trimmed, "-"+id) {
			return nil
		}
		session, messages, err := parseSessionFile(path)
		if err != nil {
			return nil
		}
		result = &SessionDetail{
			Session:  *session,
			Messages: messages,
		}
		return filepath.SkipDir
	})
	return result, walkErr
}
func (a *App) GetCodexSessionContent(id string) (*SessionDetail, error) {
	// 在 sessions 目录递归查找
	if dir, err := codexSessionsDir(); err == nil {
		if found, err := findSessionInDir(dir, id); found != nil || err != nil {
			return found, err
		}
	}

	// 在 archived_sessions 目录查找（扁平结构）
	if dir, err := codexArchivedSessionsDir(); err == nil {
		if found, err := findSessionInDir(dir, id); found != nil || err != nil {
			return found, err
		}
	}

	return nil, fmt.Errorf("未找到会话: %s", id)
}

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

// MigrateCodexProviders 一键迁移所有会话的 model_provider
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

	// 1. 备份 sessions 目录
	backupDir, err := codexBackupDir()
	if err != nil {
		result.Error = err.Error()
		return result, err
	}
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		result.Error = err.Error()
		return result, err
	}
	backupPath := filepath.Join(backupDir,
		fmt.Sprintf("sessions_backup_%s.tar", time.Now().Format("20060102_150405")))

	// 收集需要备份的目录
	var sessionDirs []string
	if dir, err := codexSessionsDir(); err == nil {
		sessionDirs = append(sessionDirs, dir)
	}
	if dir, err := codexArchivedSessionsDir(); err == nil {
		sessionDirs = append(sessionDirs, dir)
	}

	// 创建 tar 备份
	if err := createTarBackup(backupPath, sessionDirs); err != nil {
		result.Error = fmt.Sprintf("备份失败: %v", err)
		return result, err
	}
	result.BackupPath = backupPath

	// 2. 迁移 JSONL 文件
	migrated, err := migrateJSONLFiles(from, to)
	if err != nil {
		result.Error = fmt.Sprintf("JSONL 迁移部分失败: %v", err)
		return result, err
	}
	result.MigratedCount = migrated

	// 3. 迁移 SQLite
	if sqliteMigrated, err := migrateSQLite(from, to); err != nil {
		a.appendLog("warn", "app", fmt.Sprintf("SQLite 迁移失败: %v", err), "")
	} else {
		result.MigratedCount += sqliteMigrated
	}

	return result, nil
}

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

	// 按文件名降序（最新的在前）
	sort.Slice(backups, func(i, j int) bool {
		return backups[i] > backups[j]
	})

	return backups, nil
}

// RestoreCodexSessions 从备份文件恢复会话到原始位置
// 恢复前会先创建当前状态的新备份
func (a *App) RestoreCodexSessions(backupPath string) (*MigrationResult, error) {
	result := &MigrationResult{}

	// 验证备份文件存在
	if _, err := os.Stat(backupPath); err != nil {
		result.Error = fmt.Sprintf("备份文件不存在: %s", backupPath)
		return result, fmt.Errorf("备份文件不存在: %s", backupPath)
	}

	// 1. 先备份当前会话状态
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

	// 2. 读取备份文件并恢复
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

	// 解析备份格式: name|size\ncontent
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

		// name|size
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

		// 接下来 size 字节是文件内容
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
			// 跳到内容之后
			i += strings.Count(content, "\n")
		}

		// 写入文件
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
	// 精确匹配 ID（支持 <uuid>.jsonl 和 rollout-<ts>-<uuid>.jsonl）
	matchFilename := func(name string) bool {
		trimmed := strings.TrimSuffix(name, ".jsonl")
		return trimmed == id || strings.HasSuffix(trimmed, "-"+id)
	}

	// 在 sessions 目录递归查找
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

	// 在 archived_sessions 目录查找
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
		// 从 archived_sessions 移回 sessions
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
		// 重新解析新路径
		session, _, err := parseSessionFile(dest)
		if err != nil {
			return nil, err
		}
		session.IsArchived = false
		return session, nil
	}

	// 移入 archived_sessions
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
	// 重新解析新路径
	session, _, err := parseSessionFile(dest)
	if err != nil {
		return nil, err
	}
	session.IsArchived = true
	return session, nil
}

// ---------- migration internals ----------

func migrateJSONLFiles(from, to string) (int, error) {
	migrated := 0

	// 递归扫描 sessions 目录
	if dir, err := codexSessionsDir(); err == nil {
		_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() || !strings.HasSuffix(info.Name(), ".jsonl") {
				return nil
			}
			if changed, e := migrateJSONLFile(path, from, to); e == nil && changed {
				migrated++
			}
			return nil
		})
	}

	// flat 扫描 archived_sessions 目录
	if dir, err := codexArchivedSessionsDir(); err == nil {
		entries, err := os.ReadDir(dir)
		if err != nil {
			if !os.IsNotExist(err) {
				return migrated, err
			}
		} else {
			for _, entry := range entries {
				if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") {
					continue
				}
				if changed, e := migrateJSONLFile(filepath.Join(dir, entry.Name()), from, to); e == nil && changed {
					migrated++
				}
			}
		}
	}

	return migrated, nil
}

// migrateJSONLFile 修改单个 JSONL 文件中 model_provider
// 实际格式: {"timestamp":"...","type":"session_meta","payload":{"model_provider":"...",...}}
func migrateJSONLFile(path, from, to string) (bool, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}

	lines := bytes.Split(raw, []byte("\n"))
	firstIndex := -1

	for i, line := range lines {
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		if bytes.Contains(line, []byte(`"session_meta"`)) {
			firstIndex = i
			break
		}
	}

	if firstIndex < 0 {
		return false, nil
	}

	var meta codexSessionMeta
	if err := json.Unmarshal(lines[firstIndex], &meta); err != nil {
		return false, err
	}
	if meta.Payload == nil {
		return false, nil
	}
	if meta.Payload.ModelProvider != from {
		return false, nil
	}

	meta.Payload.ModelProvider = to

	newLine, err := json.Marshal(meta)
	if err != nil {
		return false, err
	}

	lines[firstIndex] = newLine
	result := bytes.Join(lines, []byte("\n"))

	if err := os.WriteFile(path, result, 0o600); err != nil {
		return false, err
	}

	return true, nil
}
func migrateSQLite(from, to string) (int, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return 0, err
	}
	codexDir := filepath.Join(home, ".codex")

	entries, err := os.ReadDir(codexDir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}

	migrated := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), "state_") || !strings.HasSuffix(entry.Name(), ".sqlite") {
			continue
		}
		dbPath := filepath.Join(codexDir, entry.Name())

		count, err := migrateSQLiteFile(dbPath, from, to)
		if err != nil {
			continue
		}
		migrated += count
	}

	return migrated, nil
}

func migrateSQLiteFile(dbPath, from, to string) (int, error) {
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return 0, nil
	}
	defer db.Close()

	// 检查是否有 threads 表
	var tableCount int
	err = db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='threads'").Scan(&tableCount)
	if err != nil || tableCount != 1 {
		return 0, nil
	}

	// 检查 model_provider 列是否存在
	var colCount int
	err = db.QueryRow("SELECT COUNT(*) FROM pragma_table_info('threads') WHERE name='model_provider'").Scan(&colCount)
	if err != nil || colCount != 1 {
		return 0, nil
	}

	// 执行迁移
	result, err := db.Exec("UPDATE threads SET model_provider=? WHERE model_provider=?", to, from)
	if err != nil {
		return 0, fmt.Errorf("更新失败: %w", err)
	}
	affected, _ := result.RowsAffected()
	return int(affected), nil
}

// ---------- backup utility ----------

func createTarBackup(tarPath string, dirs []string) error {
	// 检查是否有文件需要备份
	hasFiles := false
	for _, dir := range dirs {
		if entries, err := os.ReadDir(dir); err == nil && len(entries) > 0 {
			hasFiles = true
			break
		}
	}
	if !hasFiles {
		// 没有文件需要备份，创建空文件标记
		return os.WriteFile(tarPath, []byte{}, 0o600)
	}

	// 使用简单的 tar 格式打包
	// 格式: name \x00 size \n content...  (重复)
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

	// 写入校验和
	hash := sha256.Sum256(buf.Bytes())
	if _, err := fmt.Fprintf(&buf, "CHECKSUM|%x\n", hash); err != nil {
		return err
	}

	if err := os.WriteFile(tarPath, buf.Bytes(), 0o600); err != nil {
		return err
	}
	return nil
}
