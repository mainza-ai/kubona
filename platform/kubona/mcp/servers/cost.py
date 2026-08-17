"""Cost-governance MCP server (Phase 0 stub).

Tool: estimate_cost — static pricing-model estimate + pre-flight verdict.
The full service (burn-rate telemetry, ceilings, two-stage approval, x402-compatible
payment interface) is Phase 1 per the roadmap.
"""

from __future__ import annotations

import asyncio

from mcp.server.mcpserver import MCPServer

from kubona.circuits import parse_qasm2
from kubona.cost import preflight
from kubona.schema import load_provider

mcp = MCPServer("kubona-cost")


def estimate_cost(circuit_qasm: str, provider: str = "local-simulator", shots: int = 1024,
                  budget_ceiling_usd: float | None = None) -> dict:
    """Estimate the cost of running a circuit on a provider + pre-flight verdict."""
    circuit = parse_qasm2(circuit_qasm)
    capability = load_provider(provider)
    pf = preflight(capability, circuit, shots, budget_ceiling_usd)
    return {"provider": provider, "preflight_ok": pf["preflight_ok"], "reason": pf["reason"],
            "estimate": pf["estimate"]}


mcp.add_tool(estimate_cost, name="estimate_cost",
             description="Estimate cost + mandatory pre-flight verdict for a circuit on a provider.")

if __name__ == "__main__":
    asyncio.run(mcp.run_stdio_async())
