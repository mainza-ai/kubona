"""Reference lowering pipeline (QASM 3 subset -> QIR-like text)."""

from __future__ import annotations

import math

from kubona.circuits import Circuit, Gate, parse_qasm2

# Native set the reference pipeline targets (matches local simulator + ibm basics)
NATIVE_SET = ("rx", "ry", "rz", "cx", "x", "h", "measure")


def transpile_to_native(circuit: Circuit) -> Circuit:
    """Decompose u2/u3/cswap into the native set (single-pass reference).

    u3(theta, phi, lam) = rz(phi) ry(theta) rz(lam)   [ZXZ convention]
    u2(phi, lam)       = u3(pi/2, phi, lam)
    cswap             = two cx + ccx -> (not decomposed further here; Phase 2)
    """
    out_gates: list[Gate] = []
    for g in circuit.gates:
        if g.name == "u3":
            th, ph, la = g.params
            q = g.qubits[0]
            out_gates += [Gate("rz", (q,), (la,)), Gate("ry", (q,), (th,)), Gate("rz", (q,), (ph,))]
        elif g.name == "u2":
            ph, la = g.params
            q = g.qubits[0]
            out_gates += [Gate("rz", (q,), (la,)), Gate("ry", (q,), (math.pi / 2,)), Gate("rz", (q,), (ph,))]
        elif g.name == "u1":
            la = g.params[0]
            out_gates.append(Gate("rz", g.qubits, (la,)))
        elif g.name == "cswap":
            raise ValueError("cswap decomposition is Phase 2 (needs a Toffoli decomposition)")
        else:
            out_gates.append(g)
    return Circuit(name=circuit.name, num_qubits=circuit.num_qubits, gates=out_gates)


def _qir_text(circuit: Circuit) -> str:
    """Emit QIR-like textual LLVM IR for the circuit (reference semantics).

    Structure mirrors the QIR spec: qubit allocation, __quantum__qis__* calls,
    measurement, and a result record. Not bytecode — the Rust core emits bitcode.
    """
    lines = [
        '; QIR-like textual IR (reference lowering)',
        f'; circuit: {circuit.name}  qubits: {circuit.num_qubits}',
        'define void @main() {',
        'entry:',
    ]
    for q in range(circuit.num_qubits):
        lines.append(f"  %q{q} = call %Qubit* @__quantum__rt__qubit_allocate()")
    for i, g in enumerate(circuit.gates):
        qs = ", ".join(f"%q{q}" for q in g.qubits)
        if g.params:
            ps = ", ".join(f"{p:g}" for p in g.params)
            lines.append(f"  call void @__quantum__qis__{g.name}__body({ps}, {qs})")
        else:
            lines.append(f"  call void @__quantum__qis__{g.name}__body({qs})")
    for q in range(circuit.num_qubits):
        lines.append(f"  call void @__quantum__rt__record_output({q})")
    lines.append("  ret void")
    lines.append("}")
    return "\n".join(lines) + "\n"


def lower(circuit_qasm: str, native_gate_set: tuple[str, ...] = NATIVE_SET) -> dict:
    """Lower a circuit to QIR(s): the Rust `qir-core` core when available (production),
    else the Python reference (offline/CI fallback). Both emit QIR-like text."""
    from kubona.qir.native import lower_with_rust

    rust = lower_with_rust(circuit_qasm)
    if rust is not None:
        rust_circuit = parse_qasm2(circuit_qasm)
        rust["circuit_name"] = rust_circuit.name
        return rust
    # fallback: Python reference (pre-Rust-integration / no cargo-built binary)
    circuit = parse_qasm2(circuit_qasm)
    native = transpile_to_native(circuit)
    return {
        "ok": True,
        "circuit_name": circuit.name,
        "num_qubits": circuit.num_qubits,
        "native_gate_set": list(native_gate_set),
        "passes": ["parse_qasm", "decompose_u2_u3_u1", "emit_qir_like"],
        "qir_text": _qir_text(native),
        "native_gate_count": {g: sum(1 for x in native.gates if x.name == g) for g in NATIVE_SET if
                              any(x.name == g for x in native.gates)},
        "engine": "python-reference",
    }
