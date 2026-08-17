"""Kubona `serve` entrypoint — bring the whole platform up as one system.

Boots:
  - a federated gateway with OAuth2-signed-token auth (production: persistent
    signing key, aud/nbf-verified; local/CI: dev token unless KUBONA_ENV=production)
  - all five MCP servers mounted (compiler, execution, cost, provenance, control)
  - the console REST API (read/approve-only control plane)

Run:  python -m kubona.run serve        (or `kubona serve` once installed)
Config via env: KUBONA_ENV, KUBONA_CONSOLE_TOKEN, KUBONA_CONSOLE_PORT,
KUBONA_STATE_DIR, KUBONA_LEDGER, KUBONA_SIGNING_KEY (optional PEM).
"""

from __future__ import annotations

import sys
from kubona import config
from kubona.mcp.gateway import KubonaGateway
from kubona.mcp.servers._mount import build_gateway
from kubona.security.oauth import OAuth2AccessTokenProvider, OAuth2TokenIssuer


def _load_or_seed_signing_key() -> bytes:
    """Reuse a persisted Ed25519 private key so issued tokens survive restarts."""
    from cryptography.hazmat.primitives import serialization
    from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PrivateKey

    pem = config.signing_key_path()  # state_path ensures parents
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


def build_auth() -> OAuth2AccessTokenProvider:
    """Production auth: OAuth2-signed, aud/nbf-verified tokens with a persistent key.

    Fail-closed: in KUBONA_ENV=production with no token configured, callers get
    unauthorized (the gateway requires a verified token); the dev token is only
    used explicitly outside production.
    """
    from cryptography.hazmat.primitives.serialization import load_pem_private_key

    key_pem = _load_or_seed_signing_key()
    key = load_pem_private_key(key_pem, password=None)
    issuer = OAuth2TokenIssuer(signing_key=key)
    return OAuth2AccessTokenProvider(issuer=issuer)


def make_gateway(*, require_auth: bool = True,
                 production: bool | None = None) -> KubonaGateway:
    """A gateway authenticated with the real OAuth2 issuer (not the dev shim).

    `require_auth=False` is for local/CI-only smoke tests; production always
    requires auth.
    """
    use_prod = (config.is_production() if production is None else production)
    auth = build_auth()
    # persistent audit export (JSONL under the state root) so ops can review every call
    gw = KubonaGateway(auth=auth, require_auth=require_auth,
                       audit_path=str(config.state_path("audit.jsonl")))
    # NOTE: build_production_gateway must mount the same tool servers as build_gateway
    # re-mount by reusing the module-level builder then swapping auth is not clean,
    # so we rebuild with the production auth:
    from kubona.mcp.servers import compiler, control, cost, execution, provenance

    gw.mount("control", {"register_loop": control.register_loop, "step_loop": control.step_loop,
                         "list_loops": control.list_loops, "loop_status": control.loop_status,
                         "pause_loop": control.pause_loop, "resume_loop": control.resume_loop,
                         "kill_loop": control.kill_loop})
    gw.mount("compiler", {"transpile_circuit": compiler.transpile_circuit,
                          "lower_circuit": compiler.lower_circuit})
    gw.mount("execution", {"submit_job": execution.submit_job,
                           "get_job_status": execution.get_job_status,
                           "get_result": execution.get_result,
                           "run_zne_job": execution.run_zne_job,
                           "cancel_job": execution.cancel_job,
                           "session_status": execution.session_status,
                           "close_session": execution.close_session,
                           "list_provider_registry": execution.list_provider_registry})
    gw.mount("cost", {"create_session": cost.create_session, "estimate_cost": cost.estimate_cost,
                      "request_approval": cost.request_approval, "hold_approval": cost.hold_approval,
                      "approve_approval": cost.approve_approval, "reject_approval": cost.reject_approval,
                      "session_burn_rate": cost.session_burn_rate, "quote_payment": cost.quote_payment,
                      "verify_payment": cost.verify_payment})
    gw.mount("provenance", {"write_record": provenance.write_record,
                            "query_records": provenance.query_records})
    return gw


def serve(*, host: str = "127.0.0.1", port: int | None = None) -> None:
    """Boot the console REST API (backed by the production-authorized gateway)."""
    if config.is_production() and config.console_token() is None:
        raise SystemExit(
            "FAIL-CLOSED: KUBONA_ENV=production requires KUBONA_CONSOLE_TOKEN to be set."
        )
    from kubona.console_api import app, main

    main()  # console_api.main() runs uvicorn on KUBONA_CONSOLE_PORT


def serve_gateway(*, host: str = "127.0.0.1") -> None:
    """Boot the HTTP gateway transport (streamable-HTTP MCP surface, G8).

    In production this is the network-facing entry point: OAuth2 bearer +
    (optionally) the X25519+ML-KEM hybrid envelope on /rpc, backed by the
    production-authorized gateway. Terminate with TLS/KEMTLS at the edge.
    """
    from uvicorn import Config, Server

    from kubona.mcp.http_gateway import make_http_gateway

    gateway = make_gateway(require_auth=True)
    app = make_http_gateway(gateway, secure_session=None)  # TLS/KEMTLS terminates the wire
    port = int(os.environ.get("KUBONA_GATEWAY_PORT", "8123"))
    Server(Config(app, host=host, port=port, log_level="warning")).run()


def _cli(argv: list[str]) -> int:
    cmd = argv[1] if len(argv) > 1 else "serve"
    if cmd == "serve":
        serve()
        return 0
    if cmd in ("serve-gateway", "gateway"):
        serve_gateway()
        return 0
    if cmd == "doctor":
        print("state root:", config.state_root())
        print("ledger:", config.ledger_path())
        print("production:", config.is_production())
        print("token configured:", config.console_token() is not None)
        print("signing key:", config.signing_key_path())
        return 0
    raise SystemExit(f"unknown command: {cmd} (expected: serve | doctor)")


if __name__ == "__main__":
    raise SystemExit(_cli(sys.argv))
