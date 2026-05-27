package main

import (
	"encoding/json"
	"testing"
)

func TestTranslateChatCompletionsUsesMapping(t *testing.T) {
	cfg := defaultConfig()
	cfg.Mappings["gpt-4.1"] = "deepseek-v4-flash"

	body := []byte(`{"model":"gpt-4.1","messages":[{"role":"user","content":"hello"}],"stream":true}`)
	translated, err := translateChatCompletions(body, cfg)
	if err != nil {
		t.Fatalf("translateChatCompletions returned error: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(translated, &payload); err != nil {
		t.Fatalf("unmarshal translated body failed: %v", err)
	}

	if payload["model"] != "deepseek-v4-flash" {
		t.Fatalf("expected mapped model deepseek-v4-flash, got %v", payload["model"])
	}
}

func TestUpstreamResourceURLNormalizesBasePath(t *testing.T) {
	got, err := upstreamResourceURL("https://api.deepseek.com/v1", "chat/completions")
	if err != nil {
		t.Fatalf("upstreamResourceURL returned error: %v", err)
	}

	want := "https://api.deepseek.com/v1/chat/completions"
	if got != want {
		t.Fatalf("expected %s, got %s", want, got)
	}
}

func TestUpstreamResourceURLAddsV1ForBareDomain(t *testing.T) {
	got, err := upstreamResourceURL("https://api.custom.com", "chat/completions")
	if err != nil {
		t.Fatalf("upstreamResourceURL returned error: %v", err)
	}

	want := "https://api.custom.com/v1/chat/completions"
	if got != want {
		t.Fatalf("expected %s, got %s", want, got)
	}
}

func TestUpstreamResourceURLPreservesNonV1Path(t *testing.T) {
	got, err := upstreamResourceURL("https://open.bigmodel.cn/api/paas/v4", "chat/completions")
	if err != nil {
		t.Fatalf("upstreamResourceURL returned error: %v", err)
	}

	want := "https://open.bigmodel.cn/api/paas/v4/chat/completions"
	if got != want {
		t.Fatalf("expected %s, got %s", want, got)
	}
}

func TestTranslateResponsesToChatCompletionsMapsToolsAndToolOutputs(t *testing.T) {
	cfg := defaultConfig()
	cfg.Mappings["gpt-4.1"] = "deepseek-v4-flash"

	body := []byte(`{
		"model":"gpt-4.1",
		"input":[
			{"type":"message","role":"user","content":[{"type":"input_text","text":"hi"}]},
			{"type":"function_call_output","call_id":"call_1","output":"Python 3.11"}
		],
		"tools":[
			{"type":"function","name":"bash","description":"run shell","parameters":{"type":"object","properties":{}}},
			{"type":"custom","name":"custom_tool","parameters":{"type":"object","properties":{}}},
			{"type":"namespace","name":"ignored","tools":[{"type":"function","name":"fs_read","parameters":{"type":"object","properties":{}}}]}
		],
		"tool_choice":"auto"
	}`)

	translated, _, _, err := translateResponsesToChatCompletions(body, cfg)
	if err != nil {
		t.Fatalf("translateResponsesToChatCompletions returned error: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(translated, &payload); err != nil {
		t.Fatalf("unmarshal translated body failed: %v", err)
	}

	if payload["model"] != "deepseek-v4-flash" {
		t.Fatalf("expected mapped model deepseek-v4-flash, got %v", payload["model"])
	}
	if payload["tool_choice"] != "auto" {
		t.Fatalf("expected tool_choice auto, got %v", payload["tool_choice"])
	}
	if _, ok := payload["tools"]; !ok {
		t.Fatalf("expected tools to be present")
	}
	if toolsAny, ok := payload["tools"].([]any); !ok || len(toolsAny) != 3 {
		t.Fatalf("expected 3 tools after normalization, got %T len=%d", payload["tools"], len(toolsAny))
	} else {
		for _, toolAny := range toolsAny {
			tool, ok := toolAny.(map[string]any)
			if !ok {
				t.Fatalf("expected tool to be object")
			}
			if tool["type"] != "function" {
				t.Fatalf("expected tool type function, got %v", tool["type"])
			}
			if _, ok := tool["function"].(map[string]any); !ok {
				t.Fatalf("expected tool.function to be present")
			}
		}
	}

	msgs, ok := payload["messages"].([]any)
	if !ok || len(msgs) < 2 {
		t.Fatalf("expected at least 2 messages, got %T len=%d", payload["messages"], len(msgs))
	}
	last, ok := msgs[len(msgs)-1].(map[string]any)
	if !ok {
		t.Fatalf("expected last message to be object")
	}
	if last["role"] != "tool" {
		t.Fatalf("expected tool role, got %v", last["role"])
	}
	if last["tool_call_id"] != "call_1" {
		t.Fatalf("expected tool_call_id call_1, got %v", last["tool_call_id"])
	}
	if last["content"] != "Python 3.11" {
		t.Fatalf("expected tool output content, got %v", last["content"])
	}
}

func TestTranslateChatCompletionToResponsesMapsToolCalls(t *testing.T) {
	body := []byte(`{
		"choices":[
			{"message":{
				"role":"assistant",
				"content":null,
				"reasoning_content":"secret",
				"tool_calls":[
					{"id":"call_1","type":"function","function":{"name":"bash","arguments":"{\"cmd\":\"python3 --version\"}"}}
				]
			}}
		],
		"usage":{"prompt_tokens":1,"completion_tokens":2,"total_tokens":3}
	}`)

	resp, err := translateChatCompletionToResponses(body, "deepseek-v4-flash")
	if err != nil {
		t.Fatalf("translateChatCompletionToResponses returned error: %v", err)
	}

	if resp["status"] != "requires_action" {
		t.Fatalf("expected status requires_action, got %v", resp["status"])
	}
	outputAny, ok := resp["output"].([]any)
	if !ok || len(outputAny) != 2 {
		t.Fatalf("expected 2 output items (message + function_call), got %T len=%d", resp["output"], len(outputAny))
	}
	first, ok := outputAny[0].(map[string]any)
	if !ok || first["type"] != "message" {
		t.Fatalf("expected first item to be message, got %v", outputAny[0])
	}
	if first["reasoning_content"] != "secret" {
		t.Fatalf("expected reasoning_content secret, got %v", first["reasoning_content"])
	}
	item, ok := outputAny[1].(map[string]any)
	if !ok || item["type"] != "function_call" {
		t.Fatalf("expected second item to be function_call, got %v", outputAny[1])
	}
	if item["call_id"] != "call_1" {
		t.Fatalf("expected call_id call_1, got %v", item["call_id"])
	}
	if item["name"] != "bash" {
		t.Fatalf("expected name bash, got %v", item["name"])
	}
	if item["arguments"] != `{"cmd":"python3 --version"}` {
		t.Fatalf("expected arguments, got %v", item["arguments"])
	}
	if _, ok := resp["required_action"]; !ok {
		t.Fatalf("expected required_action to be present")
	}
	if usageAny, ok := resp["usage"].(map[string]any); !ok || usageAny["total_tokens"] != float64(3) {
		t.Fatalf("expected usage to be mapped, got %v", resp["usage"])
	}
}

func TestResponsesInputToMessagesPassesReasoningContent(t *testing.T) {
	payload := map[string]any{
		"input": []any{
			map[string]any{
				"type":              "message",
				"role":              "assistant",
				"content":           []any{map[string]any{"type": "output_text", "text": "hi"}},
				"reasoning_content": "hidden",
			},
		},
	}
	msgs, err := responsesInputToMessages(payload)
	if err != nil {
		t.Fatalf("responsesInputToMessages returned error: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	m, ok := msgs[0].(map[string]any)
	if !ok {
		t.Fatalf("expected message to be object")
	}
	if m["role"] != "assistant" {
		t.Fatalf("expected assistant role, got %v", m["role"])
	}
	if m["reasoning_content"] != "hidden" {
		t.Fatalf("expected reasoning_content hidden, got %v", m["reasoning_content"])
	}
}
