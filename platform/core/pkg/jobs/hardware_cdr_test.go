package jobs

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestHardwareExperimentCDRBindingAndValidation(t *testing.T) {
	base := HardwareExperimentParams{
		Kind:                "vqe_hardware",
		Molecule:            "H2",
		BondDistance:        0.735,
		Layers:              2,
		Shots:               4096,
		Seed:                7,
		Theta:               []float64{0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7, 0.8},
		PauliCoefficients:   map[string]float64{"II": -0.42, "IZ": -0.40, "ZI": -0.40, "ZZ": 0.10, "XX": 0.09, "YY": -0.09},
		FCIExactEnergy:      -1.137306,
		Provider:            "ibm-qiskit",
		RequestedBackend:    "ibm_fez",
		QASM:                "OPENQASM 2.0;\ninclude \"qelib1.inc\";\nqreg q[2];\ncx q[0],q[1];\n",
		FidelityQASM:        "OPENQASM 2.0;\ninclude \"qelib1.inc\";\nqreg q[2];\ncx q[0],q[1];\n",
		MaxCostUSD:          8.0,
		CallerJurisdictions: []string{"US"},
	}

	bRaw := CanonicalHardwareExperimentBinding("sess-1", base)

	cdr := base
	cdr.Mitigation = "CDR"
	cdr.CDRTrainingCircuits = 12
	cdr.LiveReadoutCal = true
	cdr.SymmetryVerification = true
	cdr.TargetShots = []int{65536, 16384}

	bCDR := CanonicalHardwareExperimentBinding("sess-1", cdr)
	if bRaw == bCDR {
		t.Fatalf("expected CDR params to change canonical mandate binding")
	}

	cdrDiff := cdr
	cdrDiff.TargetShots = []int{32768, 16384}
	if CanonicalHardwareExperimentBinding("sess-1", cdrDiff) == bCDR {
		t.Fatalf("expected target_shots change to alter canonical mandate binding")
	}

	// Runner validation refuses out-of-range cdr_training_circuits, target_shots, or CDR on non-VQE jobs.
	r := &Runner{}
	badCount := cdr
	badCount.CDRTrainingCircuits = 3
	raw, _ := json.Marshal(badCount)
	if _, _, err := r.planHardwareExperiment("t1", "s1", "hardware_experiment", raw); err == nil || !strings.Contains(err.Error(), "cdr_training_circuits") {
		t.Fatalf("expected cdr_training_circuits refusal, got %v", err)
	}

	badShots := cdr
	badShots.TargetShots = []int{256, 16384}
	raw, _ = json.Marshal(badShots)
	if _, _, err := r.planHardwareExperiment("t1", "s1", "hardware_experiment", raw); err == nil || !strings.Contains(err.Error(), "target_shots") {
		t.Fatalf("expected target_shots refusal, got %v", err)
	}

	badType := cdr
	badType.Model = "tfim"
	badType.NSites = 2
	badType.StateMode = "vqe"
	badType.TargetError = 0.05
	badType.EnergyExact = -2.0
	badType.PauliCoefficients = map[string]float64{"ZZ": -1.0, "XI": -1.0}
	raw, _ = json.Marshal(badType)
	if _, _, err := r.planHardwareExperiment("t1", "s1", "spin_chain_hardware", raw); err == nil || !strings.Contains(err.Error(), "CDR mitigation is supported") {
		t.Fatalf("expected spin_chain_hardware CDR refusal, got %v", err)
	}
}
