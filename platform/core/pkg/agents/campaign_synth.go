package agents

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	pb "github.com/mainza-ai/kubona/core/proto"
)

// QuantumFactors encapsulates the dynamic quantum information for a target
type QuantumFactors struct {
	TargetMolecule     string  `json:"target_molecule"`
	MolecularFormula   string  `json:"molecular_formula"`
	BondDistance       float64 `json:"bond_distance"`
	ActiveElectrons    int     `json:"active_electrons"`
	ActiveOrbitals     int     `json:"active_orbitals"`
	FermionMapping     string  `json:"fermion_mapping"`
	QubitCount         int     `json:"qubit_count"`
	CircuitDepth       int     `json:"circuit_depth"`
	TwoQubitGates      int     `json:"two_qubit_gates"`
	AnsatzFamily       string  `json:"ansatz_family"`
	MeyerWallachQ      float64 `json:"meyer_wallach_q,omitempty"`
	GradientVariance   float64 `json:"gradient_variance,omitempty"`
	BarrenPlateauRisk  string  `json:"barren_plateau_risk,omitempty"`
	CliffordFidelity   float64 `json:"clifford_fidelity,omitempty"`
	RecommendedBackend string  `json:"recommended_backend"`
	EstimatedSpendUSD  float64 `json:"estimated_spend_usd"`
	QASM               string  `json:"qasm"`
	// Synthesized marks the proposed circuit as generated, not measured
	// (W1.6): the h+rz+cx template below is a starting proposal for the
	// optimizer — launch binds it to a real run, never presents it as one.
	Synthesized bool  `json:"synthesized"`
	DagNodes    []any `json:"dag_nodes"`
	DagEdges    []any `json:"dag_edges"`
	WireCuts    []any `json:"wire_cuts"`
}

// ClassicalAnchor encapsulates the ab-initio physics baseline.
// AnchorComputed reports whether RHF/FCI came from a live PySCF bridge
// computation (true, AnchorSource "pyscf-live"). When the bridge is
// unavailable, the scaffold carries no borrowed energies: AnchorComputed is
// false, AnchorSource is "uncomputed", and the energy fields are omitted.
type ClassicalAnchor struct {
	Method                 string  `json:"method"`
	RhfMeanFieldEnergy     float64 `json:"rhf_mean_field_energy,omitempty"`
	FciExactEnergy         float64 `json:"fci_exact_energy,omitempty"`
	VqeExpectationEnergy   float64 `json:"vqe_expectation_energy,omitempty"`
	DiscrepancyMHa         float64 `json:"discrepancy_mha,omitempty"`
	ChemicalAccuracyPassed bool    `json:"chemical_accuracy_passed,omitempty"`
	ChemicalAccuracyLimit  float64 `json:"chemical_accuracy_limit"`
	AnchorComputed         bool    `json:"anchor_computed"`
	AnchorSource           string  `json:"anchor_source"`
}

// fallbackCaveat is retained only to repair pre-Phase-1 stored states that
// still carry the old hardcoded fallback. New proposals never write it.
const fallbackCaveat = " Classical anchor is a hardcoded fallback (bridge unavailable) — unverified."

// Legacy projection weights: used only when repairing pre-Phase-1 stored
// fallback states. New campaigns create no epistemics until a claim folds.
const (
	liveAlpha = 0.95
	liveBeta  = 0.05
	liveGamma = 0.10
)

// livePCorrelated derives from liveAlpha; retained for legacy repair only.
const livePCorrelated = liveAlpha * liveAlpha * 100.0

// synthSources attests PySCF evidence labels only for computed anchors.
// A fallback anchor cites nothing: an empty (unattested) list is legal,
// an invented PySCF label is fabrication (R1).
func synthSources(anchorComputed bool) []string {
	if !anchorComputed {
		return []string{}
	}
	return []string{"pyscf:sto-3g:rhf", "pyscf:sto-3g:fci"}
}

// TrajectoryStep represents a single optimization point. Source names the
// provenance: "measured" points are folded from real job results. The
// historical "synthesized" source can still appear in pre-Phase-1 rows;
// consumers must never present it as a measurement.
type TrajectoryStep struct {
	Step      int       `json:"step"`
	Energy    float64   `json:"energy"`
	Loss      float64   `json:"loss"`
	Theta     []float64 `json:"theta"`
	GradNorm  float64   `json:"grad_norm"`
	Watchdog  string    `json:"watchdog"`
	Timestamp string    `json:"timestamp"`
	Source    string    `json:"source,omitempty"`
}

// EpistemicState represents the tripartite credibility manifold
type EpistemicState struct {
	AlphaCorroborated float64  `json:"alpha_corroborated"`
	BetaContested     float64  `json:"beta_contested"`
	GammaUnverified   float64  `json:"gamma_unverified"`
	PCorrelated       float64  `json:"p_correlated"`
	Status            string   `json:"status"`
	LiteratureDrift   float64  `json:"literature_drift_mha"`
	Statement         string   `json:"statement"`
	Sources           []string `json:"sources"`
}

// SystematicNotebook represents the citable scientific report
type SystematicNotebook struct {
	ID               string   `json:"id"`
	Title            string   `json:"title"`
	CreatedAt        string   `json:"created_at"`
	ClassicalAnchor  string   `json:"classical_anchor"`
	QuantumEnergy    float64  `json:"quantum_energy,omitempty"`
	ClassicalEnergy  float64  `json:"classical_energy,omitempty"`
	DeltaMHa         float64  `json:"delta_mha,omitempty"`
	ChemicalAccuracy bool     `json:"chemical_accuracy,omitempty"`
	ProvenanceHash   string   `json:"provenance_hash"`
	EpistemicVerdict string   `json:"epistemic_verdict"`
	Sections         []string `json:"sections"`
}

// CampaignState is the top-level dynamic state for a research session
type CampaignState struct {
	SessionID       string              `json:"session_id"`
	ResearchGoal    string              `json:"research_goal"`
	TargetSystem    string              `json:"target_system"`
	SMILES          string              `json:"smiles,omitempty"`
	QuantumFactors  QuantumFactors      `json:"quantum_factors"`
	ClassicalAnchor ClassicalAnchor     `json:"classical_anchor"`
	Trajectory      []TrajectoryStep    `json:"trajectory"`
	Branches        []any               `json:"branches"`
	Epistemics      *EpistemicState     `json:"epistemics"`
	ADMET           any                 `json:"admet,omitempty"`
	Notebook        *SystematicNotebook `json:"notebook"`
	UpdatedAt       time.Time           `json:"updated_at"`
}

// BuildCampaignProposal constructs the truth-preserving campaign scaffold:
// target, proposed circuit, and a computed classical anchor when the bridge
// answers. It never manufactures a VQE energy, trajectory, notebook, or
// epistemics; those fields appear only when a measured job/claim folds.
func BuildCampaignProposal(
	ctx context.Context,
	sessionID, prompt, targetMolecule string,
	bridgeClient pb.QuantumBridgeServiceClient,
) (*CampaignState, error) {
	upper := strings.ToUpper(prompt)
	if targetMolecule != "" {
		upper = strings.ToUpper(targetMolecule) + " " + upper
	}

	// Canonical molecule row (W1.6): single physics source (molecule_table.go).
	// The row supplies target geometry and proposed-circuit shape. Its RHF/FCI
	// reference values are deliberately NOT copied into campaign state: they
	// are not a computation at this bond/target.
	spec := LookupMolecule(upper)
	mol := spec.Mol
	formula := spec.Formula
	bondDist := spec.BondDist
	target := spec.Target
	qubits := spec.Qubits
	depth := spec.Depth
	twoQGates := spec.TwoQGates
	ansatz := spec.Ansatz
	smiles := spec.SMILES

	bondRegex := regexp.MustCompile(`(?i)(?:R\s*=\s*|BOND\s*(?:LENGTH|DISTANCE)\s*(?:OF\s*)?)([0-9]+\.?[0-9]*)`)
	if m := bondRegex.FindStringSubmatch(prompt); len(m) > 1 {
		if parsedDist, err := strconv.ParseFloat(m[1], 64); err == nil && parsedDist >= 0.3 && parsedDist <= 10.0 {
			bondDist = parsedDist
			slog.Info("campaign proposal custom bond distance", "bond_distance_angstrom", bondDist)
		}
	}

	// The only authoritative anchor is one computed by the live bridge. A
	// missing bridge leaves the anchor explicitly uncomputed; it never borrows
	// a table value that was measured for a different setup.
	anchor := ClassicalAnchor{
		Method:                "Classical anchor not computed (bridge unavailable)",
		ChemicalAccuracyLimit: 1.6,
		AnchorComputed:        false,
		AnchorSource:          "uncomputed",
	}
	if bridgeClient != nil {
		anchorCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		resp, err := bridgeClient.ComputeClassicalAnchor(anchorCtx, &pb.ClassicalAnchorRequest{
			Molecule:     mol,
			BondDistance: bondDist,
		})
		cancel()
		if err == nil && resp.Success {
			anchor.Method = "PySCF STO-3G Full Configuration Interaction"
			anchor.RhfMeanFieldEnergy = resp.RhfEnergy
			anchor.FciExactEnergy = resp.FciExactEnergy
			anchor.AnchorComputed = true
			anchor.AnchorSource = "pyscf-live"
			slog.Info("campaign proposal ab-initio PySCF anchor", "molecule", mol, "bond_distance_angstrom", bondDist, "fci_hartree", resp.FciExactEnergy, "rhf_hartree", resp.RhfEnergy)
		} else if err != nil {
			slog.Warn("campaign proposal bridge anchor unavailable", "molecule", mol, "error", err)
		}
	}
	if !anchor.AnchorComputed && os.Getenv("KUBONA_ENV") == "production" && os.Getenv("KUBONA_ALLOW_ANCHOR_FALLBACK") != "1" {
		return nil, fmt.Errorf("classical anchor unavailable for %s at R=%.4f A: bridge unreachable; refusing a campaign without a computed anchor (KUBONA_ALLOW_ANCHOR_FALLBACK=1 only permits an explicit illustrative scaffold)", mol, bondDist)
	}

	// Proposed OpenQASM 2.0 circuit. This is a starting proposal, explicitly
	// marked Synthesized; launch binds it to a real run.
	var qasmBuilder strings.Builder
	qasmBuilder.WriteString("OPENQASM 2.0;\ninclude \"qelib1.inc\";\n")
	qasmBuilder.WriteString(fmt.Sprintf("qreg q[%d];\ncreg c[%d];\n", qubits, qubits))
	qasmBuilder.WriteString("h q[0];\n")
	for i := 0; i < qubits; i++ {
		qasmBuilder.WriteString(fmt.Sprintf("rz(%.3f) q[%d];\n", 0.785*(float64(i+1)), i))
	}
	for i := 0; i < qubits-1; i++ {
		qasmBuilder.WriteString(fmt.Sprintf("cx q[%d], q[%d];\n", i, i+1))
	}
	for i := 0; i < qubits; i++ {
		qasmBuilder.WriteString(fmt.Sprintf("measure q[%d] -> c[%d];\n", i, i))
	}
	qasmCode := qasmBuilder.String()

	// Proposed React Flow DAG: wires and parameterized rotations only. No
	// wire-cut/subcircuit nodes: nothing has planned or measured a cut.
	dagNodes := make([]any, 0, qubits*2)
	dagEdges := make([]any, 0, qubits)
	for i := 0; i < qubits; i++ {
		yPos := 70 + (i * 90)
		dagNodes = append(dagNodes, map[string]any{
			"id":       fmt.Sprintf("q%d-in", i),
			"type":     "quantumGate",
			"position": map[string]any{"x": 40, "y": yPos},
			"data":     map[string]any{"type": "Qubit Wire", "label": fmt.Sprintf("q[%d]: |0⟩", i)},
		})
		dagNodes = append(dagNodes, map[string]any{
			"id":       fmt.Sprintf("rz-q%d", i),
			"type":     "quantumGate",
			"position": map[string]any{"x": 220, "y": yPos},
			"data": map[string]any{
				"type":  "Param Rotation",
				"label": fmt.Sprintf("[ Rz(θ%d) ]", i+1),
				"angle": 0.785 * float64(i+1),
			},
		})
		dagEdges = append(dagEdges, map[string]any{
			"id":       fmt.Sprintf("e-in-rz-%d", i),
			"source":   fmt.Sprintf("q%d-in", i),
			"target":   fmt.Sprintf("rz-q%d", i),
			"animated": true,
			"style":    map[string]any{"stroke": "#00f0ff"},
		})
	}

	state := &CampaignState{
		SessionID:    sessionID,
		ResearchGoal: prompt,
		TargetSystem: target,
		SMILES:       smiles,
		QuantumFactors: QuantumFactors{
			TargetMolecule:   mol,
			MolecularFormula: formula,
			BondDistance:     bondDist,
			ActiveElectrons:  qubits,
			ActiveOrbitals:   qubits,
			FermionMapping:   "Parity (two-qubit reduction)",
			QubitCount:       qubits,
			CircuitDepth:     depth,
			TwoQubitGates:    twoQGates,
			AnsatzFamily:     ansatz,
			// Estimated properties are omitted until a live analysis exists;
			// an omitted field is visible as unknown, unlike a zero that can
			// be mistaken for a measured value.
			BarrenPlateauRisk:  "",
			RecommendedBackend: "local_simulator",
			EstimatedSpendUSD:  0.0,
			QASM:               qasmCode,
			Synthesized:        true,
			DagNodes:           dagNodes,
			DagEdges:           dagEdges,
			WireCuts:           []any{},
		},
		ClassicalAnchor: anchor,
		Trajectory:      []TrajectoryStep{},
		Branches:        []any{},
		Epistemics:      nil,
		Notebook:        nil,
		UpdatedAt:       time.Now().UTC(),
	}
	return state, nil
}
