"""Formal Claim Grammar & Scientific Assertion Schema (Ring 1).

Enforces the strict vocabulary and positive/negative claim boundaries mandated
by the Kubona Blueprint (Q-04, Q-18, and quantum-advantage-realism.md).

No claim may assert quantum advantage without verified classical anchor deltas.
Every claim is schema-validated, falsifiable, and carries hardware provenance.
"""

from __future__ import annotations

import enum
import uuid
from typing import Any, List, Optional
from pydantic import BaseModel, Field, field_validator, model_validator


class ClaimCategory(str, enum.Enum):
    ANALYTIC_PROOF = "ANALYTIC_PROOF"
    EMPIRICAL_SIMULATION = "EMPIRICAL_SIMULATION"
    QPU_MEASURED = "QPU_MEASURED"
    HYPOTHESIS_REFUTED = "HYPOTHESIS_REFUTED"


# Q-04: permitted claim verbs. A predicate must contain one of these —
# reports may only *report what was done*, never what was achieved in the
# abstract. Anything else (proves, guarantees, solves, unlocks, ...) fails
# validation with guidance, at construction time (fail-closed).
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


class CredibilityField(BaseModel):
    """What a credibility field holds (Q-04/Q-05): the collapse probability
    rho, the epistemic tier, and the ledger references backing it. Consumers
    gate assertion on rho + tier; the score is never cited without the refs."""

    rho: float = Field(ge=0.0, le=1.0)
    tier: str  # L4 (proof) | L3 (anchor) | L2 (classical baseline) | L1 (QPU run) | L0 (generative)
    evidence_refs: List[str] = Field(min_length=1)


class HardwareProvenance(BaseModel):
    backend_name: str
    backend_tier: str  # "T0_ANCHOR", "T1_LOCAL_EMU", "T2_CLOUD_SIM", "T3_CLOUD_QPU"
    shots: int = Field(ge=0)
    noise_mitigation: Optional[str] = None
    calibrated_at: Optional[str] = None
    seed: Optional[int] = None


class ScientificClaim(BaseModel):
    claim_id: uuid.UUID = Field(default_factory=uuid.uuid4)
    category: ClaimCategory
    predicate: str = Field(min_length=10)
    hypothesis_id: Optional[str] = None
    
    # Ground truth comparison against Ring 2 classical anchor
    classical_anchor_name: str
    classical_anchor_value: float
    quantum_measured_value: float
    delta_hartree: float
    chemical_accuracy_achieved: bool = False
    
    # Uncertainty quantification
    uncertainty_sigma: float = Field(ge=0.0)
    confidence_interval_95: tuple[float, float]
    falsification_criteria: List[str] = Field(min_length=1)
    
    # Provenance & Epistemics
    hardware_provenance: HardwareProvenance
    ledger_ref: Optional[str] = None
    claims_advantage: bool = False
    advantage_justification: Optional[str] = None
    credibility: Optional[CredibilityField] = None

    @field_validator("predicate")
    @classmethod
    def validate_positive_claim_vocabulary(cls, v: str) -> str:
        # Banned hype vocabulary per quantum-advantage-realism.md
        forbidden = ["quantum supremacy", "unmatched speedup", "miraculous", "quantum leap"]
        lowered = v.lower()
        for term in forbidden:
            if term in lowered:
                raise ValueError(f"Forbidden ungrounded term '{term}' in claim predicate.")
        # Q-04: permitted verbs only — the predicate must state what was done.
        if not any(verb in lowered for verb in PERMITTED_PREDICATE_VERBS):
            raise ValueError(
                "Claim predicate must contain a permitted reporting verb "
                f"{sorted(PERMITTED_PREDICATE_VERBS)}; got no permitted verb. "
                "Reports state measurements, never achievements."
            )
        return v

    @model_validator(mode="after")
    def validate_advantage_realism(self) -> ScientificClaim:
        # Quantum Advantage Realism check
        # Advantage can ONLY be claimed if chemical accuracy is achieved AND delta < 1.6e-3 Ha
        # AND category is QPU_MEASURED or EMPIRICAL_SIMULATION with positive justification.
        if self.claims_advantage:
            if not self.chemical_accuracy_achieved:
                raise ValueError(
                    "Violation of Quantum Advantage Realism: cannot claim advantage when chemical accuracy is not achieved."
                )
            if abs(self.delta_hartree) > 1.6e-3:
                raise ValueError(
                    f"Violation of Quantum Advantage Realism: delta ({self.delta_hartree:.6f} Ha) exceeds 1.6 mHa threshold."
                )
            if not self.advantage_justification or len(self.advantage_justification.strip()) < 15:
                raise ValueError(
                    "Claims of advantage require rigorous mathematical or algorithmic justification."
                )
            # Q-04: advantage claims must carry a credibility field — the score
            # is never cited without its refs — at rho >= 0.90, tier L4/L3.
            if self.credibility is None:
                raise ValueError(
                    "Claims of advantage require a credibility field (rho, tier, evidence_refs)."
                )
            if self.credibility.rho < 0.90 or self.credibility.tier not in ("L4", "L3"):
                raise ValueError(
                    f"Advantage requires credibility rho >= 0.90 at tier L4/L3; got "
                    f"rho={self.credibility.rho}, tier={self.credibility.tier}."
                )
        return self

    def formatted_statement(self) -> str:
        tag = f"[{self.category.value}]"
        adv_tag = "[ADVANTAGE GROUNDED]" if self.claims_advantage else "[NO ADVANTAGE CLAIMED]"
        base = f"{tag} {adv_tag} {self.predicate} (Δ={self.delta_hartree:.4f} Ha, σ={self.uncertainty_sigma:.4f})"
        if self.credibility is not None:
            c = self.credibility
            base += f" [ρ={c.rho:.3f} {c.tier} refs={len(c.evidence_refs)}]"
        return base
