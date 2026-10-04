package metasearch

import (
	"testing"
)

func TestMetaSearchHumanGatedEvolution(t *testing.T) {
	engine := NewMetaSearchEngine()

	// 1. Create refinement proposal
	proposal := engine.ProposeSearchRefinement(
		"ansatz-search",
		"search.md",
		"Search space: depths 1-3 with linear CNOT topology",
		"+ Add Circular entangler topology when expressibility Q < 0.6",
		"Observed expressibility saturation at depth 3 on 4-qubit cluster",
	)

	if proposal.ApprovedByHuman || proposal.Applied {
		t.Fatalf("proposal must start unapproved and unapplied")
	}

	// 2. Attempting to apply without human approval must fail closed
	_, err := engine.ApplyProposal(proposal.ID)
	if err != ErrHumanApprovalRequired {
		t.Fatalf("expected ErrHumanApprovalRequired, got %v", err)
	}

	// 3. Human reviewer signs off
	err = engine.ApproveProposal(proposal.ID, "principal-lead-scientist")
	if err != nil {
		t.Fatalf("failed to approve proposal: %v", err)
	}

	// 4. Now apply succeeds
	msg, err := engine.ApplyProposal(proposal.ID)
	if err != nil {
		t.Fatalf("failed to apply approved proposal: %v", err)
	}
	if msg == "" {
		t.Fatalf("expected application message")
	}

	if !proposal.Applied {
		t.Fatalf("expected proposal marked applied")
	}
}
