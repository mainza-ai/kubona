"""Gateway authentication boundary.

The `AuthProvider` contract + a dev implementation for local/CI use. The real
OAuth 2.1-compatible issuer lives in `kubona.security.oauth`
(`OAuth2TokenIssuer` / `OAuth2AccessTokenProvider`) and is what production
`kubona run`/`serve-gateway` wires by default.
"""

from __future__ import annotations

from abc import ABC, abstractmethod
from dataclasses import dataclass


class AuthError(Exception):
    pass


@dataclass(frozen=True)
class Principal:
    sub: str
    role: str = "operator"
    scopes: tuple[str, ...] = ("*",)


class AuthProvider(ABC):
    @abstractmethod
    def verify(self, token: str) -> Principal:
        """Verify a bearer token -> identity. Raises AuthError on failure."""


class DevAuthProvider(AuthProvider):
    """Local/CI auth: 'dev-token' authenticates as the default operator.

    Only ever used outside KUBONA_ENV=production; production wires the real
    OAuth2 issuer (see kubona.security.oauth + kubona.run.build_auth).
    """

    def __init__(self, sub: str = "researcher@kubona.local", role: str = "operator"):
        self._sub = sub
        self._role = role

    def verify(self, token: str) -> Principal:
        if token != "dev-token":
            raise AuthError("invalid or missing token (dev auth: use 'dev-token')")
        return Principal(sub=self._sub, role=self._role)
