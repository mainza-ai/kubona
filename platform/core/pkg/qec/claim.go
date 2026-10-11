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
func ValidateLogicalClaim(c LogicalClaim) error {
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
		if logicalErrs[i] <= 0 {
			return 0, errors.New("lambda fit needs positive error rates")
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
	if slope >= 0 {
		return 0, errors.New("lambda fit refused: error does not decrease with distance (no suppression)")
	}
	return math.Exp(-2 * slope), nil
}

// EffectiveCostMultiplier converts a postselected gain into its true price:
// 1/acceptance more shots for the same kept statistics. Applied to any
// estimate that leans on detection + postselection.
func EffectiveCostMultiplier(acceptanceRate float64) (float64, error) {
	if acceptanceRate <= 0 || acceptanceRate > 1 {
		return 0, errors.New("acceptance rate must be in (0, 1]")
	}
	return 1.0 / acceptanceRate, nil
}
