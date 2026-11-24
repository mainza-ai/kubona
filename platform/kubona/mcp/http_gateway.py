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
import contextlib
import hmac
import json
import os
import time

from starlette.applications import Starlette
from starlette.concurrency import run_in_threadpool
from starlette.requests import Request
from starlette.responses import JSONResponse
from starlette.routing import Route

from kubona.mcp.auth import AuthError, Principal
from kubona.mcp.gateway import KubonaGateway
from kubona.mcp.unified import CURRENT_BEARER_TOKEN, PROFILES, build_unified_mcp_server
from kubona.logging import get_logger, log_call
from kubona.security.hybrid_transport import HybridSession, TransportError
from kubona.security.secure_rpc import SecureRpc

# Hard cap on inbound bodies (W0.14): unbounded request.json() is a
# large-body DoS primitive. Envelopes are small authenticated blobs.
MAX_BODY_BYTES = 1 * 1024 * 1024


class _ApiTokenBridgeAuth:
    """Wraps a gateway AuthProvider so KUBONA_API_TOKEN is also accepted as an
    operator bearer credential on /mcp and /rpc (D-10), while preserving full
    OAuth2 JWT verification for issued agent tokens."""

    def __init__(self, inner, api_token: str) -> None:
        self._inner = inner
        self._api_token = api_token

    def verify(self, token: str) -> Principal:
        try:
            return self._inner.verify(token)
        except Exception:
            if self._api_token and token and hmac.compare_digest(token, self._api_token):
                from kubona.security.policy import OPERATOR_SCOPES
                return Principal(
                    sub="kubona-api-operator",
                    role="operator",
                    scopes=tuple(OPERATOR_SCOPES),
                    tenant="default",
                )
            raise


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
        if not isinstance(body, dict):
            return JSONResponse({"ok": False, "error": "JSON body must be an object"}, status_code=400)
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
                resp = await run_in_threadpool(SecureRpc(secure_session, gateway).handle, ct, expected_sub=outer.sub)
                log_call(_rpc_logger, server="?", tool="?", status=200,
                         duration_ms=(time.time() - t0) * 1000, mode="secure",
                         principal=outer.sub)
                return JSONResponse({"ok": True, "envelope": base64.b64encode(resp).decode()})
            # plain authenticated RPC — pass the verified bearer token so the gateway
            # re-derives the principal and applies per-call scope authorization
            if not isinstance(body.get("server"), str) or not isinstance(body.get("tool"), str):
                return JSONResponse({"ok": False, "error": "server and tool strings are required"}, status_code=400)
            result = await run_in_threadpool(
                gateway.call, body["server"], body["tool"], body.get("args", {}), token=bearer
            )
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

    # Standard MCP Streamable HTTP (JSON-RPC 2.0) on /mcp alongside /rpc.
    # Each profile gets a stateless, JSON-response MCPServer routed through
    # the same KubonaGateway (auth + PolicyEngine + MRTR approval + audit).
    profile_servers = {
        prof: build_unified_mcp_server(gateway, profile=prof)
        for prof in PROFILES
    }
    from mcp.server.transport_security import TransportSecuritySettings

    transport_sec = TransportSecuritySettings(
        enable_dns_rebinding_protection=True,
        allowed_hosts=["127.0.0.1:*", "localhost:*", "[::1]:*", "mcp:*"],
        allowed_origins=["http://127.0.0.1:*", "http://localhost:*", "http://[::1]:*"],
    )
    profile_apps = {
        prof: srv.streamable_http_app(
            streamable_http_path="/mcp",
            stateless_http=True,
            json_response=True,
            transport_security=transport_sec,
        )
        for prof, srv in profile_servers.items()
    }

    @contextlib.asynccontextmanager
    async def lifespan(_app: Starlette):
        async with contextlib.AsyncExitStack() as stack:
            for srv in profile_servers.values():
                await stack.enter_async_context(srv.session_manager.run())
            yield

    async def mcp_endpoint(scope, receive, send):
        request = Request(scope, receive)
        outer = _principal(request, gateway)
        if outer is None:
            resp = JSONResponse({"ok": False, "error": "unauthorized: Bearer <token>"}, status_code=401)
            await resp(scope, receive, send)
            return
        if require_secure:
            resp = JSONResponse(
                {"ok": False, "error": "secure envelope required: use /rpc with hybrid envelope"},
                status_code=400,
            )
            await resp(scope, receive, send)
            return
        profile = (request.query_params.get("profile") or "all").strip()
        sub_app = profile_apps.get(profile)
        if sub_app is None:
            resp = JSONResponse(
                {"ok": False, "error": f"unknown MCP profile {profile!r}; allowed: {sorted(PROFILES)}"},
                status_code=400,
            )
            await resp(scope, receive, send)
            return
        bearer = request.headers["authorization"][len("Bearer "):]
        tok = CURRENT_BEARER_TOKEN.set(bearer)
        try:
            await sub_app(scope, receive, send)
        finally:
            CURRENT_BEARER_TOKEN.reset(tok)

    class _MCPRoute(Route):
        """ASGI route delegating /mcp directly to the authenticated MCP app."""

        def __init__(self) -> None:
            super().__init__("/mcp", endpoint=mcp_endpoint, methods=["GET", "POST", "DELETE"])
            self.app = mcp_endpoint

    return Starlette(
        routes=[
            Route("/health", health),
            Route("/rpc", rpc, methods=["POST"]),
            _MCPRoute(),
        ],
        lifespan=lifespan,
    )


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
    api_token = os.environ.get("KUBONA_API_TOKEN", "").strip()
    if api_token and gateway.auth is not None and not isinstance(gateway.auth, _ApiTokenBridgeAuth):
        gateway.auth = _ApiTokenBridgeAuth(gateway.auth, api_token)
    return _make_app(gateway, secure_session, require_secure)


if __name__ == "__main__":
    from uvicorn import Config, Server

    app = make_http_gateway()
    port = int(os.environ.get("KUBONA_GATEWAY_PORT", "8123"))
    Server(Config(app, host="127.0.0.1", port=port, log_level="warning")).run()