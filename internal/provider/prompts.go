package provider

import (
	"encoding/json"
	"strings"
)

const promptHeader = "Pogu router preferences:"

func promptHeaderFor(label string) string {
	label = strings.TrimSpace(label)
	if label == "" {
		return promptHeader
	}
	return "Pogu router preferences (" + label + "):"
}

func wrapPrompt(prompt string) string {
	return promptHeader + "\n" + strings.Trim(prompt, "\n")
}

func wrapPromptLabeled(prompt, label string) string {
	return promptHeaderFor(label) + "\n" + strings.Trim(prompt, "\n")
}

// InjectChatPrompt appends the global preferences after the request's own
// system content in a canonical chat body. Messages keep their order; when
// no system message exists the prompt becomes the first message. Bodies that
// do not decode as chat objects pass through untouched.
func InjectChatPrompt(body []byte, prompt string) []byte {
	return InjectChatPromptLabeled(body, prompt, "")
}

// InjectChatPromptLabeled is InjectChatPrompt with a rule label rendered into
// the marker, e.g. "Pogu router preferences (Tool-use correction):".
func InjectChatPromptLabeled(body []byte, prompt, label string) []byte {
	if strings.TrimSpace(prompt) == "" {
		return body
	}
	var object map[string]any
	if json.Unmarshal(body, &object) != nil {
		return body
	}
	raw, ok := object["messages"].([]any)
	if !ok {
		return body
	}
	entry := map[string]any{"role": "system", "content": wrapPromptLabeled(prompt, label)}
	at := 0
	for i, item := range raw {
		if msg, ok := item.(map[string]any); ok && stringOf(msg["role"]) == "system" {
			at = i + 1
		}
	}
	messages := make([]any, 0, len(raw)+1)
	messages = append(messages, raw[:at]...)
	messages = append(messages, entry)
	messages = append(messages, raw[at:]...)
	object["messages"] = messages
	out, err := json.Marshal(object)
	if err != nil {
		return body
	}
	return out
}

// InjectAnthropicPrompt appends the global preferences after the request's
// own system content in a native Anthropic body, preserving the string or
// blocks shape of the system field.
func InjectAnthropicPrompt(body []byte, prompt string) []byte {
	return InjectAnthropicPromptLabeled(body, prompt, "")
}

// InjectAnthropicPromptLabeled is InjectAnthropicPrompt with a rule label
// rendered into the marker.
func InjectAnthropicPromptLabeled(body []byte, prompt, label string) []byte {
	if strings.TrimSpace(prompt) == "" {
		return body
	}
	var object map[string]any
	if json.Unmarshal(body, &object) != nil {
		return body
	}
	switch system := object["system"].(type) {
	case nil:
		object["system"] = wrapPromptLabeled(prompt, label)
	case string:
		if system == "" {
			object["system"] = wrapPromptLabeled(prompt, label)
		} else {
			object["system"] = system + "\n\n---\n" + wrapPromptLabeled(prompt, label)
		}
	case []any:
		object["system"] = append(system, map[string]any{"type": "text", "text": wrapPromptLabeled(prompt, label)})
	default:
		return body
	}
	out, err := json.Marshal(object)
	if err != nil {
		return body
	}
	return out
}
