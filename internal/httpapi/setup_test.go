package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nawocci/pogu/internal/crypto"
	"github.com/nawocci/pogu/internal/service"
	"github.com/nawocci/pogu/internal/store"
)

func setupFixture(t *testing.T, token string) (*API, *httptest.Server) {
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
	api := New(svc, nil)
	api.EnableSetup(token)
	return api, httptest.NewServer(api.Handler(http.NotFoundHandler()))
}

func postSetup(t *testing.T, server *httptest.Server, payload string) (int, map[string]any) {
	t.Helper()
	req, _ := http.NewRequest("POST", server.URL+"/api/auth/setup", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var v map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&v)
	return resp.StatusCode, v
}

func TestSetupStatus(t *testing.T) {
	_, server := setupFixture(t, "tok")
	defer server.Close()
	resp, err := http.Get(server.URL + "/api/auth/setup")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var v map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&v)
	if resp.StatusCode != 200 || v["setup_required"] != true {
		t.Fatalf("setup status = %d %+v", resp.StatusCode, v)
	}
}

func TestSetupGating(t *testing.T) {
	_, server := setupFixture(t, "tok")
	defer server.Close()
	for path, want := range map[string]int{
		"/api/providers":       http.StatusConflict,
		"/api/auth/login":      http.StatusConflict,
		"/v1/chat/completions": http.StatusServiceUnavailable,
		"/healthz":             http.StatusOK,
	} {
		req, _ := http.NewRequest("POST", server.URL+path, strings.NewReader(`{}`))
		if path == "/healthz" || path == "/api/providers" {
			req, _ = http.NewRequest("GET", server.URL+path, nil)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != want {
			t.Fatalf("%s = %d, want %d", path, resp.StatusCode, want)
		}
	}
}

func TestCompleteSetup(t *testing.T) {
	api, server := setupFixture(t, "tok")
	defer server.Close()

	if code, _ := postSetup(t, server, `{"token":"wrong","password":"0123456789abcdef"}`); code != http.StatusForbidden {
		t.Fatalf("wrong token = %d, want 403", code)
	}
	if code, _ := postSetup(t, server, `{"token":"tok","password":"short"}`); code != http.StatusBadRequest {
		t.Fatalf("short password = %d, want 400", code)
	}

	req, _ := http.NewRequest("POST", server.URL+"/api/auth/setup", strings.NewReader(`{"token":"tok","password":"0123456789abcdef"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var v map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&v)
	if resp.StatusCode != 200 || v["authenticated"] != true {
		t.Fatalf("setup = %d %+v", resp.StatusCode, v)
	}
	authed := false
	for _, c := range resp.Cookies() {
		if c.Name == sessionCookie && c.Value != "" {
			authed = true
		}
	}
	if !authed {
		t.Fatal("setup must establish a session")
	}
	if api.setupRequired() {
		t.Fatal("setup mode must end after completion")
	}

	if code, _ := postSetup(t, server, `{"token":"tok","password":"0123456789abcdef"}`); code != http.StatusConflict {
		t.Fatalf("second setup = %d, want 409", code)
	}

	loginReq, _ := http.NewRequest("POST", server.URL+"/api/auth/login", strings.NewReader(`{"password":"0123456789abcdef"}`))
	loginReq.Header.Set("Content-Type", "application/json")
	loginResp, err := http.DefaultClient.Do(loginReq)
	if err != nil {
		t.Fatal(err)
	}
	loginResp.Body.Close()
	if loginResp.StatusCode != 200 {
		t.Fatalf("login after setup = %d", loginResp.StatusCode)
	}

	statusResp, err := http.Get(server.URL + "/api/auth/setup")
	if err != nil {
		t.Fatal(err)
	}
	defer statusResp.Body.Close()
	var s map[string]any
	_ = json.NewDecoder(statusResp.Body).Decode(&s)
	if s["setup_required"] != false {
		t.Fatalf("setup status after completion = %+v", s)
	}
}
