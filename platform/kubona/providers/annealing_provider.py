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

    @staticmethod
    def _validate_Q(Q: np.ndarray) -> int:
        """Fail-closed Q validation (W2.18): 2-D, square, finite — else
        ProviderError. Asymmetry is NOT refused: upper-triangular Q is the
        standard QUBO convention (D-Wave form), and x^T Q x is identical for
        Q and its symmetrization, so the energy is convention-independent —
        the result labels `asymmetric_input` instead of pretending."""
        Q = np.asarray(Q, dtype=float)
        if Q.ndim != 2 or Q.shape[0] != Q.shape[1] or Q.shape[0] == 0:
            raise ProviderError(
                f"QUBO matrix must be a non-empty square 2-D array, got shape {Q.shape}")
        if not np.all(np.isfinite(Q)):
            raise ProviderError("QUBO matrix contains NaN/Inf: refusing")
        return Q.shape[0]

    def solve_qubo(self, Q: np.ndarray, n_samples: int = 64, seed: int | None = 7) -> dict[str, Any]:
        """Minimize x^T Q x over binary x.

        - n <= 16: exact enumeration (optimal: heuristic False, gap 0).
        - n > 16: dimod simulated annealing when the SDK is present (real SA,
          seeded); otherwise a labeled single-bit-flip heuristic
          (heuristic True, optimality gap UNKNOWN — never presented as
          optimal).
        """
        Q = np.asarray(Q, dtype=float)
        n = self._validate_Q(Q)
        if isinstance(n_samples, bool) or not isinstance(n_samples, int) or n_samples <= 0:
            raise ProviderError(f"n_samples must be a positive int, got {n_samples!r}")
        asymmetric = bool((Q != Q.T).any())
        if n <= 16:
            best_x, best_e = None, float("inf")
            for bits in range(1 << n):
                x = np.array([(bits >> i) & 1 for i in range(n)], dtype=float)
                e = float(x @ Q @ x)
                if e < best_e:
                    best_e, best_x = e, x
            assert best_x is not None
            return {"solution": [int(v) for v in best_x], "energy": best_e,
                    "n_qubits": n, "backend": "local-qubo-solver",
                    "has_ocean_sdk": self._has_ocean,
                    "heuristic": False, "optimality_gap": 0,
                    "asymmetric_input": asymmetric}
        if self._has_ocean:
            return self._solve_via_dimod(Q, n, n_samples, seed, asymmetric)
        rng = np.random.default_rng(seed)
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
                "n_qubits": n, "backend": "local-qubo-solver",
                "has_ocean_sdk": self._has_ocean,
                "heuristic": True, "optimality_gap": "unknown",
                "method": "single-bit-flip-heuristic_v1",
                "asymmetric_input": asymmetric}

    def _solve_via_dimod(self, Q: np.ndarray, n: int, n_samples: int,
                         seed: int | None, asymmetric: bool) -> dict[str, Any]:
        """Real simulated annealing via dimod (seeded, labeled)."""
        import dimod

        qubo = {(i, j): float(Q[i, j]) for i in range(n) for j in range(n)
                if Q[i, j] != 0.0}
        sampler = dimod.SimulatedAnnealingSampler()
        try:
            sampleset = sampler.sample_qubo(qubo, num_reads=n_samples, seed=seed)
        except TypeError:
            # Older dimod without seed support: declare nondeterminism loudly
            # rather than labeling unseeded draws seeded.
            if seed is not None:
                raise ProviderError(
                    "installed dimod does not accept a seed: refusing seeded "
                    "anneal — pass seed=None for explicitly-unseeded runs")
            sampleset = sampler.sample_qubo(qubo, num_reads=n_samples)
        best = sampleset.first
        return {"solution": [int(best.sample[i]) for i in range(n)],
                "energy": float(best.energy),
                "n_qubits": n, "backend": "dimod-simulated-annealing",
                "has_ocean_sdk": True,
                "heuristic": True, "optimality_gap": "unknown",
                "method": "dimod-simulated-annealing_v1",
                "seed": seed,
                "asymmetric_input": asymmetric}

    def health(self) -> dict[str, Any]:
        return {"provider": self.capability.provider, "ready": True, "has_ocean_sdk": self._has_ocean}
