"""Cost-governance service tests: ceilings, two-stage approval, burn-rate."""

import pytest

from kubona.cost.governance import CostGovernanceService, GovernanceError
from kubona.session import SessionStore


@pytest.fixture()
def gov(tmp_path):
    return CostGovernanceService(SessionStore(path=str(tmp_path / "sessions.jsonl")),
                                 store_path=str(tmp_path))


def test_ceiling_enforced(gov):
    s = gov.sessions.create("vqe", "agent", budget_ceiling_usd=10.0)
    gov.record_spend(s.session_id, 6.0)
    gov.record_spend(s.session_id, 3.5)  # total 9.5
    with pytest.raises(GovernanceError):
        gov.record_spend(s.session_id, 1.0)  # would exceed 10.0
    assert gov.sessions.get(s.session_id).spent_usd == 9.5


def test_two_stage_approval_lifecycle(gov):
    import os
    key = os.urandom(32)
    s = gov.sessions.create("paid-run", "agent", budget_ceiling_usd=100.0)
    a = gov.request_approval(s.session_id, 42.0, "paid provider run")
    assert a.state == "REVIEW"
    a = gov.hold_approval(a.approval_id, reviewer="pi")
    assert a.state == "HOLD"
    # W0.9: approval mints a real signed mandate bound to lab + program.
    with pytest.raises(GovernanceError):
        gov.approve(a.approval_id, reviewer="pi", note="ok")
    a = gov.approve(a.approval_id, reviewer="pi", note="ok",
                    tenant_id="lab-a", job_hash="job-42", signing_key=key)
    assert a.state == "APPROVED"
    assert a.mandate_token and a.mandate_digest
    with pytest.raises(GovernanceError):
        gov.hold_approval(a.approval_id, reviewer="pi")  # already approved


def test_reject_blocks(gov):
    s = gov.sessions.create("x", "agent", budget_ceiling_usd=100.0)
    a = gov.request_approval(s.session_id, 42.0, "run")
    a = gov.reject(a.approval_id, reviewer="pi", note="too expensive")
    assert a.state == "REJECTED"


def test_burn_rate(gov):
    s = gov.sessions.create("burn", "agent", budget_ceiling_usd=100.0)
    gov.record_spend(s.session_id, 30.0)
    # W0.10: rate is average since session start (spent/elapsed), not
    # spent/60 — assert structure + consistency, never a fixed fixture rate.
    br = gov.burn_rate(s.session_id)
    assert br["spent_usd"] == 30.0
    assert br["remaining_usd"] == 70.0
    assert br["burn_rate_usd_per_s"] > 0
    assert br["eta_s_at_current_rate"] == round(70.0 / br["burn_rate_usd_per_s"], 1)


def test_request_validation(gov):
    s = gov.sessions.create("v", "agent", budget_ceiling_usd=100.0)
    for bad in (0.0, -5.0, float("nan"), float("inf")):
        with pytest.raises(GovernanceError):
            gov.request_approval(s.session_id, bad, "run")
    with pytest.raises(GovernanceError):
        gov.request_approval(s.session_id, 10.0, "   ")
    with pytest.raises(GovernanceError):
        gov.request_approval("ghost", 10.0, "run")


def test_two_stage_gate_enforced(gov):
    import os
    key = os.urandom(32)
    s = gov.sessions.create("stage", "agent", budget_ceiling_usd=100.0)
    a = gov.request_approval(s.session_id, 10.0, "run")
    # REVIEW cannot skip HOLD.
    with pytest.raises(GovernanceError):
        gov.approve(a.approval_id, reviewer="pi", tenant_id="lab-a",
                    job_hash="job-1", signing_key=key)
    gov.hold_approval(a.approval_id, reviewer="pi")
    gov.approve(a.approval_id, reviewer="pi", tenant_id="lab-a",
                job_hash="job-1", signing_key=key)
    # Blank reviewer refuses everywhere.
    b = gov.request_approval(s.session_id, 5.0, "run2")
    with pytest.raises(GovernanceError):
        gov.hold_approval(b.approval_id, reviewer="  ")


def test_concurrent_approve_reject_deterministic(gov):
    import os
    import threading
    key = os.urandom(32)
    s = gov.sessions.create("race", "agent", budget_ceiling_usd=100.0)
    a = gov.request_approval(s.session_id, 10.0, "run")
    gov.hold_approval(a.approval_id, reviewer="pi")
    outcomes = []

    def do_approve():
        try:
            gov.approve(a.approval_id, reviewer="pi", tenant_id="lab-a",
                        job_hash="job-1", signing_key=key)
            outcomes.append("approved")
        except GovernanceError:
            outcomes.append("approve-refused")

    def do_reject():
        try:
            gov.reject(a.approval_id, reviewer="pi", note="no")
            outcomes.append("rejected")
        except GovernanceError:
            outcomes.append("reject-refused")

    ts = [threading.Thread(target=do_approve) for _ in range(4)]
    ts += [threading.Thread(target=do_reject) for _ in range(4)]
    for t in ts:
        t.start()
    for t in ts:
        t.join()
    # Exactly one money decision wins; the losers refuse (no double-append).
    assert sorted(outcomes).count("approved") + sorted(outcomes).count("rejected") == 1
    final = gov._load_approvals()[a.approval_id].state
    assert final in ("APPROVED", "REJECTED")
