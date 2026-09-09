package provider

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestNormalizeEffort(t *testing.T) {
	cases := map[string]string{
		"high":     "high",
		" HIGH ":   "high",
		"off":      "none",
		"disabled": "none",
		"none":     "none",
		"min":      "minimal",
		"minimal":  "minimal",
		"ultra":    "xhigh",
		"adaptive": "high",
	}
	for in, want := range cases {
		got, ok := NormalizeEffort(in)
		if !ok || got != want {
			t.Errorf("NormalizeEffort(%q) = %q,%v want %q", in, got, ok, want)
		}
	}
	for _, in := range []string{"", "auto", "bogus"} {
		if _, ok := NormalizeEffort(in); ok {
			t.Errorf("NormalizeEffort(%q) must not resolve", in)
		}
	}
}

func TestParseThinkingSuffix(t *testing.T) {
	clean, effort, ok := ParseThinkingSuffix("frontier(high)")
	if !ok || clean != "frontier" || effort != "high" {
		t.Fatalf("suffix level = %q,%q,%v", clean, effort, ok)
	}
	clean, effort, ok = ParseThinkingSuffix("frontier(none)")
	if !ok || clean != "frontier" || effort != "none" {
		t.Fatalf("suffix none = %q,%q,%v", clean, effort, ok)
	}
	clean, _, ok = ParseThinkingSuffix("frontier(auto)")
	if ok || clean != "frontier" {
		t.Fatalf("suffix auto must strip without override: %q,%v", clean, ok)
	}
	clean, effort, ok = ParseThinkingSuffix("frontier(8192)")
	if !ok || clean != "frontier" || effort != "medium" {
		t.Fatalf("suffix budget = %q,%q,%v", clean, effort, ok)
	}
	if clean, _, ok := ParseThinkingSuffix("frontier"); ok || clean != "frontier" {
		t.Fatalf("bare model must pass through: %q,%v", clean, ok)
	}
	if clean, effort, ok := ParseThinkingSuffix("oa/alpha(high)"); !ok || clean != "oa/alpha" || effort != "high" {
		t.Fatalf("direct route suffix must strip: %q,%q,%v", clean, effort, ok)
	}
}

func TestChatToAnthropicEmitsAdaptiveEffort(t *testing.T) {
	body := []byte(`{"model":"m","messages":[{"role":"user","content":"hi"}],"reasoning_effort":"high"}`)
	out, err := TranslateChatToAnthropic(body, false)
	if err != nil {
		t.Fatal(err)
	}
	var v struct {
		Thinking struct {
			Type string `json:"type"`
		} `json:"thinking"`
		OutputConfig struct {
			Effort string `json:"effort"`
		} `json:"output_config"`
	}
	if err := json.Unmarshal(out, &v); err != nil {
		t.Fatal(err)
	}
	if v.Thinking.Type != "adaptive" || v.OutputConfig.Effort != "high" {
		t.Fatalf("adaptive effort missing: %s", out)
	}
}

func TestChatToAnthropicMinimalMapsToLow(t *testing.T) {
	body := []byte(`{"model":"m","messages":[{"role":"user","content":"hi"}],"reasoning":{"effort":"minimal"}}`)
	out, err := TranslateChatToAnthropic(body, false)
	if err != nil {
		t.Fatal(err)
	}
	var v struct {
		OutputConfig struct {
			Effort string `json:"effort"`
		} `json:"output_config"`
	}
	if err := json.Unmarshal(out, &v); err != nil {
		t.Fatal(err)
	}
	if v.OutputConfig.Effort != "low" {
		t.Fatalf("minimal must map to low: %s", out)
	}
}

func TestChatToAnthropicDisabledEffort(t *testing.T) {
	body := []byte(`{"model":"m","messages":[{"role":"user","content":"hi"}],"reasoning_effort":"none"}`)
	out, err := TranslateChatToAnthropic(body, false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), `"type":"disabled"`) {
		t.Fatalf("disabled thinking missing: %s", out)
	}
	if strings.Contains(string(out), "output_config") {
		t.Fatalf("disabled must not carry effort: %s", out)
	}
}

func TestChatToAnthropicWithoutEffortOmitsThinking(t *testing.T) {
	body := []byte(`{"model":"m","messages":[{"role":"user","content":"hi"}]}`)
	out, err := TranslateChatToAnthropic(body, false)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), "thinking") || strings.Contains(string(out), "output_config") {
		t.Fatalf("must not invent thinking: %s", out)
	}
}

func TestAnthropicToChatPreservesEffort(t *testing.T) {
	body := []byte(`{"model":"m","max_tokens":64,"messages":[{"role":"user","content":"hi"}],
		"thinking":{"type":"adaptive"},"output_config":{"effort":"medium"}}`)
	out, err := TranslateAnthropicToChat(body, false)
	if err != nil {
		t.Fatal(err)
	}
	var v struct {
		ReasoningEffort string `json:"reasoning_effort"`
	}
	if err := json.Unmarshal(out, &v); err != nil {
		t.Fatal(err)
	}
	if v.ReasoningEffort != "medium" {
		t.Fatalf("effort lost: %s", out)
	}
}

func TestAnthropicToChatBudgetMapsToLevel(t *testing.T) {
	body := []byte(`{"model":"m","max_tokens":16000,"messages":[{"role":"user","content":"hi"}],
		"thinking":{"type":"enabled","budget_tokens":24576}}`)
	out, err := TranslateAnthropicToChat(body, false)
	if err != nil {
		t.Fatal(err)
	}
	var v struct {
		ReasoningEffort string `json:"reasoning_effort"`
	}
	if err := json.Unmarshal(out, &v); err != nil {
		t.Fatal(err)
	}
	if v.ReasoningEffort != "high" {
		t.Fatalf("budget 24576 must map to high, got %q: %s", v.ReasoningEffort, out)
	}
}

func TestAnthropicToChatDisabledCollapsesToNone(t *testing.T) {
	body := []byte(`{"model":"m","max_tokens":64,"messages":[{"role":"user","content":"hi"}],
		"thinking":{"type":"disabled"},"output_config":{"effort":"low"}}`)
	out, err := TranslateAnthropicToChat(body, false)
	if err != nil {
		t.Fatal(err)
	}
	// Disabled wins so cross-scheme failover never turns thinking back on;
	// the co-present effort is intentionally collapsed (documented limitation).
	var v struct {
		ReasoningEffort string `json:"reasoning_effort"`
	}
	if err := json.Unmarshal(out, &v); err != nil {
		t.Fatal(err)
	}
	if v.ReasoningEffort != "none" {
		t.Fatalf("disabled must collapse to none, got %q: %s", v.ReasoningEffort, out)
	}
}

func TestStripForeignFields(t *testing.T) {
	chat := StripChatForeign([]byte(`{"model":"m","reasoning_effort":"high","thinking":{"type":"adaptive"}}`))
	if strings.Contains(string(chat), "thinking") {
		t.Fatalf("chat strip failed: %s", chat)
	}
	if !strings.Contains(string(chat), "reasoning_effort") {
		t.Fatalf("chat strip removed native field: %s", chat)
	}
	anth := StripAnthropicForeign([]byte(`{"model":"m","thinking":{"type":"adaptive"},"reasoning_effort":"high"}`))
	if strings.Contains(string(anth), "reasoning_effort") {
		t.Fatalf("anthropic strip failed: %s", anth)
	}
	if !strings.Contains(string(anth), "thinking") {
		t.Fatalf("anthropic strip removed native field: %s", anth)
	}
}
