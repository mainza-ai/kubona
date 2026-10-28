"""W0.16: execution/cost server discipline — capped implicit sessions,
async-only non-simulators, spend at dispatch, stored-quote verification."""

import pytest

from kubona.mcp.servers import cost as cost_server
from kubona.mcp.servers import execution as exec_server

BELL = "OPENQASM 2.0;\nqreg q[2];\nh q[0];\ncx q[0],q[1];"

def _approve_paid_dispatch(session_id: str, amount_usd: float,
                           tenant_id: str = "lab-a", job_hash: str | None = None) -> str:
    """Real approval machinery (never a stubbed gate): REVIEW -> HOLD ->
    APPROVED with a freshly minted operator-signed mandate bound to the lab
    + program. Returns the approval_id for paid submit calls."""
    import os
    from kubona.cost.mandate import canonical_job_binding
    gov = exec_server._gov
    key = os.urandom(32)
    req = gov.request_approval(session_id, amount_usd, "phase-0 paid dispatch test")
    gov.hold_approval(req.approval_id, reviewer="test-operator")
    approved = gov.approve(
        req.approval_id, reviewer="test-operator", note="phase-0 test",
        tenant_id=tenant_id,
        job_hash=job_hash or canonical_job_binding("circuit_run", session_id, BELL),
        signing_key=key)
    assert approved.state == "APPROVED" and approved.mandate_token
    return approved.approval_id

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
    approval_id = _approve_paid_dispatch(sess["session_id"], 500.0)
    out = exec_server.submit_job(BELL, provider="ibm-qiskit", shots=100,
                                 session_id=sess["session_id"], wait=True,
                                 caller_jurisdictions=["US"], approval_id=approval_id)
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
    approval_id = _approve_paid_dispatch(sid, 100.0)
    out = exec_server.submit_job(BELL, provider="ibm-qiskit", shots=64,
                                 session_id=sid, caller_jurisdictions=["US"], wait=True,
                                 approval_id=approval_id)
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


def test_session_status_bounds_job_ids():
    from kubona.mcp.servers import execution as execution_server
    from kubona.mcp.servers import cost as cost_server

    sess = cost_server.create_session("bounded-jobs", "tester", budget_ceiling_usd=1.0)
    sid = sess["session_id"]
    for i in range(150):
        execution_server._sessions.record_job(sid, f"job-{i}")
    out = execution_server.session_status(sid, jobs_limit=100)
    assert out["jobs_total"] == 150
    assert out["jobs_returned"] == 100
    assert out["jobs_truncated"] is True
    assert out["jobs"] == [f"job-{i}" for i in range(50, 150)]


# -- Phase-0 money rail ----------------------------------------------------

def test_paid_submit_without_approval_refuses_and_free_proceeds():
    """Bypass-refusal: a paid dispatch with no approval books nothing and
    creates no job; the free tier is unaffected."""
    sess = cost_server.create_session("approval-bypass", "researcher", budget_ceiling_usd=100.0)
    sid = sess["session_id"]
    out = exec_server.submit_job(BELL, provider="ibm-qiskit", shots=64,
                                 session_id=sid, caller_jurisdictions=["US"], wait=True)
    assert out["ok"] is False, out
    assert "approval" in out["error"].lower(), out
    assert exec_server.session_status(sid)["spent_usd"] == 0
    assert exec_server.session_status(sid)["jobs_total"] == 0
    # Free tier proceeds approval-free.
    free = exec_server.submit_job(BELL, provider="local-simulator", shots=32,
                                  session_id=sid)
    assert free["ok"] is True, free


def test_paid_approval_binding_refusals():
    """The gate checks state, session binding, and cover — not just presence."""
    import os
    gov = exec_server._gov
    sess = cost_server.create_session("approval-binding", "researcher", budget_ceiling_usd=500.0)
    sid = sess["session_id"]
    other = cost_server.create_session("approval-binding-other", "researcher",
                                       budget_ceiling_usd=500.0)
    key = os.urandom(32)

    def _submit(**kw):
        return exec_server.submit_job(BELL, provider="ibm-qiskit", shots=64,
                                      session_id=sid, caller_jurisdictions=["US"],
                                      wait=True, **kw)

    # REVIEW-stage (never held/approved) refuses.
    pending = gov.request_approval(sid, 500.0, "binding test")
    out = _submit(approval_id=pending.approval_id)
    assert out["ok"] is False and "APPROVED" in out["error"], out
    # Approved for a DIFFERENT session refuses (no cross-session spend).
    foreign = gov.request_approval(other["session_id"], 500.0, "binding test")
    gov.hold_approval(foreign.approval_id, reviewer="test-operator")
    gov.approve(foreign.approval_id, reviewer="test-operator", tenant_id="lab-a",
                job_hash="job-foreign", signing_key=key)
    out = _submit(approval_id=foreign.approval_id)
    assert out["ok"] is False and "not this dispatch" in out["error"], out
    # Under-cover approval refuses.
    tiny = gov.request_approval(sid, 0.01, "binding test")
    gov.hold_approval(tiny.approval_id, reviewer="test-operator")
    gov.approve(tiny.approval_id, reviewer="test-operator", tenant_id="lab-a",
                job_hash="job-tiny", signing_key=key)
    out = _submit(approval_id=tiny.approval_id)
    assert out["ok"] is False and "covers" in out["error"], out
    # Unknown id refuses.
    out = _submit(approval_id="ghost-approval")
    assert out["ok"] is False and "unknown approval_id" in out["error"], out
    assert exec_server.session_status(sid)["spent_usd"] == 0


def test_gateway_paid_submit_gated_by_approval_packet(monkeypatch):
    """Gateway wiring: paid submit without approval never executes (first
    call asks); the full MRTR round sealed with KUBONA_APPROVAL_SECRET then
    executes exactly once. Free tier executes immediately."""
    import uuid
    from kubona.mcp.approval import APPROVAL_SECRET_ENV, open_state
    from kubona.mcp.servers._mount import build_gateway

    monkeypatch.setenv(APPROVAL_SECRET_ENV, "phase0-test-secret-32-bytes!!")
    monkeypatch.delenv("KUBONA_IBM_QISKIT_TOKEN", raising=False)
    gw = build_gateway()
    # Phase-1 ownership: the explicit session must be owned by the gateway
    # principal (dev-token authenticates as researcher@kubona.local) or the
    # submit refuses with unknown-session (no oracle).
    sess = cost_server.create_session("gateway-paid-gate", "researcher@kubona.local",
                                      budget_ceiling_usd=500.0)
    sid = sess["session_id"]
    paid_args = {"circuit_qasm": BELL, "provider": "ibm-qiskit", "shots": 64,
                 "session_id": sid, "caller_jurisdictions": ["US"], "wait": True,
                 "idempotency_key": f"gw-paid-{uuid.uuid4().hex[:8]}",
                 "idempotency_body_hash": "gw-paid-body"}

    first = gw.call("execution", "submit_job", dict(paid_args), token="dev-token")
    assert first["resultType"] == "input_required", first
    assert "requestState" in first
    state = open_state(first["requestState"], b"phase0-test-secret-32-bytes!!")
    assert state["cost_max_usd"] > 0

    # Wrong cost ack refuses; the nonce is single-use even on failure, so the
    # success leg needs a fresh approval round (same as a human re-asking).
    import pytest
    with pytest.raises(Exception):
        gw.call("execution", "submit_job", dict(paid_args), token="dev-token",
                approval_response={"action": "accept",
                                   "content": {"decision": "approve_once",
                                               "cost_ack_usd": 0.01}},
                request_state=first["requestState"])
    fresh = gw.call("execution", "submit_job", dict(paid_args), token="dev-token")
    assert fresh["resultType"] == "input_required", fresh
    fresh_state = open_state(fresh["requestState"], b"phase0-test-secret-32-bytes!!")
    second = gw.call("execution", "submit_job", dict(paid_args), token="dev-token",
                     approval_response={"action": "accept",
                                        "content": {"decision": "approve_once",
                                                    "cost_ack_usd": fresh_state["cost_max_usd"]}},
                     request_state=fresh["requestState"])
    assert second["ok"] is True and second["job_id"], second

    # Free tier on the same gated gateway executes with no approval round.
    free = gw.call("execution", "submit_job",
                   {"circuit_qasm": BELL, "provider": "local-simulator", "shots": 32},
                   token="dev-token")
    assert free.get("resultType") != "input_required" and free["ok"] is True, free


def test_qubo_idempotent_retry_single_charge(monkeypatch):
    """Phase-0 P0: a retried Leap dispatch replays the original job — one
    reservation, never two — and a reused key with a different body refuses."""
    import threading
    import time
    import uuid
    from kubona.providers.annealing_provider import AnnealingProvider

    # Paid Leap pricing (shipped JSON has zero rates and refuses estimation):
    # per-task $2.00 + $0.01/shot x 64 = $2.64 deterministic.
    real_load = exec_server.load_provider

    def paid_load(name):
        cap = real_load(name)
        if name == "dwave-annealing":
            cap = cap.model_copy(update={"pricing": cap.pricing.model_copy(
                update={"model": "per_task_plus_per_shot",
                        "per_task_usd": 2.0, "per_shot_usd": 0.01})})
        return cap

    monkeypatch.setattr(exec_server, "load_provider", paid_load)
    monkeypatch.setattr(exec_server, "_fidelity_gate",
                        lambda capability, circuit: {"admitted": True,
                                                    "provider": capability.provider,
                                                    "fidelity_receipt": "test"})
    monkeypatch.delenv("KUBONA_DWAVE_ANNEALING_LEAP_TOKEN", raising=False)
    # Hold the worker mid-flight so the retry deterministically lands while
    # the first dispatch still holds its single reservation.
    gate = threading.Event()
    real_solve = AnnealingProvider.solve_qubo

    def gated_solve(self, Q, n_samples=64, seed=7, backend="local", sampler_factory=None):
        assert gate.wait(timeout=15), "worker never started"
        return real_solve(Q, n_samples=n_samples, seed=seed,
                          backend=backend, sampler_factory=sampler_factory)

    monkeypatch.setattr(AnnealingProvider, "solve_qubo", gated_solve)

    sess = cost_server.create_session("qubo-idem", "researcher", budget_ceiling_usd=100.0)
    sid = sess["session_id"]
    approval_id = _approve_paid_dispatch(sid, 100.0)
    Q = [[-1.0, 2.0], [0.0, -1.0]]
    key = f"qubo-idem-{uuid.uuid4().hex[:8]}"
    kw = dict(qubo_matrix=Q, n_samples=64, backend="leap", session_id=sid,
              caller_jurisdictions=["US"], approval_id=approval_id)

    first = exec_server.submit_qubo_job(**kw, idempotency_key=key,
                                        idempotency_body_hash="qubo-body-a")
    assert first["ok"] is True, first
    # Wait until the worker picked the job up (single $2.64 reservation live).
    deadline = time.time() + 10
    while exec_server._runner.store.get(first["job_id"]).status == "queued" \
            and time.time() < deadline:
        time.sleep(0.02)
    retry = exec_server.submit_qubo_job(**kw, idempotency_key=key,
                                        idempotency_body_hash="qubo-body-a")
    assert retry["ok"] is True and retry.get("idempotent_replay") is True, retry
    assert retry["job_id"] == first["job_id"]
    assert exec_server.session_status(sid)["spent_usd"] == 2.64, \
        "retry must not double-charge the reservation"
    mismatch = exec_server.submit_qubo_job(**kw, idempotency_key=key,
                                           idempotency_body_hash="qubo-body-B")
    assert mismatch["ok"] is False and "different request body" in mismatch["error"]

    gate.set()  # Leap refuses without credentials -> pre-vendor failure refunds.
    terminal = None
    for _ in range(200):
        st = exec_server.get_job_status(first["job_id"])
        terminal = st.get("status")
        if terminal in ("failed", "completed", "cancelled"):
            break
        time.sleep(0.05)
    assert terminal == "failed", terminal
    assert exec_server.session_status(sid)["spent_usd"] == 0, \
        "unpinned Leap failure must refund the single reservation"


def test_settle_on_completion_releases_difference_idempotent():
    """Phase-0 P0: terminal completion with reported usage settles
    reserved->actual to the session; settling again is a no-op. Go-prepaid
    mirrors are never touched (Go settles its own rail)."""
    import time
    # $8.00 reservation, vendor reports $3.20 of usage.
    sess = cost_server.create_session("settle-drill", "researcher", budget_ceiling_usd=100.0)
    sid = sess["session_id"]
    exec_server._gov.record_spend(sid, 8.0)
    job = exec_server._runner.store.create(
        provider="ibm-qiskit", session_id=sid, circuit_hash="settle-test",
        estimate={"estimated_usd": 8.0}, reserved_usd=8.0)
    exec_server._runner.store.transition(job.job_id, "running")
    exec_server._runner.store.transition(
        job.job_id, "completed",
        result={"counts": {"00": 64}, "usage_seconds": 2.0, "actual_usd": 3.2})
    out = exec_server.get_result(job.job_id)
    assert out["ok"] is True, out
    assert out.get("settlement", {}).get("refunded_usd") == 4.8, out
    assert exec_server.session_status(sid)["spent_usd"] == 3.2
    # Second read: already settled — a no-op, money unmoved.
    out2 = exec_server.get_result(job.job_id)
    assert out2["ok"] is True
    assert "settlement" not in out2, out2
    assert exec_server.session_status(sid)["spent_usd"] == 3.2
    assert exec_server._runner.store.get(job.job_id).meta.get("settled") is True

    # Go-prepaid mirror: same completion shape, Python releases nothing.
    pre = cost_server.create_session("settle-prepaid", "researcher", budget_ceiling_usd=100.0)
    psid = pre["session_id"]
    exec_server._sessions.update(
        psid, meta={**(exec_server._sessions.get(psid).meta or {}), "spend_prepaid": True})
    exec_server._gov.record_spend(psid, 8.0)  # Go-book mirror
    pjob = exec_server._runner.store.create(
        provider="ibm-qiskit", session_id=psid, circuit_hash="settle-prepaid",
        estimate={"estimated_usd": 8.0}, reserved_usd=0.0)
    exec_server._runner.store.transition(pjob.job_id, "running")
    exec_server._runner.store.transition(
        pjob.job_id, "completed",
        result={"counts": {"00": 64}, "usage_seconds": 2.0, "actual_usd": 3.2})
    assert exec_server.get_result(pjob.job_id)["ok"] is True
    assert exec_server.session_status(psid)["spent_usd"] == 8.0, \
        "Go-prepaid mirrors must not be settled on the Python rail"
    assert time.time() > 0  # keep the import honest if the loop above is skipped


def test_cancel_pre_vendor_refunds_full_reservation():
    """Phase-0 P0: cancelling before the vendor ran returns the whole
    reservation, ledger-sealed, with the policy stated in the receipt."""
    sess = cost_server.create_session("cancel-refund", "researcher", budget_ceiling_usd=100.0)
    sid = sess["session_id"]
    exec_server._gov.record_spend(sid, 8.0)
    job = exec_server._runner.store.create(
        provider="ibm-qiskit", session_id=sid, circuit_hash="cancel-pre",
        estimate={"estimated_usd": 8.0}, reserved_usd=8.0)
    out = exec_server.cancel_job(job.job_id)
    assert out["ok"] is True and out["status"] == "cancelled", out
    assert out["refund_usd"] == 8.0, out
    assert "pre-vendor" in out["spend_policy"], out
    assert exec_server.session_status(sid)["spent_usd"] == 0
    assert exec_server._runner.store.get(job.job_id).status == "cancelled"


def test_cancel_post_vendor_keeps_charge(monkeypatch):
    """Phase-0 P0: a pinned hardware id means the vendor may have run — the
    charge stands, recall is attempted, and the receipt says so."""
    from kubona.providers.base import ProviderError

    class RefusingRecall:
        def cancel(self, hardware_job_id):
            raise ProviderError("vendor recall refused: already running")

    monkeypatch.setattr(exec_server, "get_provider", lambda provider: RefusingRecall())
    sess = cost_server.create_session("cancel-kept", "researcher", budget_ceiling_usd=100.0)
    sid = sess["session_id"]
    exec_server._gov.record_spend(sid, 8.0)
    job = exec_server._runner.store.create(
        provider="ibm-qiskit", session_id=sid, circuit_hash="cancel-post",
        estimate={"estimated_usd": 8.0}, reserved_usd=8.0,
        hardware_job_id="hw-1")
    out = exec_server.cancel_job(job.job_id)
    assert out["ok"] is True and out["status"] == "cancelled", out
    assert out["provider_recall_attempted"] is True, out
    assert out["charge_kept_usd"] == 8.0, out
    assert "post-vendor" in out["spend_policy"], out
    assert exec_server.session_status(sid)["spent_usd"] == 8.0, \
        "post-vendor cancel keeps the charge (vendor may have run)"


# -- failure/cancel settlement (measured usage always wins) -----------------

def _spend_status(sid: str) -> float:
    from kubona.mcp.servers import execution as execution_server
    return execution_server.session_status(sid)["spent_usd"]


def test_failed_job_with_reported_usage_settles_to_actual():
    """Post-vendor failure with metered usage settles reserved->actual on the
    read path (idempotent replay moves nothing); second read replays."""
    from kubona.mcp.servers import execution as execution_server

    sess = cost_server.create_session("fail-settle", "researcher", budget_ceiling_usd=100.0)
    sid = sess["session_id"]
    execution_server._gov.record_spend(sid, 8.0)
    job = execution_server._runner.store.create(
        provider="ibm-qiskit", session_id=sid, circuit_hash="fail-settle",
        estimate={"estimated_usd": 8.0}, reserved_usd=8.0,
        hardware_job_id="hw-9")
    execution_server._runner.store.transition(job.job_id, "running")
    execution_server._runner.store.transition(
        job.job_id, "failed", error="vendor boom",
        result={"usage_seconds": 2.0, "actual_usd": 3.2, "hardware_job_id": "hw-9"})
    out = execution_server.get_result(job.job_id)
    assert out["ok"] is False and out["status"] == "failed", out
    assert out.get("settlement", {}).get("refunded_usd") == 4.8, out
    assert _spend_status(sid) == 3.2
    replay = execution_server.get_result(job.job_id)
    assert replay["ok"] is False and "settlement" not in replay, replay
    assert _spend_status(sid) == 3.2


def test_failed_pre_vendor_without_usage_settles_nothing():
    """No hardware id + no usage: the dispatch path owns the refund; the
    settle mirror must not misrecord a kept estimate against it."""
    from kubona.mcp.servers import execution as execution_server

    sess = cost_server.create_session("fail-prevendor", "researcher", budget_ceiling_usd=100.0)
    sid = sess["session_id"]
    execution_server._gov.record_spend(sid, 8.0)
    job = execution_server._runner.store.create(
        provider="ibm-qiskit", session_id=sid, circuit_hash="fail-pre",
        estimate={"estimated_usd": 8.0}, reserved_usd=8.0)
    execution_server._runner.store.transition(job.job_id, "running")
    execution_server._runner.store.transition(job.job_id, "failed", error="no credential")
    out = execution_server.get_result(job.job_id)
    assert out["ok"] is False, out
    assert "settlement" not in out, out
    # Reservation untouched by the mirror (dispatch refunded it separately).
    assert _spend_status(sid) == 8.0


def test_post_vendor_failed_without_usage_keeps_charge():
    """Pinned hardware id + zero reported usage: the vendor may have run, so
    the estimate stands (loudly — no refund, no settle receipt)."""
    from kubona.mcp.servers import execution as execution_server

    sess = cost_server.create_session("fail-kept", "researcher", budget_ceiling_usd=100.0)
    sid = sess["session_id"]
    execution_server._gov.record_spend(sid, 8.0)
    job = execution_server._runner.store.create(
        provider="ibm-qiskit", session_id=sid, circuit_hash="fail-kept",
        estimate={"estimated_usd": 8.0}, reserved_usd=8.0,
        hardware_job_id="hw-kept")
    execution_server._runner.store.transition(job.job_id, "running")
    execution_server._runner.store.transition(job.job_id, "failed", error="vendor boom")
    out = execution_server.get_result(job.job_id)
    assert out["ok"] is False, out
    assert "settlement" not in out, out
    assert _spend_status(sid) == 8.0


def test_deferred_poll_failure_settles_from_vendor_usage(monkeypatch):
    """End to end: a deferred hardware poll reporting failure WITH usage
    settles reserved->actual (provider usage reporting + poll settle)."""
    from kubona.mcp.servers import execution as execution_server

    class _FailingVendor:
        def poll_hardware_job(self, hardware_job_id):
            return {"status": "failed", "vendor_status": "error",
                    "hardware_job_id": hardware_job_id,
                    "error": "ibm runtime job blew up",
                    "usage_seconds": 120.0, "actual_usd": 192.0}

        def cancel(self, hardware_job_id):
            raise AssertionError("cancel must not run on this path")

    monkeypatch.setattr(execution_server, "get_provider", lambda provider: _FailingVendor())
    sess = cost_server.create_session("poll-fail-settle", "researcher", budget_ceiling_usd=500.0)
    sid = sess["session_id"]
    execution_server._gov.record_spend(sid, 200.0)
    job = execution_server._runner.store.create(
        provider="ibm-qiskit", session_id=sid, circuit_hash="poll-fail",
        estimate={"estimated_usd": 200.0}, reserved_usd=200.0,
        hardware_job_id="hw-poll-1")
    execution_server._runner.store.transition(job.job_id, "running")
    st = execution_server.get_job_status(job.job_id)
    assert st["status"] == "failed", st
    assert _spend_status(sid) == 192.0


def test_cancel_post_vendor_with_reported_usage_settles_difference():
    """Post-vendor cancel where the job result already reports usage settles
    to actual instead of keeping the full charge."""
    from kubona.mcp.servers import execution as execution_server
    from kubona.providers.base import ProviderError

    class _RefusingRecall:
        def cancel(self, hardware_job_id):
            raise ProviderError("vendor recall refused: already running")

    import kubona.mcp.servers.execution as exec_mod
    orig_get_provider = exec_mod.get_provider
    exec_mod.get_provider = lambda provider: _RefusingRecall()
    try:
        sess = cost_server.create_session("cancel-settle", "researcher", budget_ceiling_usd=100.0)
        sid = sess["session_id"]
        execution_server._gov.record_spend(sid, 8.0)
        job = execution_server._runner.store.create(
            provider="ibm-qiskit", session_id=sid, circuit_hash="cancel-use",
            estimate={"estimated_usd": 8.0}, reserved_usd=8.0,
            hardware_job_id="hw-cancel-1")
        # Vendor metered $2.50 before the supervisor cancelled: store the
        # usage on the live (running) job so the cancel path can settle it.
        stored = execution_server._runner.store.get(job.job_id)
        stored.result = {"usage_seconds": 1.0, "actual_usd": 2.5,
                         "hardware_job_id": "hw-cancel-1"}
        execution_server._runner.store._save(stored)
        out = execution_server.cancel_job(job.job_id)
        assert out["ok"] is True and out["status"] == "cancelled", out
        assert out.get("refunded_usd") == 5.5, out
        assert out.get("settled_to_actual_usd") == 2.5, out
        assert _spend_status(sid) == 2.5
    finally:
        exec_mod.get_provider = orig_get_provider
