package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestCodexRoutingUsesWorkspaceLoginName(t *testing.T) {
	home := signIn(t)
	codexWorkspaceSignIn(t, home, "me@example.com", "ws-one", "r-one")
	rememberLogins(true)
	codexWorkspaceSignIn(t, home, "me@example.com", "ws-two", "r-two")
	rememberLogins(true)
	users, active := loginUsers(Logins("codex"))
	other := users[0]
	if other == active {
		other = users[1]
	}
	if err := SetLoginOn("codex", other, true); err != nil {
		t.Fatal(err)
	}
	p, ok := codexAccount(home)
	if !ok {
		t.Fatal("active Codex account missing")
	}
	candidates := append([]Provider{p}, p.AlsoOn()...)
	if len(candidates) != 2 {
		t.Fatalf("routing candidates: %d", len(candidates))
	}
	seen := map[string]bool{}
	for _, candidate := range candidates {
		if seen[candidate.Account.User] {
			t.Fatalf("two workspace candidates share %q", candidate.Account.User)
		}
		seen[candidate.Account.User] = true
	}
	if p.Account.User != active {
		t.Fatalf("routing account %q, active saved account %q", p.Account.User, active)
	}
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		used := 99
		if r.Header.Get("chatgpt-account-id") == "ws-two" {
			used = 10
		}
		json.NewEncoder(w).Encode(map[string]any{"plan_type": "team", "rate_limit": map[string]any{
			"primary_window": map[string]any{"used_percent": used, "limit_window_seconds": 18000}}})
	}))
	defer fake.Close()
	old := CodexBase
	CodexBase = fake.URL + "/backend-api/codex"
	t.Cleanup(func() { CodexBase = old })
	resetUsed(t)
	// The same per-user lookup routing uses must retain this workspace's own quota.
	LoginUsage(context.Background(), "codex")
	allowances := Allowances("codex")
	if used, _ := allowances[p.Account.User].For("gpt-5.5", time.Now()); used != 10 {
		t.Fatalf("active workspace allowance: %v, want 10", used)
	}
	if used, _ := allowances[other].For("gpt-5.5", time.Now()); used != 99 {
		t.Fatalf("other workspace allowance: %v, want 99", used)
	}
}
