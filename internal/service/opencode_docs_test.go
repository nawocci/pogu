package service

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const docsFixture = "# Zen\n" +
	"\n" +
	"## Endpoints\n" +
	"\n" +
	"| Model | Model ID | Endpoint | AI SDK Package |\n" +
	"| ----- | -------- | -------- | -------------- |\n" +
	"| Big Pickle | big-pickle | https://opencode.ai/zen/v1/chat/completions | openai-compatible |\n" +
	"| MiMo Free | mimo-v2.5-free | https://opencode.ai/zen/v1/chat/completions | openai-compatible |\n" +
	"| Spark Free | muse-spark-9-contributor-free | https://opencode.ai/zen/v1/responses | openai |\n" +
	"| Claude Paid | claude-paid | https://opencode.ai/zen/v1/messages | anthropic |\n" +
	"\n" +
	"## Pricing\n" +
	"\n" +
	"| Model | Input | Output | Cached Read |\n" +
	"| ----- | ----- | ------ | ----------- |\n" +
	"| Big Pickle | Free | Free | - |\n" +
	"| MiMo Free | Free | Free | - |\n" +
	"| Spark Free | Free | Free | - |\n" +
	"| Claude Paid | $1.00 | $2.00 | - |\n" +
	"| Ghost Model | Free | Free | - |\n"

func TestParseOpenCodeDocs(t *testing.T) {
	docs, err := parseOpenCodeDocs([]byte(docsFixture))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"big-pickle":                    "https://opencode.ai/zen/v1/chat/completions",
		"mimo-v2.5-free":                "https://opencode.ai/zen/v1/chat/completions",
		"muse-spark-9-contributor-free": "https://opencode.ai/zen/v1/responses",
	}
	if len(docs.Endpoints) != len(want) {
		t.Fatalf("endpoints = %+v", docs.Endpoints)
	}
	for id, ep := range want {
		if docs.Endpoints[id] != ep {
			t.Fatalf("endpoints[%q] = %q", id, docs.Endpoints[id])
		}
	}
	if SchemeForDocsEndpoint(docs.Endpoints["muse-spark-9-contributor-free"]) != string(SchemeOpenAIResponses) {
		t.Fatal("responses endpoint must map to openai-responses")
	}
	if SchemeForDocsEndpoint(docs.Endpoints["big-pickle"]) != "" {
		t.Fatal("chat endpoint must inherit")
	}
	if SchemeForDocsEndpoint("https://opencode.ai/zen/v1/messages") != string(SchemeAnthropic) {
		t.Fatal("messages endpoint must map to anthropic")
	}
	if SchemeForDocsEndpoint("https://opencode.ai/zen/v1/models/gemini-x") != "" {
		t.Fatal("unknown endpoint shape must inherit")
	}
}

func TestParseOpenCodeDocsFailures(t *testing.T) {
	for _, body := range []string{
		"",
		"# nothing here",
		"## Endpoints\n\nno table\n\n## Pricing\n\n| Model | Input | Output |\n|---|---|---|\n| X | Free | Free |\n",
		"## Endpoints\n\n| Model | Model ID | Endpoint |\n|---|---|---|\n| X | x | `e` |\n\n## Pricing\n\nno table\n",
		"## Endpoints\n\n| Model | Model ID | Endpoint |\n|---|---|---|\n| X | x | `e` |\n\n## Pricing\n\n| Model | Input | Output |\n|---|---|---|\n| X | $1 | $2 |\n",
	} {
		if _, err := parseOpenCodeDocs([]byte(body)); err == nil {
			t.Fatalf("body %q must fail", body)
		}
	}
}

func TestFetchOpenCodeDocsFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	s, _ := testService(t)
	s.OpenCodeDocsURL = srv.URL
	if _, err := fetchOpenCodeDocs(t.Context(), s.openCodeDocsURL()); err == nil {
		t.Fatal("5xx must fail")
	}
	if !strings.Contains(errText(fetchOpenCodeDocs(t.Context(), s.openCodeDocsURL())), "status 500") {
		t.Fatal("expected status in error")
	}
}

func errText(_ DocsModels, err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
