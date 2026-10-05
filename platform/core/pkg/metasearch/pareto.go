package metasearch

import (
	"sort"
	"sync"
)

type ObjectiveDirection int

const (
	Minimize ObjectiveDirection = 1
	Maximize ObjectiveDirection = -1
)

// ParetoCandidate represents an evaluated quantum artifact in multi-objective space
type ParetoCandidate struct {
	ID         string             `json:"id"`
	Iteration  int                `json:"iteration"`
	Artifact   string             `json:"artifact"`
	Objectives map[string]float64 `json:"objectives"` // e.g. "energy_error": 0.001, "cnot_count": 4, "entanglement": 0.85
	Rank       int                `json:"rank"`
}

// ParetoFrontier maintains the non-dominated Pareto frontier across competing quantum objectives
type ParetoFrontier struct {
	mu         sync.RWMutex
	directions map[string]ObjectiveDirection
	points     []*ParetoCandidate
}

// NewParetoFrontier creates a new Pareto frontier with explicit objective directions
func NewParetoFrontier(directions map[string]ObjectiveDirection) *ParetoFrontier {
	dirCopy := make(map[string]ObjectiveDirection)
	for k, v := range directions {
		dirCopy[k] = v
	}
	return &ParetoFrontier{
		directions: dirCopy,
		points:     make([]*ParetoCandidate, 0),
	}
}

// DefaultQuantumFrontier creates standard 3-objective frontier:
// - energy_error: Minimize
// - two_qubit_depth: Minimize
// - entanglement: Maximize
func DefaultQuantumFrontier() *ParetoFrontier {
	return NewParetoFrontier(map[string]ObjectiveDirection{
		"energy_error":    Minimize,
		"two_qubit_depth": Minimize,
		"entanglement":    Maximize,
	})
}

// Dominates checks whether candidate A strictly Pareto-dominates candidate B:
// A dominates B if for all objectives, A is at least as good as B, and for at least one objective, A is strictly better.
func (f *ParetoFrontier) Dominates(a, b *ParetoCandidate) bool {
	strictlyBetter := false

	for objName, dir := range f.directions {
		valA, existsA := a.Objectives[objName]
		valB, existsB := b.Objectives[objName]
		if !existsA || !existsB {
			continue
		}

		if dir == Minimize {
			if valA > valB {
				return false // A is worse than B on this objective
			}
			if valA < valB {
				strictlyBetter = true
			}
		} else { // Maximize
			if valA < valB {
				return false // A is worse than B on this objective
			}
			if valA > valB {
				strictlyBetter = true
			}
		}
	}

	return strictlyBetter
}

// AddCandidate evaluates a candidate against the frontier.
// If it is dominated by any point in the frontier, it is rejected (returns false).
// If it is non-dominated, it is added to the frontier, and any existing points it dominates are pruned (returns true).
func (f *ParetoFrontier) AddCandidate(cand *ParetoCandidate) bool {
	f.mu.Lock()
	defer f.mu.Unlock()

	// 1. Check if candidate is dominated by any existing frontier point
	for _, existing := range f.points {
		if f.Dominates(existing, cand) {
			return false // Candidate is dominated, not on frontier
		}
	}

	// 2. Candidate is non-dominated; prune any existing points that candidate dominates
	var survivorPoints []*ParetoCandidate
	for _, existing := range f.points {
		if !f.Dominates(cand, existing) {
			survivorPoints = append(survivorPoints, existing)
		}
	}

	cand.Rank = 1
	survivorPoints = append(survivorPoints, cand)
	f.points = survivorPoints
	return true
}

// Points returns a copy of the current non-dominated frontier candidates
func (f *ParetoFrontier) Points() []*ParetoCandidate {
	f.mu.RLock()
	defer f.mu.RUnlock()

	res := make([]*ParetoCandidate, len(f.points))
	copy(res, f.points)
	return res
}

// Size returns the count of non-dominated solutions on the frontier
func (f *ParetoFrontier) Size() int {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return len(f.points)
}

// BestByObjective returns the point that achieves the best single value for a given objective
func (f *ParetoFrontier) BestByObjective(objectiveName string) *ParetoCandidate {
	f.mu.RLock()
	defer f.mu.RUnlock()

	if len(f.points) == 0 {
		return nil
	}

	dir, exists := f.directions[objectiveName]
	if !exists {
		return nil
	}

	best := f.points[0]
	bestVal := best.Objectives[objectiveName]

	for _, pt := range f.points[1:] {
		val := pt.Objectives[objectiveName]
		if dir == Minimize {
			if val < bestVal {
				best = pt
				bestVal = val
			}
		} else {
			if val > bestVal {
				best = pt
				bestVal = val
			}
		}
	}

	return best
}

// NonDominatedSort sorts any arbitrary slice of candidates into Pareto fronts (ranks 1, 2, ...)
func (f *ParetoFrontier) NonDominatedSort(candidates []*ParetoCandidate) [][]*ParetoCandidate {
	if len(candidates) == 0 {
		return nil
	}

	dominationCount := make(map[string]int)
	dominatedSet := make(map[string][]*ParetoCandidate)
	var firstFront []*ParetoCandidate

	for _, p := range candidates {
		dominationCount[p.ID] = 0
		dominatedSet[p.ID] = make([]*ParetoCandidate, 0)

		for _, q := range candidates {
			if p.ID == q.ID {
				continue
			}
			if f.Dominates(p, q) {
				dominatedSet[p.ID] = append(dominatedSet[p.ID], q)
			} else if f.Dominates(q, p) {
				dominationCount[p.ID]++
			}
		}

		if dominationCount[p.ID] == 0 {
			p.Rank = 1
			firstFront = append(firstFront, p)
		}
	}

	var fronts [][]*ParetoCandidate
	fronts = append(fronts, firstFront)

	currentFront := firstFront
	currentRank := 1

	for len(currentFront) > 0 {
		var nextFront []*ParetoCandidate
		for _, p := range currentFront {
			for _, q := range dominatedSet[p.ID] {
				dominationCount[q.ID]--
				if dominationCount[q.ID] == 0 {
					q.Rank = currentRank + 1
					nextFront = append(nextFront, q)
				}
			}
		}
		currentRank++
		if len(nextFront) > 0 {
			fronts = append(fronts, nextFront)
		}
		currentFront = nextFront
	}

	// Sort candidates within each front by primary objective
	for _, front := range fronts {
		sort.Slice(front, func(i, j int) bool {
			return front[i].Iteration < front[j].Iteration
		})
	}

	return fronts
}
