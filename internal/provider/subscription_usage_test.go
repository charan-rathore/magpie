package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"
)

func TestCodexQuotaUsesProviderWindowDuration(t *testing.T) {
	for _, tt := range []struct {
		seconds int64
		want    string
	}{{5 * 60 * 60, "5 hours"}, {7 * 24 * 60 * 60, "7 days"}, {0, "Allowance"}} {
		if got := quotaDurationName(tt.seconds); got != tt.want {
			t.Errorf("quotaDurationName(%d) = %q, want %q", tt.seconds, got, tt.want)
		}
	}

	w := codexWindow{UsedPercent: 16, LimitWindowSecs: 7 * 24 * 60 * 60}
	if got := w.window(); got.Name != "7 days" || got.Used != 16 {
		t.Fatalf("Codex window = %+v", got)
	}
}

func TestCompactQuotaNumber(t *testing.T) {
	for _, tt := range []struct {
		in   float64
		want string
	}{{7, "7"}, {7.5, "7.5"}, {7.25, "7.25"}} {
		if got := compactNumber(tt.in); got != tt.want {
			t.Errorf("compactNumber(%v) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// A stale copy comes back at once while it refreshes, and accounts removed
// from magpie in the meantime are left out of it.
func TestSubscriptionUsageServesStale(t *testing.T) {
	signIn(t)
	if err := Delete("codex"); err != nil {
		t.Fatal(err)
	}
	c := &subscriptionUsageCache
	c.Lock()
	c.at, c.pending = time.Now().Add(-time.Hour), nil
	c.data = []SubscriptionQuota{{Provider: "claude", Name: "Claude Code"}, {Provider: "codex", Name: "Codex"}}
	c.Unlock()
	t.Cleanup(func() {
		for {
			c.Lock()
			p := c.pending
			if p == nil {
				c.data, c.at = nil, time.Time{}
				c.Unlock()
				return
			}
			c.Unlock()
			<-p
		}
	})

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	start := time.Now()
	got := SubscriptionUsage(ctx)
	if time.Since(start) > 40*time.Millisecond {
		t.Fatalf("waited %v for a stale copy", time.Since(start))
	}
	if len(got) != 1 || got[0].Provider != "claude" {
		t.Fatalf("got %+v, want only claude", got)
	}
	c.Lock()
	refreshing := c.pending != nil || time.Since(c.at) < time.Minute
	c.Unlock()
	if !refreshing {
		t.Fatal("a stale copy did not start a refresh")
	}
}

func TestChosenWindows(t *testing.T) {
	ws := []QuotaWindow{{Name: "Gemini 3.8 Flash (High)", Model: "gemini-3.8-flash-high"}, {Name: "Gemini 3 Flash", Model: "gemini-3-flash"}, {Name: "Weekly"}}
	got := chosenWindows(ws, map[string]bool{"gemini-3.8-flash-high": true})
	if len(got) != 2 || got[0].Model != "gemini-3.8-flash-high" || got[1].Name != "Weekly" {
		t.Fatalf("got %+v", got)
	}
	// ids the quota names that none of the enabled ones match: keep them all
	if got := chosenWindows(ws[:2], map[string]bool{"gemini-pro-agent": true}); len(got) != 2 {
		t.Fatalf("kept %d of 2", len(got))
	}
}

// The CLI and Usage page share one collector: a key balance is included even
// when there is no subscription and no bought plan. No real account is needed.
func TestQuotasIncludesKeyBalance(t *testing.T) {
	isolate(t)
	h := t.TempDir()
	t.Setenv("HOME", h)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(h, ".config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(h, ".cache"))
	t.Setenv("PATH", h)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("balance authentication: %q", r.Header.Get("Authorization"))
		}
		w.Write([]byte(`{"balance":12.34}`))
	}))
	defer srv.Close()
	if err := Save(Provider{ID: "relay", Name: "Relay", Chat: srv.URL + "/v1", Key: "test-key", BalanceURL: srv.URL + "/balance", BalancePath: "balance"}); err != nil {
		t.Fatal(err)
	}
	planQuotaCache.Lock()
	planQuotaCache.at, planQuotaCache.data = time.Now(), []SubscriptionQuota{}
	planQuotaCache.Unlock()
	keyBalanceCache.Lock()
	keyBalanceCache.at, keyBalanceCache.data = time.Time{}, nil
	keyBalanceCache.Unlock()
	subscriptionUsageCache.Lock()
	subscriptionUsageCache.at, subscriptionUsageCache.data = time.Now(), []SubscriptionQuota{}
	subscriptionUsageCache.Unlock()
	t.Cleanup(func() {
		subscriptionUsageCache.Lock()
		subscriptionUsageCache.at, subscriptionUsageCache.data = time.Time{}, nil
		subscriptionUsageCache.Unlock()
		keyBalanceCache.Lock()
		keyBalanceCache.at, keyBalanceCache.data = time.Time{}, nil
		keyBalanceCache.Unlock()
		planQuotaCache.Lock()
		planQuotaCache.at, planQuotaCache.data = time.Time{}, nil
		planQuotaCache.Unlock()
	})
	got := Quotas(context.Background())
	if len(got) != 1 || got[0].Provider != "relay" || got[0].Balance != "12.34" || got[0].Windows == nil {
		t.Fatalf("unified quotas: %+v", got)
	}
}
