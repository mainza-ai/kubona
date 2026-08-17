"""ZNE mitigation tests: noise degrades, ZNE recovers."""

import math

from kubona.circuits import parse_qasm2
from kubona.mitigation import run_zne

# Prepare |1> on q0: <Z> should be exactly -1 at zero noise.
PREP_ONE = "OPENQASM 2.0;\nqreg q[1];\nx q[0];"


def test_zne_improves_over_raw():
    circuit = parse_qasm2(PREP_ONE)
    result = run_zne(circuit, shots=20000, noise_rate=0.05, scales=(1, 3, 5), seed=11)
    raw_err = abs(result.raw_value - (-1.0))
    mit_err = abs(result.mitigated_value - (-1.0))
    assert raw_err > 0.05, f"noise should degrade raw: {result.raw_value}"
    assert mit_err < raw_err, f"ZNE should recover: raw={result.raw_value} mit={result.mitigated_value}"
    assert result.technique == "ZNE" and result.extrapolator == "Richardson"


def test_zne_exact_without_noise():
    circuit = parse_qasm2(PREP_ONE)
    result = run_zne(circuit, shots=100000, noise_rate=0.0, seed=1)
    assert abs(result.raw_value - (-1.0)) < 0.02
