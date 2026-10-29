// Package rates loads the pinned rate-card contract. The card is embedded at
// build time; the canonical file lives at platform/contracts/rate-cards.json
// and a parity test keeps the copy honest.
//
// The contract is explicitly heuristic until a vendor price sheet replaces
// it. A stale card is a hard error at daemon startup: a spend-control plane
// that prices from an expired guess is a fiction with a budget.
package rates

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

//go:embed rate-cards.json
var embedded []byte

// ErrStale means the card's effective window has elapsed.
var ErrStale = errors.New("rate card is stale")

type ModelRate struct {
	InputPer1KUSD  float64 `json:"input_per_1k_usd"`
	OutputPer1KUSD float64 `json:"output_per_1k_usd"`
}

type LensRate struct {
	CostUSD float64 `json:"cost_usd"`
	Unit    string  `json:"unit"`
}

type Card struct {
	Version        int                  `json:"version"`
	Source         string               `json:"source"`
	EffectiveDate  string               `json:"effective_date"`
	StaleAfterDays int                  `json:"stale_after_days"`
	Heuristic      bool                 `json:"heuristic"`
	Notes          string               `json:"notes"`
	Lens           map[string]LensRate  `json:"lens"`
	Models         map[string]ModelRate `json:"models"`
}

var card = mustLoad()

func mustLoad() *Card {
	var c Card
	if err := json.Unmarshal(embedded, &c); err != nil {
		panic(fmt.Sprintf("rates: embedded card is invalid JSON: %v", err))
	}
	return &c
}

// Validate checks the card's shape and effective window. It is called by the
// daemon before serving and by tests with a fixed clock.
func Validate(now time.Time) error {
	if card.Version <= 0 {
		return fmt.Errorf("rate card version must be positive")
	}
	if card.Source == "" {
		return fmt.Errorf("rate card source is required")
	}
	if card.EffectiveDate == "" || card.StaleAfterDays <= 0 {
		return fmt.Errorf("rate card effective_date and positive stale_after_days are required")
	}
	effective, err := time.Parse("2006-01-02", card.EffectiveDate)
	if err != nil {
		return fmt.Errorf("rate card effective_date: %w", err)
	}
	if now.UTC().After(effective.AddDate(0, 0, card.StaleAfterDays)) {
		return fmt.Errorf("%w: effective %s, stale after %d days; update platform/contracts/rate-cards.json", ErrStale, card.EffectiveDate, card.StaleAfterDays)
	}
	if _, ok := card.Lens["atlas-lens"]; !ok {
		return fmt.Errorf("rate card is missing lens/atlas-lens")
	}
	if len(card.Models) == 0 {
		return fmt.Errorf("rate card has no model rates")
	}
	for model, r := range card.Models {
		if r.InputPer1KUSD < 0 || r.OutputPer1KUSD < 0 {
			return fmt.Errorf("rate card model %q has a negative rate", model)
		}
	}
	return nil
}

// IsStale reports whether the card's effective window has elapsed. Pricing
// call sites use it so a long-running daemon cannot keep billing from an
// expired heuristic after startup.
func IsStale(now time.Time) bool {
	effective, err := time.Parse("2006-01-02", card.EffectiveDate)
	if err != nil {
		return true
	}
	return now.UTC().After(effective.AddDate(0, 0, card.StaleAfterDays))
}

// LensCostUSD returns the per-dispatch lens cost, or false when the card is
// missing the entry.
func LensCostUSD(name string) (float64, bool) {
	r, ok := card.Lens[name]
	if !ok {
		return 0, false
	}
	return r.CostUSD, true
}

func MustLensCostUSD(name string) float64 {
	cost, ok := LensCostUSD(name)
	if !ok {
		panic("rates: missing lens rate " + name)
	}
	return cost
}

// LookupModelRate returns the input/output rates for a model.
func LookupModelRate(model string) (ModelRate, bool) {
	r, ok := card.Models[model]
	return r, ok
}

// Card returns a copy of the loaded card for diagnostics/tests.
func CardSnapshot() Card { return *card }
