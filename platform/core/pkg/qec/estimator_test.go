package qec

import (
	"testing"
)

func TestResourceEstimatorNeutralAtom(t *testing.T) {
	est := NewResourceEstimator()

	spec := QECResourceSpec{
		NumLogicalQubits:  12,
		LogicalTCount:     240,
		LogicalClifford:   800,
		PhysicalErrorRate: 0.001,
	}

	res, err := est.EstimateNeutralAtom(spec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.Architecture != ArchNeutralAtomQuEra {
		t.Errorf("expected NEUTRAL_ATOM_QUERA, got %s", res.Architecture)
	}
	if res.PhysicalQubitsTotal > 280 {
		t.Errorf("expected physical qubits within 280-atom QuEra budget, got %d", res.PhysicalQubitsTotal)
	}
	if !res.MeetsThreshold {
		t.Errorf("expected physical error 0.001 to be below threshold 0.01")
	}

	// Exceed capacity (> 48 logical qubits)
	_, err = est.EstimateNeutralAtom(QECResourceSpec{NumLogicalQubits: 50})
	if err == nil {
		t.Errorf("expected error when requesting > 48 logical qubits on neutral-atom")
	}
}

func TestResourceEstimatorBivariateBicycle(t *testing.T) {
	est := NewResourceEstimator()

	// 12 logical qubits -> exactly 1 block of 288 physical qubits
	spec := QECResourceSpec{
		NumLogicalQubits:  12,
		LogicalTCount:     1000,
		LogicalClifford:   4000,
		PhysicalErrorRate: 0.001,
	}

	res, err := est.EstimateBivariateBicycle(spec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.Architecture != ArchBivariateBicycleIBM {
		t.Errorf("expected BIVARIATE_BICYCLE_QLDPC_IBM, got %s", res.Architecture)
	}
	if res.PhysicalQubitsTotal != 288 {
		t.Errorf("expected 288 physical qubits for 12 logical qubits (1 block), got %d", res.PhysicalQubitsTotal)
	}
	if res.CodeDistance != 12 {
		t.Errorf("expected distance 12, got %d", res.CodeDistance)
	}
	if res.PhysicalQubitOverhead != 24.0 {
		t.Errorf("expected 24x overhead (288/12), got %.2f", res.PhysicalQubitOverhead)
	}
	// Verify that logical error is exponentially suppressed
	if res.SuppressedLogicalErr > 1e-5 {
		t.Errorf("expected suppressed logical error < 1e-5, got %e", res.SuppressedLogicalErr)
	}
}
