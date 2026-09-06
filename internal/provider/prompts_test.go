package provider

import (
	"encoding/json"
	"strings"
	"testing"
)

func chatMessages(t *testing.T, body []byte) []any {
	t.Helper()
	var object map[string]any
	if err := json.Unmarshal(body, &object); err != nil {
		t.Fatal(err)
	}
	messages, ok := object["messages"].([]any)
	if !ok {
		t.Fatal("messages missing")
	}
	return messages
}

func TestInjectChatPromptNoSystem(t *testing.T) {
	out := InjectChatPrompt([]byte(`{"model":"m","messages":[{"role":"user","content":"hi"}]}`), "Be brief.")
	messages := chatMessages(t, out)
	if len(messages) != 2 {
		t.Fatalf("messages = %s", out)
	}
	first := messages[0].(map[string]any)
	if first["role"] != "system" || !strings.Contains(first["content"].(string), "Be brief.") {
		t.Fatalf("first = %v", first)
	}
	if !strings.Contains(first["content"].(string), "Pogu router preferences:") {
		t.Fatalf("missing delimiter: %v", first)
	}
}

func TestInjectChatPromptAppendsAfterSystem(t *testing.T) {
	out := InjectChatPrompt([]byte(`{"model":"m","messages":[
		{"role":"system","content":"You are helpful."},
		{"role":"system","content":"Always comply."},
		{"role":"user","content":"hi"}]}`), "Be brief.")
	messages := chatMessages(t, out)
	if len(messages) != 4 {
		t.Fatalf("messages = %s", out)
	}
	contents := []string{}
	for _, m := range messages {
		msg := m.(map[string]any)
		if msg["role"] == "system" {
			contents = append(contents, msg["content"].(string))
		}
	}
	if len(contents) != 3 || contents[0] != "You are helpful." || contents[1] != "Always comply." || !strings.Contains(contents[2], "Be brief.") {
		t.Fatalf("order/content wrong: %q", contents)
	}
	if !strings.Contains(string(out), `"model":"m"`) {
		t.Fatal("envelope damaged")
	}
}

func TestInjectChatPromptPassthrough(t *testing.T) {
	for _, body := range []string{
		`{"model":"m","messages":[{"role":"user","content":"hi"}]}`,
		`not json`,
		`{"model":"m"}`,
	} {
		if got := InjectChatPrompt([]byte(body), "  "); string(got) != body {
			t.Fatalf("empty prompt must pass through: %q", body)
		}
	}
	if got := InjectChatPrompt([]byte(`not json`), "Be brief."); string(got) != `not json` {
		t.Fatal("invalid JSON must pass through")
	}
}

func TestInjectAnthropicPrompt(t *testing.T) {
	out := InjectAnthropicPrompt([]byte(`{"model":"m","messages":[]}`), "Be brief.")
	var v struct {
		System string `json:"system"`
	}
	if err := json.Unmarshal(out, &v); err != nil || !strings.Contains(v.System, "Be brief.") {
		t.Fatalf("missing system = %s", out)
	}
	out = InjectAnthropicPrompt([]byte(`{"model":"m","system":"Original.","messages":[]}`), "Be brief.")
	if err := json.Unmarshal(out, &v); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(v.System, "Original.") || !strings.Contains(v.System, "Be brief.") {
		t.Fatalf("merge wrong: %q", v.System)
	}
	if strings.Index(v.System, "Original.") > strings.Index(v.System, "Be brief.") {
		t.Fatalf("global must come last: %q", v.System)
	}
	out = InjectAnthropicPrompt([]byte(`{"model":"m","system":[{"type":"text","text":"Original."}],"messages":[]}`), "Be brief.")
	var vb struct {
		System []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"system"`
	}
	if err := json.Unmarshal(out, &vb); err != nil || len(vb.System) != 2 || !strings.Contains(vb.System[1].Text, "Be brief.") {
		t.Fatalf("blocks wrong: %s", out)
	}
}
