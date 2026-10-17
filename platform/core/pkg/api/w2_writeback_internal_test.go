package api

import (
	"os"
	"testing"

	"github.com/mainza-ai/kubona/core/pkg/jobs"
	"github.com/mainza-ai/kubona/core/pkg/ledger"
	"github.com/mainza-ai/kubona/core/pkg/memory"
	"github.com/mainza-ai/kubona/core/pkg/session"
	"github.com/mainza-ai/kubona/core/pkg/supervisor"
	"github.com/mainza-ai/kubona/core/pkg/watchdog"
)

// W2.20: ghost turns write nothing — the old unknown→default fallback put
// one lab's turn into the default bank (cross-tenant leak). Fail-closed skip.
func TestW220RetainGhostSkips(t *testing.T) {
	dbPath := "test_w220_ghost.db"
	defer os.Remove(dbPath)
	defer os.Remove(dbPath + "-wal")
	defer os.Remove(dbPath + "-shm")
	defer os.Remove(dbPath + ".lock")

	l, err := ledger.OpenLedger(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()

	sm := session.NewManager(l)
	jr := jobs.NewRunner(l, nil, 1)
	sup := supervisor.NewSupervisor(l)
	wd := watchdog.NewTaskWatchdog(l, sup)
	srv := NewServer(l, sm, jr, sup, wd)

	srv.retainTurnExperience("ghost-session", "chat", "H2 anchor?", "answered", []string{"get_anchor"}, 0.8)

	got := srv.memoryStore.SemanticRecallScoped(memory.BankExperiences, "H2 anchor", 1000, 0.0, "default", []string{"default"})
	if len(got) != 0 {
		t.Errorf("ghost turn must write nothing, got %d items", len(got))
	}
}
