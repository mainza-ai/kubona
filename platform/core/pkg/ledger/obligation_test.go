package ledger

// R2 store tests: idempotent open, set-once owner, case upsert, closed
// immutability, unknown add/resolve. Closure RULES are tested in
// pkg/agents/obligation_test.go (AttemptClose).

import (
	"testing"
)

func mustOpenObligationLedger(t *testing.T) *Ledger {
	t.Helper()
	l, err := OpenLedger(t.TempDir() + "/obligations.db")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { l.Close() })
	return l
}

func TestOpenObligationIdempotent(t *testing.T) {
	l := mustOpenObligationLedger(t)
	a, err := l.OpenObligation("tribunal-run", "run-1", []string{"turn-1"}, []string{"r1"}, nil)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	b, err := l.OpenObligation("tribunal-run", "run-1", []string{"other"}, nil, nil)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if a.ID != b.ID {
		t.Fatal("reopen must return the same obligation, never fork")
	}
	if _, err := l.OpenObligation("", "", nil, nil, nil); err == nil {
		t.Fatal("empty subject must refuse")
	}
}

func TestSetObligationOwnerOnce(t *testing.T) {
	l := mustOpenObligationLedger(t)
	ob, _ := l.OpenObligation("k", "s", nil, nil, nil)
	if err := l.SetObligationOwner(ob.ID, "session:1"); err != nil {
		t.Fatalf("set owner: %v", err)
	}
	if err := l.SetObligationOwner(ob.ID, "session:1"); err != nil {
		t.Fatalf("same owner re-set must be a no-op receipt: %v", err)
	}
	if err := l.SetObligationOwner(ob.ID, "session:2"); err == nil {
		t.Fatal("duplicate (different) owner must refuse")
	}
	if err := l.SetObligationOwner(ob.ID, ""); err == nil {
		t.Fatal("empty owner must refuse")
	}
}

func TestObligationCaseUpsertAndClosedImmutable(t *testing.T) {
	l := mustOpenObligationLedger(t)
	ob, _ := l.OpenObligation("k", "s", nil, nil, nil)
	rec := ObligationCase{CaseKind: "turn-1", Verdict: "fail", Authority: "r1", Evidence: "e1"}
	if err := l.RecordObligationCase(ob.ID, rec); err != nil {
		t.Fatalf("record: %v", err)
	}
	rec.Verdict = "pass"
	if err := l.RecordObligationCase(ob.ID, rec); err != nil {
		t.Fatalf("re-record: %v", err)
	}
	cases, err := l.ListObligationCases(ob.ID)
	if err != nil || len(cases) != 1 || cases[0].Verdict != "pass" {
		t.Fatalf("latest verdict must win: %+v %v", cases, err)
	}
	if err := l.RecordObligationCase(ob.ID, ObligationCase{CaseKind: "x", Verdict: "bogus", Authority: "r"}); err == nil {
		t.Fatal("bad verdict must refuse")
	}
	if err := l.SetObligationDisposition(ob.ID, "closed"); err != nil {
		t.Fatalf("close: %v", err)
	}
	if err := l.RecordObligationCase(ob.ID, ObligationCase{CaseKind: "turn-2", Verdict: "pass", Authority: "r1"}); err == nil {
		t.Fatal("cases on closed history must refuse (open a new obligation)")
	}
}

func TestObligationUnknowns(t *testing.T) {
	l := mustOpenObligationLedger(t)
	ob, _ := l.OpenObligation("k", "s", nil, nil, nil)
	if err := l.RecordObligationUnknown(ob.ID, Unknown{ID: "u1", Description: "gap"}); err != nil {
		t.Fatalf("record unknown: %v", err)
	}
	if err := l.RecordObligationUnknown(ob.ID, Unknown{ID: "u1", Description: "gap"}); err != nil {
		t.Fatalf("same unknown twice must be a no-op receipt: %v", err)
	}
	if err := l.RecordObligationUnknown(ob.ID, Unknown{Description: "no id"}); err == nil {
		t.Fatal("id-less unknown must refuse")
	}
	got, _ := l.GetObligation(ob.ID)
	if len(got.Unknowns) != 1 {
		t.Fatalf("want 1 unknown, got %d", len(got.Unknowns))
	}
	if err := l.ResolveObligationUnknown(ob.ID, "missing"); err == nil {
		t.Fatal("resolving an absent unknown must refuse")
	}
	if err := l.ResolveObligationUnknown(ob.ID, "u1"); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	got, _ = l.GetObligation(ob.ID)
	if len(got.Unknowns) != 0 {
		t.Fatal("unknown must be gone after resolve")
	}
}
