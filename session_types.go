package main

import "encoding/json"

// ---------- exported session types ----------

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

// ---------- internal JSONL parse types ----------

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

// ---------- sync / status types ----------

// ProviderCounts maps provider ID to session count
type ProviderCounts map[string]int

// SyncRolloutInfo holds per-directory rollout file analysis
type SyncRolloutInfo struct {
	Sessions         ProviderCounts `json:"sessions"`
	ArchivedSessions ProviderCounts `json:"archivedSessions"`
}

// SyncStatusResult is the full diagnostic snapshot returned by GetSyncStatus
type SyncStatusResult struct {
	CodexHome                string               `json:"codexHome"`
	CurrentProvider          string               `json:"currentProvider"`
	CurrentProviderImplicit  bool                 `json:"currentProviderImplicit"`
	ConfiguredProviders      []string             `json:"configuredProviders"`
	RolloutCounts            SyncRolloutInfo      `json:"rolloutCounts"`
	LockedRolloutFiles       []string             `json:"lockedRolloutFiles"`
	EncryptedContentCounts   *SyncRolloutInfo     `json:"encryptedContentCounts,omitempty"`
	EncryptedContentWarning  string               `json:"encryptedContentWarning,omitempty"`
	SQLiteCounts             *SyncRolloutInfo     `json:"sqliteCounts,omitempty"`
	SQLiteUnreadable         bool                 `json:"sqliteUnreadable"`
	SQLiteError              string               `json:"sqliteError,omitempty"`
	SQLiteRepairStats        *SyncRepairStats     `json:"sqliteRepairStats,omitempty"`
	ProjectThreadVisibility  []ProjectThreadInfo  `json:"projectThreadVisibility"`
	BackupRoot               string               `json:"backupRoot"`
	BackupCount              int                  `json:"backupCount"`
}

// SyncRepairStats holds counts of rows that need repair in SQLite
type SyncRepairStats struct {
	UserEventRowsNeedingRepair int `json:"userEventRowsNeedingRepair"`
	CwdRowsNeedingRepair       int `json:"cwdRowsNeedingRepair"`
}

// ProjectThreadInfo describes session visibility for one workspace root
type ProjectThreadInfo struct {
	Root              string         `json:"root"`
	InteractiveThreads int           `json:"interactiveThreads"`
	FirstPageThreads  int            `json:"firstPageThreads"`
	ExactCwdMatches   int            `json:"exactCwdMatches"`
	VerbatimCwdRows   int            `json:"verbatimCwdRows"`
	TopRank           int            `json:"topRank"`
	Ranks             []int          `json:"ranks"`
	RankPreview       string         `json:"rankPreview"`
	ProviderCounts    ProviderCounts `json:"providerCounts"`
}

// SyncResult is returned by RunSync after a successful provider sync
type SyncResult struct {
	CodexHome                string         `json:"codexHome"`
	TargetProvider           string         `json:"targetProvider"`
	PreviousProvider         string         `json:"previousProvider"`
	BackupDir                string         `json:"backupDir"`
	BackupDurationMs         int64          `json:"backupDurationMs"`
	ChangedSessionFiles      int            `json:"changedSessionFiles"`
	SkippedLockedFiles       []string       `json:"skippedLockedFiles"`
	SQLiteRowsUpdated        int            `json:"sqliteRowsUpdated"`
	SQLiteProviderRowsUpdated int           `json:"sqliteProviderRowsUpdated"`
	SQLiteUserEventRowsUpdated int          `json:"sqliteUserEventRowsUpdated"`
	SQLiteCwdRowsUpdated     int            `json:"sqliteCwdRowsUpdated"`
	UpdatedWorkspaceRoots    int            `json:"updatedWorkspaceRoots"`
	SavedWorkspaceRootCount  int            `json:"savedWorkspaceRootCount"`
	SQLitePresent            bool           `json:"sqlitePresent"`
	RolloutCountsBefore      SyncRolloutInfo `json:"rolloutCountsBefore"`
	EncryptedContentWarning  string         `json:"encryptedContentWarning,omitempty"`
}

// syncRolloutChange describes a single rollout file that needs its first line rewritten
type syncRolloutChange struct {
	Path              string
	ThreadID          string
	Directory         string
	OriginalFirstLine string
	OriginalSeparator string
	OriginalOffset    int64
	OriginalSize      int64
	OriginalMtimeMs   int64
	OriginalProvider  string
	UpdatedFirstLine  string
}

// threadCwdInfo holds the cwd value from a rollout file for a given thread
type threadCwdInfo struct {
	Cwd              string
	NormalizedCwd    string
	Count            int
	UpdatedAtMs      int64
}
