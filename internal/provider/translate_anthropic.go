package provider

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const AnthropicUpstreamPath = "v1/messages"

const defaultAnthropicMaxTokens = 4096

func TranslateChatToAnthropic(body []byte, stream bool) ([]byte, error) {
	var in chatRequest
	if err := json.Unmarshal(body, &in); err != nil {
		return nil, fmt.Errorf("decode chat request: %w", err)
	}
	out := map[string]any{
		"model":      in.Model,
		"max_tokens": defaultAnthropicMaxTokens,
		"stream":     stream,
	}
	var system []string
	messages := make([]any, 0, len(in.Messages))
	flush := func(role string, blocks []any) {
		if len(blocks) == 0 {
			return
		}
		if n := len(messages); n > 0 {
			if prev, ok := messages[n-1].(map[string]any); ok && prev["role"] == role {
				prev["content"] = append(prev["content"].([]any), blocks...)
				return
			}
		}
		messages = append(messages, map[string]any{"role": role, "content": blocks})
	}
	for _, msg := range in.Messages {
		role, _ := msg["role"].(string)
		switch role {
		case "system", "developer":
			if text := instructionsText(msg["content"]); text != "" {
				system = append(system, text)
			}
		case "user", "assistant":
			blocks := chatContentToAnthropic(msg["content"])
			for _, tc := range toolCallsOf(msg) {
				blocks = append(blocks, map[string]any{
					"type": "tool_use", "id": tc.id, "name": tc.name, "input": toolArguments(tc.arguments),
				})
			}
			flush(role, blocks)
		case "tool":
			flush("user", []any{map[string]any{
				"type": "tool_result", "tool_use_id": stringOf(msg["tool_call_id"]),
				"content": toolResultContent(msg["content"]),
			}})
		}
	}
	if len(system) > 0 {
		out["system"] = strings.Join(system, "\n")
	}
	out["messages"] = messages
	if len(in.Tools) > 0 {
		tools := make([]any, 0, len(in.Tools))
		for _, tool := range in.Tools {
			fn, _ := tool["function"].(map[string]any)
			if fn == nil {
				fn = tool
			}
			name := strings.TrimSpace(stringOf(fn["name"]))
			if name == "" {
				continue
			}
			schema, _ := fn["parameters"].(map[string]any)
			if schema == nil {
				schema = map[string]any{"type": "object", "properties": map[string]any{}}
			}
			decl := map[string]any{"name": name, "input_schema": schema}
			if desc := stringOf(fn["description"]); desc != "" {
				decl["description"] = desc
			}
			tools = append(tools, decl)
		}
		if len(tools) > 0 {
			out["tools"] = tools
		}
	}
	if in.ToolChoice != nil {
		out["tool_choice"] = chatToolChoiceToAnthropic(in.ToolChoice)
	}
	if in.Temperature != nil {
		out["temperature"] = *in.Temperature
	}
	if in.TopP != nil {
		out["top_p"] = *in.TopP
	}
	if in.MaxTokens != nil {
		out["max_tokens"] = *in.MaxTokens
	} else if in.MaxCompletionTokens != nil {
		out["max_tokens"] = *in.MaxCompletionTokens
	} else if in.MaxOutputTokens != nil {
		out["max_tokens"] = *in.MaxOutputTokens
	}
	if stop := chatStopToAnthropic(in.Stop); stop != nil {
		out["stop_sequences"] = stop
	}
	return json.Marshal(out)
}

func chatContentToAnthropic(content any) []any {
	var blocks []any
	addText := func(text string) {
		if text != "" {
			blocks = append(blocks, map[string]any{"type": "text", "text": text})
		}
	}
	switch c := content.(type) {
	case string:
		addText(c)
	case []any:
		for _, p := range c {
			m, ok := p.(map[string]any)
			if !ok {
				if s, ok := p.(string); ok {
					addText(s)
				}
				continue
			}
			switch stringOf(m["type"]) {
			case "text", "input_text", "output_text":
				addText(stringOf(m["text"]))
			case "image_url":
				if block := anthropicImageBlock(m["image_url"]); block != nil {
					blocks = append(blocks, block)
				}
			case "input_image":
				if url := stringOf(m["image_url"]); url != "" {
					if block := anthropicImageBlock(url); block != nil {
						blocks = append(blocks, block)
					}
				}
			default:
				addText(stringOf(m["text"]))
			}
		}
	}
	return blocks
}

func anthropicImageBlock(v any) map[string]any {
	var url string
	switch u := v.(type) {
	case string:
		url = u
	case map[string]any:
		url = stringOf(u["url"])
	}
	if url == "" {
		return nil
	}
	if rest, ok := strings.CutPrefix(url, "data:"); ok {
		meta, data, _ := strings.Cut(rest, ",")
		media, _, _ := strings.Cut(meta, ";")
		if data == "" {
			return nil
		}
		if media == "" {
			media = "image/png"
		}
		return map[string]any{"type": "image", "source": map[string]any{
			"type": "base64", "media_type": media, "data": data,
		}}
	}
	return map[string]any{"type": "image", "source": map[string]any{"type": "url", "url": url}}
}

func toolArguments(args string) any {
	var v any
	if args != "" {
		if err := json.Unmarshal([]byte(args), &v); err == nil {
			if _, ok := v.(map[string]any); ok {
				return v
			}
		}
	}
	return map[string]any{}
}

func toolResultContent(content any) any {
	if s, ok := content.(string); ok {
		return s
	}
	if parts, ok := content.([]any); ok {
		var texts []string
		for _, p := range parts {
			if m, ok := p.(map[string]any); ok {
				if s := stringOf(m["text"]); s != "" {
					texts = append(texts, s)
				}
			} else if s, ok := p.(string); ok && s != "" {
				texts = append(texts, s)
			}
		}
		if len(texts) > 0 {
			return strings.Join(texts, "\n")
		}
	}
	if encoded, err := json.Marshal(content); err == nil {
		return string(encoded)
	}
	return ""
}

func chatToolChoiceToAnthropic(choice any) any {
	if s, ok := choice.(string); ok {
		switch s {
		case "required":
			return map[string]any{"type": "any"}
		case "none", "auto":
			return map[string]any{"type": s}
		default:
			return map[string]any{"type": "auto"}
		}
	}
	if m, ok := choice.(map[string]any); ok {
		if fn, ok := m["function"].(map[string]any); ok {
			if name := stringOf(fn["name"]); name != "" {
				return map[string]any{"type": "tool", "name": name}
			}
		}
	}
	return map[string]any{"type": "auto"}
}

func chatStopToAnthropic(stop any) []string {
	switch s := stop.(type) {
	case string:
		if s != "" {
			return []string{s}
		}
	case []any:
		var out []string
		for _, v := range s {
			if str, ok := v.(string); ok && str != "" {
				out = append(out, str)
			}
		}
		if len(out) > 0 {
			return out
		}
	}
	return nil
}

type anthropicRequest struct {
	Model       string           `json:"model"`
	System      any              `json:"system"`
	Messages    []map[string]any `json:"messages"`
	Tools       []map[string]any `json:"tools"`
	ToolChoice  any              `json:"tool_choice"`
	MaxTokens   *int             `json:"max_tokens"`
	Temperature *float64         `json:"temperature"`
	TopP        *float64         `json:"top_p"`
	Stop        any              `json:"stop_sequences"`
	Metadata    any              `json:"metadata"`
	Stream      bool             `json:"stream"`
}

func TranslateAnthropicToChat(body []byte, stream bool) ([]byte, error) {
	var in anthropicRequest
	if err := json.Unmarshal(body, &in); err != nil {
		return nil, fmt.Errorf("decode anthropic request: %w", err)
	}
	out := map[string]any{"model": in.Model, "stream": stream}
	if text := anthropicSystemText(in.System); text != "" {
		out["messages"] = append([]any{}, map[string]any{"role": "system", "content": text})
	}
	messages, _ := out["messages"].([]any)
	if messages == nil {
		messages = []any{}
	}
	for _, msg := range in.Messages {
		role, _ := msg["role"].(string)
		if role != "user" && role != "assistant" {
			continue
		}
		var parts []any
		var toolCalls []any
		var reasoning []string
		var toolMsgs []any
		var texts []string
		for _, b := range anthropicContentBlocks(msg["content"]) {
			switch stringOf(b["type"]) {
			case "text":
				if s := stringOf(b["text"]); s != "" {
					texts = append(texts, s)
				}
			case "image":
				if part := chatImagePart(b["source"]); part != nil {
					parts = append(parts, part)
				}
			case "tool_use":
				input := b["input"]
				args := "{}"
				if input != nil {
					if encoded, err := json.Marshal(input); err == nil {
						args = string(encoded)
					}
				}
				id := stringOf(b["id"])
				if id == "" {
					id = "call_" + randomHexID()
				}
				name := strings.TrimSpace(stringOf(b["name"]))
				if name == "" {
					continue
				}
				toolCalls = append(toolCalls, map[string]any{
					"id": id, "type": "function",
					"function": map[string]any{"name": name, "arguments": args},
				})
			case "tool_result":
				content := toolResultContent(b["content"])
				str := stringOf(content)
				if str == "" {
					if encoded, err := json.Marshal(content); err == nil {
						str = string(encoded)
					}
				}
				toolMsgs = append(toolMsgs, map[string]any{
					"role": "tool", "tool_call_id": stringOf(b["tool_use_id"]), "content": str,
				})
			case "thinking", "redacted_thinking":
				if s := stringOf(b["thinking"]); s != "" {
					reasoning = append(reasoning, s)
				}
			}
		}
		for _, text := range texts {
			parts = append(parts, map[string]any{"type": "text", "text": text})
		}
		if len(parts) == 0 && len(toolCalls) == 0 && len(reasoning) == 0 && len(toolMsgs) == 0 {
			continue
		}
		if len(parts) > 0 || len(toolCalls) > 0 || len(reasoning) > 0 {
			chatMsg := map[string]any{"role": role}
			switch {
			case len(parts) == 0:
				chatMsg["content"] = nil
			case len(parts) == 1 && stringOf(parts[0].(map[string]any)["type"]) == "text":
				chatMsg["content"] = stringOf(parts[0].(map[string]any)["text"])
			default:
				chatMsg["content"] = parts
			}
			if len(toolCalls) > 0 {
				chatMsg["tool_calls"] = toolCalls
			}
			if len(reasoning) > 0 {
				chatMsg["reasoning_content"] = strings.Join(reasoning, "\n")
			}
			messages = append(messages, chatMsg)
		}
		messages = append(messages, toolMsgs...)
	}
	out["messages"] = messages
	if len(in.Tools) > 0 {
		tools := make([]any, 0, len(in.Tools))
		for _, tool := range in.Tools {
			name := strings.TrimSpace(stringOf(tool["name"]))
			if name == "" {
				continue
			}
			schema, _ := tool["input_schema"].(map[string]any)
			if schema == nil {
				schema = map[string]any{"type": "object", "properties": map[string]any{}}
			}
			decl := map[string]any{"type": "function", "function": map[string]any{
				"name": name, "parameters": schema,
			}}
			if desc := stringOf(tool["description"]); desc != "" {
				decl["function"].(map[string]any)["description"] = desc
			}
			tools = append(tools, decl)
		}
		if len(tools) > 0 {
			out["tools"] = tools
		}
	}
	if in.ToolChoice != nil {
		out["tool_choice"] = anthropicToolChoiceToChat(in.ToolChoice)
	}
	if in.MaxTokens != nil {
		out["max_tokens"] = *in.MaxTokens
	}
	if in.Temperature != nil {
		out["temperature"] = *in.Temperature
	}
	if in.TopP != nil {
		out["top_p"] = *in.TopP
	}
	if stop := chatStopToAnthropic(in.Stop); stop != nil {
		out["stop"] = stop
	}
	return json.Marshal(out)
}

func anthropicSystemText(system any) string {
	switch s := system.(type) {
	case string:
		return s
	case []any:
		var parts []string
		for _, b := range s {
			if m, ok := b.(map[string]any); ok {
				if t := stringOf(m["text"]); t != "" {
					parts = append(parts, t)
				}
			}
		}
		return strings.Join(parts, "\n")
	default:
		return ""
	}
}

func anthropicContentBlocks(content any) []map[string]any {
	switch c := content.(type) {
	case string:
		if c == "" {
			return nil
		}
		return []map[string]any{{"type": "text", "text": c}}
	case []any:
		var out []map[string]any
		for _, b := range c {
			if m, ok := b.(map[string]any); ok {
				out = append(out, m)
			}
		}
		return out
	default:
		return nil
	}
}

func chatImagePart(source any) map[string]any {
	m, ok := source.(map[string]any)
	if !ok {
		return nil
	}
	switch stringOf(m["type"]) {
	case "base64":
		media := stringOf(m["media_type"])
		if media == "" {
			media = "image/png"
		}
		return map[string]any{"type": "image_url", "image_url": map[string]any{
			"url": "data:" + media + ";base64," + stringOf(m["data"]),
		}}
	case "url":
		if url := stringOf(m["url"]); url != "" {
			return map[string]any{"type": "image_url", "image_url": map[string]any{"url": url}}
		}
	}
	return nil
}

func anthropicToolChoiceToChat(choice any) any {
	m, ok := choice.(map[string]any)
	if !ok {
		return "auto"
	}
	switch stringOf(m["type"]) {
	case "any":
		return "required"
	case "tool":
		if name := stringOf(m["name"]); name != "" {
			return map[string]any{"type": "function", "function": map[string]any{"name": name}}
		}
		return "auto"
	case "none", "auto":
		return stringOf(m["type"])
	default:
		return "auto"
	}
}

func TranslateAnthropicToChatObject(data []byte) ([]byte, error) {
	var in struct {
		ID         string           `json:"id"`
		Model      string           `json:"model"`
		StopReason *string          `json:"stop_reason"`
		Content    []map[string]any `json:"content"`
		Usage      *struct {
			InputTokens  *int64 `json:"input_tokens"`
			OutputTokens *int64 `json:"output_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(data, &in); err != nil {
		return nil, fmt.Errorf("decode anthropic message: %w", err)
	}
	var texts, reasoning []string
	var toolCalls []any
	for _, b := range in.Content {
		switch stringOf(b["type"]) {
		case "text":
			if s := stringOf(b["text"]); s != "" {
				texts = append(texts, s)
			}
		case "thinking", "redacted_thinking":
			if s := stringOf(b["thinking"]); s != "" {
				reasoning = append(reasoning, s)
			}
		case "tool_use":
			input := b["input"]
			args := "{}"
			if input != nil {
				if encoded, err := json.Marshal(input); err == nil {
					args = string(encoded)
				}
			}
			toolCalls = append(toolCalls, map[string]any{
				"id": stringOf(b["id"]), "type": "function",
				"function": map[string]any{"name": stringOf(b["name"]), "arguments": args},
			})
		}
	}
	message := map[string]any{"role": "assistant", "content": strings.Join(texts, "")}
	if len(toolCalls) > 0 {
		if len(texts) == 0 {
			message["content"] = nil
		}
		message["tool_calls"] = toolCalls
	}
	if len(reasoning) > 0 {
		message["reasoning_content"] = strings.Join(reasoning, "\n")
	}
	finish := "stop"
	if len(toolCalls) > 0 {
		finish = "tool_calls"
	} else if in.StopReason != nil {
		switch *in.StopReason {
		case "max_tokens":
			finish = "length"
		case "tool_use":
			finish = "tool_calls"
		}
	}
	out := map[string]any{
		"id": "chatcmpl-" + randomHexID(), "object": "chat.completion",
		"created": time.Now().Unix(), "model": in.Model,
		"choices": []any{map[string]any{"index": 0, "message": message, "finish_reason": finish}},
	}
	if in.Usage != nil {
		var total *int64
		if in.Usage.InputTokens != nil && in.Usage.OutputTokens != nil {
			sum := *in.Usage.InputTokens + *in.Usage.OutputTokens
			total = &sum
		}
		out["usage"] = map[string]any{
			"prompt_tokens": in.Usage.InputTokens, "completion_tokens": in.Usage.OutputTokens, "total_tokens": total,
		}
	}
	return json.Marshal(out)
}

func TranslateChatToAnthropicObject(data []byte, model string) ([]byte, error) {
	var in struct {
		Model   string `json:"model"`
		Choices []struct {
			Message struct {
				Content   any `json:"content"`
				ToolCalls []struct {
					ID       string `json:"id"`
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"message"`
			FinishReason *string `json:"finish_reason"`
		} `json:"choices"`
		Usage *struct {
			PromptTokens     *int64 `json:"prompt_tokens"`
			CompletionTokens *int64 `json:"completion_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(data, &in); err != nil {
		return nil, fmt.Errorf("decode chat completion: %w", err)
	}
	if len(in.Choices) == 0 {
		return nil, fmt.Errorf("decode chat completion: no choices")
	}
	choice := in.Choices[0]
	var content []any
	if text := messageText(choice.Message.Content); text != "" {
		content = append(content, map[string]any{"type": "text", "text": text})
	}
	for _, tc := range choice.Message.ToolCalls {
		content = append(content, map[string]any{
			"type": "tool_use", "id": tc.ID, "name": tc.Function.Name, "input": toolArguments(tc.Function.Arguments),
		})
	}
	if content == nil {
		content = []any{}
	}
	stop := "end_turn"
	if choice.FinishReason != nil {
		switch *choice.FinishReason {
		case "length":
			stop = "max_tokens"
		case "tool_calls":
			stop = "tool_use"
		case "content_filter":
			stop = "refusal"
		}
	}
	if model == "" {
		model = in.Model
	}
	out := map[string]any{
		"id": "msg_" + randomHexID(), "type": "message", "role": "assistant", "model": model,
		"content": content, "stop_reason": stop, "stop_sequence": nil,
	}
	if in.Usage != nil {
		out["usage"] = map[string]any{
			"input_tokens": in.Usage.PromptTokens, "output_tokens": in.Usage.CompletionTokens,
		}
	}
	return json.Marshal(out)
}
