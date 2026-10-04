"""Non-LLM Barren Plateau & Vanishing Gradient Watchdog (Ring 0).

Evaluates the variance of quantum cost gradients across iterations.
If Var[∂θ ⟨H⟩] drops exponentially below threshold (e.g., < 1e-6 across a window),
the watchdog raises a BarrenPlateauException or pauses execution to halt wasted spend.
"""

from __future__ import annotations

import math
from typing import List


class BarrenPlateauDetectedError(RuntimeError):
    """Raised when exponential gradient vanishing indicates an untrainable plateau."""
    pass


class BarrenPlateauWatchdog:
    def __init__(self, variance_threshold: float = 1e-6, window_size: int = 8):
        self.variance_threshold = variance_threshold
        self.window_size = window_size
        self._gradient_history: List[float] = []

    def record_gradient_norm(self, norm: float) -> None:
        self._gradient_history.append(float(norm))
        if len(self._gradient_history) > self.window_size * 2:
            self._gradient_history.pop(0)

    def check(self) -> bool:
        """Returns True if barren plateau is detected, False otherwise."""
        if len(self._gradient_history) < self.window_size:
            return False

        recent = self._gradient_history[-self.window_size:]
        mean = sum(recent) / len(recent)
        variance = sum((x - mean) ** 2 for x in recent) / len(recent)

        # Barren plateau condition: both variance and magnitude are negligible
        return variance < self.variance_threshold and mean < math.sqrt(self.variance_threshold)

    def evaluate_or_raise(self) -> None:
        if self.check():
            raise BarrenPlateauDetectedError(
                f"Ring 0 Watchdog Alert: Vanishing gradient variance detected across last "
                f"{self.window_size} steps (Var < {self.variance_threshold}). "
                f"Optimizer trapped in barren plateau. Halting cloud spend."
            )
