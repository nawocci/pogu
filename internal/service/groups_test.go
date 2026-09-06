package service

import (
	"context"
	"errors"
	"testing"

	"github.com/nawocci/pogu/internal/crypto"
	"github.com/nawocci/pogu/internal/store"
)

func groupFixture(t *testing.T) (*Service, context.Context, int64, int64) {
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
	s := New(st, key)
	ctx := context.Background()
	pa, err := s.CreateProvider(ctx, "A", ProviderOpenAI, "oa", "https://a.test", "sk-a", true)
	if err != nil {
		t.Fatal(err)
	}
	pb, err := s.CreateProvider(ctx, "B", ProviderAnthropic, "an", "https://b.test", "sk-b", true)
	if err != nil {
		t.Fatal(err)
	}
	ma, err := s.CreateModel(ctx, pa.ID, "alpha", true)
	if err != nil {
		t.Fatal(err)
	}
	mb, err := s.CreateModel(ctx, pb.ID, "beta", true)
	if err != nil {
		t.Fatal(err)
	}
	return s, ctx, ma.ID, mb.ID
}

func TestGroupValidation(t *testing.T) {
	s, ctx, _, _ := groupFixture(t)
	for _, bad := range []string{"", "UPPER", "has/slash", "oa"} {
		if _, err := s.CreateGroup(ctx, bad, true, KeySelectionFirst); err == nil {
			t.Fatalf("CreateGroup(%q) must fail", bad)
		}
	}
	if _, err := s.CreateGroup(ctx, "x", true, KeySelectionFirst); err != nil {
		t.Fatalf("single-char group must pass: %v", err)
	}
	g, err := s.CreateGroup(ctx, "frontier", true, "")
	if err != nil {
		t.Fatal(err)
	}
	if g.Selection != KeySelectionFirst {
		t.Fatalf("default selection = %q", g.Selection)
	}
	if _, err := s.CreateGroup(ctx, "frontier", true, KeySelectionFirst); !errors.Is(err, ErrAlreadyExists) {
		t.Fatalf("dup group = %v", err)
	}
	if _, err := s.CreateGroup(ctx, "FRONTIER", true, KeySelectionFirst); !errors.Is(err, ErrValidation) {
		t.Fatalf("uppercase group = %v", err)
	}
}

func TestGroupResolution(t *testing.T) {
	s, ctx, maID, mbID := groupFixture(t)
	g, err := s.CreateGroup(ctx, "pool", true, KeySelectionFirst)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddGroupMember(ctx, g.ID, maID); err != nil {
		t.Fatal(err)
	}
	m2, err := s.AddGroupMember(ctx, g.ID, mbID)
	if err != nil {
		t.Fatal(err)
	}
	if m2.Position != 1 {
		t.Fatalf("position = %d", m2.Position)
	}
	got, cands, err := s.ResolveGroupTargets(ctx, "POOL", "openai")
	if err != nil || len(cands) != 2 {
		t.Fatalf("resolve = %+v, %v", cands, err)
	}
	if got.ID != g.ID || cands[0].Route.Model.PublicID != "oa/alpha" {
		t.Fatalf("order wrong: %+v", cands)
	}
	members, err := s.ListGroupMembers(ctx, g.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.ReorderGroupMembers(ctx, g.ID, []int64{members[0].ID}); err == nil {
		t.Fatal("partial reorder must fail")
	}
	if err := s.ReorderGroupMembers(ctx, g.ID, []int64{members[0].ID, members[0].ID}); err == nil {
		t.Fatal("duplicate reorder must fail")
	}
	if err := s.ReorderGroupMembers(ctx, g.ID, []int64{members[1].ID, members[0].ID}); err != nil {
		t.Fatal(err)
	}
	_, cands, _ = s.ResolveGroupTargets(ctx, "pool", "openai")
	if cands[0].Route.Model.PublicID != "an/beta" {
		t.Fatalf("reorder not applied: %+v", cands)
	}
	if err := s.DeleteGroupMember(ctx, g.ID, members[0].ID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.ResolveGroupTargets(ctx, "missing", "openai"); !errors.Is(err, ErrUnknownRoute) {
		t.Fatalf("missing group = %v", err)
	}
}

func TestGroupRoundRobin(t *testing.T) {
	s, ctx, maID, mbID := groupFixture(t)
	g, err := s.CreateGroup(ctx, "rr", true, KeySelectionRoundRobin)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []int64{maID, mbID} {
		if _, err := s.AddGroupMember(ctx, g.ID, id); err != nil {
			t.Fatal(err)
		}
	}
	var order []string
	for i := 0; i < 4; i++ {
		_, cands, err := s.ResolveGroupTargets(ctx, "rr", "openai")
		if err != nil {
			t.Fatal(err)
		}
		order = append(order, cands[0].Route.Model.PublicID)
	}
	want := []string{"oa/alpha", "an/beta", "oa/alpha", "an/beta"}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("rotation = %v", order)
		}
	}
}

func TestRoundRobinDomainsAreIndependent(t *testing.T) {
	s, ctx, maID, mbID := groupFixture(t)
	g, err := s.CreateGroup(ctx, "rr", true, KeySelectionRoundRobin)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []int64{maID, mbID} {
		if _, err := s.AddGroupMember(ctx, g.ID, id); err != nil {
			t.Fatal(err)
		}
	}
	provs, err := s.ListProviders(ctx)
	if err != nil {
		t.Fatal(err)
	}
	oaID := provs[0].ID
	if _, err := s.UpdateProvider(ctx, oaID, "A", ProviderOpenAI, "oa", "https://a.test", true, KeySelectionRoundRobin); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateProviderKey(ctx, oaID, "", "sk-a2"); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if _, _, err := s.ResolveGroupTargets(ctx, "rr", "openai"); err != nil {
			t.Fatal(err)
		}
	}
	keys, err := s.EligibleKeys(ctx, oaID)
	if err != nil {
		t.Fatal(err)
	}
	sec, _ := s.ProviderKeySecret(ctx, keys[0].ID)
	if sec != "sk-a" {
		t.Fatalf("provider cursor polluted by group rotation, head = %q", sec)
	}
	_, cands, err := s.ResolveGroupTargets(ctx, "rr", "openai")
	if err != nil {
		t.Fatal(err)
	}
	if cands[0].Route.Model.PublicID != "oa/alpha" {
		t.Fatalf("group cursor polluted by key rotation, head = %q", cands[0].Route.Model.PublicID)
	}
}

func TestKeyRoundRobin(t *testing.T) {
	s, ctx := testService(t)
	p, err := s.CreateProvider(ctx, "OA", ProviderOpenAI, "oa", "https://x.test", "sk-1", true, KeySelectionRoundRobin)
	if err != nil {
		t.Fatal(err)
	}
	for _, sec := range []string{"sk-2", "sk-3"} {
		if _, err := s.CreateProviderKey(ctx, p.ID, "", sec); err != nil {
			t.Fatal(err)
		}
	}
	var heads []string
	for i := 0; i < 3; i++ {
		keys, err := s.EligibleKeys(ctx, p.ID)
		if err != nil {
			t.Fatal(err)
		}
		sec, _ := s.ProviderKeySecret(ctx, keys[0].ID)
		heads = append(heads, sec)
	}
	if heads[0] != "sk-1" || heads[1] != "sk-2" || heads[2] != "sk-3" {
		t.Fatalf("key rotation = %v", heads)
	}
}

func TestBatchImport(t *testing.T) {
	s, ctx := testService(t)
	p, err := s.CreateProvider(ctx, "OA", ProviderOpenAI, "oa", "https://x.test", "sk-0", true)
	if err != nil {
		t.Fatal(err)
	}
	text := "work|sk-a\nsk-b\n\nbad|\nwork|sk-a\nsk-0"
	preview, err := s.ImportProviderKeys(ctx, p.ID, text, false)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Total != 5 || preview.Valid != 2 || preview.Invalid != 3 {
		t.Fatalf("preview = %+v", preview)
	}
	if preview.Entries[0].Name != "work" || preview.Entries[1].Name != "Key 2" {
		t.Fatalf("names = %+v", preview.Entries)
	}
	keys, _ := s.ListProviderKeys(ctx, p.ID)
	if len(keys) != 1 {
		t.Fatal("preview must not write")
	}
	result, err := s.ImportProviderKeys(ctx, p.ID, text, true)
	if err != nil {
		t.Fatal(err)
	}
	if result.Valid != 2 {
		t.Fatalf("commit = %+v", result)
	}
	keys, _ = s.ListProviderKeys(ctx, p.ID)
	if len(keys) != 3 {
		t.Fatalf("keys = %d", len(keys))
	}
}

func TestBuiltinAndSync(t *testing.T) {
	s, ctx := testService(t)
	if err := s.EnsureBuiltin(ctx); err != nil {
		t.Fatal(err)
	}
	b, err := s.GetBuiltinProvider(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if b.Prefix != "oc" || b.Enabled || b.Builtin != "opencode-free" {
		t.Fatalf("builtin = %+v", b)
	}
	if err := s.DeleteProvider(ctx, b.ID); !errors.Is(err, ErrBuiltin) {
		t.Fatalf("builtin delete = %v", err)
	}
	if _, err := s.CreateProvider(ctx, "Squat", ProviderOpenAI, "oc", "https://x.test", "", true); !errors.Is(err, ErrValidation) {
		t.Fatalf("oc squat = %v", err)
	}
	if !IsFreeOpenCodeModel("mimo-free") || !IsFreeOpenCodeModel("big-pickle") {
		t.Fatal("free classifier wrong")
	}
	if IsFreeOpenCodeModel("deepseek-v4-flash-free") || IsFreeOpenCodeModel("claude-sonnet-4") {
		t.Fatal("free classifier wrong")
	}
	if !IsResponsesModel("muse-spark-v1") {
		t.Fatal("responses classifier wrong")
	}
}
