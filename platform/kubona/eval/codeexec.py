"""Sandboxed code-execution harness for code corpora (HumanEval/PennyLang).

The {qasm, check} registry cannot hold *code* benchmarks without
misrepresenting them, so code tasks execute here: untrusted generated
Python runs containerized and its tests of record decide pass/fail.

Task contract (self-contained, no runner magic):
  task = {"name": str, "solution": str, "tests": str}
  - `solution` is written as solution.py (the generated completion).
  - `tests` is written as test_main.py: a plain assert script that imports
    solution.py, exit 0 = pass, nonzero = fail. HumanEval-style
    `check(candidate)` bodies wrap with a trailing `check(solution.fn)`.
  - No network, no pip, no host fallback. No daemon/image -> skip, never pass.

Security discipline mirrors the Q-31 Lean sandbox (comparator_eval.py):
pinned image, --read-only, --network none, --cap-drop ALL, resource caps,
timeout, no implicit pulls, infra-failure vs test-failure distinguished,
one ledger record per task carrying image digest + content hashes.
"""

from __future__ import annotations

import hashlib
import shutil
import subprocess
import tempfile
from dataclasses import dataclass
from pathlib import Path
from typing import Any, Callable, Optional

from kubona.eval.comparator_eval import (
    _is_infra_failure,
    docker_available,
    docker_image_present,
)
from kubona.provenance import Ledger


@dataclass(frozen=True)
class PyexecSandboxConfig:
    """Resource caps for containerized code execution.

    The image is platform/docker/Dockerfile.quantum-runner (qiskit +
    pennylane pinned at build time, unprivileged `sandbox` user); these
    caps apply at every `docker run` so one completion cannot exhaust the
    host. Network is always none at runtime; dependencies must be baked
    at image-build time.
    """

    image: str = "kubona-quantum-runner:1.1.0"
    memory: str = "2g"
    memory_swap: str = "2g"
    cpus: str = "1.0"
    pids_limit: int = 128
    timeout_seconds: int = 300


def sandbox_docker_cmd(cfg: PyexecSandboxConfig, task_dir: Path) -> list[str]:
    """The exact container invocation (unit-tested flag-for-flag)."""
    return [
        "docker", "run", "--rm",
        "--read-only",
        "--network", "none",
        "--cap-drop", "ALL",
        "--memory", cfg.memory,
        "--memory-swap", cfg.memory_swap,
        "--cpus", cfg.cpus,
        "--pids-limit", str(cfg.pids_limit),
        "--tmpfs", "/tmp",
        "-v", f"{task_dir}:/workspace:rw",
        "-w", "/workspace",
        # --entrypoint pins the interpreter explicitly: image ENTRYPOINTs
        # vary (quantum-runner's is ["python3"]) and an inherited one would
        # silently reinterpret our command (observed: `python3 python ...`).
        "--entrypoint", "python3",
        cfg.image,
        "test_main.py",
    ]


def _sha256(text: str) -> str:
    return hashlib.sha256(text.encode()).hexdigest()


def _static_check(solution: str, tests: str,
                  requires: list[str]) -> str | None:
    """Parse + closed-world import check. Returns None when clean, else the
    refusal reason. Undeclared tasks (no `requires`) skip the import half:
    open-world code cannot be judged without a declared boundary."""
    import ast
    import sys as _sys

    for label, code in (("solution.py", solution), ("test_main.py", tests)):
        try:
            tree = ast.parse(code)
        except SyntaxError as e:
            return f"{label} does not parse: {e}"
        if not requires:
            continue
        allowed = set(_sys.stdlib_module_names) | set(requires) | {"solution"}
        for node in ast.walk(tree):
            if isinstance(node, ast.Import):
                mods = [a.name.split(".")[0] for a in node.names]
            elif isinstance(node, ast.ImportFrom):
                if node.level and node.level > 0:
                    return f"{label}: relative imports refused"
                mods = [(node.module or "").split(".")[0]]
            else:
                continue
            unknown = [m for m in mods if m and m not in allowed]
            if unknown:
                return (f"{label}: imports {unknown} outside declared "
                        f"requires {sorted(requires)} (+ stdlib)")
    return None


def _probe_missing_deps(task_dir: Path, requires: list[str], cfg: PyexecSandboxConfig,
                        runner: Callable[..., Any]) -> list[str]:
    """Ask the container which declared deps are missing (one probe run).

    Returns the missing subset; empty means all present. A probe that
    cannot run (infra) yields [] and lets the real run surface the truth —
    the probe is an optimization with named skips, never a gate that can
    manufacture a pass.
    """
    if not requires:
        return []
    probe = ("import json, importlib\nmissing = []\n"
             + "\n".join(f"try:\n    importlib.import_module({r!r})\n"
                         f"except Exception:\n    missing.append({r!r})"
                         for r in requires)
             + "\nprint(json.dumps(missing))")
    (task_dir / "_probe.py").write_text(probe)
    cmd = sandbox_docker_cmd(cfg, task_dir)[:-1] + ["_probe.py"]
    try:
        proc = runner(cmd, capture_output=True, text=True,
                      timeout=min(120, cfg.timeout_seconds))
    except Exception:
        return []
    if proc.returncode != 0 or _is_infra_failure(proc.stderr or ""):
        return []
    try:
        import json as _json
        missing = _json.loads((proc.stdout or "").strip().splitlines()[-1])
        return [m for m in missing if isinstance(m, str)]
    except Exception:
        return []


def run_codeexec_task(task: dict[str, str],
                      cfg: PyexecSandboxConfig | None = None,
                      ledger: Ledger | None = None,
                      runner: Callable[..., Any] | None = None,
                      work_root: Path | None = None) -> dict[str, Any]:
    """Execute one code task containerized; return the verdict record.

    `runner` injects the subprocess call (unit tests prove verdict parsing
    without a daemon). Returns {name, ok, skip, ...}: ok=True only when the
    container ran the tests to exit 0. Infra breakdowns and missing
    daemon/image yield skip=True, never ok=True.
    """
    cfg = cfg or PyexecSandboxConfig()
    ledger = ledger or Ledger()
    name = task.get("name", "?")
    solution = task.get("solution", "")
    tests = task.get("tests", "")
    requires = task.get("requires") or []
    record_base = {
        "name": name,
        "image": cfg.image,
        "solution_sha256": _sha256(solution),
        "tests_sha256": _sha256(tests),
    }

    # Static gate first (no daemon, no container): both files must parse,
    # and when the task declares `requires`, imports must stay inside the
    # closed world of stdlib + declared deps + the solution module itself.
    # Malformed tasks refuse here instead of spending a container on them.
    static_err = _static_check(solution, tests, requires)
    if static_err is not None:
        rec = {**record_base, "ok": False, "skip": False,
               "reason": f"static gate refused: {static_err}"}
        ledger.append("codeexec", rec)
        return rec

    if runner is None:
        if not docker_available():
            rec = {**record_base, "ok": False, "skip": True,
                   "reason": "no docker daemon: refusing host-side execution"}
            ledger.append("codeexec", rec)
            return rec
        if not docker_image_present(cfg.image):
            rec = {**record_base, "ok": False, "skip": True,
                   "reason": f"pinned image {cfg.image} absent: refusing implicit pull"}
            ledger.append("codeexec", rec)
            return rec
        runner = subprocess.run

    tmp_root = work_root or Path(tempfile.gettempdir())
    task_dir = Path(tempfile.mkdtemp(prefix="kubona-codeexec-", dir=tmp_root))
    try:
        (task_dir / "solution.py").write_text(solution)
        (task_dir / "test_main.py").write_text(tests)
        # Dependency pre-flight: declared-but-missing deps skip with the
        # missing set named (the precise shopping list for the next image
        # revision) instead of dying as ModuleNotFoundError mid-run.
        if requires:
            missing = _probe_missing_deps(task_dir, requires, cfg, runner)
            if missing:
                rec = {**record_base, "ok": False, "skip": True,
                       "reason": f"missing optional deps in {cfg.image}: {missing}",
                       "missing_dependencies": missing}
                ledger.append("codeexec", rec)
                return rec
        cmd = sandbox_docker_cmd(cfg, task_dir)
        try:
            proc = runner(cmd, capture_output=True, text=True,
                          timeout=cfg.timeout_seconds)
        except subprocess.TimeoutExpired:
            rec = {**record_base, "ok": False, "skip": False,
                   "reason": f"timeout after {cfg.timeout_seconds}s: killed, not passed"}
            ledger.append("codeexec", rec)
            return rec
        except (FileNotFoundError, PermissionError, OSError) as e:
            rec = {**record_base, "ok": False, "skip": True,
                   "reason": f"container infra failure: {e}"}
            ledger.append("codeexec", rec)
            return rec
        stdout, stderr, code = (proc.stdout or "", proc.stderr or "",
                                proc.returncode)
        if code == 0:
            rec = {**record_base, "ok": True, "skip": False,
                   "stdout_tail": stdout[-2000:]}
        elif _is_infra_failure(stderr):
            rec = {**record_base, "ok": False, "skip": True,
                   "reason": f"container infra failure (not a test verdict): {stderr[-500:]}",
                   "stdout_tail": stdout[-2000:]}
        else:
            rec = {**record_base, "ok": False, "skip": False,
                   "reason": "tests failed inside sandbox",
                   "stdout_tail": stdout[-2000:],
                   "stderr_tail": stderr[-2000:]}
        ledger.append("codeexec", rec)
        return rec
    finally:
        shutil.rmtree(task_dir, ignore_errors=True)


def run_codeexec_suite(tasks: list[dict[str, str]],
                       cfg: PyexecSandboxConfig | None = None,
                       ledger: Ledger | None = None,
                       runner: Callable[..., Any] | None = None) -> dict[str, Any]:
    """Run every task; the suite verdict. An all-skip suite is NOT a pass."""
    ledger = ledger or Ledger()
    results = [run_codeexec_task(t, cfg, ledger, runner) for t in tasks]
    ran = [r for r in results if not r.get("skip")]
    failures = [r["name"] for r in ran if not r["ok"]]
    return {
        "ok": bool(ran) and not failures,
        "ran": len(ran),
        "skipped": len(results) - len(ran),
        "failures": failures,
        "results": results,
    }
