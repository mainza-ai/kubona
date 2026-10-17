package api

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"math"
	"time"

	"github.com/mainza-ai/kubona/core/pkg/agents"
	"github.com/mainza-ai/kubona/core/pkg/autoresearch"
	"github.com/mainza-ai/kubona/core/pkg/jobs"
	"github.com/mainza-ai/kubona/core/pkg/ledger"
	"github.com/mainza-ai/kubona/core/pkg/qir"
	"github.com/mainza-ai/kubona/core/pkg/sandbox"
	"github.com/mainza-ai/kubona/core/pkg/session"
	pb "github.com/mainza-ai/kubona/core/proto"
)

// bridgeTaskExecutor routes A2A delegated tasks onto ledger-bound execution:
// bridge-RPC capabilities go through the jobs runner (one budget rail);
// locally-computed capabilities (harness evaluation, QIR compilation) execute
// directly and seal a ledger receipt. Task types with no binding refuse.
type bridgeTaskExecutor struct {
	runner   *jobs.Runner
	sessions *session.Manager
	ledger   *ledger.Ledger
	harness  *autoresearch.HarnessRegistry
	sandbox  *sandbox.SandboxRunner
	qir      *qir.Compiler
	// signer signs delegation receipts (W0.18): sealReceipt with a nil key
	// recorded hash-chained-but-unsigned rows, rewrite-forgeable under the
	// ledger verifier's unsigned-event gap. Wired from the server key.
	signer ed25519.PrivateKey
}

// a2aMaxWait bounds the ledger-job poll (W2.20): without it a Background
// context (no deadline) polls forever when workers stall. The context
// deadline still wins when earlier; this is the backstop, not the budget.
// Var (not const) so tests can shrink it without waiting minutes.
var a2aMaxWait = 10 * time.Minute

// ExecuteTask implements agents.TaskExecutor.
func (e *bridgeTaskExecutor) ExecuteTask(ctx context.Context, taskType string, payload json.RawMessage, tenantID string, maxBudgetUSD float64) (map[string]any, error) {
	// Tenant-attributed delegation session: A2A spend and autonomy checks
	// resolve against the delegating lab, never a shared pseudo-session.
	// W0.18: tenantID arrives from the verified parent mandate (SubmitTask
	// refuses empty tenants) or "default" for roots; the session ceiling
	// is the task's own budget, never an invented $100.
	if tenantID == "" {
		tenantID = "default"
	}
	if maxBudgetUSD <= 0 {
		return nil, fmt.Errorf("delegation without a task budget: dispatch refused")
	}
	sessionID := "a2a-delegation-" + tenantID
	if _, err := e.sessions.EnsureSessionWithID(sessionID, "A2A delegation ("+tenantID+")", maxBudgetUSD, tenantID); err != nil {
		return nil, fmt.Errorf("delegation session unavailable for tenant %q: %w", tenantID, err)
	}

	// Locally-computed capabilities (no bridge RPC): execute + seal receipt.
	switch taskType {
	case "variational_ansatz_optimization":
		return e.executeHarness(ctx, sessionID, payload)
	case "qir_llvm_lowering":
		return e.executeQIR(sessionID, payload)
	}

	jobType, jobParams, maxCost, err := mapA2ATaskToJob(taskType, payload)
	if err != nil {
		return nil, err
	}
	job, err := e.runner.SubmitJobWithDeclaredCost(sessionID, jobType, jobParams, maxCost)
	if err != nil {
		return nil, fmt.Errorf("failed to submit ledger job for a2a task %q: %w", taskType, err)
	}

	// Wait for a worker to settle the job. Workers run in kubonad; without
	// them (or without a bridge) the context deadline fails the task honestly
	// instead of hanging forever. W2.20: a2aMaxWait caps the poll even when
	// the caller passes a deadline-less context.
	waitCtx, waitCancel := context.WithTimeout(ctx, a2aMaxWait)
	defer waitCancel()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-waitCtx.Done():
			if ctx.Err() != nil {
				return nil, fmt.Errorf("a2a task %q timed out waiting for ledger job %s: %w", taskType, job.ID, ctx.Err())
			}
			return nil, fmt.Errorf("a2a task %q timed out waiting for ledger job %s after %s (poll deadline)", taskType, job.ID, a2aMaxWait)
		case <-ticker.C:
			stored, err := e.runner.GetJob(job.ID)
			if err != nil {
				return nil, fmt.Errorf("failed to poll ledger job %s: %w", job.ID, err)
			}
			switch stored.Status {
			case pb.JobStatus_JOB_STATUS_COMPLETED:
				var result map[string]any
				if err := json.Unmarshal(stored.ResultJSON, &result); err != nil {
					return nil, fmt.Errorf("failed to decode ledger job result: %w", err)
				}
				result["job_id"] = stored.ID
				result["ledger_bound"] = true
				return result, nil
			case pb.JobStatus_JOB_STATUS_FAILED:
				return nil, fmt.Errorf("ledger job %s failed: %s", stored.ID, string(stored.ResultJSON))
			}
		}
	}
}

// executeHarness runs an autoresearch harness evaluation for a delegated
// variational task and seals a ledger receipt. The artifact must pass the
// harness's own ValidityCheck first; invalid artifacts refuse before any
// compute. Payload: {"harness_id": "ansatz-search", "artifact": "<python>"}.
func (e *bridgeTaskExecutor) executeHarness(ctx context.Context, sessionID string, payload json.RawMessage) (map[string]any, error) {
	var p struct {
		HarnessID string `json:"harness_id"`
		Artifact  string `json:"artifact"`
	}
	if err := json.Unmarshal(payload, &p); err != nil {
		return nil, fmt.Errorf("invalid harness payload JSON: %w", err)
	}
	if p.HarnessID == "" {
		return nil, fmt.Errorf("variational_ansatz_optimization requires harness_id")
	}
	if p.Artifact == "" {
		return nil, fmt.Errorf("variational_ansatz_optimization requires artifact")
	}
	h, err := e.harness.Get(p.HarnessID)
	if err != nil {
		return nil, fmt.Errorf("unknown harness %q: refused, not faked", p.HarnessID)
	}
	if ok, reason := h.ValidityCheck(p.Artifact); !ok {
		return nil, fmt.Errorf("artifact failed validity check: %s", reason)
	}
	res, err := h.Evaluate(e.sandbox, p.Artifact)
	if err != nil {
		return nil, fmt.Errorf("harness evaluation failed: %w", err)
	}
	rec, err := e.sealReceipt(sessionID, "a2a_harness_eval", map[string]any{
		"harness_id": p.HarnessID, "metric": res.Metric, "valid": res.Valid,
		"depth": res.CircuitDepth, "qubits": res.NumQubits,
	})
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"harness_id": p.HarnessID, "metric": res.Metric, "valid": res.Valid,
		"validity_reason": res.ValidityReason, "circuit_depth": res.CircuitDepth,
		"num_qubits": res.NumQubits, "two_qubit_gates": res.TwoQubitGateCount,
		"entanglement_score": res.EntanglementScore, "physical_value": res.PhysicalValue,
		"reference_value": res.ReferenceValue, "sandboxed": res.Sandboxed,
		"ledger_bound": true, "ledger_receipt": rec.EntryHash,
	}, nil
}

// executeQIR compiles QASM through the QIR compiler for a delegated task and
// seals a ledger receipt. Payload: {"qasm": "OPENQASM 2.0; ..."}.
func (e *bridgeTaskExecutor) executeQIR(sessionID string, payload json.RawMessage) (map[string]any, error) {
	var p struct {
		QASM string `json:"qasm"`
	}
	if err := json.Unmarshal(payload, &p); err != nil {
		return nil, fmt.Errorf("invalid qir payload JSON: %w", err)
	}
	if p.QASM == "" {
		return nil, fmt.Errorf("qir_llvm_lowering requires qasm")
	}
	// W1.9: validate before analysis — malformed QASM used to lower into
	// plausible-but-wrong IR (fallback estimates as compilation).
	if err := jobs.ValidateQASM(p.QASM); err != nil {
		return nil, err
	}
	summary, err := e.qir.Analyze(p.QASM)
	if err != nil {
		return nil, fmt.Errorf("qir compilation failed: %w", err)
	}
	rec, err := e.sealReceipt(sessionID, "a2a_qir_compile", map[string]any{
		"num_qubits": summary.NumQubits, "depth": summary.Depth,
		"gate_count": summary.GateCount, "compiler_used": summary.CompilerUsed,
	})
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"num_qubits": summary.NumQubits, "depth": summary.Depth,
		"gate_count": summary.GateCount, "t_count": summary.TCount,
		"qir_profile": summary.QIRProfile, "compiler_used": summary.CompilerUsed,
		"fallback_estimate": summary.FallbackEstimate,
		"ledger_bound":      true, "ledger_receipt": rec.EntryHash,
	}, nil
}

func (e *bridgeTaskExecutor) sealReceipt(sessionID, eventType string, fields map[string]any) (*ledger.EventRecord, error) {
	rec, err := e.ledger.AppendSignedEvent(sessionID, eventType, fields, e.signer)
	if err != nil {
		return nil, fmt.Errorf("failed to seal delegation receipt: %w", err)
	}
	return rec, nil
}

// mapA2ATaskToJob binds an advertised A2A capability to a ledger job type with
// strictly validated parameters. Required fields that are missing or
// non-positive are rejected — the orchestrator never guesses physics inputs.
// Every bridge-bound task must also declare max_cost_usd > 0 (W0.2): the
// delegation rail reserves it at submit, so A2A spend cannot bypass the
// budget rail the HTTP path enforces.
func mapA2ATaskToJob(taskType string, payload json.RawMessage) (string, any, float64, error) {
	var p map[string]any
	if err := json.Unmarshal(payload, &p); err != nil {
		return "", nil, 0, fmt.Errorf("invalid task payload JSON: %w", err)
	}
	// Declared spend first: no bridge task dispatches without an explicit,
	// positive, finite cost the rail can reserve.
	var maxCost float64
	costOK := false
	if v, ok := p["max_cost_usd"]; ok {
		switch n := v.(type) {
		case float64:
			maxCost, costOK = n, true
		case int:
			maxCost, costOK = float64(n), true
		default:
			costOK = false
		}
	} else {
		costOK = false
	}
	if !costOK || math.IsNaN(maxCost) || math.IsInf(maxCost, 0) || maxCost <= 0 {
		return "", nil, 0, fmt.Errorf("%s requires max_cost_usd > 0: spend must be declared, never defaulted", taskType)
	}
	strField := func(key string) string {
		v, _ := p[key].(string)
		return v
	}
	numFieldOK := func(key string) (float64, bool) {
		switch v := p[key].(type) {
		case float64:
			return v, true
		case int:
			return float64(v), true
		default:
			return 0, false
		}
	}
	numField := func(key string) float64 {
		v, _ := numFieldOK(key)
		return v
	}

	switch taskType {
	case "molecular_ground_state_pyscf":
		molecule := strField("molecule")
		if molecule == "" {
			return "", nil, 0, fmt.Errorf("molecular_ground_state_pyscf requires molecule")
		}
		bond := numField("bond_distance")
		if bond <= 0 {
			return "", nil, 0, fmt.Errorf("molecular_ground_state_pyscf requires bond_distance > 0")
		}
		basis := strField("basis_set")
		if basis == "" {
			basis = "sto-3g"
		}
		return "classical_anchor", &pb.ClassicalAnchorRequest{
			Molecule:     molecule,
			BondDistance: bond,
			BasisSet:     basis,
		}, maxCost, nil

	case "circuit_cutting_recombination":
		qasm := strField("qasm")
		if qasm == "" {
			if qasm = strField("circuit"); qasm == "" {
				return "", nil, 0, fmt.Errorf("circuit_cutting_recombination requires qasm")
			}
		}
		maxQ := int32(numField("max_subcircuit_qubits"))
		if maxQ <= 0 {
			return "", nil, 0, fmt.Errorf("circuit_cutting_recombination requires max_subcircuit_qubits > 0")
		}
		return "circuit_cut", &pb.CircuitCutRequest{
			Qasm:                qasm,
			MaxSubcircuitQubits: maxQ,
		}, maxCost, nil

	case "zne_error_mitigation":
		qasm := strField("qasm")
		if qasm == "" {
			return "", nil, 0, fmt.Errorf("zne_error_mitigation requires qasm")
		}
		method := strField("method")
		if method == "" {
			method = "ZNE"
		}
		var scales []float64
		if raw, ok := p["scale_factors"].([]any); ok {
			for _, s := range raw {
				if f, ok := s.(float64); ok && f > 0 {
					scales = append(scales, f)
				}
			}
		}
		if len(scales) == 0 {
			scales = []float64{1.0, 1.5, 2.0}
		}
		// W0.18: shots drive metered cost — required explicit, never
		// defaulted (a 1024-default invents spend the declarer never stated).
		shots, shotsOK := numFieldOK("shots")
		if !shotsOK || shots <= 0 || math.IsNaN(shots) || math.IsInf(shots, 0) {
			return "", nil, 0, fmt.Errorf("zne_error_mitigation requires shots > 0: spend must be declared, never defaulted")
		}
		return "mitigate", &pb.MitigationRequest{
			Qasm:         qasm,
			Method:       method,
			ScaleFactors: scales,
			Shots:        int32(shots),
		}, maxCost, nil

	case "krylov_subspace_ground_state":
		molecule := strField("molecule")
		if molecule == "" {
			return "", nil, 0, fmt.Errorf("krylov_subspace_ground_state requires molecule")
		}
		bond := numField("bond_distance")
		if bond <= 0 {
			return "", nil, 0, fmt.Errorf("krylov_subspace_ground_state requires bond_distance > 0")
		}
		// W0.18: subspace physics drives the verdict — dim, tau, and shots
		// are required explicit (the old dim=4/tau=0.2/shots=1024 defaults
		// minted verdicts from invented inputs). Basis keeps its sto-3g
		// default: the standard minimal basis, recorded visibly in params.
		dim, dimOK := numFieldOK("subspace_dimension")
		if !dimOK || dim <= 0 || math.IsNaN(dim) || math.IsInf(dim, 0) {
			return "", nil, 0, fmt.Errorf("krylov_subspace_ground_state requires subspace_dimension > 0")
		}
		tau, tauOK := numFieldOK("time_step")
		if !tauOK || tau <= 0 || math.IsNaN(tau) || math.IsInf(tau, 0) {
			return "", nil, 0, fmt.Errorf("krylov_subspace_ground_state requires time_step > 0")
		}
		shots, shotsOK := numFieldOK("shots")
		if !shotsOK || shots <= 0 || math.IsNaN(shots) || math.IsInf(shots, 0) {
			return "", nil, 0, fmt.Errorf("krylov_subspace_ground_state requires shots > 0")
		}
		return "krylov_solve", &pb.QuantumKrylovRequest{
			Molecule:          molecule,
			BondDistance:      bond,
			SubspaceDimension: int32(dim),
			TimeStep:          tau,
			Shots:             int32(shots),
		}, maxCost, nil

	case "epistemic_claim_evaluation":
		// The only production path into claim_eval jobs: without it the
		// verdict pipeline (bridge evaluation, replan fold, claim record)
		// is reachable only from tests. Values are caller-measured, never
		// defaulted into existence — a verdict needs a real comparison.
		claimID := strField("claim_id")
		statement := strField("statement")
		if claimID == "" || statement == "" {
			return "", nil, 0, fmt.Errorf("epistemic_claim_evaluation requires claim_id and statement")
		}
		qv, qOK := numFieldOK("quantum_value")
		cv, cOK := numFieldOK("classical_anchor_value")
		if !qOK || !cOK {
			return "", nil, 0, fmt.Errorf("epistemic_claim_evaluation requires measured quantum_value and classical_anchor_value")
		}
		var platforms []string
		if raw, ok := p["source_platforms"].([]any); ok {
			for _, s := range raw {
				if name, ok := s.(string); ok && name != "" {
					platforms = append(platforms, name)
				}
			}
		}
		if len(platforms) == 0 {
			return "", nil, 0, fmt.Errorf("epistemic_claim_evaluation requires at least one source_platform")
		}
		engagement := int64(numField("total_engagement"))
		contra, _ := numFieldOK("contradiction_signal")
		return "claim_eval", &pb.ClaimEvaluationRequest{
			ClaimId:              claimID,
			Statement:            statement,
			QuantumValue:         qv,
			ClassicalAnchorValue: cv,
			SourcePlatforms:      platforms,
			TotalEngagement:      engagement,
			ContradictionSignal:  contra,
		}, maxCost, nil

	case "potential_energy_scan":
		// Classical PES sweep: the grid is caller-stated (no defaulted
		// geometries); steps default to 5, a count, not physics. Range and
		// step bounds enforce here and again in the worker.
		molecule := strField("molecule")
		if molecule == "" {
			return "", nil, 0, fmt.Errorf("potential_energy_scan requires molecule")
		}
		rMin, minOK := numFieldOK("r_min")
		rMax, maxOK := numFieldOK("r_max")
		if !minOK || !maxOK {
			return "", nil, 0, fmt.Errorf("potential_energy_scan requires r_min and r_max")
		}
		steps := int(numField("steps"))
		if steps == 0 {
			steps = 5
		}
		params := map[string]any{
			"molecule": molecule, "r_min": rMin, "r_max": rMax, "steps": steps,
		}
		rawParams, _ := json.Marshal(params)
		if _, err := jobs.ValidateSweepParams(rawParams); err != nil {
			return "", nil, 0, err
		}
		return "pes_sweep", params, maxCost, nil

	default:
		return "", nil, 0, fmt.Errorf("no ledger-bound executor for task_type %q: refused, not faked", taskType)
	}
}

// ensure bridgeTaskExecutor satisfies the agents contract at compile time.
var _ agents.TaskExecutor = (*bridgeTaskExecutor)(nil)
