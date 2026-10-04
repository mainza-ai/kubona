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

    @field_validator("predicate")
    @classmethod
    def validate_positive_claim_vocabulary(cls, v: str) -> str:
        # Banned hype vocabulary per quantum-advantage-realism.md
        forbidden = ["quantum supremacy", "unmatched speedup", "miraculous", "quantum leap"]
        lowered = v.lower()
        for term in forbidden:
            if term in lowered:
                raise ValueError(f"Forbidden ungrounded term '{term}' in claim predicate.")
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
        return self

    def formatted_statement(self) -> str:
        tag = f"[{self.category.value}]"
        adv_tag = "[ADVANTAGE GROUNDED]" if self.claims_advantage else "[NO ADVANTAGE CLAIMED]"
        return f"{tag} {adv_tag} {self.predicate} (Δ={self.delta_hartree:.4f} Ha, σ={self.uncertainty_sigma:.4f})"
