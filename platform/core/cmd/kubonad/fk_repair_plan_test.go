package main

import (
	"path/filepath"
	"testing"

	"github.com/mainza-ai/kubona/core/pkg/ledger"
)

func TestFKRepairPlanCleanLedger(t *testing.T) {
	path := filepath.Join(t.TempDir(), "clean.db")
	l, err := ledger.OpenLedger(path)
	if err != nil {
		t.Fatalf("open ledger: %v", err)
	}
	if err := l.Close(); err != nil {
		t.Fatalf("close ledger: %v", err)
	}
	if code := runFKRepairPlan([]string{"--db", path}); code != 0 {
		t.Fatalf("clean ledger plan exit %d, want 0", code)
	}
}

func TestFKRepairPlanIsDryRun(t *testing.T) {
	path := filepath.Join(t.TempDir(), "orphan.db")
	l, err := ledger.OpenLedger(path)
	if err != nil {
		t.Fatalf("open ledger: %v", err)
	}
	if _, err := l.DB().Exec(`INSERT INTO chat_messages (id, session_id, role, content, created_at) VALUES ('orphan-msg', 'ghost-session', 'user', 'hello', CURRENT_TIMESTAMP)`); err != nil {
		t.Fatalf("insert orphan message: %v", err)
	}
	if _, err := l.DB().Exec(`INSERT INTO jobs (id, session_id, status, job_type, created_at) VALUES ('orphan-job', 'ghost-session', 'queued', 'test', CURRENT_TIMESTAMP)`); err != nil {
		t.Fatalf("insert orphan job: %v", err)
	}
	if err := l.Close(); err != nil {
		t.Fatalf("close ledger: %v", err)
	}

	if code := runFKRepairPlan([]string{"--db", path}); code != 1 {
		t.Fatalf("orphan ledger plan exit %d, want 1", code)
	}

	// Dry run: every orphan row must still be present.
	l2, err := ledger.OpenLedgerReadOnly(path)
	if err != nil {
		t.Fatalf("reopen read-only: %v", err)
	}
	defer l2.Close()
	var messages, jobs int
	if err := l2.DB().QueryRow(`SELECT COUNT(*) FROM chat_messages`).Scan(&messages); err != nil {
		t.Fatalf("count messages: %v", err)
	}
	if err := l2.DB().QueryRow(`SELECT COUNT(*) FROM jobs`).Scan(&jobs); err != nil {
		t.Fatalf("count jobs: %v", err)
	}
	if messages != 1 || jobs != 1 {
		t.Fatalf("dry run changed rows: messages=%d jobs=%d, want 1/1", messages, jobs)
	}
}
