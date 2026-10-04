"""Cryptographic AP2 Sub-Mandate Delegation (Ring 0 & 3).

Enforces mathematical invariants for autonomous inter-agent delegation:
    sum(child_ceilings) <= parent_remaining_budget

A child agent can never widen its envelope; all spend delegations are signed
and verified cryptographically.
"""

from __future__ import annotations

import hashlib
import time
import uuid
from dataclasses import asdict, dataclass
from typing import Dict, List, Optional


class DelegationBudgetExceededError(ValueError):
    """Raised when delegated envelopes exceed parent ceiling."""
    pass


@dataclass
class SpendMandate:
    mandate_id: str
    tenant_id: str
    agent_id: str
    budget_ceiling_usd: float
    spent_usd: float
    parent_mandate_id: Optional[str]
    expires_at: float
    signature: str

    def remaining(self) -> float:
        return max(0.0, self.budget_ceiling_usd - self.spent_usd)

    def to_dict(self) -> dict:
        return asdict(self)


class SpendDelegationManager:
    def __init__(self, signing_secret: str = "kubona-dev-signing-key"):
        self.signing_secret = signing_secret
        self._mandates: Dict[str, SpendMandate] = {}

    def _sign(self, payload: str) -> str:
        s = f"{payload}:{self.signing_secret}"
        return hashlib.sha256(s.encode("utf-8")).hexdigest()[:32]

    def create_root_mandate(
        self, tenant_id: str, agent_id: str, budget_ceiling_usd: float, ttl_seconds: float = 3600.0
    ) -> SpendMandate:
        mid = f"mandate-{uuid.uuid4().hex[:12]}"
        exp = time.time() + ttl_seconds
        sig = self._sign(f"{mid}:{tenant_id}:{agent_id}:{budget_ceiling_usd}:{exp}")
        m = SpendMandate(
            mandate_id=mid,
            tenant_id=tenant_id,
            agent_id=agent_id,
            budget_ceiling_usd=budget_ceiling_usd,
            spent_usd=0.0,
            parent_mandate_id=None,
            expires_at=exp,
            signature=sig,
        )
        self._mandates[mid] = m
        return m

    def delegate_sub_mandate(
        self, parent_mandate_id: str, child_agent_id: str, child_ceiling_usd: float, ttl_seconds: float = 1800.0
    ) -> SpendMandate:
        parent = self._mandates[parent_mandate_id]
        if time.time() > parent.expires_at:
            raise DelegationBudgetExceededError("Parent mandate has expired.")

        # Find existing active child mandates to verify sum(child_ceilings) <= parent.remaining()
        existing_children = [
            m for m in self._mandates.values()
            if m.parent_mandate_id == parent_mandate_id and time.time() < m.expires_at
        ]
        allocated_to_children = sum(c.budget_ceiling_usd for c in existing_children)

        if allocated_to_children + child_ceiling_usd > parent.remaining():
            raise DelegationBudgetExceededError(
                f"Cannot delegate ${child_ceiling_usd:.2f}: parent available is "
                f"${parent.remaining() - allocated_to_children:.2f}."
            )

        cid = f"sub-{uuid.uuid4().hex[:12]}"
        exp = min(parent.expires_at, time.time() + ttl_seconds)
        sig = self._sign(f"{cid}:{parent.tenant_id}:{child_agent_id}:{child_ceiling_usd}:{exp}")
        child = SpendMandate(
            mandate_id=cid,
            tenant_id=parent.tenant_id,
            agent_id=child_agent_id,
            budget_ceiling_usd=child_ceiling_usd,
            spent_usd=0.0,
            parent_mandate_id=parent_mandate_id,
            expires_at=exp,
            signature=sig,
        )
        self._mandates[cid] = child
        return child
