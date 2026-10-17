"""R1 — canonical evidence envelope: one receipt identity across both ledgers.

The sealed form is canonical JSON bytes: sorted keys, compact separators,
UTF-8, finite floats only. evidence_id = "ev_" + sha256 hex of those bytes
(computed over every field EXCEPT evidence_id itself).

Rules (mirrored exactly in platform/core/pkg/ledger/envelope.go — the
checked-in vectors in platform/contracts/evidence-vectors.json pin both):
- analysis_profile: mandatory non-empty object containing a non-empty
  "method" string (REA rule: no profile, no receipt).
- normalized_result: mandatory non-empty object.
- tenant_id: mandatory non-empty string (Kubona binding from day one).
- raw_result / mandate_ref: optional; when present must be objects
  (mandate_ref non-empty when present).
- Whole numbers must be ints on both rails (25, not 25.0): Go marshals
  float64(25) as "25" while Python dumps(25.0) as "25.0" — same value,
  different bytes, different id. Callers normalize before sealing.
- No NaN/Infinity (allow_nan=False raises) — non-finite floats refuse.
"""

from __future__ import annotations

import hashlib
import json
import math
from typing import Any


def canonical_bytes(obj: dict[str, Any]) -> bytes:
    """Canonical JSON bytes: sorted keys, compact, UTF-8, finite floats."""
    _assert_finite(obj)
    return json.dumps(
        obj, sort_keys=True, separators=(",", ":"), ensure_ascii=False
    ).encode("utf-8")


def _assert_finite(v: Any) -> None:
    if isinstance(v, float) and (math.isnan(v) or math.isinf(v)):
        raise ValueError("evidence envelope refuses non-finite float")
    if isinstance(v, dict):
        for x in v.values():
            _assert_finite(x)
    elif isinstance(v, (list, tuple)):
        for x in v:
            _assert_finite(x)


def build(
    normalized_result: dict[str, Any],
    analysis_profile: dict[str, Any],
    tenant_id: str,
    raw_result: dict[str, Any] | None = None,
    mandate_ref: dict[str, Any] | None = None,
) -> dict[str, Any]:
    """Build + seal an envelope. Refuses (ValueError) on missing bindings."""
    if not isinstance(normalized_result, dict) or not normalized_result:
        raise ValueError("envelope needs a non-empty normalized_result object")
    if not isinstance(analysis_profile, dict) or not analysis_profile:
        raise ValueError("envelope needs a non-empty analysis_profile (REA rule)")
    method = analysis_profile.get("method")
    if not isinstance(method, str) or not method:
        raise ValueError("analysis_profile.method must be a non-empty string")
    if not isinstance(tenant_id, str) or not tenant_id:
        raise ValueError("envelope needs a non-empty tenant_id")
    if raw_result is not None and not isinstance(raw_result, dict):
        raise ValueError("raw_result must be an object when present")
    if mandate_ref is not None and (not isinstance(mandate_ref, dict) or not mandate_ref):
        raise ValueError("mandate_ref must be a non-empty object when present")
    body = {
        "analysis_profile": analysis_profile,
        "mandate_ref": mandate_ref,
        "normalized_result": normalized_result,
        "raw_result": raw_result,
        "tenant_id": tenant_id,
    }
    digest = hashlib.sha256(canonical_bytes(body)).hexdigest()
    return {"evidence_id": f"ev_{digest}", **body}


def verify(envelope: dict[str, Any]) -> dict[str, Any]:
    """Recompute + revalidate. Returns the envelope; raises on any mismatch."""
    if not isinstance(envelope, dict):
        raise ValueError("envelope must be an object")
    claimed = envelope.get("evidence_id")
    if not isinstance(claimed, str) or not claimed.startswith("ev_") or len(claimed) != 67:
        raise ValueError("envelope evidence_id malformed")
    body = {k: v for k, v in envelope.items() if k != "evidence_id"}
    rebuilt = build(
        body.get("normalized_result"),
        body.get("analysis_profile"),
        body.get("tenant_id"),
        body.get("raw_result"),
        body.get("mandate_ref"),
    )
    if rebuilt["evidence_id"] != claimed:
        raise ValueError("envelope evidence_id does not match content (tampered or foreign bytes)")
    return envelope
