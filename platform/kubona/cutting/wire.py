"""Wire cutting for Z-diagonal expectation values (CNOT across the cut).

Validated protocol (numerically exact vs. the uncut simulator — see
platform/tests/test_cutting.py::test_z_cut_matches_uncut_asym and
::test_z_cut_with_post_cut_target_gate, 4-decimal match anchors):

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

from dataclasses import dataclass, field
from typing import Callable

import numpy as np

from kubona.circuits import Circuit, Gate


#: Backend protocol: backend(circuit, shots, seed) -> {"counts": {bitstring: int}}.
#: The seed is MANDATORY — cut runs thread derived child seeds into every
#: fragment execution, so a "seeded" cut never contains an unseeded draw.
BackendFn = Callable[[Circuit, int, int], dict]


@dataclass
class CutPlan:
    cnot_index: int
    control: int
    target: int
    control_gates_before: list[Gate]
    target_gates_after: list[Gate]
    num_qubits: int
    dropped_post_control_gates: list[Gate] = field(default_factory=list)

    def subcircuit_widths(self) -> tuple[int, int]:
        """Widths of the two fragments after the cut, COMPUTED from the
        fragments (distinct qubits touched, including the cut wire itself).

        The old code returned a hardcoded (1, 1) — correct for the tested
        Bell-like cases, fiction for anything with spectator entanglement.
        """
        control_qubits = {self.control}
        for g in self.control_gates_before:
            control_qubits.update(g.qubits)
        target_qubits = {self.target}
        for g in self.target_gates_after:
            target_qubits.update(g.qubits)
        # An |m>-preparation X on the target touches the target (counted above).
        return (len(control_qubits), len(target_qubits))


def plan_cut(circuit: Circuit, cnot_index: int) -> CutPlan:
    """Plans the LOCC Z-wire cut at the CNOT at ``cnot_index``.

    Partition discipline (fail-closed):
    - the indexed gate must be a 2-qubit cx;
    - any OTHER two-qubit gate touching the control or target wire refuses:
      it spans the fragments and the old code SILENTLY DROPPED it (the
      ``x.name != "cx"`` filter), returning a wrong reconstruction;
    - any gate BEFORE the cut touching the TARGET refuses: the target fragment
      re-prepares |m> from scratch and cannot reproduce pre-cut target history;
    - post-cut single-qubit gates on the control are recorded (not silently
      dropped): they cannot affect the Z-marginal at the cut because no
      cross-fragment interaction remains after the cut CX.
    """
    if isinstance(cnot_index, bool) or not isinstance(cnot_index, int):
        raise ValueError(f"cnot_index must be an int, got {cnot_index!r}")
    if not 0 <= cnot_index < len(circuit.gates):
        raise ValueError(
            f"cnot_index {cnot_index} out of range for {len(circuit.gates)} gates"
        )
    g = circuit.gates[cnot_index]
    if g.name != "cx" or len(g.qubits) != 2:
        raise ValueError(f"gate at {cnot_index} is {g.name!r}, not cx")
    control, target = g.qubits

    spanning: list[tuple[int, Gate]] = []
    pre_target: list[tuple[int, Gate]] = []
    for idx, x in enumerate(circuit.gates):
        if idx == cnot_index:
            continue
        touches_cut = control in x.qubits or target in x.qubits
        if len(x.qubits) == 2 and touches_cut:
            spanning.append((idx, x))
        if idx < cnot_index and target in x.qubits:
            pre_target.append((idx, x))
    if spanning:
        detail = ", ".join(f"{i}:{x.name}{tuple(x.qubits)}" for i, x in spanning)
        raise ValueError(
            f"cut at {cnot_index} refused: {len(spanning)} other two-qubit gate(s) "
            f"span the fragments ({detail}) — multi-CX cuts need the full "
            "quasi-probability protocol (refusing instead of silently dropping)"
        )
    if pre_target:
        detail = ", ".join(f"{i}:{x.name}{tuple(x.qubits)}" for i, x in pre_target)
        raise ValueError(
            f"cut at {cnot_index} refused: pre-cut gate(s) on the target wire "
            f"({detail}) cannot be reproduced by the |m>-preparation fragment"
        )
    control_gates = [x for x in circuit.gates[:cnot_index] if control in x.qubits]
    target_gates = [x for x in circuit.gates[cnot_index + 1:] if target in x.qubits]
    dropped_post = [x for x in circuit.gates[cnot_index + 1:]
                    if control in x.qubits and len(x.qubits) == 1]
    return CutPlan(cnot_index=cnot_index, control=control, target=target,
                   control_gates_before=control_gates, target_gates_after=target_gates,
                   num_qubits=circuit.num_qubits,
                   dropped_post_control_gates=dropped_post)


def _control_fragment(plan: CutPlan) -> Circuit:
    return Circuit(name="cut-control", num_qubits=plan.num_qubits, gates=plan.control_gates_before)


def _target_fragment(plan: CutPlan, outcome: int) -> Circuit:
    gates: list[Gate] = []
    if outcome == 1:
        gates.append(Gate("x", (plan.target,)))  # prepare |1>; outcome 0 -> |0>
    gates.extend(plan.target_gates_after)
    return Circuit(name="cut-target", num_qubits=plan.num_qubits, gates=gates)


def _require_counts(counts: dict, where: str) -> int:
    if not counts:
        raise ValueError(
            f"cut refused ({where}): backend returned empty counts "
            "(no shots recorded — a reconstruction from no data is fabrication)"
        )
    total = sum(counts.values())
    if total <= 0:
        raise ValueError(f"cut refused ({where}): counts sum to zero shots")
    return total


def _child_seeds(seed: int, n: int) -> list[int]:
    if isinstance(seed, bool) or not isinstance(seed, int):
        raise ValueError(f"seed must be an int, got {seed!r}")
    return [int(c.generate_state(1)[0]) % (2 ** 63)
            for c in np.random.SeedSequence(seed).spawn(n)]


def cut_histogram(
    circuit: Circuit,
    cnot_index: int,
    n_target: int,
    backend: BackendFn,
    shots: int,
    seed: int,
) -> dict:
    """Reconstruct the FULL target-register distribution post-cut (exact for any
    Z-diagonal protocol). p(b) = sum_m p_Z(m) * p_{B|m}(b), exact for the
    Z-basis wire-cut — the highest-value use case (populations, histograms).

    ``shots`` and ``seed`` are REQUIRED (no silent 8192/7): child seeds are
    threaded into every fragment execution. ``n_target`` must match the layout:
    the target wire has to sit inside the trailing ``n_target``-qubit register
    the mask reads, else the call refuses (remap the circuit instead).
    """
    if isinstance(shots, bool) or not isinstance(shots, int) or shots < 1:
        raise ValueError(f"shots must be a positive int, got {shots!r}")
    if isinstance(n_target, bool) or not isinstance(n_target, int) \
            or not 1 <= n_target <= circuit.num_qubits:
        raise ValueError(
            f"n_target must be in [1, {circuit.num_qubits}], got {n_target!r}"
        )
    plan = plan_cut(circuit, cnot_index)
    if plan.target < circuit.num_qubits - n_target:
        raise ValueError(
            f"target-layout mismatch: cut target q{plan.target} is outside the "
            f"trailing {n_target}-qubit register the mask reads (qubits "
            f"{circuit.num_qubits - n_target}..{circuit.num_qubits - 1}): "
            "remap the circuit or widen n_target — refusing"
        )
    s_control, s_t0, s_t1 = _child_seeds(seed, 3)
    cc = backend(_control_fragment(plan), shots, s_control)["counts"]
    total = _require_counts(cc, "control fragment")
    p_m = {m: sum(v for k, v in cc.items() if _bit(k, plan.control) == m) / total for m in (0, 1)}

    dist: dict[str, float] = {}
    for m, s_t in ((0, s_t0), (1, s_t1)):
        tc = backend(_target_fragment(plan, m), shots, s_t)["counts"]
        ttotal = _require_counts(tc, f"target fragment m={m}")
        for k, v in tc.items():
            b = _mask(k, n_target)  # suffix (big-endian) target-register bitstring
            dist[b] = dist.get(b, 0.0) + p_m[m] * (v / ttotal)

    # Renormalize AFTER rounding (documented): 6dp rounding sheds up to
    # ~1e-6 of mass; dividing by the rounded sum restores a true distribution
    # instead of shipping weights that sum to 0.999999x.
    rounded = {k: round(v, 6) for k, v in dist.items()}
    mass = sum(rounded.values())
    if mass <= 0:
        raise ValueError("cut refused: reconstructed distribution has zero mass")
    dist = {k: v / mass for k, v in rounded.items()}

    return {"distribution": dist,
            "control_probs": p_m, "protocol": "LOCC-wire-cut-Z-histogram",
            "seed": seed, "fragment_seeds": {"control": s_control, "m0": s_t0, "m1": s_t1},
            "fragment_widths": plan.subcircuit_widths()}


def cut_expectation(
    circuit: Circuit,
    cnot_index: int,
    observable_qubit: int,
    backend: BackendFn,
    shots: int,
    seed: int,
) -> dict:
    """Reconstruct <Z_q> for the single target qubit reached by the CNOT cut.

    Honest scope: a single-CNOT wire cut reaches exactly ONE target wire, so a
    Z expectation is defined on that single qubit — ``observable_qubit`` MUST
    be the cut target (anything else refuses: the protocol reconstructs no
    other wire). (A product-Z over multiple qubits needs multiple cut points —
    the full knitting/QPD domain, documented as the production toolbox path.)
    For the FULL post-cut marginal distribution use `cut_histogram`, which is
    exact for Z-diagonal and strictly more informative than a single ⟨Z⟩.

    ``backend(circuit, shots, seed)`` must return {"counts": {bitstring: int}}.
    ``shots`` and ``seed`` are REQUIRED (no silent 8192/7): every fragment run
    consumes a derived child seed, and the seeds are echoed in the result — no
    unseeded draw is ever labeled seeded.
    """
    if isinstance(shots, bool) or not isinstance(shots, int) or shots < 1:
        raise ValueError(f"shots must be a positive int, got {shots!r}")
    plan = plan_cut(circuit, cnot_index)
    if observable_qubit != plan.target:
        raise ValueError(
            f"observable qubit q{observable_qubit} is not the cut target "
            f"q{plan.target}: a single-CNOT Z-cut reconstructs exactly one wire "
            "(refusing instead of returning another wire's uncut-adjacent number)"
        )
    s_control, s_t0, s_t1 = _child_seeds(seed, 3)
    cc = backend(_control_fragment(plan), shots, s_control)["counts"]
    total = _require_counts(cc, "control fragment")
    p = {m: sum(v for k, v in cc.items() if _bit(k, plan.control) == m) / total for m in (0, 1)}

    value = 0.0
    per_outcome: dict[int, float] = {}
    for m, s_t in ((0, s_t0), (1, s_t1)):
        tc = backend(_target_fragment(plan, m), shots, s_t)["counts"]
        _require_counts(tc, f"target fragment m={m}")
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
        "seed": seed,
        "fragment_seeds": {"control": s_control, "m0": s_t0, "m1": s_t1},
    }


def _bit(bitstring: str, qubit: int) -> int:
    return int(bitstring[qubit])  # qubit 0 = most significant bit (big-endian)


def _z_expectation(counts: dict[str, int], qubit: int) -> float:
    total = _require_counts(counts, "z-expectation")
    return sum(v * (1.0 if _bit(k, qubit) == 0 else -1.0) for k, v in counts.items()) / total


def _mask(bitstring: str, n_target: int) -> str:
    """Big-endian suffix of n_target qubits (the target register after the cut)."""
    if len(bitstring) < n_target:
        raise ValueError(
            f"backend bitstring {bitstring!r} shorter than n_target={n_target}: "
            "backend width does not match the cut circuit — refusing"
        )
    return bitstring[-n_target:]
