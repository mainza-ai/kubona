---
type: concept
name: MCP & Context Flooding
date: 2026-08-16
tags: [mcp, agent-integration]
sources: [source-architecture-paper, source-gap-analysis-plan]
status: living
---

# Model Context Protocol (MCP) & Context Flooding

The communication layer between AI agents and Kubona tools. Open, universal standard using
**JSON-RPC 2.0** messaging with JSON-Schema-typed tool descriptions; agent = MCP **Host +
Client**, quantum platform = MCP **Server**, over stdio / SSE / streamable HTTP
(source-architecture-paper).

## Three primitives

| Primitive | Quantum-platform function |
|---|---|
| **Tools** | Executable functions (e.g. `transpile_circuit`, `submit_aws_task`, `apply_zne_mitigation`) — JSON Schema guides the agent's params |
| **Resources** | Read-only context data, URI-addressed (hardware topology maps, coherence times, pricing tables; also the provenance history) |
| **Prompts** | Reusable multi-step workflow templates (e.g. "Optimize QAOA Ansatz") |

## Context flooding → code-execution mode

Loading every tool JSON schema into the agent's context degrades task success sharply as the
tool count grows. Kubona's countermeasure: expose a **secure Python execution sandbox**
instead of hundreds of granular tools — the agent writes a full Qiskit/PennyLane script,
the server executes it, preserving context and leveraging the LLM's code-gen strength
(source-architecture-paper).

## Kubona's server topology (see [[overview]])

Four **federated, scoped** servers — compiler, execution, cost-governance, eval/provenance —
behind one MCP Gateway (OAuth 2.1, PQC-hybrid, identity-aware per-call RBAC). Finer-grained
RBAC + selective mounting by [[kubona|orchestration frameworks]] (LangGraph/CrewAI/AutoGen).
MCP servers are nominally stateless, so multi-hour [[vqa-vqe|VQE loops]] are reconstructed
from the ledger via the `job_id` session abstraction (source-gap-analysis-plan).

### Independent validation of "MCP for quantum"

[[arxiv-mcp-quantum]] (arXiv 2604.08318, 2026) independently demonstrates an MCP server for
quantum execution — supporting evidence that this bus choice is becoming field-standard,
not speculative (source-gap-analysis-plan §3).

Related: [[security-and-pqc]] · [[cost-governance]] · [[provenance-and-reproducibility]]