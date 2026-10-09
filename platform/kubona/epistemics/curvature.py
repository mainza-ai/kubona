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


# --- 2.1.D DYNAMIC CREDIBILITY MANIFOLD & GEODESIC CURIOSITY NAVIGATION ---

import numpy as np
from typing import Callable, Tuple


@dataclass
class ManifoldPoint:
    """A point on the epistemic credibility manifold."""
    theta: np.ndarray
    metric_tensor: np.ndarray
    ricci_scalar: float
    is_volatile: bool  # High |R| denotes volatile / uncertain domain

    def to_dict(self) -> dict[str, Any]:
        return {
            "theta": [round(float(x), 6) for x in self.theta],
            "ricci_scalar": round(float(self.ricci_scalar), 6),
            "is_volatile": self.is_volatile,
        }


@dataclass
class GeodesicTrajectory:
    """Agent exploration trajectory guided by geodesics of maximum curvature."""
    points: list[np.ndarray]
    velocities: list[np.ndarray]
    curvatures: list[float]

    def to_dict(self) -> dict[str, Any]:
        return {
            "trajectory_steps": len(self.points),
            "endpoints": {
                "start": [round(float(x), 6) for x in self.points[0]] if self.points else [],
                "end": [round(float(x), 6) for x in self.points[-1]] if self.points else [],
            },
            "mean_curvature": round(float(np.mean(self.curvatures)), 6) if self.curvatures else 0.0,
            "max_curvature": round(float(np.max(np.abs(self.curvatures))), 6) if self.curvatures else 0.0,
        }


class FisherRaoCredibilityManifold:
    """Riemannian credibility manifold equipped with the Fisher-Rao information metric.

    Equipped with:
    - Fisher-Rao metric tensor g_μν(θ) = E_θ [ (∂_μ log p) (∂_ν log p) ]
    - Christoffel symbols Γ^μ_αβ
    - Ricci curvature tensor R_μν and scalar R = g^μν R_μν
    - Geodesic exploration:
      d^2 θ^μ / dτ^2 + Γ^μ_αβ (dθ^α/dτ) (dθ^β/dτ) = -∇^μ D_KL( p(·|θ) || p(·|θ + Δ) )
    """

    def __init__(
        self,
        dimension: int = 2,
        eps: float = 1e-4,
        regularization: float = 1e-6,
        volatility_threshold: float = 2.0,
    ):
        self.dimension = dimension
        self.eps = eps
        self.regularization = regularization
        self.volatility_threshold = volatility_threshold

    def compute_metric_tensor(
        self,
        theta: np.ndarray,
        log_prob_fn: Optional[Callable[[np.ndarray, np.ndarray], float]] = None,
        samples: Optional[np.ndarray] = None,
    ) -> np.ndarray:
        """Computes Fisher-Rao metric tensor g_μν(θ) in R^{d x d}."""
        theta = np.asarray(theta, dtype=float)
        d = len(theta)

        if log_prob_fn is not None and samples is not None and len(samples) > 0:
            # Empirical Fisher information: E[ (∂_μ log p) (∂_ν log p) ]
            grads = []
            for x in samples:
                grad_i = np.zeros(d)
                base_lp = log_prob_fn(x, theta)
                for mu in range(d):
                    t_plus = theta.copy()
                    t_plus[mu] += self.eps
                    grad_i[mu] = (log_prob_fn(x, t_plus) - base_lp) / self.eps
                grads.append(grad_i)
            grads_arr = np.array(grads)  # (N, d)
            g = (grads_arr.T @ grads_arr) / len(samples)
            # Regularize to ensure positive definiteness
            g += self.regularization * np.eye(d)
            return g

        # Analytical canonical metric for epistemic Gaussian credibility variance:
        # g_μν(θ) = diag(1 / (θ_μ^2 + eps)) + cross terms decaying with parameter distance
        diag_elements = 1.0 / (np.square(theta) + self.eps)
        g = np.diag(diag_elements)
        # Add smooth correlation off-diagonals
        for i in range(d):
            for j in range(i + 1, d):
                corr = 0.1 / (1.0 + abs(theta[i] - theta[j]))
                g[i, j] = corr * math.sqrt(diag_elements[i] * diag_elements[j])
                g[j, i] = g[i, j]
        g += self.regularization * np.eye(d)
        return g

    def compute_christoffel_symbols(self, theta: np.ndarray) -> np.ndarray:
        """Computes Christoffel symbols of the second kind Γ^μ_αβ.

        Γ^μ_αβ = 0.5 * g^μσ (∂_α g_βσ + ∂_β g_ασ - ∂_σ g_αβ)
        Returns tensor of shape (d, d, d) where out[mu, alpha, beta] = Γ^μ_αβ.
        """
        theta = np.asarray(theta, dtype=float)
        d = len(theta)
        g = self.compute_metric_tensor(theta)
        g_inv = np.linalg.pinv(g)

        # Numerical partial derivatives ∂_gamma g_alpha_beta
        dg = np.zeros((d, d, d))  # dg[gamma, alpha, beta] = ∂_gamma g_alpha_beta
        for gamma in range(d):
            t_plus = theta.copy()
            t_minus = theta.copy()
            t_plus[gamma] += self.eps
            t_minus[gamma] -= self.eps
            g_plus = self.compute_metric_tensor(t_plus)
            g_minus = self.compute_metric_tensor(t_minus)
            dg[gamma] = (g_plus - g_minus) / (2.0 * self.eps)

        gamma_tensor = np.zeros((d, d, d))
        for mu in range(d):
            for alpha in range(d):
                for beta in range(d):
                    val = 0.0
                    for sigma in range(d):
                        term = dg[alpha, beta, sigma] + dg[beta, alpha, sigma] - dg[sigma, alpha, beta]
                        val += 0.5 * g_inv[mu, sigma] * term
                    gamma_tensor[mu, alpha, beta] = val

        return gamma_tensor

    def compute_ricci_curvature(self, theta: np.ndarray) -> Tuple[np.ndarray, float]:
        """Computes Ricci curvature tensor R_μν and Ricci curvature scalar R.

        R_μν = R^λ_μλν
        R = g^μν R_μν
        """
        theta = np.asarray(theta, dtype=float)
        d = len(theta)
        g = self.compute_metric_tensor(theta)
        g_inv = np.linalg.pinv(g)
        gamma = self.compute_christoffel_symbols(theta)

        # Numerical derivatives of Christoffel symbols: ∂_nu Γ^mu_alpha_beta
        dgamma = np.zeros((d, d, d, d))  # dgamma[nu, mu, alpha, beta] = ∂_nu Γ^mu_alpha_beta
        for nu in range(d):
            t_plus = theta.copy()
            t_minus = theta.copy()
            t_plus[nu] += self.eps
            t_minus[nu] -= self.eps
            gamma_plus = self.compute_christoffel_symbols(t_plus)
            gamma_minus = self.compute_christoffel_symbols(t_minus)
            dgamma[nu] = (gamma_plus - gamma_minus) / (2.0 * self.eps)

        # Riemann tensor: R^rho_sigma_mu_nu = ∂_mu Γ^rho_nu_sigma - ∂_nu Γ^rho_mu_sigma
        #                                  + Γ^rho_mu_lambda Γ^lambda_nu_sigma - Γ^rho_nu_lambda Γ^lambda_mu_sigma
        # Contract over rho and mu to get Ricci tensor: R_sigma_nu = R^mu_sigma_mu_nu
        ricci_tensor = np.zeros((d, d))
        for sigma in range(d):
            for nu in range(d):
                r_val = 0.0
                for mu in range(d):
                    # Derivative terms
                    r_val += dgamma[mu, mu, nu, sigma] - dgamma[nu, mu, mu, sigma]
                    # Product terms
                    for lam in range(d):
                        r_val += gamma[mu, mu, lam] * gamma[lam, nu, sigma] - gamma[mu, nu, lam] * gamma[lam, mu, sigma]
                ricci_tensor[sigma, nu] = r_val

        # Ricci scalar R = g^μν R_μν
        ricci_scalar = float(np.sum(g_inv * ricci_tensor))
        return ricci_tensor, ricci_scalar

    def compute_kl_gradient(self, theta: np.ndarray, delta: np.ndarray) -> np.ndarray:
        """Computes ∇^μ D_KL( p(·|θ) || p(·|θ + Δ) ) = g^μν ∂_ν D_KL.

        Under second-order expansion, D_KL( p(·|θ) || p(·|θ + Δ) ) ≈ 0.5 Δ^T g(θ) Δ.
        Hence ∂_ν D_KL ≈ 0.5 * ∂_ν (Δ^T g(θ) Δ).
        Contraction with g^μν yields the contravariant gradient driving curiosity.
        """
        theta = np.asarray(theta, dtype=float)
        delta = np.asarray(delta, dtype=float)
        d = len(theta)
        g = self.compute_metric_tensor(theta)
        g_inv = np.linalg.pinv(g)

        grad_kl_down = np.zeros(d)
        base_quad = float(delta.T @ g @ delta)
        for nu in range(d):
            t_plus = theta.copy()
            t_plus[nu] += self.eps
            g_plus = self.compute_metric_tensor(t_plus)
            quad_plus = float(delta.T @ g_plus @ delta)
            grad_kl_down[nu] = 0.5 * (quad_plus - base_quad) / self.eps

        # Contravariant vector ∇^μ D_KL = g^μν ∂_ν D_KL
        return g_inv @ grad_kl_down

    def geodesic_step(
        self,
        theta: np.ndarray,
        velocity: np.ndarray,
        dtau: float = 0.05,
        delta_perturbation: Optional[np.ndarray] = None,
    ) -> Tuple[np.ndarray, np.ndarray, float]:
        """Performs a single step of geodesic curiosity exploration:

        d^2 θ^μ / dτ^2 = -Γ^μ_αβ v^α v^β - ∇^μ D_KL
        """
        theta = np.asarray(theta, dtype=float)
        velocity = np.asarray(velocity, dtype=float)
        d = len(theta)

        gamma = self.compute_christoffel_symbols(theta)
        _, ricci_scalar = self.compute_ricci_curvature(theta)

        # Geodesic acceleration: a_geo^mu = - Γ^mu_alpha_beta * v^alpha * v^beta
        accel = np.zeros(d)
        for mu in range(d):
            accel[mu] = -float(velocity.T @ gamma[mu] @ velocity)

        # Curiosity force: - ∇^μ D_KL
        if delta_perturbation is not None:
            kl_grad = self.compute_kl_gradient(theta, delta_perturbation)
            accel -= kl_grad

        # Verlet integration step
        theta_next = theta + velocity * dtau + 0.5 * accel * (dtau ** 2)
        vel_next = velocity + accel * dtau

        return theta_next, vel_next, ricci_scalar

    def integrate_geodesic(
        self,
        theta_0: np.ndarray,
        v_0: np.ndarray,
        steps: int = 10,
        dtau: float = 0.05,
        delta_perturbation: Optional[np.ndarray] = None,
    ) -> GeodesicTrajectory:
        """Traces agent exploration trajectory along the credibility manifold."""
        cur_theta = np.asarray(theta_0, dtype=float).copy()
        cur_v = np.asarray(v_0, dtype=float).copy()

        points = [cur_theta.copy()]
        velocities = [cur_v.copy()]
        curvatures = []

        for _ in range(steps):
            cur_theta, cur_v, r_scalar = self.geodesic_step(
                cur_theta, cur_v, dtau=dtau, delta_perturbation=delta_perturbation
            )
            points.append(cur_theta.copy())
            velocities.append(cur_v.copy())
            curvatures.append(r_scalar)

        return GeodesicTrajectory(points=points, velocities=velocities, curvatures=curvatures)

