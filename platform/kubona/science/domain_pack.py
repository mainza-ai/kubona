"""Domain Pack Manifest Engine (Ring 2).

Standardizes modular science extensions via pack.yaml manifests.
Packs define Hamiltonians, classical anchor algorithms, and benchmark baselines
so new scientific domains (chemistry, materials, optimization) can be plugged in modularly.
"""

from __future__ import annotations

import os
from pathlib import Path
from typing import Any, Dict, List, Optional
from pydantic import BaseModel, Field
import yaml


class DomainPackManifest(BaseModel):
    name: str
    version: str
    domain_category: str  # "chemistry", "condensed_matter", "optimization"
    description: str
    author: str
    classical_anchor_methods: List[str]
    benchmark_systems: List[str]
    hamiltonian_generator: str
    min_qubit_support: int = Field(ge=1)


class DomainPackLoader:
    def __init__(self, packs_dir: str | Path | None = None):
        self.packs_dir = Path(packs_dir or "platform/domain_packs")
        self.packs_dir.mkdir(parents=True, exist_ok=True)

    def load_pack(self, pack_yaml_path: str | Path) -> DomainPackManifest:
        with open(pack_yaml_path, "r", encoding="utf-8") as f:
            data = yaml.safe_load(f)
        return DomainPackManifest(**data)

    def register_default_packs(self) -> List[DomainPackManifest]:
        chem_dir = self.packs_dir / "chemistry_v1"
        chem_dir.mkdir(parents=True, exist_ok=True)
        chem_manifest = {
            "name": "quantum-molecular-chemistry",
            "version": "1.0.0",
            "domain_category": "chemistry",
            "description": "Standard STO-3G minimal basis molecular Hamiltonian pack with FCI/RHF anchors.",
            "author": "Kubona Quantum Epistemic Core",
            "classical_anchor_methods": ["RHF", "Exact-Diagonalization-FCI", "DMRG"],
            "benchmark_systems": ["H2", "LiH", "BeH2"],
            "hamiltonian_generator": "kubona.anchors.chemistry.MolecularAnchorEngine",
            "min_qubit_support": 2,
        }
        with open(chem_dir / "pack.yaml", "w", encoding="utf-8") as f:
            yaml.safe_dump(chem_manifest, f)

        mat_dir = self.packs_dir / "materials_condensed_matter_v1"
        mat_dir.mkdir(parents=True, exist_ok=True)
        mat_manifest = {
            "name": "condensed-matter-hubbard-lattice",
            "version": "1.0.0",
            "domain_category": "condensed_matter",
            "description": "1D and 2D Fermi-Hubbard lattice model with Matrix Product State baselines.",
            "author": "Kubona Quantum Epistemic Core",
            "classical_anchor_methods": ["Matrix-Product-States", "Exact-Diagonalization"],
            "benchmark_systems": ["Hubbard_1D_4site", "Hubbard_2D_4site"],
            "hamiltonian_generator": "kubona.science.lattice.HubbardModel",
            "min_qubit_support": 4,
        }
        with open(mat_dir / "pack.yaml", "w", encoding="utf-8") as f:
            yaml.safe_dump(mat_manifest, f)

        return [
            self.load_pack(chem_dir / "pack.yaml"),
            self.load_pack(mat_dir / "pack.yaml"),
        ]
