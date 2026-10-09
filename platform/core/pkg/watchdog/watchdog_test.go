package watchdog

import (
	"os"
	"testing"

	"github.com/mainza-ai/kubona/core/pkg/ledger"
	"github.com/mainza-ai/kubona/core/pkg/supervisor"
)

func setupWatchdog(t *testing.T) (*TaskWatchdog, *ledger.Ledger, func()) {
	t.Helper()
	dbPath := "test_watchdog.db"
	os.Remove(dbPath)
	l, err := ledger.OpenLedger(dbPath)
	if err != nil {
		t.Fatalf("failed to open ledger: %v", err)
	}
	sup := supervisor.NewSupervisor(l)
	wd := NewTaskWatchdog(l, sup)
	cleanup := func() {
		l.Close()
		os.Remove(dbPath)
		os.Remove(dbPath + "-wal")
		os.Remove(dbPath + "-shm")
	}
	return wd, l, cleanup
}

func TestWatchdogSettledSubtree(t *testing.T) {
	wd, _, cleanup := setupWatchdog(t)
	defer cleanup()

	fp, err := wd.ComputeStopFingerprint("sess-empty")
	if err != nil {
		t.Fatalf("ComputeStopFingerprint failed: %v", err)
	}
	if fp != "" {
		t.Fatalf("empty session must yield empty fingerprint, got %s", fp)
	}

	woke, reason, err := wd.ScanSubtree("sess-empty")
	if err != nil || woke || reason != "all_settled" {
		t.Fatalf("settled subtree must not wake: woke=%v reason=%s err=%v", woke, reason, err)
	}
}

func TestWatchdogWakesOncePerFingerprint(t *testing.T) {
	wd, l, cleanup := setupWatchdog(t)
	defer cleanup()

	// Seed one unsettled job row directly: the watchdog fingerprints any
	// non-completed job for the session.
	if _, err := l.DB().Exec(
		`INSERT INTO jobs (id, session_id, status, job_type, params_json, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		"job-1", "sess-busy", "JOB_STATUS_RUNNING", "classical_anchor", "{}", "2026-10-09T00:00:00Z",
	); err != nil {
		t.Fatalf("seed job failed: %v", err)
	}

	fp, err := wd.ComputeStopFingerprint("sess-busy")
	if err != nil || fp == "" {
		t.Fatalf("busy session must yield non-empty fingerprint: %q err=%v", fp, err)
	}

	woke, got, err := wd.ScanSubtree("sess-busy")
	if err != nil || !woke || got != fp {
		t.Fatalf("first scan must wake with fingerprint: woke=%v got=%s err=%v", woke, got, err)
	}

	// Same fingerprint: already reviewed, no second wake (dedup).
	woke, reason, err := wd.ScanSubtree("sess-busy")
	if err != nil || woke || reason != "already_reviewed" {
		t.Fatalf("second scan must dedup: woke=%v reason=%s err=%v", woke, reason, err)
	}
}
