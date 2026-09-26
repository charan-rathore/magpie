package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/yetone/magpie/internal/provider"
)

// quotasCmd shows the vendor-reported allowances and balances in the same
// order and shape as the Usage page. --json is for scripts and agents.
func quotasCmd(args []string) error {
	if len(args) > 2 || len(args) == 2 && args[1] != "--json" {
		return fmt.Errorf("usage: magpie quotas [--json]")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	qs := provider.Quotas(ctx)
	if len(args) == 2 {
		return json.NewEncoder(os.Stdout).Encode(qs)
	}
	if len(qs) == 0 {
		fmt.Println("no provider allowances or balances available")
		return nil
	}
	for _, q := range qs {
		fmt.Println(quotaLine(q))
	}
	return nil
}

func quotaLine(q provider.SubscriptionQuota) string {
	name := q.Name
	if name == "" {
		name = q.Provider
	}
	if q.User != "" {
		name += " (" + q.User + ")"
	}
	if q.Plan != "" {
		name += " · " + q.Plan
	}
	var parts []string
	for _, w := range q.Windows {
		part := fmt.Sprintf("%s %.0f%% left", w.Name, max(0, 100-w.Used))
		if w.ResetsAt != nil {
			part += " · resets " + w.ResetsAt.Local().Format("Jan 2 15:04")
		} else if w.ResetSecs > 0 {
			part += " · resets in " + untilShort(time.Duration(w.ResetSecs)*time.Second)
		}
		parts = append(parts, part)
	}
	if q.Balance != "" {
		parts = append(parts, "balance "+q.Balance)
	}
	if q.Error != "" {
		parts = append(parts, "error: "+q.Error)
	}
	if len(parts) == 0 {
		parts = append(parts, "no allowance reported")
	}
	return name + ": " + strings.Join(parts, "; ")
}
