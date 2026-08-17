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
        if not self._ready:
            raise ProviderError(
                "xanadu-strawberry-fields requires optional deps + credentials: "
                f"pip install strawberryfields (missing: {self._err}). CV/photonic "
                "execution ships with credentials + a Strawberry Fields backend wiring."
            )
        raise ProviderError("strawberry-fields execution not yet wired (needs credentials + SF backend)")

    def health(self) -> dict[str, Any]:
        return {"provider": self.capability.provider, "ready": self._ready, "error": self._err}
