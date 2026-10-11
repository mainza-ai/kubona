"""Code-task corpus loader: HumanEval-style {prompt, tests, reference} tasks.

Complements corpus_loader.py (QASM satisfiability): code tasks execute via
the sandboxed codeexec harness, never the simulator registry. The same Q-26
licence rule applies per file: Apache-2.0/MIT/BSD/CC0/Unlicense/in-repo
only, else the whole file refuses.

Task contract per entry:
  {"prompt": str, "tests": str, "reference": str,
   "entry_point": str, "difficulty": str, "task_id": str,
   "requires": [import names] (optional)}
`requires` declares third-party imports the task needs beyond the base
image (qiskit/aer/pennylane/scipy/numpy). The harness pre-flights them
inside the container: missing deps skip with a named reason instead of a
cryptic ModuleNotFoundError, and the missing set becomes the precise
shopping list for the next image revision.
`wrap_for_harness(entry)` renders the codeexec {solution, tests} pair:
solution.py = prompt + reference body; test_main.py = prompt imports +
test body + `check(solution.<entry_point>)` invocation.
"""

from __future__ import annotations

import json
import re
from pathlib import Path
from typing import Any

from kubona.eval.corpus_loader import license_admissible, provenance_declared

_REQUIRED = ("prompt", "tests", "reference", "entry_point")


def _prompt_imports(prompt: str) -> str:
    return "\n".join(
        ln for ln in prompt.splitlines()
        if re.match(r"^(?:import|from)\s+\S", ln))


def wrap_for_harness(entry: dict[str, Any]) -> dict[str, str]:
    """Render a corpus entry into the codeexec {solution, tests} pair."""
    ep = entry["entry_point"]
    solution = entry["prompt"].rstrip() + "\n" + entry["reference"].lstrip("\n")
    if not solution.endswith("\n"):
        solution += "\n"
    tests = (_prompt_imports(entry["prompt"]) + "\n"
             + f"from solution import {ep}\n"
             + entry["tests"].rstrip() + "\n"
             + f"check({ep})\n")
    pair: dict[str, str] = {"solution": solution, "tests": tests}
    if entry.get("requires"):
        pair["requires"] = list(entry["requires"])
    return pair


def load_code_tasks(path: str | Path) -> dict[str, dict]:
    """Load one code-task corpus file (licence-gated, fail-closed)."""
    path = Path(path)
    data = json.loads(path.read_text())
    meta = data.get("_meta", {}) if isinstance(data, dict) else {}
    if not license_admissible(meta.get("license") if isinstance(meta, dict) else None):
        raise ValueError(f"code-task corpus {path.name} refused: licence "
                         f"{(meta.get('license') if isinstance(meta, dict) else None)!r} "
                         "not admissible (Q-26)")
    if not provenance_declared(meta if isinstance(meta, dict) else None):
        raise ValueError(f"code-task corpus {path.name} refused: provenance "
                         "undeclared (quarantine rule)")
    tasks = {}
    for name, spec in data.items():
        if name.startswith("_") or not isinstance(spec, dict):
            continue
        if any(k not in spec for k in _REQUIRED):
            continue
        if "requires" in spec and (
                not isinstance(spec["requires"], list)
                or not all(isinstance(r, str) for r in spec["requires"])):
            continue  # malformed dependency declaration: not loadable
        tasks[name] = spec
    return tasks
