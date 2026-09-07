package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/nawocci/pogu/internal/crypto"
	"github.com/nawocci/pogu/internal/store"
)

func testService(t *testing.T) (*Service, context.Context) {
	t.Helper()
	key, err := crypto.LoadOrCreateKey(t.TempDir() + "/master.key")
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(t.TempDir() + "/pogu.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return New(st, key), context.Background()
}

func TestProviderValidation(t *testing.T) {
	s, ctx := testService(t)
	for _, tc := range []struct {
		name, prefix, base string
		typ                ProviderType
		want               string
	}{
		{"", "oa", "https://x.test/v1", ProviderOpenAI, "name is required"},
		{"OA", "Bad_Prefix", "https://x.test/v1", ProviderOpenAI, "prefix must use"},
		{"OA", "oc", "https://x.test/v1", ProviderOpenAI, "reserved"},
		{"OA", "oa", "notaurl", ProviderOpenAI, "base_url must be"},
		{"OA", "oa", "https://x.test/v1", "bogus", "type must be"},
	} {
		_, err := s.CreateProvider(ctx, tc.name, tc.typ, tc.prefix, tc.base, "", true)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("CreateProvider(%q) = %v, want %q", tc.name+"/"+tc.prefix, err, tc.want)
		}
	}
}

func TestProviderModelRoute(t *testing.T) {
	s, ctx := testService(t)
	p, err := s.CreateProvider(ctx, "OA", ProviderOpenAI, "oa", "https://x.test/v1/", "sk-1", true)
	if err != nil {
		t.Fatal(err)
	}
	if p.BaseURL != "https://x.test/v1" {
		t.Fatalf("trailing slash not trimmed: %q", p.BaseURL)
	}
	if !p.HasAPIKey || p.KeyCount != 1 {
		t.Fatalf("key not attached: %+v", p)
	}
	if _, err := s.CreateProvider(ctx, "Dupe", ProviderOpenAI, "oa", "https://y.test", "", true); !errors.Is(err, ErrAlreadyExists) {
		t.Fatalf("dup prefix = %v", err)
	}
	m, err := s.CreateModel(ctx, p.ID, "alpha", true)
	if err != nil {
		t.Fatal(err)
	}
	if m.PublicID != "oa/alpha" {
		t.Fatalf("public id = %q", m.PublicID)
	}
	route, err := s.ResolveRoute(ctx, "oa/alpha")
	if err != nil {
		t.Fatal(err)
	}
	if route.Provider.ID != p.ID || route.Model.ID != m.ID {
		t.Fatalf("bad route: %+v", route)
	}
	for _, bad := range []string{"oa", "a/b/c", "/x", "zz/nope"} {
		if _, err := s.ResolveRoute(ctx, bad); err == nil {
			t.Fatalf("ResolveRoute(%q) must fail", bad)
		}
	}
	if _, err := s.CreateModel(ctx, p.ID, "alpha", true); !errors.Is(err, ErrAlreadyExists) {
		t.Fatalf("dup model = %v", err)
	}
	if _, err := s.CreateModel(ctx, p.ID, "has space", true); !errors.Is(err, ErrValidation) {
		t.Fatalf("bad model name = %v", err)
	}
	if _, err := s.CreateModel(ctx, p.ID, "has/slash", true); err != nil {
		t.Fatalf("slashed model name = %v", err)
	}
	if _, err := s.CreateModel(ctx, p.ID, "free/model:tag", true); err != nil {
		t.Fatalf("nested slash model name = %v", err)
	}
	if _, err := s.CreateModel(ctx, p.ID, "a//b", true); !errors.Is(err, ErrValidation) {
		t.Fatalf("empty slash segment = %v", err)
	}
	if _, err := s.CreateModel(ctx, p.ID, "/leading", true); !errors.Is(err, ErrValidation) {
		t.Fatalf("leading slash = %v", err)
	}
	if _, err := s.CreateModel(ctx, p.ID, "trailing/", true); !errors.Is(err, ErrValidation) {
		t.Fatalf("trailing slash = %v", err)
	}
	slashed, err := s.ResolveRoute(ctx, "oa/free/model:tag")
	if err != nil {
		t.Fatal(err)
	}
	if slashed.Model.Name != "free/model:tag" {
		t.Fatalf("slashed route model = %q", slashed.Model.Name)
	}
	if _, err := s.ResolveRoute(ctx, "oa/has/slash"); err != nil {
		t.Fatal(err)
	}
}

func TestProviderKeyLifecycle(t *testing.T) {
	s, ctx := testService(t)
	p, err := s.CreateProvider(ctx, "OA", ProviderOpenAI, "oa", "https://x.test", "sk-a", true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateProviderKey(ctx, p.ID, "", "sk-a"); !errors.Is(err, ErrAlreadyExists) {
		t.Fatalf("dup secret = %v", err)
	}
	k2, err := s.CreateProviderKey(ctx, p.ID, "", "sk-b")
	if err != nil {
		t.Fatal(err)
	}
	if k2.Name != "Key 2" {
		t.Fatalf("autoname = %q", k2.Name)
	}
	k3, err := s.CreateProviderKey(ctx, p.ID, "", "sk-c")
	if err != nil {
		t.Fatal(err)
	}
	if k3.Name != "Key 3" {
		t.Fatalf("autoname = %q", k3.Name)
	}
	if err := s.DeleteProviderKey(ctx, k2.ID); err != nil {
		t.Fatal(err)
	}
	k4, err := s.CreateProviderKey(ctx, p.ID, "", "sk-d")
	if err != nil {
		t.Fatal(err)
	}
	if k4.Name != "Key 4" {
		t.Fatalf("autoname must be monotonic, got %q", k4.Name)
	}
	if err := s.SetProviderKeyPrimary(ctx, p.ID, k4.ID); err != nil {
		t.Fatal(err)
	}
	keys, err := s.ListProviderKeys(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if keys[0].ID != k4.ID {
		t.Fatalf("primary not first: %+v", keys)
	}
	secret, err := s.ProviderPrimarySecret(ctx, p.ID)
	if err != nil || secret != "sk-d" {
		t.Fatalf("primary secret = %q, %v", secret, err)
	}
	got, err := s.UpdateProviderKey(ctx, k4.ID, "Work", false)
	if err != nil || got.Name != "Work" || got.Enabled {
		t.Fatalf("update = %+v, %v", got, err)
	}
	eligible, err := s.EligibleKeys(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, k := range eligible {
		if k.ID == k4.ID {
			t.Fatal("disabled key must be ineligible")
		}
	}
}

func TestClientKeyLifecycle(t *testing.T) {
	s, ctx := testService(t)
	key, secret, err := s.CreateAPIKey(ctx, "app")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(secret, "sk-pogu-") {
		t.Fatalf("secret shape = %q", secret)
	}
	got, err := s.AuthenticateAPIKey(ctx, secret)
	if err != nil || got.ID != key.ID {
		t.Fatalf("auth = %+v, %v", got, err)
	}
	if _, err := s.AuthenticateAPIKey(ctx, "bogus"); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("bogus auth = %v", err)
	}
	list, err := s.ListAPIKeys(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, k := range list {
		if k.ID == key.ID && k.RevokedAt != nil {
			t.Fatal("fresh key must not be revoked")
		}
	}
	if err := s.RevokeAPIKey(ctx, key.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AuthenticateAPIKey(ctx, secret); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("revoked auth = %v", err)
	}
}

func TestAdminPassword(t *testing.T) {
	s, ctx := testService(t)
	if err := s.InitializeAdmin(ctx, "short"); err == nil {
		t.Fatal("short password must fail")
	}
	if err := s.InitializeAdmin(ctx, "0123456789abcdef"); err != nil {
		t.Fatal(err)
	}
	if err := s.InitializeAdmin(ctx, "0123456789abcdef"); !errors.Is(err, ErrAlreadyExists) {
		t.Fatalf("re-init = %v", err)
	}
	ok, err := s.CheckAdminPassword(ctx, "0123456789abcdef")
	if err != nil || !ok {
		t.Fatalf("password check = %v, %v", ok, err)
	}
	ok, _ = s.CheckAdminPassword(ctx, "wrong-password!")
	if ok {
		t.Fatal("wrong password accepted")
	}
	token, err := s.CreateSession(ctx, 0)
	if err != nil || !s.ValidateSession(ctx, token) {
		t.Fatalf("session = %q, %v", token, err)
	}
	if err := s.DeleteSession(ctx, token); err != nil {
		t.Fatal(err)
	}
	if s.ValidateSession(ctx, token) {
		t.Fatal("deleted session still valid")
	}
}
