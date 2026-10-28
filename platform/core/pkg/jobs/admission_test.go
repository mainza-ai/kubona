package jobs

import (
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/mainza-ai/kubona/core/pkg/mandates"
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

// A payment-settlement row must NEVER satisfy mandate admission, even when
// cited as mandate_ref: settlement is not authorization. Pinned here so a
// future event-type drift cannot turn a settled payment into QPU spend.
func TestSettlementRowNeverPassesAdmission(t *testing.T) {
	runner, cleanup := setupRunner(t, &fakeBridge{})
	defer cleanup()

	if _, err := runner.ledger.DB().Exec(
		`INSERT INTO ledger_events (timestamp, session_id, tenant_id, event_type, payload, prev_hash, entry_hash, signer_pubkey_hex, signature_hex) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"2026-10-09T00:00:00Z", "payment-settlement", "default", "x402_payment_settled",
		`{"challenge_id":"ch-1","reference":"local:abc","simulated":true}`,
		"0", "settle-hash-1", "pub", "sig",
	); err != nil {
		t.Fatalf("seed settlement: %v", err)
	}
	err := runner.verifyMandateReceipt("settle-hash-1", "default", 1.0,
		mandates.CanonicalJobBinding("circuit_run", "sess-1", "OPENQASM 2.0;\nqreg q[1];\n"))
	if err == nil {
		t.Fatal("payment-settlement row must NOT pass mandate admission")
	}
	if !strings.Contains(err.Error(), "not a spend mandate") {
		t.Fatalf("refusal must name the settlement confusion, got: %v", err)
	}
}

// A forged mandate_verified row — right tenant, covering budget, exact job
// binding, but a signature transplanted from a DIFFERENT mandate (the DB
// writer's classic move) — must refuse. Row fields alone are not authority;
// the Ed25519 signature over the canonical payload is.
func TestForgedMandateReceiptRefusesAdmission(t *testing.T) {
	runner, cleanup := setupRunner(t, &fakeBridge{})
	defer cleanup()

	binding := mandates.CanonicalJobBinding("circuit_run", "sess-1", "OPENQASM 2.0;\nqreg q[1];\n")
	_, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("keygen: %v", err)
	}
	// Sign a $1 mandate, then transplant its signature onto a $10000 row.
	small, err := mandates.SignMandate(priv, "default", binding, 1.0,
		time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	forged, err := json.Marshal(map[string]any{
		"tenant_id": "default", "job_hash": binding, "max_budget_usd": 10000,
		"expiry_timestamp": "2030-01-01T00:00:00Z",
		"public_key_hex":   small.PublicKeyHex, "signature_hex": small.SignatureHex,
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if _, err := runner.ledger.DB().Exec(
		`INSERT INTO ledger_events (timestamp, session_id, tenant_id, event_type, payload, prev_hash, entry_hash, signer_pubkey_hex, signature_hex) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"2026-10-09T00:00:00Z", "sess-1", "default", "mandate_verified_client", string(forged),
		"0", "forged-hash-1", small.PublicKeyHex, small.SignatureHex,
	); err != nil {
		t.Fatalf("seed forged receipt: %v", err)
	}
	err = runner.verifyMandateReceipt("forged-hash-1", "default", 25.0, binding)
	if err == nil {
		t.Fatal("forged mandate receipt (transplanted signature) must NOT pass admission")
	}
	if !strings.Contains(err.Error(), "signature refused") {
		t.Fatalf("refusal must name the signature, got: %v", err)
	}
}
