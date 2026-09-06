package provider

import (
	"encoding/json"
	"fmt"
	"strings"
)

func TranslateChatToResponses(body []byte, stream bool) ([]byte, error) {
	var in chatRequest
	if err := json.Unmarshal(body, &in); err != nil {
		return nil, fmt.Errorf("decode chat request: %w", err)
	}
	out := map[string]any{
		"model":  in.Model,
		"stream": stream,
		"store":  false,
	}
	if in.Input != nil {
		var raw map[string]any
		if err := json.Unmarshal(body, &raw); err != nil {
			return nil, fmt.Errorf("decode chat request: %w", err)
		}
		raw["stream"] = stream
		raw["store"] = false
		normalizeResponsesTokenBudget(raw)
		return json.Marshal(raw)
	}
	input := make([]any, 0, len(in.Messages)+2)
	instructionsSet := false
	for _, msg := range in.Messages {
		role, _ := msg["role"].(string)
		switch role {
		case "system", "developer":
			if !instructionsSet {
				out["instructions"] = instructionsText(msg["content"])
				instructionsSet = true
				continue
			}
			input = append(input, map[string]any{
				"type": "message", "role": "developer",
				"content": []any{map[string]any{"type": "input_text", "text": messageText(msg["content"])}},
			})
		case "user", "assistant":
			if role == "assistant" {
				if item := reasoningInputItem(msg); item != nil {
					input = append(input, item)
				}
			}
			kind := "input_text"
			if role == "assistant" {
				kind = "output_text"
			}
			if parts := contentParts(msg["content"], kind); len(parts) > 0 {
				input = append(input, map[string]any{"type": "message", "role": role, "content": parts})
			}
			if role == "assistant" {
				for _, tc := range toolCallsOf(msg) {
					if tc.name == "" {
						continue
					}
					input = append(input, map[string]any{
						"type": "function_call", "call_id": tc.id,
						"name": tc.name, "arguments": tc.arguments,
					})
				}
			}
		case "tool":
			output := msg["content"]
			if s, ok := output.(string); !ok || s == "" {
				encoded, _ := json.Marshal(output)
				output = string(encoded)
			}
			input = append(input, map[string]any{
				"type":    "function_call_output",
				"call_id": stringOf(msg["tool_call_id"]),
				"output":  output,
			})
		}
	}
	if !instructionsSet {
		out["instructions"] = ""
	}
	out["input"] = input
	if len(in.Tools) > 0 {
		tools := make([]any, 0, len(in.Tools))
		for _, tool := range in.Tools {
			if converted := responsesTool(tool); converted != nil {
				tools = append(tools, converted)
			}
		}
		if len(tools) > 0 {
			out["tools"] = tools
		}
	}
	if in.ToolChoice != nil {
		out["tool_choice"] = responsesToolChoice(in.ToolChoice)
	}
	if in.Temperature != nil {
		out["temperature"] = *in.Temperature
	}
	if in.TopP != nil {
		out["top_p"] = *in.TopP
	}
	if budget := tokenBudget(in.MaxOutputTokens, in.MaxCompletionTokens, in.MaxTokens); budget != nil {
		out["max_output_tokens"] = *budget
	}
	switch {
	case in.ReasoningEffort != nil:
		out["reasoning"] = map[string]any{"effort": clampEffort(*in.ReasoningEffort), "summary": "auto"}
	case in.Reasoning != nil:
		out["reasoning"] = in.Reasoning
	}
	if in.ServiceTier != nil {
		out["service_tier"] = *in.ServiceTier
	}
	if format := responsesTextFormat(in.ResponseFormat); format != nil {
		out["text"] = map[string]any{"format": format}
	}
	return json.Marshal(out)
}

func tokenBudget(output, completion, legacy *int) *int {
	if output != nil {
		return output
	}
	if completion != nil {
		return completion
	}
	return legacy
}

func normalizeResponsesTokenBudget(raw map[string]any) {
	if _, ok := raw["max_output_tokens"]; ok {
		return
	}
	for _, key := range []string{"max_completion_tokens", "max_tokens"} {
		if v, ok := raw[key].(float64); ok {
			raw["max_output_tokens"] = v
		}
		delete(raw, key)
	}
}

func responsesTextFormat(format any) map[string]any {
	m, ok := format.(map[string]any)
	if !ok {
		return nil
	}
	switch stringOf(m["type"]) {
	case "json_schema":
		schema, _ := m["json_schema"].(map[string]any)
		converted := map[string]any{
			"type":   "json_schema",
			"name":   stringOf(schema["name"]),
			"schema": schema["schema"],
		}
		if strict, ok := schema["strict"]; ok {
			converted["strict"] = strict
		}
		if converted["name"] == "" || converted["schema"] == nil {
			return nil
		}
		return converted
	case "json_object":
		return map[string]any{"type": "json_object"}
	default:
		return nil
	}
}

func clampEffort(effort string) string {
	switch strings.ToLower(strings.TrimSpace(effort)) {
	case "max", "ultra":
		return "xhigh"
	default:
		return effort
	}
}

func contentParts(content any, kind string) []any {
	switch c := content.(type) {
	case string:
		if c == "" {
			return nil
		}
		return []any{map[string]any{"type": kind, "text": c}}
	case []any:
		var parts []any
		for _, p := range c {
			m, ok := p.(map[string]any)
			if !ok {
				if s, ok := p.(string); ok && s != "" {
					parts = append(parts, map[string]any{"type": kind, "text": s})
				}
				continue
			}
			switch stringOf(m["type"]) {
			case "text":
				parts = append(parts, map[string]any{"type": kind, "text": stringOf(m["text"])})
			case "image_url":
				url, detail := "", "auto"
				switch v := m["image_url"].(type) {
				case string:
					url = v
				case map[string]any:
					url = stringOf(v["url"])
					if d := stringOf(v["detail"]); d != "" {
						detail = d
					}
				}
				parts = append(parts, map[string]any{"type": "input_image", "image_url": url, "detail": detail})
			case "input_image":
				parts = append(parts, m)
			default:
				text := stringOf(m["text"])
				if text == "" {
					text = stringOf(m["content"])
				}
				if text == "" {
					if encoded, err := json.Marshal(m); err == nil {
						text = string(encoded)
					}
				}
				parts = append(parts, map[string]any{"type": kind, "text": text})
			}
		}
		return parts
	default:
		return nil
	}
}

func reasoningInputItem(msg map[string]any) map[string]any {
	if s := strings.TrimSpace(stringOf(msg["reasoning_content"])); s != "" {
		return map[string]any{"type": "reasoning", "summary": []any{map[string]any{"type": "summary_text", "text": s}}}
	}
	return nil
}

func responsesTool(tool map[string]any) map[string]any {
	if fn, ok := tool["function"].(map[string]any); ok {
		tool = map[string]any{"type": "function", "name": fn["name"], "description": fn["description"], "parameters": fn["parameters"], "strict": fn["strict"]}
	}
	name := strings.TrimSpace(stringOf(tool["name"]))
	if name == "" {
		return nil
	}
	params, _ := tool["parameters"].(map[string]any)
	if params == nil {
		params = map[string]any{"type": "object", "properties": map[string]any{}}
	} else if params["type"] == "object" {
		if _, ok := params["properties"]; !ok {
			params["properties"] = map[string]any{}
		}
	}
	converted := map[string]any{
		"type": "function", "name": name,
		"description": stringOf(tool["description"]), "parameters": params,
	}
	if strict, ok := tool["strict"]; ok {
		converted["strict"] = strict
	}
	return converted
}

func responsesToolChoice(choice any) any {
	if s, ok := choice.(string); ok {
		return s
	}
	if m, ok := choice.(map[string]any); ok {
		if fn, ok := m["function"].(map[string]any); ok {
			if name := stringOf(fn["name"]); name != "" {
				return map[string]any{"type": "function", "name": name}
			}
		}
		return m
	}
	return choice
}

func TranslateResponsesToChat(data []byte) ([]byte, error) {
	var in struct {
		ID     string `json:"id"`
		Model  string `json:"model"`
		Status string `json:"status"`
		Error  *struct {
			Message string `json:"message"`
		} `json:"error"`
		Output []struct {
			Type    string `json:"type"`
			Role    string `json:"role"`
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
			Summary []struct {
				Text string `json:"text"`
			} `json:"summary"`
			CallID    string `json:"call_id"`
			Name      string `json:"name"`
			Arguments any    `json:"arguments"`
			Input     any    `json:"input"`
		} `json:"output"`
		Usage *struct {
			InputTokens      *int64 `json:"input_tokens"`
			PromptTokens     *int64 `json:"prompt_tokens"`
			OutputTokens     *int64 `json:"output_tokens"`
			CompletionTokens *int64 `json:"completion_tokens"`
			TotalTokens      *int64 `json:"total_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(data, &in); err != nil {
		return nil, fmt.Errorf("decode responses object: %w", err)
	}
	if in.Error != nil && in.Error.Message != "" {
		return nil, fmt.Errorf("upstream responses error: %s", in.Error.Message)
	}
	var texts, reasoning []string
	var toolCalls []any
	for _, item := range in.Output {
		switch item.Type {
		case "message":
			for _, c := range item.Content {
				if c.Type == "output_text" && c.Text != "" {
					texts = append(texts, c.Text)
				}
			}
		case "reasoning":
			for _, s := range item.Summary {
				if s.Text != "" {
					reasoning = append(reasoning, s.Text)
				}
			}
		case "function_call", "custom_tool_call":
			if strings.TrimSpace(item.Name) == "" {
				continue
			}
			callID := item.CallID
			if callID == "" {
				callID = "call_" + randomHexID()
			}
			toolCalls = append(toolCalls, map[string]any{
				"id": callID, "type": "function",
				"function": map[string]any{"name": item.Name, "arguments": argumentsString(item.Arguments, item.Input)},
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
	switch {
	case len(toolCalls) > 0:
		finish = "tool_calls"
	case in.Status == "incomplete":
		finish = "length"
	}
	out := map[string]any{
		"id": in.ID, "object": "chat.completion", "created": nowUnix(),
		"model":   in.Model,
		"choices": []any{map[string]any{"index": 0, "message": message, "finish_reason": finish}},
	}
	if in.Usage != nil {
		prompt := firstNonNil(in.Usage.InputTokens, in.Usage.PromptTokens)
		completion := firstNonNil(in.Usage.OutputTokens, in.Usage.CompletionTokens)
		total := in.Usage.TotalTokens
		if total == nil && prompt != nil && completion != nil {
			sum := *prompt + *completion
			total = &sum
		}
		out["usage"] = map[string]any{
			"prompt_tokens": prompt, "completion_tokens": completion, "total_tokens": total,
		}
	}
	if out["id"] == "" {
		out["id"] = "chatcmpl-" + randomHexID()
	}
	return json.Marshal(out)
}
