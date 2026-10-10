"""The provider trait (plugin interface).

Mirrors the roadmap's Phase-0 requirement: an open-source provider abstraction
architected as a plugin trait from day one, so additional providers are pure
additions, not core rewrites.
"""

from __future__ import annotations

import importlib
from abc import ABC, abstractmethod
from typing import Any

from kubona.circuits import Circuit, canonical_hash
from kubona.schema import ProviderCapability, load_all_providers, load_provider


class ProviderError(RuntimeError):
    """Raised when a provider cannot execute a job."""


class Provider(ABC):
    """Execution plugin contract. One class per capability schema entry."""

    capability: ProviderCapability

    @abstractmethod
    def submit(self, circuit: Circuit, shots: int, seed: int | None = None) -> dict[str, Any]:
        """Run `circuit` and return results: {counts, ...}. Implementations must be
        deterministic under a given seed (reproducibility via the ledger)."""

    def health(self) -> dict[str, Any]:
        """Liveness report. MAY include `measured_qubits` (int) when the
        plugin can report a live capacity bound (backend properties or a
        hard-coded cap). The canary treats a missing value as unknown —
        never as a pass — and a mismatch against the declared capability
        as a failed attestation (Q-14)."""
        return {"provider": self.capability.provider, "status": "ok"}

    def calibration_snapshot(self) -> dict[str, Any] | None:
        """Measured calibration numbers, or None when unmeasurable.

        Returned dict uses CalibrationSnapshot field names (t1_us, t2_us,
        single_qubit_error, two_qubit_error, readout_error). The canary
        compares against the declared calibration (when the schema states
        one) within tolerance; mismatch fails attestation, absence reads as
        unknown. Base implementation: unmeasurable.
        """
        return None


def _instantiate(plugin_path: str, capability: ProviderCapability) -> Provider:
    mod_name, cls_name = plugin_path.rsplit(".", 1)
    mod = importlib.import_module(mod_name)
    cls = getattr(mod, cls_name)
    return cls(capability)


def get_provider(name: str) -> Provider:
    cap = load_provider(name)
    if not cap.execution_plugin:
        raise ProviderError(
            f"provider {name!r} is registered as capability data only (status={cap.status}); "
            "no execution plugin assigned yet"
        )
    return _instantiate(cap.execution_plugin, cap)


def list_providers() -> dict[str, ProviderCapability]:
    return load_all_providers()


def registry_health() -> dict[str, Any]:
    return {
        provider: {"status": cap.status, "modality": cap.modality.value, "plugin": cap.execution_plugin}
        for provider, cap in load_all_providers().items()
    }
