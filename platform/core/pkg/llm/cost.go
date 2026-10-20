package llm

import "fmt"

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
	// NVIDIA NIM hosted models, order-of-magnitude.
	"nvidia/nemotron-3.5-super-vl-120b-a12b": {inputPer1K: 0.0007, outputPer1K: 0.0015},
	"meta/llama-3.1-70b-instruct":            {inputPer1K: 0.0007, outputPer1K: 0.0015},
	"meta/llama-3.3-70b-instruct":            {inputPer1K: 0.0007, outputPer1K: 0.0015},
}

// isLocalProvider reports whether the provider runs on loopback/on-device
// with no metered spend (free by construction, never priced). Single home
// for the local set — a new local engine updates this one predicate,
// not every call site (drift-proofing per the production bar).
func isLocalProvider(p ProviderType) bool {
	switch p {
	case ProviderOMLX, ProviderOllama, ProviderLMStudio, ProviderStrata:
		return true
	}
	return false
}

// EstimateCostUSD prices measured token counts for a cloud model.
// Local providers always return (0, false) — no spend to account.
// Unknown models return (0, false): refusing to price beats inventing.
func EstimateCostUSD(provider ProviderType, model string, promptTokens, completionTokens int) (float64, bool) {
	if isLocalProvider(provider) {
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
// bound by construction, never an estimate of the typical). Unassigned,
// disabled, or local roles contribute zero (their calls fail over to the
// free algorithmic fallback, spending nothing). An unknown METERED model
// refuses outright: admitting a loop that cannot be priced would either
// lock money against a guess or stall at first billing — both are
// silent failures, so admission fails closed with the model named.
// Never returns a bound for a model it cannot price.
func LoopIterationBound(s *Settings, roles []string) (perIterUSD float64, fallbackRoles []string, err error) {
	cfg := DefaultBudgetConfig()
	for _, role := range roles {
		prov, model, err := s.GetModelForRole(role)
		if err != nil {
			continue
		}
		if isLocalProvider(prov) {
			continue
		}
		b, ok := EstimateCeilingCostUSD(prov, model, cfg.MaxTurnTokens)
		if !ok {
			return 0, nil, fmt.Errorf("role %q model %q has no ceiling rate: pick a priced model in Model Settings before starting metered loops", role, model)
		}
		perIterUSD += b
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
