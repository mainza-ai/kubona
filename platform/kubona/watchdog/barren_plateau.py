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


# =========================================================================
# 2.2.B McCLEAN-CEREZO GRADIENT VARIANCE MONITOR & NEGATIVE CERTIFICATES
# =========================================================================

import numpy as np
from typing import Callable, Dict, Optional, Tuple, Any
from kubona.epistemics.continuum import NegativeCertificate


class McCleanCerezoBarrenPlateauDetector:
    """Non-LLM McClean-Cerezo barren plateau gradient variance monitor.

    Computes numerical variance of partial derivatives across parameter perturbations:
        Var_θ [ ∂⟨H⟩ / ∂θ_k ]
    If variance drops below 1e-5 or scales as O(2^{-n}), trips the watchdog,
    halts parameter optimization, and emits a cryptographic NegativeCertificate.
    """

    def __init__(
        self,
        num_qubits: int = 4,
        variance_floor: float = 1e-5,
        exponential_scale_factor: float = 2.0,
        num_perturbations: int = 20,
        finite_diff_eps: float = 1e-3,
    ):
        self.num_qubits = num_qubits
        self.variance_floor = variance_floor
        self.exponential_scale_factor = exponential_scale_factor
        self.num_perturbations = num_perturbations
        self.finite_diff_eps = finite_diff_eps

    def compute_gradient_variance(
        self,
        cost_fn: Callable[[np.ndarray], float],
        base_parameters: np.ndarray,
        rng: Optional[np.random.Generator] = None,
    ) -> Dict[str, Any]:
        """Calculates Var_θ [ ∂⟨H⟩ / ∂θ_k ] across parameter space perturbations."""
        if rng is None:
            rng = np.random.default_rng(42)

        params = np.asarray(base_parameters, dtype=float)
        num_params = len(params)
        gradients = []

        for _ in range(self.num_perturbations):
            # Perturb around parameter manifold (or uniform random Haar-like exploration)
            perturbed = params + rng.normal(0.0, 0.5, size=num_params)
            base_val = cost_fn(perturbed)

            grad_i = np.zeros(num_params)
            for k in range(num_params):
                p_plus = perturbed.copy()
                p_plus[k] += self.finite_diff_eps
                val_plus = cost_fn(p_plus)
                grad_i[k] = (val_plus - base_val) / self.finite_diff_eps

            gradients.append(grad_i)

        grads_arr = np.array(gradients)  # shape (M, num_params)
        param_variances = np.var(grads_arr, axis=0)  # shape (num_params,)
        mean_variance = float(np.mean(param_variances))
        max_variance = float(np.max(param_variances))

        # Exponential threshold: O(2^{-n})
        exp_threshold = self.exponential_scale_factor * (2.0 ** (-float(self.num_qubits)))
        effective_threshold = max(self.variance_floor, exp_threshold)

        is_plateau = mean_variance < effective_threshold or mean_variance < self.variance_floor

        return {
            "num_qubits": self.num_qubits,
            "num_parameters": num_params,
            "num_perturbations": self.num_perturbations,
            "param_variances": param_variances.tolist(),
            "mean_variance": mean_variance,
            "max_variance": max_variance,
            "variance_floor": self.variance_floor,
            "exponential_threshold": exp_threshold,
            "effective_threshold": effective_threshold,
            "is_barren_plateau": is_plateau,
        }

    def check_and_certify(
        self,
        cost_fn: Callable[[np.ndarray], float],
        base_parameters: np.ndarray,
        ansatz_pattern: str = "HardwareEfficient_DeepAnsatz",
        hypothesis_hash: str = "0" * 64,
        target_system: str = "QuantumSystem",
    ) -> Tuple[bool, Optional[NegativeCertificate], Dict[str, Any]]:
        """Evaluates gradient variance and emits a NegativeCertificate if trapped."""
        metrics = self.compute_gradient_variance(cost_fn, base_parameters)
        if metrics["is_barren_plateau"]:
            cert_id = f"cert_bp_{hypothesis_hash[:12]}_{ansatz_pattern[:16]}"
            cert = NegativeCertificate(
                certificate_id=cert_id,
                hypothesis_hash=hypothesis_hash,
                refutation_type="BARREN_PLATEAU",
                evidence_summary=(
                    f"Vanishing gradient variance ({metrics['mean_variance']:.2e} < {metrics['effective_threshold']:.2e}) "
                    f"under {metrics['num_qubits']} qubits indicates McClean-Cerezo barren plateau."
                ),
                properties={
                    "ansatz_pattern": ansatz_pattern,
                    "target_system": target_system,
                    "mean_variance": metrics["mean_variance"],
                    "threshold": metrics["effective_threshold"],
                    "qubits": self.num_qubits,
                },
            )
            return True, cert, metrics

        return False, None, metrics

