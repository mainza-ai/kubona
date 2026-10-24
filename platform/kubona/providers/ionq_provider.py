"""IonQ provider adapter (planned, not live-verified).

Credential plumbing is real (KUBONA_IONQ_API_KEY, Bearer, fresh per call
like the other QPU plugins so rotation stays live). Execution is NOT:
submit() refuses until a live run against IonQ Cloud has been recorded
and LIVE_VERIFIED flipped — with the run ID in the flip commit. Writing
an untested SDK call would be fiction; the refusal names exactly what is
missing. See aws-braket.json notes for the broker contrast.
"""

from __future__ import annotations

from typing import Any

from kubona.circuits import Circuit
from kubona.schema import ProviderCapability

from .base import Provider, ProviderError

# Flip to True ONLY with a recorded live IonQ Cloud run (op ID + counts
# in the flip commit, user keys — never in CI). Until then every submit
# refuses, no matter the credential state.
LIVE_VERIFIED = False


class IonQProvider(Provider):
    def __init__(self, capability: ProviderCapability, secrets=None):
        self.capability = capability
        self._secrets = secrets

    def _credential(self) -> str | None:
        if self._secrets is not None:
            return self._secrets.get("ionq", "api_key")
        from kubona.secrets import SecretStore
        return SecretStore.from_env().get("ionq", "api_key")

    def health(self) -> dict[str, Any]:
        return {
            "provider": self.capability.provider,
            "wiring": "not-live-verified",
            "hardware_ready": False,
            "credential_present": self._credential() is not None,
        }

    def submit(self, circuit: Circuit, shots: int, seed: int | None = None,
               on_hardware_job=None) -> dict[str, Any]:
        _ = (circuit, shots, seed)
        if not LIVE_VERIFIED:
            raise ProviderError(
                "ionq execution is not live-verified: no run against IonQ "
                "Cloud has ever completed from this tree, so dispatch is "
                "refused even with credentials present. Credential state: "
                f"{'present' if self._credential() else 'absent'} "
                "(KUBONA_IONQ_API_KEY). Flip LIVE_VERIFIED only with a "
                "recorded live run."
            )
        raise ProviderError("ionq LIVE_VERIFIED set without an implementation: refusing")
