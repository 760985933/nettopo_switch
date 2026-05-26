package main

type ProxyStatus string

const (
	ProxyStopped  ProxyStatus = "stopped"
	ProxyStarting ProxyStatus = "starting"
	ProxyRunning  ProxyStatus = "running"
	ProxyError    ProxyStatus = "error"
)

type AppConfig struct {
	ListenHost       string            `json:"listenHost"`
	ListenPort       int               `json:"listenPort"`
	DeepseekBaseURL  string            `json:"deepseekBaseURL"`
	APIKey           string            `json:"apiKey"`
	DefaultModel     string            `json:"defaultModel"`
	RequestTimeoutMs int               `json:"requestTimeoutMs"`
	MaxRetries       int               `json:"maxRetries"`
	EnableAutoStart  bool              `json:"enableAutoStart"`
	MinimizeToTray   bool              `json:"minimizeToTray"`
	LogRetentionDays int               `json:"logRetentionDays"`
	CompactMode      bool              `json:"compactMode"`
	Mappings         map[string]string `json:"mappings"`
	Headers          map[string]string `json:"headers"`
}

type ProxyStatusPayload struct {
	Status        ProxyStatus `json:"status"`
	ListenAddress string       `json:"listenAddress"`
	StartedAt     string       `json:"startedAt"`
	UptimeSeconds int64        `json:"uptimeSeconds"`
	LastError     string       `json:"lastError"`
	RequestCount  int64        `json:"requestCount"`
}

type OverviewSnapshot struct {
	Config     AppConfig           `json:"config"`
	Status     ProxyStatusPayload `json:"status"`
	RecentLogs []LogEntry          `json:"recentLogs"`
	QuickTips  []string            `json:"quickTips"`
	Defaults   map[string]string   `json:"defaults"`
	Features   map[string]bool     `json:"features"`
}

type LogEntry struct {
	ID        string `json:"id"`
	Level     string `json:"level"`
	Timestamp string `json:"timestamp"`
	Source    string `json:"source"`
	Message   string `json:"message"`
	RequestID string `json:"requestId,omitempty"`
}

type HealthCheckResult struct {
	OK     bool              `json:"ok"`
	Checks []HealthCheckItem `json:"checks"`
}

type HealthCheckItem struct {
	Name    string `json:"name"`
	OK      bool   `json:"ok"`
	Message string `json:"message"`
}

type UpdateCheckResult struct {
	CurrentVersion string `json:"currentVersion"`
	LatestVersion  string `json:"latestVersion"`
	HasUpdate      bool   `json:"hasUpdate"`
	DownloadURL    string `json:"downloadUrl"`
	Notes          string `json:"notes"`
	CheckedAt      string `json:"checkedAt"`
}
