package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	toml "github.com/pelletier/go-toml/v2"
)

const (
	codexProviderID = "local-bridge"
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

func makeCodexBackup(path string, existing []byte) (string, error) {
	if len(existing) == 0 {
		return "", nil
	}
	dir, err := codexBackupDir()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}

	ts := time.Now().Format("20060102_150405.000")
	ts = strings.ReplaceAll(ts, ".", "_")
	base := filepath.Base(path)

	for i := 0; i < 1000; i++ {
		name := base + "." + ts
		if i > 0 {
			name = name + "_" + strconv.Itoa(i)
		}
		name = name + ".bak"

		backupPath := filepath.Join(dir, name)
		if _, statErr := os.Stat(backupPath); statErr == nil {
			continue
		} else if !errors.Is(statErr, os.ErrNotExist) {
			return "", statErr
		}

		if err := os.WriteFile(backupPath, existing, 0o600); err != nil {
			return "", err
		}
		return backupPath, nil
	}

	return "", errors.New("无法生成备份文件名")
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
	status := a.proxy.Status()
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
	status := a.proxy.Status()
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

func (a *App) ListCodexConfigBackups() ([]string, error) {
	dir, err := codexBackupDir()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []string{}, nil
		}
		return nil, err
	}

	configPath, err := codexConfigPath()
	if err != nil {
		return nil, err
	}
	base := filepath.Base(configPath)

	paths := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasPrefix(name, base+".") || !strings.HasSuffix(name, ".bak") {
			continue
		}
		paths = append(paths, filepath.Join(dir, name))
	}

	legacyBak := configPath + ".bak"
	if legacy, err := os.ReadFile(legacyBak); err == nil && len(legacy) > 0 {
		paths = append(paths, legacyBak)
	}

	sort.Slice(paths, func(i, j int) bool {
		return paths[i] > paths[j]
	})
	return paths, nil
}

func codexIsAllowedBackupPath(backupPath string) (string, error) {
	backupPath = strings.TrimSpace(backupPath)
	if backupPath == "" {
		return "", errors.New("备份路径不能为空")
	}

	dir, err := codexBackupDir()
	if err != nil {
		return "", err
	}
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	absBackup, err := filepath.Abs(backupPath)
	if err != nil {
		return "", err
	}

	path, err := codexConfigPath()
	if err != nil {
		return "", err
	}
	legacyBak := path + ".bak"
	absLegacy, _ := filepath.Abs(legacyBak)

	if strings.HasPrefix(absBackup, absDir+string(filepath.Separator)) || absBackup == absLegacy {
		return absBackup, nil
	}
	return "", errors.New("备份路径不合法")
}

func (a *App) RestoreCodexConfigTomlFromBackup(backupPath string) (string, error) {
	path, err := codexConfigPath()
	if err != nil {
		return "", err
	}
	absBackup, err := codexIsAllowedBackupPath(backupPath)
	if err != nil {
		return "", err
	}

	backup, err := os.ReadFile(absBackup)
	if err != nil {
		return "", err
	}

	if mkErr := os.MkdirAll(filepath.Dir(path), 0o755); mkErr != nil {
		return "", mkErr
	}

	existing, readErr := os.ReadFile(path)
	if readErr == nil && len(existing) > 0 {
		if preserved, err := makeCodexBackup(path, existing); err != nil {
			return "", err
		} else if strings.TrimSpace(preserved) != "" {
			a.appendLog("info", "app", "已备份当前 Codex config.toml: "+preserved, "")
		}
	}

	if err := os.WriteFile(path, backup, 0o600); err != nil {
		return "", err
	}
	a.appendLog("info", "app", "已从备份恢复 Codex config.toml: "+absBackup, "")
	return path, nil
}

func (a *App) DeleteCodexConfigBackup(backupPath string) (string, error) {
	absBackup, err := codexIsAllowedBackupPath(backupPath)
	if err != nil {
		return "", err
	}
	if err := os.Remove(absBackup); err != nil {
		return "", err
	}
	a.appendLog("info", "app", "已删除 Codex config 备份: "+absBackup, "")
	return absBackup, nil
}

func (a *App) ClearCodexConfigBackups() (int, error) {
	paths, err := a.ListCodexConfigBackups()
	if err != nil {
		return 0, err
	}
	removed := 0
	for _, p := range paths {
		if _, err := a.DeleteCodexConfigBackup(p); err != nil {
			return removed, err
		}
		removed++
	}
	return removed, nil
}

func (a *App) RestoreCodexConfigToml() (string, error) {
	path, err := codexConfigPath()
	if err != nil {
		return "", err
	}

	if backups, listErr := a.ListCodexConfigBackups(); listErr == nil && len(backups) > 0 {
		return a.RestoreCodexConfigTomlFromBackup(backups[0])
	}

	existing, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	updated, changed, err := removeCodexBridgeFromConfig(existing)
	if err != nil {
		return "", err
	}
	if !changed {
		return path, nil
	}

	if err := os.WriteFile(path, updated, 0o600); err != nil {
		return "", err
	}

	a.appendLog("info", "app", "已从 Codex config.toml 移除 local-bridge 配置: "+path, "")
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
	delete(doc, "model_catalog_json")
	doc["profile"] = codexProviderID
	if strings.TrimSpace(defaultModel) != "" {
		doc["model"] = defaultModel
	} else if _, ok := doc["model"]; !ok {
		doc["model"] = "deepseek-chat"
	}

	modelProviders := ensureTomlMap(doc, "model_providers")
	provider := ensureTomlMap(modelProviders, codexProviderID)
	provider["name"] = "Local Proxy (DeepSeek)"
	provider["base_url"] = baseURL
	provider["wire_api"] = "responses"
	delete(provider, "env_key")
	if _, ok := provider["query_params"]; !ok {
		provider["query_params"] = map[string]any{}
	}

	profiles := ensureTomlMap(doc, "profiles")
	profile := ensureTomlMap(profiles, codexProviderID)
	profile["model_provider"] = codexProviderID
	if model, ok := doc["model"].(string); ok && strings.TrimSpace(model) != "" {
		profile["model"] = model
	} else {
		profile["model"] = "deepseek-chat"
	}
	profile["openai_base_url"] = strings.TrimRight(baseURL, "/") + "/"
	delete(profile, "model_catalog_json")

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

	modelProvidersAny, ok := doc["model_providers"]
	if ok {
		if modelProviders, ok := modelProvidersAny.(map[string]any); ok {
			if _, has := modelProviders[codexProviderID]; has {
				delete(modelProviders, codexProviderID)
				changed = true
			}
			if len(modelProviders) == 0 {
				delete(doc, "model_providers")
				changed = true
			} else {
				doc["model_providers"] = modelProviders
			}

			if current, ok := doc["model_provider"].(string); ok && current == codexProviderID {
				for k := range modelProviders {
					doc["model_provider"] = k
					changed = true
					break
				}
				if doc["model_provider"] == codexProviderID {
					delete(doc, "model_provider")
					changed = true
				}
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

func ensureTomlMap(parent map[string]any, key string) map[string]any {
	if value, ok := parent[key]; ok {
		if m, ok := value.(map[string]any); ok {
			return m
		}
	}

	m := map[string]any{}
	parent[key] = m
	return m
}
