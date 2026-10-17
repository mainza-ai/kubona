"""Fragile-late gate scheduling.

Non-Clifford operations (magic-generating, typically higher-error) suffer
prolonged noise exposure when placed early: every subsequent gate drags
their outputs through more error channels. This pass pushes non-Clifford
gates later by swapping adjacent commuting pairs — semantics preserved by
construction (only provably-commuting swaps), verified by exact unitary
equality up to global phase.

Commutation rules are deliberately conservative (sound, not complete):
  - disjoint qubits always commute;
  - diagonal gates (z/s/sdg/t/tdg/rz/cp/cz) all commute with each other;
  - same single-qubit rotation (rx/rx, ry/ry, rz/rz) on the same qubit;
  - everything else is assumed non-commuting (e.g. CX vs Z on its target
    genuinely does not commute; CX vs Z on its control does, but we leave
    that optimization on the floor rather than risk wrongness).

CX control/target convention (shared with local_simulator and
mitigation/backend): ``Gate("cx", (c, t))`` — the FIRST qubit is the control
(high bit), the SECOND is the target: |c,t> -> |c, c^t>. The embedding below
treats ``gate.qubits[0]`` as control throughout; see
``test_cx_control_target_convention`` for the pinned behavior (control-|1>
flips the target, control-|0> is identity).

Metric: index of the first non-Clifford gate (higher = later = better).
"""

from __future__ import annotations

import numpy as np

from kubona.circuits.model import (
    CLIFFORD_1Q,
    CLIFFORD_2Q,
    DIAGONAL,
    EXACT_UNITARY_MAX_QUBITS,
    SAME_AXIS_ROTATIONS,
    SUPPORTED_GATES,
    Circuit,
    Gate,
)

#: Suffix marking fragile-late output. Applied idempotently: re-scheduling an
#: already-scheduled circuit keeps the name (the old code re-appended, so
#: ``schedule(schedule(c)).name`` grew a suffix per pass and receipts drifted).
FRAGILE_LATE_SUFFIX = "+fragile-late"

#: Default unitary-equality tolerance; scaled by Hilbert-space dimension at
#: each comparison (see equivalent_up_to_phase).
EQUIVALENCE_TOL_V1 = 1e-9


def is_clifford(gate: Gate) -> bool:
    if len(gate.qubits) == 1:
        return gate.name in CLIFFORD_1Q
    if len(gate.qubits) == 2:
        return gate.name in CLIFFORD_2Q
    return False


def _is_diagonal(gate: Gate) -> bool:
    return gate.name in DIAGONAL


def commutes(a: Gate, b: Gate) -> bool:
    """Conservative commutativity: True only when provably commuting."""
    qa, qb = set(a.qubits), set(b.qubits)
    if qa.isdisjoint(qb):
        return True
    if _is_diagonal(a) and _is_diagonal(b):
        return True
    if (len(qa) == 1 and qa == qb and a.name == b.name
            and a.name in SAME_AXIS_ROTATIONS):
        return True
    return False


def fragile_metrics(circuit: Circuit) -> dict[str, int]:
    """Position of fragile (non-Clifford) gates; -1 when none present."""
    idx = [i for i, g in enumerate(circuit.gates) if not is_clifford(g)]
    return {
        "n_nonclifford": len(idx),
        "first_nonclifford": idx[0] if idx else -1,
        "last_nonclifford": idx[-1] if idx else -1,
        "n_gates": len(circuit.gates),
    }


def schedule_fragile_late(circuit: Circuit) -> Circuit:
    """Bubble non-Clifford gates later past commuting neighbors.

    Stable, terminating (each swap moves a non-Clifford strictly right past
    a Clifford gate: at most n^2 swaps). Returns a new Circuit; input untouched.
    The output name carries FRAGILE_LATE_SUFFIX exactly once (idempotent).
    """
    gates = list(circuit.gates)
    moved = True
    while moved:
        moved = False
        for i in range(len(gates) - 1):
            # Only past Clifford gates: two commuting non-Cliffords swapping
            # forever would never terminate (their position-sum is invariant).
            # Their relative order is irrelevant to the fragile metric.
            if (not is_clifford(gates[i]) and is_clifford(gates[i + 1])
                    and commutes(gates[i], gates[i + 1])):
                gates[i], gates[i + 1] = gates[i + 1], gates[i]
                moved = True
    name = circuit.name
    if not name.endswith(FRAGILE_LATE_SUFFIX):
        name = name + FRAGILE_LATE_SUFFIX
    return Circuit(name=name, num_qubits=circuit.num_qubits,
                   gates=gates, num_classical=circuit.num_classical)


_SINGLE = {
    "id": np.array([[1, 0], [0, 1]], complex),
    "x": np.array([[0, 1], [1, 0]], complex),
    "y": np.array([[0, -1j], [1j, 0]], complex),
    "z": np.array([[1, 0], [0, -1]], complex),
    "h": np.array([[1, 1], [1, -1]], complex) / np.sqrt(2),
    "s": np.array([[1, 0], [0, 1j]], complex),
    "sdg": np.array([[1, 0], [0, -1j]], complex),
    "t": np.array([[1, 0], [0, np.exp(1j * np.pi / 4)]], complex),
    "tdg": np.array([[1, 0], [0, np.exp(-1j * np.pi / 4)]], complex),
}


def _gate_unitary(gate: Gate, n: int) -> np.ndarray:
    """Exact unitary for one gate (verification scale only).

    Refuses gates outside SUPPORTED_GATES with an explicit supported-set
    error — the old code fell through to ``_SINGLE[gate.name]`` (bare
    KeyError for u1/u2/u3) or a generic arity message, hiding *why* the
    verification cannot run.
    """
    if gate.name not in SUPPORTED_GATES:
        raise ValueError(
            f"no exact unitary for gate {gate.name!r}: supported set is "
            f"{sorted(SUPPORTED_GATES)} (transpile exotic gates first)"
        )
    if len(gate.qubits) == 1:
        if gate.name in ("rx", "ry", "rz"):
            if len(gate.params) != 1:
                raise ValueError(
                    f"rotation {gate.name!r} needs exactly 1 param, "
                    f"got {len(gate.params)}"
                )
            theta = gate.params[0]
            c, s = np.cos(theta / 2), np.sin(theta / 2)
            if gate.name == "rx":
                base = np.array([[c, -1j * s], [-1j * s, c]], complex)
            elif gate.name == "ry":
                base = np.array([[c, -s], [s, c]], complex)
            else:
                base = np.array([[np.exp(-1j * theta / 2), 0],
                                 [0, np.exp(1j * theta / 2)]], complex)
        else:
            if gate.params:
                raise ValueError(
                    f"gate {gate.name!r} takes no params, got {gate.params!r}"
                )
            base = _SINGLE[gate.name]
        if gate.qubits[0] >= n:
            raise ValueError(f"gate {gate.name!r} qubit out of range for n={n}")
        mats = [base if k == gate.qubits[0] else np.eye(2) for k in range(n)]
        out = mats[0]
        for m in mats[1:]:
            out = np.kron(out, m)
        return out
    if len(gate.qubits) == 2:
        a, b = gate.qubits  # a = control (high bit), b = target
        if max(a, b) >= n:
            raise ValueError(f"gate {gate.name!r} qubit out of range for n={n}")
        if gate.name == "cp" and len(gate.params) != 1:
            raise ValueError(
                f"gate 'cp' needs exactly 1 param, got {len(gate.params)}"
            )
        if gate.name != "cp" and gate.params:
            raise ValueError(
                f"gate {gate.name!r} takes no params, got {gate.params!r}"
            )
        dim = 1 << n
        out = np.zeros((dim, dim), complex)
        for col in range(dim):
            bits = [(col >> (n - 1 - k)) & 1 for k in range(n)]
            ba, bb = bits[a], bits[b]
            if gate.name == "cx":
                bits[a], bits[b] = ba, ba ^ bb
                row = sum(bit << (n - 1 - k) for k, bit in enumerate(bits))
                out[row, col] = 1
            elif gate.name == "cz":
                out[col, col] = -1 if (ba and bb) else 1
            elif gate.name == "swap":
                bits[a], bits[b] = bb, ba
                row = sum(bit << (n - 1 - k) for k, bit in enumerate(bits))
                out[row, col] = 1
            elif gate.name == "cp":
                theta = gate.params[0]
                out[col, col] = np.exp(1j * theta) if (ba and bb) else 1
        return out
    raise ValueError(f"unsupported arity {len(gate.qubits)}")


def circuit_unitary(circuit: Circuit) -> np.ndarray:
    """Exact circuit unitary (refuses above EXACT_UNITARY_MAX_QUBITS: exponential)."""
    if circuit.num_qubits > EXACT_UNITARY_MAX_QUBITS:
        raise ValueError(
            f"exact unitary refused for n={circuit.num_qubits} > "
            f"{EXACT_UNITARY_MAX_QUBITS}"
        )
    dim = 1 << circuit.num_qubits
    out = np.eye(dim, dtype=complex)
    for gate in circuit.gates:
        out = _gate_unitary(gate, circuit.num_qubits) @ out
    return out


def equivalent_up_to_phase(a: Circuit, b: Circuit,
                            tol: float = EQUIVALENCE_TOL_V1) -> bool:
    """True when both circuits implement the same unitary mod global phase.

    ``tol`` is scaled by the Hilbert-space dimension (``tol * dim``): overlap
    error accumulates over matrix entries, so a fixed absolute threshold
    silently tightens as circuits widen. Callers needing a fixed threshold
    pass an explicit ``tol``.
    """
    if tol <= 0 or not np.isfinite(tol):
        raise ValueError(f"tol must be positive finite, got {tol!r}")
    if a.num_qubits != b.num_qubits:
        return False
    ua, ub = circuit_unitary(a), circuit_unitary(b)
    dim = ua.shape[0]
    overlap = abs(np.vdot(ua.ravel(), ub.ravel())) / dim
    return bool(overlap > 1 - tol * dim)
