package provider

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestChatToAnthropicSystemAndMerge(t *testing.T) {
	body := []byte(`{"model":"m","messages":[
		{"role":"system","content":"sys"},
		{"role":"user","content":"hi"},
		{"role":"user","content":"again"},
		{"role":"assistant","content":"ok","tool_calls":[{"id":"c1","type":"function","function":{"name":"f","arguments":"{\"a\":1}"}}]},
		{"role":"tool","tool_call_id":"c1","content":"res"}
	]}`)
	out, err := TranslateChatToAnthropic(body, false)
	if err != nil {
		t.Fatal(err)
	}
	var v struct {
		System   string `json:"system"`
		Messages []struct {
			Role    string `json:"role"`
			Content []struct {
				Type    string `json:"type"`
				ToolUse string `json:"id"`
				ToolID  string `json:"tool_use_id"`
			} `json:"content"`
		} `json:"messages"`
		MaxTokens int `json:"max_tokens"`
	}
	if err := json.Unmarshal(out, &v); err != nil {
		t.Fatal(err)
	}
	if v.System != "sys" || v.MaxTokens != 4096 {
		t.Fatalf("system/max_tokens: %+v", v)
	}
	if len(v.Messages) != 3 {
		t.Fatalf("consecutive user messages must merge, got %d", len(v.Messages))
	}
	if len(v.Messages[1].Content) != 2 || v.Messages[1].Content[1].ToolUse != "c1" {
		t.Fatalf("tool_use not attached: %+v", v.Messages[1])
	}
	if v.Messages[2].Role != "user" || v.Messages[2].Content[0].ToolID != "c1" {
		t.Fatalf("tool result not mapped: %+v", v.Messages[2])
	}
}

func TestAnthropicToChatObject(t *testing.T) {
	body := []byte(`{"id":"msg_1","model":"m","stop_reason":"tool_use","content":[
		{"type":"text","text":"hi"},
		{"type":"tool_use","id":"t1","name":"f","input":{"a":1}}
	],"usage":{"input_tokens":4,"output_tokens":8}}`)
	out, err := TranslateAnthropicToChatObject(body)
	if err != nil {
		t.Fatal(err)
	}
	var v struct {
		Choices []struct {
			Message struct {
				Content   string `json:"content"`
				ToolCalls []struct {
					ID       string `json:"id"`
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens      int `json:"total_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(out, &v); err != nil {
		t.Fatal(err)
	}
	if v.Choices[0].FinishReason != "tool_calls" || v.Choices[0].Message.ToolCalls[0].ID != "t1" {
		t.Fatalf("tool_calls mapping: %+v", v.Choices[0])
	}
	if v.Usage.TotalTokens != 12 {
		t.Fatalf("usage total = %+v", v.Usage)
	}
}

func TestChatToAnthropicObject(t *testing.T) {
	body := []byte(`{"model":"m","choices":[{"message":{"role":"assistant","content":"done"},"finish_reason":"length"}],"usage":{"prompt_tokens":3,"completion_tokens":5}}`)
	out, err := TranslateChatToAnthropicObject(body, "upstream-m")
	if err != nil {
		t.Fatal(err)
	}
	var v struct {
		Model      string `json:"model"`
		StopReason string `json:"stop_reason"`
		Content    []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(out, &v); err != nil {
		t.Fatal(err)
	}
	if v.Model != "upstream-m" || v.StopReason != "max_tokens" || v.Content[0].Text != "done" {
		t.Fatalf("object mapping: %+v", v)
	}
}

func TestStreamTranslatorsRoundTrip(t *testing.T) {
	a2c := NewAnthropicToChatTranslator("m")
	var frames [][]byte
	for _, ev := range [][2]string{
		{"message_start", `{"type":"message_start","message":{"model":"m","usage":{"input_tokens":4,"output_tokens":0}}}`},
		{"content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hel"}}`},
		{"content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"lo"}}`},
		{"message_delta", `{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":3}}`},
	} {
		frames = append(frames, a2c.FeedEvent(ev[0], []byte(ev[1]))...)
	}
	frames = append(frames, a2c.Finish()...)
	joined := ""
	for _, f := range frames {
		joined += string(f)
	}
	if !strings.Contains(joined, `"content":"hel"`) || !strings.Contains(joined, `"finish_reason":"stop"`) || !strings.HasSuffix(strings.TrimSpace(joined), "data: [DONE]") {
		t.Fatalf("anthropic->chat stream wrong:\n%s", joined)
	}

	c2a := NewChatToAnthropicTranslator("m")
	var out [][]byte
	for _, chunk := range []string{
		`{"model":"m","choices":[{"delta":{"role":"assistant","content":"hi"}}]}`,
		`{"model":"m","choices":[{"delta":{},"finish_reason":"stop"}],"usage":{"completion_tokens":2}}`,
	} {
		out = append(out, c2a.FeedChunk([]byte(chunk))...)
	}
	joined = ""
	for _, f := range out {
		joined += string(f)
	}
	for _, want := range []string{"event: message_start", "event: content_block_delta", `"stop_reason":"end_turn"`, "event: message_stop"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("chat->anthropic stream missing %q:\n%s", want, joined)
		}
	}
}

func TestUsageCollector(t *testing.T) {
	c := NewSSEUsageCollector("openai")
	c.Observe([]byte(`{"model":"m","choices":[]}`))
	c.Observe([]byte(`{"usage":{"prompt_tokens":4,"completion_tokens":8,"total_tokens":12}}`))
	u := c.Usage()
	if u.Input == nil || *u.Input != 4 || u.Total == nil || *u.Total != 12 {
		t.Fatalf("usage = %+v", u)
	}
	if m, ok := c.FirstModel(); !ok || m != "m" {
		t.Fatalf("first model = %q, %v", m, ok)
	}
	if _, ok := c.FirstByte(); !ok {
		t.Fatal("first byte missing")
	}
	empty := NewSSEUsageCollector("openai")
	if !empty.Usage().Empty() {
		t.Fatal("empty collector must report empty usage")
	}
	if _, ok := ExtractUpstreamModel([]byte(`{"model":"spark-x"}`)); !ok {
		t.Fatal("model extract failed")
	}
}
