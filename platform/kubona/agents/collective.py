"""Multi-Agent Research Collective skeptic (Ring 2).

W2.13: the RoleType / AgentRoleManifest scaffolding was deleted — zero
in-repo callers (verified by grep over platform/, including tests) and no
wiring into any admission path. What remains is the disconfirmation logic
itself, kept as a TESTED reference implementation with an explicit status:
as of 2026-10-17 it has no production caller, so nothing in the platform may
claim skeptic coverage until it is wired into one.
"""

from __future__ import annotations

from typing import List
from pydantic import BaseModel, Field


class DisconfirmationVerdict(BaseModel):
    is_falsified: bool
    skeptic_notes: str
    classical_shortcut_found: bool = False
    evidence_defects: List[str] = Field(default_factory=list)


class DisconfirmationOfficer:
    """The mandatory automated scientific skeptic."""

    def challenge_finding(
        self,
        experiment_energy: float,
        classical_anchor_energy: float,
        chemical_accuracy_threshold: float = 1.6e-3,
        qubits: int = 2,
    ) -> DisconfirmationVerdict:
        defects = []
        delta = abs(experiment_energy - classical_anchor_energy)

        # 1. Did the run surpass classical accuracy?
        if delta > chemical_accuracy_threshold:
            defects.append(
                f"Variational energy ({experiment_energy:.4f} Ha) differs from classical anchor "
                f"({classical_anchor_energy:.4f} Ha) by {delta*1000:.2f} mHa, which is above chemical accuracy."
            )

        # 2. Is this system trivially classically solvable?
        classical_shortcut = qubits <= 4
        if classical_shortcut:
            defects.append(
                f"System width ({qubits} qubits) is in the classically trivial regime; "
                f"exact classical diagonalization requires < 1ms."
            )

        is_falsified = len(defects) > 0
        notes = (
            "Finding rejected by Disconfirmation Officer: does not beat classical anchor."
            if is_falsified
            else "Finding withstood disconfirmation challenges."
        )

        return DisconfirmationVerdict(
            is_falsified=is_falsified,
            skeptic_notes=notes,
            classical_shortcut_found=classical_shortcut,
            evidence_defects=defects,
        )
