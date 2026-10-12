package agents_test

import (
	"os"
	"testing"

	"github.com/mainza-ai/kubona/core/pkg/agents"
	"github.com/mainza-ai/kubona/core/pkg/ledger"
)

// K3: user agents survive a restart (SQLite), seeds stay code-owned.
func TestCustomAgentRegistryPersistsAcrossInstances(t *testing.T) {
	dbPath := "test_custom_agents_persist.db"
	cleanup := func() {
		os.Remove(dbPath)
		os.Remove(dbPath + ".lock")
		os.Remove(dbPath + "-wal")
		os.Remove(dbPath + "-shm")
	}
	defer cleanup()

	open := func() (*ledger.Ledger, *agents.CustomAgentRegistry) {
		t.Helper()
		l, err := ledger.OpenLedger(dbPath)
		if err != nil {
			t.Fatal(err)
		}
		return l, agents.NewCustomAgentRegistryWithDB(l.DB())
	}

	l1, r1 := open()
	m, err := r1.Register(agents.CustomAgentManifest{Name: "Persistent", Role: "researcher", SpendBudgetUSD: 3})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	l1.Close()

	// Fresh registry, same DB file: user row reloaded, seeds intact.
	l2, r2 := open()
	defer l2.Close()
	defer cleanup()
	found := false
	seeds := 0
	for _, a := range r2.List() {
		if a.ID == m.ID && a.Name == "Persistent" && !a.IsSystemAgent {
			found = true
		}
		if a.IsSystemAgent {
			seeds++
		}
	}
	if !found {
		t.Error("user agent must reload from SQLite after restart")
	}
	if seeds != 2 {
		t.Errorf("both seeds must be present, got %d system agents", seeds)
	}

	// Delete persists too.
	if err := r2.Delete(m.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	l2.Close()
	l3, r3 := open()
	defer l3.Close()
	for _, a := range r3.List() {
		if a.ID == m.ID {
			t.Error("deleted agent must stay deleted after restart")
		}
	}
}
