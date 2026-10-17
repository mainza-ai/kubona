"""AP2 spend mandates: canonical Ed25519 envelopes shared with the Go daemon.

Canonical bytes (MUST match Go SpendMandate.CanonicalPayload byte-for-byte):
    f"{tenant_id}:{job_hash}:{max_budget_usd:.4f}:{expiry_rfc3339}"

Signed with Ed25519 (PyNaCl). There are no default keys, no default tenants,
and no default job hashes anywhere in this module: an unsigned or unbound
mandate is not a mandate and never mints. Previously create_mandate() set
signature="" and verify_mandate_integrity() checked only expiry+positivity
while the docstring claimed operator signing (W0.9) — that fiction is gone.
"""

from __future__ import annotations

import hashlib
import math
import os
import time
from dataclasses import asdict, dataclass
from datetime import datetime, timezone
from typing import Any

from nacl.exceptions import BadSignatureError
from nacl.signing import SigningKey, VerifyKey


def _rfc3339(dt: datetime) -> str:
    if dt.tzinfo is None:
        raise ValueError("mandate expiry must be timezone-aware (UTC)")
    return dt.astimezone(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")


def _parse_rfc3339(s: str) -> datetime:
    return datetime.fromisoformat(s.replace("Z", "+00:00"))


def canonical_job_binding(job_type: str, session_id: str, qasm: str) -> str:
    """Canonical v1 job binding (W0.4): identical string to Go's
    mandates.CanonicalJobBinding and console canonicalJobBinding — pinned by
    the fixed vector test on all three sides."""
    return hashlib.sha256(f"kubona-job-v1\n{job_type}\n{session_id}\n{qasm}".encode("utf-8")).hexdigest()


@dataclass(frozen=True)
class SpendMandate:
    tenant_id: str
    job_hash: str
    max_budget_usd: float
    expiry_timestamp: str  # RFC3339 UTC
    public_key_hex: str
    signature_hex: str

    def canonical_payload(self) -> bytes:
        return (
            f"{self.tenant_id}:{self.job_hash}:"
            f"{self.max_budget_usd:.4f}:{self.expiry_timestamp}"
        ).encode("utf-8")

    def is_expired(self, now: datetime | None = None) -> bool:
        t = now if now is not None else datetime.now(timezone.utc)
        return t > _parse_rfc3339(self.expiry_timestamp)

    def to_dict(self) -> dict[str, Any]:
        return asdict(self)


def create_mandate(
    signing_key: bytes,
    tenant_id: str,
    job_hash: str,
    max_budget_usd: float,
    expiry: datetime,
) -> SpendMandate:
    """Mint and sign a spend mandate. Every binding is explicit: empty
    tenant/job refuses, non-positive/non-finite budget refuses, naive or past
    expiry refuses. The key is the operator Ed25519 seed (32 bytes) — callers
    supply it; this module never invents one."""
    if not isinstance(signing_key, (bytes, bytearray)) or len(signing_key) != 32:
        raise ValueError("signing_key must be a 32-byte Ed25519 seed: unsigned mandates never mint")
    if not tenant_id or not tenant_id.strip():
        raise ValueError("tenant_id is required: mandates bind a lab, never a default")
    if not job_hash or not job_hash.strip():
        raise ValueError("job_hash is required: mandates bind a program, never a blank")
    if not isinstance(max_budget_usd, (int, float)) or not math.isfinite(max_budget_usd) or max_budget_usd <= 0:
        raise ValueError(f"max_budget_usd must be a positive finite number, got {max_budget_usd!r}")
    expiry_iso = _rfc3339(expiry)
    if _parse_rfc3339(expiry_iso) <= datetime.now(timezone.utc):
        raise ValueError("mandate expiry must be in the future")
    key = SigningKey(bytes(signing_key))
    payload = (
        f"{tenant_id}:{job_hash}:{float(max_budget_usd):.4f}:{expiry_iso}"
    ).encode("utf-8")
    sig = key.sign(payload).signature
    return SpendMandate(
        tenant_id=tenant_id,
        job_hash=job_hash,
        max_budget_usd=float(max_budget_usd),
        expiry_timestamp=expiry_iso,
        public_key_hex=key.verify_key.encode().hex(),
        signature_hex=sig.hex(),
    )


def verify_mandate(
    mandate: SpendMandate,
    expected_tenant_id: str,
    expected_job_hash: str,
    requested_cost_usd: float,
) -> tuple[bool, str]:
    """Verify with expectations (mirrors Go SpendMandate.Verify): tenant and
    job binding, expiry, budget cover, and Ed25519 signature — in that order.
    Empty expectations refuse: verification against nothing-specific verifies
    nothing."""
    if not expected_tenant_id or mandate.tenant_id != expected_tenant_id:
        return False, f"tenant mismatch: mandate binds {mandate.tenant_id!r}"
    if not expected_job_hash or mandate.job_hash != expected_job_hash:
        return False, "job-hash mismatch: mandate binds a different program"
    if mandate.is_expired():
        return False, f"mandate expired at {mandate.expiry_timestamp}"
    if requested_cost_usd > mandate.max_budget_usd:
        return False, (
            f"cost {requested_cost_usd} exceeds mandate budget {mandate.max_budget_usd}"
        )
    try:
        VerifyKey(bytes.fromhex(mandate.public_key_hex)).verify(
            bytes.fromhex(mandate.signature_hex) + mandate.canonical_payload()
        )
    except (BadSignatureError, ValueError) as e:
        return False, f"invalid mandate signature: {e}"
    return True, "mandate valid: bound, unexpired, covering, signed"


def operator_key_from_env(env_var: str = "KUBONA_OPERATOR_PRIVKEY_HEX") -> bytes:
    """Load the operator Ed25519 seed from the environment. Missing or
    malformed refuses — servers never fall back to an ephemeral key (an
    ephemeral signer mints mandates nobody can verify after restart)."""
    raw = os.environ.get(env_var, "").strip()
    if not raw:
        raise ValueError(
            f"{env_var} is not set: spend approval requires an operator key, refusing"
        )
    try:
        key = bytes.fromhex(raw)
    except ValueError:
        raise ValueError(f"{env_var} is not hex: refusing") from None
    if len(key) != 32:
        raise ValueError(f"{env_var} must be a 32-byte seed hex: refusing")
    return key


def utc_now_plus(seconds: float) -> datetime:
    return datetime.fromtimestamp(time.time() + seconds, tz=timezone.utc)
