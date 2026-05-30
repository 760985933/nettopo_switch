package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

type ConfigStore struct {
	mu   sync.Mutex
	path string
}

func NewConfigStore() (*ConfigStore, error) {
	baseDir, err := os.UserConfigDir()
	if err != nil {
		return nil, err
	}

	dir := filepath.Join(baseDir, "codex-deepseek-bridge")
	return &ConfigStore{
		path: filepath.Join(dir, "app-config.json"),
	}, nil
}

func (s *ConfigStore) Path() string {
	return s.path
}

func (s *ConfigStore) Load() (AppConfig, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	cfg := defaultConfig()
	content, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return cfg, err
	}

	// Decode into a generic map first so we can detect old vs new format.
	raw := make(map[string]any)
	if err := json.Unmarshal(content, &raw); err != nil {
		return defaultConfig(), err
	}

	_, hasInstances := raw["instances"]
	if !hasInstances {
		// Old format: unmarshal into cfg (flat fields populated), then migrate.
		if err := json.Unmarshal(content, &cfg); err != nil {
			return defaultConfig(), err
		}
		hoistOldProfileTransport(&cfg, content)
		cfg = migrateToMultiInstance(cfg)
	} else {
		if err := json.Unmarshal(content, &cfg); err != nil {
			return defaultConfig(), err
		}
	}

	return normalizeConfig(cfg), nil
}

func (s *ConfigStore) Save(cfg AppConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	cfg = normalizeConfig(cfg)
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}

	content, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(s.path, content, 0o600)
}

// migrateToMultiInstance creates Instances from flat fields (old-format config).
func migrateToMultiInstance(cfg AppConfig) AppConfig {
	if cfg.Instances == nil {
		cfg.Instances = make(map[SourceID]*InstanceConfig)
	}
	if _, ok := cfg.Instances[SourceCodex]; !ok {
		cfg.Instances[SourceCodex] = &InstanceConfig{
			ListenHost:       cfg.ListenHost,
			ListenPort:       cfg.ListenPort,
			RequestTimeoutMs: cfg.RequestTimeoutMs,
			MaxRetries:       cfg.MaxRetries,
			Mappings:         copyMap(cfg.Mappings),
			Headers:          copyMap(cfg.Headers),
			CurrentProfileID: cfg.CurrentProfileID,
			ProxyProfileIDs:  cfg.ProxyProfileIDs,
		}
	}
	if _, ok := cfg.Instances[SourceClaude]; !ok {
		cfg.Instances[SourceClaude] = defaultInstanceConfig(SourceClaude)
	}
	return cfg
}

// hoistOldProfileTransport extracts transport fields (requestTimeoutMs, maxRetries,
// headers) from old-style profile objects in the raw JSON and promotes them to
// AppConfig-level fields. This handles the migration case where a config saved by
// a pre-Phase-1 version had these fields only inside profile objects.
func hoistOldProfileTransport(cfg *AppConfig, raw []byte) {
	if cfg == nil {
		return
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return
	}
	profilesRaw, ok := doc["profiles"]
	if !ok {
		return
	}
	profilesMap, ok := profilesRaw.(map[string]any)
	if !ok || len(profilesMap) == 0 {
		return
	}

	for _, v := range profilesMap {
		p, ok := v.(map[string]any)
		if !ok {
			continue
		}
		if val, ok := p["requestTimeoutMs"].(float64); ok && val > 0 {
			cfg.RequestTimeoutMs = int(val)
		}
		if val, ok := p["maxRetries"].(float64); ok && val >= 0 {
			cfg.MaxRetries = int(val)
		}
		if val, ok := p["headers"].(map[string]any); ok && len(val) > 0 {
			headers := make(map[string]string, len(val))
			for k, hv := range val {
				if s, ok := hv.(string); ok {
					headers[k] = s
				}
			}
			if len(headers) > 0 {
				cfg.Headers = headers
			}
		}
		break // all profiles were synced; one is enough
	}
}

func defaultConfig() AppConfig {
	p := GetDefaultProvider()
	defaultProfile := Profile{
		ID:           "default",
		Name:         p.Name,
		Provider:     string(p.ID),
		BaseURL:      p.DefaultBaseURL,
		APIKey:       "",
		DefaultModel: p.DefaultModel,
		Mappings:     copyMap(p.DefaultMappings),
	}

	codexInst := defaultInstanceConfig(SourceCodex)
	claudeInst := defaultInstanceConfig(SourceClaude)

	return AppConfig{
		// Global
		EnableAutoStart:     false,
		MinimizeToTray:      true,
		LogRetentionDays:    7,
		CompactMode:         true,
		PluginUnlockEnabled: false,

		// Flat fields synced from codex instance (backward compat)
		ListenHost:       codexInst.ListenHost,
		ListenPort:       codexInst.ListenPort,
		DeepseekBaseURL:  p.DefaultBaseURL,
		APIKey:           "",
		DefaultModel:     p.DefaultModel,
		RequestTimeoutMs: codexInst.RequestTimeoutMs,
		MaxRetries:       codexInst.MaxRetries,
		Mappings:         copyMap(codexInst.Mappings),
		Headers:          copyMap(codexInst.Headers),
		CurrentProfileID: "default",
		ProxyProfileIDs:  []string{"default"},

		// Canonical instances
		Instances: map[SourceID]*InstanceConfig{
			SourceCodex:  codexInst,
			SourceClaude: claudeInst,
		},

		Profiles: map[string]*Profile{"default": &defaultProfile},
	}
}

func defaultInstanceConfig(source SourceID) *InstanceConfig {
	port := 17419
	if source == SourceClaude {
		port = 17420
	}
	return &InstanceConfig{
		ListenHost:       "127.0.0.1",
		ListenPort:       port,
		RequestTimeoutMs: 60000,
		MaxRetries:       3,
		Mappings:         map[string]string{},
		Headers:          map[string]string{},
		CurrentProfileID: "default",
	}
}

func normalizeConfig(cfg AppConfig) AppConfig {
	defaults := defaultConfig()

	// Ensure Instances map exists
	if cfg.Instances == nil {
		cfg.Instances = make(map[SourceID]*InstanceConfig)
	}
	for _, src := range AllSources() {
		if cfg.Instances[src] == nil {
			cfg.Instances[src] = defaultInstanceConfig(src)
		}
		normalizeInstance(cfg.Instances[src], *defaultInstanceConfig(src))
		// Ensure CurrentProfileID points to a valid profile
		if _, ok := cfg.Profiles[cfg.Instances[src].CurrentProfileID]; !ok {
			for id := range cfg.Profiles {
				cfg.Instances[src].CurrentProfileID = id
				break
			}
		}
	}

	// Sync flat fields from codex instance for backward compat
	if codexInst, ok := cfg.Instances[SourceCodex]; ok {
		cfg.ListenHost = codexInst.ListenHost
		cfg.ListenPort = codexInst.ListenPort
		cfg.RequestTimeoutMs = codexInst.RequestTimeoutMs
		cfg.MaxRetries = codexInst.MaxRetries
		cfg.Mappings = codexInst.Mappings
		cfg.Headers = codexInst.Headers
		cfg.CurrentProfileID = codexInst.CurrentProfileID
		cfg.ProxyProfileIDs = codexInst.ProxyProfileIDs
	}

	// Legacy flat-field fallbacks
	if strings.TrimSpace(cfg.ListenHost) == "" {
		cfg.ListenHost = defaults.ListenHost
	}
	if cfg.ListenPort <= 0 {
		cfg.ListenPort = defaults.ListenPort
	}
	cfg.DeepseekBaseURL = strings.TrimRight(strings.TrimSpace(cfg.DeepseekBaseURL), "/")
	if cfg.DeepseekBaseURL == "" {
		cfg.DeepseekBaseURL = defaults.DeepseekBaseURL
	}
	if strings.TrimSpace(cfg.DefaultModel) == "" {
		cfg.DefaultModel = defaults.DefaultModel
	}
	if cfg.RequestTimeoutMs <= 0 {
		cfg.RequestTimeoutMs = defaults.RequestTimeoutMs
	}
	if cfg.MaxRetries < 0 {
		cfg.MaxRetries = defaults.MaxRetries
	}
	if cfg.LogRetentionDays <= 0 {
		cfg.LogRetentionDays = defaults.LogRetentionDays
	}
	if cfg.Mappings == nil {
		cfg.Mappings = map[string]string{}
	}
	if cfg.Headers == nil {
		cfg.Headers = map[string]string{}
	}

	// Fill missing mappings from provider defaults
	for key, value := range defaults.Mappings {
		if _, ok := cfg.Mappings[key]; !ok {
			cfg.Mappings[key] = value
		}
	}

	// --- Multi-profile migration & sync ---

	if len(cfg.Profiles) == 0 {
		profile := &Profile{
			ID:           "default",
			Name:         "DeepSeek",
			BaseURL:      cfg.DeepseekBaseURL,
			APIKey:       cfg.APIKey,
			DefaultModel: cfg.DefaultModel,
			Mappings:     copyMap(cfg.Mappings),
		}
		cfg.Profiles = map[string]*Profile{"default": profile}
		cfg.CurrentProfileID = "default"
	}

	// Ensure current profile ID is valid (flat field)
	if _, ok := cfg.Profiles[cfg.CurrentProfileID]; !ok {
		for id := range cfg.Profiles {
			cfg.CurrentProfileID = id
			break
		}
	}

	// Normalize current profile and sync identity fields
	if profile, ok := cfg.Profiles[cfg.CurrentProfileID]; ok {
		normalizeProfile(profile, defaults)
		cfg.DeepseekBaseURL = profile.BaseURL
		cfg.APIKey = profile.APIKey
		cfg.DefaultModel = profile.DefaultModel
		cfg.Mappings = profile.Mappings
	}

	// Normalize non-current profiles too
	for id, p := range cfg.Profiles {
		if id != cfg.CurrentProfileID {
			normalizeProfile(p, defaults)
		}
	}

	// Migration: seed ProxyProfileIDs when nil
	if cfg.ProxyProfileIDs == nil && len(cfg.Profiles) > 0 {
		ids := make([]string, 0, len(cfg.Profiles))
		for id := range cfg.Profiles {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		cfg.ProxyProfileIDs = ids
	}
	// Also sync instance-level proxyProfileIds
	for _, src := range AllSources() {
		inst := cfg.Instances[src]
		if inst.ProxyProfileIDs == nil && len(cfg.Profiles) > 0 {
			ids := make([]string, 0, len(cfg.Profiles))
			for id := range cfg.Profiles {
				ids = append(ids, id)
			}
			sort.Strings(ids)
			inst.ProxyProfileIDs = ids
		}
	}

	return cfg
}

func normalizeInstance(ic *InstanceConfig, defaults InstanceConfig) {
	if strings.TrimSpace(ic.ListenHost) == "" {
		ic.ListenHost = defaults.ListenHost
	}
	if ic.ListenPort <= 0 {
		ic.ListenPort = defaults.ListenPort
	}
	if ic.RequestTimeoutMs <= 0 {
		ic.RequestTimeoutMs = defaults.RequestTimeoutMs
	}
	if ic.MaxRetries < 0 {
		ic.MaxRetries = defaults.MaxRetries
	}
	if ic.Mappings == nil {
		ic.Mappings = map[string]string{}
	}
	if ic.Headers == nil {
		ic.Headers = map[string]string{}
	}
	if strings.TrimSpace(ic.CurrentProfileID) == "" {
		ic.CurrentProfileID = defaults.CurrentProfileID
	}
}

func normalizeProfile(p *Profile, defaults AppConfig) {
	if strings.TrimSpace(p.Provider) == "" {
		p.Provider = string(ProviderDeepSeek)
	}

	prov := GetProvider(ProviderID(p.Provider))

	p.BaseURL = strings.TrimRight(strings.TrimSpace(p.BaseURL), "/")
	if p.BaseURL == "" {
		if prov != nil {
			p.BaseURL = prov.DefaultBaseURL
		} else {
			p.BaseURL = defaults.DeepseekBaseURL
		}
	}
	if strings.TrimSpace(p.DefaultModel) == "" {
		if prov != nil {
			p.DefaultModel = prov.DefaultModel
		} else {
			p.DefaultModel = defaults.DefaultModel
		}
	}
	if p.Mappings == nil {
		p.Mappings = map[string]string{}
	}
	provMappings := defaults.Mappings
	if prov != nil && prov.DefaultMappings != nil {
		provMappings = prov.DefaultMappings
	}
	for key, value := range provMappings {
		if _, ok := p.Mappings[key]; !ok {
			p.Mappings[key] = value
		}
	}
}

func copyMap[K comparable, V any](src map[K]V) map[K]V {
	if src == nil {
		return nil
	}
	dst := make(map[K]V, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}
