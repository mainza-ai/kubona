"""Compiler portfolio: selection, receipts, evidence tiers."""

from kubona.circuits import parse_qasm2
from kubona.qir.portfolio import circuit_features, compile, select_passes

BELL = "OPENQASM 2.0;\nqreg q[2];\nh q[0];\ncx q[0],q[1];"
T_CIRCUIT = ("OPENQASM 2.0;\nqreg q[2];\nh q[0];\nt q[0];\n"
             "cx q[0],q[1];\nt q[1];")
U_CIRCUIT = ("OPENQASM 2.0;\nqreg q[1];\nu3(0.5,0.2,0.1) q[0];\nh q[0];")


def test_bell_needs_no_passes():
    out = compile(BELL)
    assert out["ok"] is True
    assert out["passes"] == []
    assert out["metrics_before"] == out["metrics_after"]


def test_t_circuit_gets_fragile_with_measured_evidence():
    out = compile(T_CIRCUIT)
    assert out["passes"] == ["fragile_late"]
    rec = out["pass_records"][0]
    assert rec["evidence"]["kind"] == "T1_VERIFIED"
    assert out["metrics_after"]["first_nonclifford"] >= \
        out["metrics_before"]["first_nonclifford"]
    # Receipt hashes chain input -> pass -> output.
    assert rec["output_hash"] != out["input_hash"]
    # Output re-parses and re-compiles stably (idempotent portfolio).
    again = compile(out["output_qasm"])
    assert again["metrics_after"] == out["metrics_after"]


def test_u_circuit_decomposes_first():
    out = compile(U_CIRCUIT)
    assert out["passes"][0] == "decompose_u2_u3"
    assert "u3" not in out["output_qasm"]


def test_noise_context_recorded_not_consumed():
    out = compile(T_CIRCUIT, two_qubit_error=0.0035)
    assert out["noise_context"]["two_qubit_error"] == 0.0035
    assert "no mapping pass" in out["noise_context"]["consumed_by"]


def test_invalid_qasm_refuses_before_passes():
    import pytest as _pytest
    with _pytest.raises(Exception):
        compile("this is not qasm")


def test_select_passes_rules():
    assert select_passes({"needs_decompose": False, "n_nonclifford": 0}) == []
    assert select_passes({"needs_decompose": True, "n_nonclifford": 0}) == ["decompose_u2_u3"]
    assert select_passes({"needs_decompose": False, "n_nonclifford": 3}) == ["fragile_late"]
    feats = circuit_features(parse_qasm2(T_CIRCUIT))
    assert feats["n_nonclifford"] == 2 and feats["needs_decompose"] is False
