package main

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/yetone/magpie/internal/provider"
)

func TestQuotaLine(t *testing.T) {
	at := time.Date(2026, 9, 27, 8, 30, 0, 0, time.Local)
	q := provider.SubscriptionQuota{
		Provider: "claude", Name: "Claude", User: "first@example.com", Plan: "Pro",
		Windows: []provider.QuotaWindow{{Name: "5 hours", Used: 42, ResetsAt: &at}},
	}
	got := quotaLine(q)
	for _, want := range []string{"Claude (first@example.com) · Pro", "5 hours 58% left", "resets Sep 27 08:30"} {
		if !strings.Contains(got, want) {
			t.Errorf("quotaLine = %q, missing %q", got, want)
		}
	}
	if got := quotaLine(provider.SubscriptionQuota{Name: "Relay", Balance: "$12.34", Error: "offline"}); got != "Relay: balance $12.34; error: offline" {
		t.Fatal(got)
	}
}

func TestQuotaJSONKeepsVendorFields(t *testing.T) {
	qs := []provider.SubscriptionQuota{{Provider: "relay", Balance: "$12.34", Windows: []provider.QuotaWindow{{Name: "daily", Used: 25}}}}
	b, err := json.Marshal(qs)
	if err != nil {
		t.Fatal(err)
	}
	var decoded []provider.SubscriptionQuota
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatal(err)
	}
	if len(decoded) != 1 || decoded[0].Balance != "$12.34" || decoded[0].Windows[0].Used != 25 {
		t.Fatal(string(b))
	}
}
