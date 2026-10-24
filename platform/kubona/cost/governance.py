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
from contextlib import contextmanager
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
    # W0.10: the lab + program the minted mandate binds (audit trail of what
    # was authorized — a mandate without a recorded binding is unreviewable).
    mandate_tenant_id: str | None = None
    mandate_job_hash: str | None = None
    mandate_signature_hex: str | None = None
    mandate_pubkey_hex: str | None = None


class CostGovernanceService:
    def __init__(self, sessions: SessionStore, store_path: str | os.PathLike | None = None):
        self.sessions = sessions
        from kubona.config import state_path

        base = Path(store_path) if store_path else state_path("governance.jsonl").parent
        base.mkdir(parents=True, exist_ok=True)
        self.path = base / "governance.jsonl"
        # W0.10: the money decision must be deterministic under concurrency.
        # The thread lock serializes in-process transitions; the file lock
        # serializes across processes sharing the store (approve+reject
        # racing used to both append, last-line-wins deciding spend).
        import threading
        self._lock = threading.Lock()
        self._lock_path = base / "governance.lock"

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

    @contextmanager
    def _money_locked(self):
        """Serialize a full load-check-mutate-append cycle across threads
        and processes sharing this store (approve+reject racing used to both
        append, last-line-wins deciding spend)."""
        import fcntl
        with self._lock:
            with open(self._lock_path, "a+b") as lf:
                fcntl.flock(lf.fileno(), fcntl.LOCK_EX)
                try:
                    yield
                finally:
                    fcntl.flock(lf.fileno(), fcntl.LOCK_UN)

    def request_approval(self, session_id: str, amount_usd: float, reason: str,
                         requested_by: str = "agent") -> Approval:
        import math
        if not isinstance(amount_usd, (int, float)) or not math.isfinite(amount_usd) or amount_usd <= 0:
            raise GovernanceError(f"amount_usd must be a positive finite number, got {amount_usd!r}")
        if not reason or not reason.strip():
            raise GovernanceError("reason is required: unattributed spend requests refuse")
        if not requested_by or not requested_by.strip():
            raise GovernanceError("requested_by is required")
        try:
            self.sessions.get(session_id)
        except KeyError:
            raise GovernanceError(f"unknown session {session_id!r}: approvals attach to live sessions") from None
        a = Approval(approval_id=uuid.uuid4().hex[:16], session_id=session_id,
                     kind="paid_job", amount_requested_usd=round(float(amount_usd), 6),
                     reason=reason, requested_by=requested_by)
        with self._money_locked():
            self._save_approval(a)
            self.sessions.update(session_id, approvals=[*self.sessions.get(session_id).approvals, a.approval_id])
        return a

    def _check_reviewer(self, reviewer: str) -> None:
        if not reviewer or not reviewer.strip():
            raise GovernanceError("reviewer is required: unattributed audit records refuse")

    def hold_approval(self, approval_id: str, reviewer: str) -> Approval:
        self._check_reviewer(reviewer)
        with self._money_locked():
            a = self._load_approvals().get(approval_id)
            if a is None:
                raise KeyError(f"unknown approval {approval_id!r}")
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
        self._check_reviewer(reviewer)
        with self._money_locked():
            a = self._load_approvals().get(approval_id)
            if a is None:
                raise KeyError(f"unknown approval {approval_id!r}")
            # Two-stage gate enforced: REVIEW must pass through HOLD.
            # Approving REVIEW directly skips the hold checkpoint (W0.10).
            if a.state != "HOLD":
                raise GovernanceError(f"approval {approval_id} is {a.state}: approve requires HOLD (request -> hold -> approve)")
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
            a.mandate_tenant_id = tenant_id
            a.mandate_job_hash = job_hash
            a.mandate_signature_hex = m.signature_hex
            a.mandate_pubkey_hex = m.public_key_hex
            self._save_approval(a)
            return a

    def reject(self, approval_id: str, reviewer: str, note: str) -> Approval:
        self._check_reviewer(reviewer)
        with self._money_locked():
            a = self._load_approvals().get(approval_id)
            if a is None:
                raise KeyError(f"unknown approval {approval_id!r}")
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
        # Estimation failures refuse as governed refusals, never as
        # uncaught exceptions: an unpriced provider (zero rates) must
        # surface the same {"ok": False} envelope as an over-budget one.
        try:
            est = estimate_cost(capability, circuit, shots)
        except ValueError as e:
            return {"ok": False, "reason": f"cost estimation refused: {e}", "estimate": None}
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
    def burn_rate(self, session_id: str) -> dict[str, Any]:
        """Average spend rate since session start (W0.10): the old code
        divided cumulative spend by a fixed 60s window, mislabeling the
        result as a rate. The window param is gone — a windowed rate needs
        timestamped spend events, which this store does not keep."""
        s = self.sessions.get(session_id)
        elapsed = max(time.time() - s.created_at, 1e-9)
        rate = round(s.spent_usd / elapsed, 6) if s.spent_usd else 0.0
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
