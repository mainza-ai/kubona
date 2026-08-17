"""Persistent, async job store + runner — the production job/session abstraction (§4.3).

A job is persisted to a JSONL store the moment it is enqueued, and every state
transition (queued -> running -> completed | failed | cancelled) is written back
AND recorded to the provenance ledger. Status reads come from the persisted store,
so long-running (real-QPU, minutes-hours) jobs survive server restarts — the
foundation for the "watch and interrupt a multi-hour loop" gate (P2) and for
horizontal scale (P5).

The runner executes enqueued jobs on a background thread pool so a slow provider
never blocks the request path; `wait=True` callers poll internally and get the
result synchronously (keeps the fast local-simulator path and agent loop UX),
while `wait=False` returns the job_id immediately for true async workflows.
"""

from __future__ import annotations

import json
import os
import threading
import time
import uuid
from concurrent.futures import ThreadPoolExecutor
from dataclasses import asdict, dataclass, field
from pathlib import Path
from typing import Any, Callable

from kubona.config import state_path


@dataclass
class Job:
    job_id: str
    provider: str
    session_id: str
    status: str = "queued"          # queued | running | completed | failed | cancelled
    created_at: float = field(default_factory=time.time)
    updated_at: float = field(default_factory=time.time)
    meta: dict[str, Any] = field(default_factory=dict)
    result: dict[str, Any] | None = None
    error: str | None = None

    def to_record(self) -> dict[str, Any]:
        d = asdict(self)
        return d

    @classmethod
    def from_record(cls, d: dict[str, Any]) -> "Job":
        return cls(**d)


class JobStore:
    """Append-only JSONL store keyed by job_id (snapshot-per-event), thread-safe."""

    def __init__(self, path: str | Path | None = None):
        self.path = Path(path) if path else state_path("jobs.jsonl")
        self.path.parent.mkdir(parents=True, exist_ok=True)
        self._lock = threading.Lock()

    def _load(self) -> dict[str, Job]:
        out: dict[str, Job] = {}
        if self.path.exists():
            for line in self.path.read_text().splitlines():
                if line.strip():
                    j = Job.from_record(json.loads(line))
                    out[j.job_id] = j
        return out

    def _save(self, j: Job) -> None:
        with open(self.path, "a") as f:
            f.write(json.dumps(j.to_record(), sort_keys=True) + "\n")

    def create(self, provider: str, session_id: str, **meta: Any) -> Job:
        with self._lock:
            j = Job(job_id=uuid.uuid4().hex[:16], provider=provider,
                    session_id=session_id, meta=meta)
            self._save(j)
            return j

    def get(self, job_id: str) -> Job | None:
        with self._lock:
            return self._load().get(job_id)

    def transition(self, job_id: str, status: str, **updates: Any) -> Job | None:
        with self._lock:
            j = self._load().get(job_id)
            if j is None:
                return None
            j.status = status
            j.updated_at = time.time()
            for k, v in updates.items():
                setattr(j, k, v)
            self._save(j)
            return j

    def jobs_for_session(self, session_id: str) -> list[Job]:
        return [j for j in self._load().values() if j.session_id == session_id]


class JobRunner:
    """Executes enqueued jobs on a background thread pool; records provenance."""

    def __init__(self, store: JobStore | None = None, ledger=None, max_workers: int = 4):
        self.store = store or JobStore()
        self.ledger = ledger  # optional provenance ledger; records each transition
        self._pool = ThreadPoolExecutor(max_workers=max_workers, thread_name_prefix="kubona-job")

    def _record(self, kind: str, payload: dict[str, Any]) -> None:
        if self.ledger is not None:
            try:
                self.ledger.append(kind, payload)
            except Exception:  # audit must never break the run path
                pass

    def enqueue(self, job_id: str, run: Callable[[], dict[str, Any]]) -> None:
        """Run `callable` in background; persist status transitions + provenance."""
        self._record("job_enqueued", {"job_id": job_id})

        def _worker() -> None:
            self.store.transition(job_id, "running")
            self._record("job_running", {"job_id": job_id})
            try:
                result = run()
                self.store.transition(job_id, "completed", result=result)
                self._record("job_completed", {"job_id": job_id, "ok": result.get("ok", True)})
            except Exception as e:  # noqa: BLE001 — job failure, not a crash
                self.store.transition(job_id, "failed", error=str(e))
                self._record("job_failed", {"job_id": job_id, "error": str(e)})

        self._pool.submit(_worker)

    def wait(self, job_id: str, timeout_s: float = 30.0, poll_s: float = 0.05) -> Job | None:
        deadline = time.time() + timeout_s
        while time.time() < deadline:
            j = self.store.get(job_id)
            if j is None:
                return None
            if j.status in ("completed", "failed", "cancelled"):
                return j
            time.sleep(poll_s)
        return self.store.get(job_id)

    def cancel(self, job_id: str) -> bool:
        j = self.store.get(job_id)
        if j is None:
            return False
        if j.status in ("queued", "running"):
            self.store.transition(job_id, "cancelled")
            self._record("job_cancelled", {"job_id": job_id})
            return True
        return False
