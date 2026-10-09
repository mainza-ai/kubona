---
type: concept
name: Protocol Strategy & Capability Core Decoupling
date: 2026-10-03
updated: 2026-10-09
tags: [protocols, mcp, a2a, architecture, capability-core, agi-robustness]
sources: [source-agent-protocol-landscape-2026, source-architecture-paper]
status: adopted
---

# Protocol Strategy & Capability Core Decoupling

Kubona's architectural boundary separating external interaction protocols from internal
quantum execution, cost governance, and verification primitives (source-agent-protocol-landscape-2026).

## The Core Thesis: Protocols as Peripherals & The TypeScript/Go Backbone

In the rapidly shifting landscape leading toward AGI/ASI, interaction protocols evolve on
12–18 month cycles (e.g. MCP's complete 2026 overhaul, ACP's absorption into A2A). In contrast,
quantum intermediate representations ([[qir-mlir-compilation]], OpenQASM 3) and verification
physics remain durable across decades.

To run thousands of concurrent autonomous agent loops, maintain real-time multiplayer lab states,
and interface cleanly with quantum runtimes, the platform divides into four specialized tiers:

```
 [ FRONTEND UI: TypeScript + React/Next.js ]
        │  (WebSockets / gRPC-Web / Yjs CRDTs)
        ▼
 [ BACKEND ORCHESTRATOR: Go (Golang) ]
        ├── Protocol Edges: MCP Gateway, A2A Agent Cards, AP2 Mandates
        ├── Agent Loop Concurrency: Goroutines & channels, state engine
        ├── Cloud/Sandbox Control: Container lifecycle (Docker/K8s) for agent code
        └── Immutable Ledger & Session Store: High-throughput audit logging
        │
        ├── (gRPC / Unix Socket IPC) ──────► [ COMPILER KERNEL: Rust ]
        │                                       • QIR Lowering, AST analysis
        ▼ (gRPC / Streaming IPC)
 [ DATA / QUANTUM BRIDGE: Python (FastAPI / gRPC) ]
        • Quantum SDKs: Qiskit, PennyLane, Cirq, pyQuil, pytket, Pulser
        • Classical Physics Anchors: PySCF, FCI/RHF numerical active-space solvers
        • AI Data Analysis & Scientific Integrations: arXiv, OpenAlex, ChEMBL
```

## Protocol Layer Map

| Layer | Standard | Role in Kubona |
|---|---|---|
| **Agent → Tool/Data** | [[mcp-and-context-flooding|MCP]] (2026-07-28) | Primary external tool edge. Agents invoke transpilation, job dispatch, and query resources. |
| **Agent ↔ Agent Delegation** | [[a2a|A2A]] (Linux Foundation) | Task delegation for multi-agent collectives and agent-created personas without exposing internal state. |
| **Spend Authorization** | [[ap2|AP2]] (Signed Mandates) | Cryptographic user intent proving human approval for QPU spending. |
| **Monetization & Settlement** | [[x402|x402]] & MPP | HTTP 402 programmable settlement (Phase 5; currently dormant). |
| **Intermediate Representation** | [[qir-mlir-compilation|QIR]] / OpenQASM 3 | Native internal circuit representation; SDK-agnostic compiler target. |
| **Hardware Control** | Native Vendor SDKs / Pulse | Low-latency, non-JSON transport directly to physical hardware backends. |

## AGI/ASI Horizon & Code Execution

As agent reasoning capabilities advance:
1. **Context Flooding Mitigation:** Exposing granular JSON-RPC tool definitions incurs heavy
   token overhead. Under code-execution mode (Anthropic / Cloudflare Code Mode), agents write
   direct Python/Rust scripts against Kubona's SDK inside a sandboxed runner, reducing context
   consumption by up to **98.7%** (source-agent-protocol-landscape-2026).
2. **Durable Governance:** Stronger, more autonomous models increase rather than decrease the
   need for hard budget ceilings, cryptographic approval mandates, and immutable audit ledgers.
   An agent with access to arbitrary code execution must still be constrained by the Capability
   Core's budget rails before jobs hit physical QPUs.

Related: [[mcp-and-context-flooding]] · [[spend-mandates-and-delegation]] · [[cost-governance]] · [[integration-topology-and-licence-discipline]]

## Contract enforcement (Q-28 closed 2026-10-09: no namespace move)

Coupling measured, not asserted: the knowledge-graph call clusters separate Go enforcement
(ledger/tenants/jobs, API handlers, A2A, memory) from edges, and zero files under
`circuits/anchors/epistemics/science/canary/providers` import MCP — coupling is
one-directional (edge → core), which is the ports-and-adapters shape. Verdict: keep
`KubonaGateway`/`Principal` where they are; moving them churns imports for zero seam change.

Instead the contract is mechanical — architectural fitness functions in CI:

- Go (`platform/core/pkg/architecture_test.go`): core packages
  (ledger/jobs/triage/mandates/epistemics/fidelity/ifc/qec/session/tenants/watchdog/cost/
  memory/qir/sandbox/vault/metasearch/autoresearch/llm/supervisor) must not import
  `pkg/api|agents|mcp|hf` (parsed with go/parser, no toolchain needed).
- Python (`platform/tests/test_architecture.py`): compute packages must not import
  `kubona.mcp`/MCP-SDK modules (parsed with ast).
- Direction rule: adapters may call core; core never imports adapters. Enforcement
  (Verify/budget/lattice/admission) lives in the core rail — asserted per-subsystem by
  the refusal tests, not duplicated in adapters.
