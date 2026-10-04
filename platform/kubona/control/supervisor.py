"""Loop supervisor — the live-session backend for the console.

Registers in-flight loops (a VQE loop, an ansatz search) with heartbeat state so
the control plane can answer "what is the agent doing now and why", pause, or
kill a loop — from the console, without touching job submission (read/approve
only; killing is an override, never a new execution path).
"""

from __future__ import annotations

import time
import uuid
from dataclasses import dataclass, field
from typing import Any, Callable

from kubona.session import SessionStore


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

    def to_dict(self) -> dict[str, Any]:
        return {k: v for k, v in self.__dict__.items() if k != "meta"} | {"meta": self.meta}


class LoopSupervisor:
    """Tracks loops; runs one step at a time with pause/kill checks and flight recorder history."""

    def __init__(self, sessions: SessionStore | None = None):
        self._loops: dict[str, LoopHandle] = {}
        self.sessions = sessions or SessionStore()

    def register(self, name: str, session_id: str, total_steps: int,
                 meta: dict[str, Any] | None = None) -> LoopHandle:
        h = LoopHandle(loop_id=uuid.uuid4().hex[:16], name=name, session_id=session_id,
                       total_steps=total_steps, meta=meta or {})
        self._loops[h.loop_id] = h
        return h

    def get(self, loop_id: str) -> LoopHandle:
        return self._loops[loop_id]

    def list(self) -> list[dict[str, Any]]:
        return [h.to_dict() for h in self._loops.values()]

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
        h.history.append({
            "step": h.step,
            "desc": step_desc,
            "metric": metric,
            "params": params,
            "watchdog": watchdog_verdict,
            "timestamp": time.time(),
        })
        return True

    def pause(self, loop_id: str) -> LoopHandle:
        h = self._loops[loop_id]
        if h.status == "running":
            h.status = "paused"
            h.paused_at = time.time()
        return h

    def resume(self, loop_id: str) -> LoopHandle:
        h = self._loops[loop_id]
        if h.status == "paused":
            h.status = "running"
        return h

    def kill(self, loop_id: str) -> LoopHandle:
        h = self._loops[loop_id]
        h.status = "killed"
        h.current_step_desc = "interrupted by operator"
        return h

    def complete(self, loop_id: str) -> LoopHandle:
        h = self._loops[loop_id]
        h.status = "completed"
        return h
