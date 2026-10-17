package agents

import (
	"encoding/json"
	"testing"
)

// P1-4 step 2, part 1 (freeze, no behavior change): the Go side of the
// future single verdict schema. Today the cross-rail facts are: turns
// carry a lifecycle verdict string ("PROPOSED"/"ANCHORED"/"AUDITED"/
// "COLLAPSED", deliberation "pass"/"fail") plus an R3 coverage map, and
// the whole turn serializes as JSON. This pins the wire shape so ANY
// future unification (Python models mirroring Go verdicts) has a
// baseline that fails loudly on drift. It makes NO canonical choice:
// which vocabulary the finding-level verdict maps to is still the
// operator's step-3-adjacent decision (see the design page).
func TestTribunalTurnVerdictWireShape(t *testing.T) {
	turn := TribunalTurn{
		TurnIndex: 2,
		Role:      RoleDisconfirmationOfficer,
		Name:      "officer",
		Content:   "finding rejected: differs from anchor",
		Verdict:   "AUDITED",
		Coverage: map[string]CoverageDecl{
			FacetAnsatz:   covered(),
			FacetAnchor:   covered(),
			FacetAudit:    covered(),
			FacetProposal: uncovered("no proposal this turn"),
		},
	}
	raw, err := json.Marshal(turn)
	if err != nil {
		t.Fatalf("turn must serialize: %v", err)
	}
	var back TribunalTurn
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatalf("turn must round-trip: %v", err)
	}
	if back.Verdict != "AUDITED" {
		t.Fatalf("verdict string must survive the wire, got %q", back.Verdict)
	}
	if len(back.Coverage) != 4 {
		t.Fatalf("all four facets must survive the wire, got %d", len(back.Coverage))
	}
	if back.Coverage[FacetProposal].Available {
		t.Fatal("unobserved facet must stay unobserved across the wire")
	}
	if back.Coverage[FacetAnchor].Reason != "" {
		t.Fatal("covered facet must carry no reason")
	}

	// The lifecycle vocabulary the tribunal emits (collective.go sets
	// exactly these four across the turn pipeline). Each must survive
	// the wire: if a state is renamed, this fails and the cross-rail
	// mapping gets revisited deliberately instead of drifting silently.
	for _, v := range []string{"PROPOSED", "ANCHORED", "AUDITED", "COLLAPSED"} {
		turn.Verdict = v
		raw, err := json.Marshal(turn)
		if err != nil {
			t.Fatalf("verdict %q must serialize: %v", v, err)
		}
		var back TribunalTurn
		if err := json.Unmarshal(raw, &back); err != nil {
			t.Fatalf("verdict %q must round-trip: %v", v, err)
		}
		if back.Verdict != v {
			t.Fatalf("verdict %q did not survive the wire, got %q", v, back.Verdict)
		}
	}
}
