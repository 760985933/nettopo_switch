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
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type BridgeRuntime struct {
	app *App

	mu            sync.RWMutex
	server        *http.Server
	listener      net.Listener
	status        BridgeStatus
	listenAddress string
	startedAt     time.Time
	lastError     string
	requestCount  int64
	config        AppConfig
	lastReasoning string
	lastReasonAt  time.Time
}

func NewBridgeRuntime(app *App) *BridgeRuntime {
	return &BridgeRuntime{
		app:    app,
		status: BridgeStopped,
	}
}

func (b *BridgeRuntime) Start(cfg AppConfig) error {
	b.mu.Lock()

	if b.server != nil {
		b.mu.Unlock()
		return errors.New("桥接服务已经在运行")
	}

	addr := net.JoinHostPort(cfg.ListenHost, strconv.Itoa(cfg.ListenPort))
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		b.status = BridgeError
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

	b.server = &http.Server{
		Handler:           b.withAccessLog(mux),
		ReadHeaderTimeout: 10 * time.Second,
	}
	b.listener = listener
	b.status = BridgeRunning
	b.listenAddress = "http://" + listener.Addr().String()
	b.startedAt = time.Now()
	b.lastError = ""
	b.requestCount = 0
	b.config = cfg
	b.lastReasoning = ""
	b.lastReasonAt = time.Time{}
	server := b.server
	ln := b.listener
	listenAddress := b.listenAddress
	b.mu.Unlock()

	b.app.appendLog("info", "proxy", "桥接服务已监听: "+listenAddress, "")
	b.app.emitStatus()

	go func() {
		err := server.Serve(ln)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			b.setStatus(BridgeError, err.Error())
			b.app.appendLog("error", "proxy", "服务异常退出: "+err.Error(), "")
			return
		}
		b.app.appendLog("info", "proxy", "服务已退出", "")
	}()

	return nil
}

type statusRecorder struct {
	http.ResponseWriter
	status int
	size   int64
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Write(p []byte) (int, error) {
	if s.status == 0 {
		s.status = http.StatusOK
	}
	n, err := s.ResponseWriter.Write(p)
	s.size += int64(n)
	return n, err
}

func (s *statusRecorder) Flush() {
	if flusher, ok := s.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (b *BridgeRuntime) withAccessLog(next http.Handler) http.Handler {
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
		requestID := recorder.Header().Get("x-bridge-request-id")
		message := fmt.Sprintf("%s %s -> %d (%dms) bytes=%d ua=%s", r.Method, r.URL.Path, statusCode, duration, recorder.size, ua)
		if strings.TrimSpace(requestID) != "" {
			message += " rid=" + requestID
		}
		b.app.appendLog(level, "proxy", message, "")
	})
}

func (b *BridgeRuntime) Stop() error {
	b.mu.Lock()
	server := b.server
	b.server = nil
	listener := b.listener
	b.listener = nil
	b.mu.Unlock()

	if server == nil {
		b.setStatus(BridgeStopped, "")
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	err := server.Shutdown(ctx)
	if listener != nil {
		_ = listener.Close()
	}
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		b.setStatus(BridgeError, err.Error())
		b.app.appendLog("error", "proxy", "停止服务失败: "+err.Error(), "")
		return err
	}

	b.setStatus(BridgeStopped, "")
	b.app.appendLog("info", "proxy", "桥接服务已停止", "")
	return nil
}

func (b *BridgeRuntime) Status() BridgeStatusPayload {
	b.mu.RLock()
	defer b.mu.RUnlock()

	payload := BridgeStatusPayload{
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

func (b *BridgeRuntime) IsRunning() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.status == BridgeRunning && b.server != nil
}

func (b *BridgeRuntime) CheckUpstream(cfg AppConfig) error {
	resourceURL, err := upstreamResourceURL(cfg.DeepseekBaseURL, "models")
	if err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodGet, resourceURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+cfg.APIKey)

	client := &http.Client{
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

func (b *BridgeRuntime) handleHealth(w http.ResponseWriter, _ *http.Request) {
	b.writeJSON(w, http.StatusOK, map[string]any{
		"status":         b.Status().Status,
		"listen_address": b.Status().ListenAddress,
		"request_count":  b.Status().RequestCount,
	})
}

func (b *BridgeRuntime) handleRoot(w http.ResponseWriter, _ *http.Request) {
	b.writeJSON(w, http.StatusOK, map[string]any{
		"name":   "Nettopo Codex Bridge",
		"status": b.Status(),
		"endpoints": map[string]string{
			"health":                    "/health",
			"models":                    "/v1/models",
			"chat_completions":          "/v1/chat/completions",
			"responses":                 "/v1/responses",
			"chat_completions_upstream": "DeepSeek /v1/chat/completions",
		},
		"hint": "将本地地址填入 Codex Desktop 的端点；浏览器访问 /health 可检查服务状态。",
	})
}

func (b *BridgeRuntime) handleModels(w http.ResponseWriter, r *http.Request) {
	cfg := b.snapshotConfig()
	requestID := fmt.Sprintf("req_%d", time.Now().UnixNano())
	w.Header().Set("x-bridge-request-id", requestID)

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
		"deepseek-v4-pro",
		"deepseek-v4-flash",
		"deepseek-chat",
		"deepseek-reasoner",
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

func (b *BridgeRuntime) handleChatCompletions(w http.ResponseWriter, r *http.Request) {
	cfg := b.snapshotConfig()
	requestID := fmt.Sprintf("req_%d", time.Now().UnixNano())
	startedAt := time.Now()
	w.Header().Set("x-bridge-request-id", requestID)

	body, err := io.ReadAll(io.LimitReader(r.Body, 4<<20))
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
	b.copyHeaders(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)

	if streaming {
		b.streamResponse(w, resp.Body)
	} else {
		_, _ = io.Copy(w, resp.Body)
	}

	duration := time.Since(startedAt).Milliseconds()
	b.app.appendLog(
		statusToLevel(resp.StatusCode),
		"proxy",
		fmt.Sprintf("POST /v1/chat/completions -> %d (%dms)", resp.StatusCode, duration),
		requestID,
	)
}

func (b *BridgeRuntime) handleResponses(w http.ResponseWriter, r *http.Request) {
	cfg := b.snapshotConfig()
	requestID := fmt.Sprintf("req_%d", time.Now().UnixNano())
	startedAt := time.Now()
	statusCode := 0
	w.Header().Set("x-bridge-request-id", requestID)

	body, err := io.ReadAll(io.LimitReader(r.Body, 4<<20))
	if err != nil {
		b.app.appendLog("error", "proxy", "responses 读取请求体失败", requestID)
		b.writeProxyError(w, http.StatusBadRequest, "读取请求体失败")
		return
	}

	reqSummary := summarizeResponsesRequest(body)
	var upstreamRaw []byte

	chatBody, streaming, model, err := translateResponsesToChatCompletions(body, cfg)
	if err != nil {
		b.app.appendLog("warn", "proxy", "responses 请求体解析失败: "+err.Error()+" keys="+summarizeJSONKeys(body), requestID)
		b.writeProxyError(w, http.StatusBadRequest, err.Error())
		return
	}
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

		textLen, toolNames := b.streamChatToResponses(w, resp.Body, model)
		duration := time.Since(startedAt).Milliseconds()
		message := fmt.Sprintf("POST /v1/responses (stream) -> 200 (%dms)", duration)
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
			b.copyHeaders(w.Header(), resp.Header)
			w.WriteHeader(resp.StatusCode)
			_, _ = w.Write(upstreamRaw)
		} else {
			if reasoning := strings.TrimSpace(extractChatCompletionReasoningFromBody(upstreamRaw)); reasoning != "" {
				b.setLastReasoning(reasoning)
			}
			response, err := translateChatCompletionToResponses(upstreamRaw, model)
			if err != nil {
				b.app.appendLog("error", "proxy", "responses 响应转换失败: "+err.Error(), requestID)
				b.writeProxyError(w, http.StatusBadGateway, err.Error())
			} else {
				b.writeJSON(w, resp.StatusCode, response)
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

func summarizeJSONKeys(body []byte) string {
	var payload any
	if err := json.Unmarshal(body, &payload); err != nil {
		return "invalid_json"
	}
	root, ok := payload.(map[string]any)
	if !ok {
		return "non_object"
	}
	keys := make([]string, 0, len(root))
	for k := range root {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	if len(keys) > 24 {
		keys = keys[:24]
	}
	return strings.Join(keys, ",")
}

func (b *BridgeRuntime) setLastReasoning(value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	b.mu.Lock()
	b.lastReasoning = value
	b.lastReasonAt = time.Now()
	b.mu.Unlock()
}

func (b *BridgeRuntime) getLastReasoning() string {
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

func injectReasoningIntoChatPayload(body []byte, reasoning string) ([]byte, bool) {
	reasoning = strings.TrimSpace(reasoning)
	if reasoning == "" {
		return body, false
	}

	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return body, false
	}
	messages, ok := payload["messages"].([]any)
	if !ok {
		return body, false
	}

	hasReasoning := func(msg map[string]any) bool {
		if rc, ok := msg["reasoning_content"].(string); ok && strings.TrimSpace(rc) != "" {
			return true
		}
		if rc, ok := msg["reasoning"].(string); ok && strings.TrimSpace(rc) != "" {
			return true
		}
		return false
	}

	// Prefer patching the last assistant message with tool_calls. DeepSeek thinking+tools requires
	// reasoning_content to be passed back for the tool-call loop, and it must be associated with
	// the assistant tool_calls message.
	lastToolCallAssistant := -1
	for i := len(messages) - 1; i >= 0; i-- {
		msg, ok := messages[i].(map[string]any)
		if !ok {
			continue
		}
		role, _ := msg["role"].(string)
		if !strings.EqualFold(strings.TrimSpace(role), "assistant") {
			continue
		}
		if tc, ok := msg["tool_calls"].([]any); ok && len(tc) > 0 {
			lastToolCallAssistant = i
			break
		}
	}
	if lastToolCallAssistant >= 0 {
		if msg, ok := messages[lastToolCallAssistant].(map[string]any); ok && !hasReasoning(msg) {
			msg["reasoning_content"] = reasoning
			payload["messages"] = messages
			if out, err := json.Marshal(payload); err == nil {
				return out, true
			}
		}
		return body, false
	}

	// Otherwise patch the last assistant message if present.
	for i := len(messages) - 1; i >= 0; i-- {
		msg, ok := messages[i].(map[string]any)
		if !ok {
			continue
		}
		role, _ := msg["role"].(string)
		if !strings.EqualFold(strings.TrimSpace(role), "assistant") {
			continue
		}
		if hasReasoning(msg) {
			return body, false
		}
		msg["reasoning_content"] = reasoning
		payload["messages"] = messages
		if out, err := json.Marshal(payload); err == nil {
			return out, true
		}
		return body, false
	}

	// Fallback: insert a synthetic assistant message before the first tool message.
	injected := map[string]any{
		"role":              "assistant",
		"content":           "",
		"reasoning_content": reasoning,
	}

	insertAt := len(messages)
	for i, item := range messages {
		msg, ok := item.(map[string]any)
		if !ok {
			continue
		}
		role, _ := msg["role"].(string)
		if strings.EqualFold(strings.TrimSpace(role), "tool") {
			insertAt = i
			break
		}
	}

	next := make([]any, 0, len(messages)+1)
	next = append(next, messages[:insertAt]...)
	next = append(next, injected)
	next = append(next, messages[insertAt:]...)
	payload["messages"] = next

	out, err := json.Marshal(payload)
	if err != nil {
		return body, false
	}
	return out, true
}

func (b *BridgeRuntime) streamResponsesFailed(w http.ResponseWriter, errType string, message string) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		return
	}

	payload, _ := json.Marshal(map[string]any{
		"type":            "response.failed",
		"sequence_number": 1,
		"error": map[string]any{
			"message": message,
			"type":    errType,
		},
	})
	_, _ = w.Write([]byte("event: response.failed\n"))
	_, _ = w.Write([]byte("data: " + string(payload) + "\n\n"))
	flusher.Flush()
}

func debugPayloadEnabled() bool {
	v := strings.TrimSpace(os.Getenv("CODEX_BRIDGE_DEBUG_PAYLOAD"))
	if v == "" {
		return false
	}
	v = strings.ToLower(v)
	return v != "0" && v != "false" && v != "off" && v != "no"
}

func truncateForLog(value string, max int) string {
	if max <= 0 {
		return ""
	}
	value = strings.ReplaceAll(value, "\n", " ")
	value = strings.ReplaceAll(value, "\r", " ")
	if len(value) <= max {
		return value
	}
	return value[:max] + "...(truncated)"
}

func summarizeResponsesRequest(body []byte) string {
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return ""
	}

	model, _ := payload["model"].(string)
	stream := false
	if raw, ok := payload["stream"]; ok {
		switch typed := raw.(type) {
		case bool:
			stream = typed
		case string:
			switch strings.ToLower(strings.TrimSpace(typed)) {
			case "1", "true", "yes", "on":
				stream = true
			}
		case map[string]any:
			stream = true
		}
	}

	toolsSummary := summarizeTools(payload["tools"])
	inputSummary := summarizeResponsesInput(payload["input"])

	parts := make([]string, 0, 4)
	if strings.TrimSpace(model) != "" {
		parts = append(parts, "model="+model)
	}
	parts = append(parts, "stream="+strconv.FormatBool(stream))
	if toolsSummary != "" {
		parts = append(parts, "tools="+toolsSummary)
	}
	if inputSummary != "" {
		parts = append(parts, "input="+inputSummary)
	}
	return strings.Join(parts, " ")
}

func summarizeResponsesInput(value any) string {
	items, ok := value.([]any)
	if !ok || len(items) == 0 {
		return ""
	}
	counts := map[string]int{}
	for _, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		t, _ := m["type"].(string)
		if strings.TrimSpace(t) == "" {
			continue
		}
		counts[t]++
	}
	keys := make([]string, 0, len(counts))
	for k := range counts {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s:%d", k, counts[k]))
	}
	return strings.Join(parts, ",")
}

func summarizeTools(value any) string {
	toolsAny, ok := value.([]any)
	if !ok || len(toolsAny) == 0 {
		return ""
	}
	names := make([]string, 0, len(toolsAny))
	for _, toolAny := range toolsAny {
		tool, ok := toolAny.(map[string]any)
		if !ok {
			continue
		}
		t, _ := tool["type"].(string)
		if t == "namespace" {
			if nested, ok := tool["tools"].([]any); ok {
				if nestedSummary := summarizeTools(nested); nestedSummary != "" {
					names = append(names, nestedSummary)
				}
			}
			continue
		}
		if t == "custom" {
			t = "function"
		}
		if t != "function" {
			continue
		}
		if fn, ok := tool["function"].(map[string]any); ok {
			if name, ok := fn["name"].(string); ok && strings.TrimSpace(name) != "" {
				names = append(names, name)
			}
			continue
		}
		if name, ok := tool["name"].(string); ok && strings.TrimSpace(name) != "" {
			names = append(names, name)
		}
	}
	if len(names) == 0 {
		return fmt.Sprintf("n=%d", len(toolsAny))
	}
	if len(names) > 12 {
		names = append(names[:12], "...")
	}
	return fmt.Sprintf("n=%d(%s)", len(toolsAny), strings.Join(names, ","))
}

func (b *BridgeRuntime) doUpstream(req *http.Request, cfg AppConfig, streaming bool) (*http.Response, error) {
	req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	if req.Header.Get("Accept") == "" && streaming {
		req.Header.Set("Accept", "text/event-stream")
	}
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", "nettopo-switch/0.1")
	}

	client := &http.Client{
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

func (b *BridgeRuntime) streamResponse(w http.ResponseWriter, body io.Reader) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		_, _ = io.Copy(w, body)
		return
	}

	buf := make([]byte, 2048)
	for {
		n, err := body.Read(buf)
		if n > 0 {
			_, _ = w.Write(buf[:n])
			flusher.Flush()
		}
		if err != nil {
			return
		}
	}
}

func (b *BridgeRuntime) streamChatToResponses(w http.ResponseWriter, body io.Reader, model string) (int, []string) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		_, _ = io.Copy(w, body)
		return 0, nil
	}

	respID := fmt.Sprintf("resp_%d", time.Now().UnixNano())
	itemID := fmt.Sprintf("msg_%d", time.Now().UnixNano())
	createdAt := time.Now().Unix()
	seq := 1

	responseSkeleton := map[string]any{
		"id":         respID,
		"object":     "response",
		"created_at": createdAt,
		"model":      model,
		"status":     "in_progress",
		"output":     []any{},
	}

	writeEvent := func(eventType string, data map[string]any) {
		data["type"] = eventType
		data["sequence_number"] = seq
		seq++
		payload, _ := json.Marshal(data)
		_, _ = w.Write([]byte("event: " + eventType + "\n"))
		_, _ = w.Write([]byte("data: " + string(payload) + "\n\n"))
		flusher.Flush()
	}

	writeEvent("response.created", map[string]any{
		"response": responseSkeleton,
	})

	writeEvent("response.output_item.added", map[string]any{
		"output_index": 0,
		"item": map[string]any{
			"id":      itemID,
			"type":    "message",
			"role":    "assistant",
			"status":  "in_progress",
			"content": []any{},
		},
	})

	writeEvent("response.content_part.added", map[string]any{
		"item_id":       itemID,
		"output_index":  0,
		"content_index": 0,
		"part": map[string]any{
			"type": "output_text",
			"text": "",
		},
	})

	type toolState struct {
		id        string
		name      string
		arguments strings.Builder
		added     bool
	}

	toolStates := map[int]*toolState{}

	var buf strings.Builder
	var reasoningBuf strings.Builder
	reader := bufio.NewScanner(body)
	reader.Buffer(make([]byte, 0, 64*1024), 2*1024*1024)
	for reader.Scan() {
		line := strings.TrimSpace(reader.Text())
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			break
		}

		var chunk map[string]any
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}

		if choicesAny, ok := chunk["choices"].([]any); ok && len(choicesAny) > 0 {
			if first, ok := choicesAny[0].(map[string]any); ok {
				if deltaAny, ok := first["delta"].(map[string]any); ok {
					if rc, ok := deltaAny["reasoning_content"].(string); ok && strings.TrimSpace(rc) != "" {
						reasoningBuf.WriteString(rc)
					} else if rc, ok := deltaAny["reasoning"].(string); ok && strings.TrimSpace(rc) != "" {
						reasoningBuf.WriteString(rc)
					}
					if rawToolCalls, ok := deltaAny["tool_calls"].([]any); ok && len(rawToolCalls) > 0 {
						for fallbackIndex, raw := range rawToolCalls {
							call, ok := raw.(map[string]any)
							if !ok {
								continue
							}
							index := fallbackIndex
							if v, ok := call["index"].(float64); ok {
								index = int(v)
							}

							id, _ := call["id"].(string)
							fn, _ := call["function"].(map[string]any)
							name, _ := fn["name"].(string)
							argsDelta, _ := fn["arguments"].(string)
							if strings.TrimSpace(id) == "" {
								id = fmt.Sprintf("call_%d", time.Now().UnixNano())
							}

							state := toolStates[index]
							if state == nil {
								state = &toolState{id: id}
								toolStates[index] = state
							}
							if strings.TrimSpace(state.id) == "" {
								state.id = id
							}
							if strings.TrimSpace(name) != "" {
								state.name = name
							}

							if !state.added {
								state.added = true
								writeEvent("response.output_item.added", map[string]any{
									"output_index": 1 + index,
									"item": map[string]any{
										"id":        state.id,
										"type":      "function_call",
										"call_id":   state.id,
										"name":      state.name,
										"arguments": "",
										"status":    "in_progress",
									},
								})
							}

							if strings.TrimSpace(argsDelta) != "" {
								state.arguments.WriteString(argsDelta)
								writeEvent("response.function_call_arguments.delta", map[string]any{
									"item_id":      state.id,
									"output_index": 1 + index,
									"delta":        argsDelta,
								})
							}
						}
					}
				}
			}
		}

		deltaText := extractChatDeltaText(chunk)
		if deltaText != "" {
			buf.WriteString(deltaText)
			writeEvent("response.output_text.delta", map[string]any{
				"item_id":       itemID,
				"output_index":  0,
				"content_index": 0,
				"delta":         deltaText,
			})
		}
	}

	finalText := buf.String()
	finalReasoning := reasoningBuf.String()
	if strings.TrimSpace(finalReasoning) != "" {
		b.setLastReasoning(finalReasoning)
	}

	writeEvent("response.output_text.done", map[string]any{
		"item_id":       itemID,
		"output_index":  0,
		"content_index": 0,
		"text":          finalText,
	})

	finalItem := map[string]any{
		"id":     itemID,
		"type":   "message",
		"role":   "assistant",
		"status": "completed",
		"content": []any{
			map[string]any{
				"type": "output_text",
				"text": finalText,
			},
		},
	}
	if strings.TrimSpace(finalReasoning) != "" {
		finalItem["reasoning_content"] = finalReasoning
	}

	writeEvent("response.output_item.done", map[string]any{
		"output_index": 0,
		"item":         finalItem,
	})

	toolIndices := make([]int, 0, len(toolStates))
	for idx := range toolStates {
		toolIndices = append(toolIndices, idx)
	}
	sort.Ints(toolIndices)

	finalOutput := make([]any, 0, 1+len(toolIndices))
	finalOutput = append(finalOutput, finalItem)
	requiredToolCalls := make([]any, 0, len(toolIndices))

	for _, idx := range toolIndices {
		state := toolStates[idx]
		if state == nil {
			continue
		}
		args := state.arguments.String()
		writeEvent("response.function_call_arguments.done", map[string]any{
			"item_id":      state.id,
			"output_index": 1 + idx,
			"arguments":    args,
		})

		callItem := map[string]any{
			"id":        state.id,
			"type":      "function_call",
			"call_id":   state.id,
			"name":      state.name,
			"arguments": args,
			"status":    "completed",
		}

		writeEvent("response.output_item.done", map[string]any{
			"output_index": 1 + idx,
			"item":         callItem,
		})

		finalOutput = append(finalOutput, callItem)
		requiredToolCalls = append(requiredToolCalls, map[string]any{
			"id":   state.id,
			"type": "function",
			"function": map[string]any{
				"name":      state.name,
				"arguments": args,
			},
		})
	}

	finalResponse := map[string]any{
		"id":         respID,
		"object":     "response",
		"created_at": createdAt,
		"model":      model,
		"status":     "completed",
		"output":     finalOutput,
	}
	if len(requiredToolCalls) > 0 {
		finalResponse["status"] = "requires_action"
		finalResponse["required_action"] = map[string]any{
			"type": "submit_tool_outputs",
			"submit_tool_outputs": map[string]any{
				"tool_calls": requiredToolCalls,
			},
		}
	}

	writeEvent("response.completed", map[string]any{
		"response": finalResponse,
	})

	toolNames := make([]string, 0, len(toolIndices))
	for _, idx := range toolIndices {
		state := toolStates[idx]
		if state == nil {
			continue
		}
		if strings.TrimSpace(state.name) != "" {
			toolNames = append(toolNames, state.name)
		}
	}
	if len(toolNames) > 12 {
		toolNames = append(toolNames[:12], "...")
	}

	return len(finalText), toolNames
}

func (b *BridgeRuntime) setStatus(status BridgeStatus, lastError string) {
	b.mu.Lock()
	b.status = status
	b.lastError = lastError
	if status == BridgeStopped {
		b.listenAddress = ""
		b.startedAt = time.Time{}
		atomic.StoreInt64(&b.requestCount, 0)
		b.lastReasoning = ""
		b.lastReasonAt = time.Time{}
	}
	b.mu.Unlock()

	b.app.emitStatus()
}

func (b *BridgeRuntime) snapshotConfig() AppConfig {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.config
}

func (b *BridgeRuntime) writeJSON(w http.ResponseWriter, statusCode int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(payload)
}

func (b *BridgeRuntime) writeProxyError(w http.ResponseWriter, statusCode int, message string) {
	code := "bridge_error"
	if statusCode == http.StatusBadGateway {
		code = "bad_gateway"
	}
	b.writeJSON(w, statusCode, map[string]any{
		"error": map[string]any{
			"message": message,
			"type":    "bridge_error",
			"code":    code,
		},
	})
}

func (b *BridgeRuntime) copyHeaders(dst http.Header, src http.Header) {
	for key, values := range src {
		if strings.EqualFold(key, "Content-Length") {
			continue
		}
		for _, value := range values {
			dst.Add(key, value)
		}
	}
}

func translateChatCompletions(body []byte, cfg AppConfig) ([]byte, error) {
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, errors.New("请求体不是有效的 JSON")
	}

	model := strings.TrimSpace(cfg.DefaultModel)
	if rawModel, ok := payload["model"].(string); ok && strings.TrimSpace(rawModel) != "" {
		mappedModel := cfg.Mappings[rawModel]
		if strings.TrimSpace(mappedModel) != "" {
			model = mappedModel
		} else {
			model = rawModel
		}
	}
	payload["model"] = model

	if messagesAny, ok := payload["messages"].([]any); ok {
		for _, item := range messagesAny {
			msg, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if role, ok := msg["role"].(string); ok {
				msg["role"] = normalizeChatRole(role)
			}
		}
		payload["messages"] = messagesAny
	}

	if v, ok := payload["tools"]; ok {
		payload["tools"] = normalizeToolsForChatCompletions(v)
	}
	if v, ok := payload["tool_choice"]; ok {
		payload["tool_choice"] = normalizeToolChoiceForChatCompletions(v)
	}
	if _, ok := payload["thinking"]; !ok {
		if thinking := autoThinkingParam(payload["tools"]); thinking != nil {
			payload["thinking"] = thinking
		}
	}

	translatedBody, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return translatedBody, nil
}

func normalizeChatRole(role string) string {
	role = strings.TrimSpace(strings.ToLower(role))
	switch role {
	case "system", "user", "assistant", "tool", "latest_reminder":
		return role
	case "developer":
		return "system"
	default:
		return "user"
	}
}

func translateResponsesToChatCompletions(body []byte, cfg AppConfig) ([]byte, bool, string, error) {
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, false, "", errors.New("请求体不是有效的 JSON")
	}

	streaming := false
	if raw, ok := payload["stream"]; ok {
		switch typed := raw.(type) {
		case bool:
			streaming = typed
		case string:
			switch strings.ToLower(strings.TrimSpace(typed)) {
			case "1", "true", "yes", "on":
				streaming = true
			}
		case map[string]any:
			streaming = true
		}
	}

	model := strings.TrimSpace(cfg.DefaultModel)
	if rawModel, ok := payload["model"].(string); ok && strings.TrimSpace(rawModel) != "" {
		mapped := cfg.Mappings[rawModel]
		if strings.TrimSpace(mapped) != "" {
			model = mapped
		} else {
			model = rawModel
		}
	}

	messages, err := responsesInputToMessages(payload)
	if err != nil {
		return nil, false, "", err
	}
	if len(messages) == 0 {
		return nil, false, "", errors.New("无法从 input/messages 提取有效文本内容")
	}

	chatPayload := map[string]any{
		"model":    model,
		"messages": messages,
		"stream":   streaming,
	}

	if v, ok := payload["temperature"]; ok {
		chatPayload["temperature"] = v
	}
	if v, ok := payload["top_p"]; ok {
		chatPayload["top_p"] = v
	}
	if v, ok := payload["max_output_tokens"]; ok {
		chatPayload["max_tokens"] = v
	}
	if v, ok := payload["max_tokens"]; ok {
		chatPayload["max_tokens"] = v
	}
	if v, ok := payload["tools"]; ok {
		chatPayload["tools"] = normalizeToolsForChatCompletions(v)
	}
	if v, ok := payload["tool_choice"]; ok {
		chatPayload["tool_choice"] = normalizeToolChoiceForChatCompletions(v)
	}
	if v, ok := payload["parallel_tool_calls"]; ok {
		chatPayload["parallel_tool_calls"] = v
	}
	if v, ok := payload["thinking"]; ok {
		chatPayload["thinking"] = v
	} else if thinking := autoThinkingParam(chatPayload["tools"]); thinking != nil {
		chatPayload["thinking"] = thinking
	}

	out, err := json.Marshal(chatPayload)
	if err != nil {
		return nil, false, "", err
	}
	return out, streaming, model, nil
}

func normalizeToolsForChatCompletions(value any) any {
	tools, ok := value.([]any)
	if !ok {
		return value
	}

	out := make([]any, 0, len(tools))
	for _, item := range tools {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}

		t, _ := m["type"].(string)
		if strings.TrimSpace(t) == "" {
			continue
		}

		if t == "custom" {
			t = "function"
		}

		if t == "namespace" {
			if nested, ok := m["tools"].([]any); ok && len(nested) > 0 {
				if normalized, ok := normalizeToolsForChatCompletions(nested).([]any); ok {
					out = append(out, normalized...)
				}
			}
			continue
		}

		if t != "function" {
			continue
		}

		if _, ok := m["function"].(map[string]any); ok {
			out = append(out, item)
			continue
		}

		name, _ := m["name"].(string)
		desc, _ := m["description"].(string)
		params := m["parameters"]

		fn := map[string]any{
			"name": name,
		}
		if strings.TrimSpace(desc) != "" {
			fn["description"] = desc
		}
		if params != nil {
			fn["parameters"] = params
		}

		normalized := map[string]any{
			"type":     "function",
			"function": fn,
		}
		if v, ok := m["strict"]; ok {
			normalized["strict"] = v
		}
		out = append(out, normalized)
	}
	return out
}

func normalizeToolChoiceForChatCompletions(value any) any {
	if s, ok := value.(string); ok {
		return s
	}
	m, ok := value.(map[string]any)
	if !ok {
		return value
	}
	t, _ := m["type"].(string)
	if t == "namespace" {
		return "auto"
	}
	if t == "custom" {
		t = "function"
	}
	if t != "function" {
		return value
	}
	if _, ok := m["function"].(map[string]any); ok {
		return value
	}
	name, _ := m["name"].(string)
	if strings.TrimSpace(name) == "" {
		return value
	}
	return map[string]any{
		"type": "function",
		"function": map[string]any{
			"name": name,
		},
	}
}

func autoThinkingParam(tools any) any {
	mode := strings.ToLower(strings.TrimSpace(os.Getenv("CODEX_BRIDGE_THINKING")))
	switch mode {
	case "1", "true", "on", "enabled":
		return map[string]any{"type": "enabled"}
	case "0", "false", "off", "disabled":
		return map[string]any{"type": "disabled"}
	}

	toolsAny, ok := tools.([]any)
	if ok && len(toolsAny) > 0 {
		return map[string]any{"type": "disabled"}
	}
	return nil
}

func responsesInputToMessages(payload map[string]any) ([]any, error) {
	messages := make([]any, 0, 8)

	if instructions, ok := payload["instructions"].(string); ok && strings.TrimSpace(instructions) != "" {
		messages = append(messages, map[string]any{
			"role":    "system",
			"content": instructions,
		})
	}

	input, ok := payload["input"]
	if !ok {
		if fallback, ok := payload["messages"]; ok {
			input = fallback
		} else {
			return messages, nil
		}
	}

	switch typed := input.(type) {
	case string:
		if strings.TrimSpace(typed) != "" {
			messages = append(messages, map[string]any{"role": "user", "content": typed})
		}
	case []any:
		for _, item := range typed {
			msg, ok := item.(map[string]any)
			if !ok {
				if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
					messages = append(messages, map[string]any{"role": "user", "content": s})
				}
				continue
			}

			if t, ok := msg["type"].(string); ok && strings.TrimSpace(t) != "" {
				switch strings.TrimSpace(t) {
				case "message":
					// fallthrough to role/content parsing below
				case "input_text":
					if text := strings.TrimSpace(flattenResponsesContent(msg)); text != "" {
						messages = append(messages, map[string]any{"role": "user", "content": text})
					}
					continue
				case "function_call_output":
					callID, _ := msg["call_id"].(string)
					if callID == "" {
						callID, _ = msg["tool_call_id"].(string)
					}
					if callID == "" {
						callID, _ = msg["id"].(string)
					}
					output := strings.TrimSpace(flattenAnyText(msg["output"]))
					if output == "" {
						output = strings.TrimSpace(flattenResponsesContent(msg["content"]))
					}
					if output == "" {
						output = strings.TrimSpace(flattenResponsesContent(msg))
					}
					messages = append(messages, map[string]any{
						"role":         "tool",
						"tool_call_id": callID,
						"content":      output,
					})
					continue
				case "function_call":
					callID, _ := msg["call_id"].(string)
					if callID == "" {
						callID, _ = msg["id"].(string)
					}
					name, _ := msg["name"].(string)
					arguments := ""
					switch v := msg["arguments"].(type) {
					case string:
						arguments = v
					case map[string]any, []any:
						if out, err := json.Marshal(v); err == nil {
							arguments = string(out)
						}
					}
					if callID == "" && name == "" && strings.TrimSpace(arguments) == "" {
						continue
					}
					messages = append(messages, map[string]any{
						"role":    "assistant",
						"content": "",
						"tool_calls": []any{
							map[string]any{
								"id":   callID,
								"type": "function",
								"function": map[string]any{
									"name":      name,
									"arguments": arguments,
								},
							},
						},
					})
					continue
				}
			}

			role, _ := msg["role"].(string)
			role = normalizeChatRole(role)

			content := flattenResponsesContent(msg["content"])
			if strings.TrimSpace(content) == "" {
				content = flattenResponsesContent(msg)
			}
			reasoning := ""
			if role == "assistant" {
				reasoning = strings.TrimSpace(extractResponsesReasoningContent(msg))
			}
			if strings.TrimSpace(content) == "" && reasoning == "" {
				continue
			}
			chatMsg := map[string]any{"role": role, "content": content}
			if role == "assistant" && reasoning != "" {
				chatMsg["reasoning_content"] = reasoning
			}
			messages = append(messages, chatMsg)
		}
	case map[string]any:
		role, _ := typed["role"].(string)
		role = normalizeChatRole(role)
		content := flattenResponsesContent(typed["content"])
		if strings.TrimSpace(content) == "" {
			content = flattenResponsesContent(typed)
		}
		reasoning := ""
		if role == "assistant" {
			reasoning = strings.TrimSpace(extractResponsesReasoningContent(typed))
		}
		if strings.TrimSpace(content) != "" || reasoning != "" {
			chatMsg := map[string]any{"role": role, "content": content}
			if role == "assistant" && reasoning != "" {
				chatMsg["reasoning_content"] = reasoning
			}
			messages = append(messages, chatMsg)
		}
	default:
		return nil, errors.New("Responses input 格式不支持")
	}

	return messages, nil
}

func extractResponsesReasoningContent(msg map[string]any) string {
	if v, ok := msg["reasoning_content"].(string); ok && strings.TrimSpace(v) != "" {
		return v
	}
	if v, ok := msg["reasoning"].(string); ok && strings.TrimSpace(v) != "" {
		return v
	}

	contentAny, ok := msg["content"].([]any)
	if !ok || len(contentAny) == 0 {
		return ""
	}

	var parts []string
	for _, partAny := range contentAny {
		part, ok := partAny.(map[string]any)
		if !ok {
			continue
		}
		t, _ := part["type"].(string)
		t = strings.ToLower(strings.TrimSpace(t))
		if t == "" {
			continue
		}
		if strings.Contains(t, "reason") || strings.Contains(t, "think") {
			if text := strings.TrimSpace(flattenAnyText(part["text"])); text != "" {
				parts = append(parts, text)
				continue
			}
			if text := strings.TrimSpace(flattenAnyText(part["reasoning_content"])); text != "" {
				parts = append(parts, text)
				continue
			}
		}
	}
	return strings.Join(parts, "")
}

func flattenResponsesContent(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case []any:
		var parts []string
		for _, part := range typed {
			switch p := part.(type) {
			case string:
				if strings.TrimSpace(p) != "" {
					parts = append(parts, p)
				}
			case map[string]any:
				if t := strings.TrimSpace(flattenAnyText(p["text"])); t != "" {
					parts = append(parts, t)
					continue
				}
				if t := strings.TrimSpace(flattenAnyText(p["input_text"])); t != "" {
					parts = append(parts, t)
					continue
				}
				if t := strings.TrimSpace(flattenAnyText(p["content"])); t != "" {
					parts = append(parts, t)
					continue
				}
				if t, ok := p["type"].(string); ok && t == "input_text" {
					if t2 := strings.TrimSpace(flattenAnyText(p["text"])); t2 != "" {
						parts = append(parts, t2)
					}
					continue
				}
			}
		}
		return strings.Join(parts, "")
	case map[string]any:
		if t := strings.TrimSpace(flattenAnyText(typed["text"])); t != "" {
			return t
		}
		if t := strings.TrimSpace(flattenAnyText(typed["input_text"])); t != "" {
			return t
		}
		if t := strings.TrimSpace(flattenAnyText(typed["content"])); t != "" {
			return t
		}
	}
	return ""
}

func extractChatDeltaText(chunk map[string]any) string {
	choicesAny, ok := chunk["choices"]
	if !ok {
		return ""
	}
	choices, ok := choicesAny.([]any)
	if !ok || len(choices) == 0 {
		return ""
	}
	first, ok := choices[0].(map[string]any)
	if !ok {
		return ""
	}
	deltaAny, ok := first["delta"]
	if !ok {
		return ""
	}
	delta, ok := deltaAny.(map[string]any)
	if !ok {
		return ""
	}
	if content, ok := delta["content"].(string); ok {
		return content
	}
	return ""
}

type chatToolCall struct {
	ID        string
	Name      string
	Arguments string
}

func extractChatDeltaToolCalls(chunk map[string]any) []chatToolCall {
	choicesAny, ok := chunk["choices"]
	if !ok {
		return nil
	}
	choices, ok := choicesAny.([]any)
	if !ok || len(choices) == 0 {
		return nil
	}
	first, ok := choices[0].(map[string]any)
	if !ok {
		return nil
	}
	deltaAny, ok := first["delta"]
	if !ok {
		return nil
	}
	delta, ok := deltaAny.(map[string]any)
	if !ok {
		return nil
	}
	raw, ok := delta["tool_calls"].([]any)
	if !ok || len(raw) == 0 {
		return nil
	}

	out := make([]chatToolCall, 0, len(raw))
	for _, item := range raw {
		tc, ok := item.(map[string]any)
		if !ok {
			continue
		}
		id, _ := tc["id"].(string)
		fn, _ := tc["function"].(map[string]any)
		name, _ := fn["name"].(string)
		args, _ := fn["arguments"].(string)
		out = append(out, chatToolCall{ID: id, Name: name, Arguments: args})
	}
	return out
}

func extractChatCompletionToolCalls(payload map[string]any) []chatToolCall {
	choicesAny, ok := payload["choices"]
	if !ok {
		return nil
	}
	choices, ok := choicesAny.([]any)
	if !ok || len(choices) == 0 {
		return nil
	}
	first, ok := choices[0].(map[string]any)
	if !ok {
		return nil
	}
	msgAny, ok := first["message"].(map[string]any)
	if !ok {
		return nil
	}
	raw, ok := msgAny["tool_calls"].([]any)
	if !ok || len(raw) == 0 {
		return nil
	}

	out := make([]chatToolCall, 0, len(raw))
	for _, item := range raw {
		tc, ok := item.(map[string]any)
		if !ok {
			continue
		}
		id, _ := tc["id"].(string)
		fn, _ := tc["function"].(map[string]any)
		name, _ := fn["name"].(string)
		args, _ := fn["arguments"].(string)
		out = append(out, chatToolCall{ID: id, Name: name, Arguments: args})
	}
	return out
}

func translateChatCompletionToResponses(body []byte, model string) (map[string]any, error) {
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, errors.New("上游响应不是有效的 JSON")
	}

	text := extractChatCompletionText(payload)
	toolCalls := extractChatCompletionToolCalls(payload)
	reasoning := extractChatCompletionReasoning(payload)

	usage := map[string]any{}
	if usageAny, ok := payload["usage"].(map[string]any); ok {
		if v, ok := usageAny["prompt_tokens"]; ok {
			usage["input_tokens"] = v
		}
		if v, ok := usageAny["completion_tokens"]; ok {
			usage["output_tokens"] = v
		}
		if v, ok := usageAny["total_tokens"]; ok {
			usage["total_tokens"] = v
		}
	}

	respID := fmt.Sprintf("resp_%d", time.Now().UnixNano())
	createdAt := time.Now().Unix()

	output := make([]any, 0, 1+len(toolCalls))
	if strings.TrimSpace(reasoning) != "" {
		itemID := fmt.Sprintf("msg_%d", time.Now().UnixNano())
		output = append(output, map[string]any{
			"id":                itemID,
			"type":              "message",
			"role":              "assistant",
			"status":            "completed",
			"content":           []any{},
			"reasoning_content": reasoning,
		})
	}

	if strings.TrimSpace(text) != "" {
		itemID := fmt.Sprintf("msg_%d", time.Now().UnixNano())
		item := map[string]any{
			"id":     itemID,
			"type":   "message",
			"role":   "assistant",
			"status": "completed",
			"content": []any{
				map[string]any{
					"type": "output_text",
					"text": text,
				},
			},
		}
		if strings.TrimSpace(reasoning) != "" {
			item["reasoning_content"] = reasoning
		}
		output = append(output, item)
	}

	requiredToolCalls := make([]any, 0, len(toolCalls))
	for _, tc := range toolCalls {
		callID := strings.TrimSpace(tc.ID)
		if callID == "" {
			callID = fmt.Sprintf("call_%d", time.Now().UnixNano())
		}
		output = append(output, map[string]any{
			"id":        callID,
			"type":      "function_call",
			"call_id":   callID,
			"name":      tc.Name,
			"arguments": tc.Arguments,
		})
		requiredToolCalls = append(requiredToolCalls, map[string]any{
			"id":   callID,
			"type": "function",
			"function": map[string]any{
				"name":      tc.Name,
				"arguments": tc.Arguments,
			},
		})
	}

	response := map[string]any{
		"id":         respID,
		"object":     "response",
		"created_at": createdAt,
		"model":      model,
		"status":     "completed",
		"output":     output,
	}
	if len(requiredToolCalls) > 0 {
		response["status"] = "requires_action"
		response["required_action"] = map[string]any{
			"type": "submit_tool_outputs",
			"submit_tool_outputs": map[string]any{
				"tool_calls": requiredToolCalls,
			},
		}
	}
	if len(usage) > 0 {
		response["usage"] = usage
	}
	return response, nil
}

func extractChatCompletionReasoning(payload map[string]any) string {
	choicesAny, ok := payload["choices"]
	if !ok {
		return ""
	}
	choices, ok := choicesAny.([]any)
	if !ok || len(choices) == 0 {
		return ""
	}
	first, ok := choices[0].(map[string]any)
	if !ok {
		return ""
	}
	msgAny, ok := first["message"].(map[string]any)
	if !ok {
		return ""
	}
	if rc, ok := msgAny["reasoning_content"].(string); ok {
		return rc
	}
	if rc, ok := msgAny["reasoning"].(string); ok {
		return rc
	}
	return ""
}

func extractChatCompletionReasoningFromBody(body []byte) string {
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return ""
	}
	return extractChatCompletionReasoning(payload)
}

func extractChatCompletionText(payload map[string]any) string {
	choicesAny, ok := payload["choices"]
	if !ok {
		return ""
	}
	choices, ok := choicesAny.([]any)
	if !ok || len(choices) == 0 {
		return ""
	}
	first, ok := choices[0].(map[string]any)
	if !ok {
		return ""
	}

	if msgAny, ok := first["message"].(map[string]any); ok {
		if content, ok := msgAny["content"]; ok {
			if text := flattenAnyText(content); strings.TrimSpace(text) != "" {
				return text
			}
		}
	}

	if textAny, ok := first["text"]; ok {
		return flattenAnyText(textAny)
	}

	return ""
}

func flattenAnyText(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	case []any:
		parts := make([]string, 0, len(typed))
		for _, part := range typed {
			p := strings.TrimSpace(flattenAnyText(part))
			if p != "" {
				parts = append(parts, p)
			}
		}
		return strings.Join(parts, "")
	case map[string]any:
		if t, ok := typed["text"].(string); ok && strings.TrimSpace(t) != "" {
			return t
		}
		if t, ok := typed["content"].(string); ok && strings.TrimSpace(t) != "" {
			return t
		}
		if t, ok := typed["input_text"].(string); ok && strings.TrimSpace(t) != "" {
			return t
		}
		if t, ok := typed["value"].(string); ok && strings.TrimSpace(t) != "" {
			return t
		}
		if v, ok := typed["text"]; ok {
			if t := strings.TrimSpace(flattenAnyText(v)); t != "" {
				return t
			}
		}
		if v, ok := typed["content"]; ok {
			if t := strings.TrimSpace(flattenAnyText(v)); t != "" {
				return t
			}
		}
	}
	return ""
}

func upstreamResourceURL(base string, resource string) (string, error) {
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	resource = strings.Trim(strings.TrimSpace(resource), "/")
	if base == "" || resource == "" {
		return "", errors.New("上游地址配置不完整")
	}

	parsed, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return "", errors.New("DeepSeek Base URL 格式不正确")
	}

	path := strings.TrimRight(parsed.Path, "/")
	if path == "" {
		path = "/v1"
	}
	if !strings.HasSuffix(path, "/v1") {
		path = path + "/v1"
	}
	parsed.Path = path + "/" + resource
	return parsed.String(), nil
}

func copyRequestHeaders(dst http.Header, src http.Header, extraHeaders map[string]string) {
	for key, values := range src {
		switch {
		case strings.EqualFold(key, "Authorization"):
			continue
		case strings.EqualFold(key, "Host"):
			continue
		case strings.EqualFold(key, "Content-Length"):
			continue
		}
		for _, value := range values {
			dst.Add(key, value)
		}
	}

	for key, value := range extraHeaders {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key != "" && value != "" {
			dst.Set(key, value)
		}
	}
}

func statusToLevel(statusCode int) string {
	switch {
	case statusCode >= 500:
		return "error"
	case statusCode >= 400:
		return "warn"
	default:
		return "info"
	}
}
