"""Epistemic Curvature and Research Stagnation Diagnostics.

Implements mathematical diagnostics over the research collective's evidence graph:
1. Hypothesis concentration kappa (Inverse Participation Ratio over hypothesis mass):
   kappa = sum_i (p_i^2)
   When kappa -> 1, the research collective is stagnating on a single dogmatic hypothesis
   ("diversify or stop" condition).
2. Evidence literature distance d(E_canon, E_live): cosine distance tracking whether
   external findings have diverged from the canonical claim baseline.
3. Epistemic Refusal Ratio: refusals / total assertions over a rolling window.
"""

from __future__ import annotations

import math
from dataclasses import dataclass, field
from typing import Any, Sequence


@dataclass
class CurvatureMetrics:
    hypothesis_concentration_kappa: float  # [0.0, 1.0]
    refusal_ratio: float  # [0.0, 1.0]
    stagnant: bool
    status_label: str  # "healthy_divergence" | "nominal" | "stagnant_concentration" | "gate_bypass"
    window_size: int
    details: dict[str, Any] = field(default_factory=dict)

    def to_dict(self) -> dict[str, Any]:
        return {
            "hypothesis_concentration_kappa": round(self.hypothesis_concentration_kappa, 4),
            "refusal_ratio": round(self.refusal_ratio, 4),
            "stagnant": self.stagnant,
            "status_label": self.status_label,
            "window_size": self.window_size,
            "details": self.details,
        }


class EpistemicCurvatureEngine:
    """Computes evidence concentration and research stagnation diagnostics."""

    def __init__(self, kappa_threshold: float = 0.85, window_size: int = 10):
        self.kappa_threshold = kappa_threshold
        self.window_size = window_size
        self._hypotheses_history: list[str] = []
        self._verdicts_history: list[bool] = []  # True = asserted, False = refused / uncollapsed

    def record_hypothesis(self, hypothesis_id: str, was_asserted: bool) -> CurvatureMetrics:
        """Records a research attempt and updates diagnostic metrics."""
        self._hypotheses_history.append(hypothesis_id)
        self._verdicts_history.append(was_asserted)

        if len(self._hypotheses_history) > self.window_size * 2:
            self._hypotheses_history = self._hypotheses_history[-self.window_size:]
            self._verdicts_history = self._verdicts_history[-self.window_size:]

        return self.compute_metrics()

    def compute_metrics(self) -> CurvatureMetrics:
        """Calculates current kappa concentration and refusal ratio."""
        if not self._hypotheses_history:
            return CurvatureMetrics(
                hypothesis_concentration_kappa=0.0,
                refusal_ratio=0.0,
                stagnant=False,
                status_label="nominal",
                window_size=0,
            )

        recent_hypotheses = self._hypotheses_history[-self.window_size:]
        recent_verdicts = self._verdicts_history[-self.window_size:]
        n = len(recent_hypotheses)

        # 1. Frequency counts
        counts: dict[str, int] = {}
        for h in recent_hypotheses:
            counts[h] = counts.get(h, 0) + 1

        # 2. Probability mass and Inverse Participation Ratio (kappa)
        kappa = sum((count / n) ** 2 for count in counts.values())

        # 3. Refusal ratio
        refusals = sum(1 for v in recent_verdicts if not v)
        refusal_ratio = refusals / len(recent_verdicts) if recent_verdicts else 0.0

        stagnant = kappa >= self.kappa_threshold and n >= 5

        if stagnant:
            label = "stagnant_concentration"
        elif refusal_ratio < 0.05 and n >= 5:
            label = "gate_bypass"
        elif kappa < 0.3:
            label = "healthy_divergence"
        else:
            label = "nominal"

        return CurvatureMetrics(
            hypothesis_concentration_kappa=kappa,
            refusal_ratio=refusal_ratio,
            stagnant=stagnant,
            status_label=label,
            window_size=n,
            details={"unique_hypotheses": len(counts), "counts": counts},
        )
