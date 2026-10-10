"""Fidelity gate wiring (Q-07): predict-then-admit beside the money rail."""

from kubona.mcp.servers import execution as exec_server

BELL = "OPENQASM 2.0;\nqreg q[2];\nh q[0];\ncx q[0],q[1];"


def _deep_qasm(twoq=200):
    lines = ["OPENQASM 2.0;", "qreg q[2];", "h q[0];"]
    lines += ["cx q[0],q[1];"] * twoq
    return "\n".join(lines)


def test_simulator_skips_fidelity_gate():
    out = exec_server.submit_job(BELL, provider="local-simulator", shots=100)
    assert out["ok"] is True, out


def test_shallow_qpu_circuit_admitted_with_verdict():
    out = exec_server.submit_job(BELL, provider="ibm-qiskit", shots=100,
                                     budget_ceiling_usd=150.0)
    assert out["ok"] is True, out


def test_deep_circuit_refused_with_receipt_not_remediation():
    out = exec_server.submit_job(_deep_qasm(), provider="ibm-qiskit", shots=100,
                                     budget_ceiling_usd=150.0)
    assert out["ok"] is False, out
    assert "fidelity" in out and out["fidelity"]["fidelity_receipt"], out
    assert out["fidelity"]["recommended_action"] in (
        "DOWNGRADE_TO_AER_SIMULATOR", "APPLY_ZNE_MITIGATION"), out


def test_unknown_device_refuses_as_unpredictable():
    out = exec_server.submit_job(BELL, provider="quantinuum", shots=100,
                                     budget_ceiling_usd=150.0)
    assert out["ok"] is False, out
    assert "no fidelity profile" in out["error"], out
