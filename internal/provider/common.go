package provider

import (
	"encoding/json"
	"strings"
	"time"
)

func nowUnix() int64 { return time.Now().Unix() }

type chatRequest struct {
	Model               string           `json:"model"`
	Messages            []map[string]any `json:"messages"`
	Tools               []map[string]any `json:"tools"`
	ToolChoice          any              `json:"tool_choice"`
	Temperature         *float64         `json:"temperature"`
	TopP                *float64         `json:"top_p"`
	MaxTokens           *int             `json:"max_tokens"`
	MaxCompletionTokens *int             `json:"max_completion_tokens"`
	MaxOutputTokens     *int             `json:"max_output_tokens"`
	ReasoningEffort     *string          `json:"reasoning_effort"`
	Reasoning           any              `json:"reasoning"`
	ServiceTier         *string          `json:"service_tier"`
	ResponseFormat      any              `json:"response_format"`
	Stop                any              `json:"stop"`
	Input               any              `json:"input"`
}

func stringOf(v any) string {
	s, _ := v.(string)
	return s
}

func instructionsText(content any) string {
	switch c := content.(type) {
	case string:
		return c
	case []any:
		var parts []string
		for _, p := range c {
			if m, ok := p.(map[string]any); ok {
				if s := stringOf(m["text"]); s != "" {
					parts = append(parts, s)
				} else if s := stringOf(m["content"]); s != "" {
					parts = append(parts, s)
				}
			} else if s, ok := p.(string); ok {
				parts = append(parts, s)
			}
		}
		return strings.Join(parts, "\n")
	default:
		return ""
	}
}

func messageText(content any) string {
	return instructionsText(content)
}

type chatToolCall struct {
	id        string
	name      string
	arguments string
}

func toolCallsOf(msg map[string]any) []chatToolCall {
	raw, ok := msg["tool_calls"].([]any)
	if !ok {
		return nil
	}
	var out []chatToolCall
	for _, item := range raw {
		tc, ok := item.(map[string]any)
		if !ok {
			continue
		}
		fn, _ := tc["function"].(map[string]any)
		name := strings.TrimSpace(stringOf(fn["name"]))
		if name == "" {
			continue
		}
		args := stringOf(fn["arguments"])
		if args == "" {
			args = "{}"
		}
		id := stringOf(tc["id"])
		if id == "" {
			id = "call_" + randomHexID()
		}
		out = append(out, chatToolCall{id: id, name: name, arguments: args})
	}
	return out
}

func upstreamModelName(body []byte) string {
	var envelope struct {
		Model string `json:"model"`
	}
	if json.Unmarshal(body, &envelope) != nil {
		return ""
	}
	return envelope.Model
}
