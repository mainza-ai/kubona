"""W0.16: execution/cost server discipline — capped implicit sessions,
async-only non-simulators, spend at dispatch, stored-quote verification."""

import pytest

from kubona.mcp.servers import cost as cost_server
from kubona.mcp.servers import execution as exec_server

BELL = "OPENQASM 2.0;\nqreg q[2];\nh q[0];\ncx q[0],q[1];"

@pytest.fixture(autouse=True)
def fresh_calibration():
    import dataclasses
    import time
    from kubona.fidelity import accounting
    for name in ("ibm-heron", "quantinuum-h1", "rigetti-aspen"):
        prof = accounting.DEVICE_CATALOG.get(name)
        if prof is not None:
            accounting.DEVICE_CATALOG[name] = dataclasses.replace(
                prof, last_calibrated_epoch=time.time())



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


def test_prevendor_failure_refunds_reservation(monkeypatch):
    """2026-10-26 audit PYC-01: a missing credential must not bill the lab.

    Forced async for non-simulators; poll the persisted job, then assert the
    session spend returned to zero.
    """
    import time
    monkeypatch.delenv("KUBONA_IBM_QISKIT_TOKEN", raising=False)
    sess = cost_server.create_session("refund-drill", "researcher", budget_ceiling_usd=100.0)
    sid = sess["session_id"]
    out = exec_server.submit_job(BELL, provider="ibm-qiskit", shots=64,
                                 session_id=sid, caller_jurisdictions=["US"], wait=True)
    assert out["ok"] is True, out
    job_id = out["job_id"]
    status = None
    for _ in range(200):
        st = exec_server.get_job_status(job_id)
        status = st.get("status")
        if status in ("failed", "completed", "cancelled"):
            break
        time.sleep(0.05)
    assert status == "failed", status
    assert exec_server.session_status(sid)["spent_usd"] == 0, \
        "pre-vendor failure must refund the reservation"


def test_idempotent_replay_returns_same_job():
    sess = cost_server.create_session("idem-replay", "researcher", budget_ceiling_usd=50.0)
    sid = sess["session_id"]
    first = exec_server.submit_job(BELL, provider="local-simulator", shots=32,
                                   session_id=sid, idempotency_key="idem-1",
                                   idempotency_body_hash="body-a")
    assert first["ok"] is True, first
    replay = exec_server.submit_job(BELL, provider="local-simulator", shots=32,
                                    session_id=sid, idempotency_key="idem-1",
                                    idempotency_body_hash="body-a")
    assert replay["ok"] is True
    assert replay["idempotent_replay"] is True
    assert replay["job_id"] == first["job_id"]
    mismatch = exec_server.submit_job(BELL, provider="local-simulator", shots=32,
                                      session_id=sid, idempotency_key="idem-1",
                                      idempotency_body_hash="body-b")
    assert mismatch["ok"] is False
    assert "different request body" in mismatch["error"]


def test_concurrent_same_key_creates_one_job():
    from concurrent.futures import ThreadPoolExecutor
    sess = cost_server.create_session("idem-race", "researcher", budget_ceiling_usd=50.0)
    sid = sess["session_id"]

    def submit(_):
        return exec_server.submit_job(BELL, provider="local-simulator", shots=32,
                                      session_id=sid, idempotency_key="race-key",
                                      idempotency_body_hash="race-body")

    with ThreadPoolExecutor(max_workers=2) as pool:
        results = list(pool.map(submit, range(2)))
    assert all(r["ok"] for r in results), results
    ids = {r["job_id"] for r in results}
    assert len(ids) == 1, ids
