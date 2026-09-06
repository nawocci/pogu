package service

import (
	"errors"
	"strings"
	"testing"
)

func TestGlobalPromptRoundTrip(t *testing.T) {
	s, ctx := testService(t)
	text, enabled, err := s.GetGlobalPrompt(ctx)
	if err != nil || text != "" || enabled {
		t.Fatalf("defaults = %q, %v, %v", text, enabled, err)
	}
	if err := s.SetGlobalPrompt(ctx, "Be concise.", true); err != nil {
		t.Fatal(err)
	}
	text, enabled, err = s.GetGlobalPrompt(ctx)
	if err != nil || text != "Be concise." || !enabled {
		t.Fatalf("stored = %q, %v, %v", text, enabled, err)
	}
	if err := s.SetGlobalPrompt(ctx, "", false); err != nil {
		t.Fatal(err)
	}
	if ActivePrompt(" x ", true) == "" || ActivePrompt("x", false) != "" || ActivePrompt("  ", true) != "" {
		t.Fatal("activation rule wrong")
	}
	if err := s.SetGlobalPrompt(ctx, strings.Repeat("a", 16385), true); !errors.Is(err, ErrValidation) {
		t.Fatalf("oversize prompt = %v", err)
	}
}

func TestChangeAdminPassword(t *testing.T) {
	s, ctx := testService(t)
	if err := s.InitializeAdmin(ctx, "0123456789abcdef"); err != nil {
		t.Fatal(err)
	}
	token, err := s.CreateSession(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.ChangeAdminPassword(ctx, "wrong-password!", "new-password-123"); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("wrong current = %v", err)
	}
	if err := s.ChangeAdminPassword(ctx, "0123456789abcdef", "short"); !errors.Is(err, ErrValidation) {
		t.Fatalf("short next = %v", err)
	}
	if err := s.ChangeAdminPassword(ctx, "0123456789abcdef", "new-password-123"); err != nil {
		t.Fatal(err)
	}
	if s.ValidateSession(ctx, token) {
		t.Fatal("rotation must invalidate sessions")
	}
	ok, err := s.CheckAdminPassword(ctx, "new-password-123")
	if err != nil || !ok {
		t.Fatalf("new password check = %v, %v", ok, err)
	}
}
