package ledger_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/mainza-ai/kubona/core/pkg/ledger"
)

func TestEnforcedOpenCleanLedger(t *testing.T) {
	path := filepath.Join(t.TempDir(), "clean.db")
	l, err := ledger.OpenLedgerWithOptions(path, ledger.Options{EnforceForeignKeys: true})
	if err != nil {
		t.Fatalf("enforced open clean ledger: %v", err)
	}
	defer l.Close()
	var fk int
	if err := l.DB().QueryRow(`PRAGMA foreign_keys`).Scan(&fk); err != nil {
		t.Fatalf("read foreign_keys: %v", err)
	}
	if fk != 1 {
		t.Fatalf("foreign_keys = %d, want 1", fk)
	}
}

func TestEnforcedOpenRefusesOrphans(t *testing.T) {
	path := filepath.Join(t.TempDir(), "orphan.db")
	l, err := ledger.OpenLedger(path)
	if err != nil {
		t.Fatalf("open dev ledger: %v", err)
	}
	if _, err := l.DB().Exec(`INSERT INTO jobs (id, session_id, status, job_type, created_at) VALUES ('orphan-job', 'ghost-session', 'queued', 'test', CURRENT_TIMESTAMP)`); err != nil {
		t.Fatalf("insert orphan: %v", err)
	}
	if err := l.Close(); err != nil {
		t.Fatalf("close dev ledger: %v", err)
	}

	if _, err := ledger.OpenLedgerWithOptions(path, ledger.Options{EnforceForeignKeys: true}); err == nil {
		t.Fatal("enforced open with an orphan must refuse")
	} else if !strings.Contains(err.Error(), "foreign-key orphan") || !strings.Contains(err.Error(), "fk-quarantine") {
		t.Fatalf("refusal must name the repair, got %v", err)
	}
}

func TestDevOpenAllowsOrphans(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dev-orphan.db")
	l, err := ledger.OpenLedger(path)
	if err != nil {
		t.Fatalf("open dev ledger: %v", err)
	}
	if _, err := l.DB().Exec(`INSERT INTO jobs (id, session_id, status, job_type, created_at) VALUES ('orphan-job', 'ghost-session', 'queued', 'test', CURRENT_TIMESTAMP)`); err != nil {
		t.Fatalf("insert orphan: %v", err)
	}
	l.Close()
	l2, err := ledger.OpenLedger(path)
	if err != nil {
		t.Fatalf("dev open must allow the orphan for repair tools: %v", err)
	}
	l2.Close()
}
