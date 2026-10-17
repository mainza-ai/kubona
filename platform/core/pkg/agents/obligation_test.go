package agents_test

// R2 lifecycle tests: AttemptClose refuses every reason named in the
// spec (no owner, missing case, contradiction, missing authority,
// open dependency, residual unknown, empty input) and closes the happy
// path — asserted by QUERYING the tables, per the R2 accept criterion.

import (
	"testing"

	"github.com/mainza-ai/kubona/core/pkg/agents"
	"github.com/mainza-ai/kubona/core/pkg/ledger"
)

func mustOpenCloseLedger(t *testing.T) *ledger.Ledger {
	t.Helper()
	l, err := ledger.OpenLedger(t.TempDir() + "/close.db")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { l.Close() })
	return l
}

func openFull(t *testing.T, l *ledger.Ledger) string {
	t.Helper()
	return openSubject(t, l, "run-x")
}

// openSubject opens a FRESH obligation per subject: OpenObligation is
// idempotent on subject, so sub-cases must not share one (their cases
// would accumulate on the same row and every later assertion would see
// earlier cases — exactly the isolation bug this helper prevents).
func openSubject(t *testing.T, l *ledger.Ledger, subject string) string {
	t.Helper()
	ob, err := l.OpenObligation("tribunal-run", subject,
		[]string{"turn-1", "turn-2"},
		[]string{"ansatz-scout", "disconfirmation-officer"}, nil)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	return ob.ID
}

func record(t *testing.T, l *ledger.Ledger, id, kind, verdict, auth string) {
	t.Helper()
	if err := l.RecordObligationCase(id, ledger.ObligationCase{
		CaseKind: kind, Verdict: verdict, Authority: auth, Evidence: "e",
	}); err != nil {
		t.Fatalf("record %s/%s: %v", kind, verdict, err)
	}
}

func TestAttemptCloseHappyPath(t *testing.T) {
	l := mustOpenCloseLedger(t)
	id := openFull(t, l)
	if err := l.SetObligationOwner(id, "session:s"); err != nil {
		t.Fatalf("owner: %v", err)
	}
	record(t, l, id, "turn-1", "pass", "ansatz-scout")
	record(t, l, id, "turn-2", "pass", "disconfirmation-officer")
	closed, reasons, err := agents.AttemptClose(l, id)
	if err != nil {
		t.Fatalf("close: %v", err)
	}
	if !closed || len(reasons) != 0 {
		t.Fatalf("must close, got %v %v", closed, reasons)
	}
	ob, _ := l.GetObligation(id)
	if ob.Disposition != "closed" {
		t.Fatal("disposition must be closed")
	}
	// Closure sealed its own queryable case.
	cases, _ := l.ListObligationCases(id)
	found := false
	for _, c := range cases {
		if c.CaseKind == "closure" && c.Verdict == "pass" && c.Authority == "obligation-engine" {
			found = true
		}
	}
	if !found {
		t.Fatal("closure case must be on record")
	}
	// Closed stays closed (idempotent receipt).
	closed, _, err = agents.AttemptClose(l, id)
	if err != nil || !closed {
		t.Fatalf("re-close must stay closed: %v %v", closed, err)
	}
}

func TestAttemptCloseRefusals(t *testing.T) {
	l := mustOpenCloseLedger(t)

	// No owner.
	id := openSubject(t, l, "run-case-2")
	record(t, l, id, "turn-1", "pass", "ansatz-scout")
	record(t, l, id, "turn-2", "pass", "disconfirmation-officer")
	closed, reasons, _ := agents.AttemptClose(l, id)
	if closed || len(reasons) == 0 {
		t.Fatal("ownerless close must refuse with reasons")
	}

	// Missing required case.
	id2 := openSubject(t, l, "run-case-3")
	_ = l.SetObligationOwner(id2, "session:s")
	record(t, l, id2, "turn-1", "pass", "ansatz-scout")
	closed, reasons, _ = agents.AttemptClose(l, id2)
	if closed {
		t.Fatal("missing required case must refuse")
	}

	// Contradiction: fail on a required case.
	record(t, l, id2, "turn-2", "fail", "disconfirmation-officer")
	closed, reasons, _ = agents.AttemptClose(l, id2)
	if closed {
		t.Fatal("failed required case must refuse")
	}

	// Contradiction on a NON-required kind still blocks (no outvoting).
	id3 := openSubject(t, l, "run-case-4")
	_ = l.SetObligationOwner(id3, "session:s")
	record(t, l, id3, "turn-1", "pass", "ansatz-scout")
	record(t, l, id3, "turn-2", "pass", "disconfirmation-officer")
	record(t, l, id3, "deliberation", "contested", "collective")
	closed, _, _ = agents.AttemptClose(l, id3)
	if closed {
		t.Fatal("contested side case must refuse")
	}

	// Missing authority: both cases passed by the same role.
	id4 := openSubject(t, l, "run-case-5")
	_ = l.SetObligationOwner(id4, "session:s")
	record(t, l, id4, "turn-1", "pass", "ansatz-scout")
	record(t, l, id4, "turn-2", "pass", "ansatz-scout")
	closed, _, _ = agents.AttemptClose(l, id4)
	if closed {
		t.Fatal("unrepresented required authority must refuse")
	}

	// Residual unknown blocks.
	id5 := openSubject(t, l, "run-case-6")
	_ = l.SetObligationOwner(id5, "session:s")
	record(t, l, id5, "turn-1", "pass", "ansatz-scout")
	record(t, l, id5, "turn-2", "pass", "disconfirmation-officer")
	_ = l.RecordObligationUnknown(id5, ledger.Unknown{ID: "u", Description: "gap"})
	closed, _, _ = agents.AttemptClose(l, id5)
	if closed {
		t.Fatal("residual unknown must refuse")
	}
	_ = l.ResolveObligationUnknown(id5, "u")
	closed, _, err := agents.AttemptClose(l, id5)
	if err != nil || !closed {
		t.Fatalf("resolved unknown must unblock: %v", closed)
	}

	// Open dependency blocks; closed dependency unblocks.
	dep, _ := l.OpenObligation("k", "dep", []string{"c"}, []string{"a"}, nil)
	withDep, _ := l.OpenObligation("k", "withdep", nil, nil, []string{dep.ID})
	_ = l.SetObligationOwner(withDep.ID, "session:s")
	_ = l.RecordObligationCase(withDep.ID, ledger.ObligationCase{CaseKind: "x", Verdict: "pass", Authority: "a"})
	closed, _, _ = agents.AttemptClose(l, withDep.ID)
	if closed {
		t.Fatal("open dependency must refuse")
	}
	_ = l.SetObligationOwner(dep.ID, "session:s")
	_ = l.RecordObligationCase(dep.ID, ledger.ObligationCase{CaseKind: "c", Verdict: "pass", Authority: "a"})
	if closed, _, _ := agents.AttemptClose(l, dep.ID); !closed {
		t.Fatal("dependency must close")
	}
	if closed, _, _ := agents.AttemptClose(l, withDep.ID); !closed {
		t.Fatal("closed dependency must unblock")
	}

	// Empty input never closes.
	empty, _ := l.OpenObligation("k", "empty", nil, nil, nil)
	_ = l.SetObligationOwner(empty.ID, "session:s")
	closed, _, _ = agents.AttemptClose(l, empty.ID)
	if closed {
		t.Fatal("empty input must never close")
	}
}
