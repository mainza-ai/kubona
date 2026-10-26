"""The `job_id` session abstraction (\u00a74.3) — Phase 1.

MCP servers are nominally stateless; a multi-step job (a VQE loop, an ansatz
search) is reconstructed from session state plus the provenance ledger. Sessions
persist to `KUBONA_STATE_DIR/sessions.jsonl` so state survives server restarts
and horizontal scaling (the ledger is the system of record; sessions are the
reconstruction cache).
"""

from __future__ import annotations

import fcntl
import json
import math
import os
import tempfile
import threading
import time
import uuid
from contextlib import contextmanager
from dataclasses import asdict, dataclass, field
from pathlib import Path
from typing import Any, Iterator

from kubona.config import state_path
from kubona.retention import maybe_rotate, rotated_paths


def tenant_slug(tenant: str) -> str:
    """Canonical tenant slug (W0.13, single shared rule): the historical
    charset mapping (alnum + dash/underscore, case preserved — existing
    session dirs depend on it), plus validity: non-empty after stripping,
    max 64 chars. Anything else refuses — the old strip-to-empty rule
    collapsed hostile names like "..." onto the default namespace
    (cross-tenant session/budget bleed). labs.py derives tenants through
    this function. Residual: case-only-distinct names ("LabA" vs "laba")
    share a directory on case-insensitive filesystems — treat tenant names
    as case-sensitive identifiers and don't mint near-collisions."""
    if not isinstance(tenant, str):
        raise ValueError(f"tenant must be a string, got {type(tenant).__name__}")
    slug = "".join(ch for ch in tenant.strip() if ch.isalnum() or ch in "-_")
    if not slug or len(slug) > 64:
        raise ValueError(f"tenant {tenant!r} is not a valid lab slug: refusing")
    return slug


def _tenant_dir(tenant: str) -> str:
    return tenant_slug(tenant)


# update() may only write these fields (W0.13): the old arbitrary setattr
# let any caller raise ceilings, unwind spend, flip status, even rewrite
# session_id. spent_usd is deliberately absent — spend moves only through
# record_spend (ceiling-checked under lock); status only open|closed.
MUTABLE_FIELDS = frozenset({"name", "status", "jobs", "meta", "approvals"})
SESSION_STATUSES = frozenset({"open", "closed"})


def _check_amount(usd: float, what: str) -> float:
    if not isinstance(usd, (int, float)) or not math.isfinite(usd) or usd <= 0:
        raise ValueError(f"{what} must be a positive finite number, got {usd!r}")
    return float(usd)


def _state_dir() -> str | os.PathLike:
    return state_path("sessions.jsonl").parent


@dataclass
class SessionState:
    session_id: str
    name: str
    owner: str
    budget_ceiling_usd: float | None = None
    spent_usd: float = 0.0
    status: str = "open"  # open | closed
    created_at: float = field(default_factory=time.time)
    updated_at: float = field(default_factory=time.time)
    jobs: list[str] = field(default_factory=list)
    approvals: list[str] = field(default_factory=list)
    meta: dict[str, Any] = field(default_factory=dict)

    def remaining(self) -> float | None:
        if self.budget_ceiling_usd is None:
            return None
        return round(self.budget_ceiling_usd - self.spent_usd, 6)


class SessionStore:
    """Append-only JSONL store keyed by session_id (snapshot-per-event).

    Multi-tenant (Phase 5): a `tenant` scopes sessions to a per-lab directory so
    different laboratories never share namespace or budgets.
    """

    def __init__(self, path: str | os.PathLike | None = None, tenant: str | None = None):
        if path is None:
            if tenant:
                path = _state_dir() / _tenant_dir(tenant) / "sessions.jsonl"
            else:
                path = _state_dir() / "sessions.jsonl"
        self.path = Path(path)
        self.path.parent.mkdir(parents=True, exist_ok=True)
        self._lock = threading.Lock()
        self._lock_path = self.path.parent / (self.path.name + ".lock")

    @contextmanager
    def _exclusive(self) -> Iterator[None]:
        """Serialize load-modify-append across threads and processes (W0.13):
        record_spend was read-check-then-append with no lock — two concurrent
        spends both passed the ceiling, then both landed."""
        with self._lock:
            with open(self._lock_path, "a+b") as lf:
                fcntl.flock(lf.fileno(), fcntl.LOCK_EX)
                try:
                    yield
                finally:
                    fcntl.flock(lf.fileno(), fcntl.LOCK_UN)

    def _load(self) -> dict[str, SessionState]:
        out: dict[str, SessionState] = {}
        skipped = 0
        # I2: replay every retained segment oldest-first (see jobs.py).
        for seg in rotated_paths(self.path):
            for line in seg.read_text().splitlines():
                if not line.strip():
                    continue
                try:
                    s = SessionState(**json.loads(line))
                except (ValueError, TypeError, KeyError):
                    skipped += 1  # corrupt line quarantined, never fatal
                    continue
                out[s.session_id] = s
        return out

    def _save(self, s: SessionState) -> None:
        # I2: rotate-before-append; caller must hold _exclusive (create
        # takes it below; update/record paths already do).
        maybe_rotate(self.path)
        with open(self.path, "a") as f:
            f.write(json.dumps(asdict(s), sort_keys=True) + "\n")

    def create(self, name: str, owner: str, budget_ceiling_usd: float | None = None) -> SessionState:
        if budget_ceiling_usd is not None and (
            not isinstance(budget_ceiling_usd, (int, float))
            or not math.isfinite(budget_ceiling_usd)
            or budget_ceiling_usd < 0
        ):
            raise ValueError(f"budget_ceiling_usd must be finite >= 0 or None, got {budget_ceiling_usd!r}")
        s = SessionState(
            session_id=uuid.uuid4().hex[:16],
            name=name,
            owner=owner,
            budget_ceiling_usd=budget_ceiling_usd,
        )
        # _save rotates (I2): takes the lock like every other writer —
        # the unlocked append used to race locked ones.
        with self._exclusive():
            self._save(s)
        return s

    def get(self, session_id: str) -> SessionState:
        s = self._load().get(session_id)
        if s is None:
            raise KeyError(f"unknown session {session_id!r}")
        return s

    def update(self, session_id: str, **changes: Any) -> SessionState:
        for k in changes:
            if k not in MUTABLE_FIELDS:
                raise ValueError(f"session field {k!r} is not mutable via update (spend moves via record_spend)")
        if "status" in changes and changes["status"] not in SESSION_STATUSES:
            raise ValueError(f"session status must be one of {sorted(SESSION_STATUSES)}, got {changes['status']!r}")
        with self._exclusive():
            s = self.get(session_id)
            for k, v in changes.items():
                setattr(s, k, v)
            s.updated_at = time.time()
            self._save(s)
            return s

    def record_job(self, session_id: str, job_id: str) -> SessionState:
        s = self.get(session_id)
        if job_id not in s.jobs:
            s.jobs.append(job_id)
        return self.update(session_id, jobs=s.jobs)

    def record_spend(self, session_id: str, usd: float) -> SessionState:
        amount = _check_amount(usd, "spend")
        with self._exclusive():
            s = self.get(session_id)
            new_spend = round(s.spent_usd + amount, 6)
            if s.budget_ceiling_usd is not None and new_spend > s.budget_ceiling_usd:
                raise ValueError(
                    f"session {session_id} would exceed budget ceiling "
                    f"({new_spend} > {s.budget_ceiling_usd})"
                )
            s.spent_usd = new_spend
            s.updated_at = time.time()
            self._save(s)
            return s

    def release_spend(self, session_id: str, usd: float, reason: str) -> SessionState:
        """Audited refund primitive (R-32, conservative core): return spent
        dollars to the session. Mirrors the Go tenant RefundSpend contract:
        explicit amount (zero refuses), mandatory reason (unexplained money
        movement refuses), no-negative-books clamp (over-refund refuses,
        recorded). Policy still open (who signs, window, partials) — this
        is the mechanism, gated by the caller (bridge route is bearer-only
        like every spend path)."""
        amount = _check_amount(usd, "refund")
        if amount == 0:
            raise ValueError("zero refund refused: use an explicit amount or skip the call")
        if not reason or not reason.strip():
            raise ValueError("refund needs a reason: unexplained money movement refused")
        with self._exclusive():
            s = self.get(session_id)
            if round(amount, 6) > round(s.spent_usd, 6):
                raise ValueError(
                    f"refund of {amount} exceeds recorded spend {s.spent_usd}: "
                    "refusing (no negative books)")
            s.spent_usd = round(s.spent_usd - amount, 6)
            s.updated_at = time.time()
            self._save(s)
            return s

    def close(self, session_id: str) -> SessionState:
        return self.update(session_id, status="closed")

    def sync_external_session(
        self,
        session_id: str,
        name: str = "console-session",
        owner: str = "human",
        budget_ceiling_usd: float | None = None,
        spent_usd: float = 0.0,
        tenant_id: str = "",
    ) -> SessionState:
        """Mirror a verified Go SQLite session into the Python SessionStore
        (F-NEW-01 / F-NEW-05: Go SQLite is the single financial authority for
        HTTP-dispatched sessions). Creates the row if absent or synchronizes
        budget_ceiling_usd and spent_usd under the exclusive store lock so
        preflight and refunds stay in lockstep across rails."""
        if not session_id or not isinstance(session_id, str) or not session_id.strip():
            raise ValueError("session_id must be a non-empty string")
        if budget_ceiling_usd is not None and (
            not isinstance(budget_ceiling_usd, (int, float))
            or not math.isfinite(budget_ceiling_usd)
            or budget_ceiling_usd < 0
        ):
            raise ValueError(f"budget_ceiling_usd must be finite >= 0 or None, got {budget_ceiling_usd!r}")
        if not isinstance(spent_usd, (int, float)) or not math.isfinite(spent_usd) or spent_usd < 0:
            raise ValueError(f"spent_usd must be finite >= 0, got {spent_usd!r}")
        with self._exclusive():
            existing = self._load().get(session_id)
            now = time.time()
            if existing is None:
                meta: dict[str, Any] = {"origin": "go-sqlite-authority"}
                if tenant_id:
                    meta["tenant_id"] = tenant_id
                s = SessionState(
                    session_id=session_id,
                    name=name,
                    owner=owner,
                    budget_ceiling_usd=float(budget_ceiling_usd) if budget_ceiling_usd is not None else None,
                    spent_usd=round(float(spent_usd), 6),
                    created_at=now,
                    updated_at=now,
                    meta=meta,
                )
            else:
                s = existing
                if budget_ceiling_usd is not None:
                    s.budget_ceiling_usd = float(budget_ceiling_usd)
                s.spent_usd = round(float(spent_usd), 6)
                meta = dict(s.meta or {})
                meta.setdefault("origin", "go-sqlite-authority")
                if tenant_id:
                    meta["tenant_id"] = tenant_id
                s.meta = meta
                s.updated_at = now
            self._save(s)
            return s
