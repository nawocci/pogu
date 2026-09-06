package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/nawocci/pogu/internal/crypto"
	"github.com/nawocci/pogu/internal/service"
	"github.com/nawocci/pogu/internal/store"
	"github.com/nawocci/pogu/internal/telemetry"
)

type mockUpstream struct {
	protocol string
	key      string
	failNext atomic.Int32
}

func (m *mockUpstream) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/models" {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"object":"list","data":[]}`)
			return
		}
		if m.protocol == "openai" {
			if r.Header.Get("Authorization") != "Bearer "+m.key {
				http.Error(w, `{"error":{"message":"bad key"}}`, http.StatusUnauthorized)
				return
			}
		} else {
			if r.Header.Get("x-api-key") != m.key {
				w.WriteHeader(http.StatusUnauthorized)
				fmt.Fprint(w, `{"type":"error","error":{"type":"authentication_error","message":"bad key"}}`)
				return
			}
		}
		if m.failNext.Add(-1) >= 0 {
			http.Error(w, `{"error":{"message":"denied"}}`, http.StatusUnauthorized)
			return
		}
		body, _ := io.ReadAll(r.Body)
		var env struct {
			Model  string `json:"model"`
			Stream bool   `json:"stream"`
		}
		_ = json.Unmarshal(body, &env)
		if m.protocol == "openai" {
			if env.Stream {
				w.Header().Set("Content-Type", "text/event-stream")
				fmt.Fprintf(w, "data: {\"id\":\"c\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":%q,\"choices\":[{\"index\":0,\"delta\":{\"content\":\"Hel\"}}]}\n\n", env.Model)
				fmt.Fprintf(w, "data: {\"id\":\"c\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":%q,\"choices\":[{\"index\":0,\"delta\":{}}],\"usage\":{\"prompt_tokens\":4,\"completion_tokens\":4,\"total_tokens\":8}}\n\ndata: [DONE]\n\n", env.Model)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{"id":"c","object":"chat.completion","created":1,"model":%q,"choices":[{"index":0,"message":{"role":"assistant","content":"hello"},"finish_reason":"stop"}],"usage":{"prompt_tokens":4,"completion_tokens":4,"total_tokens":8}}`, env.Model)
			return
		}
		if env.Stream {
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprintf(w, "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"m\",\"model\":%q,\"usage\":{\"input_tokens\":4,\"output_tokens\":0}}}\n\n", env.Model)
			fmt.Fprint(w, "event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"Hel\"}}\n\n")
			fmt.Fprint(w, "event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":4}}\n\n")
			fmt.Fprint(w, "event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"id":"m","type":"message","role":"assistant","model":%q,"content":[{"type":"text","text":"hello"}],"stop_reason":"end_turn","usage":{"input_tokens":4,"output_tokens":4}}`, env.Model)
	})
}

type e2e struct {
	api    *API
	server *httptest.Server
	key    string
	ctx    context.Context
}

func newE2E(t *testing.T, oa, an *mockUpstream) *e2e {
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
	ctx := context.Background()
	if err := svc.InitializeAdmin(ctx, "0123456789abcdef"); err != nil {
		t.Fatal(err)
	}
	idOf := map[string]int64{}
	for _, item := range []struct {
		name string
		mock *mockUpstream
		typ  service.ProviderType
	}{
		{"OA", oa, service.ProviderOpenAI},
		{"AN", an, service.ProviderAnthropic},
	} {
		srv := httptest.NewServer(item.mock.handler())
		t.Cleanup(srv.Close)
		p, err := svc.CreateProvider(ctx, item.name, item.typ, strings.ToLower(item.name), srv.URL, item.mock.key, true)
		if err != nil {
			t.Fatal(err)
		}
		idOf[item.name] = p.ID
	}
	if _, err := svc.CreateModel(ctx, idOf["OA"], "alpha", true); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreateModel(ctx, idOf["AN"], "gamma", true); err != nil {
		t.Fatal(err)
	}
	_, secret, err := svc.CreateAPIKey(ctx, "app")
	if err != nil {
		t.Fatal(err)
	}
	api := New(svc, slog.New(slog.DiscardHandler))
	return &e2e{api: api, server: httptest.NewServer(api.Handler(http.NotFoundHandler())), key: secret, ctx: ctx}
}

func (e *e2e) post(t *testing.T, path, inlet, payload string) (int, string, http.Header) {
	t.Helper()
	req, _ := http.NewRequest("POST", e.server.URL+path, strings.NewReader(payload))
	if inlet == "anthropic" {
		req.Header.Set("x-api-key", e.key)
		req.Header.Set("anthropic-version", "2023-06-01")
	} else {
		req.Header.Set("Authorization", "Bearer "+e.key)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(body), resp.Header
}

func TestGatewayFourDirections(t *testing.T) {
	oa := &mockUpstream{protocol: "openai", key: "k1"}
	an := &mockUpstream{protocol: "anthropic", key: "k2"}
	e := newE2E(t, oa, an)
	defer e.server.Close()
	cases := []struct{ path, inlet, model, want string }{
		{"/v1/chat/completions", "openai", "oa/alpha", `"content":"hello"`},
		{"/v1/chat/completions", "openai", "an/gamma", `"content":"hello"`},
		{"/v1/messages", "anthropic", "an/gamma", `"text":"hello"`},
		{"/v1/messages", "anthropic", "oa/alpha", `"text":"hello"`},
	}
	for _, c := range cases {
		var payload string
		if c.inlet == "anthropic" {
			payload = fmt.Sprintf(`{"model":%q,"max_tokens":16,"messages":[{"role":"user","content":"hi"}]}`, c.model)
		} else {
			payload = fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"hi"}]}`, c.model)
		}
		status, body, hdr := e.post(t, c.path, c.inlet, payload)
		if status != 200 || !strings.Contains(body, c.want) {
			t.Errorf("%s %s = %d %s", c.path, c.model, status, body)
		}
		if hdr.Get("X-Request-Id") == "" {
			t.Errorf("%s %s missing request id", c.path, c.model)
		}
	}
	recs, err := e.api.Telemetry.List(e.ctx, 10)
	if err != nil || len(recs) != 4 {
		t.Fatalf("telemetry = %d, %v", len(recs), err)
	}
	for _, r := range recs {
		if r.Status != telemetry.StatusSuccess || r.HTTPStatus == nil || *r.HTTPStatus != 200 {
			t.Fatalf("bad record: %+v", r.Request)
		}
		if r.Usage.Input == nil || *r.Usage.Input != 4 {
			t.Fatalf("bad usage: %+v", r.Usage)
		}
	}
}

func TestGatewayFailover(t *testing.T) {
	oa := &mockUpstream{protocol: "openai", key: "k1"}
	an := &mockUpstream{protocol: "anthropic", key: "k2"}
	e := newE2E(t, oa, an)
	defer e.server.Close()
	ctx := e.ctx
	provs, _ := e.api.Service.ListProviders(ctx)
	var oaID int64
	for _, p := range provs {
		if p.Prefix == "oa" {
			oaID = p.ID
		}
	}
	bad, err := e.api.Service.CreateProviderKey(ctx, oaID, "bad", "wrong")
	if err != nil {
		t.Fatal(err)
	}
	if err := e.api.Service.SetProviderKeyPrimary(ctx, oaID, bad.ID); err != nil {
		t.Fatal(err)
	}
	status, body, _ := e.post(t, "/v1/chat/completions", "openai", `{"model":"oa/alpha","messages":[{"role":"user","content":"hi"}]}`)
	if status != 200 || !strings.Contains(body, "hello") {
		t.Fatalf("failover = %d %s", status, body)
	}
	recs, err := e.api.Telemetry.List(ctx, 10)
	if err != nil || len(recs) != 1 || len(recs[0].Attempts) != 2 {
		t.Fatalf("attempts = %+v, %v", recs, err)
	}
	if recs[0].Attempts[0].ErrorCategory != telemetry.ErrAuth || !recs[0].Attempts[1].Success {
		t.Fatalf("categories wrong: %+v", recs[0].Attempts)
	}
}

func TestGatewayErrors(t *testing.T) {
	oa := &mockUpstream{protocol: "openai", key: "k1"}
	an := &mockUpstream{protocol: "anthropic", key: "k2"}
	e := newE2E(t, oa, an)
	defer e.server.Close()
	status, _, _ := e.post(t, "/v1/chat/completions", "openai", `{"model":"zz/nope","messages":[]}`)
	if status != 404 {
		t.Fatalf("unknown model = %d", status)
	}
	req, _ := http.NewRequest("POST", e.server.URL+"/v1/chat/completions", strings.NewReader(`{"model":"oa/alpha","messages":[]}`))
	req.Header.Set("Authorization", "Bearer bogus")
	req.Header.Set("Content-Type", "application/json")
	resp, _ := http.DefaultClient.Do(req)
	resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Fatalf("bad key = %d", resp.StatusCode)
	}
	req2, _ := http.NewRequest("GET", e.server.URL+"/v1/models", nil)
	req2.Header.Set("Authorization", "Bearer "+e.key)
	resp2, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	var listed struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp2.Body).Decode(&listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.Data) != 2 || listed.Data[0].ID != "oa/alpha" {
		t.Fatalf("models = %+v", listed)
	}
}
