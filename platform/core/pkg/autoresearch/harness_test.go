package autoresearch

import (
	"strings"
	"testing"
	"time"

	"github.com/mainza-ai/kubona/core/pkg/sandbox"
)

func TestHarnessRegistryAllFive(t *testing.T) {
	reg := NewHarnessRegistry()

	harnesses := reg.List()
	if len(harnesses) != 5 {
		t.Fatalf("expected 5 shipping harnesses, got %d", len(harnesses))
	}

	expectedIDs := []string{
		"ansatz-search",
		"qaoa-combinatorial",
		"error-mitigation",
		"compilation-pass",
		"correctness-regression",
	}

	for _, id := range expectedIDs {
		h, err := reg.Get(id)
		if err != nil {
			t.Fatalf("missing expected harness '%s': %v", id, err)
		}
		if h.ID() != id {
			t.Fatalf("expected ID %s, got %s", id, h.ID())
		}
		if h.FixedSetup() == "" {
			t.Fatalf("harness %s must have non-empty FixedSetup", id)
		}
	}
}

func TestAnsatzSearchHarnessValidityAndEvaluation(t *testing.T) {
	reg := NewHarnessRegistry()
	h, err := reg.Get("ansatz-search")
	if err != nil {
		t.Fatalf("failed to get ansatz-search harness: %v", err)
	}

	// 1. Invalid artifact check
	valid, reason := h.ValidityCheck("print('hello classical world')")
	if valid {
		t.Fatalf("expected invalid artifact")
	}
	if reason == "" {
		t.Fatalf("expected explanation for invalidity")
	}

	// 2. Valid artifact check
	validAnsatzCode := `
from qiskit import QuantumCircuit
from qiskit.circuit import Parameter

def build_circuit():
    qc = QuantumCircuit(2)
    t0 = Parameter('t0')
    t1 = Parameter('t1')
    qc.ry(t0, 0)
    qc.rz(t1, 1)
    qc.cx(0, 1)
    qc.ry(t0, 0)
    return qc
`
	valid, _ = h.ValidityCheck(validAnsatzCode)
	if !valid {
		t.Fatalf("expected valid quantum circuit ansatz")
	}

	cfg := sandbox.DefaultConfig()
	cfg.Timeout = 15 * time.Second
	cfg.AllowHostFallback = true
	runner := sandbox.NewSandboxRunnerWithConfig(cfg)

	res, err := h.Evaluate(runner, validAnsatzCode)
	if err != nil {
		t.Fatalf("failed to evaluate ansatz harness: %v", err)
	}

	if !res.Valid {
		t.Fatalf("expected valid evaluation result, got reason: %s (stderr: %s)", res.ValidityReason, res.RawStderr)
	}

	if res.Metric <= 0.0 {
		t.Fatalf("expected positive energy error metric, got %f", res.Metric)
	}

	if res.NumQubits != 2 {
		t.Fatalf("expected 2 qubits, got %d", res.NumQubits)
	}
}

func TestQAOACombinatorialHarnessEvaluation(t *testing.T) {
	reg := NewHarnessRegistry()
	qaoa, _ := reg.Get("qaoa-combinatorial")

	validSchedule := `
gamma = [0.35, 0.42]
beta = [0.20, 0.15]
`
	valid, _ := qaoa.ValidityCheck(validSchedule)
	if !valid {
		t.Fatalf("expected valid QAOA schedule")
	}

	cfg := sandbox.DefaultConfig()
	cfg.Timeout = 15 * time.Second
	cfg.AllowHostFallback = true
	runner := sandbox.NewSandboxRunnerWithConfig(cfg)

	res, err := qaoa.Evaluate(runner, validSchedule)
	if err != nil {
		t.Fatalf("failed to evaluate qaoa harness: %v", err)
	}

	if !res.Valid {
		t.Fatalf("expected valid QAOA evaluation, reason: %s", res.ValidityReason)
	}

	if res.PhysicalValue <= 0.0 || res.PhysicalValue > 1.0 {
		t.Fatalf("expected approximation ratio in (0, 1], got %f", res.PhysicalValue)
	}
}

func TestErrorMitigationHarnessEvaluation(t *testing.T) {
	reg := NewHarnessRegistry()
	mit, _ := reg.Get("error-mitigation")

	validConfig := `{"scale_factors": [1.0, 2.0, 3.0], "extrapolation": "richardson"}`
	valid, _ := mit.ValidityCheck(validConfig)
	if !valid {
		t.Fatalf("expected valid mitigation config")
	}

	cfg := sandbox.DefaultConfig()
	cfg.Timeout = 15 * time.Second
	cfg.AllowHostFallback = true
	runner := sandbox.NewSandboxRunnerWithConfig(cfg)

	res, err := mit.Evaluate(runner, validConfig)
	if err != nil {
		t.Fatalf("failed to evaluate mitigation harness: %v", err)
	}

	if !res.Valid {
		t.Fatalf("expected valid mitigation evaluation, reason: %s", res.ValidityReason)
	}

	if res.Metric < 0.0 {
		t.Fatalf("expected non-negative residual error, got %f", res.Metric)
	}
}

func TestCorrectnessRegressionHarnessEvaluation(t *testing.T) {
	reg := NewHarnessRegistry()
	regHarness, _ := reg.Get("correctness-regression")

	cfg := sandbox.DefaultConfig()
	cfg.Timeout = 15 * time.Second
	cfg.AllowHostFallback = true
	runner := sandbox.NewSandboxRunnerWithConfig(cfg)

	res, err := regHarness.Evaluate(runner, "")
	if err != nil {
		t.Fatalf("failed to evaluate correctness regression: %v", err)
	}

	if !res.Valid {
		t.Fatalf("expected valid regression result, reason: %s", res.ValidityReason)
	}

	if res.PhysicalValue < 0.99 {
		t.Fatalf("expected fidelity ~1.0 on Bell state, got %f", res.PhysicalValue)
	}
}

// R5: the nil-runner default must not smuggle host fallback in. Dockerless
// execution stays refused unless the operator opts in with
// KUBONA_ALLOW_HOST_FALLBACK=1 — the same gate the daemon uses.
func TestEnsureRunnerDefaultsFailClosed(t *testing.T) {
	t.Setenv("KUBONA_ALLOW_HOST_FALLBACK", "")
	if got := ensureRunner(nil).Config().AllowHostFallback; got {
		t.Error("nil runner must default to AllowHostFallback=false")
	}
	t.Setenv("KUBONA_ALLOW_HOST_FALLBACK", "1")
	if got := ensureRunner(nil).Config().AllowHostFallback; !got {
		t.Error("KUBONA_ALLOW_HOST_FALLBACK=1 must opt in to host fallback")
	}
	// An explicit runner is never rewritten.
	cfg := sandbox.DefaultConfig()
	cfg.AllowHostFallback = true
	if got := ensureRunner(sandbox.NewSandboxRunnerWithConfig(cfg)); got.Config().AllowHostFallback != true {
		t.Error("explicit runner must pass through untouched")
	}
}

func TestParseHarnessOutputBoundsStreams(t *testing.T) {
	big := strings.Repeat("E", 100000)
	res := &sandbox.ExecutionResult{
		Stdout:   "{\"metric\": 1.5}\n" + strings.Repeat("o", 50000),
		Stderr:   big,
		ExitCode: 0, Sandboxed: true,
	}
	hr, err := parseHarnessOutput("ansatz-search", res, "test-nonce")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(hr.RawStderr) != sandbox.MaxStoredOutputBytes {
		t.Fatalf("stderr must cap at %d, got %d", sandbox.MaxStoredOutputBytes, len(hr.RawStderr))
	}
	if !hr.StderrTruncated || hr.StderrBytes != len(big) {
		t.Fatalf("truncation must be flagged with original size, got %+v", hr)
	}
	if !hr.StdoutTruncated || hr.StdoutBytes <= sandbox.MaxStoredOutputBytes {
		t.Fatalf("stdout must flag too, got %+v", hr)
	}
}

func TestParseHarnessOutputReasonBounded(t *testing.T) {
	// Unparseable output previously embedded FULL streams in the reason.
	res := &sandbox.ExecutionResult{
		Stdout:   strings.Repeat("x", 50000),
		Stderr:   strings.Repeat("y", 50000),
		ExitCode: 1, Sandboxed: true,
	}
	hr, err := parseHarnessOutput("ansatz-search", res, "test-nonce")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if hr.Valid {
		t.Fatalf("garbage must stay invalid")
	}
	if len(hr.ValidityReason) > 2*sandbox.MaxStoredOutputBytes+200 {
		t.Fatalf("reason must be bounded, got %d bytes", len(hr.ValidityReason))
	}
}

// W1.1: only the fenced, nonced payload parses — bare JSON (including
// atexit-delayed forgeries printed after the benchmark) never mints a score.
func TestParseHarnessOutputFencedOnly(t *testing.T) {
	good := &sandbox.ExecutionResult{
		Stdout: "some log\nKUBONA_RESULT_JSON n-1 {\"valid\": true, \"metric\": 0.5}\n",
		Stderr: "", ExitCode: 0, Sandboxed: true,
	}
	hr, err := parseHarnessOutput("ansatz-search", good, "n-1")
	if err != nil {
		t.Fatal(err)
	}
	if !hr.Valid || hr.Metric != 0.5 {
		t.Fatalf("fenced payload must parse: %+v", hr)
	}
	// Bare JSON without the fence: invalid (the old last-{...}-wins rule
	// would have minted metric 0.0 here).
	bare := &sandbox.ExecutionResult{
		Stdout: "{\"valid\": true, \"metric\": 0.0}\n",
		Stderr: "", ExitCode: 0, Sandboxed: true,
	}
	hr, _ = parseHarnessOutput("ansatz-search", bare, "n-1")
	if hr.Valid {
		t.Fatalf("unfenced JSON must never parse as a score")
	}
	// Forged second fenced line (wrong nonce + replayed nonce): ambiguous.
	forged := &sandbox.ExecutionResult{
		Stdout: "KUBONA_RESULT_JSON n-1 {\"valid\": true, \"metric\": 0.5}\n" +
			"KUBONA_RESULT_JSON n-1 {\"valid\": true, \"metric\": 0.0}\n",
		Stderr: "", ExitCode: 0, Sandboxed: true,
	}
	hr, _ = parseHarnessOutput("ansatz-search", forged, "n-1")
	if hr.Valid {
		t.Fatalf("ambiguous payloads must invalidate, never resolve by position")
	}
}
