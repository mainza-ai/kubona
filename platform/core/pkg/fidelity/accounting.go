package fidelity

import (
	"fmt"
	"math"
	"sync"
)

type AdmissionDecision string

const (
	DecisionAdmit     AdmissionDecision = "ADMIT"
	DecisionDowngrade AdmissionDecision = "DOWNGRADE_TO_SIMULATOR"
	DecisionMitigate  AdmissionDecision = "APPLY_MITIGATION"
	DecisionRefuse    AdmissionDecision = "REFUSE"
)

type DeviceProfile struct {
	Name           string  `json:"name"`
	Qubits         int     `json:"qubits"`
	T1Us           float64 `json:"t1_us"`
	T2Us           float64 `json:"t2_us"`
	SingleQubitErr float64 `json:"single_qubit_error"`
	TwoQubitErr    float64 `json:"two_qubit_error"`
	// W2.5 provenance: hardcoded profiles must say where they came from.
	// A profile without source/version/date refuses to bootstrap.
	Source       string `json:"source"`
	Version      string `json:"version"`
	CalibratedAt string `json:"calibrated_at"`
}

type FidelityAssessment struct {
	DeviceName        string  `json:"device_name"`
	PredictedFidelity float64 `json:"predicted_fidelity"`
	MeasuredClifford  float64 `json:"measured_clifford"`
	// ProbeRun reports whether a Clifford probe measurement actually ran.
	// EvaluateAdmission receives a measurement argument (true); the
	// floor-only path never probes (false) — a zero MeasuredClifford
	// beside probe_run:false reads as "not measured", never as a 0.0
	// probe. T-02: the /api/fidelity handler previously passed a
	// hardcoded 0.94, so ADMIT cited probe alignment no probe performed.
	ProbeRun          bool              `json:"probe_run"`
	Decision          AdmissionDecision `json:"decision"`
	Reason            string            `json:"reason"`
	RecommendedAction string            `json:"recommended_action"`
}

type FidelityGate struct {
	mu             sync.RWMutex
	profiles       map[string]DeviceProfile
	floorThreshold float64
	driftTolerance float64
}

func NewFidelityGate() *FidelityGate {
	fg := &FidelityGate{
		profiles:       make(map[string]DeviceProfile),
		floorThreshold: 0.60, // Minimum acceptable fidelity before hardware spend
		driftTolerance: 0.15, // Maximum allowed discrepancy between predicted and Clifford probe
	}
	fg.bootstrapProfiles()
	return fg
}

// FloorThreshold exposes the admission floor for ledger sealing (one source:
// the gate's own field, never a copied literal).
func (fg *FidelityGate) FloorThreshold() float64 {
	fg.mu.RLock()
	defer fg.mu.RUnlock()
	return fg.floorThreshold
}

// DriftTolerance exposes the probe-drift tolerance for ledger sealing.
func (fg *FidelityGate) DriftTolerance() float64 {
	fg.mu.RLock()
	defer fg.mu.RUnlock()
	return fg.driftTolerance
}

func (fg *FidelityGate) bootstrapProfiles() {
	// W2.5: v1 heuristic device data (order-of-magnitude, labeled). T1/T2
	// ride along for forensics and calibration display; the floor model
	// prices gate errors (PredictFidelity), not coherence — T1/T2 are
	// validated for physicality (T2 <= 2*T1) at bootstrap, never silently
	// consumed.
	for _, p := range []DeviceProfile{
		{Name: "ibm_heron", Qubits: 156,
			T1Us: 180.0, T2Us: 120.0,
			SingleQubitErr: 0.00012, TwoQubitErr: 0.0021,
			Source:  "IBM Quantum backend calibration medians, Heron r2 class",
			Version: "v1-heuristic-2026-10", CalibratedAt: "2026-10-01"},
		{Name: "quantinuum_h1", Qubits: 20,
			T1Us: 1000000.0, T2Us: 1000000.0,
			SingleQubitErr: 0.00003, TwoQubitErr: 0.0015,
			Source:  "Quantinuum H1 published system specs (trapped-ion coherence)",
			Version: "v1-heuristic-2026-10", CalibratedAt: "2026-10-01"},
		{Name: "rigetti_aspen", Qubits: 80,
			T1Us: 35.0, T2Us: 25.0,
			SingleQubitErr: 0.0005, TwoQubitErr: 0.0080,
			Source:  "Rigetti Aspen-class published medians",
			Version: "v1-heuristic-2026-10", CalibratedAt: "2026-10-01"},
	} {
		if p.T2Us > 2*p.T1Us {
			panic(fmt.Sprintf("device profile %s violates T2 <= 2*T1: refusing to boot on unphysical data", p.Name))
		}
		if p.Source == "" || p.Version == "" || p.CalibratedAt == "" {
			panic(fmt.Sprintf("device profile %s lacks provenance: refusing", p.Name))
		}
		fg.profiles[p.Name] = p
	}
}

func (fg *FidelityGate) PredictFidelity(deviceName string, depth, twoQubitGates int) (float64, error) {
	fg.mu.RLock()
	defer fg.mu.RUnlock()

	prof, exists := fg.profiles[deviceName]
	if !exists {
		return 0.0, fmt.Errorf("unknown device: %s", deviceName)
	}

	p1 := math.Pow(1.0-prof.SingleQubitErr, float64(depth))
	p2 := math.Pow(1.0-prof.TwoQubitErr, float64(twoQubitGates))
	return math.Max(0.01, math.Min(1.0, p1*p2)), nil
}

// EvaluateAdmissionFloorOnly applies the floor + known-device rules without a
// Clifford probe result. Used at dispatch admission where no probe has run:
// unknown devices refuse (unpredictable), below-floor refuses or mitigates,
// everything else admits. Probe-gated drift checks belong to paths that
// actually ran a probe (see EvaluateAdmission).
func (fg *FidelityGate) EvaluateAdmissionFloorOnly(deviceName string, depth, twoQubitGates int) FidelityAssessment {
	fPred, err := fg.PredictFidelity(deviceName, depth, twoQubitGates)
	if err != nil {
		return FidelityAssessment{
			DeviceName:        deviceName,
			Decision:          DecisionRefuse,
			Reason:            fmt.Sprintf("no fidelity profile for device: %v (unpredictable — refused, never simulated silently)", err),
			RecommendedAction: "Register a calibrated device profile or route to a simulator backend",
		}
	}
	if fPred < fg.floorThreshold {
		return FidelityAssessment{
			DeviceName:        deviceName,
			PredictedFidelity: fPred,
			Decision:          DecisionMitigate,
			Reason:            fmt.Sprintf("Predicted fidelity %.3f below floor threshold %.3f", fPred, fg.floorThreshold),
			RecommendedAction: "Resubmit explicitly with mitigation (ZNE/cutting) — never applied silently",
		}
	}
	return FidelityAssessment{
		DeviceName:        deviceName,
		PredictedFidelity: fPred,
		Decision:          DecisionAdmit,
		Reason:            "Predicted fidelity meets floor on a calibrated device profile",
		RecommendedAction: "Proceed to spend authorization",
	}
}

func (fg *FidelityGate) EvaluateAdmission(deviceName string, depth, twoQubitGates int, measuredClifford float64) FidelityAssessment {
	fPred, err := fg.PredictFidelity(deviceName, depth, twoQubitGates)
	if err != nil {
		return FidelityAssessment{
			DeviceName:        deviceName,
			ProbeRun:          true,
			Decision:          DecisionRefuse,
			Reason:            err.Error(),
			RecommendedAction: "Aborting job submission due to invalid device profile",
		}
	}

	drift := math.Abs(fPred - measuredClifford)

	// Case 1: Clifford probe shows massive drift from calibration model
	if drift > fg.driftTolerance {
		return FidelityAssessment{
			DeviceName:        deviceName,
			PredictedFidelity: fPred,
			MeasuredClifford:  measuredClifford,
			ProbeRun:          true,
			Decision:          DecisionDowngrade,
			Reason:            fmt.Sprintf("Calibration drift detected: |F_pred (%.3f) - F_probe (%.3f)| = %.3f > tolerance %.3f", fPred, measuredClifford, drift, fg.driftTolerance),
			RecommendedAction: "Downgrade to Aer simulator with noisy snapshot; do not bill client for uncalibrated hardware run",
		}
	}

	// Case 2: Deep circuit fidelity below floor
	if fPred < fg.floorThreshold {
		return FidelityAssessment{
			DeviceName:        deviceName,
			PredictedFidelity: fPred,
			MeasuredClifford:  measuredClifford,
			ProbeRun:          true,
			Decision:          DecisionMitigate,
			Reason:            fmt.Sprintf("Predicted fidelity %.3f below floor threshold %.3f", fPred, fg.floorThreshold),
			RecommendedAction: "Apply PennyLane circuit cutting or Richardson ZNE mitigation to reduce gate depth before dispatching to QPU",
		}
	}

	// Case 3: Admitted
	return FidelityAssessment{
		DeviceName:        deviceName,
		PredictedFidelity: fPred,
		MeasuredClifford:  measuredClifford,
		ProbeRun:          true,
		Decision:          DecisionAdmit,
		Reason:            "Fidelity exceeds floor and aligns with Clifford probe. Hardware execution authorized.",
		RecommendedAction: "Proceed with cryptographic AP2 spend mandate signing",
	}
}
