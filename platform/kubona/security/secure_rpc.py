"""ML-KEM-secured RPC envelope — wires the hybrid transport onto a call path.

A client encrypts {server, tool, token, args} with the HybridSession; the server
decrypts, runs the gateway call, and encrypts the response. Demonstrates the
harvest-now-decrypt-later mitigations on an actual governed call path (a stand-in
until the gateway HTTP/TLS termination carries the same suite on the wire).

Envelope discipline (W0.14) — every item refuses, never degrades:
  * nonce (16 random bytes, hex): single-use per server instance. A captured
    envelope replayed re-executes spend — replays refuse.
  * exp (unix seconds, default 300s TTL): stale envelopes refuse.
  * directional AAD: requests seal under -req/v1, responses under -resp/v1.
    Reflecting a response back as a request refuses (shared key + shared AAD
    used to make reflection structurally possible).
  * inner token required: anonymous envelopes refuse before dispatch.
  * identity match (server side, when the transport principal is known):
    inner token sub must equal the outer bearer sub — the confused deputy
    (audit attributes spend to inner while entry was granted to outer) refuses.
  * uniform failure shape: every post-decrypt failure returns the same
    encrypted {"ok": False, "error"} envelope. Undecryptable envelopes raise
    TransportError to the transport (HTTP 400 there) — AES-GCM fails closed
    on forgery, so the status distinction is not a decrypt oracle, and
    operators keep a truthful transport error.
"""

from __future__ import annotations

import json
import os
import time
from typing import Any

from kubona.mcp.auth import AuthError
from kubona.mcp.gateway import KubonaGateway
from kubona.security.hybrid_transport import HybridSession, TransportError

AAD_REQ = b"kubona-secure-rpc-req/v1"
AAD_RESP = b"kubona-secure-rpc-resp/v1"

ENVELOPE_TTL_S = 300
MAX_SEEN_NONCES = 10000


class SecureRpc:
    """Client<->server secure call envelope over a hybrid (X25519+ML-KEM) session."""

    def __init__(self, session: HybridSession, gateway: KubonaGateway | None = None):
        self.session = session
        self.gateway = gateway

    @property
    def _seen(self) -> dict[str, float]:
        # Replay state lives on the session (shared across wrapper
        # instances and requests); the wrapper is stateless.
        return self.session.envelope_nonces

    # -- client side -------------------------------------------------------
    def wrap_call(self, server: str, tool: str, args: dict[str, Any],
                  token: str | None = None, ttl_s: int = ENVELOPE_TTL_S) -> bytes:
        if not token or not token.strip():
            raise ValueError("secure envelopes require a bearer token: anonymous calls refuse")
        if ttl_s <= 0:
            raise ValueError("envelope TTL must be positive")
        plain = json.dumps({"server": server, "tool": tool, "args": args,
                            "token": token, "nonce": os.urandom(16).hex(),
                            "exp": int(time.time()) + ttl_s}).encode()
        return self.session.encrypt(plain, aad=AAD_REQ)

    def unwrap_response(self, ciphertext: bytes) -> dict[str, Any]:
        return json.loads(self.session.decrypt(ciphertext, aad=AAD_RESP))

    # -- server side -------------------------------------------------------
    def unwrap_call(self, ciphertext: bytes) -> dict[str, Any]:
        call = json.loads(self.session.decrypt(ciphertext, aad=AAD_REQ))
        nonce = call.get("nonce")
        exp = call.get("exp")
        if not isinstance(nonce, str) or not nonce:
            raise TransportError("envelope without nonce: replay protection requires one")
        if not isinstance(exp, (int, float)):
            raise TransportError("envelope without expiry: refusing")
        now = time.time()
        if now > float(exp):
            raise TransportError("stale envelope: refusing")
        # Sweep expired nonces first so the registry cannot fill with dead
        # entries; if still full, refuse rather than evict live nonces
        # (eviction would resurrect a spent nonce).
        for n, e in [item for item in self._seen.items() if item[1] <= now]:
            del self._seen[n]
        if nonce in self._seen:
            raise TransportError("envelope nonce already spent: replay refused")
        if len(self._seen) >= MAX_SEEN_NONCES:
            raise TransportError("replay registry full: refusing (never evict live nonces)")
        self._seen[nonce] = float(exp)
        return call

    def wrap_response(self, result: dict[str, Any]) -> bytes:
        return self.session.encrypt(json.dumps(result).encode(), aad=AAD_RESP)

    def handle(self, ciphertext: bytes, expected_sub: str | None = None) -> bytes:
        """Full server path: unwrap -> identity-match -> gateway call -> wrap.

        TransportError (undecryptable/forged/stale/replayed envelope)
        propagates to the transport for an honest transport error. Every
        post-decrypt failure returns the same encrypted error shape.
        """
        if self.gateway is None:
            raise ValueError("a gateway is required on the server side")
        call = self.unwrap_call(ciphertext)  # may raise TransportError
        try:
            inner = call.get("token")
            if not inner or not str(inner).strip():
                raise AuthError("envelope carries no bearer token: refusing")
            if expected_sub is not None:
                try:
                    inner_sub = self.gateway.auth.verify(str(inner)).sub
                except AuthError as e:
                    raise AuthError(f"envelope identity unverifiable: {e}") from None
                if inner_sub != expected_sub:
                    raise AuthError(
                        "envelope identity does not match transport identity: "
                        "confused deputy refused"
                    )
            result = self.gateway.call(call["server"], call["tool"], call["args"],
                                       token=str(inner))
        except Exception as e:  # noqa: BLE001 — uniform post-decrypt failure shape
            result = {"ok": False, "error": str(e)}
        return self.wrap_response(result)
