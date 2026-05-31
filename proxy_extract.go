package main

import (
	"encoding/json"
	"strings"
)

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

// convertResponsesContentToChatContent converts Responses API content (string or array of parts)
// to Chat Completions format. When non-text media (images, audio) is present, returns an array
// of content parts; otherwise returns a plain string. visionSupported controls whether image
// content blocks are included (unsupported providers will have them silently dropped).
func convertResponsesContentToChatContent(content any, visionSupported bool) any {
	s, ok := content.(string)
	if ok {
		return s
	}
	parts, ok := content.([]any)
	if !ok {
		return flattenResponsesContent(content)
	}
	hasMedia := false
	chatParts := make([]any, 0, len(parts))
	for _, item := range parts {
		p, ok := item.(map[string]any)
		if !ok {
			continue
		}
		t, _ := p["type"].(string)
		switch t {
		case "input_image":
			if visionSupported {
				hasMedia = true
				imageURL := extractImageURL(p["image_url"])
				chatParts = append(chatParts, map[string]any{
					"type": "image_url",
					"image_url": map[string]any{
						"url": imageURL,
					},
				})
			}
		case "input_audio":
			hasMedia = true
			audioData, _ := p["input_audio"].(map[string]any)
			chatParts = append(chatParts, map[string]any{
				"type":        "input_audio",
				"input_audio": audioData,
			})
		case "input_text":
			text, _ := p["text"].(string)
			chatParts = append(chatParts, map[string]any{
				"type": "text",
				"text": text,
			})
		default:
			if text := flattenAnyText(p["text"]); text != "" {
				chatParts = append(chatParts, map[string]any{
					"type": "text",
					"text": text,
				})
			}
		}
	}
	if hasMedia {
		return chatParts
	}
	var texts []string
	for _, cp := range chatParts {
		if m, ok := cp.(map[string]any); ok {
			if t, ok := m["text"].(string); ok {
				texts = append(texts, t)
			}
		}
	}
	return strings.Join(texts, "")
}

// extractImageURL handles both string and object formats for the image_url field.
// The Responses API allows image_url as a plain URL string or as {url, detail}.
func extractImageURL(value any) string {
	if s, ok := value.(string); ok && strings.TrimSpace(s) != "" {
		return s
	}
	if m, ok := value.(map[string]any); ok {
		if url, ok := m["url"].(string); ok && strings.TrimSpace(url) != "" {
			return url
		}
	}
	return ""
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

func extractUsage(body []byte) (map[string]int64, bool) {
	var payload struct {
		Usage *struct {
			PromptTokens     int64 `json:"prompt_tokens"`
			CompletionTokens int64 `json:"completion_tokens"`
			TotalTokens      int64 `json:"total_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(body, &payload); err == nil && payload.Usage != nil {
		return map[string]int64{
			"prompt_tokens":     payload.Usage.PromptTokens,
			"completion_tokens": payload.Usage.CompletionTokens,
			"total_tokens":      payload.Usage.TotalTokens,
		}, true
	}
	return nil, false
}

func extractGoogleUsage(body []byte) (promptTokens, completionTokens, totalTokens int64) {
	var payload struct {
		UsageMetadata *struct {
			PromptTokenCount     float64 `json:"promptTokenCount"`
			CandidatesTokenCount float64 `json:"candidatesTokenCount"`
			TotalTokenCount      float64 `json:"totalTokenCount"`
		} `json:"usageMetadata"`
	}
	if err := json.Unmarshal(body, &payload); err == nil && payload.UsageMetadata != nil {
		return int64(payload.UsageMetadata.PromptTokenCount),
			int64(payload.UsageMetadata.CandidatesTokenCount),
			int64(payload.UsageMetadata.TotalTokenCount)
	}
	return 0, 0, 0
}

// extractMessagesUsage extracts token counts from Messages API format responses.
// Messages format: {"usage":{"input_tokens":X,"output_tokens":Y}} or Chat: {"usage":{"prompt_tokens":X,...}}
func extractMessagesUsage(body []byte) (promptTokens, completionTokens, totalTokens int64) {
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return 0, 0, 0
	}
	usage, ok := payload["usage"].(map[string]any)
	if !ok {
		return 0, 0, 0
	}
	if v, ok := usage["prompt_tokens"].(float64); ok {
		promptTokens = int64(v)
	} else if v, ok := usage["input_tokens"].(float64); ok {
		promptTokens = int64(v)
	}
	if v, ok := usage["completion_tokens"].(float64); ok {
		completionTokens = int64(v)
	} else if v, ok := usage["output_tokens"].(float64); ok {
		completionTokens = int64(v)
	}
	if v, ok := usage["total_tokens"].(float64); ok {
		totalTokens = int64(v)
	}
	return
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


