package ledger_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mainza-ai/kubona/core/pkg/ledger"
)

func TestAppendEventWithSpoolSealsWhenHealthy(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sealed.db")
	l, err := ledger.OpenLedger(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer l.Close()
	sealed, err := l.AppendEventWithSpool("sess-sealed", "annotation", map[string]any{"x": 1})
	if err != nil {
		t.Fatalf("append: %v", err)
	}
	if !sealed {
		t.Fatal("healthy ledger must report sealed=true")
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(path), "provenance-spool")); !os.IsNotExist(err) {
		t.Fatalf("healthy append must not create a spool: %v", err)
	}
}

func TestAppendEventWithSpoolReplaysAfterReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "spool.db")
	l, err := ledger.OpenLedger(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := l.DB().Close(); err != nil {
		t.Fatalf("close db handle: %v", err)
	}
	sealed, err := l.AppendEventWithSpool("sess-spool", "annotation", map[string]any{"x": 2})
	if err != nil {
		t.Fatalf("spool write: %v", err)
	}
	if sealed {
		t.Fatal("closed ledger must report sealed=false")
	}
	spoolDir := filepath.Join(filepath.Dir(path), "provenance-spool")
	entries, err := os.ReadDir(spoolDir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("spool files = %d, err=%v", len(entries), err)
	}
	_ = l.Close() // releases the lock after the DB handle was closed

	l2, err := ledger.OpenLedger(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer l2.Close()
	entries, err = os.ReadDir(spoolDir)
	if err != nil {
		t.Fatalf("read spool after replay: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("spool not drained after replay: %d files", len(entries))
	}
	var count int
	if err := l2.DB().QueryRow(`SELECT COUNT(*) FROM ledger_events WHERE event_type='annotation' AND session_id='sess-spool'`).Scan(&count); err != nil {
		t.Fatalf("count replayed events: %v", err)
	}
	if count != 1 {
		t.Fatalf("replayed events = %d, want 1", count)
	}
}

func TestAppendEventWithSpoolFailsWhenBothFail(t *testing.T) {
	path := filepath.Join(t.TempDir(), "both.db")
	l, err := ledger.OpenLedger(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer l.Close()
	// A regular file at the spool path makes MkdirAll fail.
	if err := os.WriteFile(filepath.Join(filepath.Dir(path), "provenance-spool"), []byte("not a dir"), 0o600); err != nil {
		t.Fatalf("seed spool blocker: %v", err)
	}
	if err := l.DB().Close(); err != nil {
		t.Fatalf("close db handle: %v", err)
	}
	if _, err := l.AppendEventWithSpool("sess-both", "annotation", map[string]any{"x": 3}); err == nil {
		t.Fatal("both ledger and spool failing must return an error")
	}
}
