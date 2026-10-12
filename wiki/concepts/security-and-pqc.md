---
type: concept
name: Security & PQC
date: 2026-08-16
updated: 2026-10-12
tags: [security, pqc, oauth, mcp]
sources: [source-gap-analysis-plan, source-architecture-paper]
status: adopted
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

## Transport decision (2026-10-12, E-1e)

Measured state: mandate-envelope ML-KEM-768 is live and tested
(`pkg/mandates/pqc.go`, parameter sizes + tamper + wrong-key pinned);
the Go gateway, the Python bridge wire, and the console API all run
plain HTTP. "Hybrid post-quantum transport" above therefore describes
the envelope layer, **not** the wire — claiming otherwise would be the
kind of fiction this wiki exists to prevent.

Decision: TLS termination at the deployment edge (operator-owned certs,
documented in the runbook) carries confidentiality and server identity;
ML-KEM-768 mandate envelopes stay the PQC layer for spend authority;
KEMTLS is deferred until a mature Go implementation exists to evaluate.
Rejected: rolling custom transport crypto (no-bespoke-crypto rule), and
treating the Python `hybrid_transport` demo as production (it is
explicitly a stand-in).

Done looks like: every gateway/bridge/console HTTP surface behind TLS
in every non-loopback deployment, verified by a config test asserting
non-local binds refuse without TLS material; envelope-only mandate
transport (no bare mandates on any transport). Until then the posture is:
loopback-only dev, envelope-protected mandates, plain-HTTP LAN segments
declared — never implied secure.