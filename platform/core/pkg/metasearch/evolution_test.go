package metasearch

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/mainza-ai/kubona/core/pkg/ledger"
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

// K4: proposals (including approvals) survive engine restarts via SQLite,
// and Apply records without writing files (message says so).
func TestMetaSearchPersistsAcrossInstances(t *testing.T) {
	dbPath := "test_meta_persist.db"
	cleanup := func() {
		os.Remove(dbPath)
		os.Remove(dbPath + ".lock")
		os.Remove(dbPath + "-wal")
		os.Remove(dbPath + "-shm")
	}
	defer cleanup()

	openEngine := func() (*MetaSearchEngine, func()) {
		t.Helper()
		l, err := ledger.OpenLedger(dbPath)
		if err != nil {
			t.Fatal(err)
		}
		e := NewMetaSearchEngine()
		e.SetDB(l.DB())
		return e, func() { l.Close() }
	}

	e1, close1 := openEngine()
	p := e1.ProposeSearchRefinement("loop-1", "ansatz-search", "search.md", "cur", "diff", "why")
	if err := e1.ApproveProposal(p.ID, "alice"); err != nil {
		t.Fatal(err)
	}
	close1()

	e2, close2 := openEngine()
	defer close2()
	defer cleanup()
	got, err := e2.GetProposal(p.ID)
	if err != nil {
		t.Fatalf("approved proposal must reload after restart: %v", err)
	}
	if got.Status != ProposalApproved || got.ApprovedBy != "alice" {
		t.Errorf("approval state must survive, got %+v", got)
	}
	msg, err := e2.ApplyProposal(p.ID)
	if err != nil {
		t.Fatalf("apply after reload: %v", err)
	}
	if !strings.Contains(msg, "no files written") {
		t.Errorf("apply message must state the no-write design, got %q", msg)
	}
}

func TestMetaSearchDoubleApplyRefused(t *testing.T) {
	engine := NewMetaSearchEngine()
	ctx := context.Background()
	prop, err := engine.GenerateStagnationProposal(ctx, "loop-202", "ansatz-search", "default instructions", "qc = QuantumCircuit(2)", 5)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if err := engine.ApproveProposal(prop.ID, "quantum-engineer-bob"); err != nil {
		t.Fatalf("approve: %v", err)
	}
	if _, err := engine.ApplyProposal(prop.ID); err != nil {
		t.Fatalf("first apply: %v", err)
	}
	// Second apply must refuse (status is now applied, not approved): the
	// handler returns before loop.Resume(), so no double side-effect.
	if _, err := engine.ApplyProposal(prop.ID); err != ErrHumanApprovalRequired {
		t.Fatalf("double apply must refuse with ErrHumanApprovalRequired, got: %v", err)
	}
	if p, _ := engine.GetProposal(prop.ID); p.Status != ProposalApplied {
		t.Fatalf("status must stay applied, got %s", p.Status)
	}
}
