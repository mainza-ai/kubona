package jobs

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mainza-ai/kubona/core/pkg/ledger"
)

func TestAppendMoneyEventSpoolsWhenLedgerDown(t *testing.T) {
	path := filepath.Join(t.TempDir(), "jobs.db")
	l, err := ledger.OpenLedger(path)
	if err != nil {
		t.Fatalf("open ledger: %v", err)
	}
	r := &Runner{ledger: l}
	if err := l.DB().Close(); err != nil {
		t.Fatalf("close db handle: %v", err)
	}
	r.appendMoneyEvent("sess-job-spool", "job_settled", map[string]any{"job_id": "j1"})
	entries, err := os.ReadDir(filepath.Join(filepath.Dir(path), "provenance-spool"))
	if err != nil {
		t.Fatalf("read spool: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("spooled money events = %d, want 1", len(entries))
	}
	_ = l.Close()
}
