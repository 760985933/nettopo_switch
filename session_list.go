package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	_ "github.com/mattn/go-sqlite3"
)

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
			if strings.Contains(text, "<environment_context>") {
				continue
			}
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
func extractPartsText(raw json.RawMessage) string {
	var parts []map[string]any
	if err := json.Unmarshal(raw, &parts); err != nil {
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

// ---------- model map from SQLite ----------

// loadModelMap 从 SQLite state_*.sqlite 中查询所有线程的 model 名称
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

// ---------- Wails bindings ----------

// ListCodexSessions 扫描 sessions 和 archived_sessions 目录，返回所有会话列表
func (a *App) ListCodexSessions() ([]CodexSession, error) {
	sessions := make([]CodexSession, 0)

	modelMap := loadModelMap()

	if dir, err := codexSessionsDir(); err == nil {
		_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() || !strings.HasSuffix(info.Name(), ".jsonl") {
				return nil
			}
			session, _, err := parseSessionFile(path)
			if err != nil {
				return nil
			}
			if m, ok := modelMap[session.ID]; ok {
				session.Model = m
			}
			sessions = append(sessions, *session)
			return nil
		})
	}

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
				if m, ok := modelMap[session.ID]; ok {
					session.Model = m
				}
				sessions = append(sessions, *session)
			}
		}
	}

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
	if dir, err := codexSessionsDir(); err == nil {
		if found, err := findSessionInDir(dir, id); found != nil || err != nil {
			return found, err
		}
	}

	if dir, err := codexArchivedSessionsDir(); err == nil {
		if found, err := findSessionInDir(dir, id); found != nil || err != nil {
			return found, err
		}
	}

	return nil, fmt.Errorf("未找到会话: %s", id)
}
