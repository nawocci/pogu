package provider

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
)

// Canonical effort handling for cross-scheme group routing.
//
// Clients express thinking depth in scheme-native shapes:
//   - OpenAI chat: reasoning_effort / reasoning.effort ("off","minimal","low","medium","high","xhigh","max")
//   - Anthropic: thinking ({type,budget_tokens}) + output_config.effort
//   - model suffix: "group(high)", "group(8192)", "group(none)", "group(auto)"
//
// The gateway normalizes intent once, then each per-target translator emits
// the native shape (birfrost ModelCaps / 9router thinkingUnified pattern,
// reduced to pogu's three schemes).

// NormalizeEffort lowercases/trims and maps aliases onto the canonical ladder:
// none, minimal, low, medium, high, xhigh, max.
// "off"/"disabled" -> "none", "min" -> "minimal", "ultra" -> "xhigh".
// "adaptive" is a thinking mode, not a level (birfrost guard) -> "high".
// "auto" means "no override" and returns ok=false.
func NormalizeEffort(s string) (string, bool) {
	e := strings.ToLower(strings.TrimSpace(s))
	switch e {
	case "":
		return "", false
	case "auto":
		return "", false
	case "off", "none", "disabled":
		return "none", true
	case "min", "minimal":
		return "minimal", true
	case "low", "medium", "high", "xhigh", "max":
		return e, true
	case "ultra":
		return "xhigh", true
	case "adaptive":
		return "high", true
	default:
		return "", false
	}
}

// MapChatEffortToAnthropic maps a canonical effort onto Anthropic adaptive
// effort levels. Anthropic has no "minimal" (birfrost: minimal->low).
func MapChatEffortToAnthropic(effort string) string {
	switch effort {
	case "minimal":
		return "low"
	case "adaptive":
		return "high"
	default:
		return effort
	}
}

// BudgetToLevel maps numeric budget_tokens onto a canonical level
// (9router thinking.js thresholds, with >32768 landing on max).
func BudgetToLevel(budget int) (string, bool) {
	if budget <= 0 {
		return "none", true
	}
	switch {
	case budget <= 768:
		return "minimal", true
	case budget <= 4096:
		return "low", true
	case budget <= 16384:
		return "medium", true
	case budget <= 28672:
		return "high", true
	case budget <= 32768:
		return "xhigh", true
	default:
		return "max", true
	}
}

var suffixRe = regexp.MustCompile(`^(.*)\(([^()]+)\)\s*$`)

// ParseThinkingSuffix splits "model(value)" into clean model + canonical effort.
// Values: level names, "none"/"off", "auto" (clean only, no override),
// "ultra", or bare numbers (budget_tokens -> level).
// Returns clean model, effort, hasOverride. "auto" strips with hasOverride=false.
func ParseThinkingSuffix(model string) (clean, effort string, hasOverride bool) {
	m := suffixRe.FindStringSubmatch(model)
	if m == nil {
		return model, "", false
	}
	clean = strings.TrimSpace(m[1])
	if clean == "" {
		return model, "", false
	}
	raw := strings.ToLower(strings.TrimSpace(m[2]))
	switch raw {
	case "none", "off":
		return clean, "none", true
	case "auto":
		return clean, "", false
	}
	if n, err := strconv.Atoi(raw); err == nil {
		if lvl, ok := BudgetToLevel(n); ok {
			return clean, lvl, true
		}
		return clean, "", false
	}
	if lvl, ok := NormalizeEffort(raw); ok {
		return clean, lvl, true
	}
	return model, "", false
}

// ExtractChatEffort reads canonical effort from a chat-completions body,
// checking reasoning_effort first, then reasoning.effort (OpenRouter style).
func ExtractChatEffort(body []byte) (string, bool) {
	var in chatRequest
	if err := json.Unmarshal(body, &in); err != nil {
		return "", false
	}
	if in.ReasoningEffort != nil {
		if lvl, ok := NormalizeEffort(*in.ReasoningEffort); ok {
			return lvl, true
		}
	}
	if in.Reasoning != nil {
		if m, ok := in.Reasoning.(map[string]any); ok {
			if lvl, ok := NormalizeEffort(stringOf(m["effort"])); ok {
				return lvl, true
			}
		}
	}
	return "", false
}

// ExtractAnthropicEffort reads intent from a native Anthropic body.
// Priority mirrors 9router extractThinking: output_config.effort first,
// then thinking (disabled -> none, budget -> level, adaptive/enabled w/o
// budget or effort -> no specific level).
func ExtractAnthropicEffort(body []byte) (string, bool) {
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		return "", false
	}
	// Explicit disabled wins over any co-present effort: the canonical chat
	// shape carries a single effort, and re-enabling thinking downstream
	// against an explicit off would bill tokens the caller refused.
	if th, ok := raw["thinking"].(map[string]any); ok {
		if strings.EqualFold(strings.TrimSpace(stringOf(th["type"])), "disabled") {
			return "none", true
		}
	}
	if oc, ok := raw["output_config"].(map[string]any); ok {
		if lvl, ok := NormalizeEffort(stringOf(oc["effort"])); ok {
			return lvl, true
		}
		// output_config present but effort "auto"/unknown -> no override
		if s := stringOf(oc["effort"]); strings.EqualFold(strings.TrimSpace(s), "auto") {
			return "", false
		}
	}
	th, _ := raw["thinking"].(map[string]any)
	if th == nil {
		return "", false
	}
	typ := strings.ToLower(strings.TrimSpace(stringOf(th["type"])))
	switch typ {
	case "disabled":
		return "none", true
	case "adaptive", "enabled":
		// explicit effort nested inside thinking (some gateways) wins
		if lvl, ok := NormalizeEffort(stringOf(th["effort"])); ok {
			return lvl, true
		}
		var budget int
		switch v := th["budget_tokens"].(type) {
		case float64:
			budget = int(v)
		case int:
			budget = v
		case string:
			if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
				budget = n
			}
		}
		if budget > 0 {
			return BudgetToLevel(budget)
		}
		return "", false
	default:
		return "", false
	}
}

// SetChatEffort sets reasoning_effort on a chat-completions body.
func SetChatEffort(body []byte, effort string) []byte {
	var object map[string]any
	if json.Unmarshal(body, &object) != nil {
		return body
	}
	object["reasoning_effort"] = effort
	out, err := json.Marshal(object)
	if err != nil {
		return body
	}
	return out
}

// SetAnthropicThinking sets native thinking + output_config on an Anthropic
// body. "none" -> thinking disabled (output_config removed); otherwise
// thinking adaptive + output_config effort (minimal->low).
func SetAnthropicThinking(body []byte, effort string) []byte {
	var object map[string]any
	if json.Unmarshal(body, &object) != nil {
		return body
	}
	if effort == "none" {
		object["thinking"] = map[string]any{"type": "disabled"}
		delete(object, "output_config")
	} else {
		object["thinking"] = map[string]any{"type": "adaptive"}
		object["output_config"] = map[string]any{"effort": MapChatEffortToAnthropic(effort)}
	}
	out, err := json.Marshal(object)
	if err != nil {
		return body
	}
	return out
}

// SetAnthropicBudget sets legacy thinking enabled+budget_tokens (used for
// numeric suffixes targeting Anthropic directly).
func SetAnthropicBudget(body []byte, budget int) []byte {
	if budget < 1024 {
		budget = 1024
	}
	var object map[string]any
	if json.Unmarshal(body, &object) != nil {
		return body
	}
	object["thinking"] = map[string]any{"type": "enabled", "budget_tokens": budget}
	out, err := json.Marshal(object)
	if err != nil {
		return body
	}
	return out
}

// StripChatForeign removes Anthropic-native control fields from a chat body
// so OpenAI/Responses upstreams never see them (avoids 400s on strict
// providers). reasoning_* intentionally preserved.
func StripChatForeign(body []byte) []byte {
	var object map[string]any
	if json.Unmarshal(body, &object) != nil {
		return body
	}
	changed := false
	for _, k := range []string{"thinking", "output_config"} {
		if _, ok := object[k]; ok {
			delete(object, k)
			changed = true
		}
	}
	if !changed {
		return body
	}
	out, err := json.Marshal(object)
	if err != nil {
		return body
	}
	return out
}

// StripAnthropicForeign removes OpenAI-native reasoning fields from an
// Anthropic body so strict Anthropic-compatible upstreams never see them.
func StripAnthropicForeign(body []byte) []byte {
	var object map[string]any
	if json.Unmarshal(body, &object) != nil {
		return body
	}
	changed := false
	for _, k := range []string{"reasoning_effort", "reasoning"} {
		if _, ok := object[k]; ok {
			delete(object, k)
			changed = true
		}
	}
	if !changed {
		return body
	}
	out, err := json.Marshal(object)
	if err != nil {
		return body
	}
	return out
}
