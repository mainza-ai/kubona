"""QIR reference lowering tests (Python pipeline)."""

from kubona.qir import lower, transpile_to_native
from kubona.circuits import parse_qasm2


def test_lower_emits_qir():
    out = lower("OPENQASM 2.0;\nqreg q[2];\nh q[0];\ncx q[0],q[1];")
    assert out["ok"] and out["num_qubits"] == 2
    assert "__quantum__qis__h__body" in out["qir_text"]
    assert "__quantum__qis__cx__body" in out["qir_text"]
    assert "__quantum__rt__qubit_allocate" in out["qir_text"]


def test_u3_decomposition():
    c = parse_qasm2("OPENQASM 2.0;\nqreg q[1];\nu3(0.5, 0.2, 0.1) q[0];")
    native = transpile_to_native(c)
    names = [g.name for g in native.gates]
    assert names == ["rz", "ry", "rz"]
    assert all(g.name in ("rx", "ry", "rz", "cx", "x", "h") for g in native.gates)
