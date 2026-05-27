package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	goRuntime "runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

const appVersion = "0.0.6"
const updateManifestURL = "https://nettopo.com/nettopo-switch-version.txt"
const updateDownloadURLTemplate = ""

type App struct {
	ctx   context.Context
	store *ConfigStore
	proxy *ProxyRuntime

	mu     sync.RWMutex
	config AppConfig

	logsMu sync.RWMutex
	logs   []LogEntry
}

func NewApp() *App {
	store, err := NewConfigStore()
	if err != nil {
		panic(err)
	}

	app := &App{
		store: store,
	}
	app.proxy = NewProxyRuntime(app)
	return app
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	cfg, err := a.store.Load()
	if err != nil {
		a.appendLog("error", "app", "读取本地配置失败: "+err.Error(), "")
		cfg = defaultConfig()
	}

	a.mu.Lock()
	a.config = cfg
	a.mu.Unlock()

	a.appendLog("info", "app", "配置文件路径: "+a.store.Path(), "")

	if cfg.EnableAutoStart {
		go func() {
			time.Sleep(800 * time.Millisecond)
			if _, err := a.StartProxy(); err != nil {
				a.appendLog("error", "app", "自动启动失败: "+err.Error(), "")
			}
		}()
	}
}

func (a *App) GetAppVersion() string {
	return "v" + appVersion
}

func (a *App) CheckForUpdates() (UpdateCheckResult, error) {
	now := time.Now()
	result := UpdateCheckResult{
		CurrentVersion: "v" + appVersion,
		CheckedAt:      now.Format(time.RFC3339),
	}

	manifestURL := strings.TrimSpace(os.Getenv("NETTOPO_SWITCH_UPDATE_URL"))
	if manifestURL == "" {
		manifestURL = updateManifestURL
	}
	if manifestURL == "" {
		return result, errors.New("未配置更新地址")
	}

	client := &http.Client{Timeout: 8 * time.Second}
	req, err := http.NewRequest(http.MethodGet, manifestURL, nil)
	if err != nil {
		return result, err
	}
	req.Header.Set("User-Agent", "nettopo-switch/"+appVersion)
	resp, err := client.Do(req)
	if err != nil {
		return result, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= http.StatusBadRequest {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return result, fmt.Errorf("更新检查失败: %s", strings.TrimSpace(string(raw)))
	}

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		return result, err
	}

	latest, downloadURL, notes, err := parseUpdateManifest(raw)
	if err != nil {
		return result, err
	}
	result.LatestVersion = latest
	result.DownloadURL = strings.TrimSpace(downloadURL)
	result.Notes = strings.TrimSpace(notes)

	if compareSemver(latest, "v"+appVersion) > 0 {
		result.HasUpdate = true
		if result.DownloadURL == "" {
			result.DownloadURL = buildDownloadURL(latest)
		}
	}

	return result, nil
}

func parseUpdateManifest(raw []byte) (string, string, string, error) {
	text := strings.TrimSpace(string(raw))
	if text == "" {
		return "", "", "", errors.New("更新描述为空")
	}
	if strings.HasPrefix(text, "{") {
		var payload map[string]any
		if err := json.Unmarshal([]byte(text), &payload); err != nil {
			return "", "", "", err
		}
		version, _ := payload["version"].(string)
		if strings.TrimSpace(version) == "" {
			version, _ = payload["latest"].(string)
		}
		if strings.TrimSpace(version) == "" {
			return "", "", "", errors.New("更新描述缺少 version")
		}
		url, _ := payload["url"].(string)
		notes, _ := payload["notes"].(string)
		return strings.TrimSpace(version), strings.TrimSpace(url), strings.TrimSpace(notes), nil
	}

	lines := strings.Split(text, "\n")
	version := ""
	url := ""
	notes := ""
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.Contains(line, "=") {
			parts := strings.SplitN(line, "=", 2)
			key := strings.ToLower(strings.TrimSpace(parts[0]))
			val := strings.TrimSpace(parts[1])
			switch key {
			case "version", "latest":
				version = val
			case "url", "download":
				url = val
			case "notes":
				notes = val
			}
			continue
		}
		if version == "" {
			version = line
			continue
		}
		if url == "" {
			url = line
			continue
		}
		if notes == "" {
			notes = line
		}
	}
	if strings.TrimSpace(version) == "" {
		return "", "", "", errors.New("更新描述缺少 version")
	}
	return version, url, notes, nil
}

func compareSemver(a string, b string) int {
	pa := parseSemverParts(a)
	pb := parseSemverParts(b)
	for i := 0; i < 3; i++ {
		if pa[i] > pb[i] {
			return 1
		}
		if pa[i] < pb[i] {
			return -1
		}
	}
	return 0
}

func parseSemverParts(v string) [3]int {
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(strings.ToLower(v), "v")
	if idx := strings.IndexAny(v, "+-"); idx >= 0 {
		v = v[:idx]
	}
	out := [3]int{}
	parts := strings.Split(v, ".")
	for i := 0; i < 3 && i < len(parts); i++ {
		n, _ := strconv.Atoi(strings.TrimSpace(parts[i]))
		out[i] = n
	}
	return out
}

func buildDownloadURL(latest string) string {
	template := strings.TrimSpace(os.Getenv("NETTOPO_SWITCH_DOWNLOAD_URL_TEMPLATE"))
	if template == "" {
		template = updateDownloadURLTemplate
	}
	if template == "" {
		return ""
	}
	ver := strings.TrimPrefix(strings.TrimSpace(latest), "v")
	ext := "zip"
	if goRuntime.GOOS == "windows" {
		ext = "exe"
	}
	if goRuntime.GOOS == "darwin" {
		ext = "app"
	}
	out := strings.ReplaceAll(template, "{version}", ver)
	out = strings.ReplaceAll(out, "{os}", goRuntime.GOOS)
	out = strings.ReplaceAll(out, "{arch}", goRuntime.GOARCH)
	out = strings.ReplaceAll(out, "{ext}", ext)
	return out
}

func (a *App) GetAppConfig() (AppConfig, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.config, nil
}

func (a *App) SaveAppConfig(cfg AppConfig) (AppConfig, error) {
	cfg = normalizeConfig(cfg)
	if err := validateConfig(cfg, false); err != nil {
		a.appendLog("warn", "app", "配置校验失败: "+err.Error(), "")
		return AppConfig{}, err
	}
	if err := a.store.Save(cfg); err != nil {
		a.appendLog("error", "app", "保存配置失败: "+err.Error(), "")
		return AppConfig{}, err
	}

	a.mu.Lock()
	a.config = cfg
	a.mu.Unlock()

	a.appendLog("info", "app", "配置已保存", "")
	return cfg, nil
}

func (a *App) SetCurrentProfile(id string) (AppConfig, error) {
	cfg, err := a.GetAppConfig()
	if err != nil {
		return AppConfig{}, err
	}

	if _, ok := cfg.Profiles[id]; !ok {
		return AppConfig{}, errors.New("配置 ID 不存在: " + id)
	}

	cfg.CurrentProfileID = id
	cfg = normalizeConfig(cfg)

	if err := a.store.Save(cfg); err != nil {
		return AppConfig{}, err
	}

	a.mu.Lock()
	a.config = cfg
	a.mu.Unlock()

	a.appendLog("info", "app", "已切换到配置: "+cfg.Profiles[id].Name, "")
	return cfg, nil
}

func (a *App) ExportConfig() (string, error) {
	cfg, err := a.GetAppConfig()
	if err != nil {
		return "", err
	}
	content, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return "", err
	}
	return string(content), nil
}

func (a *App) ImportConfig(payload string) (AppConfig, error) {
	var cfg AppConfig
	if err := json.Unmarshal([]byte(payload), &cfg); err != nil {
		return AppConfig{}, err
	}
	return a.SaveAppConfig(cfg)
}

func (a *App) StartProxy() (ProxyStatusPayload, error) {
	cfg, err := a.GetAppConfig()
	if err != nil {
		return ProxyStatusPayload{}, err
	}
	profile, ok := cfg.Profiles[cfg.CurrentProfileID]
	if !ok {
		return ProxyStatusPayload{}, errors.New("当前配置不存在")
	}
	a.appendLog("info", "app", fmt.Sprintf("收到启动请求 [%s]: %s:%d -> %s (%s)", profile.Name, cfg.ListenHost, cfg.ListenPort, profile.BaseURL, profile.DefaultModel), "")
	if err := validateConfig(cfg, true); err != nil {
		a.appendLog("warn", "app", "启动前配置校验失败: "+err.Error(), "")
		return ProxyStatusPayload{}, err
	}
	if err := a.proxy.Start(cfg); err != nil {
		a.appendLog("error", "app", "启动代理失败: "+err.Error(), "")
		return ProxyStatusPayload{}, err
	}
	status := a.proxy.Status()
	a.appendLog("info", "app", "启动命令已提交: "+status.ListenAddress, "")

	// Plugin unlock injection (non-blocking)
	if cfg.PluginUnlockEnabled {
		go func() {
			if err := TryPluginUnlock(a.appendLog); err != nil {
				a.appendLog("warn", "plugin", "插件解锁失败: "+err.Error(), "")
			}
		}()
	}

	return status, nil
}

func (a *App) StopProxy() (ProxyStatusPayload, error) {
	a.appendLog("info", "app", "收到停止请求", "")
	if err := a.proxy.Stop(); err != nil {
		a.appendLog("error", "app", "停止代理失败: "+err.Error(), "")
		return ProxyStatusPayload{}, err
	}
	return a.proxy.Status(), nil
}

func (a *App) RestartProxy() (ProxyStatusPayload, error) {
	a.appendLog("info", "app", "收到重启请求", "")
	if err := a.proxy.Stop(); err != nil {
		a.appendLog("error", "app", "重启时停止失败: "+err.Error(), "")
		return ProxyStatusPayload{}, err
	}
	return a.StartProxy()
}

func (a *App) GetProxyStatus() ProxyStatusPayload {
	return a.proxy.Status()
}

func (a *App) GetOverviewSnapshot() (OverviewSnapshot, error) {
	cfg, err := a.GetAppConfig()
	if err != nil {
		return OverviewSnapshot{}, err
	}

	profileName := ""
	if p, ok := cfg.Profiles[cfg.CurrentProfileID]; ok {
		profileName = p.Name
	}

	return OverviewSnapshot{
		Config:     cfg,
		Status:     a.proxy.Status(),
		RecentLogs: a.GetLogHistory(6),
		QuickTips: []string{
			"先填写 API Base URL、API Key 和默认模型。",
			"启动后将本地地址填入 Codex Desktop 的服务端点。",
			"请求失败时先查看最近日志，再进入完整诊断页。",
		},
		Defaults: map[string]string{
			"baseURL":     "https://api.deepseek.com/v1",
			"model":       "deepseek-v4-flash",
			"profileName": profileName,
		},
		Features: map[string]bool{
			"streamingProxy":   true,
			"healthCheck":      true,
			"logPush":          true,
			"compactDashboard": cfg.CompactMode,
			"pluginUnlock":     true,
		},
	}, nil
}

func (a *App) RunHealthCheck() (HealthCheckResult, error) {
	cfg, err := a.GetAppConfig()
	if err != nil {
		return HealthCheckResult{}, err
	}

	result := HealthCheckResult{
		OK:     true,
		Checks: make([]HealthCheckItem, 0, 3),
	}

	if err := validateConfig(cfg, true); err != nil {
		result.OK = false
		result.Checks = append(result.Checks, HealthCheckItem{
			Name:    "配置完整性",
			OK:      false,
			Message: err.Error(),
		})
	} else {
		result.Checks = append(result.Checks, HealthCheckItem{
			Name:    "配置完整性",
			OK:      true,
			Message: "核心配置已填写",
		})
	}

	if a.proxy.IsRunning() {
		result.Checks = append(result.Checks, HealthCheckItem{
			Name:    "本地代理服务",
			OK:      true,
			Message: "代理服务正在运行: " + a.proxy.Status().ListenAddress,
		})
	} else {
		result.OK = false
		result.Checks = append(result.Checks, HealthCheckItem{
			Name:    "本地代理服务",
			OK:      false,
			Message: "代理服务未启动",
		})
	}

	upstreamErr := a.proxy.CheckUpstream(cfg)
	if upstreamErr != nil {
		result.OK = false
		result.Checks = append(result.Checks, HealthCheckItem{
			Name:    "DeepSeek 上游接口",
			OK:      false,
			Message: upstreamErr.Error(),
		})
	} else {
		result.Checks = append(result.Checks, HealthCheckItem{
			Name:    "DeepSeek 上游接口",
			OK:      true,
			Message: "上游接口可访问",
		})
	}

	if status, msg, at := a.proxy.getLastUpstreamFailure(); status == 402 && !at.IsZero() && time.Since(at) < 24*time.Hour {
		result.OK = false
		hint := "检测到最近一次上游请求返回 402（余额不足/额度不足）。请充值或更换 API Key。"
		if strings.TrimSpace(msg) != "" {
			hint += " " + strings.TrimSpace(msg)
		}
		result.Checks = append(result.Checks, HealthCheckItem{
			Name:    "DeepSeek 余额/额度",
			OK:      false,
			Message: hint,
		})
	}

	return result, nil
}

func (a *App) GetUsageBalance() UsageBalance {
	cfg, err := a.GetAppConfig()
	if err != nil {
		return UsageBalance{Error: err.Error()}
	}

	profile, ok := cfg.Profiles[cfg.CurrentProfileID]
	if !ok {
		return UsageBalance{Error: "当前配置不存在"}
	}

	if strings.TrimSpace(profile.APIKey) == "" {
		return UsageBalance{Error: "API Key 未配置"}
	}

	baseURL := strings.TrimRight(profile.BaseURL, "/")
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return UsageBalance{Error: "Base URL 格式错误"}
	}

	// Build balance URL from scheme + host (strip path like /v1)
	balanceURL := fmt.Sprintf("%s://%s/user/balance", parsed.Scheme, parsed.Host)

	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest(http.MethodGet, balanceURL, nil)
	if err != nil {
		return UsageBalance{Error: err.Error()}
	}
	req.Header.Set("Authorization", "Bearer "+profile.APIKey)
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return UsageBalance{Error: err.Error()}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return UsageBalance{Error: fmt.Sprintf("API 返回状态 %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))}
	}

	var balanceResp struct {
		IsAvailable      bool   `json:"is_available"`
		AvailableBalance string `json:"available_balance"`
		TotalBalance     string `json:"total_balance"`
		IsDepleted       bool   `json:"is_depleted"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&balanceResp); err != nil {
		return UsageBalance{Error: "解析响应失败: " + err.Error()}
	}

	return UsageBalance{
		AvailableBalance: balanceResp.AvailableBalance,
		TotalBalance:     balanceResp.TotalBalance,
		IsDepleted:       balanceResp.IsDepleted,
	}
}

func (a *App) GetLogHistory(limit int) []LogEntry {
	a.logsMu.RLock()
	defer a.logsMu.RUnlock()

	if limit <= 0 || limit >= len(a.logs) {
		return append([]LogEntry(nil), a.logs...)
	}

	start := len(a.logs) - limit
	return append([]LogEntry(nil), a.logs[start:]...)
}

func (a *App) appendLog(level string, source string, message string, requestID string) {
	entry := LogEntry{
		ID:        fmt.Sprintf("%d", time.Now().UnixNano()),
		Level:     level,
		Timestamp: time.Now().Format(time.RFC3339),
		Source:    source,
		Message:   message,
		RequestID: requestID,
	}

	a.logsMu.Lock()
	a.logs = append(a.logs, entry)
	if len(a.logs) > 500 {
		a.logs = a.logs[len(a.logs)-500:]
	}
	a.logsMu.Unlock()

	if a.ctx != nil {
		ctx := a.ctx
		go runtime.EventsEmit(ctx, "log:entry", entry)
	}
}

func (a *App) emitStatus() {
	if a.ctx != nil {
		ctx := a.ctx
		payload := a.proxy.Status()
		go runtime.EventsEmit(ctx, "proxy:status", payload)
	}
}

func validateConfig(cfg AppConfig, requireCredentials bool) error {
	if strings.TrimSpace(cfg.ListenHost) == "" {
		return errors.New("监听地址不能为空")
	}
	if cfg.ListenPort <= 0 {
		return errors.New("监听端口必须大于 0")
	}

	profile, ok := cfg.Profiles[cfg.CurrentProfileID]
	if !ok {
		return errors.New("当前配置不存在")
	}

	if strings.TrimSpace(profile.BaseURL) == "" {
		return errors.New("API Base URL 不能为空")
	}
	if parsed, err := url.Parse(strings.TrimSpace(profile.BaseURL)); err == nil && parsed.Host != "" {
		if parsed.Port() != "" {
			if net.JoinHostPort(parsed.Hostname(), parsed.Port()) == net.JoinHostPort(strings.TrimSpace(cfg.ListenHost), fmt.Sprintf("%d", cfg.ListenPort)) {
				return errors.New("Base URL 不能指向本代理地址（会导致请求循环）")
			}
		} else if parsed.Hostname() == strings.TrimSpace(cfg.ListenHost) {
			return errors.New("Base URL 不能指向本代理地址（会导致请求循环）")
		}
	}
	if requireCredentials && strings.TrimSpace(profile.APIKey) == "" {
		return errors.New("API Key 不能为空")
	}
	if strings.TrimSpace(profile.DefaultModel) == "" {
		return errors.New("默认模型不能为空")
	}
	return nil
}
