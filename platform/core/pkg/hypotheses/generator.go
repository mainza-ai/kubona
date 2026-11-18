// Package hypotheses derives falsifiable, evidence-grounded research probes
// from a measured campaign state (AG-2).
//
// This is deliberately not an LLM free-association step: every candidate names
// the observation that motivates it, the experiment family that would test it,
// and the observation that would falsify it. Novelty is never invented — it is
// "unmeasured" unless a caller supplies a measured distance. An LLM proposer
// can be layered on top later; it must not replace these diagnostics.
package hypotheses

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/mainza-ai/kubona/core/pkg/agents"
)

// Candidate is one proposed probe before persistence assigns an id.
type Candidate struct {
	Statement      string
	Rationale      string
	Falsifier      string
	ExperimentKind string
	ExperimentHint string
	Literature     []string
	Score          float64
	Source         string
	// Novelty is filled by the caller that has a prior-art corpus. The
	// generator and proposer leave it empty: they have no corpus, so novelty
	// is "unmeasured" rather than guessed.
	NoveltyStatus   string
	NoveltyMethod   string
	NoveltyDistance *float64
}

const chemicalAccuracyMHa = 1.6

// Generate returns ranked, de-duplicated candidates. At least ten distinct
// probes are produced when the campaign has a measured point; fewer only when
// the state genuinely supports fewer.
func Generate(campaign *agents.CampaignState, literature []string, max int) []Candidate {
	if max <= 0 || max > 50 {
		max = 20
	}
	if campaign == nil {
		campaign = &agents.CampaignState{}
	}
	target := strings.TrimSpace(campaign.TargetSystem)
	if target == "" {
		target = "the target system"
	}
	_, hardware, claimable, delta, hasPoint := point(campaign)
	out := []Candidate{}

	add := func(statement, rationale, falsifier, kind, hint string, score float64, refs []string) {
		if refs == nil {
			refs = []string{}
		}
		out = append(out, Candidate{
			Statement: statement, Rationale: rationale, Falsifier: falsifier,
			ExperimentKind: kind, ExperimentHint: hint, Score: clamp(score),
			Source: "diagnostic", Literature: refs,
		})
	}

	if !hasPoint {
		add(fmt.Sprintf("Run a measured baseline of %s before proposing a mechanism.", target),
			"No measured point exists in the campaign, so any explanatory hypothesis is unanchored.",
			"The baseline fails or reports measured=false; then the bottleneck is execution, not the hypothesis.",
			"vqe", "run the smallest measured baseline with a recorded anchor", 0.9, nil)
		add(fmt.Sprintf("Verify the classical anchor for %s independently of the quantum run.", target),
			"Without an anchor, a measured number cannot be interpreted.",
			"The anchor disagrees with a second method (RHF/FCI/DMRG) or is absent.",
			"anchor", "compute the exact/active-space anchor and record it", 0.85, nil)
		add(fmt.Sprintf("Establish prior art for %s before claiming discovery.", target),
			"Novelty is unmeasured; a prior publication would change the objective.",
			"A publication reports the same result at the same scope.",
			"literature", "search OpenAlex/Crossref for the target and mechanism", 0.7, literature)
	} else {
		gapScore := clamp(math.Abs(delta) / 50.0)
		add(fmt.Sprintf("The %s ansatz is under-expressive at the measured point.", target),
			fmt.Sprintf("The measured point misses the anchor by %.3f mHa; expressivity is the first candidate mechanism.", delta),
			fmt.Sprintf("Increasing expressivity leaves the discrepancy above %.1f mHa.", chemicalAccuracyMHa),
			"vqe", "increase layers by 1 and re-run with the same optimizer budget", 0.70+0.2*gapScore, nil)
		add(fmt.Sprintf("The optimizer is trapped in a local minimum for %s.", target),
			"The measured point may reflect optimization, not model capacity.",
			"Multi-start or a stronger optimizer leaves the energy unchanged.",
			"vqe", "run 3 seeded restarts and compare the best energy", 0.68+0.2*gapScore, nil)
		add(fmt.Sprintf("Shot noise explains the %s discrepancy.", target),
			"A finite-shot estimate can be below the exact anchor by chance.",
			"Quadrupling shots does not shrink the discrepancy/uncertainty.",
			"vqe", "re-run with 4x shots and compare CI95", 0.60+0.2*gapScore, nil)
		add(fmt.Sprintf("The active space or basis is the limiting approximation for %s.", target),
			"The model may be under-specified rather than under-optimized.",
			"Enlarging the active space/basis does not move the anchor-relative error.",
			"vqe", "re-run with the next active-space/basis tier", 0.58+0.2*gapScore, nil)
		add(fmt.Sprintf("The molecular geometry is not the one the anchor assumes for %s.", target),
			"A geometry mismatch produces a systematic energy offset.",
			"Re-optimizing the geometry at the same level leaves the residual unchanged.",
			"geometry_optimization", "relax the geometry and re-anchor", 0.50+0.2*gapScore, nil)
		if hardware {
			add(fmt.Sprintf("Hardware noise, not the algorithm, causes the %s gap.", target),
				"The point was measured on a QPU, where calibration and readout noise dominate.",
				"ZNE/calibration refresh does not improve the anchor-relative error.",
				"mitigate", "apply ZNE and refresh calibration before the next run", 0.72+0.2*gapScore, nil)
			add(fmt.Sprintf("The %s calibration snapshot is stale for this claim.", target),
				"Claimability depends on a fresh calibration snapshot.",
				"A fresh calibration does not change the claimability verdict.",
				"calibration", "refresh the provider snapshot and re-evaluate claimability", 0.60+0.2*gapScore, nil)
		} else if !claimable {
			add(fmt.Sprintf("The simulator result for %s is not claimable because the anchor scope differs.", target),
				"Claimability is scoped to the recorded anchor and backend.",
				"Recomputing the anchor at the same scope does not make the result claimable.",
				"anchor", "recompute the anchor at the exact model scope", 0.50, nil)
		}
		add(fmt.Sprintf("A classical method already matches %s at this scale.", target),
			"Quantum advantage cannot be claimed if a classical baseline matches.",
			"A classical RHF/FCI/DMRG run at the same setting matches the measured value within uncertainty.",
			"anchor", "run the strongest classical baseline at the same setting", 0.62, nil)
		add(fmt.Sprintf("Prior art already reports the claimed %s result.", target),
			"Novelty is unmeasured; a prior result changes what counts as discovery.",
			"A publication reports the same result at the same scope.",
			"literature", "search OpenAlex/Crossref for the target and mechanism", 0.45, literature)
		add(fmt.Sprintf("A larger system would distinguish the mechanism from a finite-size effect for %s.", target),
			"The measured point may be a small-system artifact.",
			"The effect disappears or reverses at the next system size.",
			"scaling", "repeat at the next size/active-space tier", 0.50, nil)
		add(fmt.Sprintf("The result for %s is stable under a different seed/backend.", target),
			"A single run cannot separate signal from run-to-run variance.",
			"The conclusion changes across seeds/backends.",
			"vqe_repeats", "repeat with 3 seeds and report the interval", 0.55, nil)
	}

	out = dedupe(out)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Score == out[j].Score {
			return out[i].Statement < out[j].Statement
		}
		return out[i].Score > out[j].Score
	})
	if len(out) > max {
		out = out[:max]
	}
	return out
}

func point(c *agents.CampaignState) (measured, hardware, claimable bool, delta float64, ok bool) {
	if c.Notebook != nil {
		d := c.Notebook.DeltaMHa
		if d == 0 {
			d = math.Abs(c.Notebook.QuantumEnergy-c.Notebook.ClassicalEnergy) * 1000
		}
		hw := c.Measurement != nil && c.Measurement.Hardware
		return true, hw, c.Notebook.ChemicalAccuracy, math.Abs(d), true
	}
	if c.Measurement != nil && c.Measurement.Measured {
		d := c.Measurement.DeltaMHa
		if d == 0 {
			anchor := c.Measurement.EnergyExact
			if anchor == 0 {
				anchor = c.Measurement.FCIEnergy
			}
			if anchor != 0 {
				d = math.Abs(c.Measurement.Energy-anchor) * 1000
			}
		}
		return true, c.Measurement.Hardware, c.Measurement.Claimable, d, true
	}
	return false, false, false, 0, false
}

func dedupe(in []Candidate) []Candidate {
	out := make([]Candidate, 0, len(in))
	for _, c := range in {
		dup := false
		for _, existing := range out {
			if jaccard(c.Statement, existing.Statement) >= 0.8 {
				dup = true
				break
			}
		}
		if !dup {
			out = append(out, c)
		}
	}
	return out
}

func jaccard(a, b string) float64 {
	as := tokens(a)
	bs := tokens(b)
	if len(as) == 0 || len(bs) == 0 {
		return 0
	}
	inter := 0
	for t := range as {
		if bs[t] {
			inter++
		}
	}
	union := len(as) + len(bs) - inter
	if union == 0 {
		return 0
	}
	return float64(inter) / float64(union)
}

func tokens(s string) map[string]bool {
	out := map[string]bool{}
	for _, t := range strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !(r >= 'a' && r <= 'z') && !(r >= '0' && r <= '9')
	}) {
		if len(t) > 2 {
			out[t] = true
		}
	}
	return out
}

func clamp(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
