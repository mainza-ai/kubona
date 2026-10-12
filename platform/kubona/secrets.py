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

import httpx

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


class OpenBaoSecretSource(SecretSource):
    """OpenBao KV-v2 backend: secret/data/quantum/<provider>.

    AppRole credentials come from the environment (BAO_ROLE_ID,
    BAO_SECRET_ID; BAO_TOKEN dev override skips login). Reads use a cached
    login token; every failure mode is explicit:
      - transport/auth/permission errors RAISE (fail-closed): an outage
        must never masquerade as "no credential", which would silently
        downgrade hardware paths to simulators and defeat rotation.
      - 404 (no such secret) returns None: absent credentials keep the
        existing provider fail-closed path (Aer fallback with guidance).
    Revocation (revoke_lease) exists for dynamic mounts; static KV secrets
    carry no meaningful lease, so no submit path calls it spuriously.
    """

    def __init__(self, addr: str, role_id: str = "", secret_id: str = "",
                 token: str = "", timeout_s: float = 10.0):
        self._httpx = httpx
        self.addr = addr.strip().rstrip("/")
        if not role_id and not token:
            raise ValueError(
                "OpenBao source needs BAO_ROLE_ID + BAO_SECRET_ID (or BAO_TOKEN dev override)"
            )
        self.role_id = role_id
        self.secret_id = secret_id
        self.token = token
        self.timeout_s = timeout_s

    def _login(self) -> str:
        if self.token:
            return self.token
        try:
            resp = self._httpx.post(
                f"{self.addr}/v1/auth/approle/login",
                json={"role_id": self.role_id, "secret_id": self.secret_id},
                timeout=self.timeout_s,
            )
        except Exception as e:
            raise ConnectionError(f"OpenBao AppRole login unreachable at {self.addr}: {e}")
        if resp.status_code != 200:
            raise PermissionError(
                f"OpenBao AppRole login refused (HTTP {resp.status_code}): {resp.text[:200]}"
            )
        try:
            token = resp.json()["auth"]["client_token"]
        except Exception as e:
            raise ValueError(f"OpenBao login response unparseable: {e}")
        if not token:
            raise ValueError("OpenBao login returned no client token")
        self.token = token
        return token

    def get(self, provider: str, key: str) -> str | None:
        token = self._login()
        # Same normalization as _env_key (uppercase classes first), then
        # lowercase with dashes: "ibm_qiskit" and "ibm-qiskit" agree.
        slug = _PROVIDER_RE.sub("-", provider.upper()).strip("-").lower()
        try:
            resp = self._httpx.get(
                f"{self.addr}/v1/secret/data/quantum/{slug}",
                headers={"X-Vault-Token": token},
                timeout=self.timeout_s,
            )
        except Exception as e:
            raise ConnectionError(
                f"OpenBao read unreachable at {self.addr} (no silent env fallback): {e}"
            )
        if resp.status_code == 404:
            return None
        if resp.status_code == 403:
            raise PermissionError(
                f"OpenBao denied read of secret/quantum/{slug} (HTTP 403)"
            )
        if resp.status_code != 200:
            raise ConnectionError(
                f"OpenBao read failed (HTTP {resp.status_code}): {resp.text[:200]}"
            )
        try:
            data = resp.json()["data"]["data"]
        except Exception as e:
            raise ValueError(f"OpenBao KV response unparseable for {slug}: {e}")
        value = data.get(key)
        if value is None:
            return None
        return str(value)

    def revoke_lease(self, lease_id: str) -> None:
        """Revoke a dynamic-secret lease. For static KV secrets there is no
        meaningful lease — callers must not invoke this spuriously."""
        if not lease_id:
            raise ValueError("revoke_lease requires a lease id")
        token = self._login()
        try:
            resp = self._httpx.put(
                f"{self.addr}/v1/sys/leases/revoke",
                headers={"X-Vault-Token": token},
                json={"lease_id": lease_id},
                timeout=self.timeout_s,
            )
        except Exception as e:
            raise ConnectionError(f"OpenBao revoke unreachable at {self.addr}: {e}")
        if resp.status_code not in (200, 204):
            raise PermissionError(
                f"OpenBao revoke refused (HTTP {resp.status_code}): {resp.text[:200]}"
            )


class SecretStore:
    def __init__(self, source: SecretSource | None = None):
        self.source = source or EnvSecretSource()

    @classmethod
    def from_env(cls) -> "SecretStore":
        """Select the secret backend from the environment, read at CALL time
        (never import time) so tests and rotation stay honest:
          - BAO_ADDR set    -> OpenBao (authoritative; failures raise, no
                               env fallback — rotation/revocation must bite)
          - BAO_ADDR unset  -> environment (current behavior everywhere)
        """
        addr = os.environ.get("BAO_ADDR", "").strip()
        if not addr:
            return cls()
        return cls(OpenBaoSecretSource(
            addr=addr,
            role_id=os.environ.get("BAO_ROLE_ID", ""),
            secret_id=os.environ.get("BAO_SECRET_ID", ""),
            token=os.environ.get("BAO_TOKEN", ""),
        ))

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
