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
