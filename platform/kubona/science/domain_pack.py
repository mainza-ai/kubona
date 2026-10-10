"""Domain Pack Manifest Engine (Ring 2).

Standardizes modular science extensions via pack.yaml manifests.
Packs define Hamiltonians, classical anchor algorithms, and benchmark baselines
so new scientific domains (chemistry, materials, optimization) can be plugged in modularly.
"""

from __future__ import annotations

import hashlib
import importlib
import json
import os
import re
import time
from pathlib import Path
from typing import Any, Dict, List, Optional
from pydantic import BaseModel, Field, field_validator, model_validator

import yaml


PACK_SCHEMA_VERSION = "kubona-pack/v1"

_SEMVER_RE = re.compile(r"^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$")

# Generators must live inside our own tree: packs declare capabilities, never
# supply chains. Anything outside kubona.* refuses at resolve time.
GENERATOR_ALLOWLIST_ROOTS = ("kubona.",)


class PackError(ValueError):
    """Raised when a pack manifest is invalid, unresolvable, or revoked."""


class EvidenceRequirements(BaseModel):
    """What a run under this pack must produce to count as evidence."""

    min_shots: int = Field(default=1024, ge=1)
    collapse_mode: str = "corroborated"
    fidelity_floor: float = Field(default=0.8, ge=0.0, le=1.0)


class DomainPackManifest(BaseModel):
    name: str
    version: str
    schema_version: str = PACK_SCHEMA_VERSION
    domain_category: str  # "chemistry", "condensed_matter", "optimization"
    description: str
    author: str
    classical_anchor_methods: List[str]
    benchmark_systems: List[str]
    hamiltonian_generator: str
    min_qubit_support: int = Field(ge=1)
    capabilities: List[str] = Field(default_factory=list)
    evidence_requirements: EvidenceRequirements = Field(default_factory=EvidenceRequirements)
    entry_points: Dict[str, str] = Field(default_factory=dict)
    owner_pubkey_hex: Optional[str] = None
    signature_hex: Optional[str] = None

    @field_validator("version")
    @classmethod
    def _semver(cls, v: str) -> str:
        if not _SEMVER_RE.match(v or ""):
            raise ValueError(f"pack version {v!r} is not X.Y.Z semver: refusing")
        return v

    @field_validator("schema_version")
    @classmethod
    def _schema_version(cls, v: str) -> str:
        if v != PACK_SCHEMA_VERSION:
            raise ValueError(f"unsupported pack schema {v!r}: want {PACK_SCHEMA_VERSION}")
        return v

    @field_validator("classical_anchor_methods")
    @classmethod
    def _anchor_required(cls, v: List[str]) -> List[str]:
        if not v:
            raise ValueError("a pack that cannot state its classical anchor does not load")
        return v

    def canonical_bytes(self) -> bytes:
        """Deterministic bytes the owner signs (everything except the signature)."""
        d = self.model_dump(mode="json", exclude={"signature_hex"})
        return json.dumps(d, sort_keys=True).encode()

    def content_hash(self) -> str:
        return hashlib.sha256(self.canonical_bytes()).hexdigest()

    def verify_signature(self) -> None:
        """Verify owner attestation. Unsigned packs load (author is a claim);
        signed-but-invalid packs refuse (forgery is worse than anonymity)."""
        if not self.signature_hex:
            return
        if not self.owner_pubkey_hex:
            raise PackError("signed pack without owner pubkey: refusing")
        try:
            from nacl.signing import VerifyKey
            vk = VerifyKey(bytes.fromhex(self.owner_pubkey_hex))
            vk.verify(self.canonical_bytes(), bytes.fromhex(self.signature_hex))
        except Exception as e:
            raise PackError(f"pack signature invalid for {self.name}: {e}")


class DomainPackLoader:
    def __init__(self, packs_dir: str | Path | None = None):
        self.packs_dir = Path(packs_dir or "platform/domain_packs")
        self.packs_dir.mkdir(parents=True, exist_ok=True)

    def load_pack(self, pack_yaml_path: str | Path) -> DomainPackManifest:
        with open(pack_yaml_path, "r", encoding="utf-8") as f:
            data = yaml.safe_load(f)
        try:
            manifest = DomainPackManifest(**data)
        except Exception as e:
            raise PackError(f"invalid manifest {pack_yaml_path}: {e}")
        manifest.verify_signature()
        return manifest

    def register_default_packs(self) -> List[DomainPackManifest]:
        chem_dir = self.packs_dir / "chemistry_v1"
        chem_dir.mkdir(parents=True, exist_ok=True)
        chem_manifest = {
            "name": "quantum-molecular-chemistry",
            "version": "1.0.0",
            "schema_version": PACK_SCHEMA_VERSION,
            "domain_category": "chemistry",
            "description": "Standard STO-3G minimal basis molecular Hamiltonian pack with FCI/RHF anchors.",
            "author": "Kubona Quantum Epistemic Core",
            "classical_anchor_methods": ["RHF", "Exact-Diagonalization-FCI", "DMRG"],
            "benchmark_systems": ["H2", "LiH", "BeH2"],
            "hamiltonian_generator": "kubona.anchors.chemistry.MolecularAnchorEngine",
            "min_qubit_support": 2,
            "capabilities": ["vqe-chemistry", "anchor-fci"],
            "evidence_requirements": {"min_shots": 1024, "collapse_mode": "corroborated",
                                       "fidelity_floor": 0.8},
            "entry_points": {"anchor_engine": "kubona.anchors.chemistry.MolecularAnchorEngine"},
        }
        with open(chem_dir / "pack.yaml", "w", encoding="utf-8") as f:
            yaml.safe_dump(chem_manifest, f)

        mat_dir = self.packs_dir / "materials_condensed_matter_v1"
        mat_dir.mkdir(parents=True, exist_ok=True)
        mat_manifest = {
            "name": "condensed-matter-hubbard-lattice",
            "version": "1.0.0",
            "schema_version": PACK_SCHEMA_VERSION,
            "domain_category": "condensed_matter",
            "description": "1D and 2D Fermi-Hubbard lattice model with Matrix Product State baselines.",
            "author": "Kubona Quantum Epistemic Core",
            "classical_anchor_methods": ["Matrix-Product-States", "Exact-Diagonalization"],
            "benchmark_systems": ["Hubbard_1D_4site", "Hubbard_2D_4site"],
            "hamiltonian_generator": "kubona.science.lattice.HubbardModel",
            "min_qubit_support": 4,
            "capabilities": ["hubbard-lattice", "anchor-mps"],
            "evidence_requirements": {"min_shots": 1024, "collapse_mode": "corroborated",
                                       "fidelity_floor": 0.8},
            "entry_points": {},
        }
        with open(mat_dir / "pack.yaml", "w", encoding="utf-8") as f:
            yaml.safe_dump(mat_manifest, f)

        math_dir = self.packs_dir / "math_physics_v1"
        math_dir.mkdir(parents=True, exist_ok=True)
        math_manifest = {
            "name": "mathematical-physics-frontier",
            "version": "1.0.0",
            "schema_version": PACK_SCHEMA_VERSION,
            "domain_category": "mathematical_physics",
            "description": "Rigorous analytical ground truth baselines and exact proofs from openai/math.",
            "author": "Kubona Quantum Epistemic Core",
            "classical_anchor_methods": ["Analytical-Math-Proof", "Bloch-Spin-Wave", "Parisi-RSB", "Haldane-Gap-Proof"],
            "benchmark_systems": ["Heisenberg_Ferromagnet_3D", "SK_Spin_Glass_Ground_State", "Haldane_Spin_1_Chain"],
            "hamiltonian_generator": "kubona.anchors.math_physics.MathPhysicsAnchorEngine",
            "min_qubit_support": 2,
            "capabilities": ["math-anchors", "bloch-law", "sk-parisi"],
            "evidence_requirements": {"min_shots": 1024, "collapse_mode": "corroborated",
                                       "fidelity_floor": 0.8},
            "entry_points": {"anchor_engine": "kubona.anchors.math_physics.MathPhysicsAnchorEngine"},
        }
        with open(math_dir / "pack.yaml", "w", encoding="utf-8") as f:
            yaml.safe_dump(math_manifest, f)

        return [
            self.load_pack(chem_dir / "pack.yaml"),
            self.load_pack(mat_dir / "pack.yaml"),
            self.load_pack(math_dir / "pack.yaml"),
        ]


def resolve_generator(dotted_path: str) -> Any:
    """Import a pack's generator class. Allowlisted to our own tree: packs
    declare capabilities, never supply chains — anything outside kubona.*
    refuses (fail-closed against dependency smuggling)."""
    if not any(dotted_path == root.rstrip(".") or dotted_path.startswith(root)
               for root in GENERATOR_ALLOWLIST_ROOTS):
        raise PackError(
            f"generator {dotted_path!r} outside allowlist {GENERATOR_ALLOWLIST_ROOTS}: refused")
    mod_name, _, attr = dotted_path.rpartition(".")
    if not mod_name or not attr:
        raise PackError(f"malformed generator path {dotted_path!r}: refused")
    try:
        mod = importlib.import_module(mod_name)
    except ImportError as e:
        raise PackError(f"generator module {mod_name!r} unresolvable: {e}")
    try:
        return getattr(mod, attr)
    except AttributeError:
        raise PackError(f"generator {dotted_path!r} has no attribute {attr!r}: refused")


class PackRegistry:
    """Trust + revocation for packs. Revocation lives here (a registry file),
    never in the pack: a revoked pack refuses everywhere it is loaded."""

    def __init__(self, packs_dir: str | Path | None = None,
                 revocations_path: str | Path | None = None):
        self.loader = DomainPackLoader(packs_dir)
        self.revocations_path = Path(
            revocations_path or (Path(self.loader.packs_dir) / "revoked.json"))

    def _revoked_ids(self) -> set[str]:
        if not self.revocations_path.exists():
            return set()
        try:
            data = json.loads(self.revocations_path.read_text())
        except (json.JSONDecodeError, OSError):
            return set()
        return {str(e.get("pack_id")) for e in data if e.get("pack_id")}

    def is_revoked(self, pack_id: str) -> bool:
        return pack_id in self._revoked_ids()

    def revoke(self, pack_id: str, reason: str) -> dict[str, Any]:
        """Operator revocation: recorded with reason + timestamp. Append-only
        in spirit (entries are never edited, only superseded by re-issue)."""
        if not reason or not reason.strip():
            raise PackError("revocation requires a reason")
        entry = {"pack_id": pack_id, "reason": reason.strip(),
                 "revoked_at": time.time()}
        existing: list[dict[str, Any]] = []
        if self.revocations_path.exists():
            try:
                existing = json.loads(self.revocations_path.read_text())
            except (json.JSONDecodeError, OSError):
                existing = []
        existing.append(entry)
        self.revocations_path.parent.mkdir(parents=True, exist_ok=True)
        self.revocations_path.write_text(json.dumps(existing, indent=2))
        return entry

    def load_all(self) -> Dict[str, DomainPackManifest]:
        """Load + validate every pack.yaml; revoked packs refuse."""
        out: Dict[str, DomainPackManifest] = {}
        if not self.loader.packs_dir.exists():
            return out
        for path in sorted(self.loader.packs_dir.glob("*/pack.yaml")):
            manifest = self.loader.load_pack(path)
            if self.is_revoked(manifest.name):
                raise PackError(
                    f"pack {manifest.name!r} is revoked; refusing to load")
            out[manifest.name] = manifest
        return out
