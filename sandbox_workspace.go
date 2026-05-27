package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"

	toml "github.com/pelletier/go-toml/v2"
)

const (
	defaultSandboxMode    = "workspace-write"
	defaultApprovalPolicy = "on-request"
)

func sandboxConfigPath() (string, error) {
	return codexConfigPath()
}

func sandboxReadSection(doc map[string]any) (SandboxWorkspaceConfig, bool) {
	cfg := SandboxWorkspaceConfig{
		NetworkAccess:  true,
		SandboxMode:    defaultSandboxMode,
		ApprovalPolicy: defaultApprovalPolicy,
	}
	if sw, ok := doc["sandbox_workspace_write"]; ok {
		if swMap, ok := sw.(map[string]any); ok {
			if na, ok := swMap["network_access"]; ok {
				if b, ok := na.(bool); ok {
					cfg.NetworkAccess = b
				}
			}
			if sm, ok := swMap["sandbox_mode"]; ok {
				if s, ok := sm.(string); ok && s != "" {
					cfg.SandboxMode = s
				}
			}
			if ap, ok := swMap["approval_policy"]; ok {
				if s, ok := ap.(string); ok && s != "" {
					cfg.ApprovalPolicy = s
				}
			}
		}
		return cfg, true
	}
	return cfg, false
}

func (a *App) GetSandboxConfig() (SandboxWorkspaceConfig, error) {
	path, err := sandboxConfigPath()
	if err != nil {
		return SandboxWorkspaceConfig{}, err
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return SandboxWorkspaceConfig{
				NetworkAccess:  true,
				SandboxMode:    defaultSandboxMode,
				ApprovalPolicy: defaultApprovalPolicy,
			}, nil
		}
		return SandboxWorkspaceConfig{}, err
	}

	doc := map[string]any{}
	if len(bytes.TrimSpace(raw)) > 0 {
		if err := toml.Unmarshal(raw, &doc); err != nil {
			return SandboxWorkspaceConfig{}, err
		}
	}

	cfg, _ := sandboxReadSection(doc)
	return cfg, nil
}

func sandboxConfigsEqual(a, b SandboxWorkspaceConfig) bool {
	return a.NetworkAccess == b.NetworkAccess &&
		a.SandboxMode == b.SandboxMode &&
		a.ApprovalPolicy == b.ApprovalPolicy
}

func (a *App) SetSandboxConfig(cfg SandboxWorkspaceConfig) (SandboxWorkspaceConfig, error) {
	path, err := sandboxConfigPath()
	if err != nil {
		return SandboxWorkspaceConfig{}, err
	}

	raw := []byte{}
	existing, readErr := os.ReadFile(path)
	if readErr == nil && len(existing) > 0 {
		raw = existing
	}

	doc := map[string]any{}
	if len(bytes.TrimSpace(raw)) > 0 {
		if err := toml.Unmarshal(raw, &doc); err != nil {
			return SandboxWorkspaceConfig{}, err
		}
	}

	// Dedup: skip write only if the section already exists and values haven't changed
	current, found := sandboxReadSection(doc)
	if found && sandboxConfigsEqual(current, cfg) {
		return cfg, nil
	}

	doc["sandbox_workspace_write"] = map[string]any{
		"network_access":  cfg.NetworkAccess,
		"sandbox_mode":    cfg.SandboxMode,
		"approval_policy": cfg.ApprovalPolicy,
	}

	out, err := toml.Marshal(doc)
	if err != nil {
		return SandboxWorkspaceConfig{}, err
	}

	if mkErr := os.MkdirAll(filepath.Dir(path), 0o755); mkErr != nil {
		return SandboxWorkspaceConfig{}, mkErr
	}

	if err := os.WriteFile(path, out, 0o600); err != nil {
		return SandboxWorkspaceConfig{}, err
	}

	a.appendLog("info", "app", "已更新 sandbox 配置 → "+path, "")
	return cfg, nil
}
