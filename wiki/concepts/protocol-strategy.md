---
type: concept
name: Protocol Strategy & Capability Core Decoupling
date: 2026-10-03
updated: 2026-10-03
tags: [protocols, mcp, a2a, architecture, capability-core, agi-robustness]
sources: [source-agent-protocol-landscape-2026, source-architecture-paper]
status: adopted
---

# Protocol Strategy & Capability Core Decoupling

Kubona's architectural boundary separating external interaction protocols from internal
quantum execution, cost governance, and verification primitives (source-agent-protocol-landscape-2026).

## The Core Thesis: Protocols as Peripherals

In the rapidly shifting landscape leading toward AGI/ASI, interaction protocols evolve on
12–18 month cycles (e.g. MCP's complete 2026 overhaul, ACP's absorption into A2A). In contrast,
quantum intermediate representations ([[qir-mlir-compilation]], OpenQASM 3) and verification
physics remain durable across decades.

Kubona treats all agent protocols—including [[mcp-and-context-flooding|MCP]] and [[a2a|A2A]]—as
**thin, replaceable edge adapters**. The core assets of the platform (cost governance, immutable
ledger, provider federation, ansatz optimization, classical anchors) reside in a protocol-neutral
**Capability Core**.

```
 agents / hosts / humans
   │   MCP (tool/data access)     A2A (delegation)     HTTP/WS (approval console)
   ▼
 ┌────────────── Protocol Adapters (thin, swappable) ────────────────┐
 │  MCP Server (JSON-RPC)       A2A Agent Card (RPC)   REST Gateway  │
 └───────────────────────────────┬───────────────────────────────────┘
                                 ▼
 ┌───────────────────────────────────────────────────────────────────┐
 │                   Kubona Capability Core                          │
 │  • Gateway dispatch & authorization (Principal, Scopes)          │
 │  • Hardware Cost Governance (Pre-flight, ceilings, approval gate) │
 │  • Execution & Federation (Provider abstraction, session handles) │
 │  • Provenance & Epistemics (Content-addressed ledger, anchors)    │
 └───────────────────────────────┬───────────────────────────────────┘
                                 ▼
                  QIR / OpenQASM 3 / MLIR  →  Provider SDKs / QPUs
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
