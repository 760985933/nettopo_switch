package main

import "time"

type ProxyStatus string

const (
	ProxyStopped  ProxyStatus = "stopped"
	ProxyStarting ProxyStatus = "starting"
	ProxyRunning  ProxyStatus = "running"
	ProxyError    ProxyStatus = "error"
)

// SourceID identifies a proxy instance.
type SourceID string

const (
	SourceCodex  SourceID = "codex"
	SourceClaude SourceID = "claude"
)

func AllSources() []SourceID { return []SourceID{SourceCodex, SourceClaude} }

type Profile struct {
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	Provider     string            `json:"provider"`
	BaseURL      string            `json:"baseURL"`
	APIKey       string            `json:"apiKey"`
	DefaultModel string            `json:"defaultModel"`
	Mappings     map[string]string `json:"mappings"`
	APIType      string            `json:"apiType,omitempty"`
}

// InstanceConfig holds the per-source proxy instance configuration.
type InstanceConfig struct {
	ListenHost       string            `json:"listenHost"`
	ListenPort       int               `json:"listenPort"`
	RequestTimeoutMs int               `json:"requestTimeoutMs"`
	MaxRetries       int               `json:"maxRetries"`
	Mappings         map[string]string `json:"mappings"`
	Headers          map[string]string `json:"headers"`
	CurrentProfileID string            `json:"currentProfileId"`
	ProxyProfileIDs  []string          `json:"proxyProfileIds,omitempty"`
}

type AppConfig struct {
	// Global behaviour
	EnableAutoStart     bool `json:"enableAutoStart"`
	MinimizeToTray      bool `json:"minimizeToTray"`
	LogRetentionDays    int  `json:"logRetentionDays"`
	CompactMode         bool `json:"compactMode"`
	PluginUnlockEnabled bool `json:"pluginUnlockEnabled"`

	// ── Flat transport / identity fields (runtime working set, synced from current instance) ──
	ListenHost       string            `json:"listenHost"`
	ListenPort       int               `json:"listenPort"`
	DeepseekBaseURL  string            `json:"deepseekBaseURL"`
	APIKey           string            `json:"apiKey"`
	DefaultModel     string            `json:"defaultModel"`
	RequestTimeoutMs int               `json:"requestTimeoutMs"`
	MaxRetries       int               `json:"maxRetries"`
	Mappings         map[string]string `json:"mappings"`
	Headers          map[string]string `json:"headers"`
	CurrentProfileID string            `json:"currentProfileId,omitempty"`
	ProxyProfileIDs  []string          `json:"proxyProfileIds,omitempty"`

	// ── Multi-instance configs (canonical storage) ──
	Instances map[SourceID]*InstanceConfig `json:"instances,omitempty"`

	// ── Shared profile definitions ──
	Profiles map[string]*Profile `json:"profiles,omitempty"`
}

// EffectiveConfig builds a flat AppConfig for the given source by merging
// the instance config with its selected profile. Used by ProxyRuntime at start time.
func (cfg AppConfig) EffectiveConfig(source SourceID) (AppConfig, bool) {
	ic, ok := cfg.Instances[source]
	if !ok {
		return AppConfig{}, false
	}
	effective := AppConfig{
		ListenHost:       ic.ListenHost,
		ListenPort:       ic.ListenPort,
		RequestTimeoutMs: ic.RequestTimeoutMs,
		MaxRetries:       ic.MaxRetries,
		Mappings:         copyMap(ic.Mappings),
		Headers:          copyMap(ic.Headers),
		CurrentProfileID: ic.CurrentProfileID,
		ProxyProfileIDs:  ic.ProxyProfileIDs,
		Profiles:         cfg.Profiles,
		// copy global fields
		EnableAutoStart:     cfg.EnableAutoStart,
		MinimizeToTray:      cfg.MinimizeToTray,
		LogRetentionDays:    cfg.LogRetentionDays,
		CompactMode:         cfg.CompactMode,
		PluginUnlockEnabled: cfg.PluginUnlockEnabled,
	}
	if profile, ok := cfg.Profiles[ic.CurrentProfileID]; ok {
		effective.DeepseekBaseURL = profile.BaseURL
		effective.APIKey = profile.APIKey
		effective.DefaultModel = profile.DefaultModel
	}
	return effective, true
}

type ProxyStatusPayload struct {
	Source        SourceID    `json:"source"`
	Status        ProxyStatus `json:"status"`
	ListenAddress string      `json:"listenAddress"`
	StartedAt     string      `json:"startedAt"`
	UptimeSeconds int64       `json:"uptimeSeconds"`
	LastError     string      `json:"lastError"`
	RequestCount  int64       `json:"requestCount"`
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

type SandboxWorkspaceConfig struct {
	NetworkAccess  bool   `json:"networkAccess" toml:"network_access"`
	SandboxMode    string `json:"sandboxMode" toml:"sandbox_mode"`
	ApprovalPolicy string `json:"approvalPolicy" toml:"approval_policy"`
}

type UsageBalance struct {
	AvailableBalance string `json:"availableBalance"`
	TotalBalance     string `json:"totalBalance"`
	Currency         string `json:"currency"`
	IsDepleted       bool   `json:"isDepleted"`
	Error            string `json:"error,omitempty"`
}

type UsageRecord struct {
	ID               string    `json:"id"`
	Provider         string    `json:"provider"`
	ProfileName      string    `json:"profileName"`
	Model            string    `json:"model"`
	Endpoint         string    `json:"endpoint"`
	PromptTokens     int64     `json:"promptTokens"`
	CompletionTokens int64     `json:"completionTokens"`
	TotalTokens      int64     `json:"totalTokens"`
	Success          bool      `json:"success"`
	StatusCode       int       `json:"statusCode"`
	DurationMs       int64     `json:"durationMs"`
	CreatedAt        time.Time `json:"createdAt"`
}

type UsageStats struct {
	Provider         string  `json:"provider"`
	RequestCount     int64   `json:"requestCount"`
	SuccessCount     int64   `json:"successCount"`
	FailureCount     int64   `json:"failureCount"`
	TotalTokens      int64   `json:"totalTokens"`
	PromptTokens     int64   `json:"promptTokens"`
	CompletionTokens int64   `json:"completionTokens"`
	AvgDurationMs    float64 `json:"avgDurationMs"`
}

type ModelStats struct {
	Provider         string  `json:"provider"`
	Model            string  `json:"model"`
	RequestCount     int64   `json:"requestCount"`
	SuccessCount     int64   `json:"successCount"`
	FailureCount     int64   `json:"failureCount"`
	TotalTokens      int64   `json:"totalTokens"`
	PromptTokens     int64   `json:"promptTokens"`
	CompletionTokens int64   `json:"completionTokens"`
	AvgDurationMs    float64 `json:"avgDurationMs"`
}

type TimeSeriesPoint struct {
	Date             string `json:"date"`
	TotalTokens      int64  `json:"totalTokens"`
	PromptTokens     int64  `json:"promptTokens"`
	CompletionTokens int64  `json:"completionTokens"`
	RequestCount     int64  `json:"requestCount"`
}

type UsageStatsResponse struct {
	Today      []UsageStats      `json:"today"`
	ThisWeek   []UsageStats      `json:"thisWeek"`
	ThisMonth  []UsageStats      `json:"thisMonth"`
	ThisYear   []UsageStats      `json:"thisYear"`
	Models     []ModelStats      `json:"models"`
	TimeSeries []TimeSeriesPoint `json:"timeSeries"`
}
