"""Compiler MCP server (Phase 0 stub).

Tool: transpile_circuit — validates QASM 2, builds the SDK-independent IR, and
returns the canonical (content-addressed) form. Real QIR/MLIR lowering is Phase 1
(Rust core); this stub fixes the tool schema and the pre-transpile validation.
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


mcp.add_tool(transpile_circuit, name="transpile_circuit",
             description="Validate and canonicalize an OpenQASM 2 circuit into Kubona's SDK-independent IR.")
mcp.add_tool(lower_circuit, name="lower_circuit",
             description="Lower a circuit to QIR-like IR (reference pipeline; Rust core is qir-core).")

if __name__ == "__main__":
    asyncio.run(mcp.run_stdio_async())
