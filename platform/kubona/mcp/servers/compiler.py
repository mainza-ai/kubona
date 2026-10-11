"""Compiler MCP server.

Tools: transpile_circuit (validates QASM 2, builds the SDK-independent IR, returns
the canonical content-addressed form) and lower_circuit — which lowers to QIR via
the Rust `qir-core` core (falling back to the Python reference when the binary
isn't built) — plus compile_circuit (feature-based pass portfolio with per-pass
equivalence evidence and compile receipts).
"""

from __future__ import annotations

import asyncio

from mcp.server.mcpserver import MCPServer

from kubona.circuits import canonical_hash, parse_qasm2, to_qasm

mcp = MCPServer("kubona-compiler")


def transpile_circuit(circuit_qasm: str) -> dict:
    """Validate and canonicalize an OpenQASM 2 circuit into Kubona's IR."""
    circuit = parse_qasm2(circuit_qasm)
    return {
        "ok": True,
        "circuit_name": circuit.name,
        "num_qubits": circuit.num_qubits,
        "depth": circuit.depth(),
        "circuit_hash": canonical_hash(circuit),
        "canonical_qasm": to_qasm(circuit),
    }


from kubona.qir import lower as qir_lower


def lower_circuit(circuit_qasm: str) -> dict:
    """Lower a circuit to QIR-like IR via the reference pipeline (Phase 1)."""
    return qir_lower(circuit_qasm)


def compile_circuit(circuit_qasm: str, target: str = "local",
                    two_qubit_error: float | None = None) -> dict:
    """Run the compiler pass portfolio: feature-based pass selection with
    per-pass equivalence evidence and a chained compile receipt."""
    from kubona.qir.portfolio import compile as portfolio_compile
    return portfolio_compile(circuit_qasm, target=target,
                             two_qubit_error=two_qubit_error)


mcp.add_tool(transpile_circuit, name="transpile_circuit",
             description="Validate and canonicalize an OpenQASM 2 circuit into Kubona's SDK-independent IR.")
mcp.add_tool(lower_circuit, name="lower_circuit",
             description="Lower a circuit to QIR-like IR (reference pipeline; Rust core is qir-core).")
mcp.add_tool(compile_circuit, name="compile_circuit",
             description="Compile with the pass portfolio: feature-selected passes, equivalence evidence, compile receipt.")

if __name__ == "__main__":
    asyncio.run(mcp.run_stdio_async())
