package session_test

import (
	"testing"

	"github.com/mainza-ai/kubona/core/pkg/ledger"
	"github.com/mainza-ai/kubona/core/pkg/session"
)

// W2.2: AddMessage refuses ghost sessions (fail-closed) — the FK is
// declared but unenforced without PRAGMA foreign_keys, so code enforces it.
func TestW22AddMessageRefusesGhost(t *testing.T) {
	l, err := ledger.OpenLedger(t.TempDir() + "/w22.db")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	m := session.NewManager(l)
	if _, err := m.AddMessage("ghost", "user", "hi", nil); err == nil {
		t.Error("ghost AddMessage must refuse")
	}
	s, err := m.CreateSession("w22 drill", 50, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.AddMessage(s.ID, "user", "hi", nil); err != nil {
		t.Errorf("real AddMessage must succeed, got %v", err)
	}
}
