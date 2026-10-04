"""Epistemics package: Claim Credibility Wavefunctions and epistemic curvature."""

from .curvature import CurvatureMetrics, EpistemicCurvatureEngine
from .wavefunction import (
    BASIS_STATES,
    ClaimCredibilityWavefunction,
    CredibilityPolicy,
    DEFAULT_POLICY,
    EpistemicGateError,
    EvidenceSignal,
    WavefunctionState,
)

__all__ = [
    "BASIS_STATES",
    "ClaimCredibilityWavefunction",
    "CredibilityPolicy",
    "CurvatureMetrics",
    "DEFAULT_POLICY",
    "EpistemicCurvatureEngine",
    "EpistemicGateError",
    "EvidenceSignal",
    "WavefunctionState",
]
