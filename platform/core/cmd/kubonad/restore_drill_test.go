package main

import (
	"path/filepath"
	"testing"

	"github.com/mainza-ai/kubona/core/pkg/ledger"
)

func TestRestoreDrillCreatesVerifiedScratchCopy(t *testing.T) {
	dir := t.TempDir()
	db := filepath.Join(dir, "live.db")
	l, err := ledger.OpenLedger(db)
	if err != nil {
		t.Fatal(err)
	}
	if err := l.AppendEvent("drill-session", "annotation", map[string]any{"ok": true}); err != nil {
		t.Fatal(err)
	}
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	if code := runRestoreDrill([]string{"--db", db, "--scratch", filepath.Join(dir, "scratch")}); code != 0 {
		t.Fatalf("restore-drill exit %d", code)
	}
}
