package main

import (
	"path/filepath"
	"testing"

	"github.com/mainza-ai/kubona/core/pkg/ledger"
)

func TestFKAuditCleanLedger(t *testing.T) {
	path := filepath.Join(t.TempDir(), "clean.db")
	l, err := ledger.OpenLedger(path)
	if err != nil {
		t.Fatalf("open ledger: %v", err)
	}
	if err := l.Close(); err != nil {
		t.Fatalf("close ledger: %v", err)
	}
	if code := runFKAudit([]string{"--db", path}); code != 0 {
		t.Fatalf("clean ledger audit exit %d, want 0", code)
	}
}

func TestFKAuditDetectsOrphan(t *testing.T) {
	path := filepath.Join(t.TempDir(), "orphan.db")
	l, err := ledger.OpenLedger(path)
	if err != nil {
		t.Fatalf("open ledger: %v", err)
	}
	// foreign_keys is off by default, so this insert succeeds and leaves
	// a row whose parent session does not exist. The audit must find it
	// before anyone enables enforcement.
	if _, err := l.DB().Exec(`INSERT INTO jobs (id, session_id, status, job_type, created_at) VALUES ('orphan-job', 'ghost-session', 'queued', 'test', CURRENT_TIMESTAMP)`); err != nil {
		t.Fatalf("insert orphan job: %v", err)
	}
	if err := l.Close(); err != nil {
		t.Fatalf("close ledger: %v", err)
	}
	if code := runFKAudit([]string{"--db", path}); code != 1 {
		t.Fatalf("orphan ledger audit exit %d, want 1", code)
	}
}
