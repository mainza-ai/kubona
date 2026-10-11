package api

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/mainza-ai/kubona/core/pkg/jobs"
	"github.com/mainza-ai/kubona/core/pkg/ledger"
	"github.com/mainza-ai/kubona/core/pkg/session"
	"github.com/mainza-ai/kubona/core/pkg/supervisor"
	"github.com/mainza-ai/kubona/core/pkg/watchdog"
)

// B0-3: fallback scaffolding is attributable and labeled. Nil bridge here,
// so the anchor string must carry the fallback label, not read as computed.
func TestFallbackProposalAttributed(t *testing.T) {
	dbPath := "test_fallback.db"
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

	if got := srv.fallbackProposal("hello there", "general chat"); got != nil {
		t.Error("no molecule detected must yield no proposal, never scaffolding")
	}

	prop := srv.fallbackProposal("run H2 VQE please", "working on it")
	if prop == nil {
		t.Fatal("H2 must yield scaffolding")
	}
	var p map[string]any
	if err := json.Unmarshal([]byte(*prop), &p); err != nil {
		t.Fatal(err)
	}
	if p["designed_by"] != "server-fallback" {
		t.Errorf("scaffolding must be attributed, got %v", p["designed_by"])
	}
	anchor, _ := p["classical_anchor"].(string)
	if !strings.Contains(anchor, "unverified fallback") {
		t.Errorf("bridgeless anchor must read as fallback, got %q", anchor)
	}
	qasm, _ := p["qasm"].(string)
	// The scaffolding must pass the REAL launch gate: submit it.
	sess, err := sm.CreateSession("scaffold gate", 50, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := jr.SubmitJob(sess.ID, "circuit_run", map[string]any{
		"qasm": qasm, "requested_backend": "local_simulator", "shots": 100,
	}); err != nil {
		t.Errorf("scaffolding must pass the launch gate: %v", err)
	}
}
