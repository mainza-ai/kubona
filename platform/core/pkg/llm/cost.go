package llm

// Cloud LLM spend accounting (K4). Tribunal and copilot turns can call
// metered cloud models (opencode/deepseek, anthropic, openai); without a
// dollar rail that spend is invisible and uncapped — the same defect
// cost-governance fixed for QPU time. This file prices measured token
// usage; the server records per-call events and enforces an optional
// global cap (KUBONA_LLM_BUDGET_USD).
//
// RATES ARE V1 HEURISTICS (2026-10-12), NOT vendor price sheets. Every
// estimate carries heuristic:true; unknown models refuse to price
// (ok=false) instead of inventing a rate. The cap therefore enforces on
// approximate spend — a guardrail, not a meter. Calibrate before
// treating any number as a bill.
type modelRate struct {
	inputPer1K  float64
	outputPer1K float64
}

var heuristicRates = map[string]modelRate{
	// Flash-class distilled models, order-of-magnitude.
	"deepseek-v4.1-flash": {inputPer1K: 0.0001, outputPer1K: 0.0004},
	"deepseek-chat":       {inputPer1K: 0.0002, outputPer1K: 0.0008},
	// Frontier classes, order-of-magnitude.
	"claude-3-5-sonnet": {inputPer1K: 0.003, outputPer1K: 0.015},
	"claude-3-7-sonnet": {inputPer1K: 0.003, outputPer1K: 0.015},
	"gpt-4o":            {inputPer1K: 0.0025, outputPer1K: 0.01},
	"o3":                {inputPer1K: 0.01, outputPer1K: 0.04},
	"gemini-2.5-pro":    {inputPer1K: 0.00125, outputPer1K: 0.01},
}

// EstimateCostUSD prices measured token counts for a cloud model.
// Local providers always return (0, false) — no spend to account.
// Unknown models return (0, false): refusing to price beats inventing.
func EstimateCostUSD(provider ProviderType, model string, promptTokens, completionTokens int) (float64, bool) {
	if provider == ProviderOMLX || provider == ProviderOllama || provider == ProviderLMStudio {
		return 0, false
	}
	rate, ok := heuristicRates[model]
	if !ok {
		return 0, false
	}
	return rate.inputPer1K*float64(promptTokens)/1000.0 +
		rate.outputPer1K*float64(completionTokens)/1000.0, true
}

// LoopIterationBound prices one worst-case loop iteration (I3): the sum
// over roles of ceiling-cost at MaxTurnTokens (output rate — an UPPER
// bound by construction, never an estimate of the typical). Unassigned
// or disabled roles contribute zero (their calls fail over to the free
// algorithmic fallback, spending nothing). Unknown metered models fall
// back to the priciest KNOWN rate (safe direction; remainder releases
// at loop end, and the fallback is named in fallbackRoles for the
// audit event). Local providers contribute zero. Never errors —
// admission refuses on the tenant ceiling, not on pricing.
func LoopIterationBound(s *Settings, roles []string) (perIterUSD float64, fallbackRoles []string) {
	cfg := DefaultBudgetConfig()
	maxKnown := 0.0
	for _, rate := range heuristicRates {
		if rate.outputPer1K > maxKnown {
			maxKnown = rate.outputPer1K
		}
	}
	for _, role := range roles {
		prov, model, err := s.GetModelForRole(role)
		if err != nil {
			continue
		}
		if prov == ProviderOMLX || prov == ProviderOllama || prov == ProviderLMStudio {
			continue
		}
		if b, ok := EstimateCeilingCostUSD(prov, model, cfg.MaxTurnTokens); ok {
			perIterUSD += b
			continue
		}
		perIterUSD += maxKnown * float64(cfg.MaxTurnTokens) / 1000.0
		fallbackRoles = append(fallbackRoles, role+":"+model)
	}
	return perIterUSD, fallbackRoles
}

// (W0.19): the call's real split is unknown, so the upper bound bills.
// Unknown models still refuse (no ceiling to price against either).
// EstimateCeilingCostUSD prices a token ceiling at the model's output rate
func EstimateCeilingCostUSD(provider ProviderType, model string, ceilingTokens int) (float64, bool) {
	if provider == ProviderOMLX || provider == ProviderOllama || provider == ProviderLMStudio {
		return 0, false
	}
	rate, ok := heuristicRates[model]
	if !ok {
		return 0, false
	}
	return rate.outputPer1K * float64(ceilingTokens) / 1000.0, true
}

// SpendRecord reports one metered cloud completion for cost accounting.
// Token counts are provider-reported; UsageReported is false when the
// endpoint sent no usage (never backfilled by estimation). CeilingApplied
// marks a ceiling estimate (tokens unknown, ceiling billed at the output
// rate as an upper bound); Unmetered marks blind spend with no ceiling
// (streams only — Complete refuses instead).
type SpendRecord struct {
	Provider         string
	Model            string
	PromptTokens     int
	CompletionTokens int
	UsageReported    bool
	CeilingApplied   bool
	CeilingTokens    int
	Unmetered        bool
}
