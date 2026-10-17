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

Cancellation is cooperative (W0.11): cancel() sets the job's cancel event and
transitions queued|running -> cancelled. A cancelled-queued job never runs; a
cancel landing mid-run cannot recall provider-side work (providers expose no
cancel API — stated, not hidden) but the worker never overwrites the terminal
'cancelled' status, and the ledger records job_cancelled_mid_run. The old code
just relabeled while the worker kept spending and then stamped 'completed' over
the cancellation.
"""

from __future__ import annotations

import fcntl
import json
import threading
import time
import uuid
from concurrent.futures import ThreadPoolExecutor
from contextlib import contextmanager
from dataclasses import asdict, dataclass, field
from pathlib import Path
from typing import Any, Callable, Iterator

from kubona.config import state_path
from kubona.logging import get_logger
from kubona.retention import maybe_rotate, rotated_paths

log = get_logger("jobs")

# Allowlisted transition graph (W0.11): terminal states are immutable —
# completed/failed/cancelled jobs can never resurrect (the old transition()
# allowed any->any, so cancelled jobs resurrected as completed with
# fabricated results). Unknown ids return None (unchanged); illegal edges
# raise InvalidTransitionError (fail-loud, never silent).
TRANSITIONS: dict[str, frozenset[str]] = {
    "queued": frozenset({"running", "cancelled"}),
    "running": frozenset({"completed", "failed", "cancelled"}),
    "completed": frozenset(),
    "failed": frozenset(),
    "cancelled": frozenset(),
}
TERMINAL = frozenset({"completed", "failed", "cancelled"})

# transition() may only write these fields besides status/updated_at (W0.11):
# the old setattr on arbitrary keys let any caller rewrite job_id and friends.
MUTABLE_FIELDS = frozenset({"result", "error"})


class InvalidTransitionError(ValueError):
    """Illegal lifecycle edge (e.g. cancelled -> completed)."""


class RunnerBusyError(RuntimeError):
    """Admission-time rejection: more outstanding jobs than the farm allows."""


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
        self._lock_path = self.path.parent / (self.path.name + ".lock")

    @contextmanager
    def _exclusive(self) -> Iterator[None]:
        """Serialize load-modify-append across threads AND processes sharing
        the file (W0.11): two gateway workers interleaving read-modify-append
        used to double-run one QPU job or lose terminal transitions."""
        with self._lock:
            with open(self._lock_path, "a+b") as lf:
                fcntl.flock(lf.fileno(), fcntl.LOCK_EX)
                try:
                    yield
                finally:
                    fcntl.flock(lf.fileno(), fcntl.LOCK_UN)

    def _load(self) -> dict[str, Job]:
        out: dict[str, Job] = {}
        skipped = 0
        # I2: replay every retained segment oldest-first (rotation must
        # never hide rows — newer snapshots overwrite older ones).
        for seg in rotated_paths(self.path):
            for line in seg.read_text().splitlines():
                if not line.strip():
                    continue
                try:
                    j = Job.from_record(json.loads(line))
                except (ValueError, TypeError, KeyError):
                    skipped += 1  # corrupt line quarantined, never fatal
                    continue
                out[j.job_id] = j
        if skipped:
            log.warning("job store skipped corrupt lines", extra={"skipped": skipped})
        return out

    def _save(self, j: Job) -> None:
        # I2: rotate-before-append INSIDE the caller's lock (all _save
        # callers hold _exclusive): the triggering write lands in a fresh
        # file, and rotation can never race an append.
        maybe_rotate(self.path)
        with open(self.path, "a") as f:
            f.write(json.dumps(j.to_record(), sort_keys=True) + "\n")

    def create(self, provider: str, session_id: str, **meta: Any) -> Job:
        with self._exclusive():
            j = Job(job_id=uuid.uuid4().hex[:16], provider=provider,
                    session_id=session_id, meta=meta)
            self._save(j)
            return j

    def get(self, job_id: str) -> Job | None:
        with self._lock:
            return self._load().get(job_id)

    def transition(self, job_id: str, status: str, **updates: Any) -> Job | None:
        with self._exclusive():
            j = self._load().get(job_id)
            if j is None:
                return None
            allowed = TRANSITIONS.get(j.status)
            if allowed is None:
                raise InvalidTransitionError(f"job {job_id} has unknown status {j.status!r}: refused")
            if status not in allowed:
                raise InvalidTransitionError(
                    f"job {job_id} cannot go {j.status} -> {status}: terminal states are immutable"
                )
            for k in updates:
                if k not in MUTABLE_FIELDS:
                    raise InvalidTransitionError(f"job field {k!r} is not mutable via transition")
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
        # Backpressure (W0.11): at most max_workers jobs outstanding
        # (queued + running). Beyond that enqueue refuses explicitly instead
        # of silently queueing hours-long QPU work behind a full farm.
        self._slots = threading.Semaphore(max_workers)
        self._cancel: dict[str, threading.Event] = {}
        self._cancel_lock = threading.Lock()

    def _cancel_event(self, job_id: str) -> threading.Event:
        with self._cancel_lock:
            ev = self._cancel.get(job_id)
            if ev is None:
                ev = threading.Event()
                self._cancel[job_id] = ev
            return ev

    def _record(self, kind: str, payload: dict[str, Any]) -> None:
        if self.ledger is not None:
            try:
                self.ledger.append(kind, payload)
            except Exception as e:  # audit must never break the run path...
                # ...but silence hid audit gaps exactly where money moved
                # (W0.11). Loud log + the job row itself remains the record.
                log.warning("ledger append failed for job event",
                            extra={"event": kind, "error": str(e)})

    def enqueue(self, job_id: str, run: Callable[[], dict[str, Any]]) -> None:
        """Run `callable` in background; persist status transitions + provenance.

        The job must exist (unknown ids refuse BEFORE any ledger record —
        the old code recorded job_enqueued for jobs that didn't exist).
        Raises RunnerBusyError when the farm is full (explicit backpressure).
        """
        if self.store.get(job_id) is None:
            raise KeyError(f"unknown job {job_id!r}: refusing to enqueue ghosts")
        if not self._slots.acquire(blocking=False):
            raise RunnerBusyError(
                "job farm full: explicit rejection, retry or cancel outstanding work"
            )
        self._record("job_enqueued", {"job_id": job_id})

        def _worker() -> None:
            try:
                if self._cancel_event(job_id).is_set():
                    # Cancelled while queued: never run, keep terminal.
                    self._record("job_cancelled_before_run", {"job_id": job_id})
                    return
                try:
                    self.store.transition(job_id, "running")
                except InvalidTransitionError:
                    # Lost the race with cancel (terminal already): leave it.
                    return
                self._record("job_running", {"job_id": job_id})
                try:
                    result = run()
                except Exception as e:  # noqa: BLE001 — job failure, not a crash
                    if self._cancel_event(job_id).is_set():
                        self._record("job_cancelled_mid_run",
                                     {"job_id": job_id, "error": str(e),
                                      "note": "provider work could not be recalled; result discarded"})
                    else:
                        self.store.transition(job_id, "failed", error=str(e))
                        self._record("job_failed", {"job_id": job_id, "error": str(e)})
                    return
                if self._cancel_event(job_id).is_set():
                    # Cancelled mid-run: result discarded, terminal kept.
                    # Provider-side work cannot be recalled (no cancel API) —
                    # recorded honestly instead of stamped completed.
                    self._record("job_cancelled_mid_run",
                                 {"job_id": job_id,
                                  "note": "provider work could not be recalled; result discarded"})
                    return
                self.store.transition(job_id, "completed", result=result)
                ok = result.get("ok", True) if isinstance(result, dict) else True
                self._record("job_completed", {"job_id": job_id, "ok": ok})
            finally:
                self._slots.release()
                with self._cancel_lock:
                    self._cancel.pop(job_id, None)

        self._pool.submit(_worker)

    def wait(self, job_id: str, timeout_s: float = 30.0, poll_s: float = 0.05) -> Job:
        """Poll until terminal. Unknown ids raise KeyError (was: conflated
        with timeout, both returning None-shaped answers). Timeouts raise
        TimeoutError naming the last-seen status (was: silent non-terminal
        return the caller misread as done)."""
        deadline = time.time() + timeout_s
        last: Job | None = None
        while time.time() < deadline:
            j = self.store.get(job_id)
            if j is None:
                raise KeyError(f"unknown job {job_id!r}")
            if j.status in ("completed", "failed", "cancelled"):
                return j
            last = j
            time.sleep(poll_s)
        seen = last.status if last else "unknown"
        raise TimeoutError(f"job {job_id} still {seen} after {timeout_s}s: poll again, don't assume done")

    def cancel(self, job_id: str) -> bool:
        """Cooperative cancel: set the fence event, then transition
        queued|running -> cancelled. Terminal or unknown ids return False.
        In-flight provider work runs to completion underneath but its result
        is discarded and the status stays cancelled (W0.11)."""
        j = self.store.get(job_id)
        if j is None:
            return False
        if j.status not in ("queued", "running"):
            return False
        self._cancel_event(job_id).set()
        try:
            self.store.transition(job_id, "cancelled")
        except InvalidTransitionError:
            # Settled terminal between the read and the transition: the
            # terminal state wins, cancellation is a no-op.
            return False
        self._record("job_cancelled", {"job_id": job_id})
        return True
