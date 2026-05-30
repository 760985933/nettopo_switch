package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"

	toml "github.com/pelletier/go-toml/v2"
)

const (
	codexProfileName     = "local"
	codexProfileProvider = "local"
)

// mergeCodexConfigTomlProfiles merges a [profiles.local] section into the
// existing Codex config.toml. It preserves all other top-level sections
// (sandbox_workspace_write, MCP, approvals, etc.) and only updates the
// profiles.local block. Conflicting top-level keys (model_provider,
// openai_base_url) are removed.
func mergeCodexConfigTomlProfiles(existing []byte, baseURL string, defaultModel string) ([]byte, error) {
	doc := map[string]any{}

	if len(bytes.TrimSpace(existing)) > 0 {
		if err := toml.Unmarshal(existing, &doc); err != nil {
			return nil, err
		}
	}

	// Clean top-level keys that conflict with profiles
	delete(doc, "model_provider")
	delete(doc, "openai_base_url")
	doc["profile"] = codexProfileName // activate [profiles.local] section

	if strings.TrimSpace(defaultModel) == "" {
		defaultModel = "deepseek-v4-flash"
	}

	// Build or update profiles map
	profiles := map[string]any{}
	if existingProfiles, ok := doc["profiles"].(map[string]any); ok {
		for k, v := range existingProfiles {
			profiles[k] = v
		}
	}

	profiles[codexProfileName] = map[string]any{
		"model":          defaultModel,
		"model_provider": codexProfileProvider,
		"openai_base_url": strings.TrimRight(baseURL, "/") + "/",
	}
	doc["profiles"] = profiles

	// Build or update model_providers map — add a "local" entry so
	// Codex knows how to reach the provider referenced by [profiles.local].
	modelProviders := map[string]any{}
	if existingMP, ok := doc["model_providers"].(map[string]any); ok {
		for k, v := range existingMP {
			if k != "Local" { // clean legacy capital-L key
				modelProviders[k] = v
			}
		}
	}
	modelProviders[codexProfileName] = map[string]any{
		"name":     codexProfileName,
		"base_url": strings.TrimRight(baseURL, "/") + "/",
		"wire_api": "responses",
	}
	doc["model_providers"] = modelProviders

	return toml.Marshal(doc)
}

// GenerateCodexConfigTomlProfiles returns the TOML content that would be
// written by WriteCodexConfigTomlProfiles (for preview / clipboard).
func (a *App) GenerateCodexConfigTomlProfiles() (string, error) {
	status := a.proxies[SourceCodex].Status()
	if strings.TrimSpace(status.ListenAddress) == "" {
		return "", errors.New("代理服务未启动，无法生成 base_url")
	}

	cfg, err := a.GetAppConfig()
	if err != nil {
		return "", err
	}

	baseURL := strings.TrimRight(status.ListenAddress, "/") + "/v1"
	merged, err := mergeCodexConfigTomlProfiles(nil, baseURL, cfg.DefaultModel)
	if err != nil {
		return "", err
	}
	return string(merged), nil
}

// WriteCodexConfigTomlProfiles writes the Codex config.toml using the
// [profiles.local] format (model_provider = 'local'). It merges into the
// existing file so that unrelated sections like sandbox_workspace_write
// and MCP configs are preserved.
func (a *App) WriteCodexConfigTomlProfiles() (string, error) {
	status := a.proxies[SourceCodex].Status()
	if strings.TrimSpace(status.ListenAddress) == "" {
		return "", errors.New("代理服务未启动，无法生成 base_url")
	}

	cfg, err := a.GetAppConfig()
	if err != nil {
		return "", err
	}

	baseURL := strings.TrimRight(status.ListenAddress, "/") + "/v1"

	path, err := codexConfigPath()
	if err != nil {
		return "", err
	}

	if mkErr := os.MkdirAll(filepath.Dir(path), 0o755); mkErr != nil {
		return "", mkErr
	}

	existing, readErr := os.ReadFile(path)
	if readErr == nil && len(existing) > 0 {
		if backupPath, backupErr := makeCodexBackup(path, existing); backupErr != nil {
			return "", backupErr
		} else if strings.TrimSpace(backupPath) != "" {
			a.appendLog("info", "app", "已备份原 Codex config.toml: "+backupPath, "")
		}
	}

	merged, err := mergeCodexConfigTomlProfiles(existing, baseURL, cfg.DefaultModel)
	if err != nil {
		return "", err
	}

	if err := os.WriteFile(path, merged, 0o600); err != nil {
		return "", err
	}

	a.appendLog("info", "app", "已更新 Codex config.toml（profiles.local 模式）: "+path, "")
	return path, nil
}
