"""Rigetti provider adapter (Phase 2) — optional dependency: pyquil."""

from __future__ import annotations

from typing import Any

from kubona.circuits import Circuit
from kubona.schema import ProviderCapability

from .base import Provider, ProviderError


class PyQuilProvider(Provider):
    def __init__(self, capability: ProviderCapability):
        self.capability = capability
        try:
            import pyquil  # noqa: F401
            self._ready, self._err = True, None
        except ImportError as e:
            self._ready, self._err = False, str(e)

    def submit(self, circuit: Circuit, shots: int, seed: int | None = None) -> dict[str, Any]:
        if not self._ready:
            raise ProviderError(
                "rigetti provider requires optional deps + credentials: pip install pyquil "
                f"(missing: {self._err}). Capability registered in Phase 2."
            )
        raise ProviderError("rigetti execution not yet wired (needs API credentials + pyquil wiring)")

    def health(self) -> dict[str, Any]:
        return {"provider": self.capability.provider, "ready": self._ready, "error": self._err}
