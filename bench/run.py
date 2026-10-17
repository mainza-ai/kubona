#!/usr/bin/env python3
"""Kubona bench runner (L2): hermetic task grading + timing.

Every task runs in-process against the live tree with state sandboxed
to bench/work (never the daemon's ~/.kubona). No LLM calls, no network,
no keys — deterministic floors, CI-safe. Keyed-model comparisons run
elsewhere (operator periodic); this runner refuses to fake them: there
is no model arm here, only pass/fail floors.

Usage: python bench/run.py [--reps N] [--tasks id,...]
Results: bench/results/<utc>.jsonl + markdown table on stdout.
Exit nonzero on any failure.
"""

from __future__ import annotations

import argparse
import json
import os
import re
import shutil
import sys
import time
from datetime import datetime, timezone
from pathlib import Path

BENCH = Path(__file__).resolve().parent
REPO = BENCH.parent
sys.path.insert(0, str(REPO / "platform"))

# Re-exec under the repo venv: system python has neither kubona deps
# nor the mcp SDK (same bootstrap as bench/ctx.sh).
_VENV_PY = REPO / "platform" / ".venv" / "bin" / "python"
if Path(sys.executable).resolve() != _VENV_PY.resolve() and _VENV_PY.exists():
    os.execv(str(_VENV_PY), [str(_VENV_PY), str(Path(__file__).resolve()), *sys.argv[1:]])

WORK = BENCH / "work"
RESULTS = BENCH / "results"


def fresh_state() -> None:
    if WORK.exists():
        shutil.rmtree(WORK)
    WORK.mkdir(parents=True)
    os.environ["KUBONA_STATE_DIR"] = str(WORK)


def render_args(args: dict, prev: dict) -> dict:
    out = {}
    for k, v in args.items():
        if isinstance(v, str) and v.startswith("{{prev.") and v.endswith("}}"):
            key = v[len("{{prev."):-len("}}")]
            if key not in prev:
                raise KeyError(f"template {v} has no prev value")
            out[k] = prev[key]
        else:
            out[k] = v
    return out


def run_gateway_task(task: dict, gw) -> dict:
    prev: dict = {}
    transcript = []
    # Most tools envelope {"ok": true, ...}; list-style reads return bare
    # payloads. require_ok:false grades those on keys alone (their missing
    # envelope is a known contract wart, noted in BENCHMARK.md — bench
    # measures, it does not fix product).
    require_ok = task.get("require_ok", True)
    for step in task.get("steps", [task]):
        if "tool" not in step:
            continue
        args = render_args(step.get("args", {}), prev)
        started = time.monotonic()
        out = gw.call(step["server"], step["tool"], args, token="dev-token")
        wall = time.monotonic() - started
        if require_ok and (not isinstance(out, dict) or out.get("ok") is not True):
            return {"ok": False, "error": f"step failed: {out!r:.200}", "wall_s": wall}
        prev = out if isinstance(out, dict) else {}
        transcript.append(out)
    return {"ok": True, "output": transcript, "wall_s": wall}


def run_mandate_task() -> dict:
    import secrets

    from kubona.cost.mandate import create_mandate, verify_mandate, utc_now_plus

    started = time.monotonic()
    m = create_mandate(secrets.token_bytes(32), "bench-tenant",
                       "job-sha256-bench", 25.0, utc_now_plus(3600))
    ok, reason = verify_mandate(m, "bench-tenant", "job-sha256-bench", 10.0)
    ok2, reason2 = verify_mandate(m, "bench-tenant", "job-sha256-bench", 99.0)
    wall = time.monotonic() - started
    return {"ok": True,
            "output": [{"valid": ok, "reason": reason},
                       {"valid": ok2, "reason": reason2}],
            "wall_s": wall}


def run_ledger_task() -> dict:
    from kubona.provenance import Ledger

    started = time.monotonic()
    led = Ledger()
    led.append("trial", {"harness": "bench", "artifact": "1.0"})
    ok, msg = led.validate_chain()
    wall = time.monotonic() - started
    return {"ok": ok, "output": [{"chain": msg}], "wall_s": wall}


RUNNERS = {
    "gateway": lambda task, gw: run_gateway_task(task, gw),
    "gateway-steps": lambda task, gw: run_gateway_task(task, gw),
    "mandate": lambda task, gw: run_mandate_task(),
    "ledger": lambda task, gw: run_ledger_task(),
}


def grade(task: dict, result: dict) -> tuple[bool, list[str]]:
    missed = []
    blob = json.dumps(result.get("output", []))
    for name, pattern in task.get("keys", {}).items():
        try:
            rx = re.compile(pattern)
        except re.error as e:
            return False, [f"key {name!r} does not compile: {e}"]
        if not rx.search(blob):
            missed.append(name)
    return (not missed) and result.get("ok") is True, missed


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--reps", type=int, default=1)
    ap.add_argument("--tasks", default="")
    args = ap.parse_args()

    tasks = json.loads((BENCH / "tasks.json").read_text())
    want = {t.strip() for t in args.tasks.split(",") if t.strip()}
    if want:
        tasks = [t for t in tasks if t["id"] in want]
    for t in tasks:
        for req in ("id", "keys"):
            if req not in t:
                print(f"task missing {req!r}: {t.get('id')}")
                return 2

    fresh_state()
    from kubona.mcp.servers._mount import build_gateway
    gw = build_gateway(require_auth=False)

    RESULTS.mkdir(exist_ok=True)
    stamp = datetime.now(timezone.utc).strftime("%Y%m%dT%H%M%SZ")
    out_path = RESULTS / f"{stamp}.jsonl"
    failures = 0
    print(f"{'task':<22}{'rep':<5}{'wall_s':<9}{'result'}")
    with out_path.open("w") as fh:
        for task in tasks:
            kind = task.get("kind", "gateway")
            if kind not in RUNNERS:
                print(f"unknown kind {kind!r} in task {task['id']}")
                return 2
            for rep in range(args.reps):
                try:
                    res = RUNNERS[kind](task, gw)
                    passed, missed = grade(task, res)
                except Exception as e:  # noqa: BLE001 — bench reports, never masks
                    res = {"ok": False, "error": f"{type(e).__name__}: {e}"}
                    passed, missed = False, ["exception"]
                row = {"task": task["id"], "rep": rep, "passed": passed,
                       "missed": missed, "wall_s": round(res.get("wall_s", -1), 3),
                       "at": stamp}
                fh.write(json.dumps(row) + "\n")
                mark = "PASS" if passed else f"FAIL missed={missed}"
                print(f"{task['id']:<22}{rep:<5}{row['wall_s']:<9}{mark}")
                failures += not passed
    print(f"results: {out_path} ({failures} failures)")
    return 1 if failures else 0


if __name__ == "__main__":
    raise SystemExit(main())
