package llm

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/mainza-ai/kubona/core/pkg/rates"
)

// Cloud LLM spend accounting (K4). Tribunal and copilot turns can call
// metered cloud models (opencode/deepseek, anthropic, openai); without a
// dollar rail that spend is invisible and uncapped — the same defect
// cost-governance fixed for QPU time. This file prices measured token
// usage; the server records per-call events and enforces an optional
// global cap (KUBONA_LLM_BUDGET_USD).
//
// Rates live in platform/contracts/rate-cards.json (embedded by pkg/rates).
// The card is explicitly heuristic until a vendor sheet replaces it; a stale
// card refuses to price. Unknown models still return ok=false — refusing to
// price beats inventing a rate.
// isLocalProvider is the single home for the local set — a new local
// engine updates this one predicate, not every call site (drift-proofing
// per the production bar).
func isLocalProvider(p ProviderType) bool {
	switch p {
	case ProviderOMLX, ProviderOllama, ProviderLMStudio, ProviderStrata:
		return true
	}
	return false
}

// IsLocalProvider reports whether the provider runs on loopback/on-device
// with no metered spend (free by construction, never priced). Exported so
// the api rail shares the single home instead of copying the list (audit
// 2026-10-21: server.go carried its own triple and missed Strata — a Strata
// loop with measured tokens errored "unpriced model" instead of costing 0).
func IsLocalProvider(p ProviderType) bool {
	return isLocalProvider(p)
}

// EstimateCostUSD prices measured token counts for a cloud model.
// Local providers always return (0, false) — no spend to account.
// Unknown models return (0, false): refusing to price beats inventing.
func EstimateCostUSD(provider ProviderType, model string, promptTokens, completionTokens int) (float64, bool) {
	return EstimateCostUSDWithCache(provider, model, promptTokens, completionTokens, 0, 0)
}

// EstimateCostUSDWithCache prices measured token counts applying cached-input
// discounts and cache-write multipliers from the rate card when present.
func EstimateCostUSDWithCache(provider ProviderType, model string, promptTokens, completionTokens, cachedInputTokens, cacheWriteTokens int) (float64, bool) {
	if isLocalProvider(provider) {
		return 0, false
	}
	if productionNeedsExternalRateCard() {
		return 0, false
	}
	if rates.IsStale(time.Now().UTC()) {
		return 0, false
	}
	rate, ok := rates.LookupModelRate(string(provider), model)
	if !ok {
		return 0, false
	}
	uncachedIn := promptTokens - cachedInputTokens - cacheWriteTokens
	if uncachedIn < 0 {
		uncachedIn = 0
	}
	cachedRate := rate.InputPer1KUSD
	if rate.CachedInputPer1KUSD > 0 {
		cachedRate = rate.CachedInputPer1KUSD
	}
	writeRate := rate.InputPer1KUSD
	if rate.CacheWritePer1KUSD > 0 {
		writeRate = rate.CacheWritePer1KUSD
	}
	return rate.InputPer1KUSD*float64(uncachedIn)/1000.0 +
		cachedRate*float64(cachedInputTokens)/1000.0 +
		writeRate*float64(cacheWriteTokens)/1000.0 +
		rate.OutputPer1KUSD*float64(completionTokens)/1000.0, true
}

// EstimateCostUSDForSettings uses the instance's endpoint-aware locality
// predicate (custom loopback endpoints cost $0; remote custom endpoints price).
func EstimateCostUSDForSettings(s *Settings, provider ProviderType, model string, promptTokens, completionTokens, cachedInputTokens, cacheWriteTokens int) (float64, bool) {
	if s != nil && s.IsProviderLocal(provider) {
		return 0, false
	}
	return EstimateCostUSDWithCache(provider, model, promptTokens, completionTokens, cachedInputTokens, cacheWriteTokens)
}

// productionNeedsExternalRateCard refuses to price cloud spend from the
// embedded bootstrap in production. The operator must install a runtime
// vendor-derived card (KUBONA_RATE_CARD_PATH/URL); local providers are free
// and never reach this check.
func productionNeedsExternalRateCard() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv("KUBONA_ENV")), "production") && !rates.IsExternal()
}

// LoopIterationBound prices one worst-case loop iteration (I3): the sum
// over roles of ceiling-cost at MaxTurnTokens (output rate — an UPPER
// bound by construction, never an estimate of the typical). Each role checks
// its full ordered chain [primary, ...fallback], taking the maximum callable
// ceiling across hops so a local primary backed by a metered cloud fallback
// still reserves for the worst case. Unassigned, disabled, or local roles
// contribute zero. An unknown METERED model in any callable hop refuses
// outright: admitting a loop whose fallback or primary cannot be priced would
// lock money against a guess or stall at billing — fail closed.
func LoopIterationBound(s *Settings, roles []string) (perIterUSD float64, fallbackRoles []string, err error) {
	cfg := DefaultBudgetConfig()
	snap := s.GetSnapshot()
	for _, role := range roles {
		plan := s.ResolveRolePlan(role)
		roleMaxUSD := 0.0
		for _, hop := range plan.Hops {
			prov, ok := snap.Providers[hop.Provider]
			if !ok || !prov.Enabled || strings.TrimSpace(hop.Model) == "" {
				continue
			}
			local := s.IsProviderLocal(hop.Provider)
			if snap.AirGapped && !local {
				continue
			}
			if productionCloudWithoutOptIn(local) {
				continue
			}
			if local {
				continue
			}
			b, ok := EstimateCeilingCostUSD(hop.Provider, hop.Model, cfg.MaxTurnTokens)
			if !ok {
				return 0, nil, fmt.Errorf("role %q model %q has no ceiling rate: pick a priced model in Model Settings before starting metered loops", role, hop.Model)
			}
			if b > roleMaxUSD {
				roleMaxUSD = b
			}
		}
		perIterUSD += roleMaxUSD
	}
	return perIterUSD, nil, nil
}

// (W0.19): the call's real split is unknown, so the upper bound bills.
// Unknown models still refuse (no ceiling to price against either).
// EstimateCeilingCostUSD prices a token ceiling at the model's output rate
func EstimateCeilingCostUSD(provider ProviderType, model string, ceilingTokens int) (float64, bool) {
	if isLocalProvider(provider) {
		return 0, false
	}
	if productionNeedsExternalRateCard() {
		return 0, false
	}
	if rates.IsStale(time.Now().UTC()) {
		return 0, false
	}
	rate, ok := rates.LookupModelRate(string(provider), model)
	if !ok {
		return 0, false
	}
	return rate.OutputPer1KUSD * float64(ceilingTokens) / 1000.0, true
}

// SpendRecord reports one metered cloud completion for cost accounting.
// Token counts are provider-reported; UsageReported is false when the
// endpoint sent no usage (never backfilled by estimation). CeilingApplied
// marks a ceiling estimate (tokens unknown, ceiling billed at the output
// rate as an upper bound); Unmetered marks blind spend with no ceiling
// (streams only — Complete refuses instead).
type SpendRecord struct {
	Provider          string
	Model             string
	PromptTokens      int
	CompletionTokens  int
	CachedInputTokens int
	CacheWriteTokens  int
	UsageReported     bool
	CeilingApplied    bool
	CeilingTokens     int
	Unmetered         bool
}
