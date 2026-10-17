package agents

import "testing"

// W1.6: single physics source — tribunal and synth read the same numbers,
// and token matching never confuses CO/CO2, H2/H2O/BEH2.
func TestMoleculeTableSingleSource(t *testing.T) {
	lih := LookupMolecule("LiH ground state VQE")
	if lih.FCI != -7.88239 {
		t.Fatalf("LiH FCI must be the canonical -7.88239, got %v", lih.FCI)
	}
	be := LookupMolecule("run BEH2 linear")
	if be.FCI != -15.57580 {
		t.Fatalf("BeH2 FCI must be the canonical -15.57580, got %v", be.FCI)
	}
	for _, tc := range []struct{ prompt, want string }{
		{"CO2 dissociation", "CO2"},
		{"CO molecule", "CO"},
		{"H2O ground state", "H2O"},
		{"H2 ground state VQE", "H2"},
		{"BEH2 linear", "BeH2"},
		{"something unknown entirely", "H2"},
	} {
		if got := LookupMolecule(tc.prompt); got.Mol != tc.want {
			t.Errorf("prompt %q routed to %q, want %q", tc.prompt, got.Mol, tc.want)
		}
	}
	if got := FCIAnchorLabel(-1.13727); got != "PySCF STO-3G FCI (-1.13727 Ha)" {
		t.Errorf("anchor label format: %q", got)
	}
}
