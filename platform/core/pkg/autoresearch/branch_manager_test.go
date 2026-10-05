package autoresearch

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mainza-ai/kubona/core/pkg/sandbox"
)

func setupTestGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %s failed: %v, output: %s", strings.Join(args, " "), err, string(out))
		}
	}

	run("init", "-b", "main")
	run("config", "user.name", "Kubona Bot")
	run("config", "user.email", "bot@kubona.local")

	initialFile := filepath.Join(dir, "artifact.py")
	if err := os.WriteFile(initialFile, []byte("# initial artifact\n"), 0644); err != nil {
		t.Fatalf("failed to write initial artifact: %v", err)
	}

	run("add", "artifact.py")
	run("commit", "-m", "initial baseline")

	return dir
}

func TestBranchManager_Lifecycle(t *testing.T) {
	repoDir := setupTestGitRepo(t)

	bm, err := NewBranchManager(repoDir, "main", "artifact.py")
	if err != nil {
		t.Fatalf("NewBranchManager failed: %v", err)
	}

	cur, err := bm.CurrentBranch()
	if err != nil || cur != "main" {
		t.Fatalf("expected current branch 'main', got %s (err: %v)", cur, err)
	}

	// 1. Create experiment branch & discard
	b1, err := bm.CreateExperimentBranch("ansatz-search", 1)
	if err != nil {
		t.Fatalf("CreateExperimentBranch failed: %v", err)
	}
	if b1 != "experiment/ansatz-search/iter-1" {
		t.Fatalf("unexpected branch name: %s", b1)
	}

	if err := bm.WriteArtifact("# discarded mutation\n"); err != nil {
		t.Fatalf("WriteArtifact failed: %v", err)
	}

	// Discard
	if err := bm.DiscardBranch(b1); err != nil {
		t.Fatalf("DiscardBranch failed: %v", err)
	}

	cur, err = bm.CurrentBranch()
	if err != nil || cur != "main" {
		t.Fatalf("expected to be back on 'main', got %s", cur)
	}

	content, err := os.ReadFile(filepath.Join(repoDir, "artifact.py"))
	if err != nil {
		t.Fatalf("failed to read artifact: %v", err)
	}
	if string(content) != "# initial artifact\n" {
		t.Fatalf("expected artifact to be reverted, got: %s", string(content))
	}

	// 2. Create experiment branch & promote
	b2, err := bm.CreateExperimentBranch("ansatz-search", 2)
	if err != nil {
		t.Fatalf("CreateExperimentBranch failed: %v", err)
	}

	winningCode := "# champion mutation 2\n"
	if err := bm.WriteArtifact(winningCode); err != nil {
		t.Fatalf("WriteArtifact failed: %v", err)
	}

	if err := bm.CommitAndPromote(b2, "iter 2 champion", "champion-iter-2"); err != nil {
		t.Fatalf("CommitAndPromote failed: %v", err)
	}

	cur, err = bm.CurrentBranch()
	if err != nil || cur != "main" {
		t.Fatalf("expected to be on 'main', got %s", cur)
	}

	content, err = os.ReadFile(filepath.Join(repoDir, "artifact.py"))
	if err != nil {
		t.Fatalf("failed to read promoted artifact: %v", err)
	}
	if string(content) != winningCode {
		t.Fatalf("expected artifact to contain winning code, got: %s", string(content))
	}
}

type testHarness struct {
	id     string
	scores []float64
	idx    int
}

func (h *testHarness) ID() string               { return h.id }
func (h *testHarness) FixedSetup() string       { return "test" }
func (h *testHarness) MutableArtifact() string  { return "circuit" }
func (h *testHarness) Metric(artifact string) (float64, error) {
	return 0, nil
}
func (h *testHarness) ValidityCheck(artifact string) (bool, string) {
	return true, ""
}
func (h *testHarness) Evaluate(runner *sandbox.SandboxRunner, artifact string) (*HarnessResult, error) {
	score := 500.0
	if h.idx < len(h.scores) {
		score = h.scores[h.idx]
		h.idx++
	}
	return &HarnessResult{
		Metric:            score,
		Valid:             true,
		CircuitDepth:      4,
		TwoQubitGateCount: 2,
		NumQubits:         2,
		Sandboxed:         false,
	}, nil
}

func TestBranchManager_LoopEngineIntegration(t *testing.T) {
	repoDir := setupTestGitRepo(t)
	bm, err := NewBranchManager(repoDir, "main", "artifact.py")
	if err != nil {
		t.Fatalf("NewBranchManager failed: %v", err)
	}

	harness := &testHarness{
		id:     "opt-loop",
		scores: []float64{10.0, 50.0, 5.0}, // baseline (10.0), discard (50.0), keep (5.0)
	}

	runner := sandbox.NewSandboxRunnerWithConfig(sandbox.DefaultConfig())
	engine := NewLoopEngine("test-loop", "sess-1", harness, runner, nil, "# initial\n", 10)
	engine.SetBranchManager(bm)

	ctx := context.Background()
	_, err = engine.InitializeBaseline(ctx)
	if err != nil {
		t.Fatalf("InitializeBaseline failed: %v", err)
	}

	// Step 1: score = 50.0 (> 10.0, so discard)
	rec1, err := engine.Step(ctx)
	if err != nil {
		t.Fatalf("Step 1 failed: %v", err)
	}
	if rec1.Decision != DecisionDiscard {
		t.Fatalf("expected Step 1 to discard, got %s", rec1.Decision)
	}
	cur, _ := bm.CurrentBranch()
	if cur != "main" {
		t.Fatalf("expected main branch after discard, got %s", cur)
	}

	// Step 2: score = 5.0 (< 10.0, so keep)
	rec2, err := engine.Step(ctx)
	if err != nil {
		t.Fatalf("Step 2 failed: %v", err)
	}
	if rec2.Decision != DecisionKeep {
		t.Fatalf("expected Step 2 to keep, got %s", rec2.Decision)
	}
	cur, _ = bm.CurrentBranch()
	if cur != "main" {
		t.Fatalf("expected main branch after keep, got %s", cur)
	}
}
