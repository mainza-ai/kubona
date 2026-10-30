package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/mainza-ai/kubona/core/pkg/ledger"
	"github.com/mainza-ai/kubona/core/pkg/llm"
	"github.com/mainza-ai/kubona/core/pkg/rates"
)

// kubonad rates-audit --db /abs/kubona.db [--json]
//
// Dynamic rate/model audit: read the persisted role assignments, verify each
// cloud model still appears in the provider's live /models response, and
// verify it is priced by the active (runtime) rate card. This is the "capture
// what is actually used" check; the rate card itself is data, not code.
func runRatesAudit(args []string) int {
	fs := flag.NewFlagSet("rates-audit", flag.ContinueOnError)
	dbPath := fs.String("db", "", "path to SQLite WAL ledger (default: ~/.kubona/kubona.db)")
	asJSON := fs.Bool("json", false, "emit machine-readable JSON")
	timeout := fs.Duration("timeout", 15*time.Second, "per-provider /models timeout")
	writeCard := fs.String("write-card", "", "write a minimal used-model card to this absolute path")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	resolved := strings.TrimSpace(*dbPath)
	if resolved == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			fmt.Fprintln(os.Stderr, "error: cannot resolve home directory:", err)
			return 1
		}
		resolved = filepath.Join(home, ".kubona", "kubona.db")
	}
	if !filepath.IsAbs(resolved) {
		fmt.Fprintln(os.Stderr, "error: --db must be an absolute path")
		return 1
	}
	if err := rates.ConfigureFromEnv(context.Background()); err != nil {
		fmt.Fprintln(os.Stderr, "error: rate card:", err)
		return 1
	}
	l, err := ledger.OpenLedgerReadOnly(resolved)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error: open ledger:", err)
		return 1
	}
	defer l.Close()
	settings, err := llm.LoadSettingsReadOnly(l.DB())
	if err != nil {
		fmt.Fprintln(os.Stderr, "error: read LLM settings:", err)
		return 1
	}
	client := llm.NewClient(settings)
	snap := settings.GetSnapshot()
	card := rates.CardSnapshot()

	type auditRole struct {
		Role     string `json:"role"`
		Provider string `json:"provider"`
		Model    string `json:"model"`
		Status   string `json:"status"`
		Detail   string `json:"detail,omitempty"`
	}
	roles := make([]auditRole, 0, len(snap.RoleAssignments))
	used := map[string]rates.ModelRate{}
	var failures []string
	if strings.EqualFold(strings.TrimSpace(os.Getenv("KUBONA_ENV")), "production") && !rates.IsExternal() {
		failures = append(failures, "production cloud pricing requires KUBONA_RATE_CARD_PATH or KUBONA_RATE_CARD_URL (embedded bootstrap refused)")
	}
	for role, assign := range snap.RoleAssignments {
		ar := auditRole{Role: role, Provider: string(assign.Provider), Model: assign.Model, Status: "ok"}
		if llm.IsLocalProvider(assign.Provider) {
			ar.Status = "local"
			roles = append(roles, ar)
			continue
		}
		if strings.TrimSpace(assign.Model) == "" {
			ar.Status = "missing_model"
			ar.Detail = "role has no model configured"
			failures = append(failures, role+": missing model")
			roles = append(roles, ar)
			continue
		}
		if pc, ok := snap.Providers[assign.Provider]; !ok || !pc.Enabled {
			ar.Status = "provider_disabled"
			ar.Detail = "provider disabled or unconfigured (key/model)"
			failures = append(failures, role+": provider disabled")
			roles = append(roles, ar)
			continue
		}
		if key, rate, ok := rates.ModelRateKey(string(assign.Provider), assign.Model); !ok {
			ar.Status = "unpriced"
			ar.Detail = "no active rate-card entry (retired, unknown, or missing provider-qualified key)"
			failures = append(failures, role+": unpriced "+assign.Model)
		} else {
			used[key] = rate
		}
		ctx, cancel := context.WithTimeout(context.Background(), *timeout)
		models, ferr := client.FetchModels(ctx, assign.Provider)
		cancel()
		if ferr != nil {
			ar.Status = "models_unverified"
			ar.Detail = "provider /models unavailable: " + ferr.Error()
			failures = append(failures, role+": /models unavailable")
		} else {
			live := make([]string, 0, len(models))
			found := false
			for _, m := range models {
				live = append(live, m.ID)
				if m.ID == assign.Model {
					found = true
				}
			}
			if !found {
				ar.Status = "retired_or_unavailable"
				ar.Detail = "model absent from live /models"
				failures = append(failures, role+": model not in live /models: "+assign.Model)
			}
			sort.Strings(live)
			if ar.Status == "ok" && len(live) > 0 {
				ar.Detail = fmt.Sprintf("%d live models", len(live))
			}
		}
		roles = append(roles, ar)
	}
	sort.Slice(roles, func(i, j int) bool { return roles[i].Role < roles[j].Role })
	if err := rates.Validate(time.Now().UTC()); err != nil {
		failures = append(failures, "rate card: "+err.Error())
	}
	if *writeCard != "" {
		if len(failures) != 0 {
			fmt.Fprintln(os.Stderr, "rates-audit: refusing to write a card while failures exist")
			return 1
		}
		path := strings.TrimSpace(*writeCard)
		if !filepath.IsAbs(path) {
			fmt.Fprintln(os.Stderr, "rates-audit: --write-card must be an absolute path")
			return 1
		}
		today := time.Now().UTC().Format("2006-01-02")
		capture := rates.CardSnapshot()
		capture.Version++
		capture.Source = "captured:" + rates.Source() + "@" + today
		capture.EffectiveDate = today
		capture.Notes = "Captured only the cloud models assigned in the audited LLM settings; " +
			"vendor evidence and freshness remain the operator's responsibility."
		capture.Models = used
		raw, merr := json.MarshalIndent(capture, "", "  ")
		if merr != nil {
			fmt.Fprintln(os.Stderr, "rates-audit: marshal captured card:", merr)
			return 1
		}
		raw = append(raw, '\n')
		if err := rates.Load(raw, "captured"); err != nil {
			fmt.Fprintln(os.Stderr, "rates-audit: captured card invalid:", err)
			return 1
		}
		if err := rates.Validate(time.Now().UTC()); err != nil {
			fmt.Fprintln(os.Stderr, "rates-audit: captured card refused:", err)
			return 1
		}
		tmp := path + ".tmp"
		if err := os.WriteFile(tmp, raw, 0o600); err != nil {
			fmt.Fprintln(os.Stderr, "rates-audit: write captured card:", err)
			return 1
		}
		if err := os.Rename(tmp, path); err != nil {
			_ = os.Remove(tmp)
			fmt.Fprintln(os.Stderr, "rates-audit: publish captured card:", err)
			return 1
		}
		fmt.Fprintln(os.Stderr, "rates-audit: captured used-model card ->", path)
	}
	out := map[string]any{
		"ok": len(failures) == 0, "db": resolved,
		"rate_source": rates.Source(), "rate_version": card.Version,
		"effective_date": card.EffectiveDate, "heuristic": card.Heuristic,
		"roles": roles, "failures": failures,
	}
	if *asJSON {
		raw, _ := json.MarshalIndent(out, "", "  ")
		fmt.Println(string(raw))
	} else {
		fmt.Printf("rate card: %s (v%d, effective %s, heuristic=%t)\n",
			rates.Source(), card.Version, card.EffectiveDate, card.Heuristic)
		for _, r := range roles {
			fmt.Printf("  %-24s %-12s %-48s %s", r.Role, r.Provider, r.Model, r.Status)
			if r.Detail != "" {
				fmt.Printf(" — %s", r.Detail)
			}
			fmt.Println()
		}
		if len(failures) == 0 {
			fmt.Println("rates-audit: clean")
		} else {
			fmt.Println("rates-audit: FAILED")
			for _, f := range failures {
				fmt.Println("  -", f)
			}
		}
	}
	if len(failures) != 0 {
		return 1
	}
	return 0
}
