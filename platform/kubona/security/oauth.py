"""OAuth 2.1-shaped token issuer + gateway AuthProvider (closes the Phase-3 IdP gap).

A minimal, real issuer: signs short-lived, scoped access tokens (JWT-style,
Ed25519-signed) that the gateway verifies per call. Exposed as an AuthProvider
swappable with the dev provider — the token endpoint / PKCE flow is deployment
scope, but token issuance, scope, expiry, and tamper-rejection are real and tested.
"""

from __future__ import annotations

import base64
import json
import time
from typing import Any

from cryptography.exceptions import InvalidSignature
from cryptography.hazmat.primitives import hashes, serialization
from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PrivateKey, Ed25519PublicKey

from kubona.mcp.auth import AuthError, AuthProvider, Principal


def _b64url(data: bytes) -> str:
    return base64.urlsafe_b64encode(data).rstrip(b"=").decode()


def _b64url_decode(s: str) -> bytes:
    return base64.urlsafe_b64decode(s + "=" * (-len(s) % 4))


class OAuth2TokenIssuer:
    """Issues and verifies signed access tokens (sub, role, scopes, exp)."""

    def __init__(self, issuer: str = "https://idp.kubona.local", ttl_s: int = 300,
                 audience: str = "kubona-gateway",
                 signing_key: Ed25519PrivateKey | None = None):
        self.issuer = issuer
        self.audience = audience  # tokens are bound to this audience; verified on decode
        self.ttl_s = ttl_s
        self._key = signing_key or Ed25519PrivateKey.generate()

    def public_pem(self) -> bytes:
        return self._key.public_key().public_bytes(
            serialization.Encoding.PEM, serialization.PublicFormat.SubjectPublicKeyInfo)

    def issue(self, sub: str, role: str = "operator", scopes: tuple[str, ...] = ("*",),
              ttl_s: int | None = None) -> str:
        now = int(time.time())
        header = {"alg": "EdDSA", "typ": "JWT"}
        payload = {"iss": self.issuer, "aud": self.audience, "sub": sub, "role": role,
                   "scopes": list(scopes), "iat": now, "nbf": now,
                   "exp": now + (ttl_s or self.ttl_s)}
        h = _b64url(json.dumps(header, separators=(",", ":")).encode())
        p = _b64url(json.dumps(payload, separators=(",", ":")).encode())
        signing_input = f"{h}.{p}".encode()
        sig = self._key.sign(signing_input)
        return f"{h}.{p}.{_b64url(sig)}"

    def decode(self, token: str) -> Principal:
        try:
            h, p, s = token.split(".")
            payload = json.loads(_b64url_decode(p))
            key = self._key.public_key()
            key.verify(_b64url_decode(s), f"{h}.{p}".encode())
        except (ValueError, KeyError, json.JSONDecodeError, InvalidSignature) as e:
            raise AuthError("invalid or tampered token") from e
        now = time.time()
        if now < float(payload.get("nbf", 0)):
            raise AuthError("token not yet valid")
        if now > float(payload.get("exp", 0)):
            raise AuthError("token expired")
        if payload.get("iss") != self.issuer:
            raise AuthError("unexpected token issuer")
        if payload.get("aud") != self.audience:
            raise AuthError("unexpected token audience")
        return Principal(sub=payload["sub"], role=payload.get("role", "user"),
                         scopes=tuple(payload.get("scopes", ())))

    def bearer(self, principal_name: str, role: str, scopes: tuple[str, ...],
               ttl_s: int | None = None) -> str:
        return self.issue(principal_name, role, scopes, ttl_s)


class OAuth2AccessTokenProvider(AuthProvider):
    """Gateway AuthProvider that verifies issuer-signed bearer tokens."""

    def __init__(self, issuer: OAuth2TokenIssuer | None = None):
        self.issuer = issuer or OAuth2TokenIssuer()

    def verify(self, token: str) -> Principal:
        return self.issuer.decode(token)
