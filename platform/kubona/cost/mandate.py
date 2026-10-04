"""Agent Payments Protocol (AP2) mandate tokens and verification.

Models the cost-governance approval gate as a cryptographic spend mandate:
{tenant_id, session_id, job_hash, max_budget_usd, expiry_timestamp}
signed by an operator's public key (e.g. ed25519) to prevent ambient token abuse
and LLM hallucination spend holes.
"""

from __future__ import annotations

import hashlib
import json
import time
from dataclasses import asdict, dataclass
from typing import Any


@dataclass(frozen=True)
class SpendMandate:
    mandate_id: str
    tenant_id: str
    session_id: str
    job_hash: str
    max_budget_usd: float
    expiry_timestamp: float
    approved_by: str
    signature: str = ""

    def canonical_payload(self) -> bytes:
        data = {
            "mandate_id": self.mandate_id,
            "tenant_id": self.tenant_id,
            "session_id": self.session_id,
            "job_hash": self.job_hash,
            "max_budget_usd": round(self.max_budget_usd, 6),
            "expiry_timestamp": round(self.expiry_timestamp, 3),
            "approved_by": self.approved_by,
        }
        return json.dumps(data, sort_keys=True).encode("utf-8")

    def digest(self) -> str:
        return hashlib.sha256(self.canonical_payload()).hexdigest()

    def is_expired(self, now: float | None = None) -> bool:
        t = now if now is not None else time.time()
        return t > self.expiry_timestamp

    def to_dict(self) -> dict[str, Any]:
        return asdict(self)


def create_mandate(
    mandate_id: str,
    tenant_id: str,
    session_id: str,
    job_hash: str,
    max_budget_usd: float,
    ttl_seconds: float = 3600.0,
    approved_by: str = "operator@kubona.local",
) -> SpendMandate:
    return SpendMandate(
        mandate_id=mandate_id,
        tenant_id=tenant_id,
        session_id=session_id,
        job_hash=job_hash,
        max_budget_usd=max_budget_usd,
        expiry_timestamp=time.time() + ttl_seconds,
        approved_by=approved_by,
        signature="",
    )


def verify_mandate_integrity(mandate: SpendMandate) -> tuple[bool, str]:
    if mandate.is_expired():
        return False, f"mandate {mandate.mandate_id} expired at {mandate.expiry_timestamp}"
    if mandate.max_budget_usd <= 0.0:
        return False, f"mandate {mandate.mandate_id} has invalid budget {mandate.max_budget_usd}"
    return True, "mandate valid and within TTL"
