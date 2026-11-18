package api

// Campaign → synthesis facts (AG-3b).
//
// Only recorded campaign values become facts. The canonical molecule row is
// used only when the campaign names that molecule; an unknown target is left
// empty so the synthesizer refuses instead of silently planning H2.

import (
	"strings"

	"github.com/mainza-ai/kubona/core/pkg/agents"
	"github.com/mainza-ai/kubona/core/pkg/plans"
)

func planFactsFromCampaign(c *agents.CampaignState) plans.Facts {
	if c == nil {
		return plans.Facts{}
	}
	f := plans.Facts{
		TargetSystem: c.TargetSystem,
		Goal:         c.ResearchGoal,
		Molecule:     strings.TrimSpace(c.QuantumFactors.TargetMolecule),
		BondDistance: c.QuantumFactors.BondDistance,
	}
	text := strings.TrimSpace(c.TargetSystem + " " + c.ResearchGoal)
	if f.Molecule == "" || f.BondDistance <= 0 {
		spec := agents.LookupMolecule(text)
		if f.Molecule == "" {
			// LookupMolecule falls back to H2 for unmatched text. That fallback
			// is a proposal default, not evidence that the campaign targets H2,
			// so accept it only when the campaign actually names H2.
			if spec.Mol != "H2" || hasUpperToken(text, "H2") {
				f.Molecule = spec.Mol
			}
		}
		if f.BondDistance <= 0 && f.Molecule == spec.Mol {
			f.BondDistance = spec.BondDist
		}
	}
	if c.Measurement != nil {
		f.Ansatz = c.Measurement.Ansatz
		f.Shots = c.Measurement.Shots
		f.Seed = c.Measurement.Seed
	}
	for i := len(c.Experiments) - 1; i >= 0; i-- {
		e := c.Experiments[i]
		if strings.EqualFold(e.Kind, "spin_chain") {
			f.NSites = e.NSites
			f.Model = e.Model
			break
		}
	}
	return f
}

func hasUpperToken(s, want string) bool {
	for _, tok := range strings.FieldsFunc(strings.ToUpper(s), func(r rune) bool {
		return !(r >= 'A' && r <= 'Z' || r >= '0' && r <= '9')
	}) {
		if tok == want {
			return true
		}
	}
	return false
}
