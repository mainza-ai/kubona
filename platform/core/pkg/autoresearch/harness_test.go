package autoresearch

import (
	"testing"
)

func TestHarnessRegistry(t *testing.T) {
	reg := NewHarnessRegistry()

	harnesses := reg.List()
	if len(harnesses) != 4 {
		t.Fatalf("expected 4 harnesses, got %d", len(harnesses))
	}

	ansatzHarness, err := reg.Get("ansatz-search")
	if err != nil {
		t.Fatalf("failed to get ansatz-search harness: %v", err)
	}

	// Test invalid artifact
	valid, reason := ansatzHarness.ValidityCheck("print('hello world')")
	if valid {
		t.Fatalf("expected non-quantum artifact to be invalid")
	}
	if reason == "" {
		t.Fatalf("expected reason for invalidity")
	}

	// Test valid artifact
	validCode := `
def build_ansatz(theta):
    circuit.ry(theta[0], 0)
    circuit.rz(theta[1], 1)
    circuit.cx(0, 1)
    circuit.cx(1, 0)
`
	valid, _ = ansatzHarness.ValidityCheck(validCode)
	if !valid {
		t.Fatalf("expected valid quantum circuit artifact")
	}

	score, err := ansatzHarness.Metric(validCode)
	if err != nil {
		t.Fatalf("failed to calculate metric: %v", err)
	}
	if score <= 0.0 || score > 0.1 {
		t.Fatalf("unexpected metric score: %f", score)
	}
}

func TestQAOACombinatorialHarness(t *testing.T) {
	reg := NewHarnessRegistry()
	qaoa, _ := reg.Get("qaoa-combinatorial")

	validYaml := `
layer_1:
  gamma: 0.35
  beta: 0.82
layer_2:
  gamma: 0.18
  beta: 0.44
`
	valid, _ := qaoa.ValidityCheck(validYaml)
	if !valid {
		t.Fatalf("expected valid QAOA schedule")
	}

	ratio, err := qaoa.Metric(validYaml)
	if err != nil || ratio < 0.75 {
		t.Fatalf("expected ratio >= 0.75, got %f", ratio)
	}
}
