package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nawocci/pogu/internal/store"
)

func TestParseDefaultCavemanSkill(t *testing.T) {
	skill, err := ParseCavemanSkill(defaultCavemanSkillMD)
	if err != nil {
		t.Fatalf("failed to parse default caveman skill: %v", err)
	}

	if skill.LeadingInstruction == "" {
		t.Fatal("leading instruction should not be empty")
	}
	if len(skill.Levels) < 3 {
		t.Fatalf("expected at least 3 levels, got %d", len(skill.Levels))
	}
	for _, expected := range []string{"full", "lite", "ultra"} {
		if _, ok := skill.Levels[expected]; !ok {
			t.Fatalf("expected level %q not found in parsed levels: %v", expected, skill.Levels)
		}
	}
	if skill.Rules == "" {
		t.Fatal("rules should not be empty")
	}
	if skill.AutoClarity == "" {
		t.Fatal("auto-clarity should not be empty")
	}
	if skill.Boundaries == "" {
		t.Fatal("boundaries should not be empty")
	}

	// Test PromptForLevel
	promptFull := skill.PromptForLevel("full")
	if !strings.Contains(promptFull, "[Caveman Mode: full]") {
		t.Fatalf("prompt missing level tag: %s", promptFull)
	}
	if !strings.Contains(promptFull, "Intensity (full):") {
		t.Fatalf("prompt missing intensity section: %s", promptFull)
	}
	if !strings.Contains(promptFull, "Rules:") {
		t.Fatalf("prompt missing rules section: %s", promptFull)
	}
	if !strings.Contains(promptFull, "Auto-Clarity:") {
		t.Fatalf("prompt missing auto-clarity section: %s", promptFull)
	}
	if !strings.Contains(promptFull, "Boundaries:") {
		t.Fatalf("prompt missing boundaries section: %s", promptFull)
	}
	if !strings.Contains(promptFull, "Reasoning/Thinking:") {
		t.Fatalf("prompt missing reasoning/thinking preservation: %s", promptFull)
	}

	// Test PromptForLevel for lite
	promptLite := skill.PromptForLevel("lite")
	if !strings.Contains(promptLite, "[Caveman Mode: lite]") {
		t.Fatalf("prompt missing level tag: %s", promptLite)
	}
	if !strings.Contains(promptLite, "Intensity (lite):") {
		t.Fatalf("prompt missing intensity section: %s", promptLite)
	}

	// Test fallback for unknown level
	promptUnknown := skill.PromptForLevel("non-existent-level")
	if !strings.Contains(promptUnknown, "[Caveman Mode: non-existent-level]") {
		t.Fatalf("prompt missing level tag: %s", promptUnknown)
	}
	// Fallback description uses default level
	if !strings.Contains(promptUnknown, skill.Levels["full"]) {
		t.Fatalf("unknown level should fall back to full level description: %s", promptUnknown)
	}
}

func TestCavemanServiceLifecycle(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	svc := New(st, []byte("01234567890123456789012345678901"))

	// Default settings: disabled, level "full"
	settings, err := svc.GetCavemanSettings(ctx)
	if err != nil {
		t.Fatalf("GetCavemanSettings failed: %v", err)
	}
	if settings.Enabled {
		t.Fatal("expected caveman to be disabled by default")
	}
	if settings.Level != "full" {
		t.Fatalf("expected default level 'full', got %q", settings.Level)
	}
	if len(settings.Levels) == 0 {
		t.Fatal("expected level metadata to be populated")
	}

	// Active prompt when disabled should be empty
	active, err := svc.ActiveCavemanPrompt(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if active != "" {
		t.Fatalf("expected empty prompt when disabled, got: %s", active)
	}

	// Enable caveman with ultra
	if err := svc.SetCavemanSettings(ctx, true, "ultra"); err != nil {
		t.Fatalf("SetCavemanSettings failed: %v", err)
	}

	settings, err = svc.GetCavemanSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !settings.Enabled || settings.Level != "ultra" {
		t.Fatalf("unexpected settings: %+v", settings)
	}

	// Active prompt should now return ultra prompt
	active, err = svc.ActiveCavemanPrompt(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(active, "[Caveman Mode: ultra]") {
		t.Fatalf("expected ultra prompt, got: %s", active)
	}

	// Invalid level returns error
	if err := svc.SetCavemanSettings(ctx, true, "invalid_level_xyz"); err == nil {
		t.Fatal("expected error for invalid level")
	}
}

func TestSyncCavemanSkill(t *testing.T) {
	mockSkill := `---
name: caveman
---

Mock lead instruction.

## Intensity

| Level | What change |
|-------|------------|
| **full** | Full mock mode |
| **lite** | Lite mock mode |

## Rules

Mock rules here.

## Auto-Clarity

Mock auto clarity.

## Boundaries

Mock boundaries.
`
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(mockSkill))
	}))
	defer ts.Close()

	ctx := context.Background()
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	svc := New(st, []byte("01234567890123456789012345678901"))
	svc.CavemanSkillURL = ts.URL

	if err := svc.SyncCavemanSkill(ctx); err != nil {
		t.Fatalf("SyncCavemanSkill failed: %v", err)
	}

	settings, err := svc.GetCavemanSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if settings.LastSyncedAt == "" {
		t.Fatal("expected LastSyncedAt to be populated after sync")
	}

	prompt, err := svc.GetCavemanPrompt(ctx, "full")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prompt, "Mock lead instruction.") {
		t.Fatalf("expected synced mock instruction, got: %s", prompt)
	}
	if !strings.Contains(prompt, "Full mock mode") {
		t.Fatalf("expected synced full mock mode, got: %s", prompt)
	}
}
