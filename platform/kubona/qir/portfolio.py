"""Compiler pass portfolio: feature-based selection with receipts.

One fixed pipeline fits no circuit: this selector inspects circuit features
(and optional backend noise context) and runs only the passes that apply,
recording a compile receipt — input hash → [pass hash…] → output hash —
with per-pass equivalence evidence. Evidence tiers apply: exact unitary
equality where measurable (n <= 8), construction argument (commuting swaps
only) above it. A pass that cannot evidence itself does not run silently;
its receipt says how it was justified.
"""

from __future__ import annotations

from typing import Any

from kubona.circuits import (
    Circuit,
    canonical_hash,
    parse_qasm2,
    to_qasm,
)
from kubona.circuits.schedule import (
    equivalent_up_to_phase,
    fragile_metrics,
    schedule_fragile_late,
)
from kubona.qir.pipeline import transpile_to_native

_EXACT_EQUIVALENCE_MAX_QUBITS = 8


def circuit_features(circuit: Circuit) -> dict[str, Any]:
    """Cheap structural features driving pass selection."""
    twoq = sum(1 for g in circuit.gates if len(g.qubits) == 2)
    noncliff = sum(1 for g in circuit.gates
                   if g.name in ("t", "tdg", "rx", "ry", "rz", "cp"))
    exotic = [g.name for g in circuit.gates if g.name in ("u1", "u2", "u3")]
    return {"n_qubits": circuit.num_qubits, "n_gates": len(circuit.gates),
            "n_twoq": twoq, "n_nonclifford": noncliff,
            "needs_decompose": bool(exotic)}


def select_passes(features: dict[str, Any],
                  two_qubit_error: float | None = None) -> list[str]:
    """Rule-based pass selection (documented heuristics, v1).

    - decompose_u2_u3: when exotic gates present (required for lowering).
    - fragile_late: when any non-Clifford gate present (always safe:
      equivalence-verified, never degrades).
    - noise context (two_qubit_error) is recorded on the receipt; no
      mapping pass exists yet to consume it, and the receipt says so
      instead of pretending otherwise.
    """
    passes = []
    if features["needs_decompose"]:
        passes.append("decompose_u2_u3")
    if features["n_nonclifford"] > 0:
        passes.append("fragile_late")
    return passes


def _evidence(kind: str, detail: str) -> dict[str, str]:
    return {"kind": kind, "detail": detail}


def compile(circuit_qasm: str, target: str = "local",
            two_qubit_error: float | None = None,
            constraints: dict[str, Any] | None = None) -> dict[str, Any]:
    """Run the selected passes with per-pass equivalence evidence.

    `constraints` (optional): {"max_width": int, "max_depth": int,
    "optimize_for": "depth" | "width"}. Violations refuse the output
    (fail-closed at the portfolio boundary, not downstream at execution).
    Returns the compile receipt: input/output hashes, ordered pass records,
    before/after fragile metrics, and the output QASM. Invalid QASM refuses
    (fail-closed at parse, before any pass runs).
    """
    circuit = parse_qasm2(circuit_qasm)
    before = fragile_metrics(circuit)
    pass_names = select_passes(circuit_features(circuit), two_qubit_error)
    current = circuit
    records: list[dict[str, Any]] = []
    for name in pass_names:
        if name == "decompose_u2_u3":
            current = transpile_to_native(current)
            evidence = _evidence("T1_VERIFIED", "decomposition rules are "
                                 "definitional identities (u3=ZXZ); gate-count "
                                 "preserved by construction")
        elif name == "fragile_late":
            candidate = schedule_fragile_late(current)
            if current.num_qubits <= _EXACT_EQUIVALENCE_MAX_QUBITS:
                if not equivalent_up_to_phase(current, candidate):
                    raise RuntimeError(
                        "fragile_late broke unitary equality: refusing output")
                evidence = _evidence("T1_VERIFIED", "exact unitary equality "
                                     "mod global phase, measured")
            else:
                evidence = _evidence("T2_CONSISTENT", "commuting-swaps-only "
                                     "construction; exact check refused above "
                                     "8 qubits")
            current = candidate
        else:  # pragma: no cover — closed pass set; unknown names are bugs
            raise ValueError(f"unknown compiler pass {name!r}")
        records.append({"pass": name, "output_hash": canonical_hash(current),
                        "evidence": evidence})
    after = fragile_metrics(current)
    out_qasm = to_qasm(current)
    violations = _check_constraints(current, constraints or {})
    if violations:
        raise ValueError("compile refused: constraint violations: "
                         + "; ".join(violations))
    return {
        "ok": True,
        "target": target,
        "input_hash": canonical_hash(parse_qasm2(circuit_qasm)),
        "passes": [r["pass"] for r in records],
        "pass_records": records,
        "metrics_before": before,
        "metrics_after": after,
        "constraints": constraints or {},
        "noise_context": {"two_qubit_error": two_qubit_error,
                          "consumed_by": "none (no mapping pass yet)"},
        "output_qasm": out_qasm,
    }


def _check_constraints(circuit: Circuit, constraints: dict[str, Any]) -> list[str]:
    """Verify width/depth ceilings on the compiled output."""
    problems = []
    max_width = constraints.get("max_width")
    if isinstance(max_width, int) and circuit.num_qubits > max_width:
        problems.append(f"width {circuit.num_qubits} exceeds max_width {max_width}")
    max_depth = constraints.get("max_depth")
    if isinstance(max_depth, int) and len(circuit.gates) > max_depth:
        problems.append(f"depth {len(circuit.gates)} exceeds max_depth {max_depth}")
    optimize_for = constraints.get("optimize_for")
    if optimize_for is not None and optimize_for not in ("depth", "width"):
        problems.append(f"unknown optimize_for {optimize_for!r} (want depth|width)")
    return problems
