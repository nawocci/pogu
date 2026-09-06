package provider

import (
	"encoding/json"
	"time"
)

type ResponsesToChatTranslator struct {
	model            string
	id               string
	created          int64
	roleSent         bool
	toolIndex        int
	toolByItem       map[string]int
	argsEmitted      map[int]bool
	promptTokens     *int64
	completionTokens *int64
	finished         bool
}

func NewResponsesToChatTranslator(model string) *ResponsesToChatTranslator {
	return &ResponsesToChatTranslator{
		model:       model,
		id:          "chatcmpl-" + randomHexID(),
		created:     time.Now().Unix(),
		toolByItem:  make(map[string]int),
		argsEmitted: make(map[int]bool),
	}
}

type responsesEvent struct {
	Type   string `json:"type"`
	Delta  string `json:"delta"`
	ItemID string `json:"item_id"`
	Item   *struct {
		ID        string `json:"id"`
		Type      string `json:"type"`
		CallID    string `json:"call_id"`
		Name      string `json:"name"`
		Arguments any    `json:"arguments"`
		Input     any    `json:"input"`
	} `json:"item"`
	Response *struct {
		Model string `json:"model"`
		Usage *struct {
			InputTokens      *int64 `json:"input_tokens"`
			PromptTokens     *int64 `json:"prompt_tokens"`
			OutputTokens     *int64 `json:"output_tokens"`
			CompletionTokens *int64 `json:"completion_tokens"`
		} `json:"usage"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	} `json:"response"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

func usagePair(u *responsesEvent) (*int64, *int64) {
	if u.Response == nil || u.Response.Usage == nil {
		return nil, nil
	}
	usage := u.Response.Usage
	input := usage.InputTokens
	if input == nil {
		input = usage.PromptTokens
	}
	output := usage.OutputTokens
	if output == nil {
		output = usage.CompletionTokens
	}
	return input, output
}

func (t *ResponsesToChatTranslator) FeedEvent(event string, data []byte) [][]byte {
	if t.finished {
		return nil
	}
	var evt responsesEvent
	if err := json.Unmarshal(data, &evt); err != nil {
		return nil
	}
	typ := event
	if evt.Type != "" {
		typ = evt.Type
	}
	if t.model == "" && evt.Response != nil && evt.Response.Model != "" {
		t.model = evt.Response.Model
	}
	switch typ {
	case "response.output_text.delta":
		if evt.Delta == "" {
			return nil
		}
		return [][]byte{t.chunk(t.delta(map[string]any{"content": evt.Delta}), nil)}
	case "response.output_text.done":
		return nil
	case "response.output_item.added":
		if evt.Item == nil || (evt.Item.Type != "function_call" && evt.Item.Type != "custom_tool_call") {
			return nil
		}
		key := evt.Item.ID
		if key == "" {
			key = evt.ItemID
		}
		if key == "" {
			key = evt.Item.CallID
		}
		idx, ok := t.toolByItem[key]
		if !ok {
			idx = t.toolIndex
			t.toolIndex++
			if key != "" {
				t.toolByItem[key] = idx
			}
		}
		callID := evt.Item.CallID
		if callID == "" {
			callID = "call_" + randomHexID()
		}
		return [][]byte{t.chunk(t.delta(map[string]any{
			"tool_calls": []any{map[string]any{
				"index": idx, "id": callID, "type": "function",
				"function": map[string]any{"name": evt.Item.Name, "arguments": ""},
			}},
		}), nil)}
	case "response.function_call_arguments.delta", "response.custom_tool_call_input.delta":
		if evt.Delta == "" {
			return nil
		}
		idx := t.toolIndex - 1
		if known, ok := t.toolByItem[evt.ItemID]; ok {
			idx = known
		}
		if idx < 0 {
			idx = 0
		}
		t.argsEmitted[idx] = true
		return [][]byte{t.chunk(t.delta(map[string]any{
			"tool_calls": []any{map[string]any{
				"index": idx, "function": map[string]any{"arguments": evt.Delta},
			}},
		}), nil)}
	case "response.output_item.done":
		if evt.Item == nil || (evt.Item.Type != "function_call" && evt.Item.Type != "custom_tool_call") {
			return nil
		}
		key := evt.Item.ID
		if key == "" {
			key = evt.ItemID
		}
		idx := t.toolIndex - 1
		if known, ok := t.toolByItem[key]; ok {
			idx = known
		}
		if idx < 0 {
			idx = 0
		}
		if t.argsEmitted[idx] {
			return nil
		}
		args := argumentsString(evt.Item.Arguments, evt.Item.Input)
		if args == "" {
			return nil
		}
		t.argsEmitted[idx] = true
		return [][]byte{t.chunk(t.delta(map[string]any{
			"tool_calls": []any{map[string]any{
				"index": idx, "function": map[string]any{"arguments": args},
			}},
		}), nil)}
	case "response.completed", "response.done", "response.incomplete":
		if input, output := usagePair(&evt); input != nil || output != nil {
			t.promptTokens, t.completionTokens = input, output
		}
		return t.terminal()
	case "error", "response.failed":
		message := ""
		if evt.Error != nil {
			message = evt.Error.Message
		} else if evt.Response != nil && evt.Response.Error != nil {
			message = evt.Response.Error.Message
		}
		if message == "" {
			if raw, err := json.Marshal(evt); err == nil {
				message = string(raw)
			}
		}
		t.finished = true
		stop := "stop"
		return [][]byte{
			t.chunk(t.delta(map[string]any{"content": "[Error] " + message}), &stop),
			[]byte("data: [DONE]\n\n"),
		}
	case "response.reasoning_summary_text.delta":
		if evt.Delta == "" {
			return nil
		}
		return [][]byte{t.chunk(t.delta(map[string]any{"reasoning_content": evt.Delta}), nil)}
	default:
		return nil
	}
}

func (t *ResponsesToChatTranslator) Finish() [][]byte {
	if t.finished {
		return nil
	}
	return t.terminal()
}

func (t *ResponsesToChatTranslator) terminal() [][]byte {
	t.finished = true
	reason := "stop"
	if t.toolIndex > 0 {
		reason = "tool_calls"
	}
	frames := [][]byte{t.finalChunk(reason)}
	return append(frames, []byte("data: [DONE]\n\n"))
}

func (t *ResponsesToChatTranslator) delta(delta map[string]any) map[string]any {
	if !t.roleSent {
		t.roleSent = true
		delta["role"] = "assistant"
	}
	return delta
}

func (t *ResponsesToChatTranslator) chunk(delta map[string]any, finish *string) []byte {
	choice := map[string]any{"index": 0, "delta": delta, "finish_reason": nil}
	if finish != nil {
		choice["finish_reason"] = *finish
	}
	payload, _ := json.Marshal(map[string]any{
		"id": t.id, "object": "chat.completion.chunk", "created": t.created,
		"model": t.model, "choices": []any{choice},
	})
	return []byte("data: " + string(payload) + "\n\n")
}

func (t *ResponsesToChatTranslator) finalChunk(reason string) []byte {
	choice := map[string]any{"index": 0, "delta": map[string]any{}, "finish_reason": reason}
	message := map[string]any{
		"id": t.id, "object": "chat.completion.chunk", "created": t.created,
		"model": t.model, "choices": []any{choice},
	}
	if t.promptTokens != nil || t.completionTokens != nil {
		var total *int64
		if t.promptTokens != nil && t.completionTokens != nil {
			sum := *t.promptTokens + *t.completionTokens
			total = &sum
		}
		message["usage"] = map[string]any{
			"prompt_tokens": t.promptTokens, "completion_tokens": t.completionTokens, "total_tokens": total,
		}
	}
	payload, _ := json.Marshal(message)
	return []byte("data: " + string(payload) + "\n\n")
}

func argumentsString(primary, fallback any) string {
	if s, ok := primary.(string); ok {
		return s
	}
	if s, ok := fallback.(string); ok {
		return s
	}
	for _, v := range []any{primary, fallback} {
		if v == nil {
			continue
		}
		if encoded, err := json.Marshal(v); err == nil {
			return string(encoded)
		}
	}
	return ""
}

func firstNonNil(values ...*int64) *int64 {
	for _, v := range values {
		if v != nil {
			return v
		}
	}
	return nil
}
