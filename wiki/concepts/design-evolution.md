---
type: concept
name: Design Evolution (paper → plan)
date: 2026-08-16
updated: 2026-09-27
tags: [kubona, synthesis, decisions]
sources: [source-architecture-paper, source-gap-analysis-plan]
status: adopted
---

# Design Evolution — From White Paper to Buildable Plan

The gap-analysis source's "Summary of What Changed" (§10), preserved as a durable artifact:
how Kubona's design moved from the architecture paper's vision to the expanded architecture.

| Dimension | Paper (§source-architecture-paper) | Plan (§source-gap-analysis-plan) |
|---|---|---|
| **Hardware coverage** | 4 providers (IBM, D-Wave, Q#/Azure, Cirq) | 11+ via data-driven [[provider-capability-schema]], direct open-source SDK integrations |
| **Security** | RBAC + secrets manager | OAuth 2.1 + PQC-hybrid transport + per-call identity-aware auth + immutable audit ledger ([[security-and-pqc]]) |
| **Orchestration** | Monolithic MCP server | 4 federated, scoped servers + `job_id` session abstraction ([[mcp-and-context-flooding]]) |
| **Cost** | Routing heuristic in prose | Mandatory pre-flight + budget ceilings + two-stage approval ([[cost-governance]], [[quantum-cloud-pricing]]) |
| **Reproducibility** | Absent | Content-addressed provenance ledger, agent-queryable ([[provenance-and-reproducibility]]) |
| **Testing** | Code-gen evals only | + hardware canary CI + agent-behavior evals ([[evaluation-and-benchmarks]]) |
| **Frontend** | Read-only dashboard | Circuit visualizer + approval console + live loop inspector ([[agent-ux-control-plane]]) |
| **UI/UX** | Single "observability dashboard" line item | Six defined console surfaces, grounded in quantum-viz prior art ([[ibm-quantum-composer]], [[qflow-studio]]) and 2026 agent-UX practice |
| **Legal/licensing** | Unaddressed | Apache-2.0 from day one + export-control-aware routing + contribution governance |
| **Competitive awareness** | None | Explicit positioning vs [[qbraid]] / [[strangeworks]] / [[nvidia-cuda-q]] as prior art, never dependency |
| **Algorithm R&D** | None | A self-improving algorithm engine ([[karpathy-autoresearch]]) spanning ansatz/circuit optimization *and* correctness regression, + a §13 meta-level loop that evolves the *search itself* ([[meta-search-and-self-evolution]]) |
| **Collaboration & discovery** | Single human operator + one platform agent | Multi-user labs, keypair identities for every actor, user/agent-created workflows & agents, autonomous experiments with structured reports ([[science-discovery]], [[user-identity-and-profiles]], [[workflows-and-agent-authoring]]) — maintainer synthesis |

## What stayed constant

The core bets that needed no re-arguing: **MCP as the agent-to-tool bus**, **QIR/MLIR as
the hardware-agnostic compilation core**, **Rust for the performance-critical compiler +
WASM edge**, and the **polyglot split** (now Go/TS/Python/Rust; Dart retired Q-22).

Related: [[overview]] · [[kubona]] · [[roadmap]]