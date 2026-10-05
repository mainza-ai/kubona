package autoresearch

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mainza-ai/kubona/core/pkg/memory"
	"github.com/mainza-ai/kubona/core/pkg/sandbox"
)

func TestLoopEngineBaselineAndStep(t *testing.T) {
	reg := NewHarnessRegistry()
	harness, err := reg.Get("ansatz-search")
	if err != nil {
		t.Fatalf("failed to get harness: %v", err)
	}

	cfg := sandbox.DefaultConfig()
	cfg.Timeout = 15 * time.Second
	cfg.AllowHostFallback = true
	runner := sandbox.NewSandboxRunnerWithConfig(cfg)

	initialAnsatz := `from qiskit import QuantumCircuit
from qiskit.circuit import Parameter

def build_circuit():
    qc = QuantumCircuit(2)
    t0 = Parameter('t0')
    t1 = Parameter('t1')
    qc.ry(t0, 0)
    qc.rz(t1, 1)
    qc.cx(0, 1)
    return qc
`

	engine := NewLoopEngine("test-loop-1", "session-test", harness, runner, nil, initialAnsatz, 10)

	ctx := context.Background()
	baseRec, err := engine.InitializeBaseline(ctx)
	if err != nil {
		t.Fatalf("failed to initialize baseline: %v", err)
	}

	if baseRec.Decision != DecisionKeep {
		t.Fatalf("expected baseline to be keep, got: %s", baseRec.Decision)
	}
	if baseRec.Metric <= 0.0 {
		t.Fatalf("expected positive baseline metric, got: %f", baseRec.Metric)
	}

	state := engine.State()
	if state.Status != StatusRunning {
		t.Fatalf("expected status running, got: %s", state.Status)
	}
	if len(state.ParetoFrontier) != 1 {
		t.Fatalf("expected 1 initial pareto point, got: %d", len(state.ParetoFrontier))
	}

	// Run step 1
	step1, err := engine.Step(ctx)
	if err != nil {
		t.Fatalf("failed to run step 1: %v", err)
	}

	if step1.Iteration != 1 {
		t.Fatalf("expected iteration 1, got %d", step1.Iteration)
	}
	if step1.UnifiedDiff == "" {
		t.Fatalf("expected unified diff to be populated")
	}

	stateAfter := engine.State()
	if stateAfter.CurrentIteration != 1 {
		t.Fatalf("expected current iteration 1, got %d", stateAfter.CurrentIteration)
	}
}

func TestLoopEngineStagnationTrigger(t *testing.T) {
	reg := NewHarnessRegistry()
	harness, _ := reg.Get("ansatz-search")

	cfg := sandbox.DefaultConfig()
	cfg.Timeout = 15 * time.Second
	cfg.AllowHostFallback = true
	runner := sandbox.NewSandboxRunnerWithConfig(cfg)

	engine := NewLoopEngine("test-loop-stag", "session-stag", harness, runner, nil, "", 20)

	var stagnationFired atomic.Bool
	engine.SetStagnationCallback(func(loopID string, discards int, champion string) {
		stagnationFired.Store(true)
	})

	ctx := context.Background()
	_, _ = engine.InitializeBaseline(ctx)

	// Artificially set champion metric to a nearly impossible value (0.0000000001)
	// so that subsequent mutations will be discarded
	engine.mu.Lock()
	engine.state.ChampionMetric = 0.0000000001
	engine.mu.Unlock()

	for i := 0; i < 5; i++ {
		_, err := engine.Step(ctx)
		if err != nil {
			t.Fatalf("step %d failed: %v", i+1, err)
		}
	}

	state := engine.State()
	if state.ConsecutiveDiscards < 5 {
		t.Fatalf("expected >= 5 consecutive discards, got %d", state.ConsecutiveDiscards)
	}
	if state.Status != StatusStagnant {
		t.Fatalf("expected status stagnant, got: %s", state.Status)
	}

	// Give goroutine a moment to fire
	time.Sleep(50 * time.Millisecond)
	if !stagnationFired.Load() {
		t.Fatalf("expected stagnation callback to be invoked on 5 discards")
	}

	// Test Resume
	engine.Resume()
	if engine.State().Status != StatusRunning {
		t.Fatalf("expected status running after resume")
	}
}

func TestLoopEngineStagnationAutonomousReflection(t *testing.T) {
	reg := NewHarnessRegistry()
	harness, _ := reg.Get("ansatz-search")

	cfg := sandbox.DefaultConfig()
	cfg.Timeout = 15 * time.Second
	cfg.AllowHostFallback = true
	runner := sandbox.NewSandboxRunnerWithConfig(cfg)

	engine := NewLoopEngine("test-loop-reflect", "session-reflect", harness, runner, nil, "", 20)

	memStore := memory.NewHindsightStore()
	engine.SetMemoryStore(memStore)

	ctx := context.Background()
	_, _ = engine.InitializeBaseline(ctx)

	// Set impossible champion metric to force consecutive discards
	engine.mu.Lock()
	engine.state.ChampionMetric = 0.0000000001
	engine.mu.Unlock()

	for i := 0; i < 5; i++ {
		_, err := engine.Step(ctx)
		if err != nil {
			t.Fatalf("step %d failed: %v", i+1, err)
		}
	}

	state := engine.State()
	if state.Status != StatusStagnant {
		t.Fatalf("expected status stagnant, got: %s", state.Status)
	}

	// Explicitly invoke or await reflection trigger
	reflection, err := engine.TriggerAutonomousReflection(ctx, memStore)
	if err != nil {
		t.Fatalf("failed to trigger autonomous reflection: %v", err)
	}
	if reflection == "" {
		t.Fatalf("expected non-empty reflection diagnosis")
	}

	// Verify that BankMentalModels received the consolidated reflection
	recalled := memStore.Recall(memory.BankMentalModels, "Stagnation Diagnosis", 500)
	if len(recalled) == 0 {
		t.Fatalf("expected memory store to retain reflected mental model, got none")
	}

	if recalled[0].Bank != memory.BankMentalModels {
		t.Fatalf("expected mental model bank, got %s", recalled[0].Bank)
	}

	if recalled[0].EffectiveScore != 1.0 {
		t.Fatalf("expected reflected model to have full score 1.0, got %f", recalled[0].EffectiveScore)
	}
}
