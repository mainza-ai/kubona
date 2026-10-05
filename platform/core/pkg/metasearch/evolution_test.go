package metasearch

import (
	"context"
	"testing"
)

func TestMetaSearchEvolutionLifecycle(t *testing.T) {
	engine := NewMetaSearchEngine()

	// 1. Generate stagnation proposal
	ctx := context.Background()
	prop, err := engine.GenerateStagnationProposal(ctx, "loop-101", "ansatz-search", "default instructions", "qc = QuantumCircuit(2)", 5)
	if err != nil {
		t.Fatalf("failed to generate stagnation proposal: %v", err)
	}

	if prop.ID == "" {
		t.Fatalf("expected non-empty proposal ID")
	}
	if prop.Status != ProposalPending {
		t.Fatalf("expected pending status, got: %s", prop.Status)
	}
	if prop.ProposedDiff == "" {
		t.Fatalf("expected proposed diff to be populated")
	}

	// 2. Applying without approval must fail with ErrHumanApprovalRequired
	_, err = engine.ApplyProposal(prop.ID)
	if err != ErrHumanApprovalRequired {
		t.Fatalf("expected ErrHumanApprovalRequired, got: %v", err)
	}

	// 3. Human approval
	err = engine.ApproveProposal(prop.ID, "quantum-engineer-alice")
	if err != nil {
		t.Fatalf("failed to approve proposal: %v", err)
	}

	approvedProp, err := engine.GetProposal(prop.ID)
	if err != nil || approvedProp.Status != ProposalApproved {
		t.Fatalf("expected approved status")
	}
	if approvedProp.ApprovedBy != "quantum-engineer-alice" {
		t.Fatalf("expected approved by alice, got: %s", approvedProp.ApprovedBy)
	}

	// 4. Applying with approval must succeed
	msg, err := engine.ApplyProposal(prop.ID)
	if err != nil {
		t.Fatalf("failed to apply approved proposal: %v", err)
	}
	if msg == "" {
		t.Fatalf("expected confirmation message")
	}

	appliedProp, _ := engine.GetProposal(prop.ID)
	if appliedProp.Status != ProposalApplied {
		t.Fatalf("expected applied status, got: %s", appliedProp.Status)
	}
}

func TestMetaSearchRejection(t *testing.T) {
	engine := NewMetaSearchEngine()
	prop := engine.ProposeSearchRefinement("loop-202", "qaoa-combinatorial", "search.md", "old", "diff", "test rationale")

	err := engine.RejectProposal(prop.ID, "reviewer-bob")
	if err != nil {
		t.Fatalf("failed to reject proposal: %v", err)
	}

	p, _ := engine.GetProposal(prop.ID)
	if p.Status != ProposalRejected {
		t.Fatalf("expected rejected status, got: %s", p.Status)
	}

	_, err = engine.ApplyProposal(prop.ID)
	if err != ErrHumanApprovalRequired {
		t.Fatalf("expected error applying rejected proposal")
	}
}
