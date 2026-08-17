"""ML-KEM-secured RPC envelope — wires the hybrid transport onto a call path.

A client encrypts {server, tool, token, args} with the HybridSession; the server
decrypts, runs the gateway call, and encrypts the response. Demonstrates the
harvest-now-decrypt-later mitigations on an actual governed call path (a stand-in
until the gateway HTTP/TLS termination carries the same suite on the wire).
"""

from __future__ import annotations

from typing import Any

from kubona.mcp.auth import AuthError
from kubona.mcp.gateway import KubonaGateway
from kubona.security.hybrid_transport import HybridSession, TransportError


class SecureRpc:
    """Client<->server secure call envelope over a hybrid (X25519+ML-KEM) session."""

    def __init__(self, session: HybridSession, gateway: KubonaGateway | None = None):
        self.session = session
        self.gateway = gateway

    # -- client side -------------------------------------------------------
    def wrap_call(self, server: str, tool: str, args: dict[str, Any],
                  token: str | None = None) -> bytes:
        import json as _json

        plain = _json.dumps({"server": server, "tool": tool, "args": args,
                             "token": token}).encode()
        return self.session.encrypt(plain, aad=b"kubona-secure-rpc")

    def unwrap_response(self, ciphertext: bytes) -> dict[str, Any]:
        import json as _json

        return _json.loads(self.session.decrypt(ciphertext, aad=b"kubona-secure-rpc"))

    # -- server side -------------------------------------------------------
    def unwrap_call(self, ciphertext: bytes) -> dict[str, Any]:
        import json as _json

        return _json.loads(self.session.decrypt(ciphertext, aad=b"kubona-secure-rpc"))

    def wrap_response(self, result: dict[str, Any]) -> bytes:
        import json as _json

        return self.session.encrypt(_json.dumps(result).encode(), aad=b"kubona-secure-rpc")

    def handle(self, ciphertext: bytes) -> bytes:
        """Full server path: unwrap -> authorize+call gateway -> wrap response."""
        if self.gateway is None:
            raise ValueError("a gateway is required on the server side")
        call = self.unwrap_call(ciphertext)
        try:
            result = self.gateway.call(call["server"], call["tool"], call["args"],
                                       token=call.get("token"))
        except (AuthError, KeyError, Exception) as e:  # noqa: BLE001 — errors become responses
            result = {"ok": False, "error": str(e)}
        return self.wrap_response(result)
