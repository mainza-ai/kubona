"""
Epistemic Tripartite Wavefunction and Claim Evaluation Pipeline.
Implements the credibility wavefunction alpha|corroborated> + beta|contested> + gamma|unverified>
from chickensoup (src/quantum_credibility/wavefunction.py) and strict quantum advantage realism.
"""
from __future__ import annotations

import math
from typing import Dict, Any, List
import numpy as np


def compute_wavefunction_amplitudes(
    source_platforms: List[str],
    total_engagement: int,
    contradiction_signal: float,
) -> Tuple[float, float, float]:
    """
    Computes normalized amplitudes (|alpha|^2 + |beta|^2 + |gamma|^2 = 1.0)
    for the credibility statevector.
    """
    # 1. Source diversity factor in [0, 1]
    n_sources = len(set(p.lower().strip() for p in source_platforms if p))
    diversity_score = min(1.0, n_sources / 4.0)

    # 2. Log-scaled engagement in [0, 1]
    engagement_score = min(1.0, math.log1p(max(0, total_engagement)) / math.log1p(10000))

    # Base weights
    w_corroborated = 0.5 * diversity_score + 0.3 * (1.0 - min(1.0, contradiction_signal)) + 0.2 * engagement_score
    w_contested = 0.7 * min(1.0, contradiction_signal) + 0.3 * (1.0 - diversity_score if engagement_score > 0.5 else 0.0)
    w_unverified = max(0.1, 1.0 - (w_corroborated + w_contested) * 0.5)

    # Convert to probability vector then normalize amplitudes
    weights = np.array([max(0.01, w_corroborated), max(0.01, w_contested), max(0.01, w_unverified)], dtype=float)
    probs = weights / np.sum(weights)
    amplitudes = np.sqrt(probs)
    return float(amplitudes[0]), float(amplitudes[1]), float(amplitudes[2])


def evaluate_claim_credibility(
    claim_id: str,
    statement: str,
    quantum_value: float,
    classical_anchor_value: float,
    source_platforms: List[str],
    total_engagement: int,
    contradiction_signal: float,
) -> Dict[str, Any]:
    """
    Evaluates a claim against classical anchors and collapses epistemic credibility.
    Enforces quantum advantage realism: delta must be within chemical accuracy (< 1.6 mHa).
    """
    alpha, beta, gamma = compute_wavefunction_amplitudes(
        source_platforms, total_engagement, contradiction_signal
    )

    # Chemical accuracy threshold: 1 kcal/mol = 1.5936e-3 Hartree ~ 1.6 mHa
    CHEMICAL_ACCURACY_HA = 0.0016
    delta_ha = abs(quantum_value - classical_anchor_value)
    delta_mha = delta_ha * 1000.0

    passes_chemical_accuracy = delta_ha <= CHEMICAL_ACCURACY_HA

    is_advantage_claim = "advantage" in statement.lower() or "quantum supremacy" in statement.lower()
    advantage_verified = False

    if is_advantage_claim:
        if passes_chemical_accuracy and quantum_value < classical_anchor_value:
            advantage_verified = True
            verdict = "Verified: Quantum state found lower bound beyond classical FCI with chemical precision."
        else:
            advantage_verified = False
            verdict = f"REJECTED: Advantage claim failed classical benchmark (Delta = {delta_mha:.3f} mHa exceeds 1.6 mHa limit)."
    else:
        if passes_chemical_accuracy:
            verdict = f"Validated: Calculation aligns with classical physics anchor within {delta_mha:.3f} mHa."
        else:
            verdict = f"Flagged: Discrepancy observed ({delta_mha:.3f} mHa vs classical anchor)."

    return {
        "claim_id": claim_id,
        "corroborated_amplitude": alpha,
        "contested_amplitude": beta,
        "unverified_amplitude": gamma,
        "passes_chemical_accuracy": passes_chemical_accuracy,
        "delta_mha": float(delta_mha),
        "quantum_advantage_claimed": is_advantage_claim,
        "quantum_advantage_verified": advantage_verified,
        "verdict_reason": verdict,
    }
