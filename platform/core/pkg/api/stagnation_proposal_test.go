package api

// T-12 pin: the stagnation callback mints a proposal AND surfaces its id
// on the ledger event (the operator-visible half). Pre-fix the event
// carried only loop_id/discards/harness_id — the proposal row existed
// but was unreachable.

import (
	"encoding/json"
	"testing"

	"github.com/mainza-ai/kubona/core/pkg/jobs"
	"github.com/mainza-ai/kubona/core/pkg/ledger"
	"github.com/mainza-ai/kubona/core/pkg/session"
	"github.com/mainza-ai/kubona/core/pkg/supervisor"
	"github.com/mainza-ai/kubona/core/pkg/watchdog"
)

func TestStagnationProposalSurfacesID(t *testing.T) {
	l, err := ledger.OpenLedger(t.TempDir() + "/stag.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	sm := session.NewManager(l)
	jr := jobs.NewRunner(l, nil, 1)
	sup := supervisor.NewSupervisor(l)
	wd := watchdog.NewTaskWatchdog(l, sup)
	srv := NewServer(l, sm, jr, sup, wd)

	srv.onLoopStagnation("sess-stag", "ansatz-search", "loop-9", 5, "champion-code")

	props := srv.metaSearchEngine.ListProposals()
	if len(props) != 1 {
		t.Fatalf("expected 1 minted proposal, got %d", len(props))
	}
	if props[0].Status != "pending_approval" {
		t.Errorf("minted proposal must be pending, got %q", props[0].Status)
	}
	events, err := l.GetRecentEvents("sess-stag", 10)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, e := range events {
		if e.EventType != "autoresearch_stagnation_triggered" {
			continue
		}
		var payload map[string]any
		if err := json.Unmarshal(e.Payload, &payload); err != nil {
			t.Fatal(err)
		}
		if payload["proposal_id"] == props[0].ID && payload["proposal_status"] == "pending_approval" {
			found = true
		}
	}
	if !found {
		t.Errorf("stagnation event must carry the minted proposal id; events: %+v", events)
	}
}
