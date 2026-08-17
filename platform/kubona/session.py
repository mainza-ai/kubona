"""The `job_id` session abstraction (\u00a74.3) — Phase 1.

MCP servers are nominally stateless; a multi-step job (a VQE loop, an ansatz
search) is reconstructed from session state plus the provenance ledger. Sessions
persist to `KUBONA_STATE_DIR/sessions.jsonl` so state survives server restarts
and horizontal scaling (the ledger is the system of record; sessions are the
reconstruction cache).
"""

from __future__ import annotations

import json
import os
import tempfile
import time
import uuid
from dataclasses import asdict, dataclass, field
from pathlib import Path
from typing import Any

from kubona.config import state_path


def _tenant_dir(tenant: str) -> str:
    return "".join(ch for ch in tenant if ch.isalnum() or ch in "-_")


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

    def _load(self) -> dict[str, SessionState]:
        out: dict[str, SessionState] = {}
        if self.path.exists():
            for line in self.path.read_text().splitlines():
                if line.strip():
                    s = SessionState(**json.loads(line))
                    out[s.session_id] = s
        return out

    def _save(self, s: SessionState) -> None:
        with open(self.path, "a") as f:
            f.write(json.dumps(asdict(s), sort_keys=True) + "\n")

    def create(self, name: str, owner: str, budget_ceiling_usd: float | None = None) -> SessionState:
        s = SessionState(
            session_id=uuid.uuid4().hex[:16],
            name=name,
            owner=owner,
            budget_ceiling_usd=budget_ceiling_usd,
        )
        self._save(s)
        return s

    def get(self, session_id: str) -> SessionState:
        s = self._load().get(session_id)
        if s is None:
            raise KeyError(f"unknown session {session_id!r}")
        return s

    def update(self, session_id: str, **changes: Any) -> SessionState:
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
        s = self.get(session_id)
        new_spend = round(s.spent_usd + usd, 6)
        if s.budget_ceiling_usd is not None and new_spend > s.budget_ceiling_usd:
            raise ValueError(
                f"session {session_id} would exceed budget ceiling "
                f"({new_spend} > {s.budget_ceiling_usd})"
            )
        return self.update(session_id, spent_usd=new_spend)

    def close(self, session_id: str) -> SessionState:
        return self.update(session_id, status="closed")
