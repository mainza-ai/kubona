"""AG-1: end-to-end benchmark for Kubona-domain scientific re-discovery.

The runner drives the real HTTP API (no simulator shortcuts): it creates a
session, submits the task's experiment, polls the job, extracts the measured
field, and scores it against a hidden target. Metrics are pass@k, cost, wall
time, and a failure label. Run it against an isolated stack, never the
operator's live ledger:

  KUBONA_API_TOKEN=... python3 platform/bench_agentic/runner.py \
      --api-url http://127.0.0.1:18080 --attempts 1 --json-out /tmp/bench.json
"""

from __future__ import annotations

import argparse
import json
import os
import sys
import time
import urllib.error
import urllib.request
from pathlib import Path
from typing import Any, Dict, List, Optional, Tuple

HERE = Path(__file__).resolve().parent
TASKS_PATH = HERE / "tasks.json"
TERMINAL = {"completed", "failed", "cancelled", "interrupted", "outcome_unknown"}


class BenchError(Exception):
    pass


def load_tasks(path: Path = TASKS_PATH) -> List[Dict[str, Any]]:
    data = json.loads(path.read_text(encoding="utf-8"))
    tasks = data.get("tasks")
    if not isinstance(tasks, list) or not tasks:
        raise BenchError(f"{path} has no tasks")
    seen = set()
    for task in tasks:
        for key in ("id", "kind", "spec", "extract", "tolerance"):
            if key not in task:
                raise BenchError(f"task {task.get('id', '?')} missing {key}")
        if task["id"] in seen:
            raise BenchError(f"duplicate task id {task['id']}")
        seen.add(task["id"])
        if "target" not in task and "target_from" not in task:
            raise BenchError(f"task {task['id']} has no target")
    return tasks


def extract_path(value: Any, path: str) -> Any:
    current = value
    for part in path.split("."):
        if not isinstance(current, dict) or part not in current:
            return None
        current = current[part]
    return current


def resolve_target(task: Dict[str, Any], result: Dict[str, Any]) -> Optional[float]:
    if "target" in task:
        return float(task["target"])
    raw = extract_path(result, str(task["target_from"]))
    return float(raw) if raw is not None else None


def score_attempt(task: Dict[str, Any], result: Dict[str, Any]) -> Dict[str, Any]:
    target = resolve_target(task, result)
    measured = extract_path(result, str(task["extract"]))
    if measured is None:
        return {"pass": False, "failure": "missing_metric", "measured": None,
                "target": target, "delta": None}
    if target is None:
        return {"pass": False, "failure": "missing_target", "measured": float(measured),
                "target": None, "delta": None}
    delta = abs(float(measured) - target)
    passed = delta <= float(task["tolerance"])
    out = {"pass": passed, "failure": None if passed else "tolerance",
           "measured": float(measured), "target": target, "delta": delta}
    chemical = task.get("chemical_accuracy")
    if chemical is not None:
        out["chemical_accuracy"] = delta <= float(chemical)
    # Record the experiment's own shot uncertainty when the result carries it,
    # so a tolerance can be read against the measurement noise rather than in
    # isolation. The z-score is |measured-target| / uncertainty.
    unc = result.get("uncertainty")
    if isinstance(unc, (int, float)) and float(unc) >= 0:
        out["uncertainty"] = float(unc)
        if float(unc) > 0:
            out["z"] = delta / float(unc)
    return out


def summarize(attempts: List[Dict[str, Any]]) -> Dict[str, Any]:
    runs = len(attempts)
    passed = sum(1 for a in attempts if a.get("pass"))
    costs = [a.get("cost_usd", 0.0) for a in attempts]
    times = [a.get("wall_seconds", 0.0) for a in attempts]
    failures: Dict[str, int] = {}
    for a in attempts:
        label = a.get("failure") or "pass"
        failures[label] = failures.get(label, 0) + 1
    return {
        "runs": runs,
        "passes": passed,
        "pass_at_k": 1.0 if passed > 0 else 0.0,
        "pass_rate": (passed / runs) if runs else 0.0,
        "mean_cost_usd": (sum(costs) / runs) if runs else 0.0,
        "mean_wall_seconds": (sum(times) / runs) if runs else 0.0,
        "failure_labels": failures,
    }


class Client:
    def __init__(self, base_url: str, token: str, timeout: float = 60.0):
        self.base = base_url.rstrip("/")
        self.token = token
        self.timeout = timeout

    def call(self, method: str, path: str, body: Any = None) -> Tuple[int, Any]:
        data = None if body is None else json.dumps(body).encode()
        req = urllib.request.Request(self.base + path, data=data, method=method)
        req.add_header("Content-Type", "application/json")
        if self.token:
            req.add_header("Authorization", "Bearer " + self.token)
        try:
            with urllib.request.urlopen(req, timeout=self.timeout) as resp:
                raw = resp.read()
                return resp.status, json.loads(raw) if raw else {}
        except urllib.error.HTTPError as exc:
            raw = exc.read()
            try:
                return exc.code, json.loads(raw)
            except Exception:
                return exc.code, {"raw": raw.decode(errors="replace")}


def run_task(client: Client, task: Dict[str, Any], attempt: int,
             poll_seconds: float = 2.0, timeout_seconds: float = 600.0) -> Dict[str, Any]:
    started = time.time()
    prefix = f"bench-{task['id']}-{attempt}"
    status, session = client.call("POST", "/api/sessions", {"title": prefix})
    if status not in (200, 201) or "id" not in session:
        return {"pass": False, "failure": "session_create", "error": session,
                "wall_seconds": time.time() - started}
    sid = session["id"]

    body = {
        "session_id": sid,
        "kind": task["kind"],
        "spec": task["spec"],
        "request_id": f"{prefix}-{int(time.time() * 1000)}",
        "budget_usd": float(task.get("budget_usd", 1.0)),
    }
    status, submitted = client.call("POST", "/api/experiments", body)
    if status != 201 or "job" not in submitted:
        return {"pass": False, "failure": "submit", "error": submitted,
                "wall_seconds": time.time() - started, "session_id": sid}
    job_id = submitted["job"]["id"]

    deadline = time.time() + timeout_seconds
    job: Dict[str, Any] = {}
    while time.time() < deadline:
        status, payload = client.call("GET", f"/api/jobs/{job_id}?session_id={sid}")
        if status != 200:
            return {"pass": False, "failure": "poll", "error": payload,
                    "wall_seconds": time.time() - started, "session_id": sid, "job_id": job_id}
        job = payload.get("job") or payload
        state = str(job.get("status", "")).lower().replace("job_status_", "")
        if state in TERMINAL:
            break
        time.sleep(poll_seconds)
    else:
        return {"pass": False, "failure": "timeout", "wall_seconds": time.time() - started,
                "session_id": sid, "job_id": job_id}

    result = job.get("result_json") or {}
    if not isinstance(result, dict):
        result = {}
    scored = score_attempt(task, result)
    cost = result.get("actual_usd")
    if cost is None:
        cost = job.get("actual_usd")
    if cost is None:
        cost = job.get("reserved_amount", 0.0)
    scored.update({
        "task_id": task["id"],
        "attempt": attempt,
        "job_id": job_id,
        "session_id": sid,
        "status": job.get("status"),
        "cost_usd": float(cost or 0.0),
        "wall_seconds": time.time() - started,
    })
    if str(job.get("status", "")).lower().replace("job_status_", "") != "completed" and scored["failure"] is None:
        scored["pass"] = False
        scored["failure"] = "execution"
    return scored


def main(argv: Optional[List[str]] = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--api-url", default=os.environ.get("KUBONA_API_URL", "http://127.0.0.1:8080"))
    parser.add_argument("--token", default=os.environ.get("KUBONA_API_TOKEN", ""))
    parser.add_argument("--tasks", default=str(TASKS_PATH))
    parser.add_argument("--task", action="append", default=[], help="task id (repeatable)")
    parser.add_argument("--attempts", type=int, default=1)
    parser.add_argument("--json-out")
    parser.add_argument("--allow-failures", action="store_true",
                        help="exit 0 even when tasks miss their targets (nightly baseline)")
    parser.add_argument("--dry-run", action="store_true")
    args = parser.parse_args(argv)

    tasks = load_tasks(Path(args.tasks))
    if args.task:
        wanted = set(args.task)
        tasks = [t for t in tasks if t["id"] in wanted]
        if not tasks:
            print("no matching tasks", file=sys.stderr)
            return 2
    if args.dry_run:
        for t in tasks:
            print(f"{t['id']}: {t['description']}")
        return 0
    if not args.token and "127.0.0.1" not in args.api_url and "localhost" not in args.api_url:
        print("refusing a remote API without a token", file=sys.stderr)
        return 2

    client = Client(args.api_url, args.token)
    report: Dict[str, Any] = {"api_url": args.api_url, "attempts": args.attempts, "tasks": []}
    all_ok = True
    for task in tasks:
        attempts = [run_task(client, task, i + 1) for i in range(max(1, args.attempts))]
        summary = summarize(attempts)
        summary.update({"task_id": task["id"], "description": task["description"]})
        report["tasks"].append({"summary": summary, "attempts": attempts})
        print(f"{task['id']}: pass@k={summary['pass_at_k']:.1f} "
              f"pass_rate={summary['pass_rate']:.2f} "
              f"cost=${summary['mean_cost_usd']:.4f} "
              f"wall={summary['mean_wall_seconds']:.1f}s "
              f"failures={summary['failure_labels']}")
        if summary["passes"] == 0:
            all_ok = False

    if args.json_out:
        Path(args.json_out).write_text(json.dumps(report, indent=2), encoding="utf-8")
    return 0 if (all_ok or args.allow_failures) else 1


if __name__ == "__main__":  # pragma: no cover
    raise SystemExit(main())
