"""Minimal, SDK-independent circuit IR.

A small, SDK-independent gate model + canonical serialization. This is the input
IR the compiler server transpiles (and lowers to QIR via the Rust `qir-core`
core); the canonical hash is the ledger's content address for circuits.

Gate-set constants live here as the single source: ``schedule.py`` (Clifford /
diagonal classification) and ``qir/portfolio.py`` (feature counting) import
from this module so the sets cannot rot apart.
"""

from __future__ import annotations

import hashlib
import json
import math
import re
from dataclasses import dataclass, field

# ---------------------------------------------------------------------------
# Shared gate-set constants (single source for schedule.py + qir/portfolio.py)
# ---------------------------------------------------------------------------

#: Single-qubit Clifford gates.
CLIFFORD_1Q = frozenset({"id", "x", "y", "z", "h", "s", "sdg"})
#: Two-qubit Clifford gates. NOTE: CX control/target convention — in every
#: unitary embedding in this repo (schedule._gate_unitary, local_simulator,
#: mitigation/backend) the FIRST qubit is the control (high bit) and the
#: SECOND is the target, i.e. ``Gate("cx", (c, t))`` maps |c,t> -> |c, c^t>.
CLIFFORD_2Q = frozenset({"cx", "cz", "swap"})
#: Gates diagonal in the computational basis (all commute with each other).
DIAGONAL = frozenset({"z", "s", "sdg", "t", "tdg", "rz", "cp", "cz"})
#: Single-qubit rotations where same-name/same-qubit pairs commute.
SAME_AXIS_ROTATIONS = frozenset({"rx", "ry", "rz"})
#: Non-Clifford gates the fragile-late pass pushes later (schedule.py metric).
NON_CLIFFORD = frozenset({"t", "tdg", "rx", "ry", "rz", "cp"})
#: Exotic gates needing decomposition before lowering (qir/pipeline.py).
EXOTIC_GATES = frozenset({"u1", "u2", "u3"})
#: Gates the local statevector simulator executes (single-qubit set).
SUPPORTED_GATES_1Q = frozenset(
    {"id", "x", "y", "z", "h", "s", "sdg", "t", "tdg", "rx", "ry", "rz"}
)
#: Two-qubit gates the local statevector simulator executes.
SUPPORTED_GATES_2Q = frozenset({"cx", "cz", "swap", "cp"})
#: Full executable native set (simulator + noise-aware backend).
SUPPORTED_GATES = SUPPORTED_GATES_1Q | SUPPORTED_GATES_2Q

#: Exact unitaries are exponential in width; verification helpers refuse above this.
EXACT_UNITARY_MAX_QUBITS = 8

_GATE_NAME_RE = re.compile(r"^[a-zA-Z_][a-zA-Z0-9_]*$")

#: Gate name -> (arity, n_params or None when variadic). Used by the parser for
#: fail-closed arity validation. ``None`` params means "checked structurally".
GATE_ARITY: dict[str, tuple[int, int | None]] = {
    "id": (1, 0), "x": (1, 0), "y": (1, 0), "z": (1, 0),
    "h": (1, 0), "s": (1, 0), "sdg": (1, 0),
    "t": (1, 0), "tdg": (1, 0),
    "rx": (1, 1), "ry": (1, 1), "rz": (1, 1),
    "cx": (2, 0), "cz": (2, 0), "swap": (2, 0), "cp": (2, 1),
    "u1": (1, 1), "u2": (1, 2), "u3": (1, 3),
}


@dataclass(frozen=True)
class Gate:
    name: str
    qubits: tuple[int, ...]
    params: tuple[float, ...] = ()

    def __post_init__(self) -> None:
        if not isinstance(self.name, str) or not _GATE_NAME_RE.match(self.name):
            raise ValueError(f"invalid gate name {self.name!r}")
        if not isinstance(self.qubits, tuple) or not self.qubits:
            raise ValueError(f"gate {self.name!r} needs a non-empty qubit tuple")
        for q in self.qubits:
            if isinstance(q, bool) or not isinstance(q, int) or q < 0:
                raise ValueError(
                    f"gate {self.name!r} has invalid qubit index {q!r}: "
                    "indices must be non-negative ints"
                )
        for p in self.params:
            if isinstance(p, bool) or not isinstance(p, (int, float)) \
                    or not math.isfinite(float(p)):
                raise ValueError(
                    f"gate {self.name!r} has non-finite parameter {p!r}"
                )
        if len(set(self.qubits)) != len(self.qubits):
            raise ValueError(
                f"gate {self.name!r} has duplicate qubit operands: {self.qubits!r}"
            )


@dataclass
class Circuit:
    name: str
    num_qubits: int
    gates: list[Gate] = field(default_factory=list)
    num_classical: int = 0
    skipped_directives: list[str] = field(default_factory=list)

    def __post_init__(self) -> None:
        if not isinstance(self.name, str) or not self.name:
            raise ValueError("circuit name must be a non-empty string")
        if isinstance(self.num_qubits, bool) or not isinstance(self.num_qubits, int) \
                or self.num_qubits < 1:
            raise ValueError(
                f"num_qubits must be a positive int, got {self.num_qubits!r}"
            )
        if isinstance(self.num_classical, bool) or not isinstance(self.num_classical, int) \
                or self.num_classical < 0:
            raise ValueError("num_classical must be a non-negative int")
        for g in self.gates:
            for q in g.qubits:
                if q >= self.num_qubits:
                    raise ValueError(
                        f"gate {g.name!r} references qubit {q} "
                        f"beyond circuit width {self.num_qubits}"
                    )

    def gate_count(self) -> int:
        """Number of gates (serial count, not depth)."""
        return len(self.gates)

    def depth(self) -> int:
        """Real circuit depth: number of parallel gate layers.

        Greedy layering: each gate lands on the first layer where all of its
        qubits are free. Disjoint-qubit gates share a layer; the old
        implementation returned ``len(self.gates)`` (serial count), which
        over-reports depth for any parallel circuit.
        """
        if not self.gates:
            return 0
        qubit_depth = [0] * self.num_qubits
        for g in self.gates:
            layer = max(qubit_depth[q] for q in g.qubits) + 1
            for q in g.qubits:
                qubit_depth[q] = layer
        return max(qubit_depth)

    def canonical_repr(self) -> str:
        # Exact float repr (round-trip through json): the old code used
        # round(p, 10), so two circuits differing only below 1e-10 aliased to
        # the same ledger hash. json emits shortest-round-trip repr already.
        return json.dumps(
            {
                "name": self.name,
                "num_qubits": self.num_qubits,
                "num_classical": self.num_classical,
                "gates": [
                    {"name": g.name, "qubits": list(g.qubits),
                     "params": [float(p) for p in g.params]}
                    for g in self.gates
                ],
            },
            sort_keys=True,
        )


def canonical_hash(circuit: Circuit) -> str:
    """Content address of a circuit (the ledger's circuit-hash key).

    Full 64-hex SHA-256 digest. The old code truncated to 16 hex chars
    (64 bits) — a collision-prone ledger key presented as a content address.
    """
    return hashlib.sha256(circuit.canonical_repr().encode()).hexdigest()


def to_qasm(circuit: Circuit) -> str:
    """Serialize a circuit to OpenQASM 2.0 (round-trips through parse_qasm2)."""
    for g in circuit.gates:
        if not _GATE_NAME_RE.match(g.name):
            raise ValueError(f"cannot serialize gate with invalid name {g.name!r}")
        if not g.qubits:
            raise ValueError(f"cannot serialize gate {g.name!r} with no qubits")
    lines = ["OPENQASM 2.0;", f"qreg q[{circuit.num_qubits}];"]
    if circuit.num_classical:
        lines.append(f"creg c[{circuit.num_classical}];")
    for g in circuit.gates:
        if g.params:
            params = ",".join(f"{p!r}" for p in g.params)
            qs = ",".join(f"q[{q}]" for q in g.qubits)
            lines.append(f"{g.name}({params}) {qs};")
        else:
            qs = ",".join(f"q[{q}]" for q in g.qubits)
            lines.append(f"{g.name} {qs};")
    return "\n".join(lines) + "\n"
