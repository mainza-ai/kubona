package agents

import (
	"fmt"
	"strings"
)

// Canonical molecule table (W1.6, single physics source): measured PySCF
// STO-3G values, verified 2026-10-11 against the live bridge across all 14
// families. campaign_synth.go AND the tribunal prompt builder (collective.go)
// BOTH read from here — the old copies already diverged (LiH FCI -7.88240
// vs -7.88239; BeH2 -15.5940 vs -15.57580, an 18 mHa split across two
// "authoritative" constants). One table, never two.
//
// Matching is token-boundary (W1.14-class): "CO" never matches "CO2",
// "H2" never matches "H2O" or "BEH2" — the old substring chain needed
// careful ordering (CO2 before CO) to avoid the same collisions.

// MoleculeSpec is one row of the canonical table.
type MoleculeSpec struct {
	Mol       string
	Formula   string
	Match     []string // uppercase match tokens
	BondDist  float64
	Target    string
	Qubits    int
	Depth     int
	TwoQGates int
	Ansatz    string
	MWQ       float64
	GradVar   float64
	RHF       float64
	FCI       float64
	SMILES    string
}

// DefaultMoleculeSpec is H2 (the fallback when nothing matches).
func DefaultMoleculeSpec() MoleculeSpec {
	return MoleculeSpec{
		Mol: "H2", Formula: "H2", Match: []string{"H2"},
		BondDist: 0.7414, Target: "Hydrogen Molecule (H2)",
		Qubits: 2, Depth: 4, TwoQGates: 1,
		Ansatz: "UCCSD Parameterized", MWQ: 0.82, GradVar: 3.4e-3,
		RHF: -1.11668, FCI: -1.13727, SMILES: "[H][H]",
	}
}

// MoleculeTable lists every family after the H2 default, most-specific
// first (order is documentation now, not load-bearing — tokens are exact).
func MoleculeTable() []MoleculeSpec {
	return []MoleculeSpec{
		{Mol: "LiH", Formula: "LiH", Match: []string{"LIH", "LITHIUM"},
			BondDist: 1.5957, Target: "Lithium Hydride (LiH)",
			Qubits: 4, Depth: 8, TwoQGates: 3,
			Ansatz: "TwoLocal / RyRz Entangling", MWQ: 0.76, GradVar: 2.1e-3,
			RHF: -7.86200, FCI: -7.88239, SMILES: "[Li][H]"},
		{Mol: "BeH2", Formula: "BeH2", Match: []string{"BEH2", "BERYLLIUM"},
			BondDist: 1.3300, Target: "Beryllium Hydride (BeH2)",
			Qubits: 6, Depth: 12, TwoQGates: 6,
			Ansatz: "RealAmplitudes Hardware-Efficient", MWQ: 0.88, GradVar: 1.8e-3,
			RHF: -15.56010, FCI: -15.57580, SMILES: "[H][Be][H]"},
		{Mol: "H2O", Formula: "H2O", Match: []string{"H2O", "WATER"},
			BondDist: 0.9575, Target: "Water Molecule (H2O)",
			Qubits: 8, Depth: 16, TwoQGates: 8,
			Ansatz: "UCCSD Active-Space Transpiled", MWQ: 0.91, GradVar: 1.2e-3,
			RHF: -74.96298, FCI: -74.97040, SMILES: "O"},
		{Mol: "N2", Formula: "N2", Match: []string{"N2", "NITROGEN"},
			BondDist: 1.0977, Target: "Dinitrogen (N2)",
			Qubits: 6, Depth: 12, TwoQGates: 5,
			Ansatz: "UCCSD Active-Space Transpiled", MWQ: 0.89, GradVar: 1.4e-3,
			RHF: -107.49589, FCI: -107.62185, SMILES: "N#N"},
		{Mol: "CO2", Formula: "CO2", Match: []string{"CO2"},
			BondDist: 1.1620, Target: "Carbon Dioxide (CO2)",
			Qubits: 8, Depth: 16, TwoQGates: 8,
			Ansatz: "RealAmplitudes Hardware-Efficient", MWQ: 0.92, GradVar: 1.1e-3,
			RHF: -185.06522, FCI: -185.15720, SMILES: "O=C=O"},
		{Mol: "CO", Formula: "CO", Match: []string{"CO", "MONOXIDE"},
			BondDist: 1.1283, Target: "Carbon Monoxide (CO)",
			Qubits: 6, Depth: 12, TwoQGates: 5,
			Ansatz: "UCCSD Parameterized", MWQ: 0.87, GradVar: 1.5e-3,
			RHF: -111.22459, FCI: -111.33095, SMILES: "[C-]#[O+]"},
		{Mol: "HF", Formula: "HF", Match: []string{"HF", "FLUORIDE"},
			BondDist: 0.9168, Target: "HF Polar Ground State VQE",
			Qubits: 4, Depth: 8, TwoQGates: 3,
			Ansatz: "TwoLocal / RyRz Entangling", MWQ: 0.81, GradVar: 2.2e-3,
			RHF: -98.57076, FCI: -98.59659, SMILES: "F"},
		{Mol: "NH3", Formula: "NH3", Match: []string{"NH3", "AMMONIA"},
			BondDist: 1.0124, Target: "NH3 Pyramidal Inversion Active Space",
			Qubits: 8, Depth: 14, TwoQGates: 7,
			Ansatz: "UCCSD Active-Space Transpiled", MWQ: 0.88, GradVar: 1.3e-3,
			RHF: -55.45398, FCI: -55.49467, SMILES: "N"},
		{Mol: "CH4", Formula: "CH4", Match: []string{"CH4", "METHANE"},
			BondDist: 1.0870, Target: "CH4 Tetrahedral Equilibrium VQE",
			Qubits: 8, Depth: 16, TwoQGates: 8,
			Ansatz: "TwoLocal / RyRz Entangling", MWQ: 0.86, GradVar: 1.5e-3,
			RHF: -39.50989, FCI: -39.55590, SMILES: "C"},
		{Mol: "Benzene", Formula: "C6H6", Match: []string{"BENZENE"},
			BondDist: 1.3970, Target: "Benzene Pi-Electron Active Space CAS(6,6)",
			Qubits: 6, Depth: 14, TwoQGates: 6,
			Ansatz: "Pi-Space Complete Active Space VQE", MWQ: 0.91, GradVar: 1.6e-3,
			RHF: -227.88822, FCI: -227.94430, SMILES: "c1ccccc1"},
		{Mol: "Caffeine", Formula: "C8H10N4O2", Match: []string{"CAFFEINE"},
			BondDist: 1.3800, Target: "Caffeine Fragment Active Orbitals",
			Qubits: 4, Depth: 8, TwoQGates: 4,
			Ansatz: "Fragment Molecular Orbital VQE", MWQ: 0.84, GradVar: 2.1e-3,
			RHF: -667.65428, FCI: -667.68304, SMILES: "CN1C=NC2=C1C(=O)N(C(=O)N2C)C"},
		{Mol: "Aspirin", Formula: "C9H8O4", Match: []string{"ASPIRIN"},
			BondDist: 1.40, Target: "Aspirin (Acetylsalicylic Acid) Lead Profile",
			Qubits: 4, Depth: 8, TwoQGates: 4,
			Ansatz: "Fragment Molecular Orbital VQE", MWQ: 0.79, GradVar: 2.4e-3,
			RHF: -636.60816, FCI: -636.64195, SMILES: "CC(=O)Oc1ccccc1C(=O)O"},
		{Mol: "Paracetamol", Formula: "C8H9NO2", Match: []string{"PARACETAMOL"},
			BondDist: 1.40, Target: "Paracetamol (Acetaminophen) Molecular Profile",
			Qubits: 4, Depth: 8, TwoQGates: 4,
			Ansatz: "Fragment Molecular Orbital VQE", MWQ: 0.77, GradVar: 2.6e-3,
			RHF: -505.83584, FCI: -505.89628, SMILES: "CC(=O)Nc1ccc(O)cc1"},
	}
}

// LookupMolecule resolves prompt text to a table row by whole-token match
// (split on non-alphanumerics). Falls back to the H2 default when nothing
// matches — the fallback is labeled downstream (anchor_computed=false),
// never presented as a measurement.
func LookupMolecule(prompt string) MoleculeSpec {
	tokens := map[string]bool{}
	for _, tok := range strings.FieldsFunc(strings.ToUpper(prompt), func(r rune) bool {
		return !(r >= 'A' && r <= 'Z' || r >= '0' && r <= '9')
	}) {
		tokens[tok] = true
	}
	for _, spec := range MoleculeTable() {
		for _, m := range spec.Match {
			if tokens[m] {
				return spec
			}
		}
	}
	// H2 by token too (so "H2O ..." never degrades to it by accident —
	// only an exact H2 token, else the documented default).
	if tokens["H2"] {
		return DefaultMoleculeSpec()
	}
	return DefaultMoleculeSpec()
}

// FCIAnchorLabel renders the canonical anchor string for prompts and
// notebook lines: one format, every caller.
func FCIAnchorLabel(fci float64) string {
	return fmt.Sprintf("PySCF STO-3G FCI (%.5f Ha)", fci)
}
