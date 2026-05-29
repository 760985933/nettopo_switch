package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"github.com/gorilla/websocket"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
)

func (b *ProxyRuntime) streamResponsesFailed(w http.ResponseWriter, errType string, message string) {
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

func (b *ProxyRuntime) streamMessagesError(w http.ResponseWriter, errType string, message string) {
	data, _ := json.Marshal(map[string]any{
		"type":  "error",
		"error": map[string]string{"type": errType, "message": message},
	})
	fmt.Fprintf(w, "event: error\ndata: %s\n\n", data)
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}

// streamChatToMessages converts a Chat Completions SSE stream to Messages API SSE events.
func (b *ProxyRuntime) streamChatToMessages(w http.ResponseWriter, body io.Reader, model string) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		return
	}

	respID := fmt.Sprintf("msg_%d", time.Now().UnixNano())
	var contentText strings.Builder
	var stopReason string
	var outputTokens int

	writeSSE := func(event string, data any) {
		payload, _ := json.Marshal(data)
		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, payload)
		flusher.Flush()
	}

	hasStarted := false
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

		choices, _ := chunk["choices"].([]any)
		if len(choices) == 0 {
			continue
		}
		first, _ := choices[0].(map[string]any)
		if first == nil {
			continue
		}

		delta, _ := first["delta"].(map[string]any)
		if !hasStarted {
			writeSSE("message_start", map[string]any{
				"type": "message_start",
				"message": map[string]any{
					"id":      respID,
					"type":    "message",
					"role":    "assistant",
					"content": []any{},
					"model":   model,
				},
			})
			writeSSE("content_block_start", map[string]any{
				"type":          "content_block_start",
				"index":         0,
				"content_block": map[string]any{"type": "text", "text": ""},
			})
			hasStarted = true
		}

		if delta != nil {
			if text, ok := delta["content"].(string); ok && text != "" {
				contentText.WriteString(text)
				writeSSE("content_block_delta", map[string]any{
					"type":  "content_block_delta",
					"index": 0,
					"delta": map[string]any{"type": "text_delta", "text": text},
				})
			}
		}

		if fr, ok := first["finish_reason"].(string); ok && fr != "" {
			stopReason = fr
		}

		if usage, ok := chunk["usage"].(map[string]any); ok {
			if ct, ok := usage["completion_tokens"].(float64); ok {
				outputTokens = int(ct)
			}
		}
	}

	if hasStarted {
		writeSSE("content_block_stop", map[string]any{
			"type":  "content_block_stop",
			"index": 0,
		})
		mappedReason := mapFinishReason(stopReason)
		writeSSE("message_delta", map[string]any{
			"type":  "message_delta",
			"delta": map[string]any{"stop_reason": mappedReason, "stop_sequence": nil},
			"usage": map[string]any{"output_tokens": outputTokens},
		})
		writeSSE("message_stop", map[string]any{"type": "message_stop"})
	}
}

func (b *ProxyRuntime) streamResponse(w http.ResponseWriter, body io.Reader) {
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

// streamPassthrough 同格式直通时的 SSE 流复制
func (b *ProxyRuntime) streamPassthrough(w http.ResponseWriter, body io.Reader) {
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

func (b *ProxyRuntime) streamChatToResponses(w http.ResponseWriter, body io.Reader, model string) (int, []string, int64, int64, int64) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		_, _ = io.Copy(w, body)
		return 0, nil, 0, 0, 0
	}

	seq := 1
	writeEvent := func(eventType string, data map[string]any) {
		data["type"] = eventType
		data["sequence_number"] = seq
		seq++
		payload, _ := json.Marshal(data)
		_, _ = w.Write([]byte("event: " + eventType + "\n"))
		_, _ = w.Write([]byte("data: " + string(payload) + "\n\n"))
		flusher.Flush()
	}

	return b.processChatStreamToResponses(body, model, writeEvent)
}

func (b *ProxyRuntime) streamChatToResponsesWS(conn *websocket.Conn, body io.Reader, model string) (int, []string, int64, int64, int64) {
	seq := 1
	writeEvent := func(eventType string, data map[string]any) {
		data["type"] = eventType
		data["sequence_number"] = seq
		seq++
		_ = conn.WriteJSON(data)
	}

	return b.processChatStreamToResponses(body, model, writeEvent)
}

func (b *ProxyRuntime) processChatStreamToResponses(body io.Reader, model string, writeEvent func(string, map[string]any)) (int, []string, int64, int64, int64) {
	respID := fmt.Sprintf("resp_%d", time.Now().UnixNano())
	itemID := fmt.Sprintf("msg_%d", time.Now().UnixNano())
	createdAt := time.Now().Unix()

	responseSkeleton := map[string]any{
		"id":         respID,
		"object":     "response",
		"created_at": createdAt,
		"model":      model,
		"status":     "in_progress",
		"output":     []any{},
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
	var promptTokens, completionTokens, totalTokens int64
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

		if usageRaw, ok := chunk["usage"].(map[string]any); ok {
			if pt, ok := usageRaw["prompt_tokens"].(float64); ok {
				promptTokens = int64(pt)
			}
			if ct, ok := usageRaw["completion_tokens"].(float64); ok {
				completionTokens = int64(ct)
			}
			if tt, ok := usageRaw["total_tokens"].(float64); ok {
				totalTokens = int64(tt)
			}
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

	return len(finalText), toolNames, promptTokens, completionTokens, totalTokens
}

