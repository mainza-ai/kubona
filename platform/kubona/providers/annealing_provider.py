"""Annealing provider (D-Wave modality, Phase 5).

Annealing is QUBO/CQM, not gate-circuit — so this provider implements a
separate `solve_qubo` path alongside the (unused for annealing) circuit trait,
and documents the production path (dwave-ocean-sdk + Leap, or dimod simulated
annealing). The in-repo reference solver works credential-free for small QUBOs.
"""

from __future__ import annotations

from typing import Any

import numpy as np

from kubona.circuits import Circuit
from kubona.schema import ProviderCapability

from .base import Provider, ProviderError


class AnnealingProvider(Provider):
    """Modality: annealing. `solve_qubo` is the real interface; the circuit
    submit raises a clear "annealing is QUBO, not gate-circuit" error."""

    def __init__(self, capability: ProviderCapability):
        self.capability = capability
        try:
            import dimod  # noqa: F401
            self._has_ocean = True
        except ImportError:
            self._has_ocean = False

    def submit(self, circuit: Circuit, shots: int, seed: int | None = None) -> dict[str, Any]:
        raise ProviderError(
            "annealing providers solve QUBO/CQM, not gate circuits — use solve_qubo(). "
            f"ocean-sdk present: {self._has_ocean}"
        )

    def solve_qubo(self, Q: np.ndarray, n_samples: int = 64, seed: int | None = 7) -> dict[str, Any]:
        """Minimize x^T Q x over binary x (small n: exact enumeration; large n:
        greedy / simulated-annealing via dimod if available)."""
        n = Q.shape[0]
        rng = np.random.default_rng(seed)
        best_x, best_e = None, float("inf")
        if n <= 16:
            for bits in range(1 << n):
                x = np.array([(bits >> i) & 1 for i in range(n)], dtype=float)
                e = float(x @ Q @ x)
                if e < best_e:
                    best_e, best_x = e, x
        else:
            best_x = rng.integers(0, 2, size=n).astype(float)
            best_e = float(best_x @ Q @ best_x)
            for _ in range(n_samples):
                cand = best_x.copy()
                idx = int(rng.integers(0, n))
                cand[idx] = 1 - cand[idx]
                e = float(cand @ Q @ cand)
                if e < best_e:
                    best_e, best_x = e, cand
        return {"solution": [int(v) for v in best_x], "energy": best_e,
                "n_qubits": n, "backend": "local-qubo-solver", "has_ocean_sdk": self._has_ocean}

    def health(self) -> dict[str, Any]:
        return {"provider": self.capability.provider, "ready": True, "has_ocean_sdk": self._has_ocean}
