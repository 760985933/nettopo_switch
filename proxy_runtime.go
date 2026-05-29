package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

type ProxyRuntime struct {
	app *App

	mu                 sync.RWMutex
	server             *http.Server
	listener           net.Listener
	status             ProxyStatus
	listenAddress      string
	startedAt          time.Time
	lastError          string
	requestCount       int64
	config             AppConfig
	lastReasoning      string
	lastReasonAt       time.Time
	lastUpstreamStatus int
	lastUpstreamError  string
	lastUpstreamAt     time.Time
}

func NewProxyRuntime(app *App) *ProxyRuntime {
	return &ProxyRuntime{
		app:    app,
		status: ProxyStopped,
	}
}

func (b *ProxyRuntime) Start(cfg AppConfig) error {
	b.mu.Lock()

	if b.server != nil {
		b.mu.Unlock()
		return errors.New("代理服务已经在运行")
	}

	addr := net.JoinHostPort(cfg.ListenHost, strconv.Itoa(cfg.ListenPort))
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		b.status = ProxyError
		b.lastError = err.Error()
		b.mu.Unlock()
		b.app.appendLog("error", "proxy", "监听失败: "+err.Error(), "")
		b.app.emitStatus()
		return err
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", b.handleRoot)
	mux.HandleFunc("/health", b.handleHealth)
	mux.HandleFunc("/v1/models", b.handleModels)
	mux.HandleFunc("/v1/chat/completions", b.handleChatCompletions)
	mux.HandleFunc("/v1/responses", b.handleResponses)
	mux.HandleFunc("/v1/messages", b.handleMessages)
	b.server = &http.Server{
		Handler:           b.withAccessLog(mux),
		ReadHeaderTimeout: 10 * time.Second,
	}
	b.listener = listener
	b.status = ProxyRunning
	b.listenAddress = "http://" + listener.Addr().String()
	b.startedAt = time.Now()
	b.lastError = ""
	b.requestCount = 0
	b.config = cfg
	b.lastReasoning = ""
	b.lastReasonAt = time.Time{}
	b.lastUpstreamStatus = 0
	b.lastUpstreamError = ""
	b.lastUpstreamAt = time.Time{}
	server := b.server
	ln := b.listener
	listenAddress := b.listenAddress
	b.mu.Unlock()

	b.app.appendLog("info", "proxy", "代理服务已监听: "+listenAddress, "")
	b.app.emitStatus()

	go func() {
		err := server.Serve(ln)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			b.setStatus(ProxyError, err.Error())
			b.app.appendLog("error", "proxy", "服务异常退出: "+err.Error(), "")
			return
		}
		b.app.appendLog("info", "proxy", "服务已退出", "")
	}()

	return nil
}

func (b *ProxyRuntime) withAccessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		startedAt := time.Now()
		recorder := &statusRecorder{ResponseWriter: w}
		next.ServeHTTP(recorder, r)

		statusCode := recorder.status
		if statusCode == 0 {
			statusCode = http.StatusOK
		}
		duration := time.Since(startedAt).Milliseconds()
		level := statusToLevel(statusCode)
		ua := r.UserAgent()
		if ua == "" {
			ua = "-"
		}
		requestID := recorder.Header().Get("x-proxy-request-id")
		message := fmt.Sprintf("%s %s -> %d (%dms) bytes=%d ua=%s", r.Method, r.URL.Path, statusCode, duration, recorder.size, ua)
		if strings.TrimSpace(requestID) != "" {
			message += " rid=" + requestID
		}
		b.app.appendLog(level, "proxy", message, "")
	})
}

func (b *ProxyRuntime) Stop() error {
	b.mu.Lock()
	server := b.server
	b.server = nil
	listener := b.listener
	b.listener = nil
	b.lastUpstreamStatus = 0
	b.lastUpstreamError = ""
	b.lastUpstreamAt = time.Time{}
	b.mu.Unlock()

	if server == nil {
		b.setStatus(ProxyStopped, "")
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	err := server.Shutdown(ctx)
	if listener != nil {
		_ = listener.Close()
	}
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		b.setStatus(ProxyError, err.Error())
		b.app.appendLog("error", "proxy", "停止服务失败: "+err.Error(), "")
		return err
	}

	b.setStatus(ProxyStopped, "")
	b.app.appendLog("info", "proxy", "代理服务已停止", "")
	return nil
}

func (b *ProxyRuntime) Status() ProxyStatusPayload {
	b.mu.RLock()
	defer b.mu.RUnlock()

	payload := ProxyStatusPayload{
		Status:        b.status,
		ListenAddress: b.listenAddress,
		LastError:     b.lastError,
		RequestCount:  atomic.LoadInt64(&b.requestCount),
	}
	if !b.startedAt.IsZero() {
		payload.StartedAt = b.startedAt.Format(time.RFC3339)
		payload.UptimeSeconds = int64(time.Since(b.startedAt).Seconds())
	}
	return payload
}

func (b *ProxyRuntime) IsRunning() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.status == ProxyRunning && b.server != nil
}

func (b *ProxyRuntime) recordUsage(cfg AppConfig, model, endpoint string, promptTokens, completionTokens, totalTokens int64, statusCode int, durationMs int64, success bool) {
	provider := "unknown"
	profileName := ""
	if profile, ok := cfg.Profiles[cfg.CurrentProfileID]; ok {
		provider = profile.Provider
		profileName = profile.Name
	}
	b.app.recordUsage(provider, profileName, model, endpoint, promptTokens, completionTokens, totalTokens, success, statusCode, durationMs)
}

func (b *ProxyRuntime) CheckUpstream(cfg AppConfig) error {
	baseURL := cfg.DeepseekBaseURL
	if profile, ok := cfg.Profiles[cfg.CurrentProfileID]; ok && strings.TrimSpace(profile.BaseURL) != "" {
		baseURL = profile.BaseURL
	}
	resourceURL, err := upstreamResourceURL(baseURL, "models")
	if err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodGet, resourceURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+cfg.APIKey)

	client := &http.Client{
		Transport: &http.Transport{
			Proxy: nil,
		},
		Timeout: time.Duration(cfg.RequestTimeoutMs) * time.Millisecond,
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusBadRequest {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("上游返回 %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	return nil
}

func (b *ProxyRuntime) handleHealth(w http.ResponseWriter, _ *http.Request) {
	b.writeJSON(w, http.StatusOK, map[string]any{
		"status":         b.Status().Status,
		"listen_address": b.Status().ListenAddress,
		"request_count":  b.Status().RequestCount,
	})
}

func (b *ProxyRuntime) handleRoot(w http.ResponseWriter, _ *http.Request) {
	b.writeJSON(w, http.StatusOK, map[string]any{
		"name":   "Nettopo Codex Proxy",
		"status": b.Status(),
		"endpoints": map[string]string{
			"health":                    "/health",
			"models":                    "/v1/models",
			"chat_completions":          "/v1/chat/completions",
			"responses":                 "/v1/responses",
			"messages":                  "/v1/messages",
			},
		"hint": "将本地地址填入 Codex Desktop 的端点；浏览器访问 /health 可检查服务状态。",
	})
}

func (b *ProxyRuntime) handleModels(w http.ResponseWriter, r *http.Request) {
	cfg := b.snapshotConfig()
	requestID := fmt.Sprintf("req_%d", time.Now().UnixNano())
	w.Header().Set("x-proxy-request-id", requestID)

	seen := map[string]bool{}
	ids := make([]string, 0, 8+len(cfg.Mappings))

	addModel := func(id string) {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			return
		}
		seen[id] = true
		ids = append(ids, id)
	}

	for _, id := range []string{
		"gpt-5.5",
		"gpt-5.4",
		"gpt-5.4-mini",
		"gpt-5.3-codex",
		"gpt-4.1",
		"gpt-4o",
		"gpt-4o-mini",
		"o4-mini",
	} {
		addModel(id)
	}

	addModel(cfg.DefaultModel)
	for from, to := range cfg.Mappings {
		addModel(from)
		addModel(to)
	}

	sort.Strings(ids)
	data := make([]any, 0, len(ids))
	for _, id := range ids {
		data = append(data, map[string]any{
			"id":       id,
			"object":   "model",
			"owned_by": "nettopo-switch",
		})
	}

	b.writeJSON(w, http.StatusOK, map[string]any{
		"object": "list",
		"data":   data,
	})
}

func (b *ProxyRuntime) handleChatCompletions(w http.ResponseWriter, r *http.Request) {
	cfg := b.snapshotConfig()
	requestID := fmt.Sprintf("req_%d", time.Now().UnixNano())
	startedAt := time.Now()
	w.Header().Set("x-proxy-request-id", requestID)

	body, err := io.ReadAll(io.LimitReader(r.Body, 20<<20))
	if err != nil {
		b.app.appendLog("error", "proxy", "chat/completions 读取请求体失败", requestID)
		b.writeProxyError(w, http.StatusBadRequest, "读取请求体失败")
		return
	}

	translatedBody, err := translateChatCompletions(body, cfg)
	if err != nil {
		b.app.appendLog("warn", "proxy", "chat/completions 请求体解析失败: "+err.Error()+" keys="+summarizeJSONKeys(body), requestID)
		b.writeProxyError(w, http.StatusBadRequest, err.Error())
		return
	}
	model := extractModelFromBody(translatedBody)

	resourceURL, err := upstreamResourceURL(cfg.DeepseekBaseURL, "chat/completions")
	if err != nil {
		b.app.appendLog("error", "proxy", "chat/completions 上游地址错误: "+err.Error(), requestID)
		b.writeProxyError(w, http.StatusBadGateway, err.Error())
		return
	}

	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, resourceURL, bytes.NewReader(translatedBody))
	if err != nil {
		b.app.appendLog("error", "proxy", "chat/completions 构造上游请求失败: "+err.Error(), requestID)
		b.writeProxyError(w, http.StatusBadGateway, err.Error())
		return
	}
	req.Header.Set("Content-Type", "application/json")
	copyRequestHeaders(req.Header, r.Header, cfg.Headers)
	req.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(translatedBody)), nil
	}

	streaming := bytes.Contains(translatedBody, []byte(`"stream":true`))

	resp, err := b.doUpstream(req, cfg, streaming)
	if err != nil {
		b.writeProxyError(w, http.StatusBadGateway, err.Error())
		b.app.appendLog("error", "proxy", "转发失败: "+err.Error(), requestID)
		return
	}
	defer resp.Body.Close()

	atomic.AddInt64(&b.requestCount, 1)
	if resp.StatusCode >= http.StatusBadRequest {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
		b.setLastUpstreamFailure(resp.StatusCode, string(raw))
		b.copyHeaders(w.Header(), resp.Header)
		w.WriteHeader(resp.StatusCode)
		_, _ = w.Write(raw)

		duration := time.Since(startedAt).Milliseconds()
		msg := fmt.Sprintf("POST /v1/chat/completions -> %d (%dms)", resp.StatusCode, duration)
		if strings.TrimSpace(string(raw)) != "" {
			msg += " upstream_error=" + truncateForLog(string(raw), 2048)
		}
		b.app.appendLog(statusToLevel(resp.StatusCode), "proxy", msg, requestID)
		return
	}

	b.clearLastUpstreamFailure()
	b.copyHeaders(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)

	if streaming {
		var pt, ct, tt int64
		flusher, ok := w.(http.Flusher)
		if ok {
			scanner := bufio.NewScanner(resp.Body)
			scanner.Buffer(make([]byte, 0, 64*1024), 2*1024*1024)
			for scanner.Scan() {
				line := scanner.Text()
				if strings.TrimSpace(line) == "" {
					_, _ = fmt.Fprintf(w, "\n")
					flusher.Flush()
					continue
				}
				_, _ = fmt.Fprintf(w, "%s\n", line)
				flusher.Flush()

				if strings.HasPrefix(line, "data: ") {
					data := strings.TrimPrefix(line, "data: ")
					if data == "[DONE]" {
						break
					}
					var chunk struct {
						Usage *struct {
							PromptTokens     int64 `json:"prompt_tokens"`
							CompletionTokens int64 `json:"completion_tokens"`
							TotalTokens      int64 `json:"total_tokens"`
						} `json:"usage"`
					}
					if json.Unmarshal([]byte(data), &chunk) == nil && chunk.Usage != nil {
						pt = chunk.Usage.PromptTokens
						ct = chunk.Usage.CompletionTokens
						tt = chunk.Usage.TotalTokens
					}
				}
			}
		} else {
			_, _ = io.Copy(w, resp.Body)
		}
		durationMs := time.Since(startedAt).Milliseconds()
		b.recordUsage(cfg, model, "chat/completions", pt, ct, tt, resp.StatusCode, durationMs, true)
	} else {
		bodyBytes, readErr := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
		if readErr != nil {
			_, _ = io.Copy(w, resp.Body)
		} else {
			_, _ = w.Write(bodyBytes)
			var pt, ct, tt int64
			var usageResp struct {
				Usage *struct {
					PromptTokens     int64 `json:"prompt_tokens"`
					CompletionTokens int64 `json:"completion_tokens"`
					TotalTokens      int64 `json:"total_tokens"`
				} `json:"usage"`
			}
			if json.Unmarshal(bodyBytes, &usageResp) == nil && usageResp.Usage != nil {
				pt = usageResp.Usage.PromptTokens
				ct = usageResp.Usage.CompletionTokens
				tt = usageResp.Usage.TotalTokens
			}
			durationMs := time.Since(startedAt).Milliseconds()
			b.recordUsage(cfg, model, "chat/completions", pt, ct, tt, resp.StatusCode, durationMs, true)
		}
	}

	duration := time.Since(startedAt).Milliseconds()
	b.app.appendLog(
		statusToLevel(resp.StatusCode),
		"proxy",
		fmt.Sprintf("POST /v1/chat/completions -> %d (%dms)", resp.StatusCode, duration),
		requestID,
	)
}

func (b *ProxyRuntime) handleResponses(w http.ResponseWriter, r *http.Request) {
	cfg := b.snapshotConfig()
	requestID := fmt.Sprintf("req_%d", time.Now().UnixNano())
	startedAt := time.Now()
	statusCode := 0
	w.Header().Set("x-proxy-request-id", requestID)

	if r.Method == "GET" && strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
		b.handleResponsesWS(w, r)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 20<<20))
	if err != nil {
		b.app.appendLog("error", "proxy", "responses 读取请求体失败", requestID)
		b.writeProxyError(w, http.StatusBadRequest, "读取请求体失败")
		return
	}

	// DEBUG: 打印原始请求结构和图片相关信息
	b.logResponsesRequestDebug(body, requestID)

	reqSummary := summarizeResponsesRequest(body)
	var upstreamRaw []byte

	chatBody, streaming, model, err := translateResponsesToChatCompletions(body, cfg)
	if err != nil {
		b.app.appendLog("warn", "proxy", "responses 请求体解析失败: "+err.Error()+" keys="+summarizeJSONKeys(body), requestID)
		b.writeProxyError(w, http.StatusBadRequest, err.Error())
		return
	}
	b.logChatBodyDebug(chatBody, model, requestID)
	reasoningInjected := false
	if patched, ok := injectReasoningIntoChatPayload(chatBody, b.getLastReasoning()); ok {
		chatBody = patched
		reasoningInjected = true
	}
	if !streaming && strings.Contains(strings.ToLower(r.Header.Get("Accept")), "text/event-stream") {
		streaming = true
	}
	if streaming && !bytes.Contains(chatBody, []byte(`"stream":true`)) {
		var patched map[string]any
		if unmarshalErr := json.Unmarshal(chatBody, &patched); unmarshalErr == nil {
			patched["stream"] = true
			if out, marshalErr := json.Marshal(patched); marshalErr == nil {
				chatBody = out
			}
		}
	}

	resourceURL, err := upstreamResourceURL(cfg.DeepseekBaseURL, "chat/completions")
	if err != nil {
		b.app.appendLog("error", "proxy", "responses 上游地址错误: "+err.Error(), requestID)
		b.writeProxyError(w, http.StatusBadGateway, err.Error())
		return
	}

	if streaming {
		flusher, ok := w.(http.Flusher)
		if !ok {
			b.writeProxyError(w, http.StatusBadGateway, "客户端不支持流式输出")
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.WriteHeader(http.StatusOK)
		flusher.Flush()

		upstreamCtx := r.Context()

		req, err := http.NewRequestWithContext(upstreamCtx, http.MethodPost, resourceURL, bytes.NewReader(chatBody))
		if err != nil {
			b.streamResponsesFailed(w, "bad_gateway", err.Error())
			b.app.appendLog("error", "proxy", "responses 构造上游请求失败: "+err.Error(), requestID)
			b.app.appendLog("error", "proxy", "转发失败: "+err.Error(), requestID)
			return
		}
		req.Header.Set("Content-Type", "application/json")
		copyRequestHeaders(req.Header, r.Header, cfg.Headers)
		req.GetBody = func() (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(chatBody)), nil
		}

		resp, err := b.doUpstream(req, cfg, true)
		if err != nil {
			b.streamResponsesFailed(w, "bad_gateway", err.Error())
			b.app.appendLog("error", "proxy", "转发失败: "+err.Error(), requestID)
			return
		}
		defer resp.Body.Close()

		atomic.AddInt64(&b.requestCount, 1)

		if resp.StatusCode >= http.StatusBadRequest {
			raw, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
			msg := strings.TrimSpace(string(raw))
			if msg == "" {
				msg = fmt.Sprintf("上游返回 %d", resp.StatusCode)
			} else {
				msg = fmt.Sprintf("上游返回 %d: %s", resp.StatusCode, msg)
			}
			b.setLastUpstreamFailure(resp.StatusCode, msg)
			b.recordUsage(cfg, model, "responses", 0, 0, 0, resp.StatusCode, time.Since(startedAt).Milliseconds(), false)
			b.streamResponsesFailed(w, "bad_gateway", msg)
			message := fmt.Sprintf("POST /v1/responses (stream) -> %d (%dms)", resp.StatusCode, time.Since(startedAt).Milliseconds())
			if reqSummary != "" {
				message += " " + reqSummary
			}
			if reasoningInjected {
				message += " reasoning_injected=true"
			}
			message += " upstream_error=" + truncateForLog(msg, 2048)
			if debugPayloadEnabled() {
				message += " req_json=" + truncateForLog(string(chatBody), 4096)
			}
			b.app.appendLog("error", "proxy", message, requestID)
			return
		}

		b.clearLastUpstreamFailure()
		textLen, toolNames, pt, ct, tt := b.streamChatToResponses(w, resp.Body, model)
		durationMs := time.Since(startedAt).Milliseconds()
		b.recordUsage(cfg, model, "responses", pt, ct, tt, 200, durationMs, true)
		message := fmt.Sprintf("POST /v1/responses (stream) -> 200 (%dms)", durationMs)
		if reqSummary != "" {
			message += " " + reqSummary
		}
		if reasoningInjected {
			message += " reasoning_injected=true"
		}
		message += fmt.Sprintf(" out_text_len=%d tool_calls=%d", textLen, len(toolNames))
		if len(toolNames) > 0 {
			message += " tool_names=" + strings.Join(toolNames, ",")
		}
		if debugPayloadEnabled() {
			message += " req_json=" + truncateForLog(string(chatBody), 4096)
		}
		b.app.appendLog("info", "proxy", message, requestID)
		return
	} else {
		req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, resourceURL, bytes.NewReader(chatBody))
		if err != nil {
			b.app.appendLog("error", "proxy", "responses 构造上游请求失败: "+err.Error(), requestID)
			b.writeProxyError(w, http.StatusBadGateway, err.Error())
			return
		}
		req.Header.Set("Content-Type", "application/json")
		copyRequestHeaders(req.Header, r.Header, cfg.Headers)
		req.GetBody = func() (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(chatBody)), nil
		}

		resp, err := b.doUpstream(req, cfg, false)
		if err != nil {
			b.writeProxyError(w, http.StatusBadGateway, err.Error())
			b.app.appendLog("error", "proxy", "转发失败: "+err.Error(), requestID)
			return
		}
		defer resp.Body.Close()

		atomic.AddInt64(&b.requestCount, 1)

		upstreamRaw, _ = io.ReadAll(io.LimitReader(resp.Body, 10<<20))
		statusCode = resp.StatusCode
		if resp.StatusCode >= http.StatusBadRequest {
			b.setLastUpstreamFailure(resp.StatusCode, string(upstreamRaw))
			b.copyHeaders(w.Header(), resp.Header)
			w.WriteHeader(resp.StatusCode)
			_, _ = w.Write(upstreamRaw)
			b.recordUsage(cfg, model, "responses", 0, 0, 0, resp.StatusCode, time.Since(startedAt).Milliseconds(), false)
		} else {
			b.clearLastUpstreamFailure()
			if reasoning := strings.TrimSpace(extractChatCompletionReasoningFromBody(upstreamRaw)); reasoning != "" {
				b.setLastReasoning(reasoning)
			}
			var pt, ct, tt int64
			var usageData struct {
				Usage *struct {
					PromptTokens     int64 `json:"prompt_tokens"`
					CompletionTokens int64 `json:"completion_tokens"`
					TotalTokens      int64 `json:"total_tokens"`
				} `json:"usage"`
			}
			if json.Unmarshal(upstreamRaw, &usageData) == nil && usageData.Usage != nil {
				pt = usageData.Usage.PromptTokens
				ct = usageData.Usage.CompletionTokens
				tt = usageData.Usage.TotalTokens
			}
			response, err := translateChatCompletionToResponses(upstreamRaw, model)
			if err != nil {
				b.app.appendLog("error", "proxy", "responses 响应转换失败: "+err.Error(), requestID)
				b.writeProxyError(w, http.StatusBadGateway, err.Error())
				b.recordUsage(cfg, model, "responses", pt, ct, tt, resp.StatusCode, time.Since(startedAt).Milliseconds(), false)
			} else {
				b.writeJSON(w, resp.StatusCode, response)
				b.recordUsage(cfg, model, "responses", pt, ct, tt, resp.StatusCode, time.Since(startedAt).Milliseconds(), true)
			}
		}
	}

	duration := time.Since(startedAt).Milliseconds()
	message := fmt.Sprintf("POST /v1/responses -> %d (%dms)", statusCode, duration)
	if reqSummary != "" {
		message += " " + reqSummary
	}
	if reasoningInjected {
		message += " reasoning_injected=true"
	}
	if statusCode >= http.StatusBadRequest && len(upstreamRaw) > 0 {
		message += " upstream_error=" + truncateForLog(string(upstreamRaw), 2048)
	}
	if debugPayloadEnabled() {
		message += " req_json=" + truncateForLog(string(chatBody), 4096)
		if len(upstreamRaw) > 0 {
			message += " resp_json=" + truncateForLog(string(upstreamRaw), 4096)
		}
	}
	b.app.appendLog(statusToLevel(statusCode), "proxy", message, requestID)
}

func (b *ProxyRuntime) handleResponsesWS(w http.ResponseWriter, r *http.Request) {
	cfg := b.snapshotConfig()
	requestID := fmt.Sprintf("req_%d", time.Now().UnixNano())
	startedAt := time.Now()
	w.Header().Set("x-proxy-request-id", requestID)

	upgrader := websocket.Upgrader{
		ReadBufferSize:  65536,
		WriteBufferSize: 65536,
		CheckOrigin:     func(r *http.Request) bool { return true },
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		b.app.appendLog("error", "proxy", "responses WS 升级失败: "+err.Error(), requestID)
		return
	}
	defer conn.Close()

	conn.SetPongHandler(func(string) error {
		_ = conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})
	_ = conn.SetReadDeadline(time.Now().Add(60 * time.Second))

	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}()

	_, body, err := conn.ReadMessage()
	if err != nil {
		b.app.appendLog("warn", "proxy", "responses WS 读取消息失败: "+err.Error(), requestID)
		return
	}

	// DEBUG: 打印原始请求结构和图片相关信息
	b.logResponsesRequestDebug(body, requestID)

	chatBody, _, model, err := translateResponsesToChatCompletions(body, cfg)
	if err != nil {
		_ = conn.WriteJSON(map[string]any{
			"type": "response.failed",
			"error": map[string]any{
				"type":    "invalid_request",
				"message": err.Error(),
			},
		})
		b.app.appendLog("warn", "proxy", "responses WS 请求转换失败: "+err.Error(), requestID)
		return
	}

	b.logChatBodyDebug(chatBody, model, requestID)

	if !bytes.Contains(chatBody, []byte(`"stream":true`)) {
		var patched map[string]any
		if unmarshalErr := json.Unmarshal(chatBody, &patched); unmarshalErr == nil {
			patched["stream"] = true
			if out, marshalErr := json.Marshal(patched); marshalErr == nil {
				chatBody = out
			}
		}
	}

	resourceURL, err := upstreamResourceURL(cfg.DeepseekBaseURL, "chat/completions")
	if err != nil {
		_ = conn.WriteJSON(map[string]any{
			"type": "response.failed",
			"error": map[string]any{
				"type":    "bad_gateway",
				"message": err.Error(),
			},
		})
		b.app.appendLog("error", "proxy", "responses WS 上游地址错误: "+err.Error(), requestID)
		return
	}

	upstreamReq, err := http.NewRequestWithContext(r.Context(), http.MethodPost, resourceURL, bytes.NewReader(chatBody))
	if err != nil {
		_ = conn.WriteJSON(map[string]any{
			"type": "response.failed",
			"error": map[string]any{
				"type":    "bad_gateway",
				"message": err.Error(),
			},
		})
		b.app.appendLog("error", "proxy", "responses WS 构造上游请求失败: "+err.Error(), requestID)
		return
	}
	upstreamReq.Header.Set("Content-Type", "application/json")
	upstreamReq.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(chatBody)), nil
	}

	resp, err := b.doUpstream(upstreamReq, cfg, true)
	if err != nil {
		_ = conn.WriteJSON(map[string]any{
			"type": "response.failed",
			"error": map[string]any{
				"type":    "bad_gateway",
				"message": err.Error(),
			},
		})
		b.app.appendLog("error", "proxy", "responses WS 转发失败: "+err.Error(), requestID)
		return
	}
	defer resp.Body.Close()

	atomic.AddInt64(&b.requestCount, 1)

	if resp.StatusCode >= http.StatusBadRequest {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
		msg := strings.TrimSpace(string(raw))
		if msg == "" {
			msg = fmt.Sprintf("上游返回 %d", resp.StatusCode)
		} else {
			msg = fmt.Sprintf("上游返回 %d: %s", resp.StatusCode, msg)
		}
		b.setLastUpstreamFailure(resp.StatusCode, msg)
		_ = conn.WriteJSON(map[string]any{
			"type": "response.failed",
			"error": map[string]any{
				"type":    "bad_gateway",
				"message": msg,
			},
		})
		b.app.appendLog("error", "proxy", "responses WS 上游错误: "+msg, requestID)
		return
	}

	b.clearLastUpstreamFailure()
	textLen, toolNames, pt, ct, tt := b.streamChatToResponsesWS(conn, resp.Body, model)
	durationMs := time.Since(startedAt).Milliseconds()
	b.recordUsage(cfg, model, "responses", pt, ct, tt, 200, durationMs, true)
	duration := durationMs
	message := fmt.Sprintf("WS /v1/responses -> 200 (%dms)", duration)
	reqSummary := summarizeResponsesRequest(body)
	if reqSummary != "" {
		message += " " + reqSummary
	}
	message += fmt.Sprintf(" out_text_len=%d tool_calls=%d", textLen, len(toolNames))
	if len(toolNames) > 0 {
		message += " tool_names=" + strings.Join(toolNames, ",")
	}
	b.app.appendLog("info", "proxy", message, requestID)
}

func (b *ProxyRuntime) setLastReasoning(value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	b.mu.Lock()
	b.lastReasoning = value
	b.lastReasonAt = time.Now()
	b.mu.Unlock()
}

func (b *ProxyRuntime) getLastReasoning() string {
	b.mu.RLock()
	value := b.lastReasoning
	at := b.lastReasonAt
	b.mu.RUnlock()
	if strings.TrimSpace(value) == "" {
		return ""
	}
	if at.IsZero() {
		return value
	}
	if time.Since(at) > 30*time.Minute {
		return ""
	}
	return value
}

func (b *ProxyRuntime) setLastUpstreamFailure(status int, message string) {
	b.mu.Lock()
	b.lastUpstreamStatus = status
	b.lastUpstreamError = strings.TrimSpace(message)
	b.lastUpstreamAt = time.Now()
	b.mu.Unlock()
}

func (b *ProxyRuntime) getLastUpstreamFailure() (int, string, time.Time) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.lastUpstreamStatus, b.lastUpstreamError, b.lastUpstreamAt
}

func (b *ProxyRuntime) clearLastUpstreamFailure() {
	b.mu.Lock()
	b.lastUpstreamStatus = 0
	b.lastUpstreamError = ""
	b.lastUpstreamAt = time.Time{}
	b.mu.Unlock()
}

func (b *ProxyRuntime) handleMessages(w http.ResponseWriter, r *http.Request) {
	cfg := b.snapshotConfig()
	requestID := fmt.Sprintf("req_%d", time.Now().UnixNano())
	startedAt := time.Now()
	w.Header().Set("x-proxy-request-id", requestID)

	body, err := io.ReadAll(io.LimitReader(r.Body, 20<<20))
	if err != nil {
		b.app.appendLog("error", "proxy", "messages 读取请求体失败", requestID)
		b.writeProxyError(w, http.StatusBadRequest, "读取请求体失败")
		return
	}

	sourceFormat := APIMessages
	targetFormat := ResolveProviderFormat(cfg)

	// 翻译请求体
	upstreamBody, streaming, model, err := TranslateRequestBody(body, sourceFormat, targetFormat, cfg)
	if err != nil {
		b.app.appendLog("warn", "proxy", "messages 请求体解析失败: "+err.Error()+" keys="+summarizeJSONKeys(body), requestID)
		b.writeProxyError(w, http.StatusBadRequest, err.Error())
		return
	}

	if streaming {
		flusher, ok := w.(http.Flusher)
		if !ok {
			b.writeProxyError(w, http.StatusBadGateway, "客户端不支持流式输出")
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.WriteHeader(http.StatusOK)
		flusher.Flush()
	}

	// 构建格式感知的上游请求
	req, err := BuildUpstreamRequest(r, upstreamBody, targetFormat, cfg)
	if err != nil {
		b.app.appendLog("error", "proxy", "messages 构造上游请求失败: "+err.Error(), requestID)
		if streaming {
			b.streamMessagesError(w, "bad_gateway", err.Error())
		} else {
			b.writeProxyError(w, http.StatusBadGateway, err.Error())
		}
		return
	}

	// 同格式直通时，流式也需要 Accept 头
	if sourceFormat == targetFormat && streaming {
		req.Header.Set("Accept", "text/event-stream")
	}

	resp, err := b.doUpstream(req, cfg, streaming)
	if err != nil {
		b.app.appendLog("error", "proxy", "转发失败: "+err.Error(), requestID)
		if streaming {
			b.streamMessagesError(w, "bad_gateway", err.Error())
		} else {
			b.writeProxyError(w, http.StatusBadGateway, err.Error())
		}
		return
	}
	defer resp.Body.Close()

	atomic.AddInt64(&b.requestCount, 1)

	if resp.StatusCode >= http.StatusBadRequest {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
		msg := strings.TrimSpace(string(raw))
		if msg == "" {
			msg = fmt.Sprintf("上游返回 %d", resp.StatusCode)
		} else {
			msg = fmt.Sprintf("上游返回 %d: %s", resp.StatusCode, msg)
		}
		b.setLastUpstreamFailure(resp.StatusCode, msg)
		if streaming {
			b.streamMessagesError(w, "bad_gateway", msg)
		} else {
			b.writeProxyError(w, http.StatusBadGateway, msg)
		}
		b.app.appendLog("error", "proxy", fmt.Sprintf("POST /v1/messages -> %d (%dms) upstream_error=%s", resp.StatusCode, time.Since(startedAt).Milliseconds(), truncateForLog(msg, 2048)), requestID)
		return
	}

	b.clearLastUpstreamFailure()

	if streaming {
		if sourceFormat == targetFormat {
			// 直通：Messages SSE → Messages SSE
			b.streamPassthrough(w, resp.Body)
		} else {
			b.streamChatToMessages(w, resp.Body, model)
		}
		durationMs := time.Since(startedAt).Milliseconds()
		b.recordUsage(cfg, model, "messages", 0, 0, 0, 200, durationMs, true)
	} else {
		upstreamRaw, _ := io.ReadAll(io.LimitReader(resp.Body, 10<<20))

		if sourceFormat == targetFormat {
			// 直通：无需反向翻译
			b.copyHeaders(w.Header(), resp.Header)
			w.WriteHeader(resp.StatusCode)
			w.Write(upstreamRaw)
			b.recordUsage(cfg, model, "messages", 0, 0, 0, resp.StatusCode, time.Since(startedAt).Milliseconds(), true)
		} else {
			var pt, ct, tt int64
			if usageMap, ok := extractUsage(upstreamRaw); ok {
				pt = usageMap["prompt_tokens"]
				ct = usageMap["completion_tokens"]
				tt = usageMap["total_tokens"]
			}
			b.recordUsage(cfg, model, "messages", pt, ct, tt, resp.StatusCode, time.Since(startedAt).Milliseconds(), resp.StatusCode < http.StatusBadRequest)
			response, err := translateChatCompletionToMessages(upstreamRaw, model)
			if err != nil {
				b.app.appendLog("error", "proxy", "messages 响应转换失败: "+err.Error(), requestID)
				b.writeProxyError(w, http.StatusInternalServerError, "响应转换失败")
				return
			}
			b.writeJSON(w, http.StatusOK, response)
		}
	}

	duration := time.Since(startedAt).Milliseconds()
	b.app.appendLog("info", "proxy", fmt.Sprintf("POST /v1/messages -> 200 (%dms) format=%s→%s", duration, sourceFormat, targetFormat), requestID)
}

func (b *ProxyRuntime) doUpstream(req *http.Request, cfg AppConfig, streaming bool) (*http.Response, error) {
	// 仅在没有预设认证头时才设置默认 Bearer 认证
	if req.Header.Get("Authorization") == "" && req.Header.Get("x-api-key") == "" {
		req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	}
	if req.Header.Get("Accept") == "" && streaming {
		req.Header.Set("Accept", "text/event-stream")
	}
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", "nettopo-switch/0.1")
	}

	client := &http.Client{
		Transport: func() *http.Transport {
			tr := http.DefaultTransport.(*http.Transport).Clone()
			tr.Proxy = nil
			return tr
		}(),
		Timeout: time.Duration(cfg.RequestTimeoutMs) * time.Millisecond,
	}
	if streaming {
		client.Timeout = 0
	}

	var lastErr error
	attempts := cfg.MaxRetries + 1
	for attempt := 1; attempt <= attempts; attempt++ {
		cloned := req.Clone(req.Context())
		if req.GetBody != nil {
			body, err := req.GetBody()
			if err != nil {
				lastErr = err
				continue
			}
			cloned.Body = body
		}

		resp, err := client.Do(cloned)
		if err != nil {
			lastErr = err
		} else if resp.StatusCode >= http.StatusInternalServerError && attempt < attempts {
			io.Copy(io.Discard, io.LimitReader(resp.Body, 2048))
			_ = resp.Body.Close()
			lastErr = fmt.Errorf("上游返回 %d", resp.StatusCode)
		} else {
			return resp, nil
		}

		if attempt < attempts {
			time.Sleep(time.Duration(150*attempt) * time.Millisecond)
		}
	}

	return nil, lastErr
}

func (b *ProxyRuntime) setStatus(status ProxyStatus, lastError string) {
	b.mu.Lock()
	b.status = status
	b.lastError = lastError
	if status == ProxyStopped {
		b.listenAddress = ""
		b.startedAt = time.Time{}
		atomic.StoreInt64(&b.requestCount, 0)
		b.lastReasoning = ""
		b.lastReasonAt = time.Time{}
	}
	b.mu.Unlock()

	b.app.emitStatus()
}

func (b *ProxyRuntime) snapshotConfig() AppConfig {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.config
}

// DEBUG: 打印 Responses API 请求中的图片相关内容
func (b *ProxyRuntime) logResponsesRequestDebug(body []byte, requestID string) {
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		b.app.appendLog("info", "proxy", "[DEBUG] 请求体 JSON 解析失败: "+err.Error(), requestID)
		return
	}

	// 打印顶层 key
	keys := make([]string, 0, len(payload))
	for k := range payload {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	inputAny, _ := payload["input"]
	inputItems, _ := inputAny.([]any)

	// 扫描 input 中的所有类型
	type imageInfo struct {
		kind      string // "top_level_input_image" or "in_message_content"
		urlPrefix string // 前200字符
		urlLen    int
	}
	var images []imageInfo
	inputTypeCounts := map[string]int{}

	for i, item := range inputItems {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		t, _ := m["type"].(string)
		inputTypeCounts[t]++

		switch t {
		case "input_image":
			urlStr := extractImageURL(m["image_url"])
			prefix := urlStr
			if len(prefix) > 200 {
				prefix = prefix[:200] + "..."
			}
			images = append(images, imageInfo{
				kind:      "top_level_input_image[" + strconv.Itoa(i) + "]",
				urlPrefix: prefix,
				urlLen:    len(urlStr),
			})
		case "message":
			content, _ := m["content"].([]any)
			for ci, cp := range content {
				cm, ok := cp.(map[string]any)
				if !ok {
					continue
				}
				ct, _ := cm["type"].(string)
				if ct == "input_image" {
					urlStr := extractImageURL(cm["image_url"])
					prefix := urlStr
					if len(prefix) > 200 {
						prefix = prefix[:200] + "..."
					}
					images = append(images, imageInfo{
						kind:      "msg[" + strconv.Itoa(i) + "].content[" + strconv.Itoa(ci) + "]",
						urlPrefix: prefix,
						urlLen:    len(urlStr),
					})
				}
			}
		}
	}

	logParts := []string{fmt.Sprintf("[DEBUG] body_keys=%s", strings.Join(keys, ","))}
	logParts = append(logParts, fmt.Sprintf("input_types=%s", summarizeResponsesInput(inputItems)))
	if len(images) > 0 {
		for _, img := range images {
			logParts = append(logParts, fmt.Sprintf("IMAGE found at %s url_len=%d url_prefix=%s", img.kind, img.urlLen, img.urlPrefix))
		}
	} else {
		logParts = append(logParts, "NO_IMAGE_FOUND")
	}
	b.app.appendLog("info", "proxy", strings.Join(logParts, " | "), requestID)
}

// DEBUG: Chat Completions 翻译结果中图片相关部分
func (b *ProxyRuntime) logChatBodyDebug(chatBody []byte, model, requestID string) {
	var payload map[string]any
	if err := json.Unmarshal(chatBody, &payload); err != nil {
		b.app.appendLog("info", "proxy", "[DEBUG-CHAT] JSON parse err: "+err.Error(), requestID)
		return
	}
	messages, _ := payload["messages"].([]any)
	imageCount := 0
	var imageDetails []string
	for mi, item := range messages {
		msg, ok := item.(map[string]any)
		if !ok {
			continue
		}
		content := msg["content"]
		contentArr, ok := content.([]any)
		if !ok {
			continue
		}
		for ci, cp := range contentArr {
			cm, ok := cp.(map[string]any)
			if !ok {
				continue
			}
			ct, _ := cm["type"].(string)
			if ct == "image_url" {
				imageCount++
				if iu, ok := cm["image_url"].(map[string]any); ok {
					urlStr, _ := iu["url"].(string)
					prefix := urlStr
					if len(prefix) > 150 {
						prefix = prefix[:150] + "..."
					}
					imageDetails = append(imageDetails, fmt.Sprintf("msg[%d].content[%d] url_len=%d prefix=%s", mi, ci, len(urlStr), prefix))
				}
			}
		}
	}
	stream, _ := payload["stream"]
	modelVal, _ := payload["model"].(string)
	toolsN := 0
	if tools, ok := payload["tools"].([]any); ok {
		toolsN = len(tools)
	}
	msg := fmt.Sprintf("[DEBUG-CHAT] model=%s stream=%v messages=%d tools=%d image_count=%d", modelVal, stream, len(messages), toolsN, imageCount)
	if len(imageDetails) > 0 {
		for _, d := range imageDetails {
			msg += " | IMAGE: " + d
		}
	}
	b.app.appendLog("info", "proxy", msg, requestID)
}

func (b *ProxyRuntime) writeJSON(w http.ResponseWriter, statusCode int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(payload)
}

func (b *ProxyRuntime) writeProxyError(w http.ResponseWriter, statusCode int, message string) {
	code := "proxy_error"
	if statusCode == http.StatusBadGateway {
		code = "bad_gateway"
	}
	b.writeJSON(w, statusCode, map[string]any{
		"error": map[string]any{
			"message": message,
			"type":    "proxy_error",
			"code":    code,
		},
	})
}

func (b *ProxyRuntime) copyHeaders(dst http.Header, src http.Header) {
	for key, values := range src {
		if strings.EqualFold(key, "Content-Length") {
			continue
		}
		for _, value := range values {
			dst.Add(key, value)
		}
	}
}

