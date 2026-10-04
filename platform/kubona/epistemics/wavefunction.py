"""Policy-driven Claim Credibility Wavefunction implementation (Ring 1).

Every literature- or experiment-derived statement Kubona records carries a
three-basis state amplitude vector over:
    |corroborated⟩, |contested⟩, |unverified⟩

A claim may be formally asserted in an artifact/report ONLY when its state has
collapsed under the evidence-strength gate:
    1. Peaked state: max(P(state)) >= collapse_threshold (default 0.75)
    2. Evidence strength: diversity >= min_diversity (default 0.3) OR n_evidence >= 3
       OR engagement >= min_engagement (default 0.4) OR market_prior != None

Un-collapsed claims cannot be asserted as fact; they must be qualified as
uncertain probes or hypotheses.
"""

from __future__ import annotations

import hashlib
import json
import math
import uuid
from dataclasses import asdict, dataclass, field
from typing import Any, Sequence

# Canonical basis labels
BASIS_STATES = ("corroborated", "contested", "unverified")


@dataclass(frozen=True)
class CredibilityPolicy:
    """Versioned epistemic thresholds. Policy changes invalidate past collapse verdicts."""
    policy_id: str = "credibility-v1"
    collapse_threshold: float = 0.75
    min_diversity: float = 0.3
    min_evidence_count: int = 3
    min_engagement: float = 0.4
    amplitude_floor: float = 0.01

    def digest(self) -> str:
        s = f"{self.policy_id}:{self.collapse_threshold}:{self.min_diversity}:{self.min_evidence_count}:{self.min_engagement}:{self.amplitude_floor}"
        return hashlib.sha256(s.encode("utf-8")).hexdigest()[:16]


DEFAULT_POLICY = CredibilityPolicy()


@dataclass
class EvidenceSignal:
    """Individual item of supporting or contradicting evidence."""
    source_id: str
    source_type: str  # e.g., "arxiv", "crossref", "experiment", "qpu_canary", "reproduction"
    supports: bool = True
    confidence: float = 1.0  # [0.0, 1.0]
    engagement: float = 0.0  # Citations or verification runs (log1p scaled)
    ledger_ref: str | None = None


@dataclass
class WavefunctionState:
    """Three-basis probability distribution and collapse state."""
    amplitudes: dict[str, float]  # basis -> raw amplitude
    probabilities: dict[str, float]  # basis -> P(basis)
    collapsed: bool
    verdict: str  # "corroborated" | "contested" | "unverified" | "uncollapsed"
    evidence_count: int
    diversity_score: float
    entropy: float  # Shannon entropy over basis states
    policy_digest: str
    quantum_provenance: dict[str, Any]

    def to_dict(self) -> dict[str, Any]:
        return asdict(self)


class EpistemicGateError(ValueError):
    """Raised when an uncollapsed claim is asserted as a verified fact."""
    pass


class ClaimCredibilityWavefunction:
    """Computes quantum/classical state amplitudes and evaluates the collapse gate."""

    def __init__(self, policy: CredibilityPolicy | None = None):
        self.policy = policy or DEFAULT_POLICY

    def evaluate(
        self,
        evidence: Sequence[EvidenceSignal],
        market_prior: float | None = None,
        use_quantum_simulator: bool = False,
    ) -> WavefunctionState:
        """Evaluates evidence signals, calculates amplitudes, and checks collapse criteria."""
        n_ev = len(evidence)
        if n_ev == 0:
            # Baseline unverified prior
            probs = {"corroborated": 0.05, "contested": 0.05, "unverified": 0.90}
            return WavefunctionState(
                amplitudes={"corroborated": math.sqrt(0.05), "contested": math.sqrt(0.05), "unverified": math.sqrt(0.90)},
                probabilities=probs,
                collapsed=False,
                verdict="uncollapsed",
                evidence_count=0,
                diversity_score=0.0,
                entropy=self._shannon_entropy(probs),
                policy_digest=self.policy.digest(),
                quantum_provenance={"backend": "classical-prior", "shots": 0, "fallback_reason": "no_evidence"},
            )

        # 1. Source diversity score: unique source types / 5.0 (capped at 1.0)
        source_types = {e.source_type for e in evidence}
        diversity = min(len(source_types) / 5.0, 1.0)

        # 2. Support vs Contradiction weights
        support_weight = sum(e.confidence for e in evidence if e.supports)
        contest_weight = sum(e.confidence for e in evidence if not e.supports)
        total_weight = support_weight + contest_weight

        # 3. Maximum engagement
        max_engagement = max((e.engagement for e in evidence), default=0.0)

        # 4. Amplitude computation
        # Raw unnormalized amplitudes
        raw_corr = self.policy.amplitude_floor + (support_weight if total_weight > 0 else 0.0)
        raw_cont = self.policy.amplitude_floor + (contest_weight if total_weight > 0 else 0.0)
        # Unverified amplitude decreases with total evidence and diversity
        raw_unver = max(self.policy.amplitude_floor, 2.0 / (1.0 + total_weight + diversity * 2.0))

        if market_prior is not None:
            raw_corr += max(0.0, market_prior)
            raw_cont += max(0.0, 1.0 - market_prior)

        # Normalization
        norm = math.sqrt(raw_corr**2 + raw_cont**2 + raw_unver**2)
        amp_corr = raw_corr / norm
        amp_cont = raw_cont / norm
        amp_unver = raw_unver / norm

        amplitudes = {
            "corroborated": round(amp_corr, 6),
            "contested": round(amp_cont, 6),
            "unverified": round(amp_unver, 6),
        }

        # Quantum state preparation or Born rule evaluation
        q_prov: dict[str, Any]
        if use_quantum_simulator:
            # When quantum simulation is explicitly requested, evaluate via Statevector or Aer
            try:
                from qiskit.quantum_info import Statevector
                # 3 basis states require 2 qubits (4 state amplitudes, padded with 0.0)
                state_vec = [amp_corr, amp_cont, amp_unver, 0.0]
                sv = Statevector(state_vec)
                probs_vec = sv.probabilities()
                probabilities = {
                    "corroborated": round(float(probs_vec[0]), 6),
                    "contested": round(float(probs_vec[1]), 6),
                    "unverified": round(float(probs_vec[2]), 6),
                }
                q_prov = {
                    "backend": "qiskit-statevector",
                    "shots": None,
                    "qubits": 2,
                    "state_hash": hashlib.sha256(str(state_vec).encode()).hexdigest()[:16],
                    "fallback_reason": None,
                }
            except Exception as exc:
                probabilities = {
                    "corroborated": round(amp_corr**2, 6),
                    "contested": round(amp_cont**2, 6),
                    "unverified": round(amp_unver**2, 6),
                }
                q_prov = {
                    "backend": f"classical-fallback: {exc}",
                    "shots": 0,
                    "fallback_reason": str(exc),
                }
        else:
            # Exact Born rule probabilities
            probabilities = {
                "corroborated": round(amp_corr**2, 6),
                "contested": round(amp_cont**2, 6),
                "unverified": round(amp_unver**2, 6),
            }
            q_prov = {
                "backend": "born-rule-analytical",
                "shots": 0,
                "fallback_reason": None,
            }

        entropy = self._shannon_entropy(probabilities)

        # 5. Evidence strength gate
        # Peak probability
        max_state = max(probabilities, key=probabilities.get)  # type: ignore[arg-type]
        max_prob = probabilities[max_state]

        peaked = max_prob >= self.policy.collapse_threshold
        strength_ok = (
            diversity >= self.policy.min_diversity
            or n_ev >= self.policy.min_evidence_count
            or max_engagement >= self.policy.min_engagement
            or market_prior is not None
        )

        collapsed = bool(peaked and strength_ok)
        verdict = max_state if collapsed else "uncollapsed"

        return WavefunctionState(
            amplitudes=amplitudes,
            probabilities=probabilities,
            collapsed=collapsed,
            verdict=verdict,
            evidence_count=n_ev,
            diversity_score=round(diversity, 4),
            entropy=round(entropy, 4),
            policy_digest=self.policy.digest(),
            quantum_provenance=q_prov,
        )

    def assert_claim(self, claim_text: str, state: WavefunctionState) -> str:
        """Fails closed if uncollapsed. Returns formatted assertion string."""
        if not state.collapsed:
            raise EpistemicGateError(
                f"Claim '{claim_text}' cannot be asserted as fact: state is uncollapsed "
                f"(max P={max(state.probabilities.values()):.2f}, "
                f"diversity={state.diversity_score:.2f}, "
                f"evidence_count={state.evidence_count}). "
                f"Must qualify as hypothesis or uncertain probe."
            )
        return f"[{state.verdict.upper()}] {claim_text}"

    @staticmethod
    def _shannon_entropy(probs: dict[str, float]) -> float:
        h = 0.0
        for p in probs.values():
            if p > 1e-12:
                h -= p * math.log2(p)
        return h
