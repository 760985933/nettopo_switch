package main

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const claudeSettingsDir = ".claude"
const claudeSettingsFile = "settings.json"

// ── Claude-3p gateway config types ──

const claude3pConfigDir = "Claude-3p"
const claude3pConfigLibrary = "configLibrary"
const claude3pMetaFile = "_meta.json"

// claude3pGatewayConfig mirrors the JSON schema that the Claude-3p desktop
// app uses to declare a gateway inference provider.
type claude3pGatewayConfig struct {
	CoworkEgressAllowedHosts     []string           `json:"coworkEgressAllowedHosts"`
	DisableDeploymentModeChooser bool               `json:"disableDeploymentModeChooser"`
	InferenceGatewayApiKey       string             `json:"inferenceGatewayApiKey"`
	InferenceGatewayAuthScheme   string             `json:"inferenceGatewayAuthScheme"`
	InferenceGatewayBaseURL      string             `json:"inferenceGatewayBaseUrl"`
	InferenceModels              []claude3pModelDef `json:"inferenceModels"`
	InferenceProvider            string             `json:"inferenceProvider"`
}

type claude3pModelDef struct {
	Name          string `json:"name"`
	LabelOverride string `json:"labelOverride,omitempty"`
	Supports1M    bool   `json:"supports1m,omitempty"`
}

// claude3pMeta represents the _meta.json file that controls profile switching
// in the Claude-3p config library.
type claude3pMeta struct {
	AppliedID string            `json:"appliedId"`
	Entries   map[string]string `json:"entries"` // UUID → label
}

// generateUUID returns a random UUID v4 string.
func generateUUID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("生成随机 UUID 失败: %w", err)
	}
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 10
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}

// getClaudeSettingsPath returns the full path to the Claude Code settings file.
func getClaudeSettingsPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, claudeSettingsDir, claudeSettingsFile), nil
}

// getClaude3pConfigLibPath returns the platform-appropriate Claude-3p configLibrary path.
// macOS:  ~/Library/Application Support/Claude-3p/configLibrary/
// Windows: %AppData%/Claude-3p/configLibrary/
// Linux:  ~/.config/Claude-3p/configLibrary/
func getClaude3pConfigLibPath() (string, error) {
	cfgDir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(cfgDir, claude3pConfigDir, claude3pConfigLibrary), nil
}

func (a *App) GetClaudeSettingsPath() (string, error) {
	return getClaudeSettingsPath()
}

func (a *App) ReadClaudeSettings() (string, error) {
	path, err := getClaudeSettingsPath()
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (a *App) WriteClaudeSettings(content string) (string, error) {
	path, err := getClaudeSettingsPath()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		return "", err
	}
	a.appendLog("info", "app", "已保存 Claude Code 设置: "+path, "")
	return path, nil
}

// EnableClaudeSettings writes the given profile's model and endpoint
// configuration into ~/.claude/settings.json AND the Claude-3p desktop
// gateway config (~/Library/Application Support/Claude-3p/configLibrary/).
// The profileID must match a profile registered under the Claude instance.
func (a *App) EnableClaudeSettings(profileID string) (string, error) {
	cfg, err := a.GetAppConfig()
	if err != nil {
		return "", err
	}

	profile, ok := cfg.Profiles[profileID]
	if !ok {
		return "", errors.New("指定的模型配置不存在: " + profileID)
	}

	// Generate a random UUID for the gateway config file.
	gwUUID, err := generateUUID()
	if err != nil {
		return "", err
	}

	// Update the Claude instance's current profile to this one,
	// and store the gateway UUID for later cleanup.
	if inst, ok := cfg.Instances[SourceClaude]; ok {
		inst.CurrentProfileID = profileID
		inst.GatewayConfigUUID = gwUUID
		// Also add to proxy profile IDs if not already present.
		found := false
		for _, id := range inst.ProxyProfileIDs {
			if id == profileID {
				found = true
				break
			}
		}
		if !found {
			inst.ProxyProfileIDs = append(inst.ProxyProfileIDs, profileID)
		}
		// Save the updated config so the proxy picks it up.
		if _, saveErr := a.SaveAppConfig(cfg); saveErr != nil {
			return "", fmt.Errorf("保存配置失败: %w", err)
		}
	}

	if strings.TrimSpace(profile.APIKey) == "" {
		return "", errors.New("当前配置未设置 API Key")
	}

	baseURL := profile.BaseURL

	// Resolve tiered models from the provider when available; otherwise fall
	// back to the profile default for every slot.
	haikuModel := profile.DefaultModel
	sonnetModel := profile.DefaultModel
	opusModel := profile.DefaultModel
	defaultModel := profile.DefaultModel

	// Gateway model names — default to Anthropic tier models, overridable via Claude*Model.
	claudeHaiku := profile.DefaultModel
	claudeSonnet := profile.DefaultModel
	claudeOpus := profile.DefaultModel

	if prov := GetProvider(ProviderID(profile.Provider)); prov != nil {
		if prov.AnthropicBaseURL != "" {
			baseURL = prov.AnthropicBaseURL
		}
		if prov.AnthropicHaikuModel != "" {
			haikuModel = prov.AnthropicHaikuModel
		}
		if prov.AnthropicSonnetModel != "" {
			sonnetModel = prov.AnthropicSonnetModel
		}
		if prov.AnthropicOpusModel != "" {
			opusModel = prov.AnthropicOpusModel
		}
		if prov.ClaudeHaikuModel != "" {
			claudeHaiku = prov.ClaudeHaikuModel
		} else {
			claudeHaiku = haikuModel
		}
		if prov.ClaudeSonnetModel != "" {
			claudeSonnet = prov.ClaudeSonnetModel
		} else {
			claudeSonnet = sonnetModel
		}
		if prov.ClaudeOpusModel != "" {
			claudeOpus = prov.ClaudeOpusModel
		} else {
			claudeOpus = opusModel
		}
	}

	// 1. Write ~/.claude/settings.json
	settings := map[string]any{
		"env": map[string]string{
			"ANTHROPIC_AUTH_TOKEN":           profile.APIKey,
			"ANTHROPIC_BASE_URL":             baseURL,
			"ANTHROPIC_DEFAULT_HAIKU_MODEL":  haikuModel,
			"ANTHROPIC_DEFAULT_OPUS_MODEL":   opusModel,
			"ANTHROPIC_DEFAULT_SONNET_MODEL": sonnetModel,
			"ANTHROPIC_MODEL":                defaultModel,
		},
		"experimental": map[string]bool{
			"strip_metadata_user_id": true,
		},
	}

	content, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return "", err
	}

	path, err := a.WriteClaudeSettings(string(content))
	if err != nil {
		return "", err
	}

	// Build the local gateway base URL from the Claude instance's listen address.
	var gatewayBaseURL string
	if inst, ok := cfg.Instances[SourceClaude]; ok {
		gatewayBaseURL = fmt.Sprintf("http://%s:%d", inst.ListenHost, inst.ListenPort)
	} else {
		gatewayBaseURL = "http://127.0.0.1:17420"
	}

	// 2. Write Claude-3p gateway config so the desktop app also works.
	model1M := make(map[string]bool, len(profile.ClaudeModel1M))
	for _, m := range profile.ClaudeModel1M {
		// Resolve Claude-side model name (mapping key) to provider-side
		// model name so the Supports1M lookup matches gateway model names.
		providerModel := m
		if mapped, ok := profile.Mappings[m]; ok && mapped != "" {
			providerModel = mapped
		}
		model1M[providerModel] = true
	}
	if gwPath, gwErr := a.enableClaude3pGateway(gwUUID, gatewayBaseURL, claudeHaiku, claudeSonnet, claudeOpus, profile.Name, model1M); gwErr != nil {
		a.appendLog("warn", "app", fmt.Sprintf("Claude-3p gateway 配置写入失败: %v", gwErr), "")
	} else {
		a.appendLog("info", "app", "已写入 Claude-3p gateway 配置: "+gwPath, "")
	}

	return path, nil
}

// enableClaude3pGateway writes a gateway config JSON file with a random UUID
// and updates _meta.json under the Claude-3p configLibrary directory.
func (a *App) enableClaude3pGateway(uuid, gatewayBaseURL, haikuModel, sonnetModel, opusModel, profileName string, model1M map[string]bool) (string, error) {
	libPath, err := getClaude3pConfigLibPath()
	if err != nil {
		return "", err
	}
	if err = os.MkdirAll(libPath, 0o755); err != nil {
		return "", fmt.Errorf("创建 configLibrary 目录失败: %w", err)
	}

	if model1M == nil {
		model1M = map[string]bool{}
	}

	// Generate a random API key for the gateway.
	apiKeyStr, err := generateUUID()
	if err != nil {
		return "", err
	}
	apiKey := "ccs-" + apiKeyStr

	gw := claude3pGatewayConfig{
		CoworkEgressAllowedHosts:     []string{"*"},
		DisableDeploymentModeChooser: true,
		InferenceGatewayApiKey:       apiKey,
		InferenceGatewayAuthScheme:   "bearer",
		InferenceGatewayBaseURL:      strings.TrimRight(gatewayBaseURL, "/"),
		InferenceModels: []claude3pModelDef{
			{Name: opusModel, LabelOverride: profileName + " Opus", Supports1M: model1M[opusModel]},
			{Name: sonnetModel, LabelOverride: profileName + " Sonnet", Supports1M: model1M[sonnetModel]},
			{Name: haikuModel, LabelOverride: profileName + " Haiku", Supports1M: model1M[haikuModel]},
		},
		InferenceProvider: "gateway",
	}

	gwPath := filepath.Join(libPath, uuid+".json")
	gwData, err := json.MarshalIndent(gw, "", "  ")
	if err != nil {
		return "", fmt.Errorf("序列化 gateway 配置失败: %w", err)
	}
	if err := os.WriteFile(gwPath, gwData, 0o600); err != nil {
		return "", fmt.Errorf("写入 gateway 配置失败: %w", err)
	}


	// Update _meta.json to activate this gateway config.
	if err := a.updateClaude3pMeta(uuid, profileName); err != nil {
		return "", err
	}

	return gwPath, nil
}

// updateClaude3pMeta sets the active config entry in _meta.json.
// This is append-only: existing entries from other applications are always preserved.
func (a *App) updateClaude3pMeta(uuid, label string) error {
	libPath, err := getClaude3pConfigLibPath()
	if err != nil {
		return err
	}
	if err = os.MkdirAll(libPath, 0o755); err != nil {
		return fmt.Errorf("创建 configLibrary 目录失败: %w", err)
	}
	metaPath := filepath.Join(libPath, claude3pMetaFile)

	meta := claude3pMeta{
		AppliedID: uuid,
		Entries:   map[string]string{},
	}

	// Read existing _meta.json first — always preserve other entries.
	if data, readErr := os.ReadFile(metaPath); readErr == nil {
		var existing claude3pMeta
		if unmarshalErr := json.Unmarshal(data, &existing); unmarshalErr == nil {
			meta.Entries = existing.Entries
		}
	} else if !os.IsNotExist(readErr) {
		return fmt.Errorf("读取 _meta.json 失败: %w", readErr)
	}

	// Merge the new entry (append-only).
	meta.Entries[uuid] = label
	meta.AppliedID = uuid

	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化 _meta.json 失败: %w", err)
	}
	if err := os.WriteFile(metaPath, []byte(data), 0o600); err != nil {
		return fmt.Errorf("写入 _meta.json 失败: %w", err)
	}
	return nil
}

// RestoreClaudeSettings writes an empty settings object to ~/.claude/settings.json
// and removes the Claude-3p gateway config, so Claude Code falls back to its
// built-in defaults.
func (a *App) RestoreClaudeSettings() (string, error) {
	// 1. Overwrite ~/.claude/settings.json with empty object (not delete).
	path, err := getClaudeSettingsPath()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, []byte("{}\n"), 0o600); err != nil {
		return "", err
	}
	a.appendLog("info", "app", "已重置 Claude Code 设置: "+path, "")

	// 2. Clean up Claude-3p gateway config.
	a.restoreClaude3pGateway()

	return path, nil
}

// restoreClaude3pGateway removes the gateway config JSON and updates _meta.json.
func (a *App) restoreClaude3pGateway() {
	libPath, err := getClaude3pConfigLibPath()
	if err != nil {
		return
	}

	// Determine the current gateway UUID from the Claude instance config.
	cfg, cfgErr := a.GetAppConfig()
	var uuidToRemove string
	if cfgErr == nil {
		if inst, ok := cfg.Instances[SourceClaude]; ok && inst.GatewayConfigUUID != "" {
			uuidToRemove = inst.GatewayConfigUUID
		}
	}

	// Remove the gateway config file(s).
	gwPath := filepath.Join(libPath, uuidToRemove+".json")
	if err := os.Remove(gwPath); err != nil && !os.IsNotExist(err) {
		a.appendLog("warn", "app", "移除 Claude-3p gateway 配置失败: "+err.Error(), "")
	}


	// Remove the entry from _meta.json.
	metaPath := filepath.Join(libPath, claude3pMetaFile)
	if data, err := os.ReadFile(metaPath); err == nil {
		var meta claude3pMeta
		if err := json.Unmarshal(data, &meta); err == nil {
			delete(meta.Entries, uuidToRemove)

			if meta.AppliedID == uuidToRemove {
				meta.AppliedID = ""
				// If there are other entries, pick the first one.
				for id := range meta.Entries {
					meta.AppliedID = id
					break
				}
			}
			if len(meta.Entries) == 0 {
				_ = os.Remove(metaPath)
			} else {
				cleaned, _ := json.MarshalIndent(meta, "", "  ")
				_ = os.WriteFile(metaPath, cleaned, 0o600)
			}
		}
	}
}
