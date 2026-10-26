"""Refuted-Genome Prior Library and Cross-Device Hardware Transfer (B.3).

Codifies failure taxonomies:
  - Intrinsic mathematical failures (symmetry breaking, barren plateau, expressibility deficit)
    are permanently and globally banned across all quantum backends.
  - Hardware-dependent failures (decoherence/depth, routing overhead) are conditionally
    pruned based on target device noise profiles and connectivity graphs.
"""

from __future__ import annotations

from dataclasses import dataclass, field
from enum import Enum
from typing import Any, Dict, List, Optional, Set
import numpy as np


class FailureClass(str, Enum):
    INTRINSIC_SYMMETRY_VIOLATION = "INTRINSIC_SYMMETRY_VIOLATION"
    INTRINSIC_BARREN_PLATEAU = "INTRINSIC_BARREN_PLATEAU"
    INTRINSIC_EXPRESSIBILITY_DEFICIT = "INTRINSIC_EXPRESSIBILITY_DEFICIT"
    HARDWARE_COHERENCE_LIMIT = "HARDWARE_COHERENCE_LIMIT"
    ROUTING_OVERHEAD_EXCESS = "ROUTING_OVERHEAD_EXCESS"


@dataclass(frozen=True)
class AnsatzGenome:
    family: str
    depth: int
    n_qubits: int
    entangler: str  # "linear", "all_to_all", "circular", "star"
    symmetry: str = "none"  # "particle_number", "spin", "none"

    @property
    def signature(self) -> str:
        return f"{self.family}:d={self.depth}:q={self.n_qubits}:{self.entangler}:{self.symmetry}"


@dataclass
class GenomeEvaluationRecord:
    genome: AnsatzGenome
    target_hamiltonian: str
    device_id: str
    survives: bool
    failure_class: Optional[FailureClass] = None
    gradient_variance: float = 1.0
    expressibility_q: float = 1.0
    predicted_fidelity: float = 1.0
    achieved_energy: float = 0.0


class RefutedGenomeLibrary:
    """Maintains proven refutations and evaluates transfer eligibility across devices."""

    def __init__(self):
        self._records: List[GenomeEvaluationRecord] = []
        self._intrinsic_blacklist: Set[str] = set()

    def record_refutation(self, record: GenomeEvaluationRecord) -> None:
        """Records an evaluation outcome and indexes intrinsic failure bans."""
        self._records.append(record)
        if not record.survives and record.failure_class in {
            FailureClass.INTRINSIC_SYMMETRY_VIOLATION,
            FailureClass.INTRINSIC_BARREN_PLATEAU,
            FailureClass.INTRINSIC_EXPRESSIBILITY_DEFICIT,
        }:
            self._intrinsic_blacklist.add(record.genome.signature)

    def can_transfer_to_device(
        self,
        genome: AnsatzGenome,
        target_device_id: str,
        device_t2_us: float,
        gate_time_ns: float = 200.0,
        fidelity_floor: float = 0.80,
    ) -> tuple[bool, Optional[str]]:
        """Evaluates whether an ansatz genome is admissible on a new hardware target."""
        # 1. Check global intrinsic failure bans
        if genome.signature in self._intrinsic_blacklist:
            return False, f"Refused: genome {genome.signature} is globally refuted (intrinsic mathematical flaw)"

        # 2. Check depth against target coherence time
        estimated_circuit_time_ns = genome.depth * gate_time_ns
        t2_ns = device_t2_us * 1000.0
        if estimated_circuit_time_ns > t2_ns * 0.5:
            return False, (
                f"Refused: estimated circuit duration ({estimated_circuit_time_ns:.1f}ns) "
                f"exceeds 50% of device T2 ({t2_ns:.1f}ns) on {target_device_id}"
            )

        # 3. Check historical records on same/similar device class
        for rec in self._records:
            if rec.genome == genome and rec.device_id == target_device_id and not rec.survives:
                if rec.predicted_fidelity < fidelity_floor:
                    return False, f"Refused: previously refuted on {target_device_id} with fidelity {rec.predicted_fidelity:.3f} < {fidelity_floor}"

        return True, None
