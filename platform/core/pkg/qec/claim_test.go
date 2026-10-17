package qec

import (
	"math"
	"testing"
)

func TestFitLambdaRecoversKnownSuppression(t *testing.T) {
	// Synthetic below-threshold ladder: eps(d) = 0.02 * 2.14^(-(d-3)/2).
	distances := []int{3, 5, 7, 9}
	errs := make([]float64, len(distances))
	for i, d := range distances {
		errs[i] = 0.02 * math.Pow(2.14, -float64(d-3)/2.0)
	}
	got, err := FitLambda(distances, errs)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(got-2.14) > 0.01 {
		t.Fatalf("fit must recover Λ=2.14, got %v", got)
	}
}

func TestFitLambdaRefusesUnguessable(t *testing.T) {
	if _, err := FitLambda([]int{5}, []float64{0.01}); err == nil {
		t.Fatal("single point cannot define a scaling law")
	}
	if _, err := FitLambda([]int{5, 5}, []float64{0.01, 0.005}); err == nil {
		t.Fatal("non-increasing distances must refuse")
	}
	if _, err := FitLambda([]int{3, 5}, []float64{0.01, 0.02}); err == nil {
		t.Fatal("rising error with distance is not suppression")
	}
	if _, err := FitLambda([]int{3, 5}, []float64{0, 0.005}); err == nil {
		t.Fatal("non-positive rates must refuse")
	}
}

func TestValidateLogicalClaimShape(t *testing.T) {
	good := LogicalClaim{Lambda: 2.14, BreakevenFactor: 2.4,
		DecoderLatencyUs: 0.8, CycleTimeUs: 1.1, AcceptanceRate: 0.9}
	if err := ValidateLogicalClaim(good); err != nil {
		t.Fatalf("complete claim must pass shape: %v", err)
	}
	cases := []LogicalClaim{
		{}, // everything missing
		{Lambda: 2.14, BreakevenFactor: 2.4, DecoderLatencyUs: 1.5, CycleTimeUs: 1.1, AcceptanceRate: 1},
		{Lambda: 2.14, BreakevenFactor: 2.4, DecoderLatencyUs: 0.8, CycleTimeUs: 1.1}, // no acceptance
		{Lambda: 2.14, BreakevenFactor: 2.4, DecoderLatencyUs: 0.8, CycleTimeUs: 1.1, AcceptanceRate: 1.5},
	}
	for i, c := range cases {
		if err := ValidateLogicalClaim(c); err == nil {
			t.Fatalf("case %d must refuse, shape incomplete or unphysical", i)
		}
	}
}

func TestAcceptanceMultiplierPricesPostselection(t *testing.T) {
	m, err := EffectiveCostMultiplier(0.5)
	if err != nil || m != 2.0 {
		t.Fatalf("50%% acceptance doubles the shot bill, got %v %v", m, err)
	}
	if _, err := EffectiveCostMultiplier(0); err == nil {
		t.Fatal("zero acceptance must refuse")
	}
	if _, err := EffectiveCostMultiplier(1.2); err == nil {
		t.Fatal("acceptance above 1 must refuse")
	}
}

func TestEstimatesDefaultToNoPostselection(t *testing.T) {
	e := NewResourceEstimator()
	spec := QECResourceSpec{NumLogicalQubits: 4, PhysicalErrorRate: 0.001}
	for _, est := range []struct {
		name string
		fn   func() (*QECResourceEstimate, error)
	}{
		{"neutral-atom", func() (*QECResourceEstimate, error) { return e.EstimateNeutralAtom(spec) }},
		{"bivariate-bicycle", func() (*QECResourceEstimate, error) { return e.EstimateBivariateBicycle(spec) }},
	} {
		out, err := est.fn()
		if err != nil {
			t.Fatal(err)
		}
		if out.EffectiveShotsMultiplier != 1.0 {
			t.Fatalf("%s: unspecified acceptance must cost 1x, got %v", est.name, out.EffectiveShotsMultiplier)
		}
	}
	out, err := e.EstimateBivariateBicycle(QECResourceSpec{
		NumLogicalQubits: 12, PhysicalErrorRate: 0.001, AcceptanceRate: 0.5})
	if err != nil {
		t.Fatal(err)
	}
	if out.EffectiveShotsMultiplier != 2.0 {
		t.Fatalf("50%% acceptance must double the bill, got %v", out.EffectiveShotsMultiplier)
	}
}

// W1.11: NaN never validates; targets compare; bad inputs refuse.
func TestQECNonFiniteRefusals(t *testing.T) {
	nan := math.NaN()
	for _, c := range []LogicalClaim{
		{Lambda: nan, BreakevenFactor: 2.4, DecoderLatencyUs: 0.8, CycleTimeUs: 1.1, AcceptanceRate: 0.9},
		{Lambda: 2.14, BreakevenFactor: nan, DecoderLatencyUs: 0.8, CycleTimeUs: 1.1, AcceptanceRate: 0.9},
		{Lambda: 2.14, BreakevenFactor: 2.4, DecoderLatencyUs: 0.8, CycleTimeUs: 1.1, AcceptanceRate: nan},
	} {
		if err := ValidateLogicalClaim(c); err == nil {
			t.Fatalf("NaN claim must refuse: %+v", c)
		}
	}
	if _, err := FitLambda([]int{3, 5}, []float64{1e-3, nan}); err == nil {
		t.Fatalf("NaN fit input must refuse")
	}
	if _, err := EffectiveCostMultiplier(nan); err == nil {
		t.Fatalf("NaN acceptance must refuse")
	}
	e := NewResourceEstimator()
	if _, err := e.EstimateNeutralAtom(QECResourceSpec{NumLogicalQubits: 4, PhysicalErrorRate: 0}); err == nil {
		t.Fatalf("missing error rate must refuse (no invented physics)")
	}
	if _, err := e.EstimateNeutralAtom(QECResourceSpec{NumLogicalQubits: 4, PhysicalErrorRate: nan}); err == nil {
		t.Fatalf("NaN error rate must refuse")
	}
	if _, err := e.EstimateNeutralAtom(QECResourceSpec{
		NumLogicalQubits: 4, PhysicalErrorRate: 0.001, AcceptanceRate: -0.5,
	}); err == nil {
		t.Fatalf("negative acceptance must refuse (no silent 1.0)")
	}
	if _, err := e.EstimateNeutralAtom(QECResourceSpec{
		NumLogicalQubits: 4, LogicalTCount: -3, PhysicalErrorRate: 0.001,
	}); err == nil {
		t.Fatalf("negative gate counts must refuse")
	}
	// Target comparison is real: met vs unmet.
	met, err := e.EstimateNeutralAtom(QECResourceSpec{
		NumLogicalQubits: 4, PhysicalErrorRate: 0.001, TargetLogicalInf: 1e-3,
	})
	if err != nil || !met.MeetsTarget {
		t.Fatalf("suppressed error must meet a loose target: %+v %v", met, err)
	}
	unmet, err := e.EstimateNeutralAtom(QECResourceSpec{
		NumLogicalQubits: 4, PhysicalErrorRate: 0.001, TargetLogicalInf: 1e-30,
	})
	if err != nil || unmet.MeetsTarget {
		t.Fatalf("impossible target must not meet: %+v %v", unmet, err)
	}
	// No target set: MeetsTarget false (never vacuously true).
	plain, err := e.EstimateNeutralAtom(QECResourceSpec{NumLogicalQubits: 4, PhysicalErrorRate: 0.001})
	if err != nil || plain.MeetsTarget {
		t.Fatalf("unset target must not report met: %+v %v", plain, err)
	}
}
