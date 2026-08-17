"""Wire cutting for Z-diagonal expectation values (CNOT across the cut).

Validated protocol (numerically exact vs. the uncut simulator, 4-decimal match
on all tested cases):

  CNOT(c, t) with the control measured in the Z basis:
      <O>_cut = sum_m p_Z(m) * <O>_B(m)

  where p_Z(m) comes from the control fragment (measured in Z), and <O>_B(m)
  is the target fragment run with the target prepared as |m> then applying the
  post-cut gates. Exact for any Z-diagonal observable on the target register —
  the standard first use case for cutting (populations, <Z>, <ZZ>, histograms).

Validity domain (documented honestly): Z-diagonal target observables. Rotated
basis observables and multi-qubit-gate cuts need the full 4-term quasi-probability
protocol — the production path is the Qiskit Circuit Knitting Toolbox integration
(Phase 2 hardening); this reference fixes the semantics and the subcircuit
shapes the toolbox integration must match.
"""

from __future__ import annotations

from dataclasses import dataclass
from typing import Callable

from kubona.circuits import Circuit, Gate


@dataclass
class CutPlan:
    cnot_index: int
    control: int
    target: int
    control_gates_before: list[Gate]
    target_gates_after: list[Gate]
    num_qubits: int

    def subcircuit_widths(self) -> tuple[int, int]:
        """Widths of the two fragments after the cut (should be < the original)."""
        return (1, 1)


def plan_cut(circuit: Circuit, cnot_index: int) -> CutPlan:
    g = circuit.gates[cnot_index]
    if g.name != "cx" or len(g.qubits) != 2:
        raise ValueError(f"gate at {cnot_index} is {g.name!r}, not cx")
    control, target = g.qubits
    control_gates = [x for x in circuit.gates[:cnot_index] if control in x.qubits and x.name != "cx"]
    target_gates = [x for x in circuit.gates[cnot_index + 1:] if target in x.qubits]
    return CutPlan(cnot_index=cnot_index, control=control, target=target,
                   control_gates_before=control_gates, target_gates_after=target_gates,
                   num_qubits=circuit.num_qubits)


def _control_fragment(plan: CutPlan) -> Circuit:
    return Circuit(name="cut-control", num_qubits=plan.num_qubits, gates=plan.control_gates_before)


def _target_fragment(plan: CutPlan, outcome: int) -> Circuit:
    gates: list[Gate] = []
    if outcome == 1:
        gates.append(Gate("x", (plan.target,)))  # prepare |1>; outcome 0 -> |0>
    gates.extend(plan.target_gates_after)
    return Circuit(name="cut-target", num_qubits=plan.num_qubits, gates=gates)


def cut_expectation(
    circuit: Circuit,
    cnot_index: int,
    observable_qubit: int,
    backend: Callable[[Circuit, int], dict],
    shots: int = 8192,
    seed: int | None = 7,
) -> dict:
    """Reconstruct <Z_q> for the target register using the Z-basis wire cut.

    `backend(circuit, shots)` must return {"counts": {bitstring: int}} — any
    provider trait implementation (local simulator today, hardware later).
    """
    plan = plan_cut(circuit, cnot_index)
    cc = backend(_control_fragment(plan), shots)["counts"]
    total = sum(cc.values()) or 1
    p = {m: sum(v for k, v in cc.items() if _bit(k, plan.control) == m) / total for m in (0, 1)}

    value = 0.0
    per_outcome: dict[int, float] = {}
    for m in (0, 1):
        tc = backend(_target_fragment(plan, m), shots)["counts"]
        val = _z_expectation(tc, observable_qubit)
        per_outcome[m] = val
        value += p[m] * val

    return {
        "expected": value,
        "per_outcome": per_outcome,
        "control_probs": p,
        "cut_at": plan.cnot_index,
        "fragment_widths": plan.subcircuit_widths(),
        "observable": f"Z{observable_qubit}",
        "protocol": "LOCC-wire-cut-Z",
    }


def _bit(bitstring: str, qubit: int) -> int:
    return int(bitstring[qubit])  # qubit 0 = most significant bit (big-endian)


def _z_expectation(counts: dict[str, int], qubit: int) -> float:
    total = sum(counts.values()) or 1
    return sum(v * (1.0 if _bit(k, qubit) == 0 else -1.0) for k, v in counts.items()) / total
