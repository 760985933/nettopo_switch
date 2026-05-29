package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

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

func mapFinishReason(fr string) string {
	switch fr {
	case "stop":
		return "end_turn"
	case "length":
		return "max_tokens"
	case "tool_calls":
		return "tool_use"
	default:
		return fr
	}
}

// translateMessagesToChatCompletions converts an Anthropic Messages API request to Chat Completions.
func translateMessagesToChatCompletions(body []byte, cfg AppConfig) ([]byte, bool, string, error) {
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, false, "", errors.New("请求体不是有效的 JSON")
	}

	streaming := false
	if raw, ok := payload["stream"]; ok {
		if b, ok := raw.(bool); ok {
			streaming = b
		}
	}

	model := strings.TrimSpace(cfg.DefaultModel)
	if raw, ok := payload["model"].(string); ok && strings.TrimSpace(raw) != "" {
		if mapped, ok := cfg.Mappings[raw]; ok && strings.TrimSpace(mapped) != "" {
			model = mapped
		}
	}
	payload["model"] = model
	messages, _ := payload["messages"].([]any)
	if messages == nil {
		messages = []any{}
	}

	// Move system field to a system message at the beginning.
	// Handle array format first (newer Anthropic API), then string format.
	if sysArr, ok := payload["system"].([]any); ok {
		var texts []string
		for _, item := range sysArr {
			if m, ok := item.(map[string]any); ok {
				if t, ok := m["text"].(string); ok {
					texts = append(texts, t)
				}
			}
		}
		if len(texts) > 0 {
			systemMsg := map[string]any{"role": "system", "content": strings.Join(texts, "\n")}
			messages = append([]any{systemMsg}, messages...)
		}
	}
	if sys, ok := payload["system"].(string); ok && strings.TrimSpace(sys) != "" {
		systemMsg := map[string]any{"role": "system", "content": sys}
		messages = append([]any{systemMsg}, messages...)
	}
	delete(payload, "system")

	// Convert Messages API content blocks to Chat Completions format
	for i, item := range messages {
		msg, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if contentArr, ok := msg["content"].([]any); ok {
			var texts []string
			hasImage := false
			chatParts := make([]any, 0, len(contentArr))
			for _, block := range contentArr {
				b, ok := block.(map[string]any)
				if !ok {
					continue
				}
				switch b["type"] {
				case "image":
					hasImage = true
					source, _ := b["source"].(map[string]any)
					if source != nil {
						mediaType, _ := source["media_type"].(string)
						data, _ := source["data"].(string)
						chatParts = append(chatParts, map[string]any{
							"type": "image_url",
							"image_url": map[string]any{"url": "data:" + mediaType + ";base64," + data},
						})
					}
				case "text":
					t, _ := b["text"].(string)
					chatParts = append(chatParts, map[string]any{
						"type": "text",
						"text": t,
					})
					texts = append(texts, t)
				default:
					if t, ok := b["text"].(string); ok {
						chatParts = append(chatParts, map[string]any{
							"type": "text",
							"text": t,
						})
						texts = append(texts, t)
					}
				}
			}
			if hasImage {
				msg["content"] = chatParts
			} else if len(texts) > 0 {
				msg["content"] = strings.Join(texts, "\n")
			}
		}
		messages[i] = msg
	}
	payload["messages"] = messages

	// Rename max_tokens (Messages API) — keep as-is for Chat Completions
	if _, ok := payload["max_tokens"]; !ok {
		payload["max_tokens"] = 4096
	}

	// Remove Messages-specific fields
	delete(payload, "anthropic_version")
	delete(payload, "thinking")

	translatedBody, err := json.Marshal(payload)
	if err != nil {
		return nil, false, "", err
	}
	return translatedBody, streaming, model, nil
}

// translateChatCompletionToMessages converts a Chat Completions response to Messages API format.
func translateChatCompletionToMessages(body []byte, model string) (map[string]any, error) {
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}

	respID := fmt.Sprintf("msg_%d", time.Now().UnixNano())
	text := extractChatCompletionText(payload)
	stopReason := ""
	if choices, ok := payload["choices"].([]any); ok && len(choices) > 0 {
		if first, ok := choices[0].(map[string]any); ok {
			if fr, ok := first["finish_reason"].(string); ok {
				stopReason = mapFinishReason(fr)
			}
		}
	}

	inputTokens := 0
	outputTokens := 0
	if usage, ok := payload["usage"].(map[string]any); ok {
		if ct, ok := usage["completion_tokens"].(float64); ok {
			outputTokens = int(ct)
		}
	}

	content := []any{map[string]any{"type": "text", "text": text}}

	// Check for tool calls
	toolCalls := extractChatCompletionToolCalls(payload)
	for _, tc := range toolCalls {
		content = append(content, map[string]any{
			"type":  "tool_use",
			"id":    tc.ID,
			"name":  tc.Name,
			"input": tc.Arguments,
		})
	}

	return map[string]any{
		"id":      respID,
		"type":    "message",
		"role":    "assistant",
		"content": content,
		"model":   model,
		"stop_reason":    stopReason,
		"stop_sequence":  nil,
		"usage": map[string]any{
			"input_tokens":  inputTokens,
			"output_tokens": outputTokens,
		},
	}, nil
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
		if mapped, ok := cfg.Mappings[rawModel]; ok && strings.TrimSpace(mapped) != "" {
			model = mapped
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
		callIDsInInput := map[string]bool{}
		outputsByCallID := map[string]string{}
		for _, item := range typed {
			msg, ok := item.(map[string]any)
			if !ok {
				continue
			}
			t, _ := msg["type"].(string)
			t = strings.TrimSpace(t)
			switch t {
			case "function_call":
				callID, _ := msg["call_id"].(string)
				if callID == "" {
					callID, _ = msg["id"].(string)
				}
				if callID != "" {
					callIDsInInput[callID] = true
				}
			case "function_call_output":
				callID, _ := msg["call_id"].(string)
				if callID == "" {
					callID, _ = msg["tool_call_id"].(string)
				}
				if callID == "" {
					callID, _ = msg["id"].(string)
				}
				if callID == "" {
					continue
				}
				output := strings.TrimSpace(flattenAnyText(msg["output"]))
				if output == "" {
					output = strings.TrimSpace(flattenResponsesContent(msg["content"]))
				}
				if output == "" {
					output = strings.TrimSpace(flattenResponsesContent(msg))
				}
				outputsByCallID[callID] = output
			}
		}

		consumedOutputs := map[string]bool{}
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
				case "input_image":
					imageURL := extractImageURL(msg["image_url"])
					chatParts := []any{map[string]any{
						"type": "image_url",
						"image_url": map[string]any{"url": imageURL},
					}}
					messages = append(messages, map[string]any{
						"role":    "user",
						"content": chatParts,
					})
					continue
					case "input_audio":
						audioData, _ := msg["input_audio"].(map[string]any)
						chatParts := []any{map[string]any{
							"type":        "input_audio",
							"input_audio": audioData,
						}}
						messages = append(messages, map[string]any{
							"role":    "user",
							"content": chatParts,
						})
						continue
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
					if callID == "" {
					continue
					}
					if callIDsInInput[callID] {
					continue
					}
					messages = append(messages, map[string]any{
						"role":         "tool",
						"tool_call_id": callID,
						"content":      outputsByCallID[callID],
					})
					continue
				case "function_call":
					callID, _ := msg["call_id"].(string)
					if callID == "" {
						callID, _ = msg["id"].(string)
					}
					if callID == "" {
					continue
					}
					toolOutput, hasOutput := outputsByCallID[callID]
					if !hasOutput {
					continue
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
					if strings.TrimSpace(name) == "" && strings.TrimSpace(arguments) == "" {
					continue
					}

					messages = append(messages, map[string]any{
						"role":    "assistant",
						"content": nil,
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
					messages = append(messages, map[string]any{
						"role":         "tool",
						"tool_call_id": callID,
						"content":      toolOutput,
					})
					consumedOutputs[callID] = true
					continue
				}
			}

			role, _ := msg["role"].(string)
			role = normalizeChatRole(role)

			content := convertResponsesContentToChatContent(msg["content"])
			if contentStr, ok := content.(string); ok {
				if strings.TrimSpace(contentStr) == "" {
					content = flattenResponsesContent(msg)
				}
			}
			reasoning := ""
			if role == "assistant" {
				reasoning = strings.TrimSpace(extractResponsesReasoningContent(msg))
			}
			contentStr, isStr := content.(string)
			if isStr && strings.TrimSpace(contentStr) == "" && reasoning == "" {
				continue
			}
			// content 是数组（含图片等媒体）时，即使 reasoning 为空也保留
			if !isStr {
				if contentArr, ok := content.([]any); ok && len(contentArr) == 0 && reasoning == "" {
					continue
				}
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
		content := convertResponsesContentToChatContent(typed["content"])
		if s, ok := content.(string); ok && strings.TrimSpace(s) == "" {
			content = flattenResponsesContent(typed)
		}
		reasoning := ""
		if role == "assistant" {
			reasoning = strings.TrimSpace(extractResponsesReasoningContent(typed))
		}
		s, isStr := content.(string)
		if (isStr && strings.TrimSpace(s) != "") || reasoning != "" || !isStr {
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

