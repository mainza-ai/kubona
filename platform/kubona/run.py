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
from pathlib import Path
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


def _load_dotenv_file(path: "Path") -> None:
    """Load KEY=VALUE lines without overriding the process environment."""
    from pathlib import Path

    try:
        for line in Path(path).read_text().splitlines():
            line = line.strip()
            if not line or line.startswith("#") or "=" not in line:
                continue
            k, v = line.split("=", 1)
            k = k.strip()
            v = v.strip().strip('"\'')
            if k and not os.environ.get(k):
                os.environ[k] = v
    except Exception:
        return


def _contracts_dir() -> "Path":
    """Contracts directory (shipped in the repo/image, env-overridable)."""
    from pathlib import Path

    override = os.environ.get("KUBONA_CONTRACTS_DIR", "").strip()
    if override:
        return Path(override).expanduser()
    import kubona

    return Path(kubona.__file__).resolve().parents[1] / "contracts"


def _doctor_rate_card() -> tuple[str, bool]:
    """Rate-card freshness (hard boot gate). Returns (text, ok)."""
    import json
    from datetime import datetime, timedelta, timezone

    path = _contracts_dir() / "rate-cards.json"
    try:
        card = json.loads(path.read_text())
        effective = datetime.strptime(str(card["effective_date"]), "%Y-%m-%d").date()
        window = int(card["stale_after_days"])
        stale_on = effective + timedelta(days=window)
        today = datetime.now(timezone.utc).date()
        days = (stale_on - today).days
        text = f"{card.get('version', '?')} effective {effective} (stale {stale_on}, {days:+d}d)"
        if card.get("heuristic") is True:
            text += " · heuristic"
        if days < 0:
            return f"STALE — {text}; update before cloud/lens pricing", False
        return text, True
    except Exception as e:  # noqa: BLE001 — doctor reports, never crashes
        return f"unreadable: {e}", False


def _doctor_fk(db_path: "Path | None" = None) -> tuple[str, bool]:
    """FK orphan count on the Go ledger (read-only). Missing DB is not a failure."""
    import sqlite3

    if db_path is None:
        try:
            db_path = config.state_path("kubona.db")
        except Exception as e:  # noqa: BLE001
            return f"unresolvable: {e}", False
    if not db_path.exists():
        return "absent (fresh state — nothing to audit)", True
    try:
        conn = sqlite3.connect(f"file:{db_path}?mode=ro", uri=True)
        try:
            rows = conn.execute("PRAGMA foreign_key_check").fetchall()
        finally:
            conn.close()
        if rows:
            return f"{len(rows)} orphan row(s) — run `kubonad fk-quarantine --apply` after backup", False
        return f"clean ({db_path.name})", True
    except Exception as e:  # noqa: BLE001
        return f"unreadable: {e}", False


def _doctor_backups() -> tuple[str, bool]:
    """Scheduled-backup freshness/failure. Absence is optional, not fatal."""
    import re
    from datetime import datetime, timedelta, timezone

    raw_dir = os.environ.get("KUBONA_BACKUP_DIR", "").strip()
    try:
        backup_dir = Path(raw_dir) if raw_dir else config.state_path("backups")
    except Exception as e:  # noqa: BLE001
        return f"unresolvable: {e}", False
    if not backup_dir.exists():
        return "absent (backup profile not enabled)", True
    pattern = re.compile(r"\.backup-(\d{8}T\d{6}Z)(?:\.age)?$")
    newest = None
    count = 0
    try:
        for entry in backup_dir.iterdir():
            if not entry.is_file():
                continue
            m = pattern.search(entry.name)
            if not m:
                continue
            count += 1
            try:
                at = datetime.strptime(m.group(1), "%Y%m%dT%H%M%SZ").replace(tzinfo=timezone.utc)
            except ValueError:
                continue
            if newest is None or at > newest:
                newest = at
    except Exception as e:  # noqa: BLE001
        return f"unreadable: {e}", False
    if count == 0 or newest is None:
        return f"no verified backups in {backup_dir}", False
    max_age_text = os.environ.get("KUBONA_BACKUP_MAX_AGE", "26h").strip()
    try:
        unit = max_age_text[-1]
        amount = int(max_age_text[:-1])
        if unit == "h":
            max_age = timedelta(hours=amount)
        elif unit == "m":
            max_age = timedelta(minutes=amount)
        elif unit == "d":
            max_age = timedelta(days=amount)
        else:
            raise ValueError("use h/m/d suffix")
    except Exception:
        return f"invalid KUBONA_BACKUP_MAX_AGE={max_age_text!r}", False
    age = datetime.now(timezone.utc) - newest
    if age > max_age:
        return f"STALE — newest {newest.isoformat()} ({age} old, max {max_age_text})", False
    # A failed attempt newer than the last success is a silent backup outage.
    success = backup_dir / ".last-success"
    failure = backup_dir / ".last-failure"
    if failure.exists() and (not success.exists() or failure.stat().st_mtime > success.stat().st_mtime):
        return f"last attempt FAILED; newest copy {age} old", False
    return f"{count} verified artifact(s); newest {age} old (max {max_age_text})", True


def _doctor_llm() -> tuple[str, bool]:
    """Effective LLM posture from the Go settings DB when present, else env."""
    import json
    import sqlite3

    provider = os.environ.get("KUBONA_LLM_PROVIDER", "strata") or "strata"
    air_gap = "unknown"
    codesign = "unassigned"
    try:
        db = config.state_path("kubona.db")
        if db.exists():
            conn = sqlite3.connect(f"file:{db}?mode=ro", uri=True)
            try:
                row = conn.execute(
                    "SELECT value_json FROM llm_settings WHERE key = 'active_settings'"
                ).fetchone()
            finally:
                conn.close()
            if row:
                snap = json.loads(row[0])
                provider = snap.get("active_provider", provider)
                air_gap = "on" if snap.get("air_gapped") else "off"
                role = (snap.get("role_assignments") or {}).get("codesign") or {}
                if role:
                    codesign = f"{role.get('provider', '?')}/{role.get('model', '?')}"
    except Exception as e:  # noqa: BLE001
        if "no such table" in str(e):
            return f"active={provider}; settings not initialized (fresh state)", True
        return f"{provider}; settings unreadable: {e}", False
    cloud = "allowed" if os.environ.get("KUBONA_LLM_CLOUD") == "1" else "not opted in"
    return f"active={provider}; air-gap={air_gap}; codesign={codesign}; cloud={cloud}", True


def _doctor_keys() -> tuple[str, bool]:
    """Presence/shape only — never print key values."""
    import binascii

    reviewer = os.environ.get("KUBONA_REVIEWER_PRIVKEY_HEX", "").strip()
    operator = os.environ.get("KUBONA_OPERATOR_PRIVKEY_HEX", "").strip()
    ibm = os.environ.get("KUBONA_IBM_QISKIT_TOKEN", "").strip()
    if reviewer:
        try:
            raw = binascii.unhexlify(reviewer)
            if len(raw) not in (32, 64):
                raise ValueError(f"{len(raw)} bytes (want 32-byte seed or 64-byte key)")
            reviewer_state = f"configured ({len(raw)}B)"
        except Exception as e:  # noqa: BLE001
            reviewer_state = f"INVALID: {e}"
    else:
        reviewer_state = "absent"
    text = f"reviewer={reviewer_state}; operator={'configured' if operator else 'absent'}; IBM-QPU={'configured' if ibm else 'absent'}"
    # Reviewer is the mandate-signing blocker; QPU is optional for local-only
    # deployments, so its absence is reported but does not fail --strict.
    ok = reviewer_state.startswith("configured")
    return text, ok


def _doctor_http() -> tuple[str, bool]:
    """Local daemon readiness; a stopped stack is reported, not treated as fatal."""
    import urllib.error
    import urllib.request

    try:
        with urllib.request.urlopen("http://127.0.0.1:8080/api/ready", timeout=2) as r:
            return f"ready (HTTP {r.status})", r.status == 200
    except urllib.error.URLError as e:
        return f"not running ({e.reason})", True
    except Exception as e:  # noqa: BLE001
        return f"unreachable: {e}", True


def _doctor_bridge() -> tuple[str, bool]:
    sock = os.environ.get("KUBONA_BRIDGE_SOCKET", "").strip()
    addr = os.environ.get("KUBONA_BRIDGE_ADDR", "").strip()
    if sock:
        from pathlib import Path

        return (f"UDS {sock} ({'present' if Path(sock).exists() else 'missing'})", Path(sock).exists())
    if addr:
        return f"TCP {addr} (configured; fleet probe owns liveness)", True
    return "unconfigured (compose uses /run/kubona/bridge.sock)", True


def _cli(argv: list[str]) -> int:
    cmd = argv[1] if len(argv) > 1 else "serve"
    if cmd == "serve":
        serve()
        return 0
    if cmd in ("serve-gateway", "gateway"):
        serve_gateway()
        return 0
    if cmd == "doctor":
        args = argv[2:]
        strict = "--strict" in args
        if "--env-file" in args:
            i = args.index("--env-file")
            if i + 1 < len(args):
                _load_dotenv_file(args[i + 1])
        failures: list[str] = []

        def report(label: str, value: tuple[str, bool] | str, ok: bool = True) -> None:
            if isinstance(value, tuple):
                text, passed = value
            else:
                text, passed = value, ok
            print(f"{label}: {text}")
            if not passed:
                failures.append(label)

        print("state root:", config.state_root())
        print("ledger:", config.ledger_path())
        print("production:", config.is_production())
        print("token configured:", config.console_token() is not None)
        print("signing key:", config.signing_key_path())
        print("providers:", _doctor_providers())
        print("ledger check:", _doctor_ledger())
        print("skill:", _doctor_skill())
        print("taxonomy:", _doctor_taxonomy())
        report("rate card", _doctor_rate_card())
        report("fk orphans", _doctor_fk())
        report("llm", _doctor_llm())
        report("backups", _doctor_backups())
        report("keys", _doctor_keys())
        report("bridge", _doctor_bridge())
        report("daemon", _doctor_http())
        if strict and failures:
            print("doctor: FAILED —", ", ".join(failures))
            return 1
        if failures:
            print("doctor: warnings —", ", ".join(failures))
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
