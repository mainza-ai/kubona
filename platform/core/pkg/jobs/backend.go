package jobs

// B1 — backend ids are a closed set. A proposal is minted with a backend the
// server recognizes; launch never re-derives one from model prose. The
// classifier is ordered: exact simulator ids first, then anything that
// resolves to a calibrated hardware profile, then simulator-smelling free
// text (models write "Local Simulator (qiskit-aer-simulator)"), and nothing
// else. Provider-prefixed names are never demoted to "$0 simulator" by a
// substring: "ibm_simulator" resolves to the IBM profile and stays metered.

import (
	"fmt"
	"strings"

	"github.com/mainza-ai/kubona/core/pkg/fidelity"
)

// simulatorSmells are the free-text tier words models use for zero-cost local
// execution. They are matched only after the exact set and the hardware
// profile map, so a known provider name can never be demoted by a smell.
var simulatorSmells = []string{
	"local", "simulator", "statevector", "aer", "sim-first", "sim first",
	"starter", "shot-based", "numpy",
}

// NormalizeBackend returns the canonical backend id stored in a proposal.
// Unknown names refuse: an unrecognized backend has no cost model and no
// calibration basis, so it cannot be priced, routed, or approved honestly.
func NormalizeBackend(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", fmt.Errorf("backend required: a proposal must name where it runs")
	}
	if isSimulatorBackend(trimmed) {
		return "local_simulator", nil
	}
	if _, err := fidelity.ResolveDeviceForBackend(trimmed); err == nil {
		return trimmed, nil
	}
	lower := strings.ToLower(trimmed)
	for _, smell := range simulatorSmells {
		if strings.Contains(lower, smell) {
			return "local_simulator", nil
		}
	}
	return "", fmt.Errorf("backend %q is not a recognized simulator or calibrated hardware target: refusing to propose an unroutable, unpriced run", trimmed)
}
