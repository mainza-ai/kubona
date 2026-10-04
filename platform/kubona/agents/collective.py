"""Multi-Agent Research Collective & Role Manifests (Ring 2).

Defines specialized roles with least-privilege tool grants:
1. ansatz-scout: Proposes quantum circuit topologies.
2. compiler-scout: Optimizes hardware gate passes.
3. disconfirmation-officer: Mandatory automated skeptic that attempts to
   invalidate any claimed finding using classical shortcuts, barren plateau
   detection, or baseline anchors before a report is certified.
"""

from __future__ import annotations

import enum
from dataclasses import dataclass, field
from typing import Dict, List, Optional
from pydantic import BaseModel, Field


class RoleType(str, enum.Enum):
    ANSATZ_SCOUT = "ansatz-scout"
    COMPILER_SCOUT = "compiler-scout"
    DISCONFIRMATION_OFFICER = "disconfirmation-officer"
    DEVICE_AUDITOR = "device-auditor"


class AgentRoleManifest(BaseModel):
    role: RoleType
    name: str
    description: str
    allowed_tools: List[str]
    read_only: bool = True
    max_step_budget: int = 20


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
