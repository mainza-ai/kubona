"""Provider capability schema — the data-driven provider contract.

Mirrors the plan's \u00a74.4 example (quantinuum) and the wiki page
provider-capability-schema: onboarding a provider is a data change, not a code change.
"""

from __future__ import annotations

import json
from enum import Enum
from pathlib import Path
from typing import Any, Literal

from pydantic import BaseModel, Field

SCHEMA_DIR = Path(__file__).parent / "providers"


class Modality(str, Enum):
    GATE_MODEL = "gate_model"
    TRAPPED_ION = "trapped_ion"
    SUPERCONDUCTING = "superconducting"
    NEUTRAL_ATOM = "neutral_atom"
    ANNEALING = "annealing"
    PHOTONIC = "photonic"
    STATEVECTOR_SIMULATOR = "statevector_simulator"
    GPU_SIMULATOR = "gpu_simulator"


class PricingModel(BaseModel):
    """Pricing model per the architecture-paper's provider rows."""

    model: Literal["per_shot", "per_task_plus_per_shot", "per_second", "token_consumption", "free"] = "free"
    currency: str = "USD"
    per_shot_usd: float = 0.0
    per_task_usd: float = 0.0
    per_second_usd: float = 0.0
    min_shots_for_mitigation: int | None = None
    notes: str | None = None


class ProviderAttestation(BaseModel):
    """Measured-vs-declared calibration attestation (Q-14).

    `measured_qubits` comes from the provider plugin's health report (a live
    backend property or a hard-coded capacity bound), never from the JSON.
    `qubit_count_ok` is True/False when measured, None when the plugin cannot
    report (labeled unknown — never a pass). `snapshot_hash` seals the
    attestation for ledger admission checks.
    """

    checked_at: str = ""
    declared_qubits: int = 0
    measured_qubits: int | None = None
    qubit_count_ok: bool | None = None
    canary_ok: bool = False
    snapshot_hash: str = ""
    ledger_ref: str | None = None


class ProviderCapability(BaseModel):
    """One backend's machine-readable capability contract (\u00a74.4 shape)."""

    provider: str
    modality: Modality
    physical_qubits: int = 0
    backend: str | None = None   # target hardware backend name (IBM Runtime, etc.)
    logical_qubits_available: int | None = None
    native_gate_set: list[str] = Field(default_factory=list)
    queue_depth_estimate_s: int = 0
    pricing: PricingModel = Field(default_factory=PricingModel)
    jurisdiction: str = "none"
    export_control_flag: str = "none"
    sdk_bindings: list[str] = Field(default_factory=list)
    status: Literal["GA", "beta", "registered", "planned"] = "registered"
    execution_plugin: str | None = Field(
        default=None,
        description="Python dotted path to the provider's execution plugin (provider trait impl).",
    )
    last_attestation: ProviderAttestation | None = Field(
        default=None,
        description="Latest measured-vs-declared attestation (Q-14). Pricing and queue "
                    "depth remain declared-until-metered; only qubit capacity is attested.",
    )

    def cost_estimate(self, shots: int) -> dict[str, Any]:
        """Estimate cost for a shot count using this capability's pricing model."""
        p = self.pricing
        if p.model == "per_shot":
            total = p.per_shot_usd * shots
        elif p.model == "per_task_plus_per_shot":
            total = p.per_task_usd + p.per_shot_usd * shots
        else:  # per_second / token_consumption / free -> no static shot estimate here
            total = 0.0
        return {
            "provider": self.provider,
            "pricing_model": p.model,
            "currency": p.currency,
            "estimated_usd": round(total, 6),
            "shots": shots,
            "notes": p.notes,
        }


def load_provider(name: str) -> ProviderCapability:
    path = SCHEMA_DIR / f"{name}.json"
    if not path.exists():
        raise KeyError(f"no capability schema for provider {name!r} in {SCHEMA_DIR}")
    return ProviderCapability.model_validate(json.loads(path.read_text()))


def load_all_providers() -> dict[str, ProviderCapability]:
    out = {}
    for path in sorted(SCHEMA_DIR.glob("*.json")):
        cap = ProviderCapability.model_validate(json.loads(path.read_text()))
        out[cap.provider] = cap
    return out
