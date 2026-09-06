package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"testing"

	"github.com/nawocci/pogu/internal/service"
)

func TestGroupRoutingAndFailover(t *testing.T) {
	oa := &mockUpstream{protocol: "openai", key: "k1"}
	an := &mockUpstream{protocol: "anthropic", key: "k2"}
	e := newE2E(t, oa, an)
	defer e.server.Close()
	ctx := e.ctx

	provs, _ := e.api.Service.ListProviders(ctx)
	ids := map[string]int64{}
	for _, p := range provs {
		ids[p.Prefix] = p.ID
	}
	models, _ := e.api.Service.ListModels(ctx)
	mids := map[string]int64{}
	for _, m := range models {
		mids[m.PublicID] = m.ID
	}
	g, err := e.api.Service.CreateGroup(ctx, "pool", true, service.KeySelectionFirst)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"oa/alpha", "an/gamma"} {
		if _, err := e.api.Service.AddGroupMember(ctx, g.ID, mids[id]); err != nil {
			t.Fatal(err)
		}
	}

	status, body, _ := e.post(t, "/v1/chat/completions", "openai", `{"model":"pool","messages":[{"role":"user","content":"hi"}]}`)
	if status != 200 || !strings.Contains(body, "hello") {
		t.Fatalf("group route = %d %s", status, body)
	}
	recs, err := e.api.Telemetry.List(ctx, 10)
	if err != nil || len(recs) != 1 {
		t.Fatalf("telemetry = %+v, %v", recs, err)
	}
	if recs[0].ResolvedModel == nil || *recs[0].ResolvedModel != "oa/alpha" {
		t.Fatalf("resolution = %+v", recs[0].Request)
	}
	if recs[0].GroupName == nil || *recs[0].GroupName != "pool" {
		t.Fatalf("group snapshot = %+v", recs[0].Request)
	}

	oa.failNext.Store(2)
	status, body, _ = e.post(t, "/v1/chat/completions", "openai", `{"model":"pool","messages":[{"role":"user","content":"hi"}]}`)
	if status != 200 || !strings.Contains(body, `"model":"gamma"`) {
		t.Fatalf("group failover = %d %s", status, body)
	}
	all, _ := e.api.Telemetry.List(ctx, 10)
	var attempts int
	for _, r := range all {
		if r.GroupName != nil {
			attempts += len(r.Attempts)
		}
	}
	if attempts != 3 {
		t.Fatalf("group failover attempts = %+v", all)
	}

	status, body, _ = e.post(t, "/v1/chat/completions", "openai", `{"model":"empty","messages":[]}`)
	if status != 404 {
		t.Fatalf("unknown group = %d %s", status, body)
	}
	if _, err := e.api.Service.CreateGroup(ctx, "void", true, service.KeySelectionFirst); err != nil {
		t.Fatal(err)
	}
	status, body, _ = e.post(t, "/v1/chat/completions", "openai", `{"model":"void","messages":[]}`)
	if status != 503 || !strings.Contains(body, "no available targets") {
		t.Fatalf("empty group = %d %s", status, body)
	}
}

func TestGroupRoundRobinGateway(t *testing.T) {
	oa := &mockUpstream{protocol: "openai", key: "k1"}
	an := &mockUpstream{protocol: "anthropic", key: "k2"}
	e := newE2E(t, oa, an)
	defer e.server.Close()
	ctx := e.ctx

	models, _ := e.api.Service.ListModels(ctx)
	mids := map[string]int64{}
	for _, m := range models {
		mids[m.PublicID] = m.ID
	}
	g, err := e.api.Service.CreateGroup(ctx, "rr", true, service.KeySelectionRoundRobin)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"oa/alpha", "an/gamma"} {
		if _, err := e.api.Service.AddGroupMember(ctx, g.ID, mids[id]); err != nil {
			t.Fatal(err)
		}
	}
	var served []string
	for i := 0; i < 2; i++ {
		status, _, _ := e.post(t, "/v1/chat/completions", "openai", `{"model":"rr","messages":[{"role":"user","content":"hi"}]}`)
		if status != 200 {
			t.Fatalf("rr request %d = %d", i, status)
		}
		recs, _ := e.api.Telemetry.List(ctx, 10)
		served = append(served, *recs[0].ResolvedModel)
	}
	if served[0] == served[1] {
		t.Fatalf("round robin did not rotate: %v", served)
	}
}

func TestMonitoringEndpoints(t *testing.T) {
	oa := &mockUpstream{protocol: "openai", key: "k1"}
	an := &mockUpstream{protocol: "anthropic", key: "k2"}
	e := newE2E(t, oa, an)
	defer e.server.Close()

	status, _, _ := e.post(t, "/v1/chat/completions", "openai", `{"model":"oa/alpha","messages":[{"role":"user","content":"hi"}]}`)
	if status != 200 {
		t.Fatalf("seed request = %d", status)
	}

	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginBody := strings.NewReader(`{"password":"0123456789abcdef"}`)
	req, _ := http.NewRequest("POST", e.server.URL+"/api/auth/login", loginBody)
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("login = %d", resp.StatusCode)
	}
	get := func(path string) (int, map[string]any) {
		t.Helper()
		req, _ := http.NewRequest("GET", e.server.URL+path, nil)
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var v map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&v)
		return resp.StatusCode, v
	}

	if code, v := get("/api/monitoring/summary"); code != 200 || int(v["current"].(map[string]any)["requests"].(float64)) != 1 {
		t.Fatalf("summary = %d %+v", code, v)
	}
	if code, v := get("/api/monitoring/timeseries"); code != 200 || len(v["buckets"].([]any)) == 0 {
		t.Fatalf("timeseries = %d", code)
	}
	if code, v := get("/api/monitoring/requests"); code != 200 || int(v["total"].(float64)) != 1 {
		t.Fatalf("requests = %d %+v", code, v)
	}
	if code, _ := get("/api/monitoring/summary?range=bogus"); code != 400 {
		t.Fatalf("bad range = %d", code)
	}
	if code, _ := get("/api/providers/1/sync"); code != 404 && code != 405 {
		t.Fatalf("sync wrong method = %d", code)
	}
}
