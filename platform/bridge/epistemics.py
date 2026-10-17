"""
Epistemic Tripartite Wavefunction and Claim Evaluation Pipeline.
Implements the credibility wavefunction alpha|corroborated> + beta|contested> + gamma|unverified>
from chickensoup (src/quantum_credibility/wavefunction.py) and strict quantum advantage realism.

Production bar (v1 heuristic, labeled honestly):
- All numbers below are structural-prior weights, NOT measured physics.
- Outputs carry ``credibility_method="credibility_heuristic_v1"`` so no
  consumer mistakes them for a measured credibility.
"""
from __future__ import annotations

import math
import re
from typing import Dict, Any, List, Tuple
import numpy as np


# --- Module consts (v1 heuristic weights; shared, never inline magic) ---
CREDIBILITY_METHOD = "credibility_heuristic_v1"
CHEMICAL_ACCURACY_HA = 0.0016  # 1.6 mHa ~ 1 kcal/mol
DIVERSITY_CAP_SOURCES = 4.0
ENGAGEMENT_NORM = math.log1p(10000)
W_CORR_DIVERSITY = 0.5
W_CORR_ANCHOR = 0.3
W_CORR_ENGAGEMENT = 0.2
W_CONT_CONTRADICTION = 0.7
W_CONT_NARROW = 0.3
W_UNVERIFIED_FLOOR = 0.1
AMPLITUDE_FLOOR = 0.01

# Explicit claim-type allowlist (regex, fullmatch). Callers must name the
# claim kind; the statement text never silently decides it.
_CLAIM_TYPE_RE = re.compile(r"^(measured|benchmark|observation|advantage)$")
# Advantage language in free text: word boundaries only (no substring gate —
# "advantageous" must not trip the detector).
_ADVANTAGE_RE = re.compile(
    r"\b(quantum\s+advantage|quantum\s+supremacy|advantage)\b", re.IGNORECASE
)

# Only this anchor method can ground an advantage verdict. Anything else
# (CASCI-labeled-exact, RHF, DFT, unknown) refuses.
FCI_EXACT_ANCHOR_METHOD = "FCI-exact"


def _require_unit(x: float, name: str) -> float:
    if not isinstance(x, (int, float)) or not math.isfinite(float(x)):
        raise ValueError(f"{name} must be a finite number, got {x!r}")
    v = float(x)
    if not 0.0 <= v <= 1.0:
        raise ValueError(f"{name} must be in [0,1], got {x!r}")
    return v


def _require_nonneg_int(x: int, name: str) -> int:
    if isinstance(x, bool):
        raise ValueError(f"{name} must be a non-negative int, got {x!r}")
    if isinstance(x, float):
        if not x.is_integer():
            raise ValueError(f"{name} must be a non-negative int, got {x!r}")
        x = int(x)
    if not isinstance(x, int) or not math.isfinite(float(x)):
        raise ValueError(f"{name} must be a finite number, got {x!r}")
    if x < 0:
        raise ValueError(f"{name} must be >= 0, got {x!r}")
    return x


def compute_wavefunction_amplitudes(
    source_platforms: List[str],
    total_engagement: int,
    contradiction_signal: float,
) -> Tuple[float, float, float]:
    """
    Computes normalized amplitudes (|alpha|^2 + |beta|^2 + |gamma|^2 = 1.0)
    for the credibility statevector.

    v1 heuristic (credibility_heuristic_v1): structural prior only.
    """
    contradiction_signal = _require_unit(contradiction_signal, "contradiction_signal")
    total_engagement = _require_nonneg_int(total_engagement, "total_engagement")

    # 1. Source diversity factor in [0, 1]
    n_sources = len(set(p.lower().strip() for p in source_platforms if p))
    diversity_score = min(1.0, n_sources / DIVERSITY_CAP_SOURCES)

    # 2. Log-scaled engagement in [0, 1]
    engagement_score = min(1.0, math.log1p(total_engagement) / ENGAGEMENT_NORM)

    # Base weights
    w_corroborated = (
        W_CORR_DIVERSITY * diversity_score
        + W_CORR_ANCHOR * (1.0 - contradiction_signal)
        + W_CORR_ENGAGEMENT * engagement_score
    )
    # Engagement term (parenthesized deliberately): the narrow-source penalty
    # (1 - diversity) applies ONLY when engagement is high (> 0.5) — i.e. a
    # widely-discussed claim drawn from few distinct sources is contested.
    # At low engagement there is no crowd to be narrow about, so the term is 0.
    w_contested = W_CONT_CONTRADICTION * contradiction_signal + W_CONT_NARROW * (
        (1.0 - diversity_score) if engagement_score > 0.5 else 0.0
    )
    w_unverified = max(W_UNVERIFIED_FLOOR, 1.0 - (w_corroborated + w_contested) * 0.5)

    # Convert to probability vector then normalize amplitudes
    weights = np.array(
        [max(AMPLITUDE_FLOOR, w_corroborated), max(AMPLITUDE_FLOOR, w_contested),
         max(AMPLITUDE_FLOOR, w_unverified)],
        dtype=float,
    )
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
    *,
    anchor_method: str = "unknown",
    claim_type: str = "measured",
) -> Dict[str, Any]:
    """
    Evaluates a claim against classical anchors and collapses epistemic credibility.
    Enforces quantum advantage realism: delta must be within chemical accuracy (< 1.6 mHa)
    AND the anchor method must be 'FCI-exact' — any other anchor refuses advantage.

    claim_type must match ^(measured|benchmark|observation|advantage)$ (explicit,
    regex-validated). Advantage is claimed when claim_type == "advantage" OR the
    statement matches the word-boundary advantage regex (no substring gate).

    The verdict fuses all three amplitudes: the corroborated amplitude must
    dominate for any passing verdict; a contested/unverified-dominant prior
    qualifies or rejects even when the energy delta passes. The amplitudes are
    never decorative.
    """
    for name, val in (("quantum_value", quantum_value),
                      ("classical_anchor_value", classical_anchor_value)):
        if not isinstance(val, (int, float)) or not math.isfinite(float(val)):
            raise ValueError(f"{name} must be a finite number, got {val!r} (refusing NaN verdict)")
    if not _CLAIM_TYPE_RE.fullmatch(claim_type):
        raise ValueError(
            f"claim_type must match { _CLAIM_TYPE_RE.pattern }, got {claim_type!r}"
        )

    alpha, beta, gamma = compute_wavefunction_amplitudes(
        source_platforms, total_engagement, contradiction_signal
    )

    delta_ha = abs(float(quantum_value) - float(classical_anchor_value))
    delta_mha = delta_ha * 1000.0

    passes_chemical_accuracy = delta_ha <= CHEMICAL_ACCURACY_HA

    is_advantage_claim = (claim_type == "advantage") or bool(_ADVANTAGE_RE.search(statement))
    advantage_verified = False

    # Amplitude fusion: which basis dominates the prior?
    corroborated_dominant = alpha >= beta and alpha >= gamma
    contested_dominant = beta >= alpha and beta >= gamma

    if is_advantage_claim:
        if anchor_method != FCI_EXACT_ANCHOR_METHOD:
            advantage_verified = False
            verdict = (
                f"REJECTED: Advantage claim refused — anchor method {anchor_method!r} "
                f"is not {FCI_EXACT_ANCHOR_METHOD!r}. Advantage requires an FCI-exact anchor."
            )
        elif not corroborated_dominant:
            advantage_verified = False
            verdict = (
                f"REJECTED: Advantage claim refused — credibility prior is not corroborated "
                f"(α={alpha:.3f}, β={beta:.3f}, γ={gamma:.3f})."
            )
        elif passes_chemical_accuracy and float(quantum_value) < float(classical_anchor_value):
            advantage_verified = True
            verdict = "Verified: Quantum state found lower bound beyond classical FCI with chemical precision."
        else:
            advantage_verified = False
            verdict = f"REJECTED: Advantage claim failed classical benchmark (Delta = {delta_mha:.3f} mHa exceeds 1.6 mHa limit)."
    else:
        if passes_chemical_accuracy and corroborated_dominant:
            verdict = f"Validated: Calculation aligns with classical physics anchor within {delta_mha:.3f} mHa."
        elif passes_chemical_accuracy and contested_dominant:
            verdict = (
                f"Qualified: Delta within {delta_mha:.3f} mHa but credibility prior is contested "
                f"(α={alpha:.3f}, β={beta:.3f}, γ={gamma:.3f}) — requires further corroboration."
            )
        elif passes_chemical_accuracy:
            verdict = (
                f"Qualified: Delta within {delta_mha:.3f} mHa but credibility prior is unverified "
                f"(α={alpha:.3f}, β={beta:.3f}, γ={gamma:.3f}) — requires further corroboration."
            )
        else:
            verdict = f"Flagged: Discrepancy observed ({delta_mha:.3f} mHa vs classical anchor)."

    return {
        "claim_id": claim_id,
        "credibility_method": CREDIBILITY_METHOD,
        "corroborated_amplitude": alpha,
        "contested_amplitude": beta,
        "unverified_amplitude": gamma,
        "passes_chemical_accuracy": passes_chemical_accuracy,
        "delta_mha": float(delta_mha),
        "quantum_advantage_claimed": is_advantage_claim,
        "quantum_advantage_verified": advantage_verified,
        "anchor_method": anchor_method,
        "claim_type": claim_type,
        "verdict_reason": verdict,
    }
