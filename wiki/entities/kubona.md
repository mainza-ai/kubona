---
type: entity
name: Kubona
date: 2026-08-16
tags: [kubona, platform]
sources: [source-gap-analysis-plan]
status: concept-stage
---

# Kubona

**Kubona** — from Tonga (Zambia), *"to see"* — is the name of the platform. To see is the founding act of
science: observe, measure, record, understand. The platform surfaces quantum phenomena,
runs experiments, and shows their results systematically — so the name carries the mission
(see [[overview]]).

The platform: **the ultimate, completely open-source quantum-computing
toolset for AI agents — and an AI-enhanced science discovery platform** (maintainer
synthesis expansion: multi-user labs, user/agent-created workflows & agents, and
autonomous experiments with systematic, reproducible results — see
[[science-discovery]]) — unifying [[qiskit]], [[pennylane]], [[microsoft-azure-quantum|Q#]],
[[google-cirq]], and the major quantum cloud providers behind a single, agent-native
[[mcp-and-context-flooding|MCP]] interface (source-gap-analysis-plan).

## Posture

- Greenfield build, licensed **Apache-2.0**, public repo from Phase 0.
- Closed platforms ([[qbraid]], [[strangeworks]]) are design references and *optional,
  swappable* interop connectors — **never** load-bearing dependencies.
- Defensible product = the **agentic control/governance layer** (cost governance,
  provenance, per-call auth, routing) nobody else has shipped; the hardware-abstraction
  foundation underneath it must be genuinely open source.

## Architecture at a glance

- **Federated MCP servers:** compiler · execution · cost-governance · eval/provenance,
  behind an MCP Gateway (OAuth 2.1, PQC-hybrid transport, identity-aware per-call RBAC).
- **Compilation:** [[qir-mlir-compilation]] via a Rust core (WASM edge, PyO3 bindings).
- **Session model:** `job_id` threaded through every call; state rebuilt from the
  provenance ledger.
- **Providers (11+):** IBM · IonQ · Quantinuum · Rigetti · Azure · Google · D-Wave ·
  QuEra · Pasqal · Xanadu · local/GPU simulator — onboarded as
  [[provider-capability-schema|data, not code]].
- **Control plane:** Dart/Flutter [[agent-ux-control-plane|read/approve-only console]].

## Monetization (future)

- [[x402]] payment rail on the cost-governance server (below the human approval
  threshold); algorithm library metering; provider agent-readiness audits — all
  sequenced after a working product.

## Costs & risks

Six-language stack (Rust/TS/Dart/JS/Py/±Go) hiring burden; export-control exposure from
autonomous cross-border routing; MCP credential compromise → runaway spend (mitigated by
[[cost-governance]] being Phase-1 non-optional).
See [[overview]] and [[roadmap]].