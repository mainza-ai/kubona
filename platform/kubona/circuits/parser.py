"""Tiny OpenQASM 2.0 subset parser (for the Phase-0 skeleton).

Supports: qreg, u1/u2/u3, and the gate set the local simulator implements.
Structural directives (include, creg, measure, barrier, reset, opaque gate
definitions) are skipped by design: measurement is a backend concern
(simulators append measure_all; hardware measures natively), and classical
registers carry no quantum semantics. Unknown *gates* raise a clear error ->
the compiler server surfaces it as guidance.
"""

from __future__ import annotations

import re

from .model import Circuit, Gate

_GATE_RE = re.compile(r"^(?P<name>[a-zA-Z_][a-zA-Z0-9_]*)\s*(?:\((?P<params>[^)]*)\))?\s*(?P<qubits>[^;]+);")


def parse_qasm2(source: str) -> Circuit:
    lines = [ln.split("//")[0].strip() for ln in source.splitlines() if ln.strip()]
    num_qubits = 0
    gates: list[Gate] = []
    name = "untitled"
    for ln in lines:
        if ln.startswith("OPENQASM"):
            continue
        first = ln.split()[0].rstrip(";") if ln.split() else ""
        if first in ("include", "creg", "measure", "barrier", "reset", "opaque"):
            continue
        if ln.startswith("qreg"):
            m = re.match(r"qreg\s+\w+\[(\d+)\];", ln)
            if m:
                num_qubits = int(m.group(1))
            continue
        if ln.startswith("//") or not ln:
            continue
        m = _GATE_RE.match(ln)
        if not m:
            raise ValueError(f"unsupported QASM statement: {ln!r}")
        gname = m.group("name")
        params = [float(p.strip()) for p in m.group("params").split(",")] if m.group("params") else []
        qs = tuple(int(q) for q in re.findall(r"\[(\d+)\]", m.group("qubits")))
        gates.append(Gate(name=gname, qubits=qs, params=tuple(params)))
    if num_qubits == 0 and gates:
        num_qubits = max(max(g.qubits) for g in gates) + 1
    return Circuit(name=name, num_qubits=num_qubits, gates=gates)
