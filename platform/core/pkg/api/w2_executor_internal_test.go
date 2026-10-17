package api

import (
	"context"
	"testing"
	"time"

	"github.com/mainza-ai/kubona/core/pkg/jobs"
	"github.com/mainza-ai/kubona/core/pkg/ledger"
	"github.com/mainza-ai/kubona/core/pkg/session"
	"github.com/mainza-ai/kubona/core/pkg/supervisor"
	"github.com/mainza-ai/kubona/core/pkg/watchdog"
)

// W2.20: the A2A poll enforces a max wait even when the caller passes a
// deadline-less context — no worker, no bridge, no hang.
func TestW220TaskExecutorPollDeadline(t *testing.T) {
	old := a2aMaxWait
	a2aMaxWait = 300 * time.Millisecond
	defer func() { a2aMaxWait = old }()

	l, err := ledger.OpenLedger(t.TempDir() + "/w220exec.db")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	sm := session.NewManager(l)
	jr := jobs.NewRunner(l, nil, 1)
	sup := supervisor.NewSupervisor(l)
	wd := watchdog.NewTaskWatchdog(l, sup)
	srv := NewServer(l, sm, jr, sup, wd)
	_ = srv

	ex := &bridgeTaskExecutor{runner: jr, sessions: sm, ledger: l}
	start := time.Now()
	_, err = ex.ExecuteTask(context.Background(), "molecular_ground_state_pyscf",
		[]byte(`{"molecule":"H2","bond_distance":0.7414,"max_cost_usd":1}`), "default", 1.0)
	took := time.Since(start)
	if err == nil || !containsStr(err.Error(), "poll deadline") {
		t.Errorf("deadline-less poll must fail with poll deadline, got %v", err)
	}
	if took > 10*time.Second {
		t.Errorf("poll must not hang, took %s", took)
	}
}

func containsStr(hay, needle string) bool {
	if len(needle) == 0 {
		return true
	}
	for i := 0; i+len(needle) <= len(hay); i++ {
		if hay[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
