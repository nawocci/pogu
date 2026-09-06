package httpapi

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/nawocci/pogu/internal/crypto"
	"github.com/nawocci/pogu/internal/service"
	"github.com/nawocci/pogu/internal/store"
)

type captureStub struct {
	lastBody atomic.Value
}

func (c *captureStub) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		c.lastBody.Store(string(body))
		var env struct {
			Model string `json:"model"`
		}
		_ = json.Unmarshal(body, &env)
		if strings.HasSuffix(r.URL.Path, "/messages") {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{"id":"m","type":"message","role":"assistant","model":%q,"content":[{"type":"text","text":"hi"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`, env.Model)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"id":"c","object":"chat.completion","created":1,"model":%q,"choices":[{"index":0,"message":{"role":"assistant","content":"hi"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`, env.Model)
	})
}

func promptFixture(t *testing.T, scheme service.ProviderType, stub *captureStub) (*API, *httptest.Server, string) {
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
	if _, err := svc.CreateModel(ctx, p.ID, "m", true); err != nil {
		t.Fatal(err)
	}
	_, secret, err := svc.CreateAPIKey(ctx, "app")
	if err != nil {
		t.Fatal(err)
	}
	api := New(svc, nil)
	return api, httptest.NewServer(api.Handler(http.NotFoundHandler())), secret
}

func gatewayPost(t *testing.T, server *httptest.Server, key, path, payload, header string) int {
	t.Helper()
	req, _ := http.NewRequest("POST", server.URL+path, strings.NewReader(payload))
	if header == "anthropic" {
		req.Header.Set("x-api-key", key)
		req.Header.Set("anthropic-version", "2023-06-01")
	} else {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	_, _ = io.ReadAll(resp.Body)
	return resp.StatusCode
}

func TestGatewayInjectsGlobalPrompt(t *testing.T) {
	stub := &captureStub{}
	api, server, key := promptFixture(t, service.ProviderOpenAI, stub)
	defer server.Close()
	if err := api.Service.SetGlobalPrompt(t.Context(), "Always be concise.", true); err != nil {
		t.Fatal(err)
	}
	status := gatewayPost(t, server, key, "/v1/chat/completions",
		`{"model":"p/m","messages":[{"role":"system","content":"You are helpful."},{"role":"user","content":"hi"}]}`, "openai")
	if status != 200 {
		t.Fatalf("gateway = %d", status)
	}
	seen, _ := stub.lastBody.Load().(string)
	sysIdx := strings.Index(seen, "You are helpful.")
	promptIdx := strings.Index(seen, "Always be concise.")
	if sysIdx < 0 || promptIdx < 0 || sysIdx > promptIdx {
		t.Fatalf("injection order wrong: %s", seen)
	}
	if !strings.Contains(seen, `"model":"m"`) {
		t.Fatalf("model rewrite missing: %s", seen)
	}
}

func TestGatewayInjectsAnthropicRawBody(t *testing.T) {
	stub := &captureStub{}
	api, server, key := promptFixture(t, service.ProviderAnthropic, stub)
	defer server.Close()
	if err := api.Service.SetGlobalPrompt(t.Context(), "Always be concise.", true); err != nil {
		t.Fatal(err)
	}
	status := gatewayPost(t, server, key, "/v1/messages",
		`{"model":"p/m","max_tokens":16,"system":"Original.","messages":[{"role":"user","content":"hi"}]}`, "anthropic")
	if status != 200 {
		t.Fatalf("gateway = %d", status)
	}
	seen, _ := stub.lastBody.Load().(string)
	if !strings.HasPrefix(seen[strings.Index(seen, `"system"`):], `"system":"Original.`) {
		t.Fatalf("original system damaged: %s", seen)
	}
	if !strings.Contains(seen, "Always be concise.") {
		t.Fatalf("prompt missing: %s", seen)
	}
}

func TestGatewaySkipsDisabledPrompt(t *testing.T) {
	stub := &captureStub{}
	api, server, key := promptFixture(t, service.ProviderOpenAI, stub)
	defer server.Close()
	if err := api.Service.SetGlobalPrompt(t.Context(), "Always be concise.", false); err != nil {
		t.Fatal(err)
	}
	status := gatewayPost(t, server, key, "/v1/chat/completions",
		`{"model":"p/m","messages":[{"role":"user","content":"hi"}]}`, "openai")
	if status != 200 {
		t.Fatalf("gateway = %d", status)
	}
	seen, _ := stub.lastBody.Load().(string)
	if strings.Contains(seen, "Always be concise.") {
		t.Fatalf("disabled prompt leaked: %s", seen)
	}
}

func TestSettingsEndpoints(t *testing.T) {
	stub := &captureStub{}
	api, server, _ := promptFixture(t, service.ProviderOpenAI, stub)
	defer server.Close()
	client := authedClient(t, server.URL)

	get := func() (int, map[string]any) {
		t.Helper()
		req, _ := http.NewRequest("GET", server.URL+"/api/settings/prompt", nil)
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var v map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&v)
		return resp.StatusCode, v
	}
	put := func(payload string) (int, map[string]any) {
		t.Helper()
		req, _ := http.NewRequest("PUT", server.URL+"/api/settings/prompt", strings.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var v map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&v)
		return resp.StatusCode, v
	}
	if code, v := get(); code != 200 || v["system_prompt"] != "" || v["enabled"] != false {
		t.Fatalf("defaults = %d %+v", code, v)
	}
	if code, v := put(`{"system_prompt":"Be nice.","enabled":true}`); code != 200 || v["system_prompt"] != "Be nice." || v["enabled"] != true {
		t.Fatalf("update = %d %+v", code, v)
	}
	if code, _ := put(`{"system_prompt":123}`); code != 400 {
		t.Fatalf("bad shape = %d", code)
	}
	_ = api
}

func TestChangePasswordEndpoint(t *testing.T) {
	stub := &captureStub{}
	_, server, _ := promptFixture(t, service.ProviderOpenAI, stub)
	defer server.Close()
	client := authedClient(t, server.URL)

	post := func(payload string) int {
		t.Helper()
		req, _ := http.NewRequest("POST", server.URL+"/api/auth/password", strings.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		_, _ = io.ReadAll(resp.Body)
		return resp.StatusCode
	}
	if code := post(`{"current_password":"wrong!","new_password":"new-password-123"}`); code != 401 {
		t.Fatalf("wrong current = %d", code)
	}
	if code := post(`{"current_password":"0123456789abcdef","new_password":"short"}`); code != 400 {
		t.Fatalf("short next = %d", code)
	}
	if code := post(`{"current_password":"0123456789abcdef","new_password":"new-password-123"}`); code != 200 {
		t.Fatalf("change = %d", code)
	}
	req, _ := http.NewRequest("GET", server.URL+"/api/auth/me", nil)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var me struct {
		Authenticated bool `json:"authenticated"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&me)
	if me.Authenticated {
		t.Fatal("rotation must kill the session")
	}
}
