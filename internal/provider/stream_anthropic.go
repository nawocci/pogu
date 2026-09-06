package provider

import (
	"encoding/json"
	"fmt"
	"time"
)

type AnthropicToChatTranslator struct {
	model            string
	id               string
	created          int64
	roleSent         bool
	toolIndex        int
	finishReason     string
	toolByBlock      map[int]int
	toolStarted      map[int]bool
	promptTokens     *int64
	completionTokens *int64
	finished         bool
}

func NewAnthropicToChatTranslator(model string) *AnthropicToChatTranslator {
	return &AnthropicToChatTranslator{
		model:       model,
		id:          "chatcmpl-" + randomHexID(),
		created:     time.Now().Unix(),
		toolByBlock: make(map[int]int),
		toolStarted: make(map[int]bool),
	}
}

type anthropicEvent struct {
	Type    string `json:"type"`
	Index   *int   `json:"index"`
	Message *struct {
		Model string `json:"model"`
		Usage *struct {
			InputTokens  *int64 `json:"input_tokens"`
			OutputTokens *int64 `json:"output_tokens"`
		} `json:"usage"`
	} `json:"message"`
	ContentBlock *struct {
		Type string `json:"type"`
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"content_block"`
	Delta *struct {
		Type        string  `json:"type"`
		Text        string  `json:"text"`
		PartialJSON string  `json:"partial_json"`
		StopReason  *string `json:"stop_reason"`
	} `json:"delta"`
	Usage *struct {
		OutputTokens *int64 `json:"output_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

func (t *AnthropicToChatTranslator) FeedEvent(event string, data []byte) [][]byte {
	if t.finished {
		return nil
	}
	var evt anthropicEvent
	if err := json.Unmarshal(data, &evt); err != nil {
		return nil
	}
	typ := event
	if evt.Type != "" {
		typ = evt.Type
	}
	blockIndex := -1
	if evt.Index != nil {
		blockIndex = *evt.Index
	}
	switch typ {
	case "message_start":
		if evt.Message != nil {
			if t.model == "" && evt.Message.Model != "" {
				t.model = evt.Message.Model
			}
			if evt.Message.Usage != nil {
				t.promptTokens = evt.Message.Usage.InputTokens
			}
		}
		return nil
	case "content_block_start":
		if evt.ContentBlock == nil || evt.ContentBlock.Type != "tool_use" {
			return nil
		}
		idx := t.toolIndex
		if blockIndex >= 0 {
			if known, ok := t.toolByBlock[blockIndex]; ok {
				idx = known
			} else {
				t.toolByBlock[blockIndex] = idx
				t.toolIndex++
			}
		} else {
			t.toolIndex++
		}
		t.toolStarted[idx] = true
		callID := evt.ContentBlock.ID
		if callID == "" {
			callID = "call_" + randomHexID()
		}
		return [][]byte{t.chunk(t.delta(map[string]any{
			"tool_calls": []any{map[string]any{
				"index": idx, "id": callID, "type": "function",
				"function": map[string]any{"name": evt.ContentBlock.Name, "arguments": ""},
			}},
		}), nil)}
	case "content_block_delta":
		if evt.Delta == nil {
			return nil
		}
		switch evt.Delta.Type {
		case "text_delta":
			if evt.Delta.Text == "" {
				return nil
			}
			return [][]byte{t.chunk(t.delta(map[string]any{"content": evt.Delta.Text}), nil)}
		case "input_json_delta":
			if evt.Delta.PartialJSON == "" {
				return nil
			}
			idx := t.toolIndex - 1
			if known, ok := t.toolByBlock[blockIndex]; ok {
				idx = known
			}
			if idx < 0 {
				idx = 0
			}
			if !t.toolStarted[idx] {
				t.toolStarted[idx] = true
				return [][]byte{t.chunk(t.delta(map[string]any{
					"tool_calls": []any{map[string]any{
						"index": idx, "id": "call_" + randomHexID(), "type": "function",
						"function": map[string]any{"name": "", "arguments": ""},
					}},
				}), nil), t.chunk(map[string]any{
					"tool_calls": []any{map[string]any{
						"index": idx, "function": map[string]any{"arguments": evt.Delta.PartialJSON},
					}},
				}, nil)}
			}
			return [][]byte{t.chunk(t.delta(map[string]any{
				"tool_calls": []any{map[string]any{
					"index": idx, "function": map[string]any{"arguments": evt.Delta.PartialJSON},
				}},
			}), nil)}
		default:
			return nil
		}
	case "content_block_stop", "ping":
		return nil
	case "message_delta":
		if evt.Usage != nil {
			t.completionTokens = evt.Usage.OutputTokens
		}
		if evt.Delta != nil && evt.Delta.StopReason != nil {
			t.finishReason = anthropicStopToChat(*evt.Delta.StopReason)
		}
		return t.terminal()
	case "message_stop":
		return t.Finish()
	case "error":
		message := ""
		if evt.Error != nil {
			message = evt.Error.Message
		}
		if message == "" {
			message = string(data)
		}
		t.finished = true
		stop := "stop"
		return [][]byte{
			t.chunk(t.delta(map[string]any{"content": "[Error] " + message}), &stop),
			[]byte("data: [DONE]\n\n"),
		}
	default:
		return nil
	}
}

func (t *AnthropicToChatTranslator) Finish() [][]byte {
	if t.finished {
		return nil
	}
	return t.terminal()
}

func (t *AnthropicToChatTranslator) terminal() [][]byte {
	t.finished = true
	reason := t.finishReason
	if reason == "" {
		reason = "stop"
		if t.toolIndex > 0 {
			reason = "tool_calls"
		}
	}
	frames := [][]byte{t.finalChunk(reason)}
	return append(frames, []byte("data: [DONE]\n\n"))
}

func (t *AnthropicToChatTranslator) delta(delta map[string]any) map[string]any {
	if !t.roleSent {
		t.roleSent = true
		delta["role"] = "assistant"
	}
	return delta
}

func (t *AnthropicToChatTranslator) chunk(delta map[string]any, finish *string) []byte {
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

func (t *AnthropicToChatTranslator) finalChunk(reason string) []byte {
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

func anthropicStopToChat(stop string) string {
	switch stop {
	case "max_tokens":
		return "length"
	case "tool_use":
		return "tool_calls"
	case "refusal":
		return "content_filter"
	default:
		return "stop"
	}
}

func chatFinishToAnthropic(finish string) string {
	switch finish {
	case "length":
		return "max_tokens"
	case "tool_calls", "function_call":
		return "tool_use"
	case "content_filter":
		return "refusal"
	default:
		return "end_turn"
	}
}

type ChatToAnthropicTranslator struct {
	model      string
	id         string
	started    bool
	blockIndex int
	openBlock  string
	toolBlocks map[int]int
	finished   bool
}

func NewChatToAnthropicTranslator(model string) *ChatToAnthropicTranslator {
	return &ChatToAnthropicTranslator{
		model:      model,
		id:         "msg_" + randomHexID(),
		toolBlocks: make(map[int]int),
	}
}

type chatChunk struct {
	ID      string `json:"id"`
	Model   string `json:"model"`
	Choices []struct {
		Delta struct {
			Role             string  `json:"role"`
			Content          *string `json:"content"`
			ReasoningContent *string `json:"reasoning_content"`
			ToolCalls        []struct {
				Index    *int   `json:"index"`
				ID       string `json:"id"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"delta"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
	Usage *struct {
		PromptTokens     *int64 `json:"prompt_tokens"`
		CompletionTokens *int64 `json:"completion_tokens"`
	} `json:"usage"`
}

func (t *ChatToAnthropicTranslator) FeedChunk(data []byte) [][]byte {
	if t.finished {
		return nil
	}
	var chunk chatChunk
	if err := json.Unmarshal(data, &chunk); err != nil {
		return nil
	}
	if t.model == "" && chunk.Model != "" {
		t.model = chunk.Model
	}
	var frames [][]byte
	if !t.started {
		t.started = true
		frames = append(frames, t.frame("message_start", map[string]any{
			"type": "message_start",
			"message": map[string]any{
				"id": t.id, "type": "message", "role": "assistant", "model": t.model,
				"content": []any{}, "stop_reason": nil, "stop_sequence": nil,
				"usage": map[string]any{"input_tokens": 0, "output_tokens": 0},
			},
		}))
	}
	if len(chunk.Choices) == 0 {
		return frames
	}
	choice := chunk.Choices[0]
	if choice.Delta.Content != nil && *choice.Delta.Content != "" {
		frames = append(frames, t.textFrames(*choice.Delta.Content)...)
	}
	if choice.Delta.ReasoningContent != nil && *choice.Delta.ReasoningContent != "" {
		frames = append(frames, t.thinkingFrames(*choice.Delta.ReasoningContent)...)
	}
	for _, tc := range choice.Delta.ToolCalls {
		frames = append(frames, t.toolFrames(chatToolCallDelta{
			Index: tc.Index, ID: tc.ID, Name: tc.Function.Name, Arguments: tc.Function.Arguments,
		})...)
	}
	if choice.FinishReason != nil {
		var output *int64
		if chunk.Usage != nil {
			output = chunk.Usage.CompletionTokens
		}
		frames = append(frames, t.closeFrames(chatFinishToAnthropic(*choice.FinishReason), output)...)
		t.finished = true
	}
	return frames
}

func (t *ChatToAnthropicTranslator) Finish() [][]byte {
	if t.finished {
		return nil
	}
	t.finished = true
	return t.closeFrames("end_turn", nil)
}

func (t *ChatToAnthropicTranslator) frame(event string, data map[string]any) []byte {
	payload, _ := json.Marshal(data)
	return []byte(fmt.Sprintf("event: %s\ndata: %s\n\n", event, payload))
}

func (t *ChatToAnthropicTranslator) closeBlock() []byte {
	if t.openBlock == "" {
		return nil
	}
	t.openBlock = ""
	return t.frame("content_block_stop", map[string]any{
		"type": "content_block_stop", "index": t.blockIndex - 1,
	})
}

func (t *ChatToAnthropicTranslator) textFrames(text string) [][]byte {
	var frames [][]byte
	if t.openBlock != "text" {
		if stop := t.closeBlock(); stop != nil {
			frames = append(frames, stop)
		}
		frames = append(frames, t.frame("content_block_start", map[string]any{
			"type": "content_block_start", "index": t.blockIndex,
			"content_block": map[string]any{"type": "text", "text": ""},
		}))
		t.openBlock = "text"
		t.blockIndex++
	}
	frames = append(frames, t.frame("content_block_delta", map[string]any{
		"type": "content_block_delta", "index": t.blockIndex - 1,
		"delta": map[string]any{"type": "text_delta", "text": text},
	}))
	return frames
}

func (t *ChatToAnthropicTranslator) thinkingFrames(text string) [][]byte {
	var frames [][]byte
	if t.openBlock != "thinking" {
		if stop := t.closeBlock(); stop != nil {
			frames = append(frames, stop)
		}
		frames = append(frames, t.frame("content_block_start", map[string]any{
			"type": "content_block_start", "index": t.blockIndex,
			"content_block": map[string]any{"type": "thinking", "thinking": ""},
		}))
		t.openBlock = "thinking"
		t.blockIndex++
	}
	frames = append(frames, t.frame("content_block_delta", map[string]any{
		"type": "content_block_delta", "index": t.blockIndex - 1,
		"delta": map[string]any{"type": "thinking_delta", "thinking": text},
	}))
	return frames
}

type chatToolCallDelta struct {
	Index     *int
	ID        string
	Name      string
	Arguments string
}

func (t *ChatToAnthropicTranslator) toolFrames(tc chatToolCallDelta) [][]byte {
	chatIdx := 0
	if tc.Index != nil {
		chatIdx = *tc.Index
	}
	blockIdx, ok := t.toolBlocks[chatIdx]
	var frames [][]byte
	if !ok {
		if stop := t.closeBlock(); stop != nil {
			frames = append(frames, stop)
		}
		blockIdx = t.blockIndex
		t.toolBlocks[chatIdx] = blockIdx
		t.blockIndex++
		callID := tc.ID
		if callID == "" {
			callID = "call_" + randomHexID()
		}
		t.openBlock = "tool"
		frames = append(frames, t.frame("content_block_start", map[string]any{
			"type": "content_block_start", "index": blockIdx,
			"content_block": map[string]any{"type": "tool_use", "id": callID, "name": tc.Name, "input": map[string]any{}},
		}))
		if tc.Arguments == "" {
			return frames
		}
	}
	if tc.Arguments == "" {
		return frames
	}
	return append(frames, t.frame("content_block_delta", map[string]any{
		"type": "content_block_delta", "index": t.toolBlocks[chatIdx],
		"delta": map[string]any{"type": "input_json_delta", "partial_json": tc.Arguments},
	}))
}

func (t *ChatToAnthropicTranslator) closeFrames(stop string, output *int64) [][]byte {
	var frames [][]byte
	if stop := t.closeBlock(); stop != nil {
		frames = append(frames, stop)
	}
	frames = append(frames, t.frame("message_delta", map[string]any{
		"type":  "message_delta",
		"delta": map[string]any{"stop_reason": stop, "stop_sequence": nil},
		"usage": map[string]any{"output_tokens": output},
	}))
	frames = append(frames, t.frame("message_stop", map[string]any{"type": "message_stop"}))
	return frames
}
