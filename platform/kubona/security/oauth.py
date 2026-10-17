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


# Maximum token lifetime: a leaked bearer is live at most this long (W0.13).
# Callers requesting more get clamped, never honored.
MAX_TTL_S = 900

# Scope vocabulary guard (W0.13): scopes are lowercase alnum plus : . * - _.
# Blank or exotic scopes refuse at issuance — the verifier trusts them.
_SCOPE_RE = None  # compiled lazily to keep import light


def _scope_ok(scope: str) -> bool:
    import re
    global _SCOPE_RE
    if _SCOPE_RE is None:
        _SCOPE_RE = re.compile(r"^[a-z0-9:.*_-]+$")
    return isinstance(scope, str) and bool(_SCOPE_RE.match(scope))


def load_or_seed_signing_key() -> bytes:
    """Reuse a persisted Ed25519 private key (PEM) so issued tokens survive
    restarts (W0.13: single home for the key lifecycle — the issuer used to
    generate an ephemeral key per construction, forking auth on every
    restart; run.py's copy of this helper now delegates here)."""
    from kubona.config import signing_key_path

    pem = signing_key_path()  # state_path ensures parents
    if pem.exists():
        return pem.read_bytes()
    key = Ed25519PrivateKey.generate()
    data = key.private_bytes(
        encoding=serialization.Encoding.PEM,
        format=serialization.PrivateFormat.PKCS8,
        encryption_algorithm=serialization.NoEncryption(),
    )
    pem.write_bytes(data)
    return data


class OAuth2TokenIssuer:
    """Issues and verifies signed access tokens (sub, role, scopes, exp)."""

    def __init__(self, issuer: str = "https://idp.kubona.local", ttl_s: int = 300,
                 audience: str = "kubona-gateway",
                 signing_key: Ed25519PrivateKey | None = None):
        self.issuer = issuer
        self.audience = audience  # tokens are bound to this audience; verified on decode
        self.ttl_s = min(ttl_s, MAX_TTL_S)
        if signing_key is None:
            from cryptography.hazmat.primitives.serialization import load_pem_private_key
            signing_key = load_pem_private_key(load_or_seed_signing_key(), password=None)
            if not isinstance(signing_key, Ed25519PrivateKey):
                raise AuthError("persisted signing key is not Ed25519: refusing")
        self._key = signing_key


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

    def issue(self, sub: str, role: str = "user", scopes: tuple[str, ...] = (),
              ttl_s: int | None = None) -> str:
        # W0.13: no god-mode defaults. Every field validated: empty sub,
        # empty scopes, off-vocabulary scopes, and unbounded TTLs refuse.
        if not sub or not sub.strip():
            raise AuthError("token sub is required: refusing to mint anonymous bearers")
        if not scopes:
            raise AuthError("token scopes are required: default-deny, never ('*',)")
        clean = []
        for s in scopes:
            if not _scope_ok(s):
                raise AuthError(f"token scope {s!r} is off-vocabulary: refusing")
            clean.append(s)
        ttl = ttl_s if ttl_s is not None else self.ttl_s
        if ttl <= 0:
            raise AuthError("token TTL must be positive: refusing")
        now = int(time.time())
        header = {"alg": "EdDSA", "typ": "JWT"}
        payload = {"iss": self.issuer, "aud": self.audience, "sub": sub, "role": role,
                   "scopes": clean, "iat": now, "nbf": now,
                   "exp": now + min(ttl, MAX_TTL_S)}
        h = _b64url(json.dumps(header, separators=(",", ":")).encode())
        p = _b64url(json.dumps(payload, separators=(",", ":")).encode())
        signing_input = f"{h}.{p}".encode()
        sig = self._key.sign(signing_input)
        return f"{h}.{p}.{_b64url(sig)}"

    def decode(self, token: str) -> Principal:
        try:
            h, p, s = token.split(".")
            header = json.loads(_b64url_decode(h))
            payload = json.loads(_b64url_decode(p))
            key = self._key.public_key()
            key.verify(_b64url_decode(s), f"{h}.{p}".encode())
        except (ValueError, KeyError, json.JSONDecodeError, InvalidSignature) as e:
            raise AuthError("invalid or tampered token") from e
        # W0.13: the header is enforced, not decorative — alg confusion
        # (e.g. a re-signed 'none' header over a valid body) refuses.
        if header.get("alg") != "EdDSA" or header.get("typ") != "JWT":
            raise AuthError("unexpected token header: refusing")
        now = time.time()
        # 60s leeway on nbf only (clock skew): expiry is strict — a leeway
        # on exp would keep short-lived tokens valid past their TTL.
        if now < float(payload.get("nbf", 0)) - 60:
            raise AuthError("token not yet valid")
        if now > float(payload.get("exp", 0)):
            raise AuthError("token expired")
        if payload.get("iss") != self.issuer:
            raise AuthError("unexpected token issuer")
        if payload.get("aud") != self.audience:
            raise AuthError("unexpected token audience")
        sub = payload.get("sub")
        raw_scopes = payload.get("scopes", ())
        if not sub or not isinstance(raw_scopes, list):
            raise AuthError("token missing sub/scopes: refusing")
        return Principal(sub=sub, role=payload.get("role", "user"),
                         scopes=tuple(raw_scopes))

    def bearer(self, principal_name: str, role: str, scopes: tuple[str, ...],
               ttl_s: int | None = None) -> str:
        return self.issue(principal_name, role, scopes, ttl_s)


class OAuth2AccessTokenProvider(AuthProvider):
    """Gateway AuthProvider that verifies issuer-signed bearer tokens."""

    def __init__(self, issuer: OAuth2TokenIssuer | None = None):
        self.issuer = issuer or OAuth2TokenIssuer()

    def verify(self, token: str) -> Principal:
        return self.issuer.decode(token)
