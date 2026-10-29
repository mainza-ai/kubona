"""Loop steering, mid-flight nudging, and counterfactual forking (Ring 0 & 4).

Allows human operators or higher-tier supervisory processes to:
1. Nudge variational parameters mid-flight when stuck in local minima.
2. Hot-swap classical optimizers (e.g., COBYLA -> SPSA) without killing the loop.
3. Fork counterfactual "what-if" branches from step k.
"""

from __future__ import annotations

import copy
import time
import uuid
from typing import Any, Dict, List, Optional
from kubona.control.supervisor import LoopSupervisor, LoopHandle


class SteeringService:
    def __init__(self, loop_supervisor: LoopSupervisor):
        self.loop_supervisor = loop_supervisor

    def nudge_parameters(self, loop_id: str, parameter_deltas: List[float]) -> LoopHandle:
        """Applies an additive delta to the active parameters of a running loop."""
        rec = self.loop_supervisor.get(loop_id)
        if rec.status != "running" and rec.status != "paused":
            raise ValueError(f"Cannot nudge loop in status {rec.status}")

        # Record nudge in loop history
        nudge_entry = {
            "step": rec.step,
            "event": "parameter_nudge",
            "deltas": parameter_deltas,
            "timestamp": time.time(),
        }
        rec.history.append(nudge_entry)
        self.loop_supervisor.persist(loop_id)
        return rec

    def swap_optimizer(self, loop_id: str, new_optimizer: str, options: Optional[Dict[str, Any]] = None) -> LoopHandle:
        """Hot-swaps the active optimization strategy mid-run."""
        rec = self.loop_supervisor.get(loop_id)
        swap_entry = {
            "step": rec.step,
            "event": "optimizer_swap",
            "new_optimizer": new_optimizer,
            "options": options or {},
            "timestamp": time.time(),
        }
        rec.history.append(swap_entry)
        self.loop_supervisor.persist(loop_id)
        return rec

    def fork_counterfactual(self, parent_loop_id: str, fork_step: int, fork_name: Optional[str] = None) -> LoopHandle:
        """Forks a child research run starting from step k of the parent loop.

        The child is registered DIRECTLY under its fork id. The naive
        register()-then-rekey orphans a loop entry: register() mints a
        random UUID key in _loops (whose handle no longer matches its key
        after re-keying) AND appends a register row that the append-only
        log resurrects after restart. One key, one log row — the child
        exists only under child_id, never under a stale UUID.
        """
        parent = self.loop_supervisor.get(parent_loop_id)
        if fork_step > parent.step or fork_step < 0:
            raise ValueError(f"Invalid fork step {fork_step}: parent is at step {parent.step}")

        child_id = f"fork-{uuid.uuid4().hex[:8]}"
        if child_id in self.loop_supervisor._loops:
            raise ValueError(f"fork id collision {child_id!r}: refusing to overwrite")
        name = fork_name or f"Fork of {parent.name} @ Step {fork_step}"
        
        # Prune history up to fork_step
        forked_history = [h for h in parent.history if h.get("step", 0) <= fork_step]
        forked_history.append({
            "step": fork_step,
            "event": "fork_created",
            "parent_id": parent_loop_id,
            "timestamp": time.time(),
        })

        child = LoopHandle(
            loop_id=child_id,
            name=name,
            session_id=parent.session_id,
            total_steps=parent.total_steps,
            tenant=parent.tenant,
        )
        child.step = fork_step
        child.history = forked_history
        # Re-register under the child id (no stale UUID key is ever minted);
        # same row shape LoopSupervisor.register appends, so restart replay
        # reconstructs exactly one child.
        self.loop_supervisor._loops[child_id] = child
        self.loop_supervisor._append({"event": "register", "loop": child.to_dict()})
        return child
