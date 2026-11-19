package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/mainza-ai/kubona/core/pkg/ledger"
	"github.com/mainza-ai/kubona/core/pkg/memory"
)

func TestMemoryOpsCLI(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mem.db")
	l, err := ledger.OpenLedger(path)
	if err != nil {
		t.Fatal(err)
	}
	store := memory.NewPersistentHindsightStore(l.DB())
	old := store.RetainScoped("default", "default", memory.BankExperiences, "old", 0.5, nil)
	newer := store.RetainScoped("default", "default", memory.BankExperiences, "new", 0.5, nil)
	if err := store.SupersedeScoped("default", old.ID, newer.ID); err != nil {
		t.Fatal(err)
	}
	l.Close()

	if code := runMemoryStatus([]string{"--db", path, "--json"}); code != 0 {
		t.Fatalf("memory-status exit=%d", code)
	}
	out := filepath.Join(t.TempDir(), "graph.json")
	if code := runMemoryGraph([]string{"--db", path, "--tenant", "default", "--out", out}); code != 0 {
		t.Fatalf("memory-graph exit=%d", code)
	}
	raw, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	var graph memory.TemporalGraph
	if err := json.Unmarshal(raw, &graph); err != nil {
		t.Fatal(err)
	}
	if len(graph.Nodes) != 2 || len(graph.Edges) != 1 {
		t.Fatalf("graph=%+v", graph)
	}
	if code := runMemoryFeedback([]string{"--db", path, "--id", newer.ID, "--useful"}); code != 0 {
		t.Fatalf("memory-feedback exit=%d", code)
	}
}
