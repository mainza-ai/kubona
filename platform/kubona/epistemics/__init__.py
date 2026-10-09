"""Epistemics package: Claim Credibility Wavefunctions and epistemic curvature."""

from .continuum import (
    ComputationalHypothesis,
    EpistemicContinuum,
    EpistemicLevel,
    NegativeCertificate,
)
from .curvature import (
    CurvatureMetrics,
    EpistemicCurvatureEngine,
    FisherRaoCredibilityManifold,
    GeodesicTrajectory,
    ManifoldPoint,
)
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
    "ComputationalHypothesis",
    "CredibilityPolicy",
    "CurvatureMetrics",
    "DEFAULT_POLICY",
    "EpistemicContinuum",
    "EpistemicCurvatureEngine",
    "EpistemicGateError",
    "EpistemicLevel",
    "EvidenceSignal",
    "FisherRaoCredibilityManifold",
    "GeodesicTrajectory",
    "ManifoldPoint",
    "NegativeCertificate",
    "WavefunctionState",
]

