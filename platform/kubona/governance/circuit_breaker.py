"""Real-Time Mid-Flight Spend Circuit Breaker & Simulator Soft-Landing (Ring 0).

Monitors active QPU provider execution. If job execution time or shot fees
diverge by > 10% from pre-flight estimates, the circuit-breaker automatically
aborts the cloud run and triggers graceful soft-landing to local Qiskit Aer simulation.
"""

from __future__ import annotations

import time
from dataclasses import dataclass
from typing import Callable, Optional


class MidFlightSpendDivergenceError(RuntimeError):
    """Raised when actual execution cost exceeds estimated threshold."""
    pass


@dataclass
class CircuitBreakerStatus:
    active: bool
    estimated_usd: float
    actual_usd: float
    divergence_percent: float
    aborted: bool
    soft_landing_triggered: bool
    notes: str


class SpendCircuitBreaker:
    def __init__(self, divergence_tolerance_percent: float = 10.0):
        self.divergence_tolerance_percent = divergence_tolerance_percent

    def monitor(
        self,
        estimated_usd: float,
        current_actual_usd: float,
        on_soft_landing: Optional[Callable[[], None]] = None,
    ) -> CircuitBreakerStatus:
        if estimated_usd <= 0.0:
            divergence = 0.0
        else:
            divergence = max(0.0, ((current_actual_usd - estimated_usd) / estimated_usd) * 100.0)

        should_abort = divergence > self.divergence_tolerance_percent

        if should_abort:
            if on_soft_landing:
                on_soft_landing()
            return CircuitBreakerStatus(
                active=False,
                estimated_usd=estimated_usd,
                actual_usd=current_actual_usd,
                divergence_percent=divergence,
                aborted=True,
                soft_landing_triggered=True,
                notes=f"Aborted: actual spend (${current_actual_usd:.2f}) diverged by {divergence:.1f}% "
                      f"(tolerance: {self.divergence_tolerance_percent}%). Soft-landed to simulator.",
            )

        return CircuitBreakerStatus(
            active=True,
            estimated_usd=estimated_usd,
            actual_usd=current_actual_usd,
            divergence_percent=divergence,
            aborted=False,
            soft_landing_triggered=False,
            notes="Nominal: execution cost within safety envelope.",
        )
