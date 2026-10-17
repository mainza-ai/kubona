package autoresearch

// S3 — loop sleep/wake/drain + mid-run budget. Threat: idle loops hold
// harness/bridge resources forever; hot loops burn unbounded LLM.

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/mainza-ai/kubona/core/pkg/sandbox"
)

func newSleepTestEngine(t *testing.T) *LoopEngine {
	t.Helper()
	h := &testHarness{id: "sleep-test", scores: []float64{100, 90, 80}}
	cfg := sandbox.DefaultConfig()
	cfg.Timeout = 5 * time.Second
	runner := sandbox.NewSandboxRunnerWithConfig(cfg)
	return NewLoopEngine("loop-sleep-1", "sess-sleep", h, runner, nil, "artifact-v0", 10)
}

func TestLoopSleepWakeCycle(t *testing.T) {
	e := newSleepTestEngine(t)
	ctx := context.Background()
	if _, err := e.InitializeBaseline(ctx); err != nil {
		t.Fatalf("baseline: %v", err)
	}
	before := e.LastActivity()
	if before.IsZero() {
		t.Fatalf("baseline must touch activity")
	}
	e.Sleep()
	if got := e.State().Status; got != StatusSleeping {
		t.Fatalf("sleep must latch sleeping, got %s", got)
	}
	if _, err := e.Step(ctx); err == nil {
		t.Fatalf("step on a sleeping loop must refuse")
	}
	e.Resume()
	if got := e.State().Status; got != StatusRunning {
		t.Fatalf("wake must resume running, got %s", got)
	}
	if _, err := e.Step(ctx); err != nil {
		t.Fatalf("step after wake must proceed: %v", err)
	}
	if !e.LastActivity().After(before) {
		t.Fatalf("work must advance activity")
	}
	// Terminal states never sleep.
	e.Abort()
	e.Sleep()
	if got := e.State().Status; got != StatusAborted {
		t.Fatalf("aborted loop must stay aborted, got %s", got)
	}
}

func TestLoopDrainRefusesSteps(t *testing.T) {
	e := newSleepTestEngine(t)
	ctx := context.Background()
	if _, err := e.InitializeBaseline(ctx); err != nil {
		t.Fatalf("baseline: %v", err)
	}
	e.SetDrained(true)
	if !e.Drained() {
		t.Fatalf("drain latch must read back")
	}
	if _, err := e.Step(ctx); err == nil {
		t.Fatalf("step on a drained loop must refuse")
	}
	if !e.State().Drained {
		t.Fatalf("drain must export in state")
	}
	e.SetDrained(false)
	if _, err := e.Step(ctx); err != nil {
		t.Fatalf("step after undrain must proceed: %v", err)
	}
}

func TestLoopMidRunBudgetBreachPauses(t *testing.T) {
	e := newSleepTestEngine(t)
	ctx := context.Background()
	if _, err := e.InitializeBaseline(ctx); err != nil {
		t.Fatalf("baseline: %v", err)
	}
	calls := 0
	e.SetBudgetCheck(func() error {
		calls++
		if calls > 1 {
			return errors.New("global LLM budget exhausted")
		}
		return nil
	})
	if _, err := e.Step(ctx); err != nil {
		t.Fatalf("first step within budget must proceed: %v", err)
	}
	if _, err := e.Step(ctx); err == nil {
		t.Fatalf("breaching step must refuse")
	}
	if got := e.State().Status; got != StatusPaused {
		t.Fatalf("breach must pause (resumable), not abort, got %s", got)
	}
	// Drained + budget-breach interact: drain wins with its own message.
	e.Resume()
	e.SetDrained(true)
	if _, err := e.Step(ctx); err == nil || !strings.Contains(err.Error(), "draining") {
		t.Fatalf("drained step must refuse with drain reason, got %v", err)
	}
}
