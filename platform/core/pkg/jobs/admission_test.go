package jobs

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// W2.5: two-qubit counting covers the full vocabulary (safe direction) and
// comments never score.
func TestCountQasmGatesVocabulary(t *testing.T) {
	qasm := `OPENQASM 2.0;
include "qelib1.inc";
qreg q[3];
creg c[3];
h q[0];
// cx q[0], q[1];
cnot q[0], q[1];
cy q[0], q[2];
ch q[1], q[2];
crx(0.5) q[0], q[1];
rxx(0.1) q[1], q[2];
swap q[0], q[2];
measure q[0] -> c[0];
`
	qubits, depth, twoQ := countQasmGates(qasm)
	if qubits != 3 {
		t.Fatalf("qubits: got %d", qubits)
	}
	// h + cnot + cy + ch + crx + rxx + swap = 7 depth (measure skipped).
	if depth != 7 {
		t.Fatalf("depth: got %d", depth)
	}
	// All six entanglers count (commented cx must not).
	if twoQ != 6 {
		t.Fatalf("twoQ: got %d (commented cx must not score)", twoQ)
	}
	// Unknown 2-operand gate counts entangling (fail-closed).
	q, d, t2 := countQasmGates("OPENQASM 2.0;\nqreg q[2];\nfoo q[0], q[1];\n")
	if q != 2 || d != 1 || t2 != 1 {
		t.Fatalf("unknown 2q gate must count entangling: %d %d %d", q, d, t2)
	}
}

func TestBrierCalibrationAdmissionGate(t *testing.T) {
	runner, cleanup := setupRunner(t, &fakeBridge{})
	defer cleanup()

	sessID := "sess-brier-test"
	if _, err := runner.ledger.DB().Exec(`INSERT INTO sessions (id, tenant_id, spend_limit_micro_usd, current_spend_micro_usd) VALUES (?, ?, ?, ?)`, sessID, "default", 100000000, 0); err != nil {
		t.Fatalf("failed to insert session: %v", err)
	}

	// 1. Simulators must pass regardless of prediction history
	simParams, _ := json.Marshal(map[string]any{
		"qasm":              "OPENQASM 2.0;\nqreg q[2];\nh q[0];\n",
		"requested_backend": "local-simulator",
	})
	if _, _, err := runner.admitJob(sessID, "circuit_run", simParams, 0); err != nil {
		t.Fatalf("simulator must be admitted, got err: %v", err)
	}

	// 2. Append 5 well-calibrated predictions (brier = 0.05 <= 0.25)
	for i := 0; i < 5; i++ {
		payload := map[string]any{
			"prediction_hash": fmt.Sprintf("pred-%d", i),
			"brier_score":     0.05,
		}
		if err := runner.ledger.AppendEvent(sessID, "prediction_evaluated", payload); err != nil {
			t.Fatalf("failed to append prediction_evaluated: %v", err)
		}
	}
	if err := runner.ledger.CheckBrierCalibration(sessID); err != nil {
		t.Fatalf("expected well-calibrated, got: %v", err)
	}

	// 3. Append degraded predictions so mean Brier > 0.25 (5 more with brier = 0.80)
	for i := 5; i < 10; i++ {
		payload := map[string]any{
			"prediction_hash": fmt.Sprintf("pred-%d", i),
			"brier_score":     0.80,
		}
		if err := runner.ledger.AppendEvent(sessID, "prediction_evaluated", payload); err != nil {
			t.Fatalf("failed to append prediction_evaluated: %v", err)
		}
	}
	// Total: 5*0.05 + 5*0.80 = 4.25 / 10 = 0.425 > 0.25
	err := runner.ledger.CheckBrierCalibration(sessID)
	if err == nil || !strings.Contains(err.Error(), "epistemic calibration degraded") {
		t.Fatalf("expected epistemic calibration degraded error, got: %v", err)
	}

	// 4. Simulator still admitted!
	if _, _, err := runner.admitJob(sessID, "circuit_run", simParams, 0); err != nil {
		t.Fatalf("simulator must still be admitted when degraded, got err: %v", err)
	}

	// 5. Metered hardware must be REFUSED on degraded calibration!
	hwParams, _ := json.Marshal(map[string]any{
		"qasm":                 "OPENQASM 2.0;\nqreg q[2];\nh q[0];\n",
		"requested_backend":    "ibm_sherbrooke",
		"max_cost_usd":         10.0,
		"caller_jurisdictions": []string{"US"},
	})
	if _, _, err := runner.admitJob(sessID, "circuit_run", hwParams, 10.0); err == nil || !strings.Contains(err.Error(), "epistemic calibration degraded") {
		t.Fatalf("metered hardware must be refused on degraded calibration, got: %v", err)
	}
}

func TestCountQASMGatesHandlesAnyRegisterAndMultipleQregs(t *testing.T) {
	qubits, _, twoQ := countQasmGates("OPENQASM 2.0;\nqreg a[2];\nqreg b[3];\ncx a[0],b[1];")
	if qubits != 5 {
		t.Fatalf("qreg widths must accumulate, got qubits=%d", qubits)
	}
	if twoQ != 1 {
		t.Fatalf("non-q registers must still count as 2q operands, got twoQ=%d", twoQ)
	}
}
