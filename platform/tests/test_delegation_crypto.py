"""W0.9: delegation is real Ed25519 with enforced invariants."""

import os

import pytest

from kubona.governance.delegation import (
    DelegationBudgetExceededError,
    DelegationError,
    SpendDelegationManager,
)


def _mgr():
    return SpendDelegationManager(os.urandom(32))


def test_no_default_key():
    with pytest.raises(DelegationError):
        SpendDelegationManager()
    with pytest.raises(DelegationError):
        SpendDelegationManager("kubona-dev-signing-key")


def test_root_and_sub_mandate_verify():
    mgr = _mgr()
    root = mgr.create_root_mandate("lab-a", "agent-1", 100.0)
    assert mgr.verify(root.mandate_id).mandate_id == root.mandate_id
    child = mgr.delegate_sub_mandate(root.mandate_id, "agent-2", 40.0)
    assert mgr.verify(child.mandate_id).parent_mandate_id == root.mandate_id
    assert child.tenant_id == "lab-a"


def test_over_delegation_refuses():
    mgr = _mgr()
    root = mgr.create_root_mandate("lab-a", "agent-1", 100.0)
    mgr.delegate_sub_mandate(root.mandate_id, "agent-2", 60.0)
    with pytest.raises(DelegationBudgetExceededError):
        mgr.delegate_sub_mandate(root.mandate_id, "agent-3", 50.0)


def test_record_spend_advances_and_caps():
    mgr = _mgr()
    root = mgr.create_root_mandate("lab-a", "agent-1", 100.0)
    assert mgr.record_spend(root.mandate_id, 30.0) == 30.0
    assert mgr.verify(root.mandate_id).remaining() == 70.0
    with pytest.raises(DelegationBudgetExceededError):
        mgr.record_spend(root.mandate_id, 71.0)
    with pytest.raises(DelegationError):
        mgr.record_spend(root.mandate_id, -5.0)


def test_revoke_kills_descendants():
    mgr = _mgr()
    root = mgr.create_root_mandate("lab-a", "agent-1", 100.0)
    child = mgr.delegate_sub_mandate(root.mandate_id, "agent-2", 40.0)
    revoked = mgr.revoke(root.mandate_id)
    assert set(revoked) == {root.mandate_id, child.mandate_id}
    with pytest.raises(DelegationError):
        mgr.verify(child.mandate_id)
    with pytest.raises(DelegationError):
        mgr.delegate_sub_mandate(root.mandate_id, "agent-9", 5.0)


def test_validation():
    mgr = _mgr()
    with pytest.raises(DelegationError):
        mgr.create_root_mandate("", "agent-1", 100.0)
    with pytest.raises(DelegationError):
        mgr.create_root_mandate("lab-a", "agent-1", -10.0)
    root = mgr.create_root_mandate("lab-a", "agent-1", 100.0)
    with pytest.raises(DelegationError):
        mgr.delegate_sub_mandate("nope", "agent-2", 10.0)
    with pytest.raises(DelegationError):
        mgr.delegate_sub_mandate(root.mandate_id, "", 10.0)
    with pytest.raises(DelegationError):
        mgr.delegate_sub_mandate(root.mandate_id, "agent-2", float("nan"))
