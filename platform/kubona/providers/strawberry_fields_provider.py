"""Xanadu / Strawberry Fields provider (Phase 5) — optional dependency: strawberryfields.

Photonic / continuous-variable is a distinct modality (not a gate-architecture
variant) — the capability schema carries `modality: photonic`. Plugin fails
closed without the SDK/credentials.
"""

from __future__ import annotations

from typing import Any

from kubona.circuits import Circuit
from kubona.schema import ProviderCapability

from .base import Provider, ProviderError


class StrawberryFieldsProvider(Provider):
    def __init__(self, capability: ProviderCapability):
        self.capability = capability
        try:
            import strawberryfields  # noqa: F401
            self._ready, self._err = True, None
        except ImportError as e:
            self._ready, self._err = False, str(e)

    def submit(self, circuit: Circuit, shots: int, seed: int | None = None) -> dict[str, Any]:
        # W2.18: missing-SDK vs not-wired are SEPARATE refusals (see
        # pulser_provider for the rationale).
        if not self._ready:
            raise ProviderError(
                "photonic provider SDK missing: pip install strawberryfields "
                f"(import failed: {self._err}). This is a setup gap, not a "
                "wiring gap — execution still needs credentials + a CV "
                "backend wiring after install."
            )
        raise ProviderError(
            "strawberry-fields execution not yet wired: the SDK imports, but "
            "no CV/photonic backend is configured. Failing closed instead "
            "of simulating a distinct modality.")

    def health(self) -> dict[str, Any]:
        return {"provider": self.capability.provider, "ready": self._ready, "error": self._err,
                "wiring": "not-wired"}
