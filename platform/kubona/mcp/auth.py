"""Gateway authentication — OAuth 2.1-shaped interfaces, Phase 0 stubs.

Phase 0 delivers the boundary: a verify() contract, a dev implementation for
local/CI use, and an OAuth 2.1 verifier stub (token introspection against an IdP
lands with Phase 3 hardening per the roadmap).
"""

from __future__ import annotations

from abc import ABC, abstractmethod
from dataclasses import dataclass
from typing import Any


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
    """Local/CI auth: 'dev-token' authenticates as the default operator."""

    def __init__(self, sub: str = "researcher@kubona.local", role: str = "operator"):
        self._sub = sub
        self._role = role

    def verify(self, token: str) -> Principal:
        if token != "dev-token":
            raise AuthError("invalid or missing token (dev auth: use 'dev-token')")
        return Principal(sub=self._sub, role=self._role)


class OAuth21VerifierStub(AuthProvider):
    """Placeholder for real OAuth 2.1 + PQC-hybrid transport (Phase 3).

    Records the intended contract: introspection of short-lived, scoped tokens
    with per-call identity-aware authorization (\u00a74.7).
    """

    def __init__(self, issuer: str = "https://idp.kubona.local"):
        self.issuer = issuer

    def verify(self, token: str) -> Principal:
        raise NotImplementedError(
            "OAuth 2.1 verification is Phase 3 scope (ML-KEM hybrid transport, "
            "short-lived scoped tokens). Phase 0 uses DevAuthProvider behind the same interface."
        )
