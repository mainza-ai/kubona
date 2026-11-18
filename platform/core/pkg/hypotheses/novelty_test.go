package hypotheses

import "testing"

func TestMeasureNoveltyRequiresAReferenceCorpus(t *testing.T) {
	if _, _, ok := MeasureNovelty("The ansatz is under-expressive", nil); ok {
		t.Fatal("no references must not produce a distance")
	}
	if _, _, ok := MeasureNovelty("The ansatz is under-expressive", []PriorArt{{ID: "W1"}}); ok {
		t.Fatal("references without text must not produce a distance")
	}
	if _, _, ok := MeasureNovelty("", []PriorArt{{ID: "W1", Title: "H2 ground state"}}); ok {
		t.Fatal("an empty statement must not produce a distance")
	}
}

func TestMeasureNoveltyContainmentAndClosestRecord(t *testing.T) {
	refs := []PriorArt{
		{ID: "W1", Title: "Unrelated lattice thermalization"},
		{ID: "W2", Title: "Hardware-efficient ansatz for H2 ground state energy"},
	}
	d, method, ok := MeasureNovelty("The hardware-efficient ansatz for the H2 ground state is under-expressive", refs)
	if !ok {
		t.Fatal("expected a measured distance")
	}
	if method != NoveltyMethodLexicalContainmentV1 {
		t.Fatalf("method = %q", method)
	}
	if d < 0 || d > 1 {
		t.Fatalf("distance out of range: %v", d)
	}
	// The statement's content words are almost all in W2, so novelty is low.
	if d > 0.3 {
		t.Fatalf("distance = %v, want <= 0.3 for a near-contained statement", d)
	}

	disjoint := PriorArt{ID: "W3", Title: "Quantum error correction thresholds in superconducting qubits"}
	d2, _, ok := MeasureNovelty("The ansatz is under-expressive", []PriorArt{disjoint})
	if !ok {
		t.Fatal("expected a measured distance for a disjoint record")
	}
	if d2 < 0.5 {
		t.Fatalf("disjoint distance = %v, want high novelty", d2)
	}
}
