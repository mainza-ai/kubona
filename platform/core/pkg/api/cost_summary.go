package api

import (
	"net/http"
)

// handleCostSummary serves the read-only LLM spend summary (G10/Phase 3):
// global cap (0 = track-only), summed cloud estimates, remaining (null when
// track-only), blind-stream count, per-provider subtotals, and whether the
// active provider bypasses metering as local. Public read (aggregates only,
// heuristic-labeled); no spend mutation, no secrets.
func (s *Server) handleCostSummary(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed: refusing", http.StatusMethodNotAllowed)
		return
	}
	spent, byProvider, unmetered := s.llmSpendSummary()
	cap := llmBudgetCapUSD()
	var remaining *float64
	if cap > 0 {
		v := cap - spent
		remaining = &v
	}
	snap := s.llmSettings.GetPublicSnapshot()
	localBypass := false
	if prov, ok := snap.Providers[snap.ActiveProvider]; ok && prov.IsLocal {
		localBypass = true
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"cap_usd":         cap,
		"spent_usd":       spent,
		"remaining_usd":   remaining,
		"unmetered_count": unmetered,
		"by_provider":     byProvider,
		"active_provider": string(snap.ActiveProvider),
		"local_bypass":    localBypass,
		"heuristic":       true,
		"currency":        "USD",
		"track_only":      cap <= 0,
	})
}
