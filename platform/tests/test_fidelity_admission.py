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
    # W0.16: budgets above the implicit cap travel on explicit sessions
    # (opened via cost/create_session), never self-granted implicitly.
    from kubona.mcp.servers import cost as cost_server
    sess = cost_server.create_session("qpu-shallow", "researcher", budget_ceiling_usd=150.0)
    out = exec_server.submit_job(BELL, provider="ibm-qiskit", shots=100,
                                 session_id=sess["session_id"],
                                 caller_jurisdictions=["US"])
    assert out["ok"] is True, out
    assert out["status"] in ("queued", "running", "completed"), out


def test_deep_circuit_refused_with_receipt_not_remediation():
    out = exec_server.submit_job(_deep_qasm(), provider="ibm-qiskit", shots=100,
                                 budget_ceiling_usd=25.0, caller_jurisdictions=["US"])
    assert out["ok"] is False, out
    assert "fidelity" in out and out["fidelity"]["fidelity_receipt"], out
    assert out["fidelity"]["recommended_action"] in (
        "DOWNGRADE_TO_AER_SIMULATOR", "APPLY_ZNE_MITIGATION"), out


def test_unknown_device_refuses_as_unpredictable():
    # Unpriced hardware (quantinuum ships 0.0 rates) refuses at preflight:
    # a $0.00 estimate would read as "free tier" downstream, so the money
    # rail fails closed before the fidelity gate is even reached.
    out = exec_server.submit_job(BELL, provider="quantinuum", shots=100,
                                 budget_ceiling_usd=25.0, caller_jurisdictions=["US", "UK"],
                                 caller_export_class="allied")
    assert out["ok"] is False, out
    assert "no task/shot rate configured" in out["error"], out


def test_fidelity_gate_refuses_priced_but_unprofiled_backend():
    # The fidelity no-profile branch stays pinned even though no real
    # provider currently reaches it: a priced ibm-clone (rates intact, no
    # device mapping) must refuse as unpredictable, never inherit a profile.
    from kubona.circuits import parse_qasm2
    from kubona.schema import load_provider
    cap = load_provider("ibm-qiskit").model_copy(update={"provider": "ibm-clone"})
    verdict = exec_server._fidelity_gate(cap, parse_qasm2(BELL))
    assert verdict["admitted"] is False, verdict
    assert "no fidelity profile" in verdict["rejection_reason"], verdict
