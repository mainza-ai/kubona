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
    br = gov.burn_rate(s.session_id, window_s=60.0)
    assert br["spent_usd"] == 30.0
    assert br["remaining_usd"] == 70.0
    assert br["burn_rate_usd_per_s"] == 0.5
