"""Streamable-HTTP transport for the gateway (G8).

A real network surface so Kubona's federated MCP servers are deployable as
services (not just in-process/stdio). Two inbound modes on /rpc:

  1. Plain authenticated RPC  {server, tool, args}  gated by an OAuth2 bearer
     token (the signed-issuer provider) — the base HTTP path.
  2. Hybrid-envelope RPC  {envelope: <base64>}  carried over a real X25519+ML-KEM
     hybrid session (harvest-now-decrypt-later), unwrapped and dispatched, the
     response re-encrypted. The sender must already have established the session
     via handshake_initiator (see hybrid_transport); the server holds the
     responder side.

The streamable-HTTP MCP transport (mcp-sdk) plus TLS/KEMTLS terminate the same
posture on the wire; this module proves the path and is what `kubona serve` wires.
"""

from __future__ import annotations

import base64
import json
import os
import time
from typing import Any

from starlette.applications import Starlette
from starlette.requests import Request
from starlette.responses import JSONResponse
from starlette.routing import Route

from kubona.mcp.auth import AuthError, Principal
from kubona.mcp.gateway import KubonaGateway
from kubona.logging import get_logger, log_call
from kubona.security.hybrid_transport import HybridSession, TransportError
from kubona.security.secure_rpc import SecureRpc

# Hard cap on inbound bodies (W0.14): unbounded request.json() is a
# large-body DoS primitive. Envelopes are small authenticated blobs.
MAX_BODY_BYTES = 1 * 1024 * 1024


def _principal(request: Request, gateway: KubonaGateway) -> Principal | None:
    token = request.headers.get("authorization", "")
    if not token.startswith("Bearer "):
        return None
    try:
        return gateway.auth.verify(token[len("Bearer "):])
    except Exception:  # noqa: BLE001
        return None


_rpc_logger = get_logger("http")


def _make_app(gateway: KubonaGateway, secure_session: HybridSession | None = None,
                require_secure: bool = False) -> Starlette:
    """W0.14: require_secure refuses to serve the hybrid mode without a
    session (silent downgrade to plain used to be a None away), and the
    health surface is minimal (no server inventory for recon)."""
    if require_secure and secure_session is None:
        raise ValueError("require_secure=True needs a secure_session: refusing to serve downgraded")

    async def health(request: Request) -> JSONResponse:
        return JSONResponse({"ok": True, "service": "kubona-http-gateway"})

    async def rpc(request: Request) -> JSONResponse:
        outer = _principal(request, gateway)
        if outer is None:
            return JSONResponse({"ok": False, "error": "unauthorized: Bearer <token>"}, status_code=401)
        try:
            cl = int(request.headers.get("content-length", "0") or 0)
        except (ValueError, TypeError):
            return JSONResponse({"ok": False, "error": "invalid content-length"}, status_code=400)
        if cl > MAX_BODY_BYTES:
            return JSONResponse({"ok": False, "error": "body too large"}, status_code=413)
        bearer = request.headers["authorization"][len("Bearer "):]
        try:
            raw_body = await request.body()
            if len(raw_body) > MAX_BODY_BYTES:
                return JSONResponse({"ok": False, "error": "body too large"}, status_code=413)
            body = json.loads(raw_body.decode("utf-8"))
        except Exception:  # noqa: BLE001
            return JSONResponse({"ok": False, "error": "invalid JSON body"}, status_code=400)
        if require_secure and "envelope" not in body:
            return JSONResponse({"ok": False, "error": "secure envelope required: refusing plaintext request"}, status_code=400)
        t0 = time.time()
        try:
            if "envelope" in body and secure_session is not None:
                ct = base64.b64decode(body["envelope"])
                if len(ct) > MAX_BODY_BYTES:
                    return JSONResponse({"ok": False, "error": "envelope too large"}, status_code=413)
                # Single envelope implementation (W0.14): nonce/expiry/
                # replay/identity-match live in SecureRpc, not here.
                resp = SecureRpc(secure_session, gateway).handle(ct, expected_sub=outer.sub)
                log_call(_rpc_logger, server="?", tool="?", status=200,
                         duration_ms=(time.time() - t0) * 1000, mode="secure",
                         principal=outer.sub)
                return JSONResponse({"ok": True, "envelope": base64.b64encode(resp).decode()})
            # plain authenticated RPC — pass the verified bearer token so the gateway
            # re-derives the principal and applies per-call scope authorization
            result = gateway.call(body["server"], body["tool"], body.get("args", {}),
                                   token=bearer)
            log_call(_rpc_logger, server=body["server"], tool=body["tool"], status=200,
                      duration_ms=(time.time() - t0) * 1000, mode="plain",
                      principal=outer.sub)
            return JSONResponse({"ok": True, "result": result})
        except AuthError as e:
            log_call(_rpc_logger, server=body.get("server", "?"), tool=body.get("tool", "?"),
                      status=403, duration_ms=(time.time() - t0) * 1000, error=str(e),
                      principal=outer.sub)
            return JSONResponse({"ok": False, "error": f"authorization denied: {e}"}, status_code=403)
        except KeyError as e:
            log_call(_rpc_logger, server=body.get("server", "?"), tool=body.get("tool", "?"),
                      status=404, duration_ms=(time.time() - t0) * 1000, error=str(e),
                      principal=outer.sub)
            return JSONResponse({"ok": False, "error": f"unknown server/tool: {e}"}, status_code=404)
        except TransportError as e:
            log_call(_rpc_logger, server="?", tool="?", status=400,
                      duration_ms=(time.time() - t0) * 1000, error=str(e),
                      principal=outer.sub)
            return JSONResponse({"ok": False, "error": f"decryption failed: {e}"}, status_code=400)
        except Exception as e:  # noqa: BLE001
            log_call(_rpc_logger, server=body.get("server", "?"), tool=body.get("tool", "?"),
                      status=400, duration_ms=(time.time() - t0) * 1000, error=str(e),
                      principal=outer.sub)
            return JSONResponse({"ok": False, "error": str(e)}, status_code=400)

    return Starlette(routes=[Route("/health", health), Route("/rpc", rpc, methods=["POST"])])


def make_http_gateway(gateway: KubonaGateway | None = None,
                      secure_session: HybridSession | None = None,
                      require_secure: bool = False) -> Starlette:
    """Build the HTTP app over (optionally) a production-authorized gateway.

    Pass a `secure_session` (the server's responder side of a real X25519+ML-KEM
    handshake) to enable the hybrid-envelope mode; TLS/KEMTLS carries it publicly.
    require_secure=True refuses to build without a session (W0.14).
    """
    if gateway is None:
        from kubona.run import make_gateway

        gateway = make_gateway(require_auth=True)
    return _make_app(gateway, secure_session, require_secure)


if __name__ == "__main__":
    from uvicorn import Config, Server

    app = make_http_gateway()
    port = int(os.environ.get("KUBONA_GATEWAY_PORT", "8123"))
    Server(Config(app, host="127.0.0.1", port=port, log_level="warning")).run()