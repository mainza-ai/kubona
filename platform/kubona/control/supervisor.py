"""Loop supervisor — the live-session backend for the console.

Registers in-flight loops (a VQE loop, an ansatz search) with heartbeat state so
the control plane can answer "what is the agent doing now and why", pause, or
kill a loop — from the console, without touching job submission (read/approve
only; killing is an override, never a new execution path).
"""

from __future__ import annotations

import fcntl
import json
import os
import threading
import time
import uuid
from dataclasses import dataclass, field
from pathlib import Path
from typing import Any, Callable

from kubona.config import state_path
from kubona.session import SessionStore


MAX_HISTORY = 5000


@dataclass
class LoopHandle:
    loop_id: str
    name: str
    session_id: str
    status: str = "running"  # running | paused | completed | killed
    step: int = 0
    total_steps: int = 0
    current_step_desc: str = ""
    started_at: float = field(default_factory=time.time)
    paused_at: float | None = None
    heartbeat: float = field(default_factory=time.time)
    history: list[dict[str, Any]] = field(default_factory=list)  # 4D flight recorder trajectory
    meta: dict[str, Any] = field(default_factory=dict)
    # Owning lab tenant (isolation label). The gateway capability gate
    # authorizes WHO may call; this label scopes WHICH loops they may
    # see or touch (same requester convention as provenance reads).
    tenant: str = "default"

    def to_dict(self) -> dict[str, Any]:
        return {k: v for k, v in self.__dict__.items() if k != "meta"} | {"meta": self.meta}


class LoopSupervisor:
    """Tracks loops; runs one step at a time with pause/kill checks and flight recorder history.

    State is append-only JSONL under the shared state root (or an injected
    path). A restart reconstructs loops instead of silently forgetting them;
    loops that were ``running`` at shutdown are recovered as ``paused`` — a
    process restart is not evidence that the loop is still alive.
    """

    def __init__(self, sessions: SessionStore | None = None,
                 path: str | os.PathLike | None = None):
        self.path = Path(path) if path is not None else state_path("loops.jsonl")
        self.path.parent.mkdir(parents=True, exist_ok=True)
        self._lock = threading.Lock()
        self._lock_path = self.path.parent / (self.path.name + ".lock")
        self._loops: dict[str, LoopHandle] = {}
        self.sessions = sessions or SessionStore()
        self._load()

    def _exclusive(self):
        class _Ctx:
            def __init__(self, outer):
                self.outer = outer
                self.lf = None

            def __enter__(self):
                self.outer._lock.acquire()
                self.lf = open(self.outer._lock_path, "a+b")
                fcntl.flock(self.lf.fileno(), fcntl.LOCK_EX)
                return self

            def __exit__(self, *exc):
                try:
                    fcntl.flock(self.lf.fileno(), fcntl.LOCK_UN)
                finally:
                    self.lf.close()
                    self.outer._lock.release()
                return False
        return _Ctx(self)

    def _load(self) -> None:
        if not self.path.exists():
            return
        for line in self.path.read_text().splitlines():
            if not line.strip():
                continue
            try:
                row = json.loads(line)
            except ValueError:
                continue
            try:
                event = row.get("event")
                if event == "register" or event == "snapshot":
                    h = LoopHandle(**row["loop"])
                    self._loops[h.loop_id] = h
                elif event == "step":
                    h = self._loops.get(row["loop_id"])
                    if h is None:
                        continue
                    h.step = int(row["step"])
                    h.current_step_desc = row.get("desc", "")
                    h.heartbeat = float(row.get("timestamp", h.heartbeat))
                    h.status = row.get("status", h.status)
                    if "entry" in row:
                        h.history.append(row["entry"])
                elif event == "status":
                    h = self._loops.get(row["loop_id"])
                    if h is None:
                        continue
                    h.status = row["status"]
                    h.paused_at = row.get("paused_at")
                    h.current_step_desc = row.get("current_step_desc", h.current_step_desc)
                    h.heartbeat = float(row.get("timestamp", h.heartbeat))
                else:
                    # Legacy full-snapshot row (pre-event log).
                    h = LoopHandle(**row)
                    self._loops[h.loop_id] = h
                if len(h.history) > MAX_HISTORY:
                    h.history = h.history[-MAX_HISTORY:]
            except (TypeError, KeyError):
                continue
        for h in self._loops.values():
            if h.status == "running":
                h.status = "paused"
                h.paused_at = time.time()
                h.meta = dict(h.meta or {})
                h.meta["recovered_after_restart"] = True

    def _append(self, row: dict[str, Any]) -> None:
        with self._exclusive():
            with open(self.path, "a") as f:
                f.write(json.dumps(row, sort_keys=True) + "\n")
                f.flush()
                os.fsync(f.fileno())

    def _snapshot(self, h: LoopHandle) -> None:
        self._append({"event": "snapshot", "loop": h.to_dict()})

    def _status_event(self, h: LoopHandle) -> None:
        self._append({
            "event": "status", "loop_id": h.loop_id, "status": h.status,
            "paused_at": h.paused_at, "current_step_desc": h.current_step_desc,
            "timestamp": time.time(),
        })

    def persist(self, loop_id: str) -> LoopHandle:
        h = self._loops[loop_id]
        self._snapshot(h)
        return h

    def register(self, name: str, session_id: str, total_steps: int,
                 meta: dict[str, Any] | None = None, tenant: str = "default") -> LoopHandle:
        h = LoopHandle(loop_id=uuid.uuid4().hex[:16], name=name, session_id=session_id,
                       total_steps=total_steps, meta=meta or {}, tenant=tenant or "default")
        self._loops[h.loop_id] = h
        self._append({"event": "register", "loop": h.to_dict()})
        return h

    def get_for(self, loop_id: str, requester: str | None) -> LoopHandle:
        """Fetch a loop for a principal (tenant isolation).

        Unknown ids raise KeyError (unchanged); a loop owned by another
        tenant raises PermissionError — callers render it as a refusal,
        never as existence-or-absence signal beyond the refusal itself.
        requester None means ambient access: sees default-tenant loops
        only (never another tenant's).
        """
        h = self._loops[loop_id]
        want = requester or "default"
        if h.tenant != want:
            raise PermissionError(
                f"loop {loop_id!r} belongs to another lab tenant: refusing"
            )
        return h

    def get(self, loop_id: str) -> LoopHandle:
        return self._loops[loop_id]

    def list(self) -> list[dict[str, Any]]:
        return [h.to_dict() for h in self._loops.values()]

    def list_for(self, requester: str | None) -> list[dict[str, Any]]:
        """Tenant-scoped listing (never global-with-redaction: existence
        is information, same doctrine as provenance reads)."""
        want = requester or "default"
        return [h.to_dict() for h in self._loops.values() if h.tenant == want]

    # -- step gate: called by the loop before each step -------------------
    def step(self, loop_id: str, step_desc: str, metric: float | None = None,
             params: Any = None, watchdog_verdict: str | None = None) -> bool:
        """Advance the loop if it should run; False means paused/killed -> stop."""
        h = self._loops[loop_id]
        h.heartbeat = time.time()
        if h.status == "killed":
            return False
        if h.status == "paused":
            return False
        h.step += 1
        h.current_step_desc = step_desc
        # Append to 4D flight recorder history
        entry = {
            "step": h.step,
            "desc": step_desc,
            "metric": metric,
            "params": params,
            "watchdog": watchdog_verdict,
            "timestamp": time.time(),
        }
        h.history.append(entry)
        if len(h.history) > MAX_HISTORY:
            h.history = h.history[-MAX_HISTORY:]
        self._append({"event": "step", "loop_id": h.loop_id, "step": h.step,
                      "desc": step_desc, "status": h.status,
                      "timestamp": entry["timestamp"], "entry": entry})
        return True

    def pause(self, loop_id: str) -> LoopHandle:
        h = self._loops[loop_id]
        if h.status == "running":
            h.status = "paused"
            h.paused_at = time.time()
        self._status_event(h)
        return h

    def resume(self, loop_id: str) -> LoopHandle:
        h = self._loops[loop_id]
        if h.status == "paused":
            h.status = "running"
        self._status_event(h)
        return h

    def kill(self, loop_id: str) -> LoopHandle:
        h = self._loops[loop_id]
        h.status = "killed"
        h.current_step_desc = "interrupted by operator"
        self._status_event(h)
        return h

    def complete(self, loop_id: str) -> LoopHandle:
        h = self._loops[loop_id]
        h.status = "completed"
        self._status_event(h)
        return h
