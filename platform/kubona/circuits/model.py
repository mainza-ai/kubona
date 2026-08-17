"""Minimal, SDK-independent circuit IR.

Phase 0 scope: a small gate model + canonical serialization. QIR/MLIR lowering
(Rust core) is Phase 1 per the roadmap; this IR is the stub the compiler server
transpiles against today.
"""

from __future__ import annotations

import hashlib
import json
from dataclasses import dataclass, field


@dataclass(frozen=True)
class Gate:
    name: str
    qubits: tuple[int, ...]
    params: tuple[float, ...] = ()


@dataclass
class Circuit:
    name: str
    num_qubits: int
    gates: list[Gate] = field(default_factory=list)
    num_classical: int = 0

    def depth(self) -> int:
        """Approximate circuit depth (number of gate layers; serial for now)."""
        return len(self.gates)

    def canonical_repr(self) -> str:
        return json.dumps(
            {
                "name": self.name,
                "num_qubits": self.num_qubits,
                "gates": [
                    {"name": g.name, "qubits": list(g.qubits), "params": [round(p, 10) for p in g.params]}
                    for g in self.gates
                ],
            },
            sort_keys=True,
        )


def canonical_hash(circuit: Circuit) -> str:
    """Content address of a circuit (the ledger's circuit-hash key)."""
    return hashlib.sha256(circuit.canonical_repr().encode()).hexdigest()[:16]


def to_qasm(circuit: Circuit) -> str:
    lines = [f"OPENQASM 2.0;", f"qreg q[{circuit.num_qubits}];"]
    for g in circuit.gates:
        if g.params:
            params = ",".join(f"{p:g}" for p in g.params)
            qs = ",".join(f"q[{q}]" for q in g.qubits)
            lines.append(f"{g.name}({params}) {qs};")
        else:
            qs = ",".join(f"q[{q}]" for q in g.qubits)
            lines.append(f"{g.name} {qs};")
    return "\n".join(lines) + "\n"
