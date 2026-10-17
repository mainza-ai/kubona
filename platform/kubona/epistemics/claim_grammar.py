"""Formal Claim Grammar & Scientific Assertion Schema (Ring 1).

Enforces the strict vocabulary and positive/negative claim boundaries mandated
by the Kubona Blueprint (Q-04, Q-18, and quantum-advantage-realism.md).

No claim may assert quantum advantage without verified classical anchor deltas.
Every claim is schema-validated, falsifiable, and carries hardware provenance.
"""

from __future__ import annotations

import enum
import re
import uuid
from typing import Any, List, Optional
from pydantic import BaseModel, Field, field_validator, model_validator


# Chemical accuracy threshold: 1 kcal/mol ≈ 1.6 mHa. Single shared const —
# never an inline literal.
CHEMICAL_ACCURACY_HA = 1.6e-3


class ClaimCategory(str, enum.Enum):
    ANALYTIC_PROOF = "ANALYTIC_PROOF"
    EMPIRICAL_SIMULATION = "EMPIRICAL_SIMULATION"
    QPU_MEASURED = "QPU_MEASURED"
    HYPOTHESIS_REFUTED = "HYPOTHESIS_REFUTED"


class EvidenceTier(str, enum.Enum):
    """How the claim's number was earned — required on every claim.

    T1_VERIFIED: mechanical, parameter-free check (unitary equality,
      anchor match to tolerance, lint/type-level, formal proof step).
    T2_CONSISTENT: agrees under stated assumptions (model vs literature
      with assumed prefactors, simulator vs theory, fitted calibration).
    T3_SAMPLED: statistical interval from finite shots, runs, or seeds
      (canary Wilson intervals, seed bands, Monte Carlo estimates).
    A T3 number cited without its interval, or a T2 cited without its
    assumptions, refuses downstream — the tier names what must travel
    with the number.
    """

    T1_VERIFIED = "T1_VERIFIED"
    T2_CONSISTENT = "T2_CONSISTENT"
    T3_SAMPLED = "T3_SAMPLED"


class CredibilityTier(str, enum.Enum):
    """Epistemic tier of the credibility field (Q-04)."""

    L4 = "L4"  # proof
    L3 = "L3"  # anchor
    L2 = "L2"  # classical baseline
    L1 = "L1"  # QPU run
    L0 = "L0"  # generative


class BackendTier(str, enum.Enum):
    """Hardware backend tier."""

    T0_ANCHOR = "T0_ANCHOR"
    T1_LOCAL_EMU = "T1_LOCAL_EMU"
    T2_CLOUD_SIM = "T2_CLOUD_SIM"
    T3_CLOUD_QPU = "T3_CLOUD_QPU"


# Q-04: permitted claim verbs. A predicate must contain one of these —
# reports may only *report what was done*, never what was achieved in the
# abstract. Word-boundary regex (no substring gate: "recomputed" must not
# pass via "computed"... actually \bcomputed\b would NOT match "recomputed"
# — that is the point: the verb must stand alone).
PERMITTED_PREDICATE_VERBS = (
    "measured",
    "reproduced",
    "bounded",
    "refuted",
    "converged",
    "estimated",
    "computed",
    "observed",
    "verified",
    "rejected",
    "matched",
    "reached",
)
_PERMITTED_VERB_RE = re.compile(
    r"\b(" + "|".join(PERMITTED_PREDICATE_VERBS) + r")\b", re.IGNORECASE
)

# Hype-vocabulary screen (heuristic_v1, documented as non-exhaustive):
# these terms assert achievement rather than reporting measurement. Matched
# with word boundaries (case-insensitive); multi-word terms match literally.
# The screen is a tripwire, not a proof of sobriety — novel hype not listed
# here is a review gap, never a pass.
FORBIDDEN_HYPE_TERMS = (
    "quantum supremacy",
    "unmatched speedup",
    "exponential speedup",
    "miraculous",
    "miraculous speedup",
    "quantum leap",
    "proves",
    "guarantees",
    "solves",
    "unlocks",
    "breakthrough",
    "revolutionary",
    "unprecedented",
    "ultrafast",
)
HYPE_SCREEN_METHOD = "hype_screen_heuristic_v1"


def _hype_pattern(term: str) -> re.Pattern:
    return re.compile(r"\b" + re.escape(term) + r"\b", re.IGNORECASE)


class CredibilityField(BaseModel):
    """What a credibility field holds (Q-04/Q-05): the collapse probability
    rho, the epistemic tier, and the ledger references backing it. Consumers
    gate assertion on rho + tier; the score is never cited without the refs."""

    rho: float = Field(ge=0.0, le=1.0)
    tier: CredibilityTier  # L4 (proof) | L3 (anchor) | L2 (classical baseline) | L1 (QPU run) | L0 (generative)
    evidence_refs: List[str] = Field(min_length=1)


class HardwareProvenance(BaseModel):
    backend_name: str
    backend_tier: BackendTier  # T0_ANCHOR | T1_LOCAL_EMU | T2_CLOUD_SIM | T3_CLOUD_QPU
    shots: int = Field(ge=0)
    noise_mitigation: Optional[str] = None
    calibrated_at: Optional[str] = None
    seed: Optional[int] = None


class AdvantageJustification(BaseModel):
    """Structured justification for an advantage claim (replaces the old
    len<15 character-count proxy, which measured verbosity, not evidence).

    method: the variational/experimental method used (e.g. "VQE RY-RY-CX depth2").
    anchor_ref: ledger/citation reference to the classical anchor compared against.
    """

    method: str = Field(min_length=3)
    anchor_ref: str = Field(min_length=1)


class ScientificClaim(BaseModel):
    claim_id: uuid.UUID = Field(default_factory=uuid.uuid4)
    category: ClaimCategory
    predicate: str = Field(min_length=10)
    hypothesis_id: Optional[str] = None

    # Ground truth comparison against Ring 2 classical anchor.
    # delta_hartree is DERIVED (quantum - classical), never caller-supplied:
    # a supplied value that disagrees with the anchor values refuses.
    classical_anchor_name: str
    classical_anchor_value: float
    quantum_measured_value: float
    delta_hartree: float
    chemical_accuracy_achieved: bool = False

    # Uncertainty quantification
    uncertainty_sigma: float = Field(ge=0.0)
    confidence_interval_95: tuple[float, float]
    falsification_criteria: List[str] = Field(min_length=1)

    # Evidence tier: how the number was earned (required, no default —
    # a claim that will not say how it knows refuses at construction).
    evidence_tier: EvidenceTier

    # Provenance & Epistemics
    hardware_provenance: HardwareProvenance
    ledger_ref: Optional[str] = None
    claims_advantage: bool = False
    advantage_justification: Optional[AdvantageJustification] = None
    credibility: Optional[CredibilityField] = None

    @field_validator("predicate")
    @classmethod
    def validate_positive_claim_vocabulary(cls, v: str) -> str:
        # Banned hype vocabulary per quantum-advantage-realism.md
        lowered = v.lower()
        for term in FORBIDDEN_HYPE_TERMS:
            if _hype_pattern(term).search(lowered):
                raise ValueError(f"Forbidden ungrounded term '{term}' in claim predicate.")
        # Q-04: permitted verbs only — the predicate must state what was done.
        if not _PERMITTED_VERB_RE.search(v):
            raise ValueError(
                "Claim predicate must contain a permitted reporting verb "
                f"{sorted(PERMITTED_PREDICATE_VERBS)}; got no permitted verb. "
                "Reports state measurements, never achievements."
            )
        return v

    @field_validator("confidence_interval_95")
    @classmethod
    def validate_confidence_interval(
        cls, v: tuple[float, float]
    ) -> tuple[float, float]:
        import math

        lo, hi = v
        for bound, name in ((lo, "lower"), (hi, "upper")):
            if not isinstance(bound, (int, float)) or not math.isfinite(float(bound)):
                raise ValueError(
                    f"confidence_interval_95 {name} bound must be finite, got {bound!r}"
                )
        if not lo <= hi:
            raise ValueError(
                f"confidence_interval_95 must be ordered (lo <= hi); got ({lo}, {hi})"
            )
        return v

    @model_validator(mode="after")
    def validate_derived_fields(self) -> ScientificClaim:
        import math

        # delta_hartree is computed from anchor values, never accepted blindly.
        for name, val in (
            ("classical_anchor_value", self.classical_anchor_value),
            ("quantum_measured_value", self.quantum_measured_value),
        ):
            if not isinstance(val, (int, float)) or not math.isfinite(float(val)):
                raise ValueError(f"{name} must be finite, got {val!r}")
        expected_delta = float(self.quantum_measured_value) - float(
            self.classical_anchor_value
        )
        if abs(float(self.delta_hartree) - expected_delta) > 1e-12:
            raise ValueError(
                "delta_hartree is derived (quantum_measured_value - classical_anchor_value): "
                f"supplied {self.delta_hartree!r} disagrees with computed {expected_delta!r} — refusing."
            )
        # chemical_accuracy_achieved must agree with the derived delta.
        expected_acc = abs(expected_delta) <= CHEMICAL_ACCURACY_HA
        if bool(self.chemical_accuracy_achieved) != expected_acc:
            raise ValueError(
                f"chemical_accuracy_achieved={self.chemical_accuracy_achieved!r} contradicts "
                f"derived delta {expected_delta:.6f} Ha (threshold {CHEMICAL_ACCURACY_HA} Ha) — refusing."
            )
        # Consistency: the measured value must lie inside its own 95% interval.
        lo, hi = self.confidence_interval_95
        if not (lo <= float(self.quantum_measured_value) <= hi):
            raise ValueError(
                "confidence_interval_95 must contain quantum_measured_value "
                f"({self.quantum_measured_value!r} not in [{lo}, {hi}]) — refusing."
            )
        # Cross-field: measured categories require shots > 0 (a measurement
        # with zero shots measured nothing).
        if self.category in (ClaimCategory.QPU_MEASURED, ClaimCategory.EMPIRICAL_SIMULATION):
            if self.hardware_provenance.shots <= 0:
                raise ValueError(
                    f"category {self.category.value} requires hardware_provenance.shots > 0; "
                    f"got {self.hardware_provenance.shots}"
                )
        return self

    @model_validator(mode="after")
    def validate_advantage_realism(self) -> ScientificClaim:
        # Quantum Advantage Realism check
        # Advantage can ONLY be claimed if chemical accuracy is achieved AND delta < 1.6e-3 Ha
        # AND category is QPU_MEASURED or EMPIRICAL_SIMULATION with structured justification.
        if self.claims_advantage:
            if not self.chemical_accuracy_achieved:
                raise ValueError(
                    "Violation of Quantum Advantage Realism: cannot claim advantage when chemical accuracy is not achieved."
                )
            if abs(self.delta_hartree) > CHEMICAL_ACCURACY_HA:
                raise ValueError(
                    f"Violation of Quantum Advantage Realism: delta ({self.delta_hartree:.6f} Ha) exceeds 1.6 mHa threshold."
                )
            if self.advantage_justification is None:
                raise ValueError(
                    "Claims of advantage require a structured justification "
                    "(AdvantageJustification with method + anchor_ref)."
                )
            # Q-04: advantage claims must carry a credibility field — the score
            # is never cited without its refs — at rho >= 0.90, tier L4/L3.
            if self.credibility is None:
                raise ValueError(
                    "Claims of advantage require a credibility field (rho, tier, evidence_refs)."
                )
            if self.credibility.rho < 0.90 or self.credibility.tier not in (
                CredibilityTier.L4,
                CredibilityTier.L3,
            ):
                raise ValueError(
                    f"Advantage requires credibility rho >= 0.90 at tier L4/L3; got "
                    f"rho={self.credibility.rho}, tier={self.credibility.tier}."
                )
        return self

    def formatted_statement(self) -> str:
        tag = f"[{self.category.value}]"
        adv_tag = "[ADVANTAGE GROUNDED]" if self.claims_advantage else "[NO ADVANTAGE CLAIMED]"
        base = f"{tag} {adv_tag} {self.predicate} (Δ={self.delta_hartree:.4f} Ha, σ={self.uncertainty_sigma:.4f})"
        base += f" [{self.evidence_tier.value}]"
        if self.credibility is not None:
            c = self.credibility
            base += f" [ρ={c.rho:.3f} {c.tier.value} refs={len(c.evidence_refs)}]"
        return base
