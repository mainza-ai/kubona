"""QuEra/Pasqal neutral-atom provider adapter (Phase 2) — optional: pulser."""

from __future__ import annotations

from typing import Any

from kubona.circuits import Circuit
from kubona.schema import ProviderCapability

from .base import Provider, ProviderError


class PulserProvider(Provider):
    def __init__(self, capability: ProviderCapability):
        self.capability = capability
        try:
            import pulser  # noqa: F401
            self._ready, self._err = True, None
        except ImportError as e:
            self._ready, self._err = False, str(e)

    def submit(self, circuit: Circuit, shots: int, seed: int | None = None,
               on_hardware_job=None) -> dict[str, Any]:
        # W2.18: missing-SDK vs not-wired are SEPARATE refusals. The old
        # single message blamed "deps + credentials" together, so an operator
        # could install the SDK and still have no idea execution isn't wired.
        if not self._ready:
            raise ProviderError(
                "neutral-atom provider SDK missing: pip install pulser "
                f"(import failed: {self._err}). This is a setup gap, not a "
                "wiring gap — execution still needs credentials + pulse "
                "wiring after install."
            )
        raise ProviderError(
            "neutral-atom execution not yet wired: the SDK imports, but no "
            "pulse-sequence conversion or backend credentials are configured "
            "(AHS is a distinct modality; circuit IR must be converted to "
            "pulse sequences). Failing closed instead of simulating.")

    def health(self) -> dict[str, Any]:
        return {"provider": self.capability.provider, "ready": self._ready, "error": self._err,
                "wiring": "not-wired"}
