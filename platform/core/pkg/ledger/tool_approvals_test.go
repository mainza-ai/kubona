package ledger

// One-time tool approval store: requests reuse an identical pending row, a
// decision is a compare-and-set, a grant is spent once, and an expired grant is
// reported but never spendable.

import (
	"testing"
	"time"
)

func TestToolApprovalRequestReuseAndCAS(t *testing.T) {
	l := mustOpenObligationLedger(t)
	defer l.Close()
	sid := "sess-tool"

	first, created, err := l.RequestToolApproval(sid, "run_vqe", "hash-1", `{"a":1}`, false)
	if err != nil || !created {
		t.Fatalf("request: %v created=%v", err, created)
	}
	again, created, err := l.RequestToolApproval(sid, "run_vqe", "hash-1", `{"a":1}`, false)
	if err != nil || created {
		t.Fatalf("duplicate request must reuse: %v created=%v", err, created)
	}
	if again.ID != first.ID {
		t.Fatalf("duplicate request made a new row: %s vs %s", again.ID, first.ID)
	}
	other, created, err := l.RequestToolApproval(sid, "run_vqe", "hash-2", `{"a":2}`, false)
	if err != nil || !created || other.ID == first.ID {
		t.Fatalf("different arguments must get their own request: %v created=%v", err, created)
	}

	if _, err := l.DecideToolApproval(sid, first.ID, "approve", "ok", "tester"); err != nil {
		t.Fatal(err)
	}
	if _, err := l.DecideToolApproval(sid, first.ID, "deny", "changed my mind", "tester"); err != ErrToolApprovalNotPending {
		t.Fatalf("second decision must refuse, got %v", err)
	}

	// The approved grant is spent exactly once, for the matching digest only.
	if _, ok, err := l.ConsumeToolApproval(sid, "run_vqe", "hash-2"); err != nil || ok {
		t.Fatalf("unrelated digest spent a grant: ok=%v err=%v", ok, err)
	}
	id, ok, err := l.ConsumeToolApproval(sid, "run_vqe", "hash-1")
	if err != nil || !ok || id != first.ID {
		t.Fatalf("consume: id=%s ok=%v err=%v", id, ok, err)
	}
	if _, ok, _ := l.ConsumeToolApproval(sid, "run_vqe", "hash-1"); ok {
		t.Fatal("a grant was spent twice")
	}
	stored, _ := l.GetToolApproval(sid, first.ID)
	if stored.Status != ToolApprovalUsed || stored.UsedAt == "" {
		t.Fatalf("used grant not recorded: %+v", stored)
	}

	// Session scoping: the same id is not readable from another session.
	if _, err := l.GetToolApproval("other", first.ID); err != ErrToolApprovalNotFound {
		t.Fatalf("cross-session read = %v", err)
	}
}

func TestExpiredToolGrantCannotBeSpent(t *testing.T) {
	l := mustOpenObligationLedger(t)
	defer l.Close()
	sid := "sess-expiry"
	req, _, err := l.RequestToolApproval(sid, "run_vqe", "hash-x", `{}`, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := l.DecideToolApproval(sid, req.ID, "approve", "", "tester"); err != nil {
		t.Fatal(err)
	}
	// Force the grant into the past (a real clock advance is not available in
	// the store, so the row's own timestamp is rewritten).
	past := time.Now().UTC().Add(-time.Minute).Format(time.RFC3339)
	if _, err := l.DB().Exec(`UPDATE tool_approvals SET expires_at = ? WHERE id = ?`, past, req.ID); err != nil {
		t.Fatal(err)
	}
	stored, _ := l.GetToolApproval(sid, req.ID)
	if !stored.Expired {
		t.Fatalf("past grant not reported expired: %+v", stored)
	}
	if _, ok, _ := l.ConsumeToolApproval(sid, "run_vqe", "hash-x"); ok {
		t.Fatal("an expired grant was spendable")
	}
}
