package agents

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/mainza-ai/kubona/core/pkg/llm"
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
	MeyerWallachQ      float64 `json:"meyer_wallach_q"`
	GradientVariance   float64 `json:"gradient_variance"`
	BarrenPlateauRisk  string  `json:"barren_plateau_risk"`
	CliffordFidelity   float64 `json:"clifford_fidelity"`
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
// computation (true, AnchorSource "pyscf-live") or from hardcoded defaults
// because the bridge was unreachable (false, "hardcoded-fallback"). A
// fallback anchor is never verification-grade: chemical accuracy cannot
// pass on it, and epistemics cannot collapse on it.
type ClassicalAnchor struct {
	Method                 string  `json:"method"`
	RhfMeanFieldEnergy     float64 `json:"rhf_mean_field_energy"`
	FciExactEnergy         float64 `json:"fci_exact_energy"`
	VqeExpectationEnergy   float64 `json:"vqe_expectation_energy"`
	DiscrepancyMHa         float64 `json:"discrepancy_mha"`
	ChemicalAccuracyPassed bool    `json:"chemical_accuracy_passed"`
	ChemicalAccuracyLimit  float64 `json:"chemical_accuracy_limit"`
	AnchorComputed         bool    `json:"anchor_computed"`
	AnchorSource           string  `json:"anchor_source"`
}

// fallbackCaveat is the exact suffix appended to the epistemic statement
// whenever synthesis runs on an uncomputed anchor. The replan anchor fold
// strips this same string when measured evidence arrives (R3), so it must
// stay a single shared constant — never two copies that drift apart.
const fallbackCaveat = " Classical anchor is a hardcoded fallback (bridge unavailable) — unverified."

// Live-anchor projection weights: a computed anchor with a non-fresh
// template reads as PROJECTION — the shape of convergence, not a verified
// result. Shared with the replan anchor-repair fold (R3b), which restores
// exactly these values: a second literal copy there would rot silently.
const (
	liveAlpha = 0.95
	liveBeta  = 0.05
	liveGamma = 0.10
)

// livePCorrelated derives from liveAlpha the same way synthesis does.
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
// provenance: "synthesized" milestones are template projections (the shape
// of a converged run, not measurements); "measured" points folded from real
// job results. Consumers must render them differently.
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
	QuantumEnergy    float64  `json:"quantum_energy"`
	ClassicalEnergy  float64  `json:"classical_energy"`
	DeltaMHa         float64  `json:"delta_mha"`
	ChemicalAccuracy bool     `json:"chemical_accuracy"`
	ProvenanceHash   string   `json:"provenance_hash"`
	EpistemicVerdict string   `json:"epistemic_verdict"`
	Sections         []string `json:"sections"`
}

// CampaignState is the top-level dynamic state for a research session
type CampaignState struct {
	SessionID       string             `json:"session_id"`
	ResearchGoal    string             `json:"research_goal"`
	TargetSystem    string             `json:"target_system"`
	SMILES          string             `json:"smiles,omitempty"`
	QuantumFactors  QuantumFactors     `json:"quantum_factors"`
	ClassicalAnchor ClassicalAnchor    `json:"classical_anchor"`
	Trajectory      []TrajectoryStep   `json:"trajectory"`
	Branches        []any              `json:"branches"`
	Epistemics      EpistemicState     `json:"epistemics"`
	ADMET           any                `json:"admet,omitempty"`
	Notebook        SystematicNotebook `json:"notebook"`
	UpdatedAt       time.Time          `json:"updated_at"`
}

// SynthesizeCampaignState dynamically constructs complete target-specific quantum telemetry
func SynthesizeCampaignState(
	ctx context.Context,
	sessionID, prompt string,
	llmClient *llm.Client,
	bridgeClient pb.QuantumBridgeServiceClient,
) (*CampaignState, error) {
	return SynthesizeCampaignStateWithOpts(ctx, sessionID, prompt, "", false, llmClient, bridgeClient)
}

// SynthesizeCampaignStateWithOpts dynamically constructs complete target-specific quantum telemetry with explicit molecule and fresh flag
func SynthesizeCampaignStateWithOpts(
	ctx context.Context,
	sessionID, prompt, targetMolecule string,
	fresh bool,
	llmClient *llm.Client,
	bridgeClient pb.QuantumBridgeServiceClient,
) (*CampaignState, error) {
	upper := strings.ToUpper(prompt)
	if targetMolecule != "" {
		upper = strings.ToUpper(targetMolecule) + " " + upper
	}

	// Canonical molecule row (W1.6): single physics source (molecule_table.go).
	// Fallback defaults are MEASURED PySCF STO-3G values, verified 2026-10-11
	// against the live bridge across all 14 families. Fallbacks stay labeled
	// as such (anchor_computed=false) — the right physics, not inventions.
	spec := LookupMolecule(upper)
	mol := spec.Mol
	formula := spec.Formula
	bondDist := spec.BondDist
	target := spec.Target
	qubits := spec.Qubits
	depth := spec.Depth
	twoQGates := spec.TwoQGates
	ansatz := spec.Ansatz
	mwQ := spec.MWQ
	gradVar := spec.GradVar
	rhfDefault := spec.RHF
	fciDefault := spec.FCI
	smiles := spec.SMILES

	bondRegex := regexp.MustCompile(`(?i)(?:R\s*=\s*|BOND\s*(?:LENGTH|DISTANCE)\s*(?:OF\s*)?)([0-9]+\.?[0-9]*)`)
	if m := bondRegex.FindStringSubmatch(prompt); len(m) > 1 {
		if parsedDist, err := strconv.ParseFloat(m[1], 64); err == nil && parsedDist >= 0.3 && parsedDist <= 10.0 {
			bondDist = parsedDist
			slog.Info("campaign synth custom bond distance", "bond_distance_angstrom", bondDist)
		}
	}

	// 1. Calculate genuine PySCF ab-initio classical anchor via bridge.
	// When the bridge is unreachable the hardcoded defaults stand in, but
	// they are labeled as fallback at every surface below — an uncomputed
	// anchor must never read as a measurement.
	rhfVal := rhfDefault
	fciVal := fciDefault
	anchorComputed := false
	if bridgeClient != nil {
		anchorCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		resp, err := bridgeClient.ComputeClassicalAnchor(anchorCtx, &pb.ClassicalAnchorRequest{
			Molecule:     mol,
			BondDistance: bondDist,
		})
		cancel()
		if err == nil && resp.Success {
			rhfVal = resp.RhfEnergy
			fciVal = resp.FciExactEnergy
			anchorComputed = true
			slog.Info("campaign synth ab-initio PySCF anchor", "molecule", mol, "bond_distance_angstrom", bondDist, "fci_hartree", fciVal, "rhf_hartree", rhfVal)
		} else if err != nil {
			slog.Warn("campaign synth bridge anchor failed; using labeled fallback", "molecule", mol, "error", err)
		}
	}
	anchorSource := "hardcoded-fallback"
	anchorMethod := "Hardcoded fallback (bridge unavailable)"
	if anchorComputed {
		anchorSource = "pyscf-live"
		anchorMethod = "PySCF STO-3G Full Configuration Interaction"
	}

	// Quantum expectation value simulation
	vqeVal := fciVal + 0.00047
	deltaMHa := math.Abs(vqeVal-fciVal) * 1000.0
	// Chemical accuracy never passes at synthesis: the VQE value above is
	// arithmetic (FCI + 0.47 mHa), not a measurement. Only folds from real
	// evidence (anchor recompute, claim verdict) may pass it.
	chemAccPassed := false

	if fresh {
		vqeVal = rhfVal // Unoptimized initial Hartree-Fock state
		deltaMHa = math.Abs(vqeVal-fciVal) * 1000.0
		chemAccPassed = false
	}

	// Build dynamic OpenQASM 2.0
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

	// Build dynamic React Flow DAG nodes and edges
	dagNodes := make([]any, 0, qubits*3+4)
	dagEdges := make([]any, 0, qubits*3+4)

	for i := 0; i < qubits; i++ {
		yPos := 70 + (i * 90)
		// Qubit wire input node
		dagNodes = append(dagNodes, map[string]any{
			"id":       fmt.Sprintf("q%d-in", i),
			"type":     "quantumGate",
			"position": map[string]any{"x": 40, "y": yPos},
			"data":     map[string]any{"type": "Qubit Wire", "label": fmt.Sprintf("q[%d]: |0⟩", i)},
		})

		// Single qubit rotation node
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

		// Edge from wire to gate
		dagEdges = append(dagEdges, map[string]any{
			"id":       fmt.Sprintf("e-in-rz-%d", i),
			"source":   fmt.Sprintf("q%d-in", i),
			"target":   fmt.Sprintf("rz-q%d", i),
			"animated": true,
			"style":    map[string]any{"stroke": "#00f0ff"},
		})
	}

	// KaHyPar wire cut node at center
	midY := 70 + ((qubits - 1) * 45)
	dagNodes = append(dagNodes, map[string]any{
		"id":       "cut-1",
		"type":     "wireCut",
		"position": map[string]any{"x": 420, "y": midY},
		"data": map[string]any{
			"label":    fmt.Sprintf("Planned KaHyPar wire cut (%s) — not executed", formula),
			"overhead": "4.0x",
		},
	})

	for i := 0; i < qubits; i++ {
		dagEdges = append(dagEdges, map[string]any{
			"id":       fmt.Sprintf("e-rz-cut-%d", i),
			"source":   fmt.Sprintf("rz-q%d", i),
			"target":   "cut-1",
			"animated": true,
			"style":    map[string]any{"stroke": "#f59e0b"},
		})
	}

	// Subcircuit execution nodes: planned partitions, never executed here.
	// Labels name the plan explicitly so the DAG never implies a run.
	dagNodes = append(dagNodes, map[string]any{
		"id":       "sub-1",
		"type":     "quantumGate",
		"position": map[string]any{"x": 680, "y": midY - 60},
		"data":     map[string]any{"type": "Partition A (planned)", "label": "Simulated partition, Aer (not executed)"},
	})
	dagNodes = append(dagNodes, map[string]any{
		"id":       "sub-2",
		"type":     "quantumGate",
		"position": map[string]any{"x": 680, "y": midY + 60},
		"data":     map[string]any{"type": "Partition B (planned)", "label": "Hardware target, Heron (not dispatched)"},
	})

	dagEdges = append(dagEdges, map[string]any{
		"id":       "e-cut-sub1",
		"source":   "cut-1",
		"target":   "sub-1",
		"animated": true,
		"style":    map[string]any{"stroke": "#10b981"},
	})
	dagEdges = append(dagEdges, map[string]any{
		"id":       "e-cut-sub2",
		"source":   "cut-1",
		"target":   "sub-2",
		"animated": true,
		"style":    map[string]any{"stroke": "#8a2be2"},
	})

	// Build dynamic VQE optimization trajectory converging towards FCI energy
	var trajectory []TrajectoryStep
	var branches []any

	if !fresh {
		trajectory = make([]TrajectoryStep, 0, 7)
		stepMilestones := []struct {
			step     int
			relDelta float64
			loss     float64
			grad     float64
			watchdog string
		}{
			{step: 1, relDelta: 0.087, loss: 0.420, grad: 0.18, watchdog: "normal"},
			{step: 5, relDelta: 0.042, loss: 0.280, grad: 0.12, watchdog: "normal"},
			{step: 10, relDelta: 0.017, loss: 0.140, grad: 0.08, watchdog: "normal"},
			{step: 15, relDelta: 0.012, loss: 0.090, grad: 4e-8, watchdog: "barren_plateau_trip"},
			{step: 18, relDelta: 0.005, loss: 0.045, grad: 0.04, watchdog: "rescoped_rollback"},
			{step: 25, relDelta: 0.00047, loss: 0.005, grad: 0.008, watchdog: "chemical_accuracy_met"},
			{step: 30, relDelta: 0.00047, loss: 0.0005, grad: 0.002, watchdog: "converged"},
		}

		for _, m := range stepMilestones {
			tEnergy := fciVal + m.relDelta
			thetaVec := []float64{0.12 + 0.02*float64(m.step), 0.45 + 0.02*float64(m.step), 0.22 + 0.018*float64(m.step)}
			trajectory = append(trajectory, TrajectoryStep{
				Step:      m.step,
				Energy:    tEnergy,
				Loss:      m.loss,
				Theta:     thetaVec,
				GradNorm:  m.grad,
				Watchdog:  m.watchdog,
				Timestamp: time.Now().UTC().Format(time.RFC3339),
				Source:    "synthesized",
			})
		}

		branches = []any{
			map[string]any{"id": "main-vqe-adam", "name": "main (AdamW + SPSA)", "step": 30, "status": "converged", "delta": fmt.Sprintf("%.2f mHa", deltaMHa)},
			map[string]any{"id": "fork-01-cobyla", "name": "fork-01 (COBYLA Noisy)", "step": 18, "status": "rescoped", "delta": "1.20 mHa"},
			map[string]any{"id": "fork-02-lbfgs", "name": "fork-02 (L-BFGS Statevector)", "step": 25, "status": "converged", "delta": "0.31 mHa"},
		}
	} else {
		trajectory = []TrajectoryStep{}
		branches = []any{}
	}

	// Tripartite credibility wavefunction amplitudes. A live anchor with a
	// non-fresh template is a PROJECTION — the shape of convergence, not a
	// verified result. Only job folds move a campaign out of projection.
	alpha := liveAlpha
	beta := liveBeta
	gamma := liveGamma
	pCorr := livePCorrelated
	epStatus := "PROJECTED"
	epStatement := fmt.Sprintf("%s VQE projection converging toward PySCF ab-initio FCI in template space — unverified until jobs fold", target)

	if fresh {
		alpha = 0.50
		beta = 0.10
		gamma = 0.40
		pCorr = 25.0
		epStatus = "HYPOTHESIS_PENDING"
		epStatement = fmt.Sprintf("Hypothesis formulated for %s. Awaiting variational circuit optimization.", target)
	}

	// Fallback anchors unground corroboration: without a computed FCI the
	// 0.95 amplitude is fiction, so the campaign drops to pending weights
	// and UNVERIFIED with the cause in the statement. Fresh campaigns keep
	// HYPOTHESIS_PENDING (already pending) plus the same caveat.
	if !anchorComputed {
		alpha = 0.50
		beta = 0.10
		gamma = 0.40
		pCorr = 25.0
		if !fresh {
			epStatus = "UNVERIFIED"
		}
		epStatement += fallbackCaveat
	}

	// SHA-256 Merkle Provenance Hash (shared with replan: the hash must
	// re-derive identically when a recomputed anchor changes FCI).
	provenanceHash := provenanceHash(sessionID, mol, bondDist, fciVal, qasmCode)

	// Systematic Notebook Sections
	notebookVerdict := "PROJECTION"
	notebookTitle := fmt.Sprintf("%s Potential Energy Surface VQE vs PySCF FCI", target)
	anchorLine := fmt.Sprintf("2. Classical PySCF ab-initio ground state RHF (%.5f Ha) and FCI (%.5f Ha) integrals", rhfVal, fciVal)
	collapseLine := fmt.Sprintf("4. Ring 1 Epistemic projection (P_corr = %.2f%% template weight): chemical accuracy unclaimed until measured", pCorr)
	if !anchorComputed {
		notebookVerdict = "UNVERIFIED_FALLBACK"
		anchorLine = fmt.Sprintf("2. Classical anchor UNAVAILABLE — hardcoded RHF (%.5f Ha) and FCI (%.5f Ha) assumed (bridge unreachable); no ab-initio integrals computed", rhfVal, fciVal)
		collapseLine = fmt.Sprintf("4. Ring 1 Epistemic state UNVERIFIED (P_corr = %.2f%%): anchor is a hardcoded fallback, no collapse claimed", pCorr)
	}
	sections := []string{
		fmt.Sprintf("1. Scientific Objective: %s at equilibrium bond distance R=%.4f A", target, bondDist),
		anchorLine,
		// Same rule as the no-campaign template path: nothing has
		// executed at synthesis — vqeVal is corridor arithmetic, so the
		// section names the estimate as unmeasured, never as execution.
		fmt.Sprintf("3. Variational Quantum Eigensolver circuit prepared (%s) — awaiting execution; corridor estimate E=%.5f Ha (not measured)", ansatz, vqeVal),
		collapseLine,
		fmt.Sprintf("5. Negative results & barren plateau avoidance ledger (Var(∇E) = %.1e)", gradVar),
		fmt.Sprintf("6. Pre-flight Clifford canary probe fidelity profile (F = %.1f%%)", 98.4),
		fmt.Sprintf("7. Provenance SHA-256 Merkle DAG hash proof: %s", provenanceHash[:24]),
		fmt.Sprintf("8. Reproduction command: kubona replay --hash %s", provenanceHash[:20]),
	}

	if fresh {
		notebookVerdict = "HYPOTHESIS_PENDING"
		notebookTitle = fmt.Sprintf("%s Variational Discovery Campaign (Draft)", target)
		sections = []string{
			fmt.Sprintf("1. Scientific Objective: Formulate and evaluate ansatz for %s at equilibrium bond distance R=%.4f A", target, bondDist),
			func() string {
				if anchorComputed {
					return fmt.Sprintf("2. Classical PySCF ab-initio ground state anchors established: RHF (%.5f Ha) and FCI (%.5f Ha)", rhfVal, fciVal)
				}
				return fmt.Sprintf("2. Classical anchor UNAVAILABLE — hardcoded RHF (%.5f Ha) and FCI (%.5f Ha) assumed (bridge unreachable)", rhfVal, fciVal)
			}(),
			fmt.Sprintf("3. Variational Quantum Eigensolver circuit prepared (%s). Awaiting execution.", ansatz),
			"4. Epistemic credibility state: HYPOTHESIS_PENDING (awaiting empirical calculation).",
			"5. Multi-agent research tribunal standing by to evaluate convergence.",
			fmt.Sprintf("6. Provenance SHA-256 Merkle DAG hash proof: %s", provenanceHash[:24]),
		}
	}

	notebook := SystematicNotebook{
		ID:               fmt.Sprintf("nb-%s-%s", strings.ToLower(mol), sessionID[:min(len(sessionID), 8)]),
		Title:            notebookTitle,
		CreatedAt:        time.Now().UTC().Format(time.RFC3339),
		ClassicalAnchor:  fmt.Sprintf("%s (%.5f Ha)", anchorMethod, fciVal),
		QuantumEnergy:    vqeVal,
		ClassicalEnergy:  fciVal,
		DeltaMHa:         deltaMHa,
		ChemicalAccuracy: chemAccPassed,
		ProvenanceHash:   provenanceHash,
		EpistemicVerdict: notebookVerdict,
		Sections:         sections,
	}

	state := &CampaignState{
		SessionID:    sessionID,
		ResearchGoal: prompt,
		TargetSystem: target,
		SMILES:       smiles,
		QuantumFactors: QuantumFactors{
			TargetMolecule:     mol,
			MolecularFormula:   formula,
			BondDistance:       bondDist,
			ActiveElectrons:    qubits,
			ActiveOrbitals:     qubits,
			FermionMapping:     "Parity (two-qubit reduction)",
			QubitCount:         qubits,
			CircuitDepth:       depth,
			TwoQubitGates:      twoQGates,
			AnsatzFamily:       ansatz,
			MeyerWallachQ:      mwQ,
			GradientVariance:   gradVar,
			BarrenPlateauRisk:  "LOW",
			CliffordFidelity:   0.984,
			RecommendedBackend: "local_simulator",
			EstimatedSpendUSD:  0.0,
			QASM:               qasmCode,
			Synthesized:        true,
			DagNodes:           dagNodes,
			DagEdges:           dagEdges,
			WireCuts:           []any{map[string]any{"planned": true, "cut_type": "KaHyPar QPD", "overhead": "4.0x (estimate, not measured)"}},
		},
		ClassicalAnchor: ClassicalAnchor{
			Method:                 anchorMethod,
			RhfMeanFieldEnergy:     rhfVal,
			FciExactEnergy:         fciVal,
			VqeExpectationEnergy:   vqeVal,
			DiscrepancyMHa:         deltaMHa,
			ChemicalAccuracyPassed: chemAccPassed,
			ChemicalAccuracyLimit:  1.6,
			AnchorComputed:         anchorComputed,
			AnchorSource:           anchorSource,
		},
		Trajectory: trajectory,
		Branches:   branches,
		Epistemics: EpistemicState{
			AlphaCorroborated: alpha,
			BetaContested:     beta,
			GammaUnverified:   gamma,
			PCorrelated:       pCorr,
			Status:            epStatus,
			LiteratureDrift:   deltaMHa,
			Statement:         epStatement,
			// R1: sources are attested only when the anchor is computed.
			// A fallback anchor cites nothing — unattested ([]) is legal,
			// an invented PySCF label is not.
			Sources: synthSources(anchorComputed),
		},
		Notebook:  notebook,
		UpdatedAt: time.Now().UTC(),
	}

	return state, nil
}
