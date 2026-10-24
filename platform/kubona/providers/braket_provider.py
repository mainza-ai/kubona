"""AWS Braket provider adapter (planned, not integrated).

Braket auth is AWS SigV4 + device ARNs — not a single token — so there is
deliberately NO credential plumbing here. Adding a KUBONA_AWS_BRAKET_*
variable that nothing signs with would be a false affordance. When the
SigV4 + device-ARN binding lands, credentials come from the standard AWS
chain (env/role), read fresh per call like every other plugin.
Until then submit() refuses with the exact missing piece.
"""

from __future__ import annotations

from typing import Any

from kubona.circuits import Circuit
from kubona.schema import ProviderCapability

from .base import Provider, ProviderError


class BraketProvider(Provider):
    def __init__(self, capability: ProviderCapability, secrets=None):
        self.capability = capability
        self._secrets = secrets

    def health(self) -> dict[str, Any]:
        return {
            "provider": self.capability.provider,
            "wiring": "not-integrated",
            "hardware_ready": False,
        }

    def submit(self, circuit: Circuit, shots: int, seed: int | None = None,
               on_hardware_job=None) -> dict[str, Any]:
        _ = (circuit, shots, seed)
        raise ProviderError(
            "aws-braket execution is not integrated: Braket needs AWS SigV4 "
            "signing plus a device ARN binding (IonQ/Rigetti/IQM/OQC targets), "
            "none of which exists in this tree. No credential variable is "
            "defined on purpose — a token-shaped env var would imply a submit "
            "path that does not exist."
        )
