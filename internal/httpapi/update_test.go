package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"testing"
)

func authedClient(t *testing.T, base string) *http.Client {
	t.Helper()
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	body := strings.NewReader(`{"password":"0123456789abcdef"}`)
	req, _ := http.NewRequest("POST", base+"/api/auth/login", body)
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("login = %d", resp.StatusCode)
	}
	return client
}

func putJSON(t *testing.T, client *http.Client, url, payload string) (int, map[string]any) {
	t.Helper()
	req, _ := http.NewRequest("PUT", url, strings.NewReader(payload))
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

func TestUpdatePreservesEnabled(t *testing.T) {
	oa := &mockUpstream{protocol: "openai", key: "k1"}
	an := &mockUpstream{protocol: "anthropic", key: "k2"}
	e := newE2E(t, oa, an)
	defer e.server.Close()
	client := authedClient(t, e.server.URL)

	code, p := putJSON(t, client, e.server.URL+"/api/providers/1",
		`{"name":"OA","type":"openai","prefix":"oa","base_url":"http://x.test","key_selection":"first","enabled":false}`)
	if code != 200 || p["enabled"] != false {
		t.Fatalf("disable provider = %d %+v", code, p)
	}
	code, p = putJSON(t, client, e.server.URL+"/api/providers/1",
		`{"name":"OA renamed","type":"openai","prefix":"oa","base_url":"http://x.test","key_selection":"first"}`)
	if code != 200 || p["enabled"] != false || p["name"] != "OA renamed" {
		t.Fatalf("rename must preserve disabled = %d %+v", code, p)
	}

	code, m := putJSON(t, client, e.server.URL+"/api/models/1", `{"provider_id":1,"name":"alpha","enabled":false}`)
	if code != 200 || m["enabled"] != false {
		t.Fatalf("disable model = %d %+v", code, m)
	}
	code, m = putJSON(t, client, e.server.URL+"/api/models/1", `{"provider_id":1,"name":"alpha2"}`)
	if code != 200 || m["enabled"] != false || m["name"] != "alpha2" {
		t.Fatalf("rename must preserve disabled = %d %+v", code, m)
	}

	code, k := putJSON(t, client, e.server.URL+"/api/providers/1/keys/1", `{"name":"Key 1","enabled":false}`)
	if code != 200 || k["enabled"] != false {
		t.Fatalf("disable key = %d %+v", code, k)
	}
	code, k = putJSON(t, client, e.server.URL+"/api/providers/1/keys/1", `{"name":"Renamed"}`)
	if code != 200 || k["enabled"] != false || k["name"] != "Renamed" {
		t.Fatalf("rename must preserve disabled = %d %+v", code, k)
	}

	code, _ = putJSON(t, client, e.server.URL+"/api/groups/1", `{"name":"g","selection":"first"}`)
	if code != 404 {
		t.Fatalf("missing group = %d", code)
	}
}
