---
type: concept
name: Security & PQC
date: 2026-08-16
tags: [security, pqc, oauth, mcp]
sources: [source-gap-analysis-plan, source-architecture-paper]
status: living
---

# Security Model & Post-Quantum Transport

The gap analysis upgrades the architecture paper's "RBAC + secrets manager" to a
2026-adequate model, because MCP servers holding live cloud-QPU credentials are now a
**documented attack target class** — a compromised session can exfiltrate proprietary
circuit IP and run up five/six-figure QPU bills before anyone notices
(source-gap-analysis-plan).

## The concrete model

- **OAuth 2.1 + scoped, short-lived tokens** replacing static API keys in every MCP
  handshake (table stakes, not aspirational).
- **Per-call, identity-aware authorization** — agent A may call `qiskit_simulator` but not
  `submit_production_qpu_job`; not coarse role buckets.
- **Hybrid post-quantum transport** — ML-KEM alongside classical TLS on the gateway, with
  *cryptographic agility* (swappable suites) from day one.
- **Immutable, append-only audit log** of every tool call, credential use, and dollar spent
  (doubles as the [[cost-governance]] ledger).
- **Sandboxed circuit-execution boundary** that rate-limits and financially caps *before* a
  job reaches a paid queue — not just RBAC on the tool call.

## Why PQC is non-negotiable for *this* platform

**Harvest-now-decrypt-later:** the platform's own outputs (optimized circuits, algorithm
results, potentially cryptographically sensitive computations) are exactly what an
adversary sitting on encrypted traffic today wants to decrypt once fault-tolerant hardware
exists. The transport is designed swappable in Phase 0, hardened with ML-KEM in Phase 3.

## Reconciliation with the wider wiki

The narrowest-scope reading of [[mcp-and-context-flooding]]'s "code-execution mode" also
requires this guardrail — a Python sandbox executing agent code is a prime target.

Related: [[mcp-and-context-flooding]] · [[cost-governance]] ·
[[provenance-and-reproducibility]] · [[kubona]]