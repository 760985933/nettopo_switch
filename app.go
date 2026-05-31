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
	"path/filepath"
	goRuntime "runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// mainAppCtx is used by platform-specific code (e.g. tray callbacks)
// that needs access to the Wails context outside of App methods.
var mainAppCtx context.Context
var forceQuit atomic.Bool

const appVersion = "0.0.9-fix2"
const updateManifestURL = "https://nettopo.com/nettopo-switch-version.txt"
const changelogURL = "https://nettopo.com/nettopo-switch-changelog.txt"
const updateDownloadURLTemplate = ""

type App struct {
	ctx        context.Context
	store      *ConfigStore
	proxies    map[SourceID]*ProxyRuntime
	usageStore *UsageStore

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

	configDir := filepath.Dir(store.Path())
	usageStore, err := NewUsageStore(configDir)
	if err != nil {
		fmt.Printf("初始化用量数据库失败: %v\n", err)
	} else {
		app.usageStore = usageStore
	}

	app.proxies = map[SourceID]*ProxyRuntime{
		SourceCodex:  NewProxyRuntime(app, SourceCodex),
		SourceClaude: NewProxyRuntime(app, SourceClaude),
	}
	return app
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	mainAppCtx = ctx

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

	a.initPlatformTray()
}

func (a *App) shutdown(ctx context.Context) {
	if a.usageStore != nil {
		if err := a.usageStore.Close(); err != nil {
			a.appendLog("error", "app", "关闭用量数据库失败: "+err.Error(), "")
		}
	}
}

func (a *App) GetAppVersion() string {
	return "v" + appVersion
}

func (a *App) ShouldHideOnClose() bool {
	if forceQuit.Load() {
		forceQuit.Store(false)
		return false
	}
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.config.MinimizeToTray
}

func (a *App) SetDebugMode(enabled bool) {
	for _, p := range a.proxies {
		p.SetDebugMode(enabled)
	}
}

func (a *App) GetDebugMode() bool {
	if p, ok := a.proxies[SourceCodex]; ok {
		return p.GetDebugMode()
	}
	return false
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

var changelogCachePath string

func (a *App) FetchChangelog() (ChangelogResult, error) {
	if changelogCachePath == "" {
		changelogCachePath = filepath.Join(filepath.Dir(a.store.Path()), "changelog-cache.txt")
	}

	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest(http.MethodGet, changelogURL, nil)
	if err != nil {
		return a.loadChangelogCache()
	}
	req.Header.Set("User-Agent", "nettopo-switch/"+appVersion)
	resp, err := client.Do(req)
	if err != nil {
		return a.loadChangelogCache()
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return a.loadChangelogCache()
	}

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 256*1024))
	if err != nil {
		return a.loadChangelogCache()
	}

	content := string(raw)
	if err := os.MkdirAll(filepath.Dir(changelogCachePath), 0o755); err != nil {
		a.appendLog("warn", "app", "创建更新记录缓存目录失败: "+err.Error(), "")
	} else if err := os.WriteFile(changelogCachePath, []byte(content), 0644); err != nil {
		a.appendLog("warn", "app", "写入更新记录缓存失败: "+err.Error(), "")
	}
	return ChangelogResult{Content: content, FromCache: false}, nil
}

func (a *App) loadChangelogCache() (ChangelogResult, error) {
	raw, err := os.ReadFile(changelogCachePath)
	if err != nil {
		return ChangelogResult{}, err
	}
	return ChangelogResult{Content: string(raw), FromCache: true}, nil
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

	for _, p := range a.proxies {
		p.RefreshConfig()
	}

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
	// 同步到 codex 实例，避免 normalizeConfig 用旧值覆盖
	if codexInst, ok := cfg.Instances[SourceCodex]; ok {
		codexInst.CurrentProfileID = id
	}
	cfg = normalizeConfig(cfg)

	if err := a.store.Save(cfg); err != nil {
		return AppConfig{}, err
	}

	a.mu.Lock()
	a.config = cfg
	a.mu.Unlock()

	for _, p := range a.proxies {
		p.RefreshConfig()
	}

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

// ── Backward-compatible proxy methods (default to SourceCodex) ──

func (a *App) StartProxy() (ProxyStatusPayload, error) {
	return a.StartProxyForSource(SourceCodex)
}

func (a *App) StopProxy() (ProxyStatusPayload, error) {
	return a.StopProxyForSource(SourceCodex)
}

func (a *App) RestartProxy() (ProxyStatusPayload, error) {
	return a.RestartProxyForSource(SourceCodex)
}

func (a *App) GetProxyStatus() ProxyStatusPayload {
	return a.GetProxyStatusForSource(SourceCodex)
}

// ── Source-aware proxy methods ──

func (a *App) StartProxyForSource(source SourceID) (ProxyStatusPayload, error) {
	proxy, ok := a.proxies[source]
	if !ok {
		return ProxyStatusPayload{}, errors.New("未知来源: " + string(source))
	}

	cfg, err := a.GetAppConfig()
	if err != nil {
		return ProxyStatusPayload{}, err
	}

	instCfg, ok := cfg.Instances[source]
	if !ok {
		return ProxyStatusPayload{}, errors.New("未知来源: " + string(source))
	}

	profile, ok := cfg.Profiles[instCfg.CurrentProfileID]
	if !ok {
		return ProxyStatusPayload{}, errors.New("当前配置不存在")
	}

	effective, ok := cfg.EffectiveConfig(source)
	if !ok {
		return ProxyStatusPayload{}, errors.New("无法构建有效配置")
	}

	a.appendLog("info", "app", fmt.Sprintf("收到启动请求 [%s] %s:%d -> %s (%s)", source, effective.ListenHost, effective.ListenPort, profile.BaseURL, profile.DefaultModel), "")
	if err := validateConfig(effective, true); err != nil {
		a.appendLog("warn", "app", "启动前配置校验失败 ["+string(source)+"]: "+err.Error(), "")
		return ProxyStatusPayload{}, err
	}
	if err := proxy.Start(effective); err != nil {
		a.appendLog("error", "app", "启动代理失败 ["+string(source)+"]: "+err.Error(), "")
		return ProxyStatusPayload{}, err
	}
	status := proxy.Status()
	a.appendLog("info", "app", "启动命令已提交 ["+string(source)+"]: "+status.ListenAddress, "")

	return status, nil
}

func (a *App) StopProxyForSource(source SourceID) (ProxyStatusPayload, error) {
	proxy, ok := a.proxies[source]
	if !ok {
		return ProxyStatusPayload{}, errors.New("未知来源: " + string(source))
	}
	a.appendLog("info", "app", "收到停止请求 ["+string(source)+"]", "")
	if err := proxy.Stop(); err != nil {
		a.appendLog("error", "app", "停止代理失败 ["+string(source)+"]: "+err.Error(), "")
		return ProxyStatusPayload{}, err
	}
	return proxy.Status(), nil
}

func (a *App) RestartProxyForSource(source SourceID) (ProxyStatusPayload, error) {
	a.appendLog("info", "app", "收到重启请求 ["+string(source)+"]", "")
	if _, err := a.StopProxyForSource(source); err != nil {
		a.appendLog("error", "app", "重启时停止失败 ["+string(source)+"]: "+err.Error(), "")
		return ProxyStatusPayload{}, err
	}
	return a.StartProxyForSource(source)
}

func (a *App) GetProxyStatusForSource(source SourceID) ProxyStatusPayload {
	if proxy, ok := a.proxies[source]; ok {
		return proxy.Status()
	}
	return ProxyStatusPayload{Source: source, Status: ProxyStopped}
}

func (a *App) GetOverviewSnapshot() (OverviewSnapshot, error) {
	return a.GetOverviewSnapshotForSource(SourceCodex)
}

func (a *App) GetOverviewSnapshotForSource(source SourceID) (OverviewSnapshot, error) {
	cfg, err := a.GetAppConfig()
	if err != nil {
		return OverviewSnapshot{}, err
	}

	instCfg, ok := cfg.Instances[source]
	if !ok {
		return OverviewSnapshot{}, errors.New("未知来源: " + string(source))
	}

	profileName := ""
	defaultBaseURL := "https://api.deepseek.com/v1"
	defaultModel := "deepseek-v4-flash"
	if p, ok := cfg.Profiles[instCfg.CurrentProfileID]; ok {
		profileName = p.Name
		prov := GetProvider(ProviderID(p.Provider))
		if prov != nil {
			defaultBaseURL = prov.DefaultBaseURL
			defaultModel = prov.DefaultModel
		} else {
			if p.BaseURL != "" {
				defaultBaseURL = p.BaseURL
			}
			if p.DefaultModel != "" {
				defaultModel = p.DefaultModel
			}
		}
	}

	status := a.GetProxyStatusForSource(source)

	return OverviewSnapshot{
		Config:     cfg,
		Status:     status,
		RecentLogs: a.GetLogHistory(6),
		QuickTips: []string{
			"先填写 API Base URL、API Key 和默认模型。",
			"启动后将本地地址填入 Codex Desktop 的服务端点。",
			"请求失败时先查看最近日志，再进入完整诊断页。",
		},
		Defaults: map[string]string{
			"baseURL":     defaultBaseURL,
			"model":       defaultModel,
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
	return a.RunHealthCheckForSource(SourceCodex)
}

func (a *App) RunHealthCheckForSource(source SourceID) (HealthCheckResult, error) {
	proxy, ok := a.proxies[source]
	if !ok {
		return HealthCheckResult{}, errors.New("未知来源: " + string(source))
	}

	cfg, err := a.GetAppConfig()
	if err != nil {
		return HealthCheckResult{}, err
	}

	effective, ok := cfg.EffectiveConfig(source)
	if !ok {
		return HealthCheckResult{}, errors.New("无法构建有效配置")
	}

	result := HealthCheckResult{
		OK:     true,
		Checks: make([]HealthCheckItem, 0, 3),
	}

	if err := validateConfig(effective, true); err != nil {
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

	if proxy.IsRunning() {
		result.Checks = append(result.Checks, HealthCheckItem{
			Name:    "本地代理服务",
			OK:      true,
			Message: "代理服务正在运行: " + proxy.Status().ListenAddress,
		})
	} else {
		result.OK = false
		result.Checks = append(result.Checks, HealthCheckItem{
			Name:    "本地代理服务",
			OK:      false,
			Message: "代理服务未启动",
		})
	}

	upstreamErr := proxy.CheckUpstream(effective)
	if upstreamErr != nil {
		result.OK = false
		result.Checks = append(result.Checks, HealthCheckItem{
			Name:    "上游接口",
			OK:      false,
			Message: upstreamErr.Error(),
		})
	} else {
		result.Checks = append(result.Checks, HealthCheckItem{
			Name:    "上游接口",
			OK:      true,
			Message: "上游接口可访问",
		})
	}

	if status, msg, at := proxy.getLastUpstreamFailure(); status == 402 && !at.IsZero() && time.Since(at) < 24*time.Hour {
		result.OK = false
		hint := "检测到最近一次上游请求返回 402（余额不足/额度不足）。请充值或更换 API Key。"
		if strings.TrimSpace(msg) != "" {
			hint += " " + strings.TrimSpace(msg)
		}
		result.Checks = append(result.Checks, HealthCheckItem{
			Name:    "上游余额/额度",
			OK:      false,
			Message: hint,
		})
	}

	return result, nil
}

func (a *App) GetUsageBalance(profileId string) UsageBalance {
	cfg, err := a.GetAppConfig()
	if err != nil {
		return UsageBalance{Error: err.Error()}
	}

	pid := profileId
	if pid == "" {
		pid = cfg.CurrentProfileID
	}
	profile, ok := cfg.Profiles[pid]
	if !ok {
		return UsageBalance{Error: "当前配置不存在"}
	}

	if strings.TrimSpace(profile.APIKey) == "" {
		return UsageBalance{Error: "API Key 未配置"}
	}

	prov := GetProvider(ProviderID(profile.Provider))
	if prov == nil || !prov.HasBalanceAPI || prov.BalanceCheckFn == nil {
		return UsageBalance{Error: "该提供商不支持余额查询"}
	}

	result, err := prov.BalanceCheckFn(profile.APIKey, profile.BaseURL)
	if err != nil {
		a.appendLog("error", "app", "用量查询请求失败: "+err.Error(), "")
		return UsageBalance{Error: err.Error()}
	}
	a.onBalanceUpdate(*result)
	return *result
}

func (a *App) recordUsage(provider, profileName, model, endpoint string, promptTokens, completionTokens, totalTokens int64, success bool, statusCode int, durationMs int64) {
	if a.usageStore == nil {
		return
	}
	record := &UsageRecord{
		ID:               fmt.Sprintf("%d", time.Now().UnixNano()),
		Provider:         provider,
		ProfileName:      profileName,
		Model:            model,
		Endpoint:         endpoint,
		PromptTokens:     promptTokens,
		CompletionTokens: completionTokens,
		TotalTokens:      totalTokens,
		Success:          success,
		StatusCode:       statusCode,
		DurationMs:       durationMs,
		CreatedAt:        time.Now(),
	}
	go a.usageStore.Insert(record)
}

func (a *App) GetUsageStats() (UsageStatsResponse, error) {
	if a.usageStore == nil {
		return UsageStatsResponse{}, errors.New("用量存储未初始化")
	}
	return a.usageStore.QueryStats()
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

func (a *App) emitStatus(source SourceID) {
	if a.ctx != nil {
		ctx := a.ctx
		if proxy, ok := a.proxies[source]; ok {
			payload := proxy.Status()
			go runtime.EventsEmit(ctx, "proxy:status", payload)
		}
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
