package provider

import (
	"encoding/json"
	"strings"
	"testing"
)

func decodeJSON(t *testing.T, data []byte) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return out
}

func TestChatToAnthropicFilePart(t *testing.T) {
	body := []byte(`{"model":"m","messages":[{"role":"user","content":[
		{"type":"text","text":"read this"},
		{"type":"file","file":{"file_data":"data:application/pdf;base64,QUJD","filename":"doc.pdf"}}
	]}]}`)
	out, err := TranslateChatToAnthropic(body, false)
	if err != nil {
		t.Fatal(err)
	}
	decoded := decodeJSON(t, out)
	msgs := decoded["messages"].([]any)
	content := msgs[0].(map[string]any)["content"].([]any)
	found := false
	for _, b := range content {
		m := b.(map[string]any)
		if m["type"] == "document" {
			found = true
			src := m["source"].(map[string]any)
			if src["type"] != "base64" || src["media_type"] != "application/pdf" || src["data"] != "QUJD" {
				t.Fatalf("bad document source: %v", src)
			}
		}
	}
	if !found {
		t.Fatalf("document block missing: %s", out)
	}
}

func TestChatToAnthropicToolResultImage(t *testing.T) {
	body := []byte(`{"model":"m","messages":[
		{"role":"assistant","tool_calls":[{"id":"c1","type":"function","function":{"name":"shot","arguments":"{}"}}]},
		{"role":"tool","tool_call_id":"c1","content":[
			{"type":"text","text":"screenshot"},
			{"type":"image_url","image_url":{"url":"data:image/png;base64,aW1n"}}
		]}
	]}`)
	out, err := TranslateChatToAnthropic(body, false)
	if err != nil {
		t.Fatal(err)
	}
	decoded := decodeJSON(t, out)
	msgs := decoded["messages"].([]any)
	var result map[string]any
	for _, m := range msgs {
		blk := m.(map[string]any)
		if blk["role"] == "user" {
			for _, b := range blk["content"].([]any) {
				if b.(map[string]any)["type"] == "tool_result" {
					result = b.(map[string]any)
				}
			}
		}
	}
	if result == nil {
		t.Fatalf("tool_result missing: %s", out)
	}
	parts := result["content"].([]any)
	hasImage := false
	for _, p := range parts {
		if p.(map[string]any)["type"] == "image" {
			hasImage = true
		}
	}
	if !hasImage {
		t.Fatalf("image block missing from tool_result: %v", result["content"])
	}
}

func TestAnthropicToChatDocumentAndToolResultImage(t *testing.T) {
	body := []byte(`{"model":"m","messages":[{"role":"user","content":[
		{"type":"document","source":{"type":"base64","media_type":"application/pdf","data":"REVG"}},
		{"type":"tool_result","tool_use_id":"t1","content":[
			{"type":"text","text":"see"},
			{"type":"image","source":{"type":"base64","media_type":"image/png","data":"aW1n"}}
		]}
	]}]}`)
	out, err := TranslateAnthropicToChat(body, false)
	if err != nil {
		t.Fatal(err)
	}
	text := string(out)
	if !strings.Contains(text, `"type":"file"`) || !strings.Contains(text, "data:application/pdf;base64,REVG") {
		t.Fatalf("file part missing: %s", text)
	}
	if !strings.Contains(text, `"type":"image_url"`) || !strings.Contains(text, "data:image/png;base64,aW1n") {
		t.Fatalf("tool-result image missing: %s", text)
	}
	decoded := decodeJSON(t, out)
	msgs := decoded["messages"].([]any)
	var toolMsg map[string]any
	for _, m := range msgs {
		blk := m.(map[string]any)
		if blk["role"] == "tool" {
			toolMsg = blk
		}
	}
	if toolMsg == nil {
		t.Fatalf("tool message missing: %s", text)
	}
	content, ok := toolMsg["content"].([]any)
	if !ok {
		t.Fatalf("tool message content should be an array when media present: %v", toolMsg["content"])
	}
	if len(content) != 2 {
		t.Fatalf("expected text+image parts, got %v", content)
	}
}

func TestAnthropicToChatToolResultTextOnlyStaysString(t *testing.T) {
	body := []byte(`{"model":"m","messages":[{"role":"user","content":[
		{"type":"tool_result","tool_use_id":"t1","content":"plain output"}
	]}]}`)
	out, err := TranslateAnthropicToChat(body, false)
	if err != nil {
		t.Fatal(err)
	}
	decoded := decodeJSON(t, out)
	for _, m := range decoded["messages"].([]any) {
		blk := m.(map[string]any)
		if blk["role"] == "tool" {
			if s, ok := blk["content"].(string); !ok || s != "plain output" {
				t.Fatalf("expected string content, got %v", blk["content"])
			}
			return
		}
	}
	t.Fatal("tool message missing")
}

func TestChatToResponsesFileAndAudio(t *testing.T) {
	body := []byte(`{"model":"m","messages":[{"role":"user","content":[
		{"type":"text","text":"hi"},
		{"type":"file","file":{"file_data":"data:application/pdf;base64,QUJD","filename":"a.pdf"}},
		{"type":"input_audio","input_audio":{"data":"QUJD","format":"wav"}}
	]}]}`)
	out, err := TranslateChatToResponses(body, false)
	if err != nil {
		t.Fatal(err)
	}
	text := string(out)
	if !strings.Contains(text, `"type":"input_file"`) || !strings.Contains(text, "data:application/pdf;base64,QUJD") {
		t.Fatalf("input_file missing: %s", text)
	}
	if !strings.Contains(text, `"type":"input_audio"`) {
		t.Fatalf("input_audio dropped: %s", text)
	}
}
