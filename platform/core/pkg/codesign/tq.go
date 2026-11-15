package codesign

import (
	"fmt"
	"math"
)

// TQ scoring — Go serving mirror of platform/kubona/codesign/tq.py.
// The Python evaluator is ground truth for validity; this package serves
// read-only TQ arithmetic + frontier cells to the console. Both sides pin
// the same golden (5T x 10Q = 50) so drift fails tests on both sides.
const (
	KappaDefault = 1
	KappaMin     = 1
	KappaMax     = 3
)

// Cost returns T * Q^kappa, fail-closed on bad inputs.
func Cost(toffoli, qubits, kappa int) (int64, error) {
	if toffoli < 0 {
		return 0, fmt.Errorf("toffoli must be >= 0, got %d: refusing", toffoli)
	}
	if qubits < 0 {
		return 0, fmt.Errorf("qubits must be >= 0, got %d: refusing", qubits)
	}
	if kappa < KappaMin || kappa > KappaMax {
		return 0, fmt.Errorf("kappa must be in [%d,%d], got %d: refusing", KappaMin, KappaMax, kappa)
	}
	qp := int64(1)
	q64 := int64(qubits)
	for i := 0; i < kappa; i++ {
		if q64 > 0 && qp > math.MaxInt64/q64 {
			return 0, fmt.Errorf("tq cost overflows int64 for qubits=%d kappa=%d", qubits, kappa)
		}
		qp *= q64
	}
	t64 := int64(toffoli)
	if qp > 0 && t64 > math.MaxInt64/qp {
		return 0, fmt.Errorf("tq cost overflows int64 for toffoli=%d qubits=%d kappa=%d", toffoli, qubits, kappa)
	}
	return t64 * qp, nil
}
