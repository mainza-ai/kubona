package hypotheses

import (
	"strings"
	"testing"

	"github.com/mainza-ai/kubona/core/pkg/agents"
)

func campaign(delta float64, hardware bool, measured bool) *agents.CampaignState {
	c := &agents.CampaignState{TargetSystem: "H2"}
	if measured {
		c.Notebook = &agents.SystematicNotebook{
			Title: "H2", QuantumEnergy: -1.12, ClassicalEnergy: -1.137,
			DeltaMHa: delta,
		}
		c.Measurement = &agents.MeasurementSummary{
			Measured: true, Energy: -1.12, EnergyExact: -1.137,
			DeltaMHa: delta, Hardware: hardware, Claimable: !hardware,
		}
	}
	return c
}

func TestGenerateProducesFalsifiableRankedCandidates(t *testing.T) {
	out := Generate(campaign(15.2, false, true), []string{"W123"}, 20)
	if len(out) < 10 {
		t.Fatalf("candidates = %d, want >= 10", len(out))
	}
	seen := map[string]bool{}
	for i, c := range out {
		if c.Falsifier == "" || c.Statement == "" || c.Rationale == "" {
			t.Fatalf("candidate %d incomplete: %+v", i, c)
		}
		if c.Score < 0 || c.Score > 1 {
			t.Fatalf("score out of range: %+v", c)
		}
		if seen[c.Statement] {
			t.Fatalf("duplicate statement: %q", c.Statement)
		}
		seen[c.Statement] = true
		if i > 0 && out[i-1].Score < c.Score {
			t.Fatalf("not sorted by score at %d", i)
		}
	}
}

func TestGenerateHardwareAndNoMeasurementBranches(t *testing.T) {
	hw := Generate(campaign(20, true, true), nil, 20)
	foundMitigation := false
	for _, c := range hw {
		if c.ExperimentKind == "mitigate" {
			foundMitigation = true
		}
	}
	if !foundMitigation {
		t.Fatal("hardware campaign has no mitigation candidate")
	}

	empty := Generate(&agents.CampaignState{TargetSystem: "LiH"}, nil, 20)
	if len(empty) < 3 {
		t.Fatalf("empty campaign candidates = %d", len(empty))
	}
	if empty[0].ExperimentKind != "vqe" {
		t.Fatalf("first empty-state probe = %+v", empty[0])
	}
}

func TestGenerateDeclaresTheInterventionForEachProbe(t *testing.T) {
	out := Generate(campaign(15.2, false, true), nil, 20)
	want := map[string]string{
		"under-expressive": "layers_delta",
		"optimizer":        "iterations_multiplier",
		"Shot noise":       "shots_multiplier",
		"stable under":     "repeats",
	}
	found := map[string]bool{}
	for _, c := range out {
		for phrase, key := range want {
			if strings.Contains(c.Statement, phrase) {
				if c.SpecPatch[key] == nil {
					t.Fatalf("%q has no %s patch: %+v", phrase, key, c.SpecPatch)
				}
				found[key] = true
			}
		}
	}
	for _, key := range want {
		if !found[key] && key != "repeats" {
			t.Fatalf("no candidate declared the %s intervention", key)
		}
	}
}

func TestGenerateDeduplicatesNearIdenticalStatements(t *testing.T) {
	out := Generate(campaign(10, false, true), nil, 50)
	for i := range out {
		for j := i + 1; j < len(out); j++ {
			if jaccard(out[i].Statement, out[j].Statement) >= 0.8 {
				t.Fatalf("near-duplicate candidates: %q / %q", out[i].Statement, out[j].Statement)
			}
		}
	}
}
