package main

import (
	"path/filepath"
	"testing"

	"github.com/mainza-ai/kubona/core/pkg/ledger"
)

func TestRunVacuumEnablesIncrementalMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "vac.db")
	l, err := ledger.OpenLedger(path)
	if err != nil {
		t.Fatal(err)
	}
	l.Close()
	if code := runVacuum([]string{"--db", path}); code != 0 {
		t.Fatalf("runVacuum exit=%d", code)
	}
	l2, err := ledger.OpenLedger(path)
	if err != nil {
		t.Fatal(err)
	}
	defer l2.Close()
	var mode int
	if err := l2.DB().QueryRow(`PRAGMA auto_vacuum`).Scan(&mode); err != nil {
		t.Fatal(err)
	}
	if mode != 2 {
		t.Fatalf("auto_vacuum=%d want 2", mode)
	}
}
