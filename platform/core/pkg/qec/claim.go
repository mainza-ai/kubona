package qec

import (
	"errors"
	"math"
)

// LogicalClaim is the four-number shape every "logical beats physical"
// assertion must carry. Missing numbers refuse — a claim without them is
// uncheckable, and uncheckable claims do not ship.
type LogicalClaim struct {
	// Lambda: error suppression per +2 code distance (fit from >=2 points).
	Lambda float64 `json:"lambda"`
	// BreakevenFactor: logical lifetime / best physical lifetime (>1 to claim).
	BreakevenFactor float64 `json:"breakeven_factor"`
	// DecoderLatencyUs vs CycleTimeUs: latency must beat the cycle or the
	// backlog grows without bound (no real-time feed-forward).
	DecoderLatencyUs float64 `json:"decoder_latency_us"`
	CycleTimeUs      float64 `json:"cycle_time_us"`
	// AcceptanceRate: fraction of shots kept (postselected detection).
	// A gain without its acceptance rate is not a scaling result.
	AcceptanceRate float64 `json:"acceptance_rate"`
}

// ValidateLogicalClaim enforces the claim shape, fail-closed. It does not
// bless the numbers — it refuses the ones that cannot be checked.
// W1.11: every field refuses NaN/Inf explicitly — NaN comparisons are all
// false, so NaN used to sail through every check below.
func ValidateLogicalClaim(c LogicalClaim) error {
	for name, v := range map[string]float64{
		"lambda": c.Lambda, "breakeven_factor": c.BreakevenFactor,
		"decoder_latency_us": c.DecoderLatencyUs, "cycle_time_us": c.CycleTimeUs,
		"acceptance_rate": c.AcceptanceRate,
	} {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return errors.New("logical claim refused: " + name + " is not a finite number")
		}
	}
	if c.Lambda <= 0 {
		return errors.New("logical claim refused: lambda (suppression per +2 distance) missing or non-positive")
	}
	if c.BreakevenFactor <= 0 {
		return errors.New("logical claim refused: breakeven factor missing or non-positive")
	}
	if c.DecoderLatencyUs <= 0 || c.CycleTimeUs <= 0 {
		return errors.New("logical claim refused: decoder latency and cycle time both required (backlog check)")
	}
	if c.DecoderLatencyUs >= c.CycleTimeUs {
		return errors.New("logical claim refused: decoder latency exceeds cycle time — backlog grows without bound")
	}
	if c.AcceptanceRate <= 0 || c.AcceptanceRate > 1 {
		return errors.New("logical claim refused: acceptance rate must be in (0, 1]")
	}
	return nil
}

// FitLambda fits suppression-per-+2-distance from measured points:
// ln(eps) vs d has slope -ln(Λ)/2. Requires >= 2 points at strictly
// increasing distances with positive error rates; anything else refuses
// (a single point cannot define a scaling law).
func FitLambda(distances []int, logicalErrs []float64) (float64, error) {
	if len(distances) < 2 || len(distances) != len(logicalErrs) {
		return 0, errors.New("lambda fit needs >= 2 (distance, error) points")
	}
	n := float64(len(distances))
	var sumD, sumL, sumDL, sumD2 float64
	for i, d := range distances {
		if i > 0 && d <= distances[i-1] {
			return 0, errors.New("lambda fit needs strictly increasing distances")
		}
		if logicalErrs[i] <= 0 || math.IsNaN(logicalErrs[i]) || math.IsInf(logicalErrs[i], 0) {
			return 0, errors.New("lambda fit needs positive finite error rates")
		}
		l := math.Log(logicalErrs[i])
		sumD += float64(d)
		sumL += l
		sumDL += float64(d) * l
		sumD2 += float64(d) * float64(d)
	}
	denom := n*sumD2 - sumD*sumD
	if denom == 0 {
		return 0, errors.New("lambda fit degenerate: distances identical")
	}
	slope := (n*sumDL - sumD*sumL) / denom
	if math.IsNaN(slope) || math.IsInf(slope, 0) {
		return 0, errors.New("lambda fit refused: non-finite slope (degenerate input)")
	}
	if slope >= 0 {
		return 0, errors.New("lambda fit refused: error does not decrease with distance (no suppression)")
	}
	out := math.Exp(-2 * slope)
	if math.IsNaN(out) || math.IsInf(out, 0) {
		return 0, errors.New("lambda fit refused: non-finite scaling factor")
	}
	return out, nil
}

// EffectiveCostMultiplier converts a postselected gain into its true price:
// 1/acceptance more shots for the same kept statistics. Applied to any
// estimate that leans on detection + postselection.
func EffectiveCostMultiplier(acceptanceRate float64) (float64, error) {
	if math.IsNaN(acceptanceRate) || math.IsInf(acceptanceRate, 0) || acceptanceRate <= 0 || acceptanceRate > 1 {
		return 0, errors.New("acceptance rate must be in (0, 1]")
	}
	return 1.0 / acceptanceRate, nil
}

// FixedBudgetCodeClaim is the fail-closed claim shape for a fixed-qubit-budget
// qLDPC/BB code comparison (Phase 2 / G9), complementing LogicalClaim (which
// requires a >=2-point distance-scaling family for FitLambda).
type FixedBudgetCodeClaim struct {
	N                   int     `json:"n"`
	K                   int     `json:"k"`
	D                   int     `json:"d"`
	DKind               string  `json:"d_kind"` // must be "proved"
	DCirc               int     `json:"d_circ"`
	TotalQubits         int     `json:"total_qubits"`
	BaselineTotalQubits int     `json:"baseline_total_qubits"`
	CodeLERUpper95      float64 `json:"code_ler_upper_95"`
	BaselineLERLower95  float64 `json:"baseline_ler_lower_95"`
	HeldOutSurvived     bool    `json:"held_out_survived"`
	DecoderLatencyUs    float64 `json:"decoder_latency_us"`
	CycleTimeUs         float64 `json:"cycle_time_us"`
}

// ValidateFixedBudgetCodeClaim enforces Gates 1, 4, and 5 for a fixed-budget
// QEC code benchmark, fail-closed.
func ValidateFixedBudgetCodeClaim(c FixedBudgetCodeClaim) error {
	if c.N <= 0 || c.K <= 0 || c.D <= 0 {
		return errors.New("fixed-budget QEC claim refused: n, k, and d must be positive")
	}
	if c.DKind != "proved" {
		return errors.New("fixed-budget QEC claim refused: distance must be proved (upper_bound cannot enter a claim)")
	}
	if c.DCirc <= 0 {
		return errors.New("fixed-budget QEC claim refused: verified circuit-level distance d_circ is required")
	}
	if c.TotalQubits <= 0 || c.BaselineTotalQubits <= 0 {
		return errors.New("fixed-budget QEC claim refused: total_qubits and baseline_total_qubits must be positive")
	}
	if c.TotalQubits > c.BaselineTotalQubits {
		return errors.New("fixed-budget QEC claim refused: candidate exceeds declared baseline physical qubit budget")
	}
	for name, v := range map[string]float64{
		"code_ler_upper_95":    c.CodeLERUpper95,
		"baseline_ler_lower_95": c.BaselineLERLower95,
		"decoder_latency_us":   c.DecoderLatencyUs,
		"cycle_time_us":        c.CycleTimeUs,
	} {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return errors.New("fixed-budget QEC claim refused: " + name + " is not finite")
		}
	}
	if c.CodeLERUpper95 < 0 || c.BaselineLERLower95 <= 0 {
		return errors.New("fixed-budget QEC claim refused: invalid LER confidence bounds")
	}
	if c.CodeLERUpper95 >= c.BaselineLERLower95 {
		return errors.New("fixed-budget QEC claim refused: candidate and baseline 95% CIs overlap (no statistically separated advantage)")
	}
	if !c.HeldOutSurvived {
		return errors.New("fixed-budget QEC claim refused: advantage did not survive held-out noise evaluation")
	}
	if c.DecoderLatencyUs <= 0 || c.CycleTimeUs <= 0 || c.DecoderLatencyUs >= c.CycleTimeUs {
		return errors.New("fixed-budget QEC claim refused: decoder latency must be positive and strictly below cycle time")
	}
	return nil
}
