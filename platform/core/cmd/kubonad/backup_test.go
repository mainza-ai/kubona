package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mainza-ai/kubona/core/pkg/ledger"
)

// A backup is only a backup after it verifies. This test takes a live
// ledger, snapshots it read-only, verifies the copy, then performs a
// restore drill by copying the verified backup to a new path and verifying
// that copy too.
func TestRunBackupCreatesVerifiedRestorableCopy(t *testing.T) {
	dir := t.TempDir()
	db := filepath.Join(dir, "ledger.db")
	l, err := ledger.OpenLedger(db)
	if err != nil {
		t.Fatalf("open ledger: %v", err)
	}
	if err := l.AppendEvent("sess-backup", "annotation", map[string]any{"x": 1}); err != nil {
		t.Fatalf("append event: %v", err)
	}
	if err := l.Close(); err != nil {
		t.Fatalf("close ledger: %v", err)
	}

	out := filepath.Join(dir, "backup.db")
	if code := runBackup([]string{"--db", db, "--out", out}); code != 0 {
		t.Fatalf("backup exit %d, want 0", code)
	}
	if _, err := os.Stat(out); err != nil {
		t.Fatalf("backup file missing: %v", err)
	}

	// Restore drill: copy the verified backup to a fresh path and verify it
	// with the same read-only path an operator would use.
	restored := filepath.Join(dir, "restored.db")
	raw, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read backup: %v", err)
	}
	if err := os.WriteFile(restored, raw, 0o600); err != nil {
		t.Fatalf("write restored copy: %v", err)
	}
	r, err := ledger.OpenLedgerReadOnly(restored)
	if err != nil {
		t.Fatalf("open restored: %v", err)
	}
	faults, err := r.VerifyChain("")
	r.Close()
	if err != nil {
		t.Fatalf("verify restored: %v", err)
	}
	if len(faults) != 0 {
		t.Fatalf("restored ledger has %d chain faults", len(faults))
	}

	// Overwrite refusal: a second backup to the same path must fail.
	if code := runBackup([]string{"--db", db, "--out", out}); code == 0 {
		t.Fatal("backup must refuse to overwrite an existing output")
	}
}
