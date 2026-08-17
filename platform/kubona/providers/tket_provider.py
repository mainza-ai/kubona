"""Quantinuum provider adapter (Phase 2) — optional dependency: pytket.

Onboarding data lives in schema/providers/quantinuum.json; the plugin adds
the execution path. Without credentials/SDK it fails with actionable guidance
instead of crashing the chain (the provider registry stays honest).
"""

from __future__ import annotations

from typing import Any

from kubona.circuits import Circuit, to_qasm
from kubona.schema import ProviderCapability

from .base import Provider, ProviderError


class TketProvider(Provider):
    def __init__(self, capability: ProviderCapability):
        self.capability = capability
        try:
            import pytket  # noqa: F401
            self._ready = True
            self._err = None
        except ImportError as e:
            self._ready, self._err = False, str(e)

    def submit(self, circuit: Circuit, shots: int, seed: int | None = None) -> dict[str, Any]:
        if not self._ready:
            raise ProviderError(
                "quantinuum provider requires optional deps + credentials: "
                f"pip install 'kubona-platform[qiskit]' and pytket (missing: {self._err}). "
                "Phase 2 registers the capability; hardware submission ships with credentials."
            )
        raise ProviderError("quantinuum execution not yet wired (needs API credentials + pytket wiring)")

    def health(self) -> dict[str, Any]:
        return {"provider": self.capability.provider, "ready": self._ready, "error": self._err}
