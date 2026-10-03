package httpapi

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nawocci/pogu/internal/crypto"
	"github.com/nawocci/pogu/internal/service"
	"github.com/nawocci/pogu/internal/store"
)

// promptRuleFixture builds an API with one provider, two models (m, m2), and a
// group "pool" containing only m. The capture stub records the upstream body.
func promptRuleFixture(t *testing.T, scheme service.ProviderType, stub *captureStub) (*API, *httptest.Server, string, int64, int64, int64) {
	t.Helper()
	master, err := crypto.LoadOrCreateKey(t.TempDir() + "/master.key")
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(t.TempDir() + "/pogu.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	svc := service.New(st, master)
	ctx := t.Context()
	if err := svc.InitializeAdmin(ctx, "0123456789abcdef"); err != nil {
		t.Fatal(err)
	}
	upstream := httptest.NewServer(stub.handler())
	t.Cleanup(upstream.Close)
	p, err := svc.CreateProvider(ctx, "P", scheme, "p", upstream.URL, "secret", true)
	if err != nil {
		t.Fatal(err)
	}
	m, err := svc.CreateModel(ctx, p.ID, "m", true)
	if err != nil {
		t.Fatal(err)
	}
	m2, err := svc.CreateModel(ctx, p.ID, "m2", true)
	if err != nil {
		t.Fatal(err)
	}
	g, err := svc.CreateGroup(ctx, "pool", true, service.KeySelectionFirst)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddGroupMember(ctx, g.ID, m.ID); err != nil {
		t.Fatal(err)
	}
	_, secret, err := svc.CreateAPIKey(ctx, "app")
	if err != nil {
		t.Fatal(err)
	}
	api := New(svc, nil)
	return api, httptest.NewServer(api.Handler(http.NotFoundHandler())), secret, m.ID, m2.ID, g.ID
}

func TestGatewayModelRuleDirectAndGroup(t *testing.T) {
	stub := &captureStub{}
	api, server, key, mID, _, gID := promptRuleFixture(t, service.ProviderOpenAI, stub)
	defer server.Close()
	ctx := t.Context()

	if _, err := api.Service.CreatePromptRule(ctx, service.PromptRuleInput{
		Name: "model-fix", Prompt: "Use the provided tools.", Inherit: true, Enabled: true,
		Targets: []service.PromptRuleTarget{{TargetType: service.PromptTargetModel, TargetID: mID}},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := api.Service.CreatePromptRule(ctx, service.PromptRuleInput{
		Name: "group-style", Prompt: "Be terse for this group.", Inherit: true, Enabled: true,
		Targets: []service.PromptRuleTarget{{TargetType: service.PromptTargetGroup, TargetID: gID}},
	}); err != nil {
		t.Fatal(err)
	}

	// Direct call: model rule only.
	if status := gatewayPost(t, server, key, "/v1/chat/completions",
		`{"model":"p/m","messages":[{"role":"user","content":"hi"}]}`, "openai"); status != 200 {
		t.Fatalf("direct = %d", status)
	}
	seen, _ := stub.lastBody.Load().(string)
	if !strings.Contains(seen, "Pogu router preferences (model-fix):") {
		t.Fatalf("model rule missing on direct call: %s", seen)
	}
	if strings.Contains(seen, "group-style") {
		t.Fatalf("group rule must not fire on a direct call: %s", seen)
	}

	// Group call: group rule + model rule, group rule after the model rule
	// because it was created later (list order).
	if status := gatewayPost(t, server, key, "/v1/chat/completions",
		`{"model":"pool","messages":[{"role":"user","content":"hi"}]}`, "openai"); status != 200 {
		t.Fatalf("group = %d", status)
	}
	seen, _ = stub.lastBody.Load().(string)
	modelIdx := strings.Index(seen, "Pogu router preferences (model-fix):")
	groupIdx := strings.Index(seen, "Pogu router preferences (group-style):")
	if modelIdx < 0 || groupIdx < 0 {
		t.Fatalf("expected both rules on group call: %s", seen)
	}
	if modelIdx > groupIdx {
		t.Fatalf("rules must stack in list order: %s", seen)
	}
}

func TestGatewayRuleSuppressesGlobal(t *testing.T) {
	stub := &captureStub{}
	api, server, key, mID, _, _ := promptRuleFixture(t, service.ProviderOpenAI, stub)
	defer server.Close()
	ctx := t.Context()

	if err := api.Service.SetGlobalPrompt(ctx, "GLOBAL-BASE", true); err != nil {
		t.Fatal(err)
	}
	if _, err := api.Service.CreatePromptRule(ctx, service.PromptRuleInput{
		Name: "override", Prompt: "RULE-BODY", Inherit: false, Enabled: true,
		Targets: []service.PromptRuleTarget{{TargetType: service.PromptTargetModel, TargetID: mID}},
	}); err != nil {
		t.Fatal(err)
	}
	if status := gatewayPost(t, server, key, "/v1/chat/completions",
		`{"model":"p/m","messages":[{"role":"user","content":"hi"}]}`, "openai"); status != 200 {
		t.Fatalf("gateway = %d", status)
	}
	seen, _ := stub.lastBody.Load().(string)
	if strings.Contains(seen, "GLOBAL-BASE") {
		t.Fatalf("global must be suppressed when inherit=false: %s", seen)
	}
	if !strings.Contains(seen, "RULE-BODY") {
		t.Fatalf("rule body missing: %s", seen)
	}

	// inherit=true keeps the global.
	if _, err := api.Service.UpdatePromptRule(ctx, mustRuleID(t, api, "override"), service.PromptRuleInput{
		Name: "override", Prompt: "RULE-BODY", Inherit: true, Enabled: true,
		Targets: []service.PromptRuleTarget{{TargetType: service.PromptTargetModel, TargetID: mID}},
	}); err != nil {
		t.Fatal(err)
	}
	if status := gatewayPost(t, server, key, "/v1/chat/completions",
		`{"model":"p/m","messages":[{"role":"user","content":"hi"}]}`, "openai"); status != 200 {
		t.Fatalf("gateway = %d", status)
	}
	seen, _ = stub.lastBody.Load().(string)
	if !strings.Contains(seen, "GLOBAL-BASE") || !strings.Contains(seen, "RULE-BODY") {
		t.Fatalf("inherit=true must stack global and rule: %s", seen)
	}
}

func TestGatewayPromptHeaderOverride(t *testing.T) {
	stub := &captureStub{}
	api, server, key, mID, _, _ := promptRuleFixture(t, service.ProviderOpenAI, stub)
	defer server.Close()
	ctx := t.Context()

	if err := api.Service.SetGlobalPrompt(ctx, "GLOBAL-BASE", true); err != nil {
		t.Fatal(err)
	}
	if _, err := api.Service.CreatePromptRule(ctx, service.PromptRuleInput{
		Name: "named-rule", Prompt: "NAMED-BODY", Inherit: true, Enabled: true,
		Targets: []service.PromptRuleTarget{{TargetType: service.PromptTargetModel, TargetID: mID}},
	}); err != nil {
		t.Fatal(err)
	}

	// X-Pogu-Prompt: off disables everything.
	if status := gatewayPostHeader(t, server, key, "/v1/chat/completions",
		`{"model":"p/m","messages":[{"role":"user","content":"hi"}]}`, "openai", "X-Pogu-Prompt", "off"); status != 200 {
		t.Fatalf("off = %d", status)
	}
	seen, _ := stub.lastBody.Load().(string)
	if strings.Contains(seen, "GLOBAL-BASE") || strings.Contains(seen, "NAMED-BODY") {
		t.Fatalf("off must suppress all router prompts: %s", seen)
	}

	// X-Pogu-Prompt: <name> forces that rule. Its own inherit flag still
	// governs the global base (named-rule inherits, so global stays).
	if status := gatewayPostHeader(t, server, key, "/v1/chat/completions",
		`{"model":"p/m2","messages":[{"role":"user","content":"hi"}]}`, "openai", "X-Pogu-Prompt", "named-rule"); status != 200 {
		t.Fatalf("named = %d", status)
	}
	seen, _ = stub.lastBody.Load().(string)
	if !strings.Contains(seen, "NAMED-BODY") {
		t.Fatalf("named rule not forced: %s", seen)
	}
	if !strings.Contains(seen, "GLOBAL-BASE") {
		t.Fatalf("forced rule with inherit=true keeps the global: %s", seen)
	}

	// Forcing a rule whose inherit is off drops the global base.
	if _, err := api.Service.UpdatePromptRule(ctx, mustRuleID(t, api, "named-rule"), service.PromptRuleInput{
		Name: "named-rule", Prompt: "NAMED-BODY", Inherit: false, Enabled: true,
		Targets: []service.PromptRuleTarget{{TargetType: service.PromptTargetModel, TargetID: mID}},
	}); err != nil {
		t.Fatal(err)
	}
	if status := gatewayPostHeader(t, server, key, "/v1/chat/completions",
		`{"model":"p/m2","messages":[{"role":"user","content":"hi"}]}`, "openai", "X-Pogu-Prompt", "named-rule"); status != 200 {
		t.Fatalf("named = %d", status)
	}
	seen, _ = stub.lastBody.Load().(string)
	if !strings.Contains(seen, "NAMED-BODY") || strings.Contains(seen, "GLOBAL-BASE") {
		t.Fatalf("forced rule with inherit=false must drop the global: %s", seen)
	}
}

func TestGatewayAnthropicRuleStacking(t *testing.T) {
	stub := &captureStub{}
	api, server, key, mID, _, _ := promptRuleFixture(t, service.ProviderAnthropic, stub)
	defer server.Close()
	ctx := t.Context()

	if err := api.Service.SetGlobalPrompt(ctx, "GLOBAL-BASE", true); err != nil {
		t.Fatal(err)
	}
	if _, err := api.Service.CreatePromptRule(ctx, service.PromptRuleInput{
		Name: "anthropic-fix", Prompt: "ANTHROPIC-RULE", Inherit: true, Enabled: true,
		Targets: []service.PromptRuleTarget{{TargetType: service.PromptTargetModel, TargetID: mID}},
	}); err != nil {
		t.Fatal(err)
	}
	if status := gatewayPost(t, server, key, "/v1/messages",
		`{"model":"p/m","max_tokens":16,"system":"Original.","messages":[{"role":"user","content":"hi"}]}`, "anthropic"); status != 200 {
		t.Fatalf("gateway = %d", status)
	}
	seen, _ := stub.lastBody.Load().(string)
	globalIdx := strings.Index(seen, "GLOBAL-BASE")
	ruleIdx := strings.Index(seen, "ANTHROPIC-RULE")
	if globalIdx < 0 || ruleIdx < 0 || globalIdx > ruleIdx {
		t.Fatalf("anthropic system order wrong: %s", seen)
	}
	if !strings.HasPrefix(seen[strings.Index(seen, `"system"`):], `"system":"Original.`) {
		t.Fatalf("original system damaged: %s", seen)
	}
}

func TestGatewayRecordsPromptRuleNamesOnly(t *testing.T) {
	stub := &captureStub{}
	api, server, key, mID, _, _ := promptRuleFixture(t, service.ProviderOpenAI, stub)
	defer server.Close()
	ctx := t.Context()

	if _, err := api.Service.CreatePromptRule(ctx, service.PromptRuleInput{
		Name: "recorded-rule", Prompt: "SECRET-PROMPT-TEXT", Inherit: true, Enabled: true,
		Targets: []service.PromptRuleTarget{{TargetType: service.PromptTargetModel, TargetID: mID}},
	}); err != nil {
		t.Fatal(err)
	}
	if status := gatewayPost(t, server, key, "/v1/chat/completions",
		`{"model":"p/m","messages":[{"role":"user","content":"hi"}]}`, "openai"); status != 200 {
		t.Fatalf("gateway = %d", status)
	}

	recs, err := api.Telemetry.List(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) == 0 {
		t.Fatal("no telemetry recorded")
	}
	var found bool
	for _, rec := range recs {
		for _, ref := range rec.Prompts {
			if ref.RuleName == "recorded-rule" {
				found = true
			}
		}
	}
	if !found {
		t.Fatalf("rule name not recorded: %+v", recs[0].Prompts)
	}
	// The prompt text must never be persisted.
	var stored string
	if err := api.Service.Store.DB.QueryRowContext(ctx,
		`SELECT GROUP_CONCAT(rule_name, '|') FROM telemetry_prompt_rules`).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(stored, "SECRET-PROMPT-TEXT") {
		t.Fatalf("prompt text leaked into telemetry: %q", stored)
	}
}

func mustRuleID(t *testing.T, api *API, name string) int64 {
	t.Helper()
	rules, err := api.Service.ListPromptRules(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rules {
		if r.Name == name {
			return r.ID
		}
	}
	t.Fatalf("rule %q not found", name)
	return 0
}

func gatewayPostHeader(t *testing.T, server *httptest.Server, key, path, payload, protocol, headerName, headerValue string) int {
	t.Helper()
	req, _ := http.NewRequest("POST", server.URL+path, strings.NewReader(payload))
	if protocol == "anthropic" {
		req.Header.Set("x-api-key", key)
		req.Header.Set("anthropic-version", "2023-06-01")
	} else {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(headerName, headerValue)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	_, _ = io.ReadAll(resp.Body)
	return resp.StatusCode
}

var _ = json.Marshal
