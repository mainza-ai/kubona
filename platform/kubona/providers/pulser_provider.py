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

    def submit(self, circuit: Circuit, shots: int, seed: int | None = None) -> dict[str, Any]:
        if not self._ready:
            raise ProviderError(
                "neutral-atom provider requires optional deps + credentials: pip install pulser "
                f"(missing: {self._err}). AHS is a distinct modality; circuit IR must be "
                "converted to pulse sequences (Phase 2 note)."
            )
        raise ProviderError("neutral-atom execution not yet wired (needs API credentials + pulser wiring)")

    def health(self) -> dict[str, Any]:
        return {"provider": self.capability.provider, "ready": self._ready, "error": self._err}
