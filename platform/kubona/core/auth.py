"""Protocol-neutral Principal, AuthError, and AuthProvider definitions (Q-28).

Decouples identity and authentication types from the MCP namespace.
"""

from __future__ import annotations

import hmac
from abc import ABC, abstractmethod
from dataclasses import dataclass

from kubona.config import is_production


class AuthError(Exception):
    """Raised when authentication fails."""
    pass


@dataclass(frozen=True)
class Principal:
    """Represents an authenticated principal (human, agent, or service).

    Least privilege by construction (W0.13): no role, no scopes unless
    explicitly granted. The old defaults (role="operator", scopes=("*",))
    made every `Principal(sub=...)` without kwargs god-mode — forgotten
    kwargs must never equal full authorization.

    Phase-2: `tenant` is a first-class field constructed at authentication
    time (issuer-signed claim validated strictly, dev provider default), not
    attached at decode. Consumers read `principal.tenant` directly;
    `kubona.security.tenant.principal_tenant` still re-validates fail-closed.
    """
    sub: str
    role: str = "user"
    scopes: tuple[str, ...] = ()
    tenant: str = "default"


class AuthProvider(ABC):
    """Abstract authentication provider contract."""

    @abstractmethod
    def verify(self, token: str) -> Principal:
        """Verify a bearer token -> identity. Raises AuthError on failure."""


class DevAuthProvider(AuthProvider):
    """Local/CI auth: 'dev-token' authenticates as the configured principal.

    Dev-only by construction (W0.13): verify() refuses outright in
    KUBONA_ENV=production (a wrong import in prod used to silently enable
    god-mode), and token comparison is constant-time. The default scopes
    are explicitly documented god-mode-for-dev — reachable only through
    this class, never through a bare Principal().
    """

    def __init__(self, sub: str = "researcher@kubona.local", role: str = "operator",
                  scopes: tuple[str, ...] = ("*",), token: str = "dev-token",
                  tenant: str = "default"):
        self._sub = sub
        self._role = role
        self._scopes = scopes
        self._token = token
        self._tenant = tenant

    def verify(self, token: str) -> Principal:
        if is_production():
            raise AuthError("dev auth is disabled in production: wire the OAuth2 issuer")
        if not hmac.compare_digest(token or "", self._token):
            raise AuthError("invalid or missing token (dev auth: use 'dev-token')")
        return Principal(sub=self._sub, role=self._role, scopes=self._scopes,
                         tenant=self._tenant)
