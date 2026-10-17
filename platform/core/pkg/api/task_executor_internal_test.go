package api

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"testing"

	"github.com/mainza-ai/kubona/core/pkg/autoresearch"
	"github.com/mainza-ai/kubona/core/pkg/jobs"
	"github.com/mainza-ai/kubona/core/pkg/ledger"
	"github.com/mainza-ai/kubona/core/pkg/qir"
	"github.com/mainza-ai/kubona/core/pkg/session"
)

// Adapter contract: the A2A edge maps advertised capabilities onto ledger
// jobs with strict parameter validation. Anything unmapped or underspecified
// is refused here — enforcement lives in the core rail, never in the adapter.
func TestMapA2ATaskToJobContract(t *testing.T) {
	// Unknown task types refuse (no invented execution paths).
	if _, _, _, err := mapA2ATaskToJob("teleportation_teleport", json.RawMessage(`{}`)); err == nil {
		t.Fatalf("unmapped capability must refuse")
	}

	// Underspecified physics inputs refuse rather than default-guess.
	for _, tc := range []struct {
		taskType string
		payload  string
	}{
		{"molecular_ground_state_pyscf", `{"bond_distance":0.7414}`},
		{"molecular_ground_state_pyscf", `{"molecule":"H2","bond_distance":0}`},
		{"circuit_cutting_recombination", `{"qasm":"q;","max_subcircuit_qubits":0}`},
		{"zne_error_mitigation", `{}`},
		{"krylov_subspace_ground_state", `{"molecule":"H2"}`},
		{"molecular_ground_state_pyscf", `not-json`},
		{"epistemic_claim_evaluation", `{}`},
		{"epistemic_claim_evaluation", `{"claim_id":"c1","statement":"s"}`},
		{"epistemic_claim_evaluation", `{"claim_id":"c1","statement":"s","quantum_value":-1.1,"classical_anchor_value":-1.13}`},
		{"potential_energy_scan", `{}`},
		{"potential_energy_scan", `{"molecule":"H2"}`},
		{"potential_energy_scan", `{"molecule":"H2","r_min":0.5,"r_max":1.5,"steps":1}`},
		{"potential_energy_scan", `{"molecule":"H2","r_min":1.5,"r_max":0.5}`},
		{"molecular_ground_state_pyscf", `{"molecule":"H2","bond_distance":0.7414}`},
		{"molecular_ground_state_pyscf", `{"molecule":"H2","bond_distance":0.7414,"max_cost_usd":0}`},
		{"molecular_ground_state_pyscf", `{"molecule":"H2","bond_distance":0.7414,"max_cost_usd":-5}`},
		{"zne_error_mitigation", `{"qasm":"q;","max_cost_usd":"a-lot"}`},
		{"zne_error_mitigation", `{"qasm":"q;","max_cost_usd":1.0}`},
		{"krylov_subspace_ground_state", `{"molecule":"H2","bond_distance":0.7414,"max_cost_usd":2.5}`},
		{"krylov_subspace_ground_state", `{"molecule":"H2","bond_distance":0.7414,"max_cost_usd":2.5,"subspace_dimension":4,"time_step":0.2}`},
	} {
		if _, _, _, err := mapA2ATaskToJob(tc.taskType, json.RawMessage(tc.payload)); err == nil {
			t.Fatalf("%s %s must refuse", tc.taskType, tc.payload)
		}
	}

	// Mapped capabilities bind the right ledger job types.
	mapped := map[string]string{
		"molecular_ground_state_pyscf":  "classical_anchor",
		"circuit_cutting_recombination": "circuit_cut",
		"zne_error_mitigation":          "mitigate",
		"krylov_subspace_ground_state":  "krylov_solve",
		"epistemic_claim_evaluation":    "claim_eval",
		"potential_energy_scan":         "pes_sweep",
	}
	payloads := map[string]string{
		"molecular_ground_state_pyscf":  `{"molecule":"H2","bond_distance":0.7414,"max_cost_usd":2.5,"subspace_dimension":4,"time_step":0.2,"shots":512}`,
		"circuit_cutting_recombination": `{"qasm":"q;","max_subcircuit_qubits":2,"max_cost_usd":1.0}`,
		"zne_error_mitigation":          `{"qasm":"q;","max_cost_usd":1.0,"shots":512}`,
		"krylov_subspace_ground_state":  `{"molecule":"H2","bond_distance":0.7414,"max_cost_usd":2.5,"subspace_dimension":4,"time_step":0.2,"shots":512}`,
		"epistemic_claim_evaluation":    `{"claim_id":"claim-h2-ground","statement":"H2 within chemical accuracy","quantum_value":-1.1368,"classical_anchor_value":-1.13727,"source_platforms":["pyscf","aer"],"total_engagement":12,"max_cost_usd":0.5}`,
		"potential_energy_scan":         `{"molecule":"H2","r_min":0.5,"r_max":1.5,"max_cost_usd":3.0}`,
	}
	for taskType, wantJob := range mapped {
		jobType, _, _, err := mapA2ATaskToJob(taskType, json.RawMessage(payloads[taskType]))
		if err != nil {
			t.Fatalf("%s: %v", taskType, err)
		}
		if jobType != wantJob {
			t.Fatalf("%s must map to %s, got %s", taskType, wantJob, jobType)
		}
	}
}

// W0.18: the delegation session ceiling is the task's own budget (never an
// invented $100), budget-less execution refuses, and delegation receipts
// carry the server signature (never nil-key unsigned rows).
func TestExecuteTaskBudgetBinding(t *testing.T) {
	l, err := ledger.OpenLedger(t.TempDir() + "/exec-budget.db")
	if err != nil {
		t.Fatalf("open ledger: %v", err)
	}
	t.Cleanup(func() { l.Close() })
	sm := session.NewManager(l)
	jr := jobs.NewRunner(l, nil, 1)
	_, priv, _ := ed25519.GenerateKey(nil)
	ex := &bridgeTaskExecutor{runner: jr, sessions: sm, ledger: l,
		harness: autoresearch.NewHarnessRegistry(), qir: qir.NewCompiler(), signer: priv}
	if _, err := sm.EnsureSessionWithID("sess-x", "x", 100.0, "default"); err != nil {
		t.Fatalf("seed session: %v", err)
	}

	// Zero budget refuses before any session or receipt exists.
	if _, err := ex.ExecuteTask(context.Background(), "qir_llvm_lowering",
		json.RawMessage(`{"qasm":"OPENQASM 2.0;\nqreg q[1];"}`), "default", 0); err == nil {
		t.Fatalf("budget-less delegation must refuse")
	}

	// Positive budget: delegation session ceiling equals the task budget.
	out, err := ex.ExecuteTask(context.Background(), "qir_llvm_lowering",
		json.RawMessage(`{"qasm":"OPENQASM 2.0;\nqreg q[1];"}`), "default", 17.5)
	if err != nil {
		t.Fatalf("budgeted delegation: %v", err)
	}
	if out["ledger_bound"] != true || out["ledger_receipt"] == "" {
		t.Fatalf("qir result must carry a ledger receipt: %+v", out)
	}
	sess, err := sm.GetSession("a2a-delegation-default")
	if err != nil {
		t.Fatalf("delegation session: %v", err)
	}
	if sess.SpendLimitUSD != 17.5 {
		t.Fatalf("session ceiling must equal task budget, got %v", sess.SpendLimitUSD)
	}
	// The receipt is server-signed (W1.12-class unsigned rows are gone here).
	var sig string
	if err := l.DB().QueryRow(
		`SELECT signature_hex FROM ledger_events WHERE entry_hash = ?`,
		out["ledger_receipt"]).Scan(&sig); err != nil || sig == "" {
		t.Fatalf("delegation receipt must be signed, got %q %v", sig, err)
	}
}
