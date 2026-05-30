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
	codexProviderID = "openai"
)

func codexConfigPath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(homeDir, ".codex", "config.toml"), nil
}

func (a *App) GetCodexConfigPath() (string, error) {
	return codexConfigPath()
}

func codexBackupDir() (string, error) {
	path, err := codexConfigPath()
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(path), "backups"), nil
}

func (a *App) ReadCodexConfigToml() (string, error) {
	path, err := codexConfigPath()
	if err != nil {
		return "", err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}
		return "", err
	}
	return string(raw), nil
}

func (a *App) GenerateCodexConfigToml() (string, error) {
	status := a.proxies[SourceCodex].Status()
	if strings.TrimSpace(status.ListenAddress) == "" {
		return "", errors.New("代理服务未启动，无法生成 base_url")
	}

	cfg, err := a.GetAppConfig()
	if err != nil {
		return "", err
	}

	baseURL := strings.TrimRight(status.ListenAddress, "/") + "/v1"
	merged, err := mergeCodexConfigToml(nil, baseURL, cfg.DefaultModel)
	if err != nil {
		return "", err
	}
	return string(merged), nil
}

func (a *App) WriteCodexConfigTomlRaw(content string) (string, error) {
	path, err := codexConfigPath()
	if err != nil {
		return "", err
	}

	doc := map[string]any{}
	if len(bytes.TrimSpace([]byte(content))) > 0 {
		if err := toml.Unmarshal([]byte(content), &doc); err != nil {
			return "", err
		}
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

	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		return "", err
	}

	a.appendLog("info", "app", "已写入 Codex config.toml: "+path, "")
	return path, nil
}

func (a *App) WriteCodexConfigToml() (string, error) {
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

	merged, err := mergeCodexConfigToml(existing, baseURL, cfg.DefaultModel)
	if err != nil {
		return "", err
	}

	if err := os.WriteFile(path, merged, 0o600); err != nil {
		return "", err
	}

	a.appendLog("info", "app", "已更新 Codex config.toml（保留原配置项）: "+path, "")
	return path, nil
}

func mergeCodexConfigToml(existing []byte, baseURL string, defaultModel string) ([]byte, error) {
	doc := map[string]any{}

	if len(bytes.TrimSpace(existing)) > 0 {
		if err := toml.Unmarshal(existing, &doc); err != nil {
			return nil, err
		}
	}

	doc["model_provider"] = codexProviderID
	delete(doc, "profile")
	if strings.TrimSpace(defaultModel) != "" {
		doc["model"] = defaultModel
	} else if _, ok := doc["model"]; !ok {
		doc["model"] = "deepseek-v4-flash"
	}

	doc["openai_base_url"] = strings.TrimRight(baseURL, "/") + "/"

	// Clean legacy custom provider configs
	if modelProviders, ok := doc["model_providers"].(map[string]any); ok {
		delete(modelProviders, "Local")
		if len(modelProviders) == 0 {
			delete(doc, "model_providers")
		} else {
			doc["model_providers"] = modelProviders
		}
	}
	if profiles, ok := doc["profiles"].(map[string]any); ok {
		delete(profiles, "Local")
		if len(profiles) == 0 {
			delete(doc, "profiles")
		} else {
			doc["profiles"] = profiles
		}
	}

	return toml.Marshal(doc)
}

func removeCodexBridgeFromConfig(existing []byte) ([]byte, bool, error) {
	doc := map[string]any{}
	if len(bytes.TrimSpace(existing)) == 0 {
		return existing, false, nil
	}
	if err := toml.Unmarshal(existing, &doc); err != nil {
		return nil, false, err
	}

	changed := false

	if _, has := doc["openai_base_url"]; has {
		delete(doc, "openai_base_url")
		changed = true
	}

	modelProvidersAny, hasMp := doc["model_providers"]
	if hasMp {
		if modelProviders, ok := modelProvidersAny.(map[string]any); ok {
			if _, has := modelProviders["Local"]; has {
				delete(modelProviders, "Local")
				changed = true
			}
			if len(modelProviders) == 0 {
				delete(doc, "model_providers")
			} else {
				doc["model_providers"] = modelProviders
			}
		}
	}

	profilesAny, hasProf := doc["profiles"]
	if hasProf {
		if profiles, ok := profilesAny.(map[string]any); ok {
			if _, has := profiles["Local"]; has {
				delete(profiles, "Local")
				changed = true
			}
			if len(profiles) == 0 {
				delete(doc, "profiles")
			} else {
				doc["profiles"] = profiles
			}
		}
	}

	if !changed {
		return existing, false, nil
	}
	out, err := toml.Marshal(doc)
	if err != nil {
		return nil, false, err
	}
	return out, true, nil
}
