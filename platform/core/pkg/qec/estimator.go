package qec

import (
	"errors"
	"math"
)

// QECArchitecture represents the target error-corrected architecture
type QECArchitecture string

const (
	ArchNeutralAtomQuEra      QECArchitecture = "NEUTRAL_ATOM_QUERA"
	ArchBivariateBicycleIBM   QECArchitecture = "BIVARIATE_BICYCLE_QLDPC_IBM"
	ArchRotatedSurfaceCode    QECArchitecture = "ROTATED_SURFACE_CODE_SUPERCONDUCTING"
)

// QECResourceSpec defines input logical algorithm requirements
type QECResourceSpec struct {
	NumLogicalQubits int     `json:"num_logical_qubits"`
	LogicalTCount    int     `json:"logical_t_count"`
	LogicalClifford  int     `json:"logical_clifford_count"`
	TargetLogicalInf float64 `json:"target_logical_infidelity"` // e.g. 1e-6
	PhysicalErrorRate float64 `json:"physical_error_rate"`       // e.g. 1e-3
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
	EstExecutionTimeMs    float64         `json:"est_execution_time_ms"`
}

// ResourceEstimator computes early fault-tolerant quantum requirements
type ResourceEstimator struct{}

func NewResourceEstimator() *ResourceEstimator {
	return &ResourceEstimator{}
}

// EstimateNeutralAtom computes physical requirements on Harvard/QuEra neutral-atom architecture (Bluvstein et al. 2024)
func (e *ResourceEstimator) EstimateNeutralAtom(spec QECResourceSpec) (*QECResourceEstimate, error) {
	if spec.NumLogicalQubits <= 0 {
		return nil, errors.New("num_logical_qubits must be positive")
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
	p := spec.PhysicalErrorRate
	if p <= 0 {
		p = 0.001
	}

	pLogical := p * math.Pow(p/threshold, float64(distance-1)/2.0)

	return &QECResourceEstimate{
		Architecture:          ArchNeutralAtomQuEra,
		PhysicalQubitsTotal:   totalPhysical,
		DataQubits:            dataQubits,
		SyndromeQubits:        syndromeQubits,
		CodeDistance:          distance,
		PhysicalQubitOverhead: float64(totalPhysical) / float64(spec.NumLogicalQubits),
		SuppressedLogicalErr:  pLogical,
		Threshold:             threshold,
		MeetsThreshold:        p < threshold,
		EstExecutionTimeMs:    float64(spec.LogicalClifford+spec.LogicalTCount) * 0.1, // ~10 kHz shuttle + gate
	}, nil
}

// EstimateBivariateBicycle computes requirements under IBM [[144, 12, 12]] Gross Code (Bravyi et al. 2024)
func (e *ResourceEstimator) EstimateBivariateBicycle(spec QECResourceSpec) (*QECResourceEstimate, error) {
	if spec.NumLogicalQubits <= 0 {
		return nil, errors.New("num_logical_qubits must be positive")
	}

	// BB Gross code packages 12 logical qubits per block of 288 physical qubits (144 data + 144 ancillae)
	blocksNeeded := int(math.Ceil(float64(spec.NumLogicalQubits) / 12.0))
	dataQubits := blocksNeeded * 144
	syndromeQubits := blocksNeeded * 144
	totalPhysical := dataQubits + syndromeQubits
	distance := 12
	threshold := 0.0075 // 0.75% threshold

	p := spec.PhysicalErrorRate
	if p <= 0 {
		p = 0.001
	}

	// Logical error suppression: ~ 0.1 * (p / p_th)^6 for distance 12
	ratio := p / threshold
	pLogical := 0.1 * math.Pow(ratio, 6.0)

	return &QECResourceEstimate{
		Architecture:          ArchBivariateBicycleIBM,
		PhysicalQubitsTotal:   totalPhysical,
		DataQubits:            dataQubits,
		SyndromeQubits:        syndromeQubits,
		CodeDistance:          distance,
		PhysicalQubitOverhead: float64(totalPhysical) / float64(spec.NumLogicalQubits),
		SuppressedLogicalErr:  pLogical,
		Threshold:             threshold,
		MeetsThreshold:        p < threshold,
		EstExecutionTimeMs:    float64(spec.LogicalClifford+spec.LogicalTCount) * 0.002, // ~500 kHz transmon rate
	}, nil
}
