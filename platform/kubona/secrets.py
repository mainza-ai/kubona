"""Secret management — production credential posture.

No credentials live in code or in the capability JSON. A `SecretStore` reads from
the environment (or an injected backend) at call time, per provider and key, and
returns None when absent so providers fail closed with actionable guidance.
Production can swap the `Source` for a KMS/secrets-manager without touching the
providers.

Env convention:  KUBONA_{PROVIDER}_{KEY}   (provider normalized: A-Z -> uppercase, - -> _)
  e.g. KUBONA_IBM_QISKIT_TOKEN, KUBONA_QUANTINUUM_API_KEY, KUBONA_DWAVE_LEAP_TOKEN
"""

from __future__ import annotations

import os
import re
from abc import ABC, abstractmethod
from typing import Any

_PROVIDER_RE = re.compile(r"[^A-Z0-9]+")


def _env_key(provider: str, key: str) -> str:
    p = _PROVIDER_RE.sub("_", provider.upper()).strip("_")
    k = _PROVIDER_RE.sub("_", key.upper()).strip("_")
    return f"KUBONA_{p}_{k}"


class SecretSource(ABC):
    @abstractmethod
    def get(self, provider: str, key: str) -> str | None:
        ...


class EnvSecretSource(SecretSource):
    """Reads KUBONA_{PROVIDER}_{KEY} from the environment (default; KMS-ready)."""

    def get(self, provider: str, key: str) -> str | None:
        return os.environ.get(_env_key(provider, key))


class SecretStore:
    def __init__(self, source: SecretSource | None = None):
        self.source = source or EnvSecretSource()

    def get(self, provider: str, key: str, *, required: bool = False) -> str | None:
        v = self.source.get(provider, key)
        if required and not v:
            return None
        return v

    def require(self, provider: str, key: str) -> str:
        v = self.source.get(provider, key)
        if not v:
            raise KeyError(
                f"{provider} requires credential '{key}' — set {_env_key(provider, key)} "
                "(or provide a SecretStore/Source backend). Providers fail closed."
            )
        return v
