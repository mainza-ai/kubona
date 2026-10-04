package memory

import (
	"testing"
)

func TestHindsightMemoryLifecycle(t *testing.T) {
	store := NewHindsightStore()

	// Retain world fact
	fact := store.Retain(BankWorldFacts, "H2 ground state equilibrium bond distance is 0.7414 Angstroms", 0.95)
	if fact.Bank != BankWorldFacts {
		t.Fatalf("expected world facts bank")
	}

	// Retain experience
	exp := store.Retain(BankExperiences, "Trial run #12 diverged due to vanishing gradients on 6-qubit hardware", 0.8)
	if exp.Bank != BankExperiences {
		t.Fatalf("expected experiences bank")
	}

	// Recall with token budget
	results := store.Recall(BankWorldFacts, "0.7414", 50)
	if len(results) != 1 || results[0].ID != fact.ID {
		t.Fatalf("expected to recall world fact within token budget")
	}

	// Small token budget should prevent over-packing
	tinyRecall := store.Recall("", "", 5)
	if len(tinyRecall) > 1 {
		t.Fatalf("tiny token budget exceeded")
	}

	// Decay test: after 20 hours, experience score should decay
	initialScore := exp.EffectiveScore
	store.Decay(20.0)
	if exp.EffectiveScore >= initialScore {
		t.Fatalf("expected experience score to decay from %f, got %f", initialScore, exp.EffectiveScore)
	}

	// Reflect into mental model
	model := store.Reflect("Avoid deep entangler layers beyond 4 qubits without barren plateau pre-checks")
	if model.Bank != BankMentalModels || model.EffectiveScore != 1.0 {
		t.Fatalf("unexpected reflected model: %+v", model)
	}
}
