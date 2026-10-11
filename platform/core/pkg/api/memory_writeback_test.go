package api

import (
	"errors"
	"os"
	"testing"

	"github.com/mainza-ai/kubona/core/pkg/jobs"
	"github.com/mainza-ai/kubona/core/pkg/ledger"
	"github.com/mainza-ai/kubona/core/pkg/memory"
	"github.com/mainza-ai/kubona/core/pkg/session"
	"github.com/mainza-ai/kubona/core/pkg/supervisor"
	"github.com/mainza-ai/kubona/core/pkg/watchdog"
)

// The memory loop must close: a retained turn is recallable by its own
// tenant through the same scoping the recall tool enforces, and invisible
// to every other tenant.
func TestRetainTurnExperienceRoundTrip(t *testing.T) {
	dbPath := "test_memory_writeback.db"
	defer os.Remove(dbPath)
	defer os.Remove(dbPath + "-wal")
	defer os.Remove(dbPath + "-shm")

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

	// Tenants are registered rows, not free text — the test registers its
	// own pair the same way production onboarding does.
	for _, tid := range []string{"lab-alpha", "lab-beta"} {
		if _, err := l.DB().Exec(
			`INSERT OR IGNORE INTO tenants (id, display_name, admin_pubkey_hex, autonomy_level, spend_ceiling_usd, status, created_at) VALUES (?, ?, '', 'L1', 150.0, 'active', CURRENT_TIMESTAMP)`,
			tid, tid); err != nil {
			t.Fatal(err)
		}
	}

	sess, err := sm.CreateSession("memory drill", 50, "lab-alpha")
	if err != nil {
		t.Fatal(err)
	}

	srv.retainTurnExperience(sess.ID, "chat", "what is the H2 bond distance?", "answered-with-proposal",
		[]string{"get_anchor", "query_literature"}, 0.8)

	// Same tenant, same principal scoping as the recall tool: found.
	got := srv.memoryStore.SemanticRecallScoped(memory.BankExperiences, "H2 bond distance", 250, 0.0, "lab-alpha", []string{"lab-alpha"})
	if len(got) != 1 {
		t.Fatalf("expected 1 recalled experience, got %d", len(got))
	}
	if got[0].Item.TenantID != "lab-alpha" || got[0].Item.Readership != "lab-alpha" {
		t.Errorf("wrong scoping: %+v", got[0].Item)
	}

	// Foreign tenant: invisible. This is the IFC boundary, not politeness.
	other := srv.memoryStore.SemanticRecallScoped(memory.BankExperiences, "H2 bond distance", 250, 0.0, "lab-beta", []string{"lab-beta"})
	if len(other) != 0 {
		t.Errorf("cross-tenant leak: lab-beta recalled %d items", len(other))
	}
}

// Empty turns write nothing — vacuous rows would pollute every recall.
func TestRetainTurnExperienceSkipsEmpty(t *testing.T) {
	dbPath := "test_memory_writeback_empty.db"
	defer os.Remove(dbPath)
	defer os.Remove(dbPath + "-wal")
	defer os.Remove(dbPath + "-shm")

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

	sess, _ := sm.CreateSession("empty drill", 50, "")
	srv.retainTurnExperience(sess.ID, "chat", "   ", "", nil, 0.5)

	got := srv.memoryStore.SemanticRecallScoped(memory.BankExperiences, "", 1000, 0.0, "default", []string{"default"})
	if len(got) != 0 {
		t.Errorf("empty turn must write nothing, got %d items", len(got))
	}
}

// Tool attribution reads the ledger, newest-first diffed against the turn
// start: earlier turns' tools must never bleed into this turn's experience.
func TestToolNamesSinceAttribution(t *testing.T) {
	dbPath := "test_memory_writeback_tools.db"
	defer os.Remove(dbPath)
	defer os.Remove(dbPath + "-wal")
	defer os.Remove(dbPath + "-shm")

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

	sess, _ := sm.CreateSession("tools drill", 50, "")
	_ = l.AppendEvent(sess.ID, "tool_called", map[string]any{"tool": "get_anchor", "ok": true})

	before := srv.latestEventID(sess.ID)
	if before == 0 {
		t.Fatal("expected nonzero event id after append")
	}
	_ = l.AppendEvent(sess.ID, "tool_called", map[string]any{"tool": "query_literature", "ok": true})
	_ = l.AppendEvent(sess.ID, "note", map[string]any{"x": 1})

	names := srv.toolNamesSince(sess.ID, before)
	if len(names) != 1 || names[0] != "query_literature" {
		t.Errorf("expected only this turn's tool, got %v", names)
	}
	if got := srv.toolNamesSince(sess.ID, before+1000); len(got) != 0 {
		t.Errorf("future watermark must yield nothing, got %v", got)
	}
}

func TestChatOutcomeClassification(t *testing.T) {
	if got := chatOutcome("", false, errors.New("boom")); got == "" || len(got) < 13 || got[:13] != "engine-error:" {
		t.Errorf("empty reply + error must classify engine-error, got %q", got)
	}
	if got := chatOutcome("text", true, nil); got != "answered-with-proposal" {
		t.Errorf("got %q", got)
	}
	if got := chatOutcome("text", false, nil); got != "answered" {
		t.Errorf("got %q", got)
	}
	if got := chatImportance(true, false, true); got != 0.8 {
		t.Errorf("proposal+tools must rate 0.8, got %v", got)
	}
}
