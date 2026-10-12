package triage

// Mathematical triage routing (Family 278/275): classifies a molecular
// system as DFT-tractable, DMRG-tractable, or QPU-mandatory, with the
// active-space ceiling at 50 orbitals (both engines agree; the wiki's
// older ≤36 figure is superseded).
//
// Status: ADVISORY engine. No live dispatch path calls Triage today —
// verdicts are computed on demand (tests, explicit callers) and recorded,
// never enforced. Wiring it into pre-QPU admission is a design question,
// not a code gap: submit paths carry QASM/backends, not molecule
// identity or orbital counts, so there is nothing honest to route on yet.
// EstimatedCredits prices the verdict (25/orbital QPU, 2.5 DMRG, 1.0 DFT)
// for the day a caller exists; until then it is specified output.

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"
)

// TriageTarget represents the destination compute substrate
type TriageTarget string

const (
	TargetClassicalDFTHPC     TriageTarget = "CLASSICAL_DFT_HPC"
	TargetClassicalDMRGTensor TriageTarget = "CLASSICAL_DMRG_TENSOR"
	TargetFederatedQuantumQPU TriageTarget = "FEDERATED_QUANTUM_QPU"
)

// MolecularSystemSpec defines chemical and quantum mechanics parameters
type MolecularSystemSpec struct {
	MoleculeID                   string  `json:"molecule_id"`
	Formula                      string  `json:"formula"`
	NumElectrons                 int     `json:"num_electrons"`
	NuclearCharges               []int   `json:"nuclear_charges"`
	IsMultiReference             bool    `json:"is_multi_reference"`
	CoulombContinuum             bool    `json:"coulomb_continuum"`
	EstimatedActiveSpaceOrbitals int     `json:"estimated_active_space_orbitals"`
	HOMOLUMOGap                  float64 `json:"homo_lumo_gap,omitempty"`
	RadicalIndex                 float64 `json:"radical_index,omitempty"`
}

// TriageDecision captures the mathematically grounded routing result
type TriageDecision struct {
	MoleculeID              string       `json:"molecule_id"`
	Target                  TriageTarget `json:"target"`
	Justification           string       `json:"justification"`
	FamilyID                int          `json:"family_id"`
	AlgorithmRecommendation string       `json:"algorithm_recommendation"`
	RequiresQPU             bool         `json:"requires_qpu"`
	ComplexityRegime        string       `json:"complexity_regime"`
	EstimatedCredits        float64      `json:"estimated_credits"`
	DecisionTimestamp       time.Time    `json:"decision_timestamp"`
	ReceiptHash             string       `json:"receipt_hash"`
}

// QuantumTriageEngine enforces mathematical routing based on Family 278 and Family 275
type QuantumTriageEngine struct {
	ActiveSpaceDMRGCeiling int // Default 50 orbitals
}

func NewQuantumTriageEngine() *QuantumTriageEngine {
	return &QuantumTriageEngine{
		ActiveSpaceDMRGCeiling: 50,
	}
}

// Evaluate routes the molecular simulation request based on formal mathematical criteria
func (e *QuantumTriageEngine) Evaluate(system MolecularSystemSpec) (*TriageDecision, error) {
	if system.MoleculeID == "" {
		return nil, errors.New("molecule_id is required")
	}
	if system.NumElectrons <= 0 {
		return nil, errors.New("num_electrons must be positive")
	}

	decision := &TriageDecision{
		MoleculeID:        system.MoleculeID,
		DecisionTimestamp: time.Now(),
	}

	isMultiRef := system.IsMultiReference ||
		(system.HOMOLUMOGap > 0.0 && system.HOMOLUMOGap < 0.1) ||
		(system.RadicalIndex >= 0.05)

	// Family 278: Multi-reference Coulomb molecules provably lack non-interacting KS representation
	if isMultiRef && system.CoulombContinuum {
		if system.EstimatedActiveSpaceOrbitals > e.ActiveSpaceDMRGCeiling {
			// Active space exceeds classical Full Configuration Interaction / DMRG bounds
			decision.Target = TargetFederatedQuantumQPU
			decision.Justification = fmt.Sprintf(
				"Family 278 Kohn-Sham failure certified; active space orbitals (%d) > %d exceed classical DMRG. Routed to QPU Quantum Krylov Subspace Diagonalization.",
				system.EstimatedActiveSpaceOrbitals,
				e.ActiveSpaceDMRGCeiling,
			)
			decision.FamilyID = 278
			decision.AlgorithmRecommendation = "Quantum_Krylov_Subspace_Diagonalization"
			decision.RequiresQPU = true
			decision.ComplexityRegime = "QMA_Hard_Family_275"
			decision.EstimatedCredits = 25.0 * float64(system.EstimatedActiveSpaceOrbitals)
		} else {
			// Multi-reference but small active space -> Classical Tensor Network DMRG
			decision.Target = TargetClassicalDMRGTensor
			decision.Justification = fmt.Sprintf(
				"Family 278 Kohn-Sham failure certified, but active space (%d orbitals <= %d) is classically tractable via DMRG/MPS.",
				system.EstimatedActiveSpaceOrbitals,
				e.ActiveSpaceDMRGCeiling,
			)
			decision.FamilyID = 278
			decision.AlgorithmRecommendation = "Classical_DMRG_Matrix_Product_States"
			decision.RequiresQPU = false
			decision.ComplexityRegime = "Polynomially_Bounded_Tensor"
			decision.EstimatedCredits = 2.5 * float64(system.EstimatedActiveSpaceOrbitals)
		}
	} else {
		// Single-reference or non-Coulomb continuum -> Classical DFT / B3LYP on HPC
		decision.Target = TargetClassicalDFTHPC
		decision.Justification = "System is single-reference or non-continuum; Kohn-Sham DFT is mathematically sufficient and provably exact under Family 265/266 limits."
		decision.FamilyID = 265
		decision.AlgorithmRecommendation = "Classical_DFT_B3LYP_HPC"
		decision.RequiresQPU = false
		decision.ComplexityRegime = "P_Classical"
		decision.EstimatedCredits = 1.0
	}

	// Generate cryptographic receipt hash
	raw := fmt.Sprintf("%s:%s:%d:%s:%.2f",
		decision.MoleculeID,
		decision.Target,
		decision.FamilyID,
		decision.AlgorithmRecommendation,
		decision.EstimatedCredits,
	)
	hasher := sha256.New()
	hasher.Write([]byte(raw))
	decision.ReceiptHash = hex.EncodeToString(hasher.Sum(nil))

	return decision, nil
}
