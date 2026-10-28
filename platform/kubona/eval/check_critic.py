"""Check-discrimination critic: a check that cannot fail is theater.

For every corpus task, the critic runs the task's own invariant check
against mutated copies of its reference QASM on the simulator:
  - breaking mutants (wrong state by construction) MUST be rejected;
  - preserving mutants (symmetries, permutations) MUST still be accepted.
A check passing a breaking mutant is vacuous; rejecting a preserving one
is over-strict. Both are reported per task and per check kind.

Known check weakness (recorded, not silently fixed): the `wstate` check
asserts support ({001,010,100}) but not balance — a mutant yielding
2740/1356 splits still passes. Balance assertion is future hardening;
changing the check would alter corpus semantics mid-stream.
"""

from __future__ import annotations

import re
from typing import Any, Callable

from kubona.circuits import parse_qasm2
from kubona.providers import get_provider

_TWOQ = re.compile(r"^(cx|cz|swap|cp)\b")


def drop_first_twoq(qasm: str) -> str | None:
    """Remove the first entangling gate (breaks entanglement invariants)."""
    lines = qasm.splitlines()
    for i, line in enumerate(lines):
        if _TWOQ.match(line.strip()):
            return "\n".join(lines[:i] + lines[i + 1:])
    return None


def h_to_x(qasm: str) -> str | None:
    """Rewrite the first H as X (breaks superposition invariants)."""
    out, n = re.subn(r"^h\s", "x ", qasm, count=1, flags=re.M)
    return out if n else None


def append_x0(qasm: str) -> str | None:
    """Flip qubit 0 at the end (breaks computational-basis outcomes)."""
    return qasm.rstrip() + "\n" + "x q[0];"


def drop_all_h(qasm: str) -> str | None:
    """Remove every H (kills superposition; uniform states collapse)."""
    out = "\n".join(l for l in qasm.splitlines() if not re.match(r"h\s", l.strip()))
    return out if out != qasm else None


def insert_z0(qasm: str) -> str | None:
    """Relative phase on qubit 0 (preserves measurement distributions)."""
    lines = qasm.splitlines()
    if len(lines) < 3:
        return None
    lines.insert(2, "z q[0];")
    return "\n".join(lines)


MUTANTS: dict[str, Callable[[str], str | None]] = {
    "drop_first_twoq": drop_first_twoq,
    "h_to_x": h_to_x,
    "append_x0": append_x0,
    "drop_all_h": drop_all_h,
    "insert_z0": insert_z0,
}


def run_mutant(qasm: str, check: Callable, shots: int = 4096,
               seed: int = 7, provider: str = "local-simulator") -> bool | None:
    """Run one mutant through the check. None = mutant didn't parse (skipped,
    never counted as a pass or a fail — an invalid mutant proves nothing)."""
    try:
        circuit = parse_qasm2(qasm)
    except Exception:
        return None
    sim = get_provider(provider)
    counts = sim.submit(circuit, shots, seed=seed)["counts"]
    return bool(check(counts, shots))


def criticize_task(task_name: str, qasm: str, check: Callable,
                   expectations: dict[str, bool],
                   shots: int = 4096) -> dict[str, Any]:
    """Grade one task: every named mutant must meet its expectation.

    expectations maps mutant name -> True (preserving, must pass) or False
    (breaking, must be rejected). Returns per-mutant verdicts + ok flag.
    """
    verdicts = {}
    for name, expected in expectations.items():
        mutated = MUTANTS[name](qasm)
        if mutated is None:
            verdicts[name] = {"applicable": False}
            continue
        got = run_mutant(mutated, check, shots)
        if got is None:
            verdicts[name] = {"applicable": False}
            continue
        verdicts[name] = {"applicable": True, "expected": expected,
                          "got": got, "ok": got == expected}
    applicable = [v for v in verdicts.values() if v.get("applicable")]
    # Fail closed: no mutants/no applicable mutant means no evidence, not a pass.
    ok = bool(applicable) and all(v.get("ok") is True for v in applicable)
    return {"task": task_name, "ok": ok, "verdicts": verdicts}
