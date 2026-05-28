package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestGenerateCodexConfigToml(t *testing.T) {
	app := NewApp()
	app.proxy.listenAddress = "http://127.0.0.1:11434"

	got, err := app.GenerateCodexConfigToml()
	if err != nil {
		t.Fatalf("GenerateCodexConfigToml returned error: %v", err)
	}
	if !strings.Contains(got, `openai_base_url = 'http://127.0.0.1:11434/v1/'`) {
		t.Fatalf("unexpected config content: %s", got)
	}
	if !strings.Contains(got, `model_provider = 'openai'`) {
		t.Fatalf("missing model_provider: %s", got)
	}
}

func TestMergeCodexConfigTomlPreservesExistingKeys(t *testing.T) {
	existing := []byte(`
approval_policy = "on-request"

[tools]
web_search = "disabled"

[model_providers.keepme]
name = "Keep Me"
base_url = "https://example.com/v1"
env_key = "KEEP_ME"
wire_api = "chat"
`)
	merged, err := mergeCodexConfigToml(existing, "http://127.0.0.1:11434/v1", "deepseek-v4-flash")
	if err != nil {
		t.Fatalf("mergeCodexConfigToml returned error: %v", err)
	}
	if !bytes.Contains(merged, []byte(`approval_policy = 'on-request'`)) {
		t.Fatalf("expected approval_policy preserved, got: %s", string(merged))
	}
	if !bytes.Contains(merged, []byte(`[model_providers.keepme]`)) {
		t.Fatalf("expected keepme provider preserved, got: %s", string(merged))
	}
	if bytes.Contains(merged, []byte(`[model_providers.Local]`)) {
		t.Fatalf("Local provider should have been removed, got: %s", string(merged))
	}
}
