"""The Kubona Epistemic Continuum, Computational Hypothesis, and Stopping Rules.

Formalizes the 5-level epistemic continuum from machine-checked deductive proof
(rho = 1.0) down to stochastic quantum execution (rho < 0.90). Defines the
computational hypothesis tuple H, the Fisher information stopping rule,
and first-class negative certificates.
"""

from __future__ import annotations

import hashlib
import json
import math
from dataclasses import dataclass, field
from enum import Enum
from typing import Any, Dict, List, Optional


class EpistemicLevel(str, Enum):
    """The 5-tier Epistemic Continuum levels."""
    LEVEL_4_FORMAL_DEDUCTION = "LEVEL_4_FORMAL_DEDUCTION"        # rho = 1.0 (Lean 4 / Comparator)
    LEVEL_3_ANALYTICAL_ANCHOR = "LEVEL_3_ANALYTICAL_ANCHOR"       # rho -> 1.0- (openai/math families)
    LEVEL_2_CLASSICAL_BASELINE = "LEVEL_2_CLASSICAL_BASELINE"     # 0.90 <= rho < 1.0 (PySCF FCI / Slurm DMRG)
    LEVEL_1_QUANTUM_SUBSPACE = "LEVEL_1_QUANTUM_SUBSPACE"         # 0.70 <= rho < 0.90 (QKSD / Shadows / QPU)
    LEVEL_0_EXPLORATORY_SEARCH = "LEVEL_0_EXPLORATORY_SEARCH"     # rho < 0.70 (Ansatz search / agent conjectures)


@dataclass(frozen=True)
class ComputationalHypothesis:
    """The formal computational hypothesis tuple H.

    H = <S_Lean4, A_anchor, C_QIR, B_AP2, DeltaS_target>
    """
    hypothesis_id: str
    lean4_statement: Optional[str]              # S_Lean4: Formal proposition in Lean 4
    anchor_system: Optional[str]                # A_anchor: Analytical anchor identifier
    circuit_qir: Optional[str]                  # C_QIR: QIR / OpenQASM circuit or ansatz
    spend_budget_usd: float                     # B_AP2: Budget allocated under spend mandate
    mandate_signature: Optional[str]            # AP2 cryptographic Ed25519 signature
    target_entropy_reduction: float             # Delta S_target: Expected Fisher information gain
    description: str = ""
    created_at_iso: str = ""

    def content_hash(self) -> str:
        """Deterministic content-addressed SHA-256 hash of the hypothesis tuple."""
        payload = {
            "lean4": self.lean4_statement or "",
            "anchor": self.anchor_system or "",
            "circuit": self.circuit_qir or "",
            "budget": self.spend_budget_usd,
            "signature": self.mandate_signature or "",
            "target_delta_s": self.target_entropy_reduction,
            "description": self.description,
        }
        encoded = json.dumps(payload, sort_keys=True).encode("utf-8")
        return hashlib.sha256(encoded).hexdigest()

    def to_dict(self) -> Dict[str, Any]:
        return {
            "hypothesis_id": self.hypothesis_id,
            "content_hash": self.content_hash(),
            "lean4_statement": self.lean4_statement,
            "anchor_system": self.anchor_system,
            "circuit_qir": self.circuit_qir,
            "spend_budget_usd": self.spend_budget_usd,
            "mandate_signature": self.mandate_signature,
            "target_entropy_reduction": self.target_entropy_reduction,
            "description": self.description,
        }


@dataclass
class NegativeCertificate:
    """First-class cryptographic negative certificate for failed or refuted hypotheses."""
    certificate_id: str
    hypothesis_hash: str
    refutation_type: str  # "AXIOMATIC_CONTRADICTION", "BARREN_PLATEAU", "ENERGY_BOUND_VIOLATION", "BUDGET_EXHAUSTED"
    evidence_summary: str
    ledger_hash: Optional[str] = None
    properties: Dict[str, Any] = field(default_factory=dict)

    def to_dict(self) -> Dict[str, Any]:
        return {
            "certificate_id": self.certificate_id,
            "hypothesis_hash": self.hypothesis_hash,
            "refutation_type": self.refutation_type,
            "evidence_summary": self.evidence_summary,
            "ledger_hash": self.ledger_hash,
            "properties": self.properties,
        }


class EpistemicContinuum:
    """Manages the epistemic confidence evaluation, stopping rules, and certificates."""

    @staticmethod
    def classify_level(rho: float) -> EpistemicLevel:
        """Classifies confidence rho into its continuum level."""
        if rho >= 0.999999:
            return EpistemicLevel.LEVEL_4_FORMAL_DEDUCTION
        elif rho >= 0.95:
            return EpistemicLevel.LEVEL_3_ANALYTICAL_ANCHOR
        elif rho >= 0.90:
            return EpistemicLevel.LEVEL_2_CLASSICAL_BASELINE
        elif rho >= 0.70:
            return EpistemicLevel.LEVEL_1_QUANTUM_SUBSPACE
        else:
            return EpistemicLevel.LEVEL_0_EXPLORATORY_SEARCH

    @staticmethod
    def evaluate_fisher_stopping_rule(
        expected_fisher_info: float,
        marginal_cost_per_step: float,
        lambda_multiplier: float = 1.0,
    ) -> bool:
        """Evaluates the Fisher Information Stopping Rule:

        E[I_Fisher(theta)] <= lambda_AP2 * (dCost / dStep)
        Returns True if the search should STOP, False if exploration should continue.
        """
        if marginal_cost_per_step <= 0.0:
            # Free step: continue exploration if there is any information gain
            return expected_fisher_info <= 1e-12
        threshold = lambda_multiplier * marginal_cost_per_step
        return expected_fisher_info <= threshold

    @classmethod
    def create_negative_certificate(
        cls,
        hypothesis: ComputationalHypothesis,
        refutation_type: str,
        evidence: str,
        extra_props: Optional[Dict[str, Any]] = None,
    ) -> NegativeCertificate:
        """Creates an immutable negative certificate."""
        h_hash = hypothesis.content_hash()
        cert_id = f"neg_cert_{h_hash[:16]}"
        return NegativeCertificate(
            certificate_id=cert_id,
            hypothesis_hash=h_hash,
            refutation_type=refutation_type,
            evidence_summary=evidence,
            properties=extra_props or {},
        )
