package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/nawocci/pogu/internal/crypto"
	"github.com/nawocci/pogu/internal/store"
)

// promptFixture builds a service with one provider, two models, and one group
// containing the first model, returning the ids needed for rule targeting.
func promptFixture(t *testing.T) (*Service, context.Context, int64, int64, int64) {
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
	p, err := s.CreateProvider(ctx, "A", ProviderOpenAI, "oa", "https://a.test", "sk-a", true)
	if err != nil {
		t.Fatal(err)
	}
	ma, err := s.CreateModel(ctx, p.ID, "alpha", true)
	if err != nil {
		t.Fatal(err)
	}
	mb, err := s.CreateModel(ctx, p.ID, "beta", true)
	if err != nil {
		t.Fatal(err)
	}
	g, err := s.CreateGroup(ctx, "pool", true, KeySelectionFirst)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddGroupMember(ctx, g.ID, ma.ID); err != nil {
		t.Fatal(err)
	}
	return s, ctx, g.ID, ma.ID, mb.ID
}

func TestPromptRuleValidation(t *testing.T) {
	s, ctx, gID, mID, _ := promptFixture(t)
	valid := []PromptRuleTarget{{TargetType: PromptTargetModel, TargetID: mID}}

	if _, err := s.CreatePromptRule(ctx, PromptRuleInput{Name: "  ", Prompt: "x", Targets: valid}); !errors.Is(err, ErrValidation) {
		t.Fatalf("empty name = %v", err)
	}
	if _, err := s.CreatePromptRule(ctx, PromptRuleInput{Name: "r", Prompt: "x"}); !errors.Is(err, ErrValidation) {
		t.Fatalf("no targets = %v", err)
	}
	if _, err := s.CreatePromptRule(ctx, PromptRuleInput{Name: "r", Prompt: "x", Targets: []PromptRuleTarget{{TargetType: "bogus", TargetID: 1}}}); !errors.Is(err, ErrValidation) {
		t.Fatalf("bad target type = %v", err)
	}
	if _, err := s.CreatePromptRule(ctx, PromptRuleInput{Name: "r", Prompt: "x", Targets: []PromptRuleTarget{valid[0], valid[0]}}); !errors.Is(err, ErrValidation) {
		t.Fatalf("duplicate target = %v", err)
	}
	if _, err := s.CreatePromptRule(ctx, PromptRuleInput{Name: "r", Prompt: "x", Targets: []PromptRuleTarget{{TargetType: PromptTargetModel, TargetID: 9999}}}); !errors.Is(err, ErrValidation) {
		t.Fatalf("missing target = %v", err)
	}
	big := strings.Repeat("a", 16385)
	if _, err := s.CreatePromptRule(ctx, PromptRuleInput{Name: "r", Prompt: big, Targets: valid}); !errors.Is(err, ErrValidation) {
		t.Fatalf("oversize prompt = %v", err)
	}

	r, err := s.CreatePromptRule(ctx, PromptRuleInput{Name: "r", Prompt: "fix tools", Inherit: true, Enabled: true, Targets: valid})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Targets) != 1 || r.Targets[0].Label != "oa/alpha" {
		t.Fatalf("target label = %+v", r.Targets)
	}

	// A target may only be claimed by one rule.
	if _, err := s.CreatePromptRule(ctx, PromptRuleInput{Name: "other", Prompt: "x", Targets: valid}); !errors.Is(err, ErrAlreadyExists) {
		t.Fatalf("target claim conflict = %v", err)
	}

	// Group targets are claimable too and labelled by group name.
	gr, err := s.CreatePromptRule(ctx, PromptRuleInput{Name: "grp", Prompt: "g", Targets: []PromptRuleTarget{{TargetType: PromptTargetGroup, TargetID: gID}}})
	if err != nil {
		t.Fatal(err)
	}
	if gr.Targets[0].Label != "pool" {
		t.Fatalf("group label = %+v", gr.Targets)
	}

	// Updating a rule to claim a target held by another rule is rejected, but
	// re-claiming its own target is allowed.
	if _, err := s.UpdatePromptRule(ctx, gr.ID, PromptRuleInput{
		Name: "grp", Prompt: "g",
		Targets: []PromptRuleTarget{{TargetType: PromptTargetModel, TargetID: mID}},
	}); !errors.Is(err, ErrAlreadyExists) {
		t.Fatalf("reassigning claimed target = %v", err)
	}
}

func TestPromptRuleResolveDirectAndGroup(t *testing.T) {
	s, ctx, gID, mID, otherID := promptFixture(t)
	modelRule, err := s.CreatePromptRule(ctx, PromptRuleInput{
		Name: "model-fix", Prompt: "use tools", Inherit: true, Enabled: true,
		Targets: []PromptRuleTarget{{TargetType: PromptTargetModel, TargetID: mID}},
	})
	if err != nil {
		t.Fatal(err)
	}
	groupRule, err := s.CreatePromptRule(ctx, PromptRuleInput{
		Name: "group-style", Prompt: "be terse", Inherit: true, Enabled: true,
		Targets: []PromptRuleTarget{{TargetType: PromptTargetGroup, TargetID: gID}},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Direct call: only the model rule matches, no group rule.
	segs, suppressed := s.ResolvePromptRules(ctx, nil, mID)
	if len(segs) != 1 || segs[0].RuleID != modelRule.ID || suppressed {
		t.Fatalf("direct resolve = %+v, suppressed=%v", segs, suppressed)
	}
	// Group call: group rule stacks with the served model's rule, in list order.
	segs, _ = s.ResolvePromptRules(ctx, &gID, mID)
	if len(segs) != 2 || segs[0].RuleID != modelRule.ID || segs[1].RuleID != groupRule.ID {
		t.Fatalf("group resolve = %+v", segs)
	}
	// Group call served by a model with no rule: only the group rule.
	segs, _ = s.ResolvePromptRules(ctx, &gID, otherID)
	if len(segs) != 1 || segs[0].RuleID != groupRule.ID {
		t.Fatalf("group-only resolve = %+v", segs)
	}
	// Direct call to a model with no rule: nothing.
	if segs, _ = s.ResolvePromptRules(ctx, nil, otherID); len(segs) != 0 {
		t.Fatalf("unmatched resolve = %+v", segs)
	}
}

func TestPromptRuleInheritSuppression(t *testing.T) {
	s, ctx, gID, mID, _ := promptFixture(t)

	// Group rule with inherit off; model rule with inherit on.
	groupRule, err := s.CreatePromptRule(ctx, PromptRuleInput{
		Name: "group", Prompt: "g", Inherit: false, Enabled: true,
		Targets: []PromptRuleTarget{{TargetType: PromptTargetGroup, TargetID: gID}},
	})
	if err != nil {
		t.Fatal(err)
	}
	modelRule, err := s.CreatePromptRule(ctx, PromptRuleInput{
		Name: "model", Prompt: "m", Inherit: true, Enabled: true,
		Targets: []PromptRuleTarget{{TargetType: PromptTargetModel, TargetID: mID}},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Most specific matching rule (model, inherit on) decides: global kept.
	if _, suppressed := s.ResolvePromptRules(ctx, &gID, mID); suppressed {
		t.Fatal("model rule inherit=on must keep global on a group call")
	}
	if _, suppressed := s.ResolvePromptRules(ctx, nil, mID); suppressed {
		t.Fatal("model rule inherit=on must keep global on a direct call")
	}
	// Group call with no model rule: the group rule (inherit off) suppresses.
	segs, suppressed := s.ResolvePromptRules(ctx, &gID, 0)
	if !suppressed || len(segs) != 1 {
		t.Fatalf("group rule inherit=off = %+v, suppressed=%v", segs, suppressed)
	}

	// Flip the model rule to inherit off: it overrides the group rule's on.
	if _, err := s.UpdatePromptRule(ctx, modelRule.ID, PromptRuleInput{
		Name: "model", Prompt: "m", Inherit: false, Enabled: true,
		Targets: []PromptRuleTarget{{TargetType: PromptTargetModel, TargetID: mID}},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpdatePromptRule(ctx, groupRule.ID, PromptRuleInput{
		Name: "group", Prompt: "g", Inherit: true, Enabled: true,
		Targets: []PromptRuleTarget{{TargetType: PromptTargetGroup, TargetID: gID}},
	}); err != nil {
		t.Fatal(err)
	}
	if _, suppressed := s.ResolvePromptRules(ctx, &gID, mID); !suppressed {
		t.Fatal("model rule inherit=off must suppress global on a group call")
	}
}

func TestPromptRuleDisabledAndOrdering(t *testing.T) {
	s, ctx, gID, mID, _ := promptFixture(t)
	a, err := s.CreatePromptRule(ctx, PromptRuleInput{
		Name: "a", Prompt: "a", Inherit: true, Enabled: true,
		Targets: []PromptRuleTarget{{TargetType: PromptTargetModel, TargetID: mID}},
	})
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.CreatePromptRule(ctx, PromptRuleInput{
		Name: "b", Prompt: "b", Inherit: true, Enabled: true,
		Targets: []PromptRuleTarget{{TargetType: PromptTargetGroup, TargetID: gID}},
	})
	if err != nil {
		t.Fatal(err)
	}
	// List order drives stacking order.
	if segs, _ := s.ResolvePromptRules(ctx, &gID, mID); len(segs) != 2 || segs[0].RuleID != a.ID || segs[1].RuleID != b.ID {
		t.Fatalf("stack order = %+v", segs)
	}
	if err := s.ReorderPromptRules(ctx, []int64{b.ID, a.ID}); err != nil {
		t.Fatal(err)
	}
	if segs, _ := s.ResolvePromptRules(ctx, &gID, mID); len(segs) != 2 || segs[0].RuleID != b.ID || segs[1].RuleID != a.ID {
		t.Fatalf("reordered stack = %+v", segs)
	}

	// Disabling a rule removes it from resolution.
	if _, err := s.UpdatePromptRule(ctx, a.ID, PromptRuleInput{
		Name: "a", Prompt: "a", Inherit: true, Enabled: false,
		Targets: []PromptRuleTarget{{TargetType: PromptTargetModel, TargetID: mID}},
	}); err != nil {
		t.Fatal(err)
	}
	if segs, _ := s.ResolvePromptRules(ctx, &gID, mID); len(segs) != 1 || segs[0].RuleID != b.ID {
		t.Fatalf("disabled rule still resolved: %+v", segs)
	}

	// Reorder validation.
	if err := s.ReorderPromptRules(ctx, []int64{a.ID, a.ID}); !errors.Is(err, ErrValidation) {
		t.Fatalf("duplicate reorder = %v", err)
	}
	if err := s.ReorderPromptRules(ctx, []int64{}); !errors.Is(err, ErrValidation) {
		t.Fatalf("missing reorder = %v", err)
	}
}

func TestPromptRuleOrphanCleanup(t *testing.T) {
	s, ctx, gID, mID, _ := promptFixture(t)
	if _, err := s.CreatePromptRule(ctx, PromptRuleInput{
		Name: "m", Prompt: "m", Inherit: true, Enabled: true,
		Targets: []PromptRuleTarget{{TargetType: PromptTargetModel, TargetID: mID}},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreatePromptRule(ctx, PromptRuleInput{
		Name: "g", Prompt: "g", Inherit: true, Enabled: true,
		Targets: []PromptRuleTarget{{TargetType: PromptTargetGroup, TargetID: gID}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteGroup(ctx, gID); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteModel(ctx, mID); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := s.Store.DB.QueryRowContext(ctx, `SELECT COUNT(1) FROM prompt_rule_targets`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("orphan targets remain: %d", n)
	}
	// Resolution is a no-op for deleted targets.
	if segs, _ := s.ResolvePromptRules(ctx, nil, mID); len(segs) != 0 {
		t.Fatalf("deleted model still matched: %+v", segs)
	}
	if segs, _ := s.ResolvePromptRules(ctx, &gID, 0); len(segs) != 0 {
		t.Fatalf("deleted group still matched: %+v", segs)
	}
}
