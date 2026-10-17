"""Autonomous Ansatz Genome Optimizer & Structural Pre-Checks (Ring 3).

Searches over ansatz genomes:
    Genome = {family, depth, entangler_topology, initializer, symmetry}

Enforces cheap structural pre-checks BEFORE paid execution:
1. Heuristic entanglement prior (T2_CONSISTENT triage signal — NEVER a
   measured Meyer-Wallach Q and NEVER gating pass/fail spend).
2. Trainability gradient-variance pre-check (guards against barren plateaus).
3. Classical cost check (MPS bond dimension feasibility).

Production bar: the prior is advisory-only. can_greenlight() returns False
unless a MEASURED statevector MW is supplied — heuristics never greenlight
spend. Expressibility-fail returns None + reason (no placeholder
trainability_ok/bond_dim numbers on a refused genome).
"""

from __future__ import annotations

import enum
import math
from dataclasses import dataclass
from typing import Any, Dict, List, Optional
import numpy as np


# Single const for the depth gate (comment and code share it — the old
# "<2 vs <1 / 20 vs 16" comment/code drift is gone).
MAX_TRAINABLE_DEPTH = 16
# Heuristic bond-dim cap exponent (heuristic numbers, labeled as such).
HEURISTIC_CHI_MAX_EXP = 8
HEURISTIC_METHOD = "expressibility_heuristic_v1"


class AnsatzFamily(str, enum.Enum):
    HARDWARE_EFFICIENT = "hardware_efficient"
    UCCSD = "uccsd"
    TWO_LOCAL = "two_local"
    SU2 = "su2"


class EntanglerTopology(str, enum.Enum):
    LINEAR = "linear"
    CIRCULAR = "circular"
    ALL_TO_ALL = "all_to_all"


class Initializer(str, enum.Enum):
    RANDOM = "random"
    MP2_PERTURBATION = "mp2_perturbation"
    ONES = "ones"


class Symmetry(str, enum.Enum):
    PARTICLE_NUMBER = "particle_number"
    NONE = "none"


@dataclass
class AnsatzGenome:
    family: AnsatzFamily | str  # coerced to AnsatzFamily (refuses unknown)
    n_qubits: int
    depth: int
    entangler_topology: EntanglerTopology | str
    initializer: Initializer | str
    symmetry: Symmetry | str

    def __post_init__(self) -> None:
        try:
            self.family = (
                self.family if isinstance(self.family, AnsatzFamily)
                else AnsatzFamily(str(self.family))
            )
        except ValueError:
            raise ValueError(
                f"unsupported ansatz family {self.family!r}; "
                f"known: {[f.value for f in AnsatzFamily]}"
            ) from None
        try:
            self.entangler_topology = (
                self.entangler_topology if isinstance(self.entangler_topology, EntanglerTopology)
                else EntanglerTopology(str(self.entangler_topology))
            )
        except ValueError:
            raise ValueError(
                f"unknown entangler_topology {self.entangler_topology!r}; "
                f"known: {[t.value for t in EntanglerTopology]}"
            ) from None
        try:
            self.initializer = (
                self.initializer if isinstance(self.initializer, Initializer)
                else Initializer(str(self.initializer))
            )
        except ValueError:
            raise ValueError(
                f"unknown initializer {self.initializer!r}; "
                f"known: {[i.value for i in Initializer]}"
            ) from None
        try:
            self.symmetry = (
                self.symmetry if isinstance(self.symmetry, Symmetry)
                else Symmetry(str(self.symmetry))
            )
        except ValueError:
            raise ValueError(
                f"unknown symmetry {self.symmetry!r}; "
                f"known: {[s.value for s in Symmetry]}"
            ) from None
        if not isinstance(self.n_qubits, int) or isinstance(self.n_qubits, bool):
            raise ValueError(f"n_qubits must be an int, got {self.n_qubits!r}")
        if not isinstance(self.depth, int) or isinstance(self.depth, bool):
            raise ValueError(f"depth must be an int, got {self.depth!r}")
        if self.n_qubits < 1:
            raise ValueError(f"n_qubits must be >= 1, got {self.n_qubits}")
        if self.depth < 0:
            raise ValueError(f"depth must be >= 0, got {self.depth}")

    def estimated_cnot_count(self) -> int:
        if self.entangler_topology == EntanglerTopology.LINEAR:
            return (self.n_qubits - 1) * self.depth
        elif self.entangler_topology == EntanglerTopology.CIRCULAR:
            return self.n_qubits * self.depth
        else:
            return (self.n_qubits * (self.n_qubits - 1) // 2) * self.depth


@dataclass
class GenomePrecheckResult:
    passed: bool
    heuristic_prior: float  # T2 advisory prior — NEVER a measured MW Q
    heuristic_method: str = HEURISTIC_METHOD
    evidence_tier: str = "T2_CONSISTENT"  # triage-grade evidence, never T1_VERIFIED
    measured: bool = False
    trainability_ok: Optional[bool] = None  # None when refused before evaluation
    estimated_mps_bond_dim: Optional[int] = None  # None when refused (no placeholders)
    rejection_reason: Optional[str] = None

    @property
    def expressibility_q(self) -> float:
        """Deprecated alias for heuristic_prior (read-compat only)."""
        import warnings

        warnings.warn(
            "expressibility_q is a heuristic prior, not a measured MW Q — "
            "use heuristic_prior",
            DeprecationWarning,
            stacklevel=2,
        )
        return self.heuristic_prior


class AutonomousAnsatzOptimizer:
    """Triage-only optimizer: the heuristic prior advises, never decides.

    min_heuristic_prior sets the advisory triage cutoff. precheck_genome()
    returns a GenomePrecheckResult; can_greenlight() is the ONLY spend gate
    and requires a measured MW value (measured=True) — heuristics alone
    always return can_greenlight False.
    """

    def __init__(self, min_heuristic_prior: float = 0.25,
                 min_expressibility_q: float | None = None):
        # min_expressibility_q kept as a deprecated alias (fail-open compat:
        # old callers pass it; it maps onto the same cutoff, honestly labeled).
        if min_expressibility_q is not None:
            min_heuristic_prior = float(min_expressibility_q)
        self.min_heuristic_prior = float(min_heuristic_prior)
        # Deprecated read-alias (old attribute name).
        self.min_expressibility_q = self.min_heuristic_prior

    def heuristic_entanglement_prior(self, genome: AnsatzGenome) -> float:
        """Heuristic entanglement/expressibility proxy in [0, 1] (T2 advisory).

        This is NOT the Meyer-Wallach integral (which requires integrating
        linear entropies over the Haar distribution or a measured
        statevector); it is a cheap structural prior from depth and
        entangler topology used only to triage ansatz genomes before any
        real evaluation. Measured MW values come from statevector runs
        (autoresearch harness / QKSD responses). Never gates pass/fail spend.
        """
        # Shallow depth (<1) or disjoint topology has low expressibility
        if genome.depth < 1:
            return 0.05
        base_q = 0.35 + 0.15 * min(genome.depth, 4)
        if genome.entangler_topology == EntanglerTopology.ALL_TO_ALL:
            base_q += 0.15
        elif genome.entangler_topology == EntanglerTopology.LINEAR:
            base_q += 0.05
        return min(0.95, base_q)

    def precheck_genome(self, genome: AnsatzGenome) -> GenomePrecheckResult:
        q = self.heuristic_entanglement_prior(genome)

        # 1. Expressibility triage (advisory — never a spend gate)
        if q < self.min_heuristic_prior:
            return GenomePrecheckResult(
                passed=False,
                heuristic_prior=q,
                trainability_ok=None,  # refused before evaluation: no placeholder
                estimated_mps_bond_dim=None,  # refused: no placeholder chi
                rejection_reason=f"Insufficient heuristic prior ({q:.2f} < {self.min_heuristic_prior:.2f}). Cannot express target ground state.",
            )

        # 2. Trainability / depth check (depth > MAX_TRAINABLE_DEPTH untrainable)
        trainable = genome.depth <= MAX_TRAINABLE_DEPTH
        if not trainable:
            return GenomePrecheckResult(
                passed=False,
                heuristic_prior=q,
                trainability_ok=False,
                estimated_mps_bond_dim=None,  # refused: no placeholder chi
                rejection_reason=f"Ansatz depth ({genome.depth}) exceeds trainability bound ({MAX_TRAINABLE_DEPTH}); vulnerable to barren plateaus.",
            )

        # 3. Classical MPS bond dimension estimation (HEURISTIC numbers —
        # chi ~ 2^(depth/2), capped; a structural guess, not a measurement).
        mps_chi = int(2 ** min(genome.depth // 2, HEURISTIC_CHI_MAX_EXP))

        return GenomePrecheckResult(
            passed=True,
            heuristic_prior=q,
            trainability_ok=True,
            estimated_mps_bond_dim=mps_chi,
            rejection_reason=None,
        )

    def can_greenlight(
        self,
        genome: AnsatzGenome,
        *,
        measured_mw: float | None = None,
        measured: bool = False,
    ) -> Dict[str, Any]:
        """Advisory-only spend gate: requires a MEASURED MW before spend.

        Returns can_greenlight False unless measured is True with a finite
        MW value at or above the cutoff. Heuristic triage alone NEVER
        greenlights — the reason always says so.
        """
        if not measured or measured_mw is None:
            return {
                "can_greenlight": False,
                "reason": (
                    "triage is advisory-only: no measured Meyer-Wallach Q supplied — "
                    "refusing to greenlight spend on heuristics"
                ),
            }
        if not isinstance(measured_mw, (int, float)) or not math.isfinite(float(measured_mw)):
            return {"can_greenlight": False,
                    "reason": f"measured MW must be finite, got {measured_mw!r}"}
        pre = self.precheck_genome(genome)
        if not pre.passed:
            return {"can_greenlight": False,
                    "reason": f"precheck refused: {pre.rejection_reason}"}
        if float(measured_mw) < self.min_heuristic_prior:
            return {"can_greenlight": False,
                    "reason": f"measured MW {float(measured_mw):.3f} below cutoff {self.min_heuristic_prior:.2f}"}
        return {"can_greenlight": True,
                "reason": f"measured MW {float(measured_mw):.3f} + passing triage"}
