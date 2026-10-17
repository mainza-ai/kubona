package qec

import (
	"errors"
	"math"
)

// QECArchitecture represents the target error-corrected architecture
type QECArchitecture string

const (
	ArchNeutralAtomQuEra    QECArchitecture = "NEUTRAL_ATOM_QUERA"
	ArchBivariateBicycleIBM QECArchitecture = "BIVARIATE_BICYCLE_QLDPC_IBM"
	ArchRotatedSurfaceCode  QECArchitecture = "ROTATED_SURFACE_CODE_SUPERCONDUCTING"
)

// QECResourceSpec defines input logical algorithm requirements
type QECResourceSpec struct {
	NumLogicalQubits  int     `json:"num_logical_qubits"`
	LogicalTCount     int     `json:"logical_t_count"`
	LogicalClifford   int     `json:"logical_clifford_count"`
	TargetLogicalInf  float64 `json:"target_logical_infidelity"` // e.g. 1e-6
	PhysicalErrorRate float64 `json:"physical_error_rate"`       // e.g. 1e-3
	// AcceptanceRate: fraction of shots kept under error detection +
	// postselection. <= 0 means unspecified (no postselection, multiplier 1).
	// A gain quoted without its acceptance rate is not a scaling result.
	AcceptanceRate float64 `json:"acceptance_rate"`
}

// QECResourceEstimate holds computed physical hardware requirements
type QECResourceEstimate struct {
	Architecture          QECArchitecture `json:"architecture"`
	PhysicalQubitsTotal   int             `json:"physical_qubits_total"`
	DataQubits            int             `json:"data_qubits"`
	SyndromeQubits        int             `json:"syndrome_qubits"`
	CodeDistance          int             `json:"code_distance"`
	PhysicalQubitOverhead float64         `json:"physical_qubit_overhead_ratio"`
	SuppressedLogicalErr  float64         `json:"suppressed_logical_error_rate"`
	Threshold             float64         `json:"threshold"`
	MeetsThreshold        bool            `json:"meets_threshold"`
	// MeetsTarget: suppressed error at or below the requested target
	// infidelity (W1.11). False when no positive target was set.
	MeetsTarget        bool    `json:"meets_target"`
	EstExecutionTimeMs float64 `json:"est_execution_time_ms"`
	// EffectiveShotsMultiplier: 1/acceptance for postselected estimates
	// (1.0 when no postselection). Multiply shot budgets by this.
	EffectiveShotsMultiplier float64 `json:"effective_shots_multiplier"`
}

// MeetsTarget reports whether the suppressed logical error meets the
// caller's target infidelity (W1.11): TargetLogicalInf was stored but never
// compared, so callers believed the target met when it was unchecked.
// Compared at estimate time; false when no positive target is set.
func (e *QECResourceEstimate) checkTarget(target float64) {
	e.MeetsTarget = target > 0 && e.SuppressedLogicalErr <= target
}

// ResourceEstimator computes early fault-tolerant quantum requirements
type ResourceEstimator struct{}

func NewResourceEstimator() *ResourceEstimator {
	return &ResourceEstimator{}
}

// validateSpec refuses unphysical inputs (W1.11): negative counts used to
// produce negative execution times, and NaN/Inf sailed through every check.
func validateSpec(spec QECResourceSpec) (float64, error) {
	if spec.NumLogicalQubits <= 0 {
		return 0, errors.New("num_logical_qubits must be positive")
	}
	if spec.LogicalTCount < 0 || spec.LogicalClifford < 0 {
		return 0, errors.New("logical gate counts must be non-negative (negative counts priced negative time)")
	}
	if math.IsNaN(spec.PhysicalErrorRate) || math.IsInf(spec.PhysicalErrorRate, 0) {
		return 0, errors.New("physical_error_rate must be finite: refusing")
	}
	// W1.11: a missing error rate used to silently become 0.001, inventing
	// physics. Non-positive rates now refuse — the caller states the device.
	if spec.PhysicalErrorRate <= 0 {
		return 0, errors.New("physical_error_rate must be positive: refusing to invent device physics")
	}
	return spec.PhysicalErrorRate, nil
}

// acceptanceMultiplier prices postselection: 0 means unspecified
// (multiplier 1); anything else must validate (W1.11: negatives used to
// hide as 1.0, masking invalid input).
func acceptanceMultiplier(rate float64) (float64, error) {
	if rate == 0 {
		return 1.0, nil
	}
	return EffectiveCostMultiplier(rate)
}

// EstimateNeutralAtom computes physical requirements on Harvard/QuEra neutral-atom architecture (Bluvstein et al. 2024)
func (e *ResourceEstimator) EstimateNeutralAtom(spec QECResourceSpec) (*QECResourceEstimate, error) {
	p, err := validateSpec(spec)
	if err != nil {
		return nil, err
	}
	if spec.NumLogicalQubits > 48 {
		return nil, errors.New("exceeds neutral-atom logical capacity (max 48 logical qubits, 280 physical atoms)")
	}

	// Neutral atom color codes / transversal gates: ~6 physical atoms per logical qubit at distance 3
	distance := 3
	atomsPerLogical := 6
	dataQubits := spec.NumLogicalQubits * atomsPerLogical
	syndromeQubits := spec.NumLogicalQubits * 2
	totalPhysical := dataQubits + syndromeQubits

	threshold := 0.01 // ~1% neutral-atom tweezer threshold

	pLogical := p * math.Pow(p/threshold, float64(distance-1)/2.0)

	multiplier, err := acceptanceMultiplier(spec.AcceptanceRate)
	if err != nil {
		return nil, err
	}

	est := &QECResourceEstimate{
		Architecture:             ArchNeutralAtomQuEra,
		PhysicalQubitsTotal:      totalPhysical,
		DataQubits:               dataQubits,
		SyndromeQubits:           syndromeQubits,
		CodeDistance:             distance,
		PhysicalQubitOverhead:    float64(totalPhysical) / float64(spec.NumLogicalQubits),
		SuppressedLogicalErr:     pLogical,
		Threshold:                threshold,
		MeetsThreshold:           p < threshold,
		EstExecutionTimeMs:       float64(spec.LogicalClifford+spec.LogicalTCount) * 0.1, // ~10 kHz shuttle + gate
		EffectiveShotsMultiplier: multiplier,
	}
	est.checkTarget(spec.TargetLogicalInf)
	return est, nil
}

// EstimateBivariateBicycle computes requirements under IBM [[144, 12, 12]] Gross Code (Bravyi et al. 2024)
func (e *ResourceEstimator) EstimateBivariateBicycle(spec QECResourceSpec) (*QECResourceEstimate, error) {
	p, err := validateSpec(spec)
	if err != nil {
		return nil, err
	}
	// v1 capacity bound (blocks scale linearly without one): 1200 logical
	// qubits ≈ 100 blocks ≈ 29k physical. Labeled heuristic, not physics.
	if spec.NumLogicalQubits > 1200 {
		return nil, errors.New("exceeds bivariate-bicycle planning capacity (max 1200 logical qubits)")
	}

	// BB Gross code packages 12 logical qubits per block of 288 physical qubits (144 data + 144 ancillae)
	blocksNeeded := int(math.Ceil(float64(spec.NumLogicalQubits) / 12.0))
	dataQubits := blocksNeeded * 144
	syndromeQubits := blocksNeeded * 144
	totalPhysical := dataQubits + syndromeQubits
	distance := 12
	threshold := 0.0075 // 0.75% threshold

	// Logical error suppression: ~ 0.1 * (p / p_th)^6 for distance 12
	ratio := p / threshold
	pLogical := 0.1 * math.Pow(ratio, 6.0)

	multiplier, err := acceptanceMultiplier(spec.AcceptanceRate)
	if err != nil {
		return nil, err
	}

	est := &QECResourceEstimate{
		Architecture:             ArchBivariateBicycleIBM,
		PhysicalQubitsTotal:      totalPhysical,
		DataQubits:               dataQubits,
		SyndromeQubits:           syndromeQubits,
		CodeDistance:             distance,
		PhysicalQubitOverhead:    float64(totalPhysical) / float64(spec.NumLogicalQubits),
		SuppressedLogicalErr:     pLogical,
		Threshold:                threshold,
		MeetsThreshold:           p < threshold,
		EstExecutionTimeMs:       float64(spec.LogicalClifford+spec.LogicalTCount) * 0.002, // ~500 kHz transmon rate
		EffectiveShotsMultiplier: multiplier,
	}
	est.checkTarget(spec.TargetLogicalInf)
	return est, nil
}
