package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
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

// upstreamError extracts a clean message from upstream API error JSON responses.
// Handles formats like:
//
//	{"error":{"message":"Authentication Fails, Your api key: ... is invalid","type":"...","param":null,"code":"..."}}
//	{"error":{"message":"Insufficient balance","type":"...","code":"..."}}
//	{"error":"not_found","error_description":"model not found"}
//
// On failure, returns the original body text cleaned up.
func upstreamError(statusCode int, body []byte) string {
	raw := strings.TrimSpace(string(body))
	if raw == "" {
		return fmt.Sprintf("API 返回状态 %d: 响应为空", statusCode)
	}

	// Try parsing different error JSON formats
	var payload struct {
		ErrorMsg string `json:"error"` // catch-all, also used for plain string errors
	}
	if err := json.Unmarshal([]byte(raw), &payload); err == nil && payload.ErrorMsg != "" {
		// "error" was a plain string like "not_found" — try error_description
		var desc struct {
			Description string `json:"error_description"`
		}
		if err := json.Unmarshal([]byte(raw), &desc); err == nil && desc.Description != "" {
			return cleanErrorMessage(desc.Description)
		}
	}

	// {"error":{"message":"..."}}
	var structured struct {
		Err struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(raw), &structured); err == nil && structured.Err.Message != "" {
		return cleanErrorMessage(structured.Err.Message)
	}

	// Check for non-JSON or unknown format — return a shortened form
	if !strings.HasPrefix(raw, "{") {
		return fmt.Sprintf("API 返回状态 %d: %s", statusCode, truncateForLog(raw, 200))
	}

	// JSON but unrecognized structure — extract what we can
	var anyMap map[string]any
	if err := json.Unmarshal([]byte(raw), &anyMap); err == nil {
		if msg, ok := anyMap["error_description"].(string); ok && msg != "" {
			return cleanErrorMessage(msg)
		}
		if msg, ok := anyMap["error"].(string); ok && msg != "" {
			return cleanErrorMessage(msg)
		}
		if msg, ok := anyMap["message"].(string); ok && msg != "" {
			return cleanErrorMessage(msg)
		}
	}

	// Fall back to shortened raw response
	short := truncateForLog(raw, 300)
	return fmt.Sprintf("API 返回状态 %d: %s", statusCode, short)
}

// cleanErrorMessage sanitizes an extracted API error message.
func cleanErrorMessage(msg string) string {
	msg = strings.TrimSpace(msg)
	msg = strings.ReplaceAll(msg, "\n", " ")
	msg = strings.ReplaceAll(msg, "\r", " ")
	// Mask potential API keys in the message (e.g. "Your api key: sk-1234" or "key 'sk-...'")
	msg = maskAPIKey(msg)
	// Limit length
	if len(msg) > 300 {
		msg = msg[:300] + "..."
	}
	return msg
}

// maskAPIKey replaces common API key patterns in error messages.
func maskAPIKey(msg string) string {
	// Match patterns like "api key: sk-xxx", "key 'sk-...'", "apikey=sk-xxx"
	patterns := []struct {
		prefix string
		suffix string
	}{
		{"api key: ", ""},
		{"api key ", ""},
		{"key: ", ""},
		{"key '", "'"},
		{"apikey: ", ""},
		{"apikey ", ""},
		{"api_key: ", ""},
	}
	lower := strings.ToLower(msg)
	for _, p := range patterns {
		idx := strings.Index(lower, p.prefix)
		if idx < 0 {
			continue
		}
		start := idx + len(p.prefix)
		var end int
		if p.suffix != "" {
			end = strings.Index(msg[start:], p.suffix)
			if end < 0 {
				continue
			}
			end = start + end + len(p.suffix)
		} else {
			// up to next space, quote, or end
			remain := msg[start:]
			spaceIdx := strings.IndexAny(remain, " \"'.,;:!?)\n\r")
			if spaceIdx > 0 {
				end = start + spaceIdx
			} else if strings.TrimSpace(remain) != "" {
				end = len(msg)
			} else {
				continue
			}
		}
		keyPart := msg[start:end]
		if len(keyPart) > 3 {
			masked := keyPart[:max(1, len(keyPart)/4)] + strings.Repeat("*", len(keyPart)-max(1, len(keyPart)/4))
			msg = msg[:start] + masked + msg[end:]
		}
		break
	}
	return msg
}

