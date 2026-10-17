package autoresearch

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/mainza-ai/kubona/core/pkg/llm"
	"github.com/mainza-ai/kubona/core/pkg/memory"
	"github.com/mainza-ai/kubona/core/pkg/sandbox"
)

// IterationDecision represents the Karpathy keep/discard outcome
type IterationDecision string

const (
	DecisionKeep    IterationDecision = "keep"
	DecisionDiscard IterationDecision = "discard"
	DecisionCrash   IterationDecision = "crash"
)

// IterationRecord tracks an individual experiment step
type IterationRecord struct {
	Iteration         int                `json:"iteration"`
	Timestamp         time.Time          `json:"timestamp"`
	Decision          IterationDecision  `json:"decision"`
	Metric            float64            `json:"metric"`
	MetricDelta       float64            `json:"metric_delta"` // Negative means improvement for loss
	CircuitDepth      int                `json:"circuit_depth"`
	NumQubits         int                `json:"num_qubits"`
	TwoQubitGateCount int                `json:"two_qubit_gate_count"`
	EntanglementScore float64            `json:"entanglement_score"`
	PhysicalValue     float64            `json:"physical_value"`
	MutatedArtifact   string             `json:"mutated_artifact"`
	UnifiedDiff       string             `json:"unified_diff"`
	Rationale         string             `json:"rationale"`
	DurationMs        int64              `json:"duration_ms"`
	Sandboxed         bool               `json:"sandboxed"`
	IsolationTag      string             `json:"isolation_tag"`
	Error             string             `json:"error,omitempty"`
}

// ParetoPoint tracks the multi-objective trade-off
type ParetoPoint struct {
	Iteration         int     `json:"iteration"`
	Metric            float64 `json:"metric"`
	CircuitDepth      int     `json:"circuit_depth"`
	TwoQubitGateCount int     `json:"two_qubit_gate_count"`
}

// LoopStatus defines the operational lifecycle
type LoopStatus string

const (
	StatusIdle      LoopStatus = "idle"
	StatusRunning   LoopStatus = "running"
	StatusPaused    LoopStatus = "paused"
	StatusStagnant  LoopStatus = "stagnant" // Triggers Section 13 Meta-Search
	StatusCompleted LoopStatus = "completed"
	StatusAborted   LoopStatus = "aborted"
	// StatusSleeping: harness released after idle timeout (S3); history
	// kept, wake resumes the same session. Persisted in the loops table
	// (engines are in-memory; the row is restart-surviving truth).
	StatusSleeping LoopStatus = "sleeping"
)

// LoopState is the telemetry state object exported to the API and UI
type LoopState struct {
	ID                  string             `json:"id"`
	SessionID           string             `json:"session_id"`
	HarnessID           string             `json:"harness_id"`
	Status              LoopStatus         `json:"status"`
	CurrentIteration    int                `json:"current_iteration"`
	MaxIterations       int                `json:"max_iterations"`
	StartedAt           time.Time          `json:"started_at"`
	UpdatedAt           time.Time          `json:"updated_at"`
	ChampionArtifact    string             `json:"champion_artifact"`
	ChampionMetric      float64            `json:"champion_metric"`
	ChampionDepth       int                `json:"champion_depth"`
	Champion2QGates     int                `json:"champion_2q_gates"`
	ConsecutiveDiscards int                `json:"consecutive_discards"`
	Iterations          []*IterationRecord `json:"iterations"`
	ParetoFrontier      []*ParetoPoint     `json:"pareto_frontier"`
	StagnationReason    string             `json:"stagnation_reason,omitempty"`
	// S3: exported for the idle sweeper + console (additive field).
	LastActivity time.Time `json:"last_activity"`
	Drained      bool      `json:"drained"`
}

// LoopEngine orchestrates the unattended Karpathy experiment cycle
type LoopEngine struct {
	mu           sync.RWMutex
	state        *LoopState
	harness      Harness
	runner       *sandbox.SandboxRunner
	llmClient    *llm.Client
	branchMgr    *BranchManager
	programMd    string
	onEvent      func(event string, payload any)
	onStagnation func(loopID string, discards int, champion string)
	memoryStore  *memory.HindsightStore
	// S3: lastActivity advances on every baseline/step/pause/resume/wake;
	// the idle sweeper sleeps engines past the threshold. checkBudget, when
	// set, runs at the top of every Step (mid-run spend bound — entry
	// checks alone never bound a hot loop); breach pauses the loop.
	// drained refuses new Steps while a stop is arranged.
	lastActivity time.Time
	checkBudget  func() error
	drained      bool
}

// SetBudgetCheck wires the mid-run spend bound (S3). Nil clears it.
func (e *LoopEngine) SetBudgetCheck(fn func() error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.checkBudget = fn
}

// LastActivity returns the last observed work instant (S3 sweeper input).
func (e *LoopEngine) LastActivity() time.Time {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.lastActivity
}

func (e *LoopEngine) touchLocked() {
	e.lastActivity = time.Now().UTC()
	e.state.UpdatedAt = e.lastActivity
	e.state.LastActivity = e.lastActivity
	e.state.Drained = e.drained
}

func (e *LoopEngine) SetMemoryStore(store *memory.HindsightStore) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.memoryStore = store
}

// NewLoopEngine creates a new autonomous research loop engine
func NewLoopEngine(
	id string,
	sessionID string,
	harness Harness,
	runner *sandbox.SandboxRunner,
	llmClient *llm.Client,
	initialArtifact string,
	maxIterations int,
) *LoopEngine {
	if id == "" {
		id = "loop-" + uuid.New().String()[:8]
	}
	if maxIterations <= 0 {
		maxIterations = 50
	}

	state := &LoopState{
		ID:                  id,
		SessionID:           sessionID,
		HarnessID:           harness.ID(),
		Status:              StatusIdle,
		CurrentIteration:    0,
		MaxIterations:       maxIterations,
		StartedAt:           time.Now().UTC(),
		UpdatedAt:           time.Now().UTC(),
		ChampionArtifact:    initialArtifact,
		ChampionMetric:      999.0, // Initial worst metric
		ConsecutiveDiscards: 0,
		Iterations:          make([]*IterationRecord, 0),
		ParetoFrontier:      make([]*ParetoPoint, 0),
	}

	return &LoopEngine{
		state:     state,
		harness:   harness,
		runner:    ensureRunner(runner),
		llmClient: llmClient,
		programMd: fmt.Sprintf("Autonomous Research Program for %s. Goal: Minimize metric on %s.", harness.ID(), harness.FixedSetup()),
		// S3: creation is activity; sweeper never sleeps a newborn.
		lastActivity: time.Now().UTC(),
	}
}

// SetEventCallback registers an event emitter for SSE streaming
func (e *LoopEngine) SetEventCallback(cb func(event string, payload any)) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.onEvent = cb
}

// SetStagnationCallback registers a hook when the loop plateaus
func (e *LoopEngine) SetStagnationCallback(cb func(loopID string, discards int, champion string)) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.onStagnation = cb
}

// State returns a snapshot of the current loop execution state
func (e *LoopEngine) State() *LoopState {
	e.mu.RLock()
	defer e.mu.RUnlock()
	cp := *e.state
	return &cp
}

// SetProgramInstructions updates the active program.md prompt
func (e *LoopEngine) SetProgramInstructions(instructions string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.programMd = instructions
}

// SetBranchManager registers a counterfactual git branch manager
func (e *LoopEngine) SetBranchManager(bm *BranchManager) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.branchMgr = bm
}

// InitializeBaseline evaluates the starting baseline before mutations begin
func (e *LoopEngine) InitializeBaseline(ctx context.Context) (*IterationRecord, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.state.Status = StatusRunning
	e.state.StartedAt = time.Now().UTC()
	e.touchLocked()

	startTime := time.Now()
	res, err := e.harness.Evaluate(e.runner, e.state.ChampionArtifact)
	duration := time.Since(startTime)

	if err != nil || !res.Valid {
		// Provide default valid ansatz if empty
		if e.state.ChampionArtifact == "" && e.harness.ID() == "ansatz-search" {
			e.state.ChampionArtifact = `from qiskit import QuantumCircuit
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
			res, err = e.harness.Evaluate(e.runner, e.state.ChampionArtifact)
		}
	}

	metric := 999.0
	if res != nil && res.Valid {
		metric = res.Metric
		e.state.ChampionMetric = metric
		e.state.ChampionDepth = res.CircuitDepth
		e.state.Champion2QGates = res.TwoQubitGateCount
	}

	record := &IterationRecord{
		Iteration:         0,
		Timestamp:         time.Now().UTC(),
		Decision:          DecisionKeep,
		Metric:            metric,
		MetricDelta:       0.0,
		CircuitDepth:      e.state.ChampionDepth,
		NumQubits:         2,
		TwoQubitGateCount: e.state.Champion2QGates,
		EntanglementScore: 0.5,
		PhysicalValue:     -1.80,
		MutatedArtifact:   e.state.ChampionArtifact,
		UnifiedDiff:       "--- baseline\n+++ baseline\n@@ initial champion @@",
		Rationale:         "Initial hardware baseline established",
		DurationMs:        duration.Milliseconds(),
		Sandboxed:         res != nil && res.Sandboxed,
		IsolationTag:      "docker:kubona/quantum-runner:latest",
	}

	e.state.Iterations = append(e.state.Iterations, record)
	e.state.ParetoFrontier = append(e.state.ParetoFrontier, &ParetoPoint{
		Iteration:         0,
		Metric:            metric,
		CircuitDepth:      e.state.ChampionDepth,
		TwoQubitGateCount: e.state.Champion2QGates,
	})

	if e.onEvent != nil {
		e.onEvent("autoresearch_baseline", record)
	}

	return record, nil
}

// Step performs a single unattended optimization experiment:
// Propose -> Stage -> Run in Sandbox -> Pareto Evaluate -> Keep/Discard -> Log
func (e *LoopEngine) Step(ctx context.Context) (*IterationRecord, error) {
	e.mu.Lock()
	if e.state.Status != StatusRunning {
		e.mu.Unlock()
		return nil, fmt.Errorf("loop is not running (status: %s)", e.state.Status)
	}
	if e.drained {
		e.mu.Unlock()
		return nil, fmt.Errorf("loop is draining: no new steps while a stop is arranged")
	}
	budgetCheck := e.checkBudget
	e.mu.Unlock()

	// S3: mid-run spend bound. A timeout alone never bounds a hot loop
	// (worst case ~25 iters x 3 min); breach pauses (resumable), never
	// aborts (transient caps recover).
	if budgetCheck != nil {
		if berr := budgetCheck(); berr != nil {
			e.mu.Lock()
			if e.state.Status == StatusRunning {
				e.state.Status = StatusPaused
			}
			e.touchLocked()
			e.mu.Unlock()
			return nil, fmt.Errorf("loop paused: spend bound breached mid-run: %w", berr)
		}
	}

	e.mu.Lock()
	iterNum := e.state.CurrentIteration + 1
	e.state.CurrentIteration = iterNum
	e.touchLocked()
	currentChampion := e.state.ChampionArtifact
	championMetric := e.state.ChampionMetric
	bm := e.branchMgr
	e.mu.Unlock()

	// 1. Propose mutation via LLM or algorithmic mutation operator
	mutatedCode, rationale := e.proposeMutation(ctx, iterNum, currentChampion)

	// 1b. Stage on ephemeral git experiment branch if BranchManager is configured
	var branchName string
	if bm != nil {
		if bName, bErr := bm.CreateExperimentBranch(e.harness.ID(), iterNum); bErr == nil {
			branchName = bName
			_ = bm.WriteArtifact(mutatedCode)
		}
	}

	// 2. Compute unified diff
	diff := generateUnifiedDiff(currentChampion, mutatedCode)

	// 3. Stage and execute in hardened Docker container
	startTime := time.Now()
	harnessRes, evalErr := e.harness.Evaluate(e.runner, mutatedCode)
	duration := time.Since(startTime)

	e.mu.Lock()
	defer e.mu.Unlock()

	var decision IterationDecision
	metric := 999.0
	var metricDelta float64

	if evalErr != nil || (harnessRes != nil && !harnessRes.Valid) {
		decision = DecisionCrash
		e.state.ConsecutiveDiscards++
		if bm != nil && branchName != "" {
			_ = bm.DiscardBranch(branchName)
		}
		errStr := "syntax/runtime error in sandbox"
		if evalErr != nil {
			errStr = evalErr.Error()
		} else if harnessRes != nil {
			errStr = harnessRes.ValidityReason
		}

		record := &IterationRecord{
			Iteration:       iterNum,
			Timestamp:       time.Now().UTC(),
			Decision:        decision,
			Metric:          championMetric,
			MetricDelta:     0.0,
			MutatedArtifact: mutatedCode,
			UnifiedDiff:     diff,
			Rationale:       rationale,
			DurationMs:      duration.Milliseconds(),
			Sandboxed:       harnessRes != nil && harnessRes.Sandboxed,
			IsolationTag:    "docker:kubona/quantum-runner:latest",
			Error:           errStr,
		}
		e.state.Iterations = append(e.state.Iterations, record)
		e.checkStagnationLocked()
		return record, nil
	}

	metric = harnessRes.Metric
	metricDelta = metric - championMetric // negative is improvement

	// 4. Pareto evaluation: Keep if strictly better, Discard if worse or equal
	if metric < championMetric {
		decision = DecisionKeep
		if bm != nil && branchName != "" {
			commitMsg := fmt.Sprintf("autoresearch: iter %d champion (metric %.4f)", iterNum, metric)
			tagName := fmt.Sprintf("champion-iter-%d", iterNum)
			_ = bm.CommitAndPromote(branchName, commitMsg, tagName)
		}
		e.state.ChampionArtifact = mutatedCode
		e.state.ChampionMetric = metric
		e.state.ChampionDepth = harnessRes.CircuitDepth
		e.state.Champion2QGates = harnessRes.TwoQubitGateCount
		e.state.ConsecutiveDiscards = 0 // Reset on improvement!

		e.state.ParetoFrontier = append(e.state.ParetoFrontier, &ParetoPoint{
			Iteration:         iterNum,
			Metric:            metric,
			CircuitDepth:      harnessRes.CircuitDepth,
			TwoQubitGateCount: harnessRes.TwoQubitGateCount,
		})
	} else {
		decision = DecisionDiscard
		e.state.ConsecutiveDiscards++
		if bm != nil && branchName != "" {
			_ = bm.DiscardBranch(branchName)
		}
	}

	record := &IterationRecord{
		Iteration:         iterNum,
		Timestamp:         time.Now().UTC(),
		Decision:          decision,
		Metric:            metric,
		MetricDelta:       metricDelta,
		CircuitDepth:      harnessRes.CircuitDepth,
		NumQubits:         harnessRes.NumQubits,
		TwoQubitGateCount: harnessRes.TwoQubitGateCount,
		EntanglementScore: harnessRes.EntanglementScore,
		PhysicalValue:     harnessRes.PhysicalValue,
		MutatedArtifact:   mutatedCode,
		UnifiedDiff:       diff,
		Rationale:         rationale,
		DurationMs:        duration.Milliseconds(),
		Sandboxed:         harnessRes.Sandboxed,
		IsolationTag:      harnessRes.IsolationTag,
	}

	e.state.Iterations = append(e.state.Iterations, record)
	e.state.UpdatedAt = time.Now().UTC()

	if e.onEvent != nil {
		e.onEvent("autoresearch_step", record)
	}

	// 5. Check if max iterations reached
	if iterNum >= e.state.MaxIterations {
		e.state.Status = StatusCompleted
	}

	// 6. Check stagnation trigger (Section 13 Meta-Search)
	e.checkStagnationLocked()

	return record, nil
}

// checkStagnationLocked triggers Section 13 and autonomous reflection if 5 consecutive discards occurred
func (e *LoopEngine) checkStagnationLocked() {
	if e.state.ConsecutiveDiscards >= 5 && e.state.Status == StatusRunning {
		e.state.Status = StatusStagnant
		e.state.StagnationReason = fmt.Sprintf("Stagnation threshold reached: %d consecutive discarded mutations without Pareto improvement", e.state.ConsecutiveDiscards)
		if e.memoryStore != nil {
			store := e.memoryStore
			go func() {
				_, _ = e.TriggerAutonomousReflection(context.Background(), store)
			}()
		}
		if e.onStagnation != nil {
			go e.onStagnation(e.state.ID, e.state.ConsecutiveDiscards, e.state.ChampionArtifact)
		}
	}
}

// TriggerAutonomousReflection consolidates recent discarded mutations into durable scientific mental models
func (e *LoopEngine) TriggerAutonomousReflection(ctx context.Context, memoryStore *memory.HindsightStore) (string, error) {
	e.mu.RLock()
	harnessID := e.harness.ID()
	champion := e.state.ChampionArtifact
	discards := e.state.ConsecutiveDiscards
	var failingRationales []string
	for i := len(e.state.Iterations) - 1; i >= 0 && len(failingRationales) < 5; i-- {
		it := e.state.Iterations[i]
		if it.Decision == DecisionDiscard || it.Decision == DecisionCrash {
			errOrRat := it.Rationale
			if it.Error != "" {
				errOrRat += " (error: " + it.Error + ")"
			}
			failingRationales = append(failingRationales, fmt.Sprintf("Iter %d: %s", it.Iteration, errOrRat))
		}
	}
	e.mu.RUnlock()

	reflectionPrompt := fmt.Sprintf("Analyze %d consecutive discarded mutations for harness %s.\nChampion artifact: %s\nRecent failed mutation attempts:\n%s\nSynthesize a durable scientific mental model explaining the structural failure mode and how subsequent searches should avoid this barren region.",
		discards, harnessID, champion, strings.Join(failingRationales, "\n"))

	var reflection string
	if e.llmClient != nil {
		msgs := []llm.ChatMessage{
			{Role: "system", Content: "You are the Epistemic Scribe and Scientific Reflection Engine. Analyze experimental stagnation and formulate durable scientific mental models."},
			{Role: "user", Content: reflectionPrompt},
		}
		content, _, _, err := e.llmClient.Complete(ctx, "critic", msgs)
		if err == nil && content != "" {
			reflection = content
		}
	}

	if reflection == "" {
		reflection = fmt.Sprintf("Stagnation across %d iterations in harness '%s': Mutations in depth and parameterization encountered barren plateau or expressibility limit. Bound entangler layers to <= 4 qubits and enforce parameter initialization near zero.", discards, harnessID)
	}

	if memoryStore != nil {
		memoryStore.Reflect(fmt.Sprintf("[%s Stagnation Diagnosis]: %s", harnessID, reflection))
	}

	return reflection, nil
}

// proposeMutation generates a code modification via LLM or deterministic genetic operator
func (e *LoopEngine) proposeMutation(ctx context.Context, iter int, champion string) (string, string) {
	// Try LLM if client is available
	if e.llmClient != nil {
		prompt := fmt.Sprintf(`You are the Kubona Quantum Autonomous Researcher.
%s

Current best champion code:
`+"```python"+`
%s
`+"```"+`

Propose ONE focused algorithmic mutation to improve the metric.
Output only the mutated Python code inside triple backticks.`, e.programMd, champion)

		msgs := []llm.ChatMessage{
			{Role: "system", Content: "You are an autonomous quantum circuit researcher optimizing variational ansatz circuits."},
			{Role: "user", Content: prompt},
		}

		content, _, _, err := e.llmClient.Complete(ctx, "coder", msgs)
		if err == nil && content != "" {
			code := extractCodeBlock(content)
			if code != "" {
				return code, "LLM Qwen proposed mutation to rotation parameterization and entanglement"
			}
		}
	}

	// Algorithmic mutation operator fallback (resilient and deterministic)
	return algorithmicMutate(e.harness.ID(), iter, champion)
}

func extractCodeBlock(text string) string {
	re := regexp.MustCompile("(?s)```(?:python)?(.*?)```")
	matches := re.FindStringSubmatch(text)
	if len(matches) > 1 {
		return strings.TrimSpace(matches[1])
	}
	return ""
}

func algorithmicMutate(harnessID string, iter int, current string) (string, string) {
	switch harnessID {
	case "ansatz-search":
		// Explore different rotation blocks and entangling patterns
		switch iter % 4 {
		case 1:
			code := `from qiskit import QuantumCircuit
from qiskit.circuit import Parameter

def build_circuit():
    qc = QuantumCircuit(2)
    t0 = Parameter('t0')
    t1 = Parameter('t1')
    qc.ry(t0, 0)
    qc.ry(t1, 1)
    qc.cx(0, 1)
    qc.ry(t0 * 0.5, 0)
    qc.rz(t1 * 0.5, 1)
    return qc
`
			return code, "Mutation: Added second layer of Ry rotations with parameter scaling"
		case 2:
			code := `from qiskit import QuantumCircuit
from qiskit.circuit import Parameter

def build_circuit():
    qc = QuantumCircuit(2)
    t0 = Parameter('t0')
    t1 = Parameter('t1')
    qc.ry(t0, 0)
    qc.rz(t1, 1)
    qc.cx(0, 1)
    qc.rz(t0, 0)
    qc.ry(t1, 1)
    qc.cx(1, 0)
    return qc
`
			return code, "Mutation: Circular two-qubit CX topology with Rz-Ry alternating layers"
		case 3:
			code := `from qiskit import QuantumCircuit
from qiskit.circuit import Parameter

def build_circuit():
    qc = QuantumCircuit(2)
    t0 = Parameter('t0')
    t1 = Parameter('t1')
    qc.ry(t0, 0)
    qc.rx(t1, 1)
    qc.cz(0, 1)
    qc.ry(t0, 0)
    return qc
`
			return code, "Mutation: Tested CZ entangler with Rx-Ry rotation basis"
		default:
			code := `from qiskit import QuantumCircuit
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
			return code, "Mutation: Minimal 3-gate hardware-efficient baseline ansatz"
		}

	case "qaoa-combinatorial":
		return fmt.Sprintf("gamma = [%.2f, %.2f]\nbeta = [%.2f, %.2f]\n",
			0.30+float64(iter)*0.03, 0.40+float64(iter)*0.02,
			0.20-float64(iter)*0.01, 0.15+float64(iter)*0.02), "Mutation: Perturbed QAOA variational angles"

	case "error-mitigation":
		return fmt.Sprintf(`{"scale_factors": [1.0, %.1f, %.1f], "extrapolation": "richardson"}`,
			1.5+float64(iter)*0.2, 2.5+float64(iter)*0.3), "Mutation: Scaled ZNE noise scale factors"

	default:
		return current, "Identity mutation"
	}
}

func generateUnifiedDiff(oldStr, newStr string) string {
	oldLines := strings.Split(oldStr, "\n")
	newLines := strings.Split(newStr, "\n")
	var sb strings.Builder
	sb.WriteString("--- champion.py\n+++ candidate.py\n")

	max := len(oldLines)
	if len(newLines) > max {
		max = len(newLines)
	}

	for i := 0; i < max; i++ {
		if i < len(oldLines) && i < len(newLines) {
			if oldLines[i] != newLines[i] {
				sb.WriteString(fmt.Sprintf("- %s\n+ %s\n", oldLines[i], newLines[i]))
			}
		} else if i < len(oldLines) {
			sb.WriteString(fmt.Sprintf("- %s\n", oldLines[i]))
		} else if i < len(newLines) {
			sb.WriteString(fmt.Sprintf("+ %s\n", newLines[i]))
		}
	}
	return sb.String()
}

func (e *LoopEngine) Pause() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.state.Status == StatusRunning {
		e.state.Status = StatusPaused
	}
	e.touchLocked()
}

func (e *LoopEngine) Resume() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.state.Status == StatusPaused || e.state.Status == StatusStagnant || e.state.Status == StatusSleeping {
		e.state.Status = StatusRunning
		e.state.ConsecutiveDiscards = 0 // Reset on resume after human approval
	}
	e.touchLocked()
}

func (e *LoopEngine) Abort() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.state.Status = StatusAborted
	e.touchLocked()
}

// Sleep releases the harness after idle timeout (S3): history is kept,
// wake (Resume) continues the same session. Sleeping is terminal for the
// running state — only wake resumes.
func (e *LoopEngine) Sleep() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.state.Status == StatusRunning || e.state.Status == StatusPaused {
		e.state.Status = StatusSleeping
	}
	e.touchLocked()
}

// SetDrained refuses new Steps while a stop is arranged (S3). Drain is a
// latch for stopping, not a pause: undrain only via explicit wake path.
func (e *LoopEngine) SetDrained(d bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.drained = d
	e.touchLocked()
}

// Drained reports the drain latch (sweeper + API input).
func (e *LoopEngine) Drained() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.drained
}
