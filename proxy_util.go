package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
)

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

func (s *statusRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if hijacker, ok := s.ResponseWriter.(http.Hijacker); ok {
		return hijacker.Hijack()
	}
	return nil, nil, errors.New("statusRecorder: hijack not supported")
}

func extractModelFromBody(body []byte) string {
	var payload struct {
		Model string `json:"model"`
	}
	if err := json.Unmarshal(body, &payload); err == nil {
		return payload.Model
	}
	return ""
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

// handleMessages converts Anthropic Messages API (/v1/messages) to Chat Completions.
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
		return "", errors.New("上游 Base URL 格式不正确")
	}

	// 当 baseURL 未包含任何路径时（如自定义提供商只填域名），自动追加 /v1
	if parsed.Path == "" || parsed.Path == "/" {
		parsed.Path = "/v1"
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/") + "/" + resource
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

