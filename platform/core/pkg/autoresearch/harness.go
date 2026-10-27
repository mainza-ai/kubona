package autoresearch

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strings"
	"sync"

	"github.com/google/uuid"
	"github.com/mainza-ai/kubona/core/pkg/sandbox"
)

// Harness defines the Karpathy 4-part contract for autonomous optimization
type Harness interface {
	ID() string
	FixedSetup() string
	MutableArtifact() string
	Metric(artifact string) (float64, error)
	ValidityCheck(artifact string) (bool, string)
	Evaluate(runner *sandbox.SandboxRunner, artifact string) (*HarnessResult, error)
}

// SelfReportedHarness marks a harness whose metric is printed by the
// artifact/evaluator in the SAME process that imports the artifact. Such a
// score is forgeable (the artifact can patch the evaluator or print a fake
// payload). Production refuses these unless the operator explicitly opts in
// for research; a measured harness (separate evaluator process) does not
// implement this interface.
type SelfReportedHarness interface {
	SelfReported() bool
}

// HarnessResult stores detailed physics metrics and execution telemetry
type HarnessResult struct {
	HarnessID         string  `json:"harness_id"`
	Metric            float64 `json:"metric"` // Primary objective (lower is better)
	Valid             bool    `json:"valid"`
	ValidityReason    string  `json:"validity_reason"`
	CircuitDepth      int     `json:"circuit_depth"`
	NumQubits         int     `json:"num_qubits"`
	TwoQubitGateCount int     `json:"two_qubit_gate_count"`
	EntanglementScore float64 `json:"entanglement_score"`
	PhysicalValue     float64 `json:"physical_value"` // Raw eigenvalue or ratio
	ReferenceValue    float64 `json:"reference_value"`
	DurationMs        int64   `json:"duration_ms"`
	Sandboxed         bool    `json:"sandboxed"`
	IsolationTag      string  `json:"isolation_tag"`
	RawStdout         string  `json:"raw_stdout,omitempty"`
	RawStderr         string  `json:"raw_stderr,omitempty"`
	// S6: bounded capture accounting. Raw* hold at most
	// sandbox.MaxStoredOutputBytes bytes; the flags + byte counts make
	// truncation visible instead of silent.
	StdoutTruncated bool `json:"stdout_truncated,omitempty"`
	StderrTruncated bool `json:"stderr_truncated,omitempty"`
	StdoutBytes     int  `json:"stdout_bytes,omitempty"`
	StderrBytes     int  `json:"stderr_bytes,omitempty"`
}

// ensureRunner provides a configured sandbox runner if none was passed.
// R5: host fallback is opt-in, never default — mirroring the daemon's
// KUBONA_ALLOW_HOST_FALLBACK=1 gate. Un-sandboxed host execution must be
// a deliberate operator choice, not a silent default.
func ensureRunner(r *sandbox.SandboxRunner) *sandbox.SandboxRunner {
	if r != nil {
		return r
	}
	cfg := sandbox.DefaultConfig()
	if os.Getenv("KUBONA_ALLOW_HOST_FALLBACK") == "1" {
		cfg.AllowHostFallback = true
	}
	return sandbox.NewSandboxRunnerWithConfig(cfg)
}

// ============================================================================
// 1. AnsatzSearchHarness — Molecular VQE ground state energy minimization
// ============================================================================
type AnsatzSearchHarness struct {
	TargetFCI float64
}

func NewAnsatzSearchHarness(targetFCI float64) *AnsatzSearchHarness {
	if targetFCI == 0 {
		targetFCI = -1.85728 // H2 STO-3G exact ground state in Hartree
	}
	return &AnsatzSearchHarness{TargetFCI: targetFCI}
}

func (h *AnsatzSearchHarness) ID() string {
	return "ansatz-search"
}

func (h *AnsatzSearchHarness) FixedSetup() string {
	return "Hamiltonian: H2 molecular bond length R=0.7414A, STO-3G basis, Exact FCI: -1.85728 Ha, JW transformation"
}

func (h *AnsatzSearchHarness) MutableArtifact() string {
	return "ansatz.py"
}

func (h *AnsatzSearchHarness) ValidityCheck(artifact string) (bool, string) {
	if !strings.Contains(artifact, "build_circuit") && !strings.Contains(artifact, "QuantumCircuit") {
		return false, "artifact must define build_circuit or construct QuantumCircuit"
	}
	if !strings.Contains(artifact, "ry") && !strings.Contains(artifact, "rz") && !strings.Contains(artifact, "rx") {
		return false, "ansatz must contain parameter rotations (ry, rz, rx)"
	}
	if !strings.Contains(artifact, "cx") && !strings.Contains(artifact, "cz") {
		return false, "ansatz must contain entangler gates (cx, cz)"
	}
	return true, "valid quantum circuit ansatz"
}

func (h *AnsatzSearchHarness) Evaluate(runner *sandbox.SandboxRunner, artifact string) (*HarnessResult, error) {
	valid, reason := h.ValidityCheck(artifact)
	if !valid {
		return &HarnessResult{
			HarnessID:      h.ID(),
			Metric:         999.0,
			Valid:          false,
			ValidityReason: reason,
		}, nil
	}

	runner = ensureRunner(runner)

	// Stage 1 is the ONLY stage that imports the artifact. It returns a
	// circuit (QASM) and nothing else. Stage 2 runs in a separate sandbox
	// invocation with no artifact mounted and computes the metric itself,
	// so the artifact cannot forge or patch the score.
	stage1 := `
import json
import sys
import os
sys.path.insert(0, '/workspace')
sys.path.insert(0, os.getcwd())

try:
    import ansatz
    qc = ansatz.build_circuit() if hasattr(ansatz, 'build_circuit') else ansatz.qc
    if qc.num_parameters > 0:
        qc = qc.assign_parameters([0.5] * qc.num_parameters)
    try:
        from qiskit import qasm2
        qasm_text = qasm2.dumps(qc)
    except Exception:
        qasm_text = qc.qasm()
    print(json.dumps({"valid": True, "qasm": qasm_text}))
except Exception as e:
    print(json.dumps({"valid": False, "error": f"Artifact error: {str(e)}"}))
`
	nonce1 := newRunNonce()
	stage1 = fenceBenchmarkOutputs(stage1, nonce1)
	res1, err := runner.ExecuteHarness(stage1, h.MutableArtifact(), artifact)
	if err != nil {
		return nil, fmt.Errorf("ansatz-search artifact stage failed: %w", err)
	}
	payload1, matches1 := fencedPayload(res1.Stdout, nonce1)
	if matches1 != 1 || payload1 == nil {
		return &HarnessResult{
			HarnessID: h.ID(), Metric: 999.0, Valid: false,
			Sandboxed: res1.Sandboxed, IsolationTag: res1.IsolationTag,
			ValidityReason: fmt.Sprintf("artifact stage must emit exactly one fenced payload, found %d", matches1),
		}, nil
	}
	if ok, _ := payload1["valid"].(bool); !ok {
		msg, _ := payload1["error"].(string)
		return &HarnessResult{
			HarnessID: h.ID(), Metric: 999.0, Valid: false,
			Sandboxed: res1.Sandboxed, IsolationTag: res1.IsolationTag,
			ValidityReason: "artifact invalid: " + msg,
		}, nil
	}
	qasm, _ := payload1["qasm"].(string)
	if strings.TrimSpace(qasm) == "" {
		return &HarnessResult{
			HarnessID: h.ID(), Metric: 999.0, Valid: false,
			Sandboxed: res1.Sandboxed, IsolationTag: res1.IsolationTag,
			ValidityReason: "artifact returned an empty circuit",
		}, nil
	}
	qasmJSON, _ := json.Marshal(qasm)

	stage2 := fmt.Sprintf(`
import json
import sys
import numpy as np
from qiskit import QuantumCircuit
from qiskit.quantum_info import SparsePauliOp, Statevector

qasm = %s
target_fci = %.6f
H = SparsePauliOp.from_list([
    ("II", -1.0523732),
    ("IZ", 0.3979374),
    ("ZI", -0.3979374),
    ("ZZ", -0.0112801),
    ("XX", 0.1809312),
])
try:
    qc = QuantumCircuit.from_qasm_str(qasm)
    qc.remove_final_measurements(inplace=True)
    sv = Statevector(qc)
    energy = float(np.real(sv.expectation_value(H)))
    delta_e = abs(energy - target_fci)
    n = qc.num_qubits
    mw = 0.0
    if n >= 2:
        arr = sv.data.reshape([2] * n)
        for k in range(n):
            other_axes = [i for i in range(n) if i != k]
            rho_k = np.tensordot(arr, arr.conj(), axes=(other_axes, other_axes))
            mw += 2.0 * (1.0 - float(np.real(np.trace(rho_k @ rho_k))))
        mw = mw / n
    two_q_gates = sum(1 for inst in qc.data if inst.operation.num_qubits == 2)
    metric = delta_e
    if mw < 0.30:
        metric += 5.0
    print(json.dumps({
        "valid": True,
        "metric": float(metric),
        "physical_value": float(energy),
        "reference_value": float(target_fci),
        "circuit_depth": int(qc.depth()),
        "num_qubits": int(n),
        "two_qubit_gate_count": int(two_q_gates),
        "entanglement_score": float(mw)
    }))
except Exception as e:
    print(json.dumps({"valid": False, "error": f"Trusted evaluator error: {str(e)}"}))
`, string(qasmJSON), h.TargetFCI)

	nonce2 := newRunNonce()
	stage2 = fenceBenchmarkOutputs(stage2, nonce2)
	// No artifact is mounted in the evaluator stage: it can only see the QASM.
	res2, err := runner.ExecuteHarness(stage2, "", "")
	if err != nil {
		return nil, fmt.Errorf("ansatz-search evaluator stage failed: %w", err)
	}
	return parseHarnessOutput(h.ID(), res2, nonce2)
}
func (h *AnsatzSearchHarness) Metric(artifact string) (float64, error) {
	res, err := h.Evaluate(nil, artifact)
	if err != nil {
		return 999.0, err
	}
	if !res.Valid {
		return 999.0, fmt.Errorf("invalid artifact: %s", res.ValidityReason)
	}
	return res.Metric, nil
}

// ============================================================================
// 2. QAOACombinatorialHarness — MaxCut 8-node 3-regular graph optimization
// ============================================================================
type QAOACombinatorialHarness struct{}

func (h *QAOACombinatorialHarness) ID() string {
	return "qaoa-combinatorial"
}

func (h *QAOACombinatorialHarness) FixedSetup() string {
	return "Graph: 8-node 3-regular graph, MaxCut cost Hamiltonian, Classical MaxCut optimum = 10"
}

func (h *QAOACombinatorialHarness) MutableArtifact() string {
	return "qaoa_schedule.py"
}

func (h *QAOACombinatorialHarness) ValidityCheck(artifact string) (bool, string) {
	if !strings.Contains(artifact, "gamma") || !strings.Contains(artifact, "beta") {
		return false, "schedule must specify gamma and beta variational angles"
	}
	return true, "valid QAOA schedule"
}

func (h *QAOACombinatorialHarness) Evaluate(runner *sandbox.SandboxRunner, artifact string) (*HarnessResult, error) {
	valid, reason := h.ValidityCheck(artifact)
	if !valid {
		return &HarnessResult{HarnessID: h.ID(), Metric: 1.0, Valid: false, ValidityReason: reason}, nil
	}

	runner = ensureRunner(runner)

	// Stage 1 imports the artifact only to extract the variational schedule.
	stage1 := `
import json
import sys
import os
import math
sys.path.insert(0, '/workspace')
sys.path.insert(0, os.getcwd())
try:
    import qaoa_schedule
    gamma = [float(x) for x in getattr(qaoa_schedule, 'gamma', [0.35])]
    beta = [float(x) for x in getattr(qaoa_schedule, 'beta', [0.20])]
    if not gamma or len(gamma) != len(beta):
        raise ValueError("gamma and beta must be non-empty and equal length")
    if not all(math.isfinite(x) for x in gamma + beta):
        raise ValueError("schedule angles must be finite")
    print(json.dumps({"valid": True, "gamma": gamma, "beta": beta}))
except Exception as e:
    print(json.dumps({"valid": False, "error": f"Schedule error: {str(e)}"}))
`
	nonce1 := newRunNonce()
	stage1 = fenceBenchmarkOutputs(stage1, nonce1)
	res1, err := runner.ExecuteHarness(stage1, h.MutableArtifact(), artifact)
	if err != nil {
		return nil, fmt.Errorf("qaoa schedule stage failed: %w", err)
	}
	payload1, matches1 := fencedPayload(res1.Stdout, nonce1)
	if matches1 != 1 || payload1 == nil {
		return &HarnessResult{HarnessID: h.ID(), Metric: 1.0, Valid: false,
			Sandboxed: res1.Sandboxed, IsolationTag: res1.IsolationTag,
			ValidityReason: fmt.Sprintf("schedule stage must emit exactly one fenced payload, found %d", matches1)}, nil
	}
	if ok, _ := payload1["valid"].(bool); !ok {
		msg, _ := payload1["error"].(string)
		return &HarnessResult{HarnessID: h.ID(), Metric: 1.0, Valid: false,
			Sandboxed: res1.Sandboxed, IsolationTag: res1.IsolationTag,
			ValidityReason: "schedule invalid: " + msg}, nil
	}
	gammaJSON, _ := json.Marshal(payload1["gamma"])
	betaJSON, _ := json.Marshal(payload1["beta"])

	stage2 := fmt.Sprintf(`
import json
import numpy as np
from qiskit import QuantumCircuit
from qiskit.quantum_info import Statevector

gamma = %s
beta = %s
edges = [(0,1), (1,2), (2,3), (3,0), (4,5), (5,6), (6,7), (7,4), (0,4), (1,5), (2,6), (3,7)]
n = 8
max_cut_opt = 10.0
try:
    p = len(gamma)
    qc = QuantumCircuit(n)
    for i in range(n):
        qc.h(i)
    for layer in range(p):
        g, b = gamma[layer], beta[layer]
        for u, v in edges:
            qc.cx(u, v)
            qc.rz(2 * g, v)
            qc.cx(u, v)
        for i in range(n):
            qc.rx(2 * b, i)
    sv = Statevector(qc)
    probs = sv.probabilities_dict()
    expected_cut = 0.0
    for bitstring, prob in probs.items():
        cut = sum(1 for u, v in edges if bitstring[n - 1 - u] != bitstring[n - 1 - v])
        expected_cut += cut * prob
    approx_ratio = expected_cut / max_cut_opt
    metric = 1.0 - approx_ratio
    two_q = sum(1 for inst in qc.data if inst.operation.num_qubits == 2)
    print(json.dumps({
        "valid": True,
        "metric": float(metric),
        "physical_value": float(approx_ratio),
        "reference_value": 1.0,
        "circuit_depth": int(qc.depth()),
        "num_qubits": int(n),
        "two_qubit_gate_count": int(two_q),
        "entanglement_score": 0.72,
        "measurement_source": "template_constant",
        "measured": False
    }))
except Exception as e:
    print(json.dumps({"valid": False, "error": f"Trusted QAOA evaluator error: {str(e)}"}))
`, string(gammaJSON), string(betaJSON))

	nonce2 := newRunNonce()
	stage2 = fenceBenchmarkOutputs(stage2, nonce2)
	res2, err := runner.ExecuteHarness(stage2, "", "")
	if err != nil {
		return nil, fmt.Errorf("qaoa evaluator stage failed: %w", err)
	}
	return parseHarnessOutput(h.ID(), res2, nonce2)
}
func (h *QAOACombinatorialHarness) Metric(artifact string) (float64, error) {
	res, err := h.Evaluate(nil, artifact)
	if err != nil {
		return 1.0, err
	}
	return res.Metric, nil
}

// ============================================================================
// 3. ErrorMitigationHarness — Zero-Noise Extrapolation (ZNE) with Richardson
// ============================================================================
type ErrorMitigationHarness struct{}

func (h *ErrorMitigationHarness) ID() string {
	return "error-mitigation"
}

func (h *ErrorMitigationHarness) FixedSetup() string {
	return "Depolarizing noise simulation: p1q=0.001, p2q=0.01, GHZ-4 ZZ observable; the trusted evaluator simulates each scale and extrapolates to zero noise"
}

func (h *ErrorMitigationHarness) MutableArtifact() string {
	return "mitigation_config.json"
}

func (h *ErrorMitigationHarness) ValidityCheck(artifact string) (bool, string) {
	var cfg map[string]any
	if err := json.Unmarshal([]byte(artifact), &cfg); err != nil {
		return false, "mitigation config must be valid JSON"
	}
	if _, ok := cfg["scale_factors"]; !ok {
		return false, "configuration must specify scale_factors list"
	}
	return true, "valid mitigation config"
}

func (h *ErrorMitigationHarness) Evaluate(runner *sandbox.SandboxRunner, artifact string) (*HarnessResult, error) {
	valid, reason := h.ValidityCheck(artifact)
	if !valid {
		return &HarnessResult{HarnessID: h.ID(), Metric: 1.0, Valid: false, ValidityReason: reason}, nil
	}

	runner = ensureRunner(runner)

	// Stage 1 is data-only: it validates the scale factors and returns them.
	stage1 := `
import json
import math
try:
    with open('mitigation_config.json') as f:
        cfg = json.load(f)
    scales = [float(x) for x in cfg.get('scale_factors', [])]
    if len(scales) < 2 or len(scales) > 6:
        raise ValueError("scale_factors must contain 2..6 values")
    if not all(math.isfinite(x) and 0 < x <= 50 for x in scales):
        raise ValueError("scale_factors must be finite and in (0, 50]")
    print(json.dumps({"valid": True, "scales": scales}))
except Exception as e:
    print(json.dumps({"valid": False, "error": f"Config error: {str(e)}"}))
`
	nonce1 := newRunNonce()
	stage1 = fenceBenchmarkOutputs(stage1, nonce1)
	res1, err := runner.ExecuteHarness(stage1, h.MutableArtifact(), artifact)
	if err != nil {
		return nil, fmt.Errorf("mitigation config stage failed: %w", err)
	}
	payload1, matches1 := fencedPayload(res1.Stdout, nonce1)
	if matches1 != 1 || payload1 == nil {
		return &HarnessResult{HarnessID: h.ID(), Metric: 1.0, Valid: false,
			Sandboxed: res1.Sandboxed, IsolationTag: res1.IsolationTag,
			ValidityReason: fmt.Sprintf("config stage must emit exactly one fenced payload, found %d", matches1)}, nil
	}
	if ok, _ := payload1["valid"].(bool); !ok {
		msg, _ := payload1["error"].(string)
		return &HarnessResult{HarnessID: h.ID(), Metric: 1.0, Valid: false,
			Sandboxed: res1.Sandboxed, IsolationTag: res1.IsolationTag,
			ValidityReason: "config invalid: " + msg}, nil
	}
	scalesJSON, _ := json.Marshal(payload1["scales"])

	// Stage 2 has no artifact: it runs a real noisy density-matrix
	// simulation at each scale and Richardson-extrapolates the GHZ ZZ
	// observable to the zero-noise limit.
	stage2 := fmt.Sprintf(`
import json
import numpy as np
from qiskit import QuantumCircuit
from qiskit_aer import AerSimulator
from qiskit_aer.noise import NoiseModel, depolarizing_error
from qiskit.quantum_info import SparsePauliOp, DensityMatrix

scales = %s
p1q, p2q = 0.001, 0.01
ideal = 1.0
observable = SparsePauliOp.from_list([("ZZII", 1.0)])
try:
    def ghz():
        qc = QuantumCircuit(4)
        qc.h(0)
        for q in range(3):
            qc.cx(q, q + 1)
        qc.save_density_matrix()
        return qc

    observed = []
    for c in scales:
        nm = NoiseModel()
        nm.add_all_qubit_quantum_error(depolarizing_error(min(p1q * c, 0.99), 1), ["h", "x", "rx", "ry", "rz"])
        nm.add_all_qubit_quantum_error(depolarizing_error(min(p2q * c, 0.99), 2), ["cx"])
        sim = AerSimulator(noise_model=nm, method="density_matrix")
        result = sim.run(ghz()).result()
        rho = DensityMatrix(result.data()["density_matrix"])
        observed.append(float(np.real(rho.expectation_value(observable))))

    poly = np.polyfit(scales, observed, deg=min(len(scales) - 1, 2))
    mitigated = float(poly[-1])
    metric = abs(mitigated - ideal)
    print(json.dumps({
        "valid": True,
        "metric": float(metric),
        "physical_value": float(mitigated),
        "reference_value": float(ideal),
        "circuit_depth": 4,
        "num_qubits": 4,
        "two_qubit_gate_count": 3,
        "entanglement_score": 1.0,
        "measurement_source": "simulated",
        "measured": True
    }))
except Exception as e:
    print(json.dumps({"valid": False, "error": f"Trusted mitigation evaluator error: {str(e)}"}))
`, string(scalesJSON))

	nonce2 := newRunNonce()
	stage2 = fenceBenchmarkOutputs(stage2, nonce2)
	res2, err := runner.ExecuteHarness(stage2, "", "")
	if err != nil {
		return nil, fmt.Errorf("mitigation evaluator stage failed: %w", err)
	}
	return parseHarnessOutput(h.ID(), res2, nonce2)
}
func (h *ErrorMitigationHarness) Metric(artifact string) (float64, error) {
	res, err := h.Evaluate(nil, artifact)
	if err != nil {
		return 1.0, err
	}
	return res.Metric, nil
}

// ============================================================================
// 4. CompilationPassHarness — Quantum transpile pass manager sequencing
// ============================================================================
type CompilationPassHarness struct{}

func (h *CompilationPassHarness) ID() string {
	return "compilation-pass"
}

func (h *CompilationPassHarness) FixedSetup() string {
	return "Compile QFT-4 to a cz/rz/ry/rx/x basis; the trusted evaluator checks unitary equivalence before scoring depth x 2Q gates"
}

func (h *CompilationPassHarness) MutableArtifact() string {
	return "pass_manager.py"
}

func (h *CompilationPassHarness) ValidityCheck(artifact string) (bool, string) {
	if !strings.Contains(artifact, "optimize") && !strings.Contains(artifact, "transpile") {
		return false, "pass manager must define optimize or transpile pipeline"
	}
	return true, "valid transpiler pass configuration"
}

func (h *CompilationPassHarness) Evaluate(runner *sandbox.SandboxRunner, artifact string) (*HarnessResult, error) {
	valid, reason := h.ValidityCheck(artifact)
	if !valid {
		return &HarnessResult{HarnessID: h.ID(), Metric: 1000.0, Valid: false, ValidityReason: reason}, nil
	}

	runner = ensureRunner(runner)

	// Stage 1 runs the candidate pass manager and returns only the compiled
	// circuit. It never computes the cost.
	stage1 := `
import json
import sys
import os
sys.path.insert(0, '/workspace')
sys.path.insert(0, os.getcwd())
try:
    from qiskit import QuantumCircuit, transpile, qasm2
    from qiskit.circuit.library import QFT
    import pass_manager
    qc = QuantumCircuit(4)
    qc.append(QFT(4), range(4))
    qc = qc.decompose()
    if hasattr(pass_manager, 'optimize_circuit'):
        out = pass_manager.optimize_circuit(qc)
    else:
        out = transpile(qc, basis_gates=['cz', 'rz', 'ry', 'rx', 'x'], optimization_level=2)
    if not isinstance(out, QuantumCircuit):
        raise TypeError("optimize_circuit must return a QuantumCircuit")
    if out.num_qubits != 4:
        raise ValueError("compiled circuit must have 4 qubits")
    if out.num_clbits:
        out.remove_final_measurements(inplace=True)
    print(json.dumps({"valid": True, "qasm": qasm2.dumps(out)}))
except Exception as e:
    print(json.dumps({"valid": False, "error": f"Compilation error: {str(e)}"}))
`
	nonce1 := newRunNonce()
	stage1 = fenceBenchmarkOutputs(stage1, nonce1)
	res1, err := runner.ExecuteHarness(stage1, h.MutableArtifact(), artifact)
	if err != nil {
		return nil, fmt.Errorf("compilation stage failed: %w", err)
	}
	payload1, matches1 := fencedPayload(res1.Stdout, nonce1)
	if matches1 != 1 || payload1 == nil {
		return &HarnessResult{HarnessID: h.ID(), Metric: 1000.0, Valid: false,
			Sandboxed: res1.Sandboxed, IsolationTag: res1.IsolationTag,
			ValidityReason: fmt.Sprintf("compilation stage must emit exactly one fenced payload, found %d", matches1)}, nil
	}
	if ok, _ := payload1["valid"].(bool); !ok {
		msg, _ := payload1["error"].(string)
		return &HarnessResult{HarnessID: h.ID(), Metric: 1000.0, Valid: false,
			Sandboxed: res1.Sandboxed, IsolationTag: res1.IsolationTag,
			ValidityReason: "compilation invalid: " + msg}, nil
	}
	qasmText, _ := payload1["qasm"].(string)
	qasmJSON, _ := json.Marshal(qasmText)

	// Stage 2 has no artifact: it verifies the compiled circuit is unitarily
	// equivalent (up to global phase) to QFT-4, then scores it.
	stage2 := fmt.Sprintf(`
import json
from qiskit import QuantumCircuit, qasm2
from qiskit.circuit.library import QFT
from qiskit.quantum_info import Operator

try:
    compiled = qasm2.loads(%s)
    ref = QuantumCircuit(4)
    ref.append(QFT(4), range(4))
    ref = ref.decompose()
    if not Operator(compiled).equiv(Operator(ref)):
        raise ValueError("compiled circuit is not unitarily equivalent to QFT-4")
    depth = int(compiled.depth())
    two_q = int(sum(1 for inst in compiled.data if inst.operation.num_qubits == 2))
    cost = float(depth * two_q) / 10.0
    print(json.dumps({
        "valid": True,
        "metric": float(cost),
        "physical_value": float(depth),
        "reference_value": 15.0,
        "circuit_depth": depth,
        "num_qubits": 4,
        "two_qubit_gate_count": two_q,
        "entanglement_score": 0.88,
        "measurement_source": "template_constant",
        "measured": False
    }))
except Exception as e:
    print(json.dumps({"valid": False, "error": f"Trusted compilation evaluator error: {str(e)}"}))
`, string(qasmJSON))

	nonce2 := newRunNonce()
	stage2 = fenceBenchmarkOutputs(stage2, nonce2)
	res2, err := runner.ExecuteHarness(stage2, "", "")
	if err != nil {
		return nil, fmt.Errorf("compilation evaluator stage failed: %w", err)
	}
	return parseHarnessOutput(h.ID(), res2, nonce2)
}
func (h *CompilationPassHarness) Metric(artifact string) (float64, error) {
	res, err := h.Evaluate(nil, artifact)
	if err != nil {
		return 1000.0, err
	}
	return res.Metric, nil
}

// ============================================================================
// 5. CorrectnessRegressionHarness — Deterministic Clifford stabilizer parity
// ============================================================================
type CorrectnessRegressionHarness struct{}

func (h *CorrectnessRegressionHarness) ID() string {
	return "correctness-regression"
}

func (h *CorrectnessRegressionHarness) FixedSetup() string {
	return "Submit a 2-qubit Bell-state circuit; the trusted evaluator simulates it and measures fidelity against |Phi+>"
}

func (h *CorrectnessRegressionHarness) MutableArtifact() string {
	return "benchmarks.qasm"
}

func (h *CorrectnessRegressionHarness) ValidityCheck(artifact string) (bool, string) {
	return true, "deterministic regression suite"
}

func (h *CorrectnessRegressionHarness) Evaluate(runner *sandbox.SandboxRunner, artifact string) (*HarnessResult, error) {
	runner = ensureRunner(runner)

	// Stage 1 parses the artifact and canonicalizes it to QASM. It never
	// computes a score: the artifact can only supply a circuit.
	stage1 := `
import json
import sys
try:
    from qiskit import qasm2
    qc = qasm2.loads(open("benchmarks.qasm").read())
    if qc.num_qubits != 2:
        raise ValueError("benchmark must use exactly 2 qubits")
    if qc.num_clbits:
        qc.remove_final_measurements(inplace=True)
    if any(inst.operation.name == "measure" for inst in qc.data):
        raise ValueError("only final measurements are supported")
    print(json.dumps({"valid": True, "qasm": qasm2.dumps(qc)}))
except Exception as e:
    print(json.dumps({"valid": False, "error": f"QASM parse error: {str(e)}"}))
`
	nonce1 := newRunNonce()
	stage1 = fenceBenchmarkOutputs(stage1, nonce1)
	// The artifact is data, not an imported module: stage 1 only parses it.
	res1, err := runner.ExecuteHarness(stage1, h.MutableArtifact(), artifact)
	if err != nil {
		return nil, fmt.Errorf("correctness parse stage failed: %w", err)
	}
	payload1, matches1 := fencedPayload(res1.Stdout, nonce1)
	if matches1 != 1 || payload1 == nil {
		return &HarnessResult{HarnessID: h.ID(), Metric: 1.0, Valid: false,
			Sandboxed: res1.Sandboxed, IsolationTag: res1.IsolationTag,
			ValidityReason: fmt.Sprintf("parse stage must emit exactly one fenced payload, found %d", matches1)}, nil
	}
	if ok, _ := payload1["valid"].(bool); !ok {
		msg, _ := payload1["error"].(string)
		return &HarnessResult{HarnessID: h.ID(), Metric: 1.0, Valid: false,
			Sandboxed: res1.Sandboxed, IsolationTag: res1.IsolationTag,
			ValidityReason: "benchmark invalid: " + msg}, nil
	}
	qasmText, _ := payload1["qasm"].(string)
	qasmJSON, _ := json.Marshal(qasmText)

	stage2 := fmt.Sprintf(`
import json
import math
import numpy as np
from qiskit import qasm2
from qiskit.quantum_info import Statevector, state_fidelity

try:
    qc = qasm2.loads(%s)
    sv = Statevector(qc)
    ideal_sv = Statevector([1.0 / math.sqrt(2), 0, 0, 1.0 / math.sqrt(2)])
    fidelity = float(state_fidelity(sv, ideal_sv))
    metric = abs(1.0 - fidelity)
    print(json.dumps({
        "valid": True,
        "metric": float(metric),
        "physical_value": float(fidelity),
        "reference_value": 1.0,
        "circuit_depth": int(qc.depth()),
        "num_qubits": int(qc.num_qubits),
        "two_qubit_gate_count": int(sum(1 for inst in qc.data if inst.operation.num_qubits == 2)),
        "entanglement_score": 1.0,
        "measurement_source": "template_constant",
        "measured": False
    }))
except Exception as e:
    print(json.dumps({"valid": False, "error": f"Trusted fidelity evaluator error: {str(e)}"}))
`, string(qasmJSON))

	nonce2 := newRunNonce()
	stage2 = fenceBenchmarkOutputs(stage2, nonce2)
	res2, err := runner.ExecuteHarness(stage2, "", "")
	if err != nil {
		return nil, fmt.Errorf("correctness evaluator stage failed: %w", err)
	}
	return parseHarnessOutput(h.ID(), res2, nonce2)
}
func (h *CorrectnessRegressionHarness) Metric(artifact string) (float64, error) {
	res, err := h.Evaluate(nil, artifact)
	if err != nil {
		return 1.0, err
	}
	return res.Metric, nil
}

// ============================================================================
// resultFencePrefix marks the ONE harness-computed payload line in runner
// stdout (W1.1): parseHarnessOutput accepts only lines starting with this
// prefix carrying the run nonce, exactly once. The old rule (last {...}
// line wins) let a workload print fake JSON — including via atexit handlers
// that fire after the benchmark — and mint its own score. A per-run nonce
// plus exactly-one-match means forgery is DETECTED (0 or 2+ matches
// invalidate), not outvoted.
const resultFencePrefix = "KUBONA_RESULT_JSON"

// fenceBenchmarkOutputs rewrites every print(json.dumps(...)) in a benchmark
// template into the fenced form carrying the run nonce. All five harnesses
// route through here: one rewrite rule, never per-harness parsing.
func fenceBenchmarkOutputs(script, nonce string) string {
	return strings.ReplaceAll(script, "print(json.dumps(",
		fmt.Sprintf("print(\"%s %s \" + json.dumps(", resultFencePrefix, nonce))
}

// newRunNonce mints the per-evaluation payload nonce.
func newRunNonce() string {
	return uuid.NewString()
}

// fencedPayload extracts exactly one nonce-fenced JSON object from stdout.
// Zero or multiple matches are ambiguous and must invalidate, never resolve.
func fencedPayload(stdout, nonce string) (map[string]any, int) {
	fence := resultFencePrefix + " " + nonce + " "
	var parsed map[string]any
	matches := 0
	for _, line := range strings.Split(stdout, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, fence) {
			continue
		}
		matches++
		if matches > 1 {
			break
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, fence)), &m); err == nil {
			parsed = m
		}
	}
	return parsed, matches
}

func parseHarnessOutput(harnessID string, res *sandbox.ExecutionResult, nonce string) (*HarnessResult, error) {
	// S6: bound agent-controlled output at this choke (the single point
	// where sandbox streams enter persisted structures). Unbounded stderr
	// previously flowed verbatim into rows served by the API.
	stdout, outTrunc, outBytes := sandbox.BoundOutput(res.Stdout)
	stderr, errTrunc, errBytes := sandbox.BoundOutput(res.Stderr)
	hr := &HarnessResult{
		HarnessID:       harnessID,
		DurationMs:      res.Duration.Milliseconds(),
		Sandboxed:       res.Sandboxed,
		IsolationTag:    res.IsolationTag,
		RawStdout:       stdout,
		RawStderr:       stderr,
		StdoutTruncated: outTrunc,
		StderrTruncated: errTrunc,
		StdoutBytes:     outBytes,
		StderrBytes:     errBytes,
	}

	// W1.1 fenced parse: only lines the harness printed (prefix + run
	// nonce) count, and exactly one must exist. Zero means the benchmark
	// never reported; two or more means something else printed a fenced
	// line (forgery or bug) — ambiguous payloads invalidate, never resolve
	// by position.
	parsedPayload, matches := fencedPayload(stdout, nonce)

	if matches != 1 || parsedPayload == nil {
		hr.Valid = false
		hr.ValidityReason = fmt.Sprintf("expected exactly 1 fenced payload, found %d: refusing (stdout: %s) (stderr: %s)", matches, stdout, stderr)
		hr.Metric = 999.0
		return hr, nil
	}

	if validVal, ok := parsedPayload["valid"].(bool); ok {
		hr.Valid = validVal
	}
	if !hr.Valid {
		if errVal, ok := parsedPayload["error"].(string); ok {
			hr.ValidityReason = errVal
		}
		hr.Metric = 999.0
		return hr, nil
	}

	if metricVal, ok := parsedPayload["metric"].(float64); ok {
		if math.IsNaN(metricVal) || math.IsInf(metricVal, 0) {
			hr.Valid = false
			hr.ValidityReason = "metric is NaN or Inf"
			hr.Metric = 999.0
			return hr, nil
		}
		hr.Metric = metricVal
	} else {
		hr.Valid = false
		hr.ValidityReason = "metric field missing or not float64"
		hr.Metric = 999.0
		return hr, nil
	}
	if physVal, ok := parsedPayload["physical_value"].(float64); ok && !math.IsNaN(physVal) && !math.IsInf(physVal, 0) {
		hr.PhysicalValue = physVal
	}
	if refVal, ok := parsedPayload["reference_value"].(float64); ok && !math.IsNaN(refVal) && !math.IsInf(refVal, 0) {
		hr.ReferenceValue = refVal
	}
	if depthVal, ok := parsedPayload["circuit_depth"].(float64); ok {
		hr.CircuitDepth = int(depthVal)
	}
	if qVal, ok := parsedPayload["num_qubits"].(float64); ok {
		hr.NumQubits = int(qVal)
	}
	if twoQVal, ok := parsedPayload["two_qubit_gate_count"].(float64); ok {
		hr.TwoQubitGateCount = int(twoQVal)
	}
	if entVal, ok := parsedPayload["entanglement_score"].(float64); ok && !math.IsNaN(entVal) && !math.IsInf(entVal, 0) {
		hr.EntanglementScore = entVal
	}

	return hr, nil
}

// ============================================================================
// Registry of active harnesses
// ============================================================================
type HarnessRegistry struct {
	mu        sync.RWMutex
	harnesses map[string]Harness
}

func NewHarnessRegistry() *HarnessRegistry {
	hr := &HarnessRegistry{
		harnesses: make(map[string]Harness),
	}
	hr.Register(NewAnsatzSearchHarness(-1.85728))
	hr.Register(&QAOACombinatorialHarness{})
	hr.Register(&ErrorMitigationHarness{})
	hr.Register(&CompilationPassHarness{})
	hr.Register(&CorrectnessRegressionHarness{})
	return hr
}

func (hr *HarnessRegistry) Register(h Harness) {
	hr.mu.Lock()
	defer hr.mu.Unlock()
	hr.harnesses[h.ID()] = h
}

func (hr *HarnessRegistry) Get(id string) (Harness, error) {
	hr.mu.RLock()
	defer hr.mu.RUnlock()
	h, exists := hr.harnesses[id]
	if !exists {
		return nil, fmt.Errorf("harness '%s' not registered", id)
	}
	return h, nil
}

func (hr *HarnessRegistry) List() []Harness {
	hr.mu.RLock()
	defer hr.mu.RUnlock()
	list := make([]Harness, 0, len(hr.harnesses))
	for _, h := range hr.harnesses {
		list = append(list, h)
	}
	return list
}
