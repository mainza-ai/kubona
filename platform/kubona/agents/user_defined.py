"""User-Defined & Standard Agent Registry (Phase 3) — Declarative Agent Ecosystem.

Provides declarative manifests, capability sandboxing, and runtime lifecycle
management for both standard system personas and user-authored scientific agents.

Invariants:
  1. Capability Sandboxing: Custom agents can only call MCP tools explicitly
     declared in their whitelist.
  2. Spend Envelopes: Cannot exceed their declared spend_budget_usd without
     prompting for an Ed25519 signature from the human principal.
  3. Modality Scoping: Restricted to declared modalities (text, 3D molecules,
     circuits, spectroscopy, audio).
"""

from __future__ import annotations

import json
from dataclasses import asdict, dataclass, field
from datetime import datetime, timezone
from pathlib import Path
from typing import Any


@dataclass
class UserDefinedAgentManifest:
    id: str
    name: str
    role: str
    description: str
    system_prompt: str
    avatar_emoji: str = "🧪"
    badge_color: str = "cyan"
    mode: str = "prompt"  # prompt | pipeline
    steps: list[dict[str, Any]] = field(default_factory=list)
    capabilities: list[str] = field(default_factory=list)
    modalities: list[str] = field(default_factory=lambda: ["text"])
    preferred_model: str = ""  # Unpinned by policy: personas carry no model IDs; resolution happens at dispatch
    mcp_tools: list[str] = field(default_factory=list)
    spend_budget_usd: float = 0.0
    memory_scopes: list[str] = field(default_factory=lambda: ["default"])
    scopes: list[str] = field(default_factory=list)
    created_by: str | None = None
    is_system_agent: bool = False
    created_at: str = field(default_factory=lambda: datetime.now(timezone.utc).isoformat())
    updated_at: str = field(default_factory=lambda: datetime.now(timezone.utc).isoformat())

    def to_dict(self) -> dict[str, Any]:
        return asdict(self)

    @classmethod
    def from_dict(cls, data: dict[str, Any]) -> UserDefinedAgentManifest:
        return cls(
            id=data["id"],
            name=data["name"],
            role=data.get("role", "custom_theorist"),
            description=data.get("description", ""),
            system_prompt=data.get("system_prompt", ""),
            avatar_emoji=data.get("avatar_emoji", "🧪"),
            badge_color=data.get("badge_color", "cyan"),
            mode=data.get("mode", "prompt"),
            steps=data.get("steps", []),
            capabilities=data.get("capabilities", []),
            modalities=data.get("modalities", ["text"]),
            preferred_model=data.get("preferred_model", ""),
            mcp_tools=data.get("mcp_tools", []),
            spend_budget_usd=float(data.get("spend_budget_usd", 0.0)),
            memory_scopes=data.get("memory_scopes", ["default"]),
            scopes=data.get("scopes", []),
            created_by=data.get("created_by"),
            is_system_agent=bool(data.get("is_system_agent", False)),
            created_at=data.get("created_at", datetime.now(timezone.utc).isoformat()),
            updated_at=data.get("updated_at", datetime.now(timezone.utc).isoformat()),
        )


STANDARD_SYSTEM_AGENTS: list[UserDefinedAgentManifest] = [
    UserDefinedAgentManifest(
        id="sys_lead_theorist",
        name="Lead Theorist",
        role="lead_theorist",
        avatar_emoji="🧠",
        badge_color="indigo",
        description="High-level scientific conjecture formation, literature synthesis, and multi-hour hypothesis planning.",
        system_prompt="You are the Lead Theorist of Kubona. Synthesize scientific literature, formulate falsifiable conjectures, and structure research campaigns.",
        capabilities=["hypothesis_formulation", "literature_synthesis", "manuscript_drafting"],
        modalities=["text", "image", "molecule_3d"],
        preferred_model="",
        is_system_agent=True,
    ),
    UserDefinedAgentManifest(
        id="sys_lean4_proof_engineer",
        name="Lean 4 Proof Engineer",
        role="lean4_proof_engineer",
        avatar_emoji="📐",
        badge_color="emerald",
        description="Formal theorem formalization, tactic cavity filling, and zero-cheat Lean 4 verification via comparator.",
        system_prompt="You are the Lean 4 Proof Engineer. Convert informal mathematical lemmas into machine-checked Lean 4 code. Obey zero-cheat axiom rules.",
        capabilities=["lean4_formalization", "tactic_cavity_filling", "comparator_verification"],
        modalities=["text", "lean4_ast"],
        preferred_model="",
        is_system_agent=True,
    ),
    UserDefinedAgentManifest(
        id="sys_classical_anchor_solver",
        name="Classical Anchor Solver",
        role="classical_anchor_solver",
        avatar_emoji="⚡",
        badge_color="amber",
        description="Exact classical calculations (PySCF FCI, DMRG, CC) providing ground truth anchors to prevent quantum-washing.",
        system_prompt="You are the Classical Anchor Solver. Compute exact numerical ground-state energies and confirm whether classical approximations fail.",
        capabilities=["pyscf_fci", "dmrg_calculation", "kohn_sham_triage"],
        modalities=["text", "molecule_3d"],
        preferred_model="",
        is_system_agent=True,
    ),
    UserDefinedAgentManifest(
        id="sys_quantum_circuit_compiler",
        name="Quantum Circuit Compiler",
        role="quantum_circuit_compiler",
        avatar_emoji="⚛️",
        badge_color="cyan",
        description="Ansatz design, Clifford+T synthesis, wire cutting, and zero-noise extrapolation (ZNE) mitigation.",
        system_prompt="You are the Quantum Circuit Compiler. Generate hardware-efficient circuits, check barren plateau risks, and compile QIR bitcode.",
        capabilities=["qasm_generation", "qir_compilation", "wire_cutting", "zne_mitigation"],
        modalities=["text", "circuit_qir"],
        preferred_model="",
        is_system_agent=True,
    ),
    UserDefinedAgentManifest(
        id="sys_adversarial_critic",
        name="Adversarial Falsification Critic",
        role="adversarial_falsification_critic",
        avatar_emoji="🛡️",
        badge_color="rose",
        description="Red-teams all hypotheses, hunts for counterexamples, checks noise vulnerabilities, and tests edge cases.",
        system_prompt="You are the Adversarial Falsification Critic. Relentlessly challenge assumptions, find counterexamples, and probe physical unviability.",
        capabilities=["counterexample_search", "noise_stress_testing", "disconfirmation"],
        modalities=["text", "image", "molecule_3d", "circuit_qir"],
        preferred_model="",
        is_system_agent=True,
    ),
    UserDefinedAgentManifest(
        id="sys_sentinel_spend_governor",
        name="Sentinel Spend Governor",
        role="sentinel_spend_governor",
        avatar_emoji="⚖️",
        badge_color="purple",
        description="Monitors QPU hardware shots, enforces AP2 cryptographic spend caps, and maintains the sovereign air-gap.",
        system_prompt="You are the Sentinel Spend Governor. Ensure that no cloud or QPU call violates cryptographic limits or the sovereign air-gap policy.",
        capabilities=["cost_accounting", "air_gap_enforcement", "spend_card_verification"],
        modalities=["text"],
        preferred_model="",
        is_system_agent=True,
    ),
    UserDefinedAgentManifest(
        id="sys_multimodal_spectroscopist",
        name="Multimodal Spectroscopist",
        role="multimodal_spectroscopist",
        avatar_emoji="🔬",
        badge_color="violet",
        description="Interprets NMR/UV-Vis spectra, XRD diffraction patterns, band diagrams, and 3D molecular conformations.",
        system_prompt="You are the Multimodal Spectroscopist. Inspect 2D/3D spectroscopic curves, crystal structures, and electron density plots.",
        capabilities=["spectroscopy_interpretation", "cif_pdb_analysis", "vision_inspection"],
        modalities=["text", "image", "molecule_3d", "spectroscopy"],
        preferred_model="",
        is_system_agent=True,
    ),
]


class CustomAgentRegistry:
    """Manages system and user-defined agents with validation and persistence.

    Backs either SQLite (kubona.db `custom_agents` table) or JSON file.
    """

    def __init__(self, storage_path: Path | str | None = None, db_path: Path | str | None = None):
        self.storage_path = Path(storage_path) if storage_path else None
        self.db_path = Path(db_path) if db_path else None
        if self.storage_path and str(self.storage_path).endswith((".db", ".sqlite")):
            self.db_path = self.storage_path
            self.storage_path = None
        self._custom_agents: dict[str, UserDefinedAgentManifest] = {}
        self._system_agents: dict[str, UserDefinedAgentManifest] = {
            a.id: a for a in STANDARD_SYSTEM_AGENTS
        }
        self._load()

    def _load(self) -> None:
        if self.db_path and self.db_path.exists():
            import sqlite3
            try:
                conn = sqlite3.connect(f"file:{self.db_path}?mode=ro", uri=True)
                rows = conn.execute("SELECT id, manifest_json FROM custom_agents").fetchall()
                conn.close()
                for aid, raw in rows:
                    try:
                        d = json.loads(raw)
                        manifest = UserDefinedAgentManifest.from_dict(d)
                        manifest.id = aid
                        manifest.is_system_agent = False
                        self._custom_agents[aid] = manifest
                    except Exception:
                        continue
            except Exception:
                pass
            return

        if self.storage_path and self.storage_path.exists():
            try:
                data = json.loads(self.storage_path.read_text(encoding="utf-8"))
                for item in data.get("custom_agents", []):
                    manifest = UserDefinedAgentManifest.from_dict(item)
                    self._custom_agents[manifest.id] = manifest
            except Exception:
                pass

    def _save(self) -> None:
        if self.db_path:
            import sqlite3
            self.db_path.parent.mkdir(parents=True, exist_ok=True)
            conn = sqlite3.connect(str(self.db_path), timeout=5.0)
            conn.execute("""
                CREATE TABLE IF NOT EXISTS custom_agents (
                    id TEXT PRIMARY KEY,
                    manifest_json TEXT NOT NULL DEFAULT '{}',
                    updated_at DATETIME NOT NULL
                )
            """)
            for aid, manifest in self._custom_agents.items():
                raw = json.dumps(manifest.to_dict())
                conn.execute("""
                    INSERT INTO custom_agents (id, manifest_json, updated_at) VALUES (?, ?, CURRENT_TIMESTAMP)
                    ON CONFLICT(id) DO UPDATE SET manifest_json = excluded.manifest_json, updated_at = CURRENT_TIMESTAMP
                """, (aid, raw))
            conn.commit()
            conn.close()
            return

        if self.storage_path:
            self.storage_path.parent.mkdir(parents=True, exist_ok=True)
            payload = {
                "custom_agents": [a.to_dict() for a in self._custom_agents.values()]
            }
            self.storage_path.write_text(json.dumps(payload, indent=2), encoding="utf-8")

    def list_all(self, include_system: bool = True) -> list[UserDefinedAgentManifest]:
        agents: list[UserDefinedAgentManifest] = []
        if include_system:
            agents.extend(self._system_agents.values())
        agents.extend(self._custom_agents.values())
        return agents

    def get(self, agent_id: str) -> UserDefinedAgentManifest | None:
        if agent_id in self._system_agents:
            return self._system_agents[agent_id]
        return self._custom_agents.get(agent_id)

    def register_custom_agent(self, manifest: UserDefinedAgentManifest) -> UserDefinedAgentManifest:
        """Validates and registers a new user-defined agent."""
        if not manifest.id or not manifest.id.strip():
            raise ValueError("Agent ID cannot be empty.")
        if manifest.id in self._system_agents:
            raise ValueError(f"Cannot overwrite reserved system agent ID '{manifest.id}'.")
        if not manifest.name or not manifest.name.strip():
            raise ValueError("Agent name cannot be empty.")
        if not manifest.system_prompt or not manifest.system_prompt.strip():
            raise ValueError("Agent system prompt cannot be empty.")
        if manifest.mode not in ("prompt", "pipeline"):
            raise ValueError(f"mode must be 'prompt' or 'pipeline', got {manifest.mode!r}")
        if manifest.mode == "pipeline":
            if not manifest.steps:
                raise ValueError("pipeline mode requires at least one step")
            for i, step in enumerate(manifest.steps):
                if not isinstance(step, dict) or not str(step.get("id", "")).strip() or not str(step.get("tool", "")).strip():
                    raise ValueError(f"step {i} requires non-empty 'id' and 'tool'")
        for sc in manifest.scopes:
            if not isinstance(sc, str) or not sc.strip():
                raise ValueError("scope cannot be empty string")

        manifest.is_system_agent = False
        manifest.updated_at = datetime.now(timezone.utc).isoformat()
        self._custom_agents[manifest.id] = manifest
        self._save()
        return manifest

    def delete_custom_agent(self, agent_id: str) -> bool:
        if agent_id in self._system_agents:
            raise ValueError("Cannot delete system agent.")
        if agent_id in self._custom_agents:
            del self._custom_agents[agent_id]
            if self.db_path and self.db_path.exists():
                import sqlite3
                conn = sqlite3.connect(str(self.db_path), timeout=5.0)
                conn.execute("DELETE FROM custom_agents WHERE id = ?", (agent_id,))
                conn.commit()
                conn.close()
            else:
                self._save()
            return True
        return False
