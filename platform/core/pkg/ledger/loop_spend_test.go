package ledger

// I3 ledger tests: drawdown accumulates, claims release exactly once,
// loop rows round-trip the spend books.

import (
	"testing"
)

func mustOpenSpendLedger(t *testing.T) *Ledger {
	t.Helper()
	l, err := OpenLedger(t.TempDir() + "/loopspend.db")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { l.Close() })
	return l
}

func seedSpendLoop(t *testing.T, l *Ledger) {
	t.Helper()
	if err := l.SaveLoopState(LoopPersist{
		ID: "loop-spend", SessionID: "s", HarnessID: "ansatz-search",
		Status: "running", StepCount: 1, MaxSteps: 10,
		ReservedAmount: 3.0, ReservedTenant: "lab-a", ConsumedAmount: 0.5,
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
}

func TestLoopSpendRoundTrip(t *testing.T) {
	l := mustOpenSpendLedger(t)
	seedSpendLoop(t, l)
	got, err := l.GetLoopState("loop-spend")
	if err != nil || got == nil {
		t.Fatalf("get: %+v %v", got, err)
	}
	if got.ReservedAmount != 3.0 || got.ReservedTenant != "lab-a" || got.ConsumedAmount != 0.5 {
		t.Fatalf("spend books must round-trip: %+v", got)
	}
	if _, err := l.GetLoopState("missing"); err != nil {
		t.Fatalf("missing loop must read nil-nil, got %v", err)
	}
}

func TestLoopDrawdownAccumulates(t *testing.T) {
	l := mustOpenSpendLedger(t)
	seedSpendLoop(t, l)
	if err := l.AddLoopConsumed("loop-spend", 0.25); err != nil {
		t.Fatalf("drawdown: %v", err)
	}
	if err := l.AddLoopConsumed("loop-spend", 0); err != nil {
		t.Fatalf("zero drawdown must be a no-op receipt: %v", err)
	}
	if err := l.AddLoopConsumed("loop-spend", -1); err == nil {
		t.Fatal("negative drawdown must refuse")
	}
	got, _ := l.GetLoopState("loop-spend")
	if got.ConsumedAmount != 0.75 {
		t.Fatalf("consumed must accumulate 0.5 + 0.25, got %v", got.ConsumedAmount)
	}
}

func TestClaimLoopSpendOnce(t *testing.T) {
	l := mustOpenSpendLedger(t)
	seedSpendLoop(t, l)
	tenant, refund, ok, err := l.ClaimLoopSpend("loop-spend")
	if err != nil || !ok || tenant != "lab-a" || refund != 2.5 {
		t.Fatalf("claim must return tenant + reserved-consumed: %q %v %v %v", tenant, refund, ok, err)
	}
	// Second claim: nothing left (kill racing completion releases once).
	_, _, ok, err = l.ClaimLoopSpend("loop-spend")
	if err != nil || ok {
		t.Fatalf("second claim must be a no-op receipt: %v %v", ok, err)
	}
	got, _ := l.GetLoopState("loop-spend")
	if got.ReservedAmount != 0 || got.ConsumedAmount != 0 {
		t.Fatalf("claimed row must read zero: %+v", got)
	}
	// Missing loop: no-op, never error.
	_, _, ok, err = l.ClaimLoopSpend("missing")
	if err != nil || ok {
		t.Fatalf("missing loop claim must be a no-op: %v %v", ok, err)
	}
}
