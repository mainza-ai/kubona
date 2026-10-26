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

func TestFloorOnlyUnknownDeviceRefuses(t *testing.T) {
	fg := NewFidelityGate()
	a := fg.EvaluateAdmissionFloorOnly("mystery_qpu", 4, 1)
	if a.Decision != DecisionRefuse {
		t.Fatalf("unknown device must refuse: %+v", a)
	}
}

func TestFloorOnlyDeepCircuitMitigates(t *testing.T) {
	fg := NewFidelityGate()
	a := fg.EvaluateAdmissionFloorOnly("ibm_heron", 400, 400)
	if a.Decision != DecisionMitigate {
		t.Fatalf("deep circuit must mitigate, got %+v", a)
	}
	if a.PredictedFidelity >= 0.60 {
		t.Fatalf("400x2q must be below floor: %f", a.PredictedFidelity)
	}
}

func TestFloorOnlyShallowAdmits(t *testing.T) {
	fg := NewFidelityGate()
	a := fg.EvaluateAdmissionFloorOnly("ibm_heron", 2, 1)
	if a.Decision != DecisionAdmit {
		t.Fatalf("shallow circuit must admit: %+v", a)
	}
}

// T-02: the probe-run signal pins the honesty contract. A supplied
// measurement (EvaluateAdmission) always reports probe_run:true; the
// floor-only path (no probe) reports false — so a zero
// measured_clifford beside probe_run:false reads as "not measured".
func TestProbeRunSignalHonest(t *testing.T) {
	fg := NewFidelityGate()
	probed := fg.EvaluateAdmission("ibm_heron", 5, 2, 0.98)
	if !probed.ProbeRun {
		t.Errorf("supplied-measurement path must report probe_run:true: %+v", probed)
	}
	unprobed := fg.EvaluateAdmissionFloorOnly("ibm_heron", 5, 2)
	if unprobed.ProbeRun {
		t.Errorf("floor-only path must report probe_run:false: %+v", unprobed)
	}
	if unprobed.MeasuredClifford != 0 {
		t.Errorf("floor-only path must not fabricate a measurement: %+v", unprobed)
	}
}

func TestFidelityContractParity(t *testing.T) {
	fg := NewFidelityGate()
	pm := ProviderDeviceMap()
	for _, prov := range []string{"ibm-qiskit", "ibm-quantum", "rigetti", "quantinuum", "quantinuum-tket"} {
		dev, ok := pm[prov]
		if !ok || dev == "" {
			t.Fatalf("missing provider %q in ProviderDeviceMap: %v", prov, pm)
		}
		if _, exists := fg.Profile(dev); !exists {
			t.Fatalf("device %q mapped for %q not registered in FidelityGate", dev, prov)
		}
		resolved, err := ResolveDeviceForBackend(prov)
		if err != nil {
			t.Fatalf("ResolveDeviceForBackend(%q) failed: %v", prov, err)
		}
		if _, exists := fg.Profile(resolved); !exists {
			t.Fatalf("resolved device %q for %q not registered in FidelityGate", resolved, prov)
		}
	}
}
