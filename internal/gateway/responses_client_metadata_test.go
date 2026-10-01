package gateway

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/yetone/magpie/internal/provider"
)

func TestResponsesClientMetadataRoundTrip(t *testing.T) {
	for _, metadata := range []string{`{"session_id":"s1","nested":{"count":9007199254740993,"enabled":true}}`, `{}`, `null`} {
		body := []byte(`{"model":"m1","input":"hi","client_metadata":` + metadata + `}`)
		r, err := parseResponses(body)
		if err != nil {
			t.Fatal(err)
		}
		var got map[string]json.RawMessage
		if err := json.Unmarshal(buildResponses(r, "m1", "relay.example", false), &got); err != nil {
			t.Fatal(err)
		}
		if string(got["client_metadata"]) != metadata {
			t.Fatalf("metadata: got %s, want %s", got["client_metadata"], metadata)
		}
		var other map[string]json.RawMessage
		if err := json.Unmarshal(buildChat(r, "m1", "relay.example", false), &other); err != nil {
			t.Fatal(err)
		}
		if _, ok := other["client_metadata"]; ok {
			t.Fatal("Responses-only metadata leaked to Chat")
		}
	}
	r, err := parseResponses([]byte(`{"model":"m1","input":"hi"}`))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(buildResponses(r, "m1", "relay.example", false)), `"client_metadata"`) {
		t.Fatal("absent metadata was added")
	}
}

func TestResponsesWebSearchKeepsClientMetadata(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	f := &fake{t: t, reply: sse(
		`data: {"type":"response.created","response":{"id":"resp_r1","status":"in_progress","output":[]}}`,
		`data: {"type":"response.output_item.done","output_index":0,"item":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"OK"}]}}`,
		`data: {"type":"response.completed","response":{"id":"resp_r1","status":"completed","output":[]}}`)}
	setup(t, provider.Responses, f)
	if _, _, ok := searcher(); ok {
		t.Fatal("unexpected searcher")
	}
	code, body := post(t, "/v1/responses", `{"model":"fake/m1","input":"hi","stream":true,"tools":[{"type":"web_search"}],"client_metadata":{"session_id":"s1"},"prompt_cache_key":"thread-1"}`)
	if code != 200 {
		t.Fatalf("%d %s", code, body)
	}
	var got struct {
		Metadata map[string]string `json:"client_metadata"`
		CacheKey string            `json:"prompt_cache_key"`
		Tools    []any             `json:"tools"`
	}
	if err := json.Unmarshal(f.got, &got); err != nil {
		t.Fatal(err)
	}
	if got.Metadata["session_id"] != "s1" || got.CacheKey != "thread-1" || len(got.Tools) != 0 {
		t.Fatalf("translated request: %s", f.got)
	}
}
