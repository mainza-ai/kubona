package ledger

// C11 store pins: the draw is an atomic CAS; caps, expiry, jobs, status, and
// duplicate rows all fail closed.

import (
	"fmt"
	"testing"
	"time"
)

func mintTestAllowance(t *testing.T, l *Ledger, total, perJob int64, maxJobs int, expires time.Time) *RunAllowance {
	t.Helper()
	a, err := l.MintRunAllowance(RunAllowance{
		ID: fmt.Sprintf("ra-%d", time.Now().UnixNano()), TenantID: "lab-a", SessionID: "sess-a",
		ScopeDigest: "sha256:test", Backends: []string{"ibm_fez"}, JobTypes: []string{"circuit_run"},
		MaxPerJobMicroUSD: perJob, MaxTotalMicroUSD: total, MaxJobs: maxJobs, ShotsCap: 1024,
		ExpiresAt: expires, BasisRateCard: "2026-10-01", BasisCalibrationDigest: "sha256:cal",
		Nonce: fmt.Sprintf("nonce-%d", time.Now().UnixNano()), SignerPubkeyHex: "ab", SignatureHex: "cd",
	})
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func draw(t *testing.T, l *Ledger, id, jobID string, amount int64) (int64, error) {
	t.Helper()
	tx, err := l.DB().Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	remaining, err := l.DrawRunAllowanceTx(tx, id, jobID, "", amount)
	if err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	return remaining, nil
}

func TestRunAllowanceDrawCAS(t *testing.T) {
	l := mustOpenObligationLedger(t)
	a := mintTestAllowance(t, l, 2_000_000, 1_000_000, 2, time.Now().Add(time.Hour))

	remaining, err := draw(t, l, a.ID, "job-1", 1_000_000)
	if err != nil || remaining != 1_000_000 {
		t.Fatalf("first draw: %d %v", remaining, err)
	}
	remaining, err = draw(t, l, a.ID, "job-2", 1_000_000)
	if err != nil || remaining != 0 {
		t.Fatalf("second draw: %d %v", remaining, err)
	}
	got, _ := l.GetRunAllowance(a.ID)
	if got.Status != RunAllowanceExhausted || got.JobsUsed != 2 {
		t.Fatalf("exhaustion state: %+v", got)
	}
	if _, err := draw(t, l, a.ID, "job-3", 1_000_000); err == nil {
		t.Fatal("draw past the cap must refuse")
	}
}

func TestRunAllowanceRefusesExpiredSuspendedAndDuplicates(t *testing.T) {
	l := mustOpenObligationLedger(t)
	a := mintTestAllowance(t, l, 2_000_000, 1_000_000, 3, time.Now().Add(time.Hour))
	if _, err := draw(t, l, a.ID, "job-1", 1_000_000); err != nil {
		t.Fatal(err)
	}
	// Same job id again: the draw row conflict aborts the whole transaction.
	if _, err := draw(t, l, a.ID, "job-1", 1_000_000); err == nil {
		t.Fatal("duplicate job draw must abort")
	}
	// Expiry is checked on every draw.
	if _, err := l.DB().Exec(`UPDATE run_allowances SET expires_at = ? WHERE id = ?`,
		time.Now().Add(-time.Minute).UTC().Format(time.RFC3339), a.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := draw(t, l, a.ID, "job-2", 1000); err == nil {
		t.Fatal("expired allowance must refuse")
	}
	// Suspension stops new draws.
	fresh := mintTestAllowance(t, l, 2_000_000, 1_000_000, 3, time.Now().Add(time.Hour))
	if err := l.SuspendRunAllowance(fresh.ID, "revoked by operator", RunAllowanceRevoked); err != nil {
		t.Fatal(err)
	}
	if _, err := draw(t, l, fresh.ID, "job-3", 1000); err == nil {
		t.Fatal("revoked allowance must refuse")
	}
}

func TestRunAllowanceMintValidationAndListing(t *testing.T) {
	l := mustOpenObligationLedger(t)
	if _, err := l.MintRunAllowance(RunAllowance{ID: "x", TenantID: "t", SessionID: "s", ExpiresAt: time.Now().Add(time.Hour)}); err == nil {
		t.Fatal("incomplete allowance must refuse")
	}
	a := mintTestAllowance(t, l, 1_000_000, 500_000, 1, time.Now().Add(time.Hour))
	rows, err := l.ListRunAllowances("sess-a")
	if err != nil || len(rows) != 1 || rows[0].ID != a.ID {
		t.Fatalf("list: %v %+v", err, rows)
	}
	active, err := l.ListActiveRunAllowances()
	if err != nil || len(active) != 1 {
		t.Fatalf("active list: %v %+v", err, active)
	}
	n, err := l.SuspendActiveRunAllowances("air_gap_engaged")
	if err != nil || n != 1 {
		t.Fatalf("bulk suspend: %d %v", n, err)
	}
	active, _ = l.ListActiveRunAllowances()
	if len(active) != 0 {
		t.Fatalf("no active allowances after suspension: %+v", active)
	}
}
