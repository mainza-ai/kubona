package plans

import (
	"strings"
	"testing"
)

func baseFacts() Facts {
	return Facts{
		TargetSystem: "Hydrogen Molecule (H2)", Goal: "H2 ground state",
		Molecule: "H2", BondDistance: 0.7414,
		Ansatz: "hardware-efficient layers=2", Shots: 4096, Seed: 11,
	}
}

func TestSynthesizeVQEUsesMeasuredBaseline(t *testing.T) {
	spec, basis, err := Synthesize(Request{Method: "vqe"}, baseFacts())
	if err != nil {
		t.Fatalf("synthesize: %v", err)
	}
	if spec["kind"] != "vqe" || spec["molecule"] != "H2" {
		t.Fatalf("spec identity = %+v", spec)
	}
	if spec["bond_distance"] != 0.7414 {
		t.Fatalf("bond distance = %v", spec["bond_distance"])
	}
	if spec["layers"] != 2 || spec["shots"] != 4096 || spec["seed"] != 11 {
		t.Fatalf("baseline not carried: %+v", spec)
	}
	if spec["max_iterations"] != 120 {
		t.Fatalf("default iterations = %v", spec["max_iterations"])
	}
	joined := ""
	for _, b := range basis {
		joined += b + "\n"
	}
	for _, want := range []string{"layers=2 from the campaign", "shots=4096", "seed=11", "molecule=H2"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("basis missing %q:\n%s", want, joined)
		}
	}
}

func TestSynthesizeAppliesHypothesisPatch(t *testing.T) {
	spec, basis, err := Synthesize(Request{
		Method: "vqe", Patch: map[string]any{"layers_delta": 1, "shots_multiplier": 4},
	}, baseFacts())
	if err != nil {
		t.Fatalf("synthesize: %v", err)
	}
	if spec["layers"] != 3 || spec["shots"] != 16384 {
		t.Fatalf("patch not applied: %+v", spec)
	}
	if !strings.Contains(basis[0], "layers") || !strings.Contains(strings.Join(basis, "\n"), "hypothesis patch") {
		t.Fatalf("patch provenance missing: %v", basis)
	}

	spec, _, err = Synthesize(Request{Method: "vqe", Patch: map[string]any{"iterations_multiplier": 2}}, baseFacts())
	if err != nil {
		t.Fatalf("iterations: %v", err)
	}
	if spec["max_iterations"] != 240 {
		t.Fatalf("iterations patch = %v", spec["max_iterations"])
	}
}

func TestSynthesizeRefusesUngroundedOrOutOfRange(t *testing.T) {
	if _, _, err := Synthesize(Request{Method: "vqe"}, Facts{}); err == nil {
		t.Fatal("missing molecule/bond must refuse")
	}
	if _, _, err := Synthesize(Request{Method: "reaction"}, baseFacts()); err == nil {
		t.Fatal("unsupported method must refuse")
	}
	if _, _, err := Synthesize(Request{Method: "vqe", Patch: map[string]any{"layers_delta": 20}}, baseFacts()); err == nil {
		t.Fatal("out-of-range layers must refuse")
	}
	if _, _, err := Synthesize(Request{Method: "vqe", Patch: map[string]any{"shots_multiplier": 1000}}, baseFacts()); err == nil {
		t.Fatal("out-of-range shots must refuse")
	}
	if _, _, err := Synthesize(Request{Method: "vqe", Patch: map[string]any{"layers_delta": "one"}}, baseFacts()); err == nil {
		t.Fatal("non-numeric patch must refuse")
	}
}

func TestSynthesizeSpinChainAndRepeats(t *testing.T) {
	facts := baseFacts()
	facts.NSites = 8
	facts.Model = "tfim"
	spec, basis, err := Synthesize(Request{Method: "spin_chain"}, facts)
	if err != nil {
		t.Fatalf("spin chain: %v", err)
	}
	if spec["n_sites"] != 8 || spec["model"] != "tfim" || spec["periodic"] != true {
		t.Fatalf("spin chain spec = %+v", spec)
	}
	if spec["target_error"] != 0.005 {
		t.Fatalf("target_error = %v", spec["target_error"])
	}
	if !strings.Contains(strings.Join(basis, "\n"), "n_sites=8") {
		t.Fatalf("spin-chain basis = %v", basis)
	}

	rep, _, err := Synthesize(Request{Method: "vqe_repeats", Patch: map[string]any{"repeats": 5}}, baseFacts())
	if err != nil {
		t.Fatalf("repeats: %v", err)
	}
	if rep["repeats"] != 5 {
		t.Fatalf("repeats = %v", rep["repeats"])
	}
}

func TestParseAnsatzLayers(t *testing.T) {
	if n, ok := ParseAnsatzLayers("hardware-efficient layers=4"); !ok || n != 4 {
		t.Fatalf("layers = %d/%t", n, ok)
	}
	for _, bad := range []string{"", "hardware-efficient", "layers=0", "layers=99"} {
		if _, ok := ParseAnsatzLayers(bad); ok {
			t.Fatalf("accepted %q", bad)
		}
	}
}
