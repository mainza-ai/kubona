"""The pluggable harness contract (§11.2) and the orchestrator loop.

The loop (karpathy/autoresearch pattern): mutate the ONE mutable artifact, run to
a fixed budget, evaluate ONE metric, keep-or-discard, repeat unattended —
simulator-only by default, with structural write-scope enforced by construction.
"""

from __future__ import annotations

import math
import random
import time
import uuid
from dataclasses import dataclass, field
from typing import Any, Callable

from kubona.provenance import Ledger

Score = float


@dataclass
class Trial:
    trial_id: str
    harness: str
    artifact: Any
    metric: Score
    valid: bool
    cost_usd: float
    ts: float
    ledger_ref: str | None = None
    meta: dict[str, Any] = field(default_factory=dict)

    def to_dict(self) -> dict[str, Any]:
        return {"trial_id": self.trial_id, "harness": self.harness,
                "artifact": self.artifact, "metric": self.metric, "valid": self.valid,
                "cost_usd": self.cost_usd, "ts": self.ts, "ledger_ref": self.ledger_ref}


class Harness:
    """Four-part contract. A harness defines WHAT may change and HOW it is scored;
    the orchestrator owns the loop. `fixed_setup` results are immutable."""

    name: str
    simulator_only: bool = True

    def __init__(self, name: str, simulator_only: bool = True):
        self.name = name
        self.simulator_only = simulator_only

    def fixed_setup(self) -> dict[str, Any]:
        """Problem instance, backend, noise model — NOT agent-editable."""
        raise NotImplementedError

    def mutable_artifact(self) -> Any:
        """The one file/object the agent is allowed to change (deep copy)."""
        raise NotImplementedError

    def metric(self, result: dict[str, Any]) -> float:
        """Single, comparable, normalized score (higher is better)."""
        raise NotImplementedError

    def validity_check(self, result: dict[str, Any]) -> bool:
        """Is this even a legal/correct output?"""
        return True

    def run(self, artifact: Any) -> dict[str, Any]:
        """Execute the harness on an artifact -> result dict (backend comes from fixed_setup)."""
        raise NotImplementedError


class Orchestrator:
    """The keep-or-discard loop. Mutates the artifact, evaluates, keeps the best."""

    def __init__(self, harness: Harness, *, budget_steps: int = 40, seed: int = 7,
                 ledger: Ledger | None = None, mutation_radius: float = 0.5,
                 resample_prob: float = 0.1):
        self.harness = harness
        self.budget_steps = budget_steps
        self.rng = random.Random(seed)
        self.ledger = ledger
        self.mutation_radius = mutation_radius
        self.resample_prob = resample_prob
        self.setup = harness.fixed_setup()

    # -- proposal strategy (the §13 meta-loop may revise THIS, not the harness) --
    def propose(self, current: Any) -> Any:
        """Random-mutation + occasional resample proposal strategy."""
        if self.rng.random() < self.resample_prob:
            return self.harness.mutable_artifact()
        if isinstance(current, float):
            return current + self.rng.gauss(0.0, self.mutation_radius)
        if isinstance(current, (tuple, list)):
            return tuple(x + self.rng.gauss(0.0, self.mutation_radius) for x in current)
        return current

    def _refine(self, history: list[Trial], half_width: float = 1.5, iters: int = 20) -> Trial:
        """Golden-section refinement around the best-so-far artifact (a 1-D scalar
        or a coordinate of the best point). Makes the search an effective
        coarse-to-fine optimizer, not just a random walk."""
        best = self.best(history)
        if best is None or not isinstance(best.artifact, float):
            return best
        phi = (math.sqrt(5) - 1) / 2
        lo, hi = best.artifact - half_width, best.artifact + half_width
        x1 = hi - phi * (hi - lo)
        x2 = lo + phi * (hi - lo)
        f1 = self.harness.metric(self.harness.run(x1))
        f2 = self.harness.metric(self.harness.run(x2))
        for _ in range(iters):
            if f1 < f2:
                lo = x1
                x1 = x2
                f1 = f2
                x2 = lo + phi * (hi - lo)
                f2 = self.harness.metric(self.harness.run(x2))
            else:
                hi = x2
                x2 = x1
                f2 = f1
                x1 = hi - phi * (hi - lo)
                f1 = self.harness.metric(self.harness.run(x1))
        best_theta = (x1 + x2) / 2
        return self.run_trial(best_theta)

    def run_trial(self, artifact: Any) -> Trial:
        result = self.harness.run(artifact)
        metric = self.harness.metric(result)
        valid = self.harness.validity_check(result)
        ledger_ref = None
        if self.ledger is not None:
            rec = self.ledger.append("trial", {"harness": self.harness.name,
                                               "artifact": str(artifact), "metric": metric,
                                               "valid": valid, "cost_usd": 0.0})
            ledger_ref = rec.record_id
        return Trial(trial_id=uuid.uuid4().hex[:12], harness=self.harness.name,
                     artifact=artifact, metric=metric, valid=valid, cost_usd=0.0,
                     ts=time.time(), ledger_ref=ledger_ref)

    def search(self, initial: Any | None = None, on_step=None) -> list[Trial]:
        """Run the keep-or-discard loop for `budget_steps`; returns all trials (sorted best first via history)."""
        current = initial if initial is not None else self.harness.mutable_artifact()
        history: list[Trial] = []
        for _ in range(self.budget_steps):
            trial = self.run_trial(current)
            history.append(trial)
            if on_step:
                on_step(trial)
            if not trial.valid:
                current = self.harness.mutable_artifact()
            else:
                current = self.propose(current)
        # coarse-to-fine: refine around the best found
        refined = self._refine(history)
        if refined is not None:
            history.append(refined)
        return history

    def best(self, history: list[Trial]) -> Trial | None:
        valid = [t for t in history if t.valid]
        return max(valid, key=lambda t: t.metric) if valid else None
