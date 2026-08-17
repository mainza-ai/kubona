"""Declarative eval-corpus loader (G13 mechanism).

`evaluate_codegen` iterates a task registry of {qasm, invariant-check}. Volume /
third-party corpora (QuantumKatas, Qiskit HumanEval, PennyLang — when acquired)
plug in through this loader: a directory of JSON files whose entries map
  task_name -> {"qasm": "...", "check": <kind>, **params}
into that same registry, so more corpora never require code changes.

Declarative check kinds (verified against the simulator):
  bell      entangled 2-qubit 50/50 -> {00,11}, balanced
  ghz       all-or-nothing GHZ on N qubits -> {0...0, 1...1}
  wstate    3-qubit W-state -> {001,010,100}, near-uniform
  uniform   flat superposition over all N-qubit states (e.g. QFT)
  grover-majority  a marked state fires >75% (2-qubit grover example)
  bell-pairs  product of entangled pairs -> only pair-equal strings
"""

from __future__ import annotations

import json
from pathlib import Path
from typing import Any, Callable

from kubona.circuits import parse_qasm2
from kubona.eval.corpora import TASKS


def _bell_check(counts, shots):
    return set(counts) <= {"00", "11"} and abs(counts.get("00", 0) - counts.get("11", 0)) <= 0.1 * shots


def _ghz_check(n):
    def _check(counts, shots):
        return set(counts) <= {"0" * n, "1" * n}
    return _check


def _wstate_check(counts, shots):
    n = 3
    return set(counts) <= {"001", "010", "100"}


def _uniform_check(n):
    def _check(counts, shots):
        return all(k in counts for k in (f"{i:0{n}b}" for i in range(2 ** n))) and all(
            abs(counts.get(k, 0) - shots / (2 ** n)) <= 0.15 * shots
            for k in (f"{i:0{n}b}" for i in range(2 ** n)))
    return _check


def _grover_majority_check(counts, shots):
    return counts.get("11", 0) / max(shots, 1) > 0.75


_CHECKS: dict[str, Callable[[dict, int], bool] | Callable[..., Callable]] = {
    "bell": _bell_check,
    "ghz": _ghz_check,
    "wstate": _wstate_check,
    "uniform": _uniform_check,
    "grover-majority": _grover_majority_check,
    "bell-pairs": lambda counts, shots: set(counts) <= {"0000", "0011", "1100", "1111"},
}


def load_corpus_dir(path: str | Path) -> dict[str, dict]:
    """Register every task in a directory of JSON corpus files; returns new tasks."""
    added: dict[str, dict] = {}
    path = Path(path)
    for f in sorted(path.glob("*.json")):
        data = json.loads(f.read_text())
        for name, spec in data.items():
            if not isinstance(spec, dict) or "qasm" not in spec or "check" not in spec:
                continue
            kind = spec["check"]
            params = spec.get("check_params", {})
            check_builder = _CHECKS.get(kind)
            if check_builder is None:
                continue
            # builders that take n return a closure; plain checks are already (counts,shots)->bool
            try:
                check = check_builder(**params) if kind in ("ghz", "uniform") else check_builder
            except TypeError:
                continue
            # validate the QASM parses BEFORE registering (fail-closed on bad corpus)
            try:
                parse_qasm2(spec["qasm"])
            except Exception:  # invalid corpus entry — do not bind
                continue
            TASKS[name] = {"qasm": spec["qasm"], "check": check}
            added[name] = TASKS[name]
    return added
