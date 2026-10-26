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
    def submit(self, circuit: Circuit, shots: int, seed: int | None = None,
               on_hardware_job=None, wait_for_result: bool = True) -> dict[str, Any]:
        """Run `circuit` and return results: {counts, ...}. Implementations must be
        deterministic under a given seed (reproducibility via the ledger).
        on_hardware_job, when given, is called with the vendor job id as
        soon as the hardware accepts the job (before results arrive) so the
        runner can record it for later recall. Simulators never call it.
        When `wait_for_result=False` on hardware modalities, returns a
        `{"status": "running", "deferred": True, "hardware_job_id": ...}`
        envelope immediately after submission without blocking on the cloud
        queue; status is then polled via `poll_hardware_job`."""

    def poll_hardware_job(self, hardware_job_id: str) -> dict[str, Any]:
        """Poll vendor status for a deferred hardware job (`wait_for_result=False`).

        Returns a dict with `status` in ("running", "completed", "failed", "cancelled")
        and `result` (when completed) or `error` (when failed). Base implementation
        refuses: only hardware providers with an async vendor job API override this.
        """
        _ = hardware_job_id
        raise ProviderError(
            f"provider {self.capability.provider!r} does not support vendor status polling"
        )

    def cancel(self, hardware_job_id: str) -> dict[str, Any]:
        """Attempt vendor-side recall of a submitted hardware job. The base
        implementation refuses: cooperative fence (JobRunner.cancel) is the
        only recall. Providers with a verified vendor cancel API override
        this. Recall never implies refund — see the refund proposal (R-32)."""
        _ = hardware_job_id
        raise ProviderError(
            f"provider {self.capability.provider!r} has no vendor recall: "
            "cooperative fence only (in-flight work runs to completion underneath)")

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
