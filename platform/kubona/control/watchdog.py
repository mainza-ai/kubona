"""Autonomous Non-LLM Mathematical Watchdog (Ring 0).

A non-LLM supervisory daemon that runs alongside iterative loops (VQE, ansatz search,
training) to mathematically detect:
1. Weak Barren Plateaus (WBP): variance of gradient/parameter step falls below threshold
   Var(Δθ) < 1e-7 across a rolling window.
2. Cost & Energy Slope Divergence: runaway loss, NaN/Inf gradients, or monotonic cost inflation.
3. Rapid Stagnation: zero metric variance over consecutive iterations.

When triggered, autonomous decisions (PAUSE, RE_SCOPE, ABORT_TO_SNAPSHOT) are executed
instantly without waiting for human review or LLM reasoning.
"""

from __future__ import annotations

import math
from dataclasses import dataclass, field
from typing import Any, Sequence


@dataclass(frozen=True)
class WatchdogThresholds:
    wbp_variance_floor: float = 1e-6  # Weak barren plateau threshold
    stagnation_steps: int = 8  # Consecutive steps with near-zero progress
    stagnation_delta: float = 1e-5
    max_divergence_slope: float = 5.0  # Slope threshold for diverging cost/energy


@dataclass
class WatchdogVerdict:
    action: str  # "CONTINUE" | "PAUSE" | "RE_SCOPE" | "ABORT_TO_SNAPSHOT"
    reason: str
    variance: float | None = None
    slope: float | None = None
    step: int = 0

    def to_dict(self) -> dict[str, Any]:
        return {
            "action": self.action,
            "reason": self.reason,
            "variance": self.variance,
            "slope": self.slope,
            "step": self.step,
        }


class Ring0Watchdog:
    """Mathematical supervisor guarding against barren plateaus and computational waste."""

    def __init__(self, thresholds: WatchdogThresholds | None = None):
        self.thresholds = thresholds or WatchdogThresholds()
        self.metrics_history: list[float] = []
        self.params_history: list[list[float]] = []
        self.snapshot_step: int = 0
        self.snapshot_metric: float | None = None

    def record_step(self, step: int, metric: float, params: Sequence[float] | float) -> WatchdogVerdict:
        """Evaluates iteration state and determines immediate Ring 0 control action."""
        if math.isnan(metric) or math.isinf(metric):
            return WatchdogVerdict(
                action="ABORT_TO_SNAPSHOT",
                reason=f"Numerical instability detected: metric is {metric}",
                step=step,
            )

        # Safely convert params to numeric floats for vector difference
        if isinstance(params, (int, float)):
            p_list = [float(params)]
        elif isinstance(params, (list, tuple)):
            try:
                p_list = [float(x) for x in params]
            except (ValueError, TypeError):
                # Non-numeric artifact representation (e.g. string/dict)
                p_list = []
        else:
            p_list = []

        self.metrics_history.append(metric)
        self.params_history.append(p_list)

        # Update snapshot on first step or when establishing a valid baseline
        if len(self.metrics_history) == 1:
            self.snapshot_step = step
            self.snapshot_metric = metric
            return WatchdogVerdict(action="CONTINUE", reason="Initial step baseline", step=step)

        # 1. Barren plateau detection via parameter step variance over sliding window (last 5)
        window = 5
        if len(self.params_history) >= window and all(len(p) > 0 for p in self.params_history[-window:]):
            recent_params = self.params_history[-window:]
            # Ensure equal dimensions across window
            if len({len(p) for p in recent_params}) == 1:
                deltas = []
                for i in range(1, len(recent_params)):
                    d = sum((a - b) ** 2 for a, b in zip(recent_params[i], recent_params[i - 1]))
                    deltas.append(math.sqrt(d))

                mean_delta = sum(deltas) / len(deltas)
                var_delta = sum((x - mean_delta) ** 2 for x in deltas) / len(deltas)

                if var_delta < self.thresholds.wbp_variance_floor and mean_delta < 1e-4:
                    return WatchdogVerdict(
                        action="RE_SCOPE",
                        reason=f"Weak Barren Plateau (WBP) detected: gradient step variance {var_delta:.2e} < {self.thresholds.wbp_variance_floor:.2e}",
                        variance=var_delta,
                        step=step,
                    )

        # 2. Rapid stagnation detection
        stag_n = self.thresholds.stagnation_steps
        if len(self.metrics_history) >= stag_n:
            recent_metrics = self.metrics_history[-stag_n:]
            metric_range = max(recent_metrics) - min(recent_metrics)
            if metric_range < self.thresholds.stagnation_delta:
                return WatchdogVerdict(
                    action="PAUSE",
                    reason=f"Optimization stagnant over {stag_n} steps (range {metric_range:.2e} < {self.thresholds.stagnation_delta:.2e})",
                    variance=metric_range,
                    step=step,
                )

        # 3. Slope divergence check
        if len(self.metrics_history) >= 4:
            m_prev = self.metrics_history[-4]
            m_curr = self.metrics_history[-1]
            slope = (m_curr - m_prev) / 3.0
            # If optimizing energy (seeking negative), sharp positive slope indicates divergence
            if slope > self.thresholds.max_divergence_slope:
                return WatchdogVerdict(
                    action="ABORT_TO_SNAPSHOT",
                    reason=f"Cost slope divergence: {slope:.2f} > {self.thresholds.max_divergence_slope:.2f}",
                    slope=slope,
                    step=step,
                )

        return WatchdogVerdict(action="CONTINUE", reason="Nominal progress", step=step)
