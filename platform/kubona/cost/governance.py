"""Cost-governance service (\u00a74.5) — Phase 1.

Four mechanisms from the plan:
  1. mandatory simulator pre-flight   (any job reaching a paid queue)
  2. hard per-session budget ceiling  (enforced at write time, not just logged)
  3. two-stage approval gate          (REVIEW -> HOLD -> APPROVED/REJECTED;
                                      the MilimoClaw REVIEW/HOLD pattern, \u00a74.5/\u00a78)
  4. real-time burn-rate telemetry    (spend, rate, remaining-at-rate)

All state persists to KUBONA_STATE_DIR (governance.jsonl) — the audit ledger
remains the immutable system of record; this store is the live control state.
"""

from __future__ import annotations

import hashlib
import json
import os
import tempfile
import time
import uuid
from dataclasses import asdict, dataclass, field
from pathlib import Path
from typing import Any

from kubona.circuits import Circuit
from kubona.cost.estimator import estimate as estimate_cost
from kubona.schema import ProviderCapability
from kubona.session import SessionState, SessionStore


class GovernanceError(RuntimeError):
    pass


@dataclass
class Approval:
    approval_id: str
    session_id: str
    kind: str  # "paid_job" | "budget_topup"
    amount_requested_usd: float
    reason: str
    state: str = "REVIEW"  # REVIEW -> HOLD -> APPROVED | REJECTED
    requested_by: str = "agent"
    reviewed_by: str | None = None
    note: str | None = None
    created_at: float = field(default_factory=time.time)
    reviewed_at: float | None = None
    mandate_token: str | None = None
    mandate_digest: str | None = None


class CostGovernanceService:
    def __init__(self, sessions: SessionStore, store_path: str | os.PathLike | None = None):
        self.sessions = sessions
        from kubona.config import state_path

        base = Path(store_path) if store_path else state_path("governance.jsonl").parent
        base.mkdir(parents=True, exist_ok=True)
        self.path = base / "governance.jsonl"

    # -- approvals (two-stage gate) -------------------------------------
    def _load_approvals(self) -> dict[str, Approval]:
        out = {}
        if self.path.exists():
            for line in self.path.read_text().splitlines():
                if line.strip():
                    a = Approval(**json.loads(line))
                    out[a.approval_id] = a
        return out

    def _save_approval(self, a: Approval) -> None:
        with open(self.path, "a") as f:
            f.write(json.dumps(asdict(a), sort_keys=True) + "\n")

    def request_approval(self, session_id: str, amount_usd: float, reason: str,
                         requested_by: str = "agent") -> Approval:
        a = Approval(approval_id=uuid.uuid4().hex[:16], session_id=session_id,
                     kind="paid_job", amount_requested_usd=round(amount_usd, 6),
                     reason=reason, requested_by=requested_by)
        self._save_approval(a)
        self.sessions.update(session_id, approvals=[*self.sessions.get(session_id).approvals, a.approval_id])
        return a

    def hold_approval(self, approval_id: str, reviewer: str) -> Approval:
        a = self._load_approvals()[approval_id]
        if a.state != "REVIEW":
            raise GovernanceError(f"approval {approval_id} is {a.state}, not REVIEW")
        a.state = "HOLD"
        a.reviewed_by = reviewer
        a.reviewed_at = time.time()
        self._save_approval(a)
        return a

    def approve(self, approval_id: str, reviewer: str, note: str | None = None,
                tenant_id: str | None = None, job_hash: str | None = None,
                signing_key: bytes | None = None) -> Approval:
        from kubona.cost.mandate import create_mandate, utc_now_plus, verify_mandate

        a = self._load_approvals()[approval_id]
        if a.state not in ("REVIEW", "HOLD"):
            raise GovernanceError(f"approval {approval_id} is {a.state}")
        # W0.9: the mandate binds an explicit lab + program under an explicit
        # operator key. No defaults anywhere: a defaulted tenant or job hash
        # is an unbound mandate, and an unsigned one never mints. (W0.10
        # propagates the real session tenant + payload hash here.)
        if not tenant_id or not tenant_id.strip():
            raise GovernanceError("approve requires tenant_id: mandates bind a lab, never a default")
        if not job_hash or not job_hash.strip():
            raise GovernanceError("approve requires job_hash: mandates bind a program, never a blank")
        if signing_key is None:
            raise GovernanceError("approve requires signing_key (operator Ed25519 seed): unsigned mandates never mint")
        m = create_mandate(
            signing_key=signing_key,
            tenant_id=tenant_id,
            job_hash=job_hash,
            max_budget_usd=a.amount_requested_usd,
            expiry=utc_now_plus(3600.0),
        )
        ok, reason = verify_mandate(m, tenant_id, job_hash, a.amount_requested_usd)
        if not ok:
            raise GovernanceError(f"freshly minted mandate must verify: {reason}")
        a.state = "APPROVED"
        a.reviewed_by = reviewer
        a.note = note
        a.reviewed_at = time.time()
        a.mandate_token = m.signature_hex
        a.mandate_digest = hashlib.sha256(m.canonical_payload()).hexdigest()
        self._save_approval(a)
        return a

    def reject(self, approval_id: str, reviewer: str, note: str) -> Approval:
        a = self._load_approvals()[approval_id]
        if a.state not in ("REVIEW", "HOLD"):
            raise GovernanceError(f"approval {approval_id} is {a.state}")
        a.state = "REJECTED"
        a.reviewed_by = reviewer
        a.note = note
        a.reviewed_at = time.time()
        self._save_approval(a)
        return a

    # -- pre-flight + spend enforcement ---------------------------------
    def preflight(self, capability: ProviderCapability, circuit: Circuit, shots: int,
                  session: SessionState) -> dict[str, Any]:
        est = estimate_cost(capability, circuit, shots)
        cost = est["estimated_usd"]
        if cost == 0.0:
            return {"ok": True, "reason": "free tier (simulator)", "estimate": est}
        if session.budget_ceiling_usd is None:
            return {"ok": False, "reason": "paid provider requires a session budget ceiling", "estimate": est}
        if cost > session.remaining():
            return {"ok": False, "reason": f"estimate {cost} exceeds remaining budget {session.remaining()}",
                    "estimate": est}
        return {"ok": True, "reason": "within session ceiling", "estimate": est}

    def record_spend(self, session_id: str, usd: float) -> float:
        """Hard ceiling enforcement at write time. Raises GovernanceError on breach."""
        try:
            self.sessions.record_spend(session_id, usd)
        except ValueError as e:
            raise GovernanceError(str(e)) from e
        return self.sessions.get(session_id).spent_usd

    # -- burn-rate telemetry --------------------------------------------
    def burn_rate(self, session_id: str, window_s: float = 60.0) -> dict[str, Any]:
        s = self.sessions.get(session_id)
        rate = round(s.spent_usd / max(window_s, 1e-9), 6) if s.spent_usd else 0.0
        remaining = s.remaining()
        eta = round(remaining / rate, 1) if (remaining is not None and rate > 0) else None
        return {
            "session_id": session_id,
            "spent_usd": s.spent_usd,
            "ceiling_usd": s.budget_ceiling_usd,
            "remaining_usd": remaining,
            "burn_rate_usd_per_s": rate,
            "eta_s_at_current_rate": eta,
            "open_approvals": [a for a in self._load_approvals().values()
                               if a.session_id == session_id and a.state in ("REVIEW", "HOLD")],
        }
