"""Tiny OpenQASM 2.0 subset parser (for the Phase-0 skeleton).

Supports: qreg, u1/u2/u3, and the gate set the local simulator implements.
Structural directives (include, creg, barrier, reset, opaque gate definitions)
are NOT silently dropped: every skipped directive is recorded on
``Circuit.skipped_directives`` so callers can audit what was ignored.
Measurement (``measure``) is a backend concern for execution, but at parse
time it is recorded like any other skipped directive — use
``refuse_measure=True`` when the caller must guarantee a unitary-only circuit
(e.g. the compiler passes, which prove unitary equality).

Unknown *gates* raise a clear error -> the compiler server surfaces it as
guidance. Unknown gate arity / out-of-range qubit indices refuse fail-closed.
"""

from __future__ import annotations

import hashlib
import re

from .model import Circuit, Gate, GATE_ARITY

_GATE_RE = re.compile(r"^(?P<name>[a-zA-Z_][a-zA-Z0-9_]*)\s*(?:\((?P<params>[^)]*)\))?\s*(?P<qubits>[^;]+);")

#: Structural directives that carry no quantum-gate semantics for this parser.
SKIPPED_DIRECTIVES = frozenset(
    {"include", "creg", "measure", "barrier", "reset", "opaque"}
)


def _derive_name(source: str, explicit: str | None, qreg_names: list[str]) -> str:
    """Circuit name: explicit param wins, else first qreg register, else a
    content-derived label. Never a hardcoded placeholder: the old code named
    every parsed circuit ``"untitled"``, aliasing distinct circuits in logs
    and receipts."""
    if explicit:
        return explicit
    if qreg_names:
        return qreg_names[0]
    digest = hashlib.sha256(source.encode()).hexdigest()[:8]
    return f"qasm-{digest}"


def parse_qasm2(source: str, name: str | None = None,
                refuse_measure: bool = False) -> Circuit:
    """Parse an OpenQASM 2.0 subset string into a :class:`Circuit`.

    Args:
        source: QASM 2.0 text.
        name: circuit name override. When omitted the name is derived from
            the first ``qreg`` register, else from the source content hash.
        refuse_measure: when True, a ``measure`` statement raises instead of
            being recorded in ``skipped_directives`` (compiler passes use
            this: they prove unitary equality, which measurement breaks).
    """
    lines = [ln.split("//")[0].strip() for ln in source.splitlines() if ln.strip()]
    num_qubits = 0
    num_classical = 0
    gates: list[Gate] = []
    skipped: list[str] = []
    qreg_names: list[str] = []
    qreg_offsets: dict[str, int] = {}
    qreg_sizes: dict[str, int] = {}
    in_opaque = False

    for ln in lines:
        if ln.startswith("OPENQASM"):
            continue
        if in_opaque:
            # An opaque gate body ends at endopaque (possibly "} endopaque"?);
            # skip everything until the terminator instead of dropping one line.
            if "endopaque" in ln:
                in_opaque = False
                skipped.append("opaque-block")
            continue
        first = ln.split()[0].rstrip(";") if ln.split() else ""
        if first in SKIPPED_DIRECTIVES:
            if first == "measure" and refuse_measure:
                raise ValueError(
                    f"measure statement refused (unitary-only context): {ln!r}"
                )
            if first == "opaque":
                # Skip the whole opaque definition through its endopaque
                # terminator (single-line "opaque ... endopaque" included).
                if "endopaque" not in ln:
                    in_opaque = True
                skipped.append(ln)
                continue
            if first == "creg":
                m = re.match(r"creg\s+\w+\[(\d+)\];", ln)
                if m:
                    num_classical += int(m.group(1))
            skipped.append(ln)
            continue
        if ln.startswith("qreg"):
            m = re.match(r"qreg\s+(\w+)\[(\d+)\];", ln)
            if not m:
                raise ValueError(f"malformed qreg statement: {ln!r}")
            rname = m.group(1)
            rsize = int(m.group(2))
            if rsize <= 0:
                raise ValueError(f"qreg width must be positive: {ln!r}")
            if rname in qreg_offsets:
                raise ValueError(f"duplicate qreg declaration: {rname!r}")
            # Widths ACCUMULATE: several qregs declare several registers.
            # The old code overwrote, silently shrinking multi-register circuits.
            qreg_offsets[rname] = num_qubits
            qreg_sizes[rname] = rsize
            qreg_names.append(rname)
            num_qubits += rsize
            continue
        if ln.startswith("//") or not ln:
            continue
        m = _GATE_RE.match(ln)
        if not m:
            raise ValueError(f"unsupported QASM statement: {ln!r}")
        gname = m.group("name")
        if gname not in GATE_ARITY:
            raise ValueError(
                f"unknown gate {gname!r} in statement {ln!r}; "
                f"supported: {sorted(GATE_ARITY)}"
            )
        raw_params = m.group("params")
        try:
            params = [float(p.strip()) for p in raw_params.split(",")] if raw_params else []
        except ValueError:
            raise ValueError(f"non-numeric gate parameter in statement: {ln!r}")
        tokens = re.findall(r"([a-zA-Z_][a-zA-Z0-9_]*)\[(\d+)\]", m.group("qubits"))
        if tokens:
            qs_list = []
            for reg, idx_str in tokens:
                idx = int(idx_str)
                if reg in qreg_offsets:
                    if idx >= qreg_sizes[reg]:
                        raise ValueError(
                            f"qubit index {idx} out of range for register {reg}[{qreg_sizes[reg]}] "
                            f"(beyond circuit width) in {ln!r}"
                        )
                    qs_list.append(qreg_offsets[reg] + idx)
                else:
                    qs_list.append(idx)
            qs = tuple(qs_list)
        else:
            qs = tuple(int(q) for q in re.findall(r"\[(\d+)\]", m.group("qubits")))
        arity, n_params = GATE_ARITY[gname]
        if len(qs) != arity:
            raise ValueError(
                f"gate {gname!r} needs {arity} qubit(s), got {len(qs)} in {ln!r}"
            )
        if n_params is not None and len(params) != n_params:
            raise ValueError(
                f"gate {gname!r} needs {n_params} param(s), got {len(params)} in {ln!r}"
            )
        gates.append(Gate(name=gname, qubits=qs, params=tuple(params)))
    if num_qubits == 0 and gates:
        num_qubits = max(max(g.qubits) for g in gates) + 1
    if num_qubits == 0 and not gates:
        raise ValueError("empty QASM: no qreg and no gates, circuit width unknown")
    circuit_name = _derive_name(source, name, qreg_names)
    # Circuit cross-validates gate indices vs width (fail-closed here, not downstream).
    return Circuit(name=circuit_name, num_qubits=num_qubits, gates=gates,
                   num_classical=num_classical, skipped_directives=skipped)
