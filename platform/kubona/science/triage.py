"""Quantum-Classical Epistemic Triage Engine (Ring 2 / Science Core).

Implements mathematical triage for quantum molecular and materials problems
based on openai/math Family 278 (Failure of Kohn-Sham ensemble representation)
and Family 275 (QMA-hardness of continuum Coulomb energy).

Mathematically distinguishes when classical DFT/B3LYP is rigorously valid vs
when multi-reference correlation forces execution onto classical tensor networks (DMRG)
or federated QPUs (Quantum Krylov Subspace Diagonalization).
"""

from __future__ import annotations

from dataclasses import dataclass
from enum import Enum
from typing import Any, Dict, List


class TriageTarget(str, Enum):
    CLASSICAL_DFT_HPC = "CLASSICAL_DFT_HPC"
    CLASSICAL_DMRG_TENSOR = "CLASSICAL_DMRG_TENSOR"
    FEDERATED_QUANTUM_QPU = "FEDERATED_QUANTUM_QPU"


import hashlib


@dataclass(frozen=True)
class MolecularSystem:
    molecule_id: str
    formula: str
    num_electrons: int
    nuclear_charges: List[int]
    is_multi_reference: bool
    coulomb_continuum: bool
    estimated_active_space_orbitals: int
    homo_lumo_gap: float = 0.15  # Hartree (Rule 1: Delta E_HOMO-LUMO >= 0.1 Ha is single-ref)
    radical_index: float = 0.01  # Diradical / polyradical character (< 0.05 is single-ref)


class QuantumTriageEngine:
    """Enforces mathematical routing based on openai/math Family 278 and Family 275."""

    @staticmethod
    def triage(system: MolecularSystem) -> Dict[str, Any]:
        """Routes a molecular simulation request based on formal mathematical criteria."""
        # Rule 1 vs Multi-Reference check:
        # A system is multi-reference if declared so, or if HOMO-LUMO gap < 0.1 Ha, or radical index >= 0.05
        is_multi_ref = (
            system.is_multi_reference
            or (system.homo_lumo_gap < 0.1)
            or (system.radical_index >= 0.05)
        )

        # Family 278: Multi-reference Coulomb molecules provably lack non-interacting KS representation
        if is_multi_ref and system.coulomb_continuum:
            if system.estimated_active_space_orbitals > 50:
                # Rule 3: Active space exceeds classical Full Configuration Interaction / DMRG bounds
                target = TriageTarget.FEDERATED_QUANTUM_QPU.value
                algo = "Quantum_Krylov_Subspace_Diagonalization"
                credits = 25.0 * float(system.estimated_active_space_orbitals)
                family_id = 278
                res = {
                    "target": target,
                    "justification": (
                        "Family 278 Kohn-Sham failure certified; active space orbitals "
                        f"({system.estimated_active_space_orbitals}) > 50 exceed classical DMRG. "
                        "Routed to QPU Quantum Krylov Subspace Diagonalization."
                    ),
                    "family_id": family_id,
                    "algorithm_recommendation": algo,
                    "requires_qpu": True,
                    "complexity_regime": "QMA_Hard_Family_275",
                    "estimated_credits": credits,
                }
            else:
                # Rule 2: Multi-reference but classically tractable via DMRG/MPS (Family 265 Area Law)
                target = TriageTarget.CLASSICAL_DMRG_TENSOR.value
                algo = "Classical_DMRG_Tensor_Network"
                credits = 2.5 * float(system.estimated_active_space_orbitals)
                family_id = 265
                res = {
                    "target": target,
                    "justification": (
                        "Multi-reference correlation present, but active space "
                        f"({system.estimated_active_space_orbitals}) <= 50 efficiently solvable via classical DMRG/PEPS."
                    ),
                    "family_id": family_id,
                    "algorithm_recommendation": algo,
                    "requires_qpu": False,
                    "complexity_regime": "Tensor_Network_Area_Law_Family_265",
                    "estimated_credits": credits,
                }
        else:
            # Rule 1: Single-reference system: standard DFT/B3LYP rigorously sufficient
            target = TriageTarget.CLASSICAL_DFT_HPC.value
            algo = "PySCF_DFT_B3LYP"
            credits = 1.0
            family_id = 278
            res = {
                "target": target,
                "justification": (
                    "Single-reference configuration; Kohn-Sham non-interacting representation is physically valid. "
                    "Dispatched to classical Slurm HPC cluster."
                ),
                "family_id": family_id,
                "algorithm_recommendation": algo,
                "requires_qpu": False,
                "complexity_regime": "Polynomial_Classical_DFT",
                "estimated_credits": credits,
            }

        # Cryptographic receipt hash for x402 resource accounting
        receipt_raw = f"{system.molecule_id}:{target}:{family_id}:{algo}:{credits:.2f}"
        res["receipt_hash"] = hashlib.sha256(receipt_raw.encode("utf-8")).hexdigest()
        return res

