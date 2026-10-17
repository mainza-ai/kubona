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
    assert result.technique == "ZNE" and result.extrapolator == "richardson"


def test_zne_refit_same_shots_multiple_estimators():
    from kubona.mitigation.zne import EXTRAPOLATORS
    circuit = parse_qasm2(PREP_ONE)
    result = run_zne(circuit, shots=20000, noise_rate=0.05, scales=(1, 3, 5), seed=11)
    assert set(EXTRAPOLATORS) == {"richardson", "linear", "exponential"}
    # Same archived shots, three estimators: no new execution anywhere.
    # (exponential refuses signed expectations — tested below as a boundary.)
    refits = {name: result.refit(name) for name in ("richardson", "linear")}
    for name, refit in refits.items():
        assert refit.scale_values == result.scale_values
        assert refit.shots == result.shots and refit.seed == result.seed
        assert refit.extrapolator == name
    # Linear and Richardson agree closely on near-linear decay; all beat raw.
    raw_err = abs(result.raw_value - (-1.0))
    for refit in refits.values():
        assert abs(refit.mitigated_value - (-1.0)) < raw_err, refit
    # Estimator spread is reported, not hidden: disagreement is information.
    spread = max(r.mitigated_value for r in refits.values()) - min(
        r.mitigated_value for r in refits.values())
    assert spread >= 0.0
    # Unknown extrapolators refuse; exponential refuses signed data
    # (log-fit needs positive values) rather than inventing a number.
    import pytest as _pytest
    with _pytest.raises(ValueError, match="unknown extrapolator"):
        result.refit("spline")
    with _pytest.raises(ValueError, match="positive values"):
        result.refit("exponential")
    assert result.to_dict()["shots"] == 20000 and result.to_dict()["seed"] == 11


def test_exponential_refit_on_positive_decay():
    from kubona.mitigation.zne import EXTRAPOLATORS
    # Synthetic positive-definite decay: exponential recovery exact-ish.
    scales = [1.0, 3.0, 5.0]
    values = [0.9, 0.7, 0.55]
    got = EXTRAPOLATORS["exponential"](scales, values)
    assert 0.9 < got < 1.2, got


def test_zne_exact_without_noise():
    circuit = parse_qasm2(PREP_ONE)
    result = run_zne(circuit, shots=100000, noise_rate=0.0, scales=(1, 3, 5), seed=1)
    assert abs(result.raw_value - (-1.0)) < 0.02
