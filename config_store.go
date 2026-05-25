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

func defaultConfig() AppConfig {
	return AppConfig{
		ListenHost:       "127.0.0.1",
		ListenPort:       17419,
		DeepseekBaseURL:  "https://api.deepseek.com/v1",
		APIKey:           "",
		DefaultModel:     "deepseek-chat",
		RequestTimeoutMs: 60000,
		MaxRetries:       1,
		EnableAutoStart:  false,
		MinimizeToTray:   false,
		LogRetentionDays: 7,
		CompactMode:      true,
		Mappings: map[string]string{
			"gpt-5.5":      "deepseek-v4-pro",
			"gpt-5.4":      "deepseek-v4-pro",
			"gpt-5.4-mini": "deepseek-v4-flash",
			"gpt-5.3-codex": "deepseek-v4-pro",
			"gpt-4.1":     "deepseek-chat",
			"gpt-4o":      "deepseek-chat",
			"gpt-4o-mini": "deepseek-chat",
			"o4-mini":     "deepseek-reasoner",
		},
		Headers: map[string]string{},
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

	for key, value := range defaults.Mappings {
		if _, ok := cfg.Mappings[key]; !ok {
			cfg.Mappings[key] = value
		}
	}

	return cfg
}
