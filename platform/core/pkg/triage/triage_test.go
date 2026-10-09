package triage

import (
	"testing"
)

func TestQuantumTriageEngine(t *testing.T) {
	engine := NewQuantumTriageEngine()

	// 1. Single reference system (e.g. simple Water or Hydrogen) -> Classical DFT
	h2o := MolecularSystemSpec{
		MoleculeID:                   "mol-h2o",
		Formula:                      "H2O",
		NumElectrons:                 10,
		NuclearCharges:               []int{8, 1, 1},
		IsMultiReference:             false,
		CoulombContinuum:             true,
		EstimatedActiveSpaceOrbitals: 6,
	}

	d1, err := engine.Evaluate(h2o)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if d1.Target != TargetClassicalDFTHPC {
		t.Errorf("expected CLASSICAL_DFT_HPC, got %s", d1.Target)
	}
	if d1.RequiresQPU {
		t.Errorf("expected RequiresQPU=false for single-reference system")
	}

	// 2. Multi-reference system with small active space -> Classical DMRG Tensor
	c2 := MolecularSystemSpec{
		MoleculeID:                   "mol-c2",
		Formula:                      "C2",
		NumElectrons:                 12,
		NuclearCharges:               []int{6, 6},
		IsMultiReference:             true,
		CoulombContinuum:             true,
		EstimatedActiveSpaceOrbitals: 16,
	}

	d2, err := engine.Evaluate(c2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if d2.Target != TargetClassicalDMRGTensor {
		t.Errorf("expected CLASSICAL_DMRG_TENSOR, got %s", d2.Target)
	}
	if d2.RequiresQPU {
		t.Errorf("expected RequiresQPU=false for active space <= 50")
	}
	if d2.FamilyID != 278 {
		t.Errorf("expected Family 278, got %d", d2.FamilyID)
	}

	// 3. Strongly correlated cluster with large active space (e.g. FeMoco active space 54) -> Federated QPU
	femoco := MolecularSystemSpec{
		MoleculeID:                   "mol-femoco",
		Formula:                      "Fe7MoS9C",
		NumElectrons:                 54,
		NuclearCharges:               []int{26, 26, 26, 26, 26, 26, 26, 42, 16, 16, 16, 16, 16, 16, 16, 16, 16, 6},
		IsMultiReference:             true,
		CoulombContinuum:             true,
		EstimatedActiveSpaceOrbitals: 54, // > 50 ceiling
	}

	d3, err := engine.Evaluate(femoco)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if d3.Target != TargetFederatedQuantumQPU {
		t.Errorf("expected FEDERATED_QUANTUM_QPU, got %s", d3.Target)
	}
	if !d3.RequiresQPU {
		t.Errorf("expected RequiresQPU=true for active space > 50")
	}
	if d3.AlgorithmRecommendation != "Quantum_Krylov_Subspace_Diagonalization" {
		t.Errorf("expected Quantum_Krylov_Subspace_Diagonalization, got %s", d3.AlgorithmRecommendation)
	}
	if len(d3.ReceiptHash) != 64 {
		t.Errorf("expected valid SHA256 receipt hash, got %s", d3.ReceiptHash)
	}

	// 4. Test Rule 1 and Rule 2 triggers via small HOMO-LUMO gap (< 0.1 Ha) or high radical index (>= 0.05)
	biradical := MolecularSystemSpec{
		MoleculeID:                   "mol-biradical-polyene",
		Formula:                      "C8H8",
		NumElectrons:                 16,
		NuclearCharges:               []int{6, 6, 6, 6, 6, 6, 6, 6},
		IsMultiReference:             false, // Not explicitly flagged, but gap and radical index detect it
		CoulombContinuum:             true,
		EstimatedActiveSpaceOrbitals: 12,
		HOMOLUMOGap:                  0.04, // < 0.1 Ha triggers multi-reference
		RadicalIndex:                 0.08, // >= 0.05 triggers multi-reference
	}

	d4, err := engine.Evaluate(biradical)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if d4.Target != TargetClassicalDMRGTensor {
		t.Errorf("expected CLASSICAL_DMRG_TENSOR for biradical with small gap, got %s", d4.Target)
	}
	if len(d4.ReceiptHash) != 64 {
		t.Errorf("expected 64-char receipt hash, got %s", d4.ReceiptHash)
	}
}
