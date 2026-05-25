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
		b.app.appendLog(level, "proxy", fmt.Sprintf("%s %s -> %d (%dms) ua=%s", r.Method, r.URL.Path, statusCode, duration, ua), "")
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

	chatBody, streaming, model, err := translateResponsesToChatCompletions(body, cfg)
	if err != nil {
		b.app.appendLog("warn", "proxy", "responses 请求体解析失败: "+err.Error()+" keys="+summarizeJSONKeys(body), requestID)
		b.writeProxyError(w, http.StatusBadRequest, err.Error())
		return
	}
	if !streaming && strings.Contains(strings.ToLower(r.Header.Get("Accept")), "text/event-stream") {
		streaming = true
	}
	if streaming && !bytes.Contains(chatBody, []byte(`"stream":true`)) {
		var patched map[string]any
		if err := json.Unmarshal(chatBody, &patched); err == nil {
			patched["stream"] = true
			if out, err := json.Marshal(patched); err == nil {
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

		upstreamCtx := context.Background()
		var cancel context.CancelFunc
		if cfg.RequestTimeoutMs > 0 {
			upstreamCtx, cancel = context.WithTimeout(upstreamCtx, time.Duration(cfg.RequestTimeoutMs)*time.Millisecond)
		} else {
			upstreamCtx, cancel = context.WithCancel(upstreamCtx)
		}
		defer cancel()

		go func() {
			select {
			case <-r.Context().Done():
				cancel()
			case <-upstreamCtx.Done():
			}
		}()

		req, err := http.NewRequestWithContext(upstreamCtx, http.MethodPost, resourceURL, bytes.NewReader(chatBody))
		if err != nil {
			writeEvent("response.failed", map[string]any{
				"error": map[string]any{
					"message": err.Error(),
					"type":    "bad_gateway",
				},
			})
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
			writeEvent("response.failed", map[string]any{
				"error": map[string]any{
					"message": err.Error(),
					"type":    "bad_gateway",
				},
			})
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
			writeEvent("response.failed", map[string]any{
				"error": map[string]any{
					"message": msg,
					"type":    "bad_gateway",
				},
			})
			b.app.appendLog("error", "proxy", msg, requestID)
			return
		}

		var buf strings.Builder
		reader := bufio.NewScanner(resp.Body)
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

			delta := extractChatDeltaText(chunk)
			if delta == "" {
				continue
			}
			buf.WriteString(delta)
			writeEvent("response.output_text.delta", map[string]any{
				"item_id":       itemID,
				"output_index":  0,
				"content_index": 0,
				"delta":         delta,
			})
		}

		finalText := buf.String()

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

		writeEvent("response.output_item.done", map[string]any{
			"output_index": 0,
			"item":         finalItem,
		})

		finalResponse := map[string]any{
			"id":         respID,
			"object":     "response",
			"created_at": createdAt,
			"model":      model,
			"status":     "completed",
			"output":     []any{finalItem},
		}

		writeEvent("response.completed", map[string]any{
			"response": finalResponse,
		})

		duration := time.Since(startedAt).Milliseconds()
		b.app.appendLog(
			"info",
			"proxy",
			fmt.Sprintf("POST /v1/responses (stream) -> 200 (%dms)", duration),
			requestID,
		)
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

		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
		statusCode = resp.StatusCode
		if resp.StatusCode >= http.StatusBadRequest {
			b.copyHeaders(w.Header(), resp.Header)
			w.WriteHeader(resp.StatusCode)
			_, _ = w.Write(raw)
		} else {
			response, err := translateChatCompletionToResponses(raw, model)
			if err != nil {
				b.app.appendLog("error", "proxy", "responses 响应转换失败: "+err.Error(), requestID)
				b.writeProxyError(w, http.StatusBadGateway, err.Error())
			} else {
				b.writeJSON(w, resp.StatusCode, response)
			}
		}
	}

	duration := time.Since(startedAt).Milliseconds()
	b.app.appendLog(
		statusToLevel(statusCode),
		"proxy",
		fmt.Sprintf("POST /v1/responses -> %d (%dms)", statusCode, duration),
		requestID,
	)
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

func (b *BridgeRuntime) streamChatToResponses(w http.ResponseWriter, body io.Reader, model string) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		_, _ = io.Copy(w, body)
		return
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

	var buf strings.Builder
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

		delta := extractChatDeltaText(chunk)
		if delta == "" {
			continue
		}
		buf.WriteString(delta)
		writeEvent("response.output_text.delta", map[string]any{
			"item_id":       itemID,
			"output_index":  0,
			"content_index": 0,
			"delta":         delta,
		})
	}

	finalText := buf.String()

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

	writeEvent("response.output_item.done", map[string]any{
		"output_index": 0,
		"item":         finalItem,
	})

	finalResponse := map[string]any{
		"id":         respID,
		"object":     "response",
		"created_at": createdAt,
		"model":      model,
		"status":     "completed",
		"output":     []any{finalItem},
	}

	writeEvent("response.completed", map[string]any{
		"response": finalResponse,
	})
}

func (b *BridgeRuntime) setStatus(status BridgeStatus, lastError string) {
	b.mu.Lock()
	b.status = status
	b.lastError = lastError
	if status == BridgeStopped {
		b.listenAddress = ""
		b.startedAt = time.Time{}
		atomic.StoreInt64(&b.requestCount, 0)
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

	out, err := json.Marshal(chatPayload)
	if err != nil {
		return nil, false, "", err
	}
	return out, streaming, model, nil
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
				}
			}

			role, _ := msg["role"].(string)
			role = normalizeChatRole(role)

			content := flattenResponsesContent(msg["content"])
			if strings.TrimSpace(content) == "" {
				content = flattenResponsesContent(msg)
			}
			if strings.TrimSpace(content) == "" {
				continue
			}
			messages = append(messages, map[string]any{"role": role, "content": content})
		}
	case map[string]any:
		role, _ := typed["role"].(string)
		role = normalizeChatRole(role)
		content := flattenResponsesContent(typed["content"])
		if strings.TrimSpace(content) == "" {
			content = flattenResponsesContent(typed)
		}
		if strings.TrimSpace(content) != "" {
			messages = append(messages, map[string]any{"role": role, "content": content})
		}
	default:
		return nil, errors.New("Responses input 格式不支持")
	}

	return messages, nil
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

func translateChatCompletionToResponses(body []byte, model string) (map[string]any, error) {
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, errors.New("上游响应不是有效的 JSON")
	}

	text := extractChatCompletionText(payload)

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
	itemID := fmt.Sprintf("msg_%d", time.Now().UnixNano())
	createdAt := time.Now().Unix()

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

	response := map[string]any{
		"id":         respID,
		"object":     "response",
		"created_at": createdAt,
		"model":      model,
		"status":     "completed",
		"output":     []any{item},
	}
	if len(usage) > 0 {
		response["usage"] = usage
	}
	return response, nil
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
