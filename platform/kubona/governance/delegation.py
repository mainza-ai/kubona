"""Cryptographic AP2 Sub-Mandate Delegation (Ring 0 & 3).

Enforces mathematical invariants for autonomous inter-agent delegation:
    sum(child_ceilings) <= parent_remaining_budget

A child agent can never widen its envelope; all spend delegations are signed
with Ed25519 and verified on every delegation and use. Previously _sign was
salted-SHA256 under a hardcoded source-visible secret with no verify method
at all (W0.9) — "cryptographically verified" was fiction. It is real now.
"""

from __future__ import annotations

import json
import math
import os
import threading
import time
import uuid
from dataclasses import asdict, dataclass
from datetime import datetime, timezone
from pathlib import Path
from typing import Dict, List, Optional

from nacl.exceptions import BadSignatureError
from nacl.signing import SigningKey, VerifyKey

from kubona.config import atomic_write_text, state_path


class DelegationError(ValueError):
    """Base for delegation failures (unknown mandate, bad key, bad amount)."""


class DelegationBudgetExceededError(DelegationError):
    """Raised when delegated envelopes exceed parent ceiling."""


def _rfc3339_now_plus(ttl_seconds: float) -> str:
    return datetime.fromtimestamp(time.time() + ttl_seconds, tz=timezone.utc).strftime(
        "%Y-%m-%dT%H:%M:%SZ"
    )


def _check_ceiling(value: float, what: str) -> float:
    if not isinstance(value, (int, float)) or not math.isfinite(value) or value <= 0:
        raise DelegationError(f"{what} must be a positive finite number, got {value!r}")
    return float(value)


def _check_ttl(ttl_seconds: float) -> float:
    if not isinstance(ttl_seconds, (int, float)) or not math.isfinite(ttl_seconds) or ttl_seconds <= 0:
        raise DelegationError(f"ttl_seconds must be positive, got {ttl_seconds!r}")
    return float(ttl_seconds)


@dataclass
class SpendMandate:
    mandate_id: str
    tenant_id: str
    agent_id: str
    budget_ceiling_usd: float
    spent_usd: float
    parent_mandate_id: Optional[str]
    expires_at: str  # RFC3339 UTC
    public_key_hex: str
    signature: str

    def remaining(self) -> float:
        return max(0.0, self.budget_ceiling_usd - self.spent_usd)

    def is_expired(self) -> bool:
        exp = datetime.fromisoformat(self.expires_at.replace("Z", "+00:00"))
        return datetime.now(timezone.utc) > exp

    def to_dict(self) -> dict:
        return asdict(self)


class SpendDelegationManager:
    def __init__(self, signing_key: bytes | None = None, path: str | os.PathLike | None = None):
        # No default key: the old hardcoded "kubona-dev-signing-key" let
        # anyone holding the source forge mandates. Fail-closed instead.
        if not isinstance(signing_key, (bytes, bytearray)) or len(signing_key) != 32:
            raise DelegationError(
                "SpendDelegationManager requires a 32-byte Ed25519 signing key: "
                "unsigned delegation never mints"
            )
        self._key = SigningKey(bytes(signing_key))
        self._mandates: Dict[str, SpendMandate] = {}
        self._revoked: set[str] = set()
        self._lock = threading.Lock()
        self.path = Path(path) if path is not None else state_path("delegations.json")
        self._load()

    def _load(self) -> None:
        if not self.path.exists():
            return
        try:
            doc = json.loads(self.path.read_text())
        except (OSError, ValueError):
            return
        revoked = doc.get("revoked", [])
        if isinstance(revoked, list):
            self._revoked = {str(r) for r in revoked}
        for raw in doc.get("mandates", []):
            try:
                m = SpendMandate(**raw)
            except (TypeError, ValueError):
                continue
            # A restart with a different signing key cannot inherit old
            # mandates: keep only rows whose signature re-verifies now.
            try:
                VerifyKey(bytes.fromhex(m.public_key_hex)).verify(
                    bytes.fromhex(m.signature) + self._payload(
                        m.mandate_id, m.tenant_id, m.agent_id,
                        m.budget_ceiling_usd, m.expires_at
                    )
                )
            except (BadSignatureError, ValueError):
                continue
            self._mandates[m.mandate_id] = m

    def _save(self) -> None:
        doc = {
            "mandates": [m.to_dict() for m in self._mandates.values()],
            "revoked": sorted(self._revoked),
        }
        atomic_write_text(self.path, json.dumps(doc, sort_keys=True))

    @property
    def public_key_hex(self) -> str:
        return self._key.verify_key.encode().hex()

    def _payload(self, mid: str, tenant_id: str, agent_id: str, ceiling: float, exp: str) -> bytes:
        return f"{mid}:{tenant_id}:{agent_id}:{ceiling:.4f}:{exp}".encode("utf-8")

    def _sign(self, payload: bytes) -> str:
        return self._key.sign(payload).signature.hex()

    def verify(self, mandate_id: str) -> SpendMandate:
        """Re-verify signature + expiry + revocation. Anything unverifiable
        raises: readers never trust a stored row on faith."""
        with self._lock:
            m = self._mandates.get(mandate_id)
            revoked = mandate_id in self._revoked
        if m is None or revoked:
            raise DelegationError(f"mandate {mandate_id} unknown or revoked")
        if m.is_expired():
            raise DelegationError(f"mandate {mandate_id} expired at {m.expires_at}")
        try:
            VerifyKey(bytes.fromhex(m.public_key_hex)).verify(
                bytes.fromhex(m.signature) + self._payload(
                    m.mandate_id, m.tenant_id, m.agent_id, m.budget_ceiling_usd, m.expires_at
                )
            )
        except (BadSignatureError, ValueError) as e:
            raise DelegationError(f"mandate {mandate_id} signature invalid: {e}") from None
        return m

    def create_root_mandate(
        self, tenant_id: str, agent_id: str, budget_ceiling_usd: float, ttl_seconds: float = 3600.0
    ) -> SpendMandate:
        if not tenant_id or not tenant_id.strip():
            raise DelegationError("tenant_id is required")
        if not agent_id or not agent_id.strip():
            raise DelegationError("agent_id is required")
        ceiling = _check_ceiling(budget_ceiling_usd, "budget_ceiling_usd")
        _check_ttl(ttl_seconds)
        mid = f"mandate-{uuid.uuid4().hex[:12]}"
        exp = _rfc3339_now_plus(ttl_seconds)
        sig = self._sign(self._payload(mid, tenant_id, agent_id, ceiling, exp))
        m = SpendMandate(
            mandate_id=mid,
            tenant_id=tenant_id,
            agent_id=agent_id,
            budget_ceiling_usd=ceiling,
            spent_usd=0.0,
            parent_mandate_id=None,
            expires_at=exp,
            public_key_hex=self.public_key_hex,
            signature=sig,
        )
        with self._lock:
            self._mandates[mid] = m
            self._save()
        return m

    def delegate_sub_mandate(
        self, parent_mandate_id: str, child_agent_id: str, child_ceiling_usd: float, ttl_seconds: float = 1800.0
    ) -> SpendMandate:
        if not child_agent_id or not child_agent_id.strip():
            raise DelegationError("child_agent_id is required")
        child_ceiling = _check_ceiling(child_ceiling_usd, "child_ceiling_usd")
        _check_ttl(ttl_seconds)
        with self._lock:
            parent = self._mandates.get(parent_mandate_id)
            if parent is None or parent_mandate_id in self._revoked:
                raise DelegationError(f"parent mandate {parent_mandate_id} unknown or revoked")
            if parent.is_expired():
                raise DelegationBudgetExceededError("Parent mandate has expired.")
            existing_children = [
                m for m in self._mandates.values()
                if m.parent_mandate_id == parent_mandate_id and not m.is_expired()
                and m.mandate_id not in self._revoked
            ]
            allocated = sum(c.remaining() for c in existing_children)
            if allocated + child_ceiling > parent.remaining():
                raise DelegationBudgetExceededError(
                    f"Cannot delegate ${child_ceiling:.2f}: parent available is "
                    f"${parent.remaining() - allocated:.2f}."
                )
            cid = f"sub-{uuid.uuid4().hex[:12]}"
            parent_exp = datetime.fromisoformat(parent.expires_at.replace("Z", "+00:00"))
            child_exp_dt = datetime.fromtimestamp(time.time() + ttl_seconds, tz=timezone.utc)
            exp = min(parent_exp, child_exp_dt).strftime("%Y-%m-%dT%H:%M:%SZ")
            sig = self._sign(self._payload(cid, parent.tenant_id, child_agent_id, child_ceiling, exp))
            child = SpendMandate(
                mandate_id=cid,
                tenant_id=parent.tenant_id,
                agent_id=child_agent_id,
                budget_ceiling_usd=child_ceiling,
                spent_usd=0.0,
                parent_mandate_id=parent_mandate_id,
                expires_at=exp,
                public_key_hex=self.public_key_hex,
                signature=sig,
            )
            self._mandates[cid] = child
            self._save()
            return child

    def record_spend(self, mandate_id: str, usd: float) -> float:
        """Advance spent_usd under the same lock as the ceiling check, so
        concurrent spends cannot jointly overshoot (the old code never
        advanced spent at all, so remaining() was permanently the ceiling)."""
        if not isinstance(usd, (int, float)) or not math.isfinite(usd) or usd <= 0:
            raise DelegationError(f"spend must be a positive finite number, got {usd!r}")
        with self._lock:
            m = self._mandates.get(mandate_id)
            if m is None or mandate_id in self._revoked:
                raise DelegationError(f"mandate {mandate_id} unknown or revoked")
            if m.is_expired():
                raise DelegationError(f"mandate {mandate_id} expired")
            if m.spent_usd + usd > m.budget_ceiling_usd:
                raise DelegationBudgetExceededError(
                    f"spend ${usd:.4f} exceeds remaining ${m.remaining():.4f}"
                )
            m.spent_usd = round(m.spent_usd + usd, 6)
            curr = m
            while curr.parent_mandate_id:
                parent = self._mandates.get(curr.parent_mandate_id)
                if parent is not None:
                    parent.spent_usd = round(parent.spent_usd + usd, 6)
                    curr = parent
                else:
                    break
            self._save()
            return m.spent_usd

    def revoke(self, mandate_id: str) -> List[str]:
        """Revoke a mandate and every live descendant (kill-switch a
        compromised parent actually kills). Returns revoked ids."""
        with self._lock:
            if mandate_id not in self._mandates:
                raise DelegationError(f"mandate {mandate_id} unknown")
            revoked = [mandate_id]
            self._revoked.add(mandate_id)
            changed = True
            while changed:
                changed = False
                for m in self._mandates.values():
                    if (
                        m.parent_mandate_id in self._revoked
                        and m.mandate_id not in self._revoked
                    ):
                        self._revoked.add(m.mandate_id)
                        revoked.append(m.mandate_id)
                        changed = True
            self._save()
            return revoked
