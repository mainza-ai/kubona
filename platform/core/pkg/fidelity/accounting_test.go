package fidelity

import (
	"testing"
)

func TestFidelityAccountingAdmission(t *testing.T) {
	gate := NewFidelityGate()

	// 1. Shallow circuit on IBM Heron should be admitted
	shallow := gate.EvaluateAdmission("ibm_heron", 5, 2, 0.98)
	if shallow.Decision != DecisionAdmit {
		t.Fatalf("expected ADMIT for shallow circuit, got %s: %s", shallow.Decision, shallow.Reason)
	}

	// 2. Calibration drift probe failure should trigger downgrade
	drifted := gate.EvaluateAdmission("ibm_heron", 5, 2, 0.60) // probe measured 0.60 vs ~0.99 predicted
	if drifted.Decision != DecisionDowngrade {
		t.Fatalf("expected DOWNGRADE_TO_SIMULATOR on calibration drift, got %s", drifted.Decision)
	}

	// 3. Deep circuit with low fidelity should trigger mitigation requirement
	deep := gate.EvaluateAdmission("rigetti_aspen", 100, 80, 0.40)
	if deep.Decision != DecisionMitigate {
		t.Fatalf("expected APPLY_MITIGATION for deep circuit, got %s: %s", deep.Decision, deep.Reason)
	}

	// 4. Unknown device should refuse
	unknown := gate.EvaluateAdmission("unknown_qpu", 5, 2, 0.9)
	if unknown.Decision != DecisionRefuse {
		t.Fatalf("expected REFUSE for unknown device, got %s", unknown.Decision)
	}
}
