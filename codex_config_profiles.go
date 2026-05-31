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

func localConfigPath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(homeDir, ".codex", "local.config.toml"), nil
}

// mergeCodexConfigTomlProfiles updates the main config.toml by adding
// the proxy model settings as top-level keys (model, model_provider,
// openai_base_url) so Codex desktop uses them directly without needing
// a profile override. It also removes the legacy profile = 'local' key
// and old [profiles] section that Codex no longer supports.
func mergeCodexConfigTomlProfiles(existing []byte, baseURL string, defaultModel string) ([]byte, error) {
	doc := map[string]any{}

	if len(bytes.TrimSpace(existing)) > 0 {
		if err := toml.Unmarshal(existing, &doc); err != nil {
			return nil, err
		}
	}

	// Remove legacy profile key and section — Codex no longer supports
	// profile = 'local' in the main config.
	delete(doc, "profile")
	delete(doc, "profiles")

	// Write proxy settings as top-level keys so they take effect directly.
	if strings.TrimSpace(defaultModel) == "" {
		defaultModel = "deepseek-v4-flash"
	}
	doc["model"] = defaultModel
	doc["model_provider"] = codexProfileProvider
	doc["openai_base_url"] = strings.TrimRight(baseURL, "/") + "/"

	// Build or update model_providers map — add a "local" entry so
	// Codex knows how to reach the provider.
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

// mergeLocalConfigToml generates the content for local.config.toml,
// which contains the profile-specific overrides (model, model_provider,
// openai_base_url) that Codex loads when --profile local is active.
func mergeLocalConfigToml(baseURL string, defaultModel string) ([]byte, error) {
	doc := map[string]any{}

	if strings.TrimSpace(defaultModel) == "" {
		defaultModel = "deepseek-v4-flash"
	}

	doc["model"] = defaultModel
	doc["model_provider"] = codexProfileProvider
	doc["openai_base_url"] = strings.TrimRight(baseURL, "/") + "/"

	return toml.Marshal(doc)
}

// GenerateCodexConfigTomlProfiles returns the TOML content for the main
// config.toml (without legacy profile = 'local'), for preview / clipboard.
func (a *App) GenerateCodexConfigTomlProfiles() (string, error) {
	proxy, ok := a.proxies[SourceCodex]
	if !ok {
		return "", errors.New("codex代理未初始化")
	}
	status := proxy.Status()
	if strings.TrimSpace(status.ListenAddress) == "" {
		return "", errors.New("代理服务未启动，无法生成 base_url")
	}

	cfg, err := a.GetAppConfig()
	if err != nil {
		return "", err
	}

	baseURL := strings.TrimRight(status.ListenAddress, "/") + "/v1"
	// Generate main config without profile = 'local'
	merged, err := mergeCodexConfigTomlProfiles(nil, baseURL, cfg.DefaultModel)
	if err != nil {
		return "", err
	}

	// Append local.config.toml content as a comment for reference
	localContent, err := mergeLocalConfigToml(baseURL, cfg.DefaultModel)
	if err != nil {
		return "", err
	}

	return string(merged) + "\n# === local.config.toml ===\n" + string(localContent), nil
}

// WriteCodexConfigTomlProfiles writes the Codex config.toml with model_providers
// and a separate local.config.toml with the profile-specific overrides. Codex
// loads local.config.toml when --profile local is specified (no legacy
// profile = 'local' key is written to the main config).
func (a *App) WriteCodexConfigTomlProfiles() (string, error) {
	proxy, ok := a.proxies[SourceCodex]
	if !ok {
		return "", errors.New("codex代理未初始化")
	}
	status := proxy.Status()
	if strings.TrimSpace(status.ListenAddress) == "" {
		return "", errors.New("代理服务未启动，无法生成 base_url")
	}

	cfg, err := a.GetAppConfig()
	if err != nil {
		return "", err
	}

	baseURL := strings.TrimRight(status.ListenAddress, "/") + "/v1"

	// --- Write main config.toml (without profile = 'local') ---
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

	// --- Write local.config.toml (profile-specific overrides) ---
	localPath, err := localConfigPath()
	if err != nil {
		return "", err
	}

	localContent, err := mergeLocalConfigToml(baseURL, cfg.DefaultModel)
	if err != nil {
		return "", err
	}

	if err := os.WriteFile(localPath, localContent, 0o600); err != nil {
		return "", err
	}

	a.appendLog("info", "app", "已更新 Codex config.toml + local.config.toml（profiles.local 模式）: "+path, "")
	return path, nil
}
