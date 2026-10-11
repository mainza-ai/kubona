package api

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/mainza-ai/kubona/core/pkg/agents"
	"github.com/mainza-ai/kubona/core/pkg/jobs"
	"github.com/mainza-ai/kubona/core/pkg/ledger"
	"github.com/mainza-ai/kubona/core/pkg/session"
	"github.com/mainza-ai/kubona/core/pkg/supervisor"
	"github.com/mainza-ai/kubona/core/pkg/watchdog"
)

func replanHookTestServer(t *testing.T, dbPath string) (*Server, *ledger.Ledger, func()) {
	t.Helper()
	l, err := ledger.OpenLedger(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	sm := session.NewManager(l)
	jr := jobs.NewRunner(l, nil, 1)
	sup := supervisor.NewSupervisor(l)
	wd := watchdog.NewTaskWatchdog(l, sup)
	srv := NewServer(l, sm, jr, sup, wd)
	cleanup := func() {
		l.Close()
		os.Remove(dbPath)
		os.Remove(dbPath + "-wal")
		os.Remove(dbPath + "-shm")
	}
	return srv, l, cleanup
}

// A settled job with no campaign behind the session replans nothing —
// and, critically, writes no version.
func TestReplanHookSkipsCampaignless(t *testing.T) {
	srv, _, cleanup := replanHookTestServer(t, "test_replan_nocamp.db")
	defer cleanup()

	srv.replanCampaignOnSettled(&jobs.Job{
		ID: "job-nocamp", SessionID: "ghost-session", JobType: "circuit_run",
		ResultJSON: json.RawMessage(`{"success":true,"meyer_wallach_entanglement":0.9}`),
	})

	versions, err := srv.ledger.GetCampaignVersions("ghost-session", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 0 {
		t.Errorf("campaignless session must gain no versions, got %d", len(versions))
	}
}

// A settled measurement folds into the campaign and lands as a labeled
// version — the plan visibly learns the run happened.
func TestReplanHookFoldsMeasurement(t *testing.T) {
	srv, l, cleanup := replanHookTestServer(t, "test_replan_fold.db")
	defer cleanup()

	sess, err := srv.sessionMgr.CreateSession("replan drill", 50, "")
	if err != nil {
		t.Fatal(err)
	}
	state, err := agents.SynthesizeCampaignState(context.Background(), sess.ID, "H2 ground state VQE", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	stateBytes, _ := json.Marshal(state)
	if err := l.SaveCampaignStateWithTrigger(sess.ID, stateBytes, "session-create"); err != nil {
		t.Fatal(err)
	}

	srv.replanCampaignOnSettled(&jobs.Job{
		ID: "job-foldme123", SessionID: sess.ID, JobType: "circuit_run",
		ResultJSON: json.RawMessage(`{"success":true,"resolved_backend":"local_simulator","meyer_wallach_entanglement":0.71,"counts":{"00":100}}`),
	})

	versions, err := l.GetCampaignVersions(sess.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 2 {
		t.Fatalf("expected session-create + replan versions, got %d", len(versions))
	}
	if !strings.HasPrefix(versions[1].Trigger, "replan:circuit_run:") {
		t.Errorf("replan version mislabeled: %q", versions[1].Trigger)
	}
	var folded agents.CampaignState
	if err := json.Unmarshal([]byte(versions[1].StateJSON), &folded); err != nil {
		t.Fatal(err)
	}
	if folded.QuantumFactors.MeyerWallachQ != 0.71 {
		t.Errorf("measured MW not in saved version: %v", folded.QuantumFactors.MeyerWallachQ)
	}

	// A fold with nothing honest to change saves nothing.
	srv.replanCampaignOnSettled(&jobs.Job{
		ID: "job-noop", SessionID: sess.ID, JobType: "circuit_run",
		ResultJSON: json.RawMessage(`{"success":false}`),
	})
	versions, _ = l.GetCampaignVersions(sess.ID, 10)
	if len(versions) != 2 {
		t.Errorf("no-change fold must not write a version, got %d", len(versions))
	}
}
