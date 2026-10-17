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

import os
import sys
from kubona import config
from kubona.mcp.gateway import KubonaGateway
from kubona.mcp.servers._mount import build_gateway
from kubona.security.oauth import OAuth2AccessTokenProvider, OAuth2TokenIssuer


def _load_or_seed_signing_key() -> bytes:
    """Reuse a persisted Ed25519 private key so issued tokens survive restarts."""
    from kubona.security.oauth import load_or_seed_signing_key
    return load_or_seed_signing_key()


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

    Single builder (W2.14): the mount table lives in
    `kubona.mcp.servers._mount.build_gateway` — this function only supplies
    the production auth + persistent audit export, so the production tool
    surface cannot drift from the tested one.

    `require_auth=False` is for local/CI-only smoke tests; production always
    requires auth.
    """
    use_prod = (config.is_production() if production is None else production)
    auth = build_auth()
    # persistent audit export (JSONL under the state root) so ops can review every call
    return build_gateway(
        require_auth=require_auth if not use_prod else True,
        auth=auth,
        audit_path=str(config.state_path("audit.jsonl")),
    )


def serve(*, host: str = "127.0.0.1", port: int | None = None) -> None:
    """Boot the console REST API (backed by the production-authorized gateway).

    `host`/`port` are threaded into the server (W2.14); `port=None` keeps
    the KUBONA_CONSOLE_PORT env default.
    """
    from kubona.logging import setup_logging
    setup_logging(log_dir=None)
    if config.is_production() and config.console_token() is None:
        raise SystemExit(
            "FAIL-CLOSED: KUBONA_ENV=production requires KUBONA_CONSOLE_TOKEN to be set."
        )
    from kubona.console_api import main

    main(host=host, port=port)  # console_api.main() runs uvicorn on host/port


def serve_gateway(*, host: str = "127.0.0.1", port: int | None = None) -> None:
    """Boot the HTTP gateway transport (streamable-HTTP MCP surface, G8).

    In production this is the network-facing entry point: OAuth2 bearer +
    (optionally) the X25519+ML-KEM hybrid envelope on /rpc, backed by the
    production-authorized gateway. Terminate with TLS/KEMTLS at the edge.
    """
    from uvicorn import Config, Server

    from kubona.mcp.http_gateway import make_http_gateway

    gateway = make_gateway(require_auth=True)
    app = make_http_gateway(gateway, secure_session=None)  # TLS/KEMTLS terminates the wire
    # W2.14: explicit `port=` wins; otherwise the env default (never ignored).
    resolved_port = port if port is not None else int(os.environ.get("KUBONA_GATEWAY_PORT", "8123"))
    Server(Config(app, host=host, port=resolved_port, log_level="warning")).run()


def _doctor_providers() -> str:
    """Provider inventory (local registry read, no network, no keys)."""
    try:
        from kubona.providers import list_providers
        names = sorted(list_providers())
        return f"{len(names)} ({', '.join(names[:5])}{'...' if len(names) > 5 else ''})"
    except Exception as e:  # noqa: BLE001 — doctor reports, never crashes
        return f"unavailable: {e}"


def _doctor_ledger() -> str:
    """Sidecar ledger readability (parse check only, not chain validation:
    live daemons append concurrently, so a torn read must never report
    CORRUPT — counts + JSON shape only, chain validity stays with the
    ledger's own tests)."""
    import json

    try:
        path = config.ledger_path()
    except Exception as e:  # noqa: BLE001
        return f"unresolvable: {e}"
    if not path.exists():
        return "missing (fresh state — nothing recorded yet)"
    try:
        lines = [ln for ln in path.read_text().splitlines() if ln.strip()]
        for ln in lines:
            json.loads(ln)
        return f"{len(lines)} records, parseable"
    except Exception as e:  # noqa: BLE001
        return f"unreadable: {e}"


def _doctor_skill() -> str:
    """Operator-guidance surface health (repo checkouts carry it;
    package installs legitimately do not — reported neutrally)."""
    from pathlib import Path

    import kubona

    skill = Path(kubona.__file__).resolve().parents[2] / "skills" / "kubona" / "SKILL.md"
    if not skill.exists():
        return "absent (repo checkout only)"
    return f"present {skill.stat().st_size} bytes"


def _doctor_taxonomy() -> str:
    """Shared event taxonomy loads (agents depend on the closed kind set)."""
    import json
    from pathlib import Path

    import kubona

    contract = Path(kubona.__file__).resolve().parents[1] / "contracts" / "event-types.json"
    try:
        kinds = len(json.loads(contract.read_text())["types"])
        return f"{kinds} kinds"
    except Exception as e:  # noqa: BLE001
        return f"unreadable: {e}"


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
        print("providers:", _doctor_providers())
        print("ledger check:", _doctor_ledger())
        print("skill:", _doctor_skill())
        print("taxonomy:", _doctor_taxonomy())
        return 0
    if cmd == "mandate-check":
        return _mandate_check(argv[2:])
    raise SystemExit(f"unknown command: {cmd} (expected: serve | doctor | mandate-check)")


def _mandate_check(args: list[str]) -> int:
    """Offline mandate first-move (L4): shape validity by crypto, admission
    by receipt lookup — reported as SEPARATE fields (LC5). Never touches
    the network and never writes. Exit 0 = answered (even when invalid);
    exit 2 = usage error.
    """
    import argparse
    import json
    import sqlite3
    from pathlib import Path

    ap = argparse.ArgumentParser(prog="kubona.run mandate-check")
    ap.add_argument("--mandate", required=True, help="mandate JSON file")
    ap.add_argument("--tenant", required=True)
    ap.add_argument("--job-hash", required=True)
    ap.add_argument("--cost", required=True, type=float)
    ap.add_argument("--db", default="", help="Go ledger SQLite (default ~/.kubona/kubona.db)")
    ap.add_argument("--window", type=int, default=1000, help="recent receipts scanned")
    try:
        ns = ap.parse_args(args)
    except SystemExit:
        return 2
    try:
        raw = json.loads(open(ns.mandate).read())
    except (OSError, ValueError) as e:
        print(json.dumps({"error": f"mandate file unreadable: {e}"}))
        return 2

    from kubona.cost.mandate import SpendMandate, verify_mandate

    try:
        mandate = SpendMandate(**{k: raw[k] for k in
                                  ("tenant_id", "job_hash", "max_budget_usd",
                                   "expiry_timestamp", "public_key_hex", "signature_hex")})
    except (KeyError, TypeError) as e:
        print(json.dumps({"shape_valid": False, "reason": f"mandate shape: {e}"}))
        return 0
    valid, reason = verify_mandate(mandate, ns.tenant, ns.job_hash, ns.cost)
    out: dict = {"shape_valid": valid, "reason": reason}

    # Admission !== shape: a valid mandate spends nothing until a
    # mandate_verified receipt cites it. Absence in the window is not
    # proof of absence — the window and db path ride along.
    db = ns.db or str(Path.home() / ".kubona" / "kubona.db")
    out["db"] = db
    out["admitted"] = False
    if os.path.exists(db):
        try:
            conn = sqlite3.connect(f"file:{db}?mode=ro", uri=True)
            rows = conn.execute(
                "SELECT payload FROM ledger_events WHERE event_type LIKE 'mandate_verified%'"
                " ORDER BY id DESC LIMIT ?", (ns.window,)).fetchall()
            conn.close()
            for (payload,) in rows:
                try:
                    if json.loads(payload).get("signature_hex") == mandate.signature_hex:
                        out["admitted"] = True
                        break
                except ValueError:
                    continue
            if not out["admitted"]:
                out["admission_note"] = (
                    f"no receipt in recent {ns.window} (not proof of absence — "
                    "check full history or admit first)")
        except Exception as e:  # noqa: BLE001 — report, never crash the check
            out["admission_note"] = f"receipt lookup failed: {e}"
    else:
        out["admission_note"] = "ledger db missing (fresh state — nothing admitted yet)"
    print(json.dumps(out, indent=2))
    return 0


if __name__ == "__main__":
    raise SystemExit(_cli(sys.argv))
