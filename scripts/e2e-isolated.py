#!/usr/bin/env python3
"""Run the browser E2E suite against a disposable local API + console.

The production live stack must never be the E2E target: those tests create labs
and campaigns that persist in the operator's workspace. This launcher builds a
temporary kubonad ledger, starts an isolated console on dedicated ports, runs
Playwright, and tears both down.

Requirements:
  - KUBONA_BRIDGE_SOCKET (recommended) and KUBONA_BRIDGE_TOKEN
  - a built console (.next) or run with --build-console
  - Playwright browser/chrome available (PLAYWRIGHT_CHANNEL=chrome for system Chrome)
"""
from __future__ import annotations

import argparse
import os
import secrets
import signal
import subprocess
import sys
import tempfile
import time
import urllib.request
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
CORE = ROOT / "platform" / "core"
CONSOLE = ROOT / "platform" / "console"


def wait_for(url: str, timeout: float = 90.0) -> None:
    deadline = time.time() + timeout
    last: Exception | None = None
    while time.time() < deadline:
        try:
            with urllib.request.urlopen(url, timeout=2) as response:
                if response.status < 500:
                    return
        except Exception as exc:  # noqa: BLE001 - launcher reports the last transport cause
            last = exc
        time.sleep(0.5)
    raise RuntimeError(f"timed out waiting for {url}: {last}")


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--api-port", type=int, default=18080)
    parser.add_argument("--console-port", type=int, default=13000)
    parser.add_argument("--build-console", action="store_true")
    parser.add_argument("playwright_args", nargs="*", help="extra args after --")
    args = parser.parse_args()

    bridge_socket = os.environ.get("KUBONA_BRIDGE_SOCKET", "")
    bridge_token = os.environ.get("KUBONA_BRIDGE_TOKEN") or os.environ.get("KUBONA_API_TOKEN", "")
    if not bridge_socket:
        return fail("KUBONA_BRIDGE_SOCKET is required; E2E must run against a real bridge")
    if not bridge_token:
        return fail("KUBONA_BRIDGE_TOKEN (or KUBONA_API_TOKEN) is required")
    if not os.environ.get("KUBONA_REVIEWER_PRIVKEY_HEX"):
        return fail("KUBONA_REVIEWER_PRIVKEY_HEX is required for a production-mode API; set it from .env")
    if not (CONSOLE / ".next").exists() or args.build_console:
        run([ "pnpm", "build" ], cwd=CONSOLE, env=os.environ.copy())

    tmp = Path(tempfile.mkdtemp(prefix="kubona-e2e-"))
    db = tmp / "kubona.db"
    state = tmp / "state"
    state.mkdir()
    api_token = os.environ.get("KUBONA_API_TOKEN") or "e2e-isolated-token"
    bin_path = tmp / "kubonad"
    run(["go", "build", "-o", str(bin_path), "./cmd/kubonad"], cwd=CORE, env=os.environ.copy())

    env = os.environ.copy()
    env.update({
        "KUBONA_ENV": "production",
        "KUBONA_SINGLE_LAB": "1",
        "KUBONA_API_TOKEN": api_token,
        "KUBONA_BRIDGE_SOCKET": bridge_socket,
        "KUBONA_BRIDGE_TOKEN": bridge_token,
        "KUBONA_STATE_DIR": str(state),
        "KUBONA_RATE_CARD_PATH": str(ROOT / "platform" / "contracts" / "rate-cards.json"),
        "KUBONA_CORS_ORIGINS": f"http://localhost:{args.console_port},http://127.0.0.1:{args.console_port}",
        "KUBONA_LEASE_HMAC_KEY": os.environ.get("KUBONA_LEASE_HMAC_KEY") or secrets.token_hex(32),
        "KUBONA_REVIEWER_PRIVKEY_HEX": os.environ["KUBONA_REVIEWER_PRIVKEY_HEX"],
        "STRATA_MODEL": os.environ.get("STRATA_MODEL", "iq3_s"),
    })
    api = subprocess.Popen(
        [str(bin_path), "--db", str(db), "--bind", "127.0.0.1", "--port", str(args.api_port)],
        cwd=CORE, env=env, stdout=(tmp / "api.log").open("w"), stderr=subprocess.STDOUT,
        start_new_session=True,
    )
    console_env = os.environ.copy()
    console_env.update({"KUBONA_CONSOLE_API_URL": f"http://localhost:{args.api_port}"})
    console = subprocess.Popen(
        ["pnpm", "exec", "next", "start", "-H", "127.0.0.1", "-p", str(args.console_port)],
        cwd=CONSOLE, env=console_env, stdout=(tmp / "console.log").open("w"), stderr=subprocess.STDOUT,
        start_new_session=True,
    )
    try:
        wait_for(f"http://127.0.0.1:{args.api_port}/api/health")
        wait_for(f"http://127.0.0.1:{args.console_port}/")
        test_env = env | {
            "KUBONA_CONSOLE_URL": f"http://localhost:{args.console_port}",
            "E2E_OPERATOR_TOKEN": api_token,
        }
        run(["pnpm", "exec", "playwright", "test", *args.playwright_args], cwd=CONSOLE, env=test_env)
        return 0
    finally:
        for proc in (console, api):
            try:
                os.killpg(os.getpgid(proc.pid), signal.SIGTERM)
            except ProcessLookupError:
                pass
        for proc in (console, api):
            try:
                proc.wait(timeout=10)
            except subprocess.TimeoutExpired:
                try:
                    os.killpg(os.getpgid(proc.pid), signal.SIGKILL)
                except ProcessLookupError:
                    pass
        print(f"e2e-isolated: state left at {tmp} for inspection")


def run(cmd: list[str], cwd: Path, env: dict[str, str]) -> None:
    result = subprocess.run(cmd, cwd=cwd, env=env)
    if result.returncode != 0:
        raise SystemExit(result.returncode)


def fail(message: str) -> int:
    print(f"e2e-isolated: {message}", file=sys.stderr)
    return 1


if __name__ == "__main__":
    raise SystemExit(main())
