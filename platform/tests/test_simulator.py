"""Unit tests for the local statevector simulator."""

import pytest

from kubona.circuits import parse_qasm2
from kubona.providers import get_provider, ProviderError


def test_bell_state():
    qasm = "OPENQASM 2.0;\nqreg q[2];\nh q[0];\ncx q[0],q[1];"
    circuit = parse_qasm2(qasm)
    sim = get_provider("local-simulator")
    res = sim.submit(circuit, shots=2000, seed=42)
    assert set(res["counts"]) <= {"00", "11"}
    assert res["counts"].get("00", 0) + res["counts"].get("11", 0) == 2000
    assert abs(res["counts"].get("00", 0) - res["counts"].get("11", 0)) < 200  # ~50/50


def test_deterministic_seed():
    qasm = "OPENQASM 2.0;\nqreg q[1];\nh q[0];\nrz(0.7) q[0];\nh q[0];"
    a = get_provider("local-simulator").submit(parse_qasm2(qasm), 1000, seed=1)
    b = get_provider("local-simulator").submit(parse_qasm2(qasm), 1000, seed=1)
    assert a["counts"] == b["counts"]


def test_unsupported_gate_rejected():
    qasm = "OPENQASM 2.0;\nqreg q[1];\nfoo q[0];"
    with pytest.raises(ProviderError):
        get_provider("local-simulator").submit(parse_qasm2(qasm), 10)
