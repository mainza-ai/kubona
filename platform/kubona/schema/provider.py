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


# Time-model constants for per-second QPU billing (v1, documented assumptions —
# NOT calibration data; the estimate is conservative by construction and the
# cost_basis string always states the model revision so approval packets show
# estimate provenance, never a bare number).
_QPU_1Q_GATE_S = 50e-9       # single-qubit gate time (superconducting, conservative)
_QPU_2Q_GATE_S = 500e-9      # two-qubit gate time (ECR-class, conservative)
_QPU_READOUT_S = 2e-6        # measurement + readout per shot
_QPU_REP_DELAY_S = 1e-6      # repetition delay (IBM Nighthawk default; Heron similar)
_QPU_SESSION_OVERHEAD_S = 5.0  # conservatism buffer for job setup (queue wait is NOT billed)
_QPU_MINIMUM_BILL_S = 1.0    # IBM PAYG 1-second minimum purchase
_TWO_Q_GATES = frozenset({"cx", "cz", "swap", "cp", "ecr", "rzz"})


def estimate_qpu_seconds(circuit: Any, shots: int,
                         rep_delay_s: float = _QPU_REP_DELAY_S,
                         session_overhead_s: float = _QPU_SESSION_OVERHEAD_S) -> dict[str, Any]:
    """Conservative QPU-time estimate for per-second billing.

    per_shot = 1q-depth * 50ns + 2q-depth * 500ns + readout 2us + rep-delay;
    total = shots * per_shot + session overhead; billed = max(total, 1s).
    Depth is serial gate count (Circuit.depth); overestimates parallel
    circuits, which is the safe direction for a spend ceiling.
    """
    if shots <= 0:
        raise ValueError("shots must be positive for a cost estimate")
    gates = list(getattr(circuit, "gates", []) or [])
    n_1q = sum(1 for g in gates if len(getattr(g, "qubits", ())) < 2)
    n_2q = sum(1 for g in gates if len(getattr(g, "qubits", ())) >= 2)
    per_shot = n_1q * _QPU_1Q_GATE_S + n_2q * _QPU_2Q_GATE_S + _QPU_READOUT_S + rep_delay_s
    raw_seconds = shots * per_shot + session_overhead_s
    billed_seconds = max(raw_seconds, _QPU_MINIMUM_BILL_S)
    return {
        "per_shot_s": per_shot,
        "raw_seconds": raw_seconds,
        "billed_seconds": billed_seconds,
        "depth_1q": n_1q,
        "depth_2q": n_2q,
        "minimum_applied": raw_seconds < _QPU_MINIMUM_BILL_S,
    }


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

    def cost_estimate(self, shots: int, circuit: Any = None) -> dict[str, Any]:
        """Estimate cost for a shot count using this capability's pricing model.

        per_second models REQUIRE the circuit (time modeling is impossible
        without gate counts) and fail closed otherwise — a per-second backend
        must never estimate $0.00 (that reads as "free tier" downstream).
        """
        p = self.pricing
        basis = p.notes or ""
        if p.model == "per_shot":
            total = p.per_shot_usd * shots
        elif p.model == "per_task_plus_per_shot":
            total = p.per_task_usd + p.per_shot_usd * shots
        elif p.model == "per_second":
            if circuit is None:
                raise ValueError(
                    f"provider {self.provider!r} bills per second: cost_estimate "
                    "requires the circuit for time modeling (refusing $0.00 default)")
            if p.per_second_usd <= 0:
                raise ValueError(
                    f"provider {self.provider!r} has no per-second rate configured: "
                    "refusing $0.00 default")
            timing = estimate_qpu_seconds(circuit, shots)
            total = p.per_second_usd * timing["billed_seconds"]
            basis = (basis + f" | time-model v1: {timing['billed_seconds']:.3f}s billed "
                             f"({timing['raw_seconds']:.4f}s raw, {timing['depth_1q']}x1q+"
                             f"{timing['depth_2q']}x2q, {shots} shots)").strip(" |")
            return {
                "provider": self.provider,
                "pricing_model": p.model,
                "currency": p.currency,
                "estimated_usd": round(total, 6),
                "shots": shots,
                "notes": p.notes,
                "cost_basis": basis,
                "timing": timing,
            }
        else:  # token_consumption / free -> no static shot estimate here
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
