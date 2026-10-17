"""W0.16: execution/cost server discipline — capped implicit sessions,
async-only non-simulators, spend at dispatch, stored-quote verification."""

import pytest

from kubona.mcp.servers import cost as cost_server
from kubona.mcp.servers import execution as exec_server

BELL = "OPENQASM 2.0;\nqreg q[2];\nh q[0];\ncx q[0],q[1];"


def test_implicit_session_cap_refuses_self_granted_budget():
    out = exec_server.submit_job(BELL, provider="local-simulator", shots=64,
                                 budget_ceiling_usd=1000000.0)
    assert out["ok"] is False and "capped" in out["error"]
    # explicit sessions (cost:manage path) carry larger budgets fine
    sess = cost_server.create_session("big", "researcher", budget_ceiling_usd=500.0)
    out = exec_server.submit_job(BELL, provider="local-simulator", shots=64,
                                 session_id=sess["session_id"])
    assert out["ok"] is True, out


def test_non_simulator_forced_async():
    sess = cost_server.create_session("qpu-async", "researcher", budget_ceiling_usd=500.0)
    out = exec_server.submit_job(BELL, provider="ibm-qiskit", shots=100,
                                 session_id=sess["session_id"], wait=True,
                                 caller_jurisdictions=["US"])
    assert out["ok"] is True, out
    assert out["status"] in ("queued", "running"), out
    assert "async" in out.get("note", ""), out


def test_unlisted_local_claim_refuses():
    out = exec_server.submit_job(BELL, provider="local-simulator", shots=64)
    assert out["ok"] is True, out  # allowlisted local-only: fine


def test_shots_and_timeout_validated():
    for bad_shots in (0, -10):
        out = exec_server.submit_job(BELL, provider="local-simulator", shots=bad_shots)
        assert out["ok"] is False, bad_shots
    out = exec_server.submit_job(BELL, provider="local-simulator", shots=64,
                                 wait_timeout_s=0)
    assert out["ok"] is False


def test_verify_payment_uses_stored_quote():
    sess = cost_server.create_session("pay", "researcher", budget_ceiling_usd=100.0)
    sid = sess["session_id"]
    # no stored quote: refuse even with a "proof"
    out = cost_server.verify_payment(sid, "x402:anything", 0.25)
    assert out["ok"] is False and "no stored quote" in out["error"]
    q = cost_server.quote_payment(sid, 0.25)
    assert q["ok"] is True
    # mismatched amount refuses (the old code re-quoted caller input)
    out = cost_server.verify_payment(sid, "x402:anything", 0.01)
    assert out["ok"] is False and "does not match" in out["error"]
    # stub rail still fails closed (no verifier configured)
    out = cost_server.verify_payment(sid, "x402:anything", 0.25)
    assert out["ok"] is False


def test_zne_args_validated():
    out = exec_server.run_zne_job(BELL, scales="1,999999")
    assert out["ok"] is False
    out = exec_server.run_zne_job(BELL, scales="a,b")
    assert out["ok"] is False
    out = exec_server.run_zne_job(BELL, shots=0)
    assert out["ok"] is False
    out = exec_server.run_zne_job(BELL, noise_rate=2.0)
    assert out["ok"] is False
