---
type: concept
name: MCP & Context Flooding
date: 2026-08-16
updated: 2026-10-09
tags: [mcp, agent-integration]
sources: [source-architecture-paper, source-gap-analysis-plan, source-agent-protocol-landscape-2026]
status: adopted
---

# Model Context Protocol (MCP) & Context Flooding

The communication layer between AI agents and Kubona tools. Open, universal standard governed by
the [[aaif|Agentic AI Foundation (AAIF)]] under the Linux Foundation (source-agent-protocol-landscape-2026).
Uses JSON-RPC 2.0 messaging with JSON-Schema-typed tool descriptions; agent = MCP **Host +
Client**, quantum platform = MCP **Server**, over stdio / Streamable HTTP.

## Three primitives & 2026 specification

| Primitive | Quantum-platform function | 2026-07-28 Revision (`SEP-2575`) |
|---|---|---|
| **Tools** | Executable functions (`transpile_circuit`, `submit_job`, `apply_zne_mitigation`) | List results cached via `ttlMs`/`cacheScope`; strict parameter header validation. |
| **Resources** | Read-only context data, URI-addressed (hardware topology maps, pricing tables, provenance) | Single stream notification via `subscriptions/listen`. |
| **Prompts** | Reusable multi-step workflow templates (e.g. "Optimize QAOA Ansatz") | Deterministic ordering for LLM prompt cache optimization. |

> ⚑ Correction (2026-10-03): this page previously noted that *"MCP servers are nominally stateless,
> so multi-hour VQE loops are reconstructed from the ledger via `job_id`"*. Under spec **2026-07-28**,
> protocol-level sessions and `Mcp-Session-Id` are **normatively removed from core**: every request
> carries capabilities in `_meta`, and cross-call handles are passed as ordinary tool arguments.
> Furthermore, **Tasks** were moved out of core into the official extension `io.modelcontextprotocol/tasks`
> (`tasks/get` polling), while Roots, Sampling, and Logging are deprecated (source-agent-protocol-landscape-2026).
> Multi-Round-Trip Requests (MRTR) now formalize approvals without holding open network connections.

## Context flooding → code-execution mode

Loading every tool JSON schema into the agent's context degrades task success sharply as the
tool count grows. Kubona's countermeasure: expose a **secure Python execution sandbox**
instead of hundreds of granular tools — the agent writes a full Qiskit/PennyLane script,
the server executes it, preserving context and leveraging the LLM's code-gen strength
(source-architecture-paper).

Empirical validation (source-agent-protocol-landscape-2026):
- Anthropic engineering demonstrates that presenting MCP tools as code APIs cuts token overhead
  by up to **98.7%**.
- Cloudflare Code Mode collapses a 2,500-endpoint catalog (>2,000,000 tokens as schemas) into
  a dynamic execution pattern consuming **~1,000 tokens**.

## Kubona's server topology & capability core

Four **federated, scoped** servers — compiler, execution, cost-governance, eval/provenance —
mounted behind a unified gateway. Per [[protocol-strategy]], the servers are thin adapters
delegating to Kubona's protocol-neutral Capability Core; capability functions accept and return
native Python data structures uncoupled from MCP SDK types.

## Target revision & extension policy (Q-29, closed 2026-10-09)

- **Target: MCP spec 2026-07-28.** Verified consequences already in the tree: protocol-level
  sessions/`Mcp-Session-Id` removed from core (handles pass as tool arguments), Tasks out of
  core into `io.modelcontextprotocol/tasks`, Roots/Sampling/Logging deprecated. The venv
  already runs SDK 2.3.0, so the "2.0.0 vs 2.3.0" fork is moot (deltas verified against the
  5 stdio servers + Streamable HTTP gateway).
- **Tasks extension: opportunistic, never mandatory (progressive enhancement).** Every flow
  must work on request/response + polling — true today (console polls on 3.5s/1s cadences,
  Go serves a tested SSE stream at `/api/events/stream`). `tasks/get` may be adopted for
  long jobs; MRTR formalizes approvals without held connections (fits the mandate gate).
  REST polling is the normative console transport; the `useTelemetry` SSE hook is staged
  for fleet-radar only, pending load measurement (`platform/console/src/lib/useTelemetry.ts`).
- **A2A adapter ships first** (ledger-bound delegation already on the jobs rail);
  MCP tasks-extension second. Enforcement lives in the Go core rail, never in adapters
  (contract-tested: unmapped capabilities refuse, underspecified physics refuses).

### Independent validation of "MCP for quantum"

[[arxiv-mcp-quantum]] (arXiv 2604.08318, 2026) independently demonstrates an MCP server for
quantum execution in hybrid quantum-HPC environments. In Sept 2026, IBM released official
[[qiskit-mcp-servers]], confirming industry standardization while highlighting Kubona's
differentiated multi-provider governance layer.

Related: [[protocol-strategy]] · [[security-and-pqc]] · [[cost-governance]] · [[provenance-and-reproducibility]] · [[spend-mandates-and-delegation]]