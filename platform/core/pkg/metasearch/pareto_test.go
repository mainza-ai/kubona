package metasearch

import (
	"testing"
)

func TestParetoDominanceMinMax(t *testing.T) {
	frontier := DefaultQuantumFrontier()

	// Candidate A: energy_error=0.005, two_qubit_depth=4, entanglement=0.8
	candA := &ParetoCandidate{
		ID:        "cand-A",
		Iteration: 1,
		Objectives: map[string]float64{
			"energy_error":    0.005,
			"two_qubit_depth": 4.0,
			"entanglement":    0.80,
		},
	}

	// Candidate B: energy_error=0.002 (better), two_qubit_depth=2 (better), entanglement=0.90 (better)
	candB := &ParetoCandidate{
		ID:        "cand-B",
		Iteration: 2,
		Objectives: map[string]float64{
			"energy_error":    0.002,
			"two_qubit_depth": 2.0,
			"entanglement":    0.90,
		},
	}

	// B strictly dominates A
	if !frontier.Dominates(candB, candA) {
		t.Fatalf("expected candB to dominate candA across all 3 objectives")
	}
	if frontier.Dominates(candA, candB) {
		t.Fatalf("candA must NOT dominate candB")
	}
}

func TestParetoFrontierPruning(t *testing.T) {
	frontier := DefaultQuantumFrontier()

	// Point 1
	p1 := &ParetoCandidate{
		ID: "p1",
		Objectives: map[string]float64{
			"energy_error":    0.010,
			"two_qubit_depth": 6.0,
			"entanglement":    0.50,
		},
	}
	added := frontier.AddCandidate(p1)
	if !added || frontier.Size() != 1 {
		t.Fatalf("expected p1 to be added, size=1")
	}

	// Point 2: trade-off point (better energy, worse depth) -> non-dominated
	p2 := &ParetoCandidate{
		ID: "p2",
		Objectives: map[string]float64{
			"energy_error":    0.005, // better
			"two_qubit_depth": 8.0,   // worse
			"entanglement":    0.50,
		},
	}
	added = frontier.AddCandidate(p2)
	if !added || frontier.Size() != 2 {
		t.Fatalf("expected trade-off p2 to be added to frontier, size=2")
	}

	// Point 3: strictly superior point (beats p1 in all dimensions, beats p2 in depth)
	p3 := &ParetoCandidate{
		ID: "p3",
		Objectives: map[string]float64{
			"energy_error":    0.001,
			"two_qubit_depth": 2.0,
			"entanglement":    0.95,
		},
	}
	added = frontier.AddCandidate(p3)
	if !added {
		t.Fatalf("expected p3 to be added")
	}

	// p3 dominates p1 and p2! Frontier should prune p1 and p2, leaving only p3
	points := frontier.Points()
	if len(points) != 1 || points[0].ID != "p3" {
		t.Fatalf("expected p3 to dominate and prune older points, got %d points", len(points))
	}
}

func TestNonDominatedSorting(t *testing.T) {
	frontier := DefaultQuantumFrontier()

	cands := []*ParetoCandidate{
		{
			ID: "rank1-A",
			Objectives: map[string]float64{
				"energy_error":    0.001,
				"two_qubit_depth": 4.0,
				"entanglement":    0.80,
			},
		},
		{
			ID: "rank1-B",
			Objectives: map[string]float64{
				"energy_error":    0.005,
				"two_qubit_depth": 2.0,
				"entanglement":    0.80,
			},
		},
		{
			ID: "rank2-C", // Dominated by rank1-A
			Objectives: map[string]float64{
				"energy_error":    0.002,
				"two_qubit_depth": 5.0,
				"entanglement":    0.75,
			},
		},
	}

	fronts := frontier.NonDominatedSort(cands)
	if len(fronts) < 2 {
		t.Fatalf("expected at least 2 fronts, got %d", len(fronts))
	}

	// Front 0 should contain rank1-A and rank1-B
	if len(fronts[0]) != 2 {
		t.Fatalf("expected 2 candidates in front 0, got %d", len(fronts[0]))
	}

	// Front 1 should contain rank2-C
	if len(fronts[1]) != 1 || fronts[1][0].ID != "rank2-C" {
		t.Fatalf("expected rank2-C in front 1")
	}
}

func TestBestByObjective(t *testing.T) {
	frontier := DefaultQuantumFrontier()

	p1 := &ParetoCandidate{
		ID: "low-energy",
		Objectives: map[string]float64{
			"energy_error":    0.0001,
			"two_qubit_depth": 10.0,
			"entanglement":    0.50,
		},
	}
	p2 := &ParetoCandidate{
		ID: "shallow-depth",
		Objectives: map[string]float64{
			"energy_error":    0.010,
			"two_qubit_depth": 2.0,
			"entanglement":    0.95,
		},
	}

	frontier.AddCandidate(p1)
	frontier.AddCandidate(p2)

	bestEnergy := frontier.BestByObjective("energy_error")
	if bestEnergy.ID != "low-energy" {
		t.Fatalf("expected low-energy point, got %s", bestEnergy.ID)
	}

	bestDepth := frontier.BestByObjective("two_qubit_depth")
	if bestDepth.ID != "shallow-depth" {
		t.Fatalf("expected shallow-depth point, got %s", bestDepth.ID)
	}

	bestEntanglement := frontier.BestByObjective("entanglement")
	if bestEntanglement.ID != "shallow-depth" {
		t.Fatalf("expected shallow-depth (entanglement=0.95), got %s", bestEntanglement.ID)
	}
}
