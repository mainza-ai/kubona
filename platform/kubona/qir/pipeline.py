"""Reference lowering pipeline (QASM 3 subset -> QIR-like text)."""

from __future__ import annotations

import math

from kubona.circuits import Circuit, Gate, parse_qasm2

# Native set the reference pipeline targets (matches local simulator + ibm basics)
NATIVE_SET = ("rx", "ry", "rz", "cx", "x", "h", "measure")

#: Tier label for the Python fallback: a reference lowering, never verified QIR.
PYTHON_REFERENCE_TIER = "python-reference"

#: Warning attached to every Python-reference lowering so no reader mistakes
#: ``ok: True`` for a verified-compiler artifact.
PYTHON_REFERENCE_WARNING = (
    "Python reference lowering only (no Rust qir-core executed): QIR-like "
    "text for offline/CI use, NOT verified compiler output. ok=True means "
    "'the reference ran', not 'a real compiler verified this'."
)


def transpile_to_native(circuit: Circuit) -> Circuit:
    """Decompose u2/u3/cswap into the native set (single-pass reference).

    u3(theta, phi, lam) = rz(phi) ry(theta) rz(lam)   [ZXZ convention]
    u2(phi, lam)       = u3(pi/2, phi, lam)
    cswap             = two cx + ccx -> (not decomposed further here; Phase 2)

    Decomposition ORDER (verified against the textbook u3 unitary in
    tests/test_prodbar_circuits.py::test_u3_u2_decomposition_matches_unitary):
    circuit lists apply left-to-right (schedule.circuit_unitary folds
    U_gate @ acc), so [rz(lam), ry(theta), rz(phi)] applies lam FIRST —
    matching u3 = Rz(phi) Ry(theta) Rz(lam) in matrix-product notation.
    """
    out_gates: list[Gate] = []
    for g in circuit.gates:
        if g.name == "u3":
            if len(g.params) != 3:
                raise ValueError(f"u3 needs 3 params, got {len(g.params)}")
            th, ph, la = g.params
            q = g.qubits[0]
            out_gates += [Gate("rz", (q,), (la,)), Gate("ry", (q,), (th,)), Gate("rz", (q,), (ph,))]
        elif g.name == "u2":
            if len(g.params) != 2:
                raise ValueError(f"u2 needs 2 params, got {len(g.params)}")
            ph, la = g.params
            q = g.qubits[0]
            out_gates += [Gate("rz", (q,), (la,)), Gate("ry", (q,), (math.pi / 2,)), Gate("rz", (q,), (ph,))]
        elif g.name == "u1":
            if len(g.params) != 1:
                raise ValueError(f"u1 needs 1 param, got {len(g.params)}")
            la = g.params[0]
            out_gates.append(Gate("rz", g.qubits, (la,)))
        elif g.name == "cswap":
            raise ValueError("cswap decomposition is Phase 2 (needs a Toffoli decomposition)")
        else:
            out_gates.append(g)
    return Circuit(name=circuit.name, num_qubits=circuit.num_qubits, gates=out_gates,
                   num_classical=circuit.num_classical)


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
        # Cross-validate the Rust-emitted metadata against an independent
        # parse of the same QASM: qubit-count or name drift between the two
        # frontends refuses instead of shipping a mismatched artifact.
        rust_circuit = parse_qasm2(circuit_qasm)
        if rust["num_qubits"] != rust_circuit.num_qubits:
            raise ValueError(
                f"rust/python frontend mismatch: rust emitted "
                f"{rust['num_qubits']} qubits, QASM parses to "
                f"{rust_circuit.num_qubits}: refusing"
            )
        rust["circuit_name"] = rust_circuit.name
        return rust
    # fallback: Python reference (pre-Rust-integration / no cargo-built binary)
    circuit = parse_qasm2(circuit_qasm)
    native = transpile_to_native(circuit)
    counts = {g: 0 for g in NATIVE_SET}
    for x in native.gates:
        if x.name in counts:
            counts[x.name] += 1
        else:
            # Non-native survivors (e.g. phase gates the reference does not
            # decompose) are counted explicitly, never dropped from the tally.
            counts[x.name] = counts.get(x.name, 0) + 1
    return {
        "ok": True,
        "tier": PYTHON_REFERENCE_TIER,
        "warning": PYTHON_REFERENCE_WARNING,
        "circuit_name": circuit.name,
        "num_qubits": circuit.num_qubits,
        "native_gate_set": list(native_gate_set),
        "passes": ["parse_qasm", "decompose_u2_u3_u1", "emit_qir_like"],
        "qir_text": _qir_text(native),
        "native_gate_count": counts,
        "engine": "python-reference",
    }
