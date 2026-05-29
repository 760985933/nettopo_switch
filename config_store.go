package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
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
	if err := json.Unmarshal(content, &cfg); err != nil {
		return defaultConfig(), err
	}
	// Hoist transport fields from old-style profile objects (pre-Phase-1 format)
	// that json.Unmarshal silently dropped from the Profile struct.
	hoistOldProfileTransport(&cfg, content)
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

// hoistOldProfileTransport extracts transport fields (requestTimeoutMs, maxRetries,
// headers) from old-style profile objects in the raw JSON and promotes them to
// AppConfig-level fields. This handles the migration case where a config saved by
// a pre-Phase-1 version had these fields only inside profile objects — after the
// Profile struct removed them, json.Unmarshal would silently drop the values.
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

	// Hoist all transport fields from profile objects — the old config format
	// kept these in profiles as the canonical copy (normalizeConfig synced
	// profile → top-level). After Unmarshal, any profile-level-only fields
	// are lost from the struct, so we restore them from raw JSON here.
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

	return AppConfig{
		ListenHost:       "127.0.0.1",
		ListenPort:       17419,
		DeepseekBaseURL:  p.DefaultBaseURL,
		APIKey:           "",
		DefaultModel:     p.DefaultModel,
		RequestTimeoutMs: 60000,
		MaxRetries:       3,
		EnableAutoStart:  false,
		MinimizeToTray:   true,
		LogRetentionDays: 7,
		CompactMode:         true,
		PluginUnlockEnabled: false,
		Mappings:            copyMap(p.DefaultMappings),
		Headers:             map[string]string{},
		Profiles:            map[string]*Profile{"default": &defaultProfile},
		CurrentProfileID:    "default",
	}
}

func normalizeConfig(cfg AppConfig) AppConfig {
	defaults := defaultConfig()

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

	// 从 provider 默认值补充缺失的 mappings
	for key, value := range defaults.Mappings {
		if _, ok := cfg.Mappings[key]; !ok {
			cfg.Mappings[key] = value
		}
	}

	// --- Multi-profile migration & sync ---

	// Migration: if no profiles exist, create one from old flat fields
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

	// Ensure current profile ID is valid
	if _, ok := cfg.Profiles[cfg.CurrentProfileID]; !ok {
		for id := range cfg.Profiles {
			cfg.CurrentProfileID = id
			break
		}
	}

	// Normalize current profile and sync identity fields back to flat fields for backward compat
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

	// Migration: only seed once when the field has never been set (nil),
	// not when the user intentionally cleared all proxy entries (empty slice).
	if cfg.ProxyProfileIDs == nil && len(cfg.Profiles) > 0 {
		ids := make([]string, 0, len(cfg.Profiles))
		for id := range cfg.Profiles {
			ids = append(ids, id)
		}
		cfg.ProxyProfileIDs = ids
	}

	return cfg
}

func normalizeProfile(p *Profile, defaults AppConfig) {
	// Default provider to "deepseek" for backward compatibility
	if strings.TrimSpace(p.Provider) == "" {
		p.Provider = string(ProviderDeepSeek)
	}

	// Use provider-specific defaults if available
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
	// Fill in missing mappings from provider defaults first, then global defaults
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
