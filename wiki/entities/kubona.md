---
type: entity
name: Kubona
date: 2026-08-16
updated: 2026-09-27
tags: [kubona, platform]
sources: [source-gap-analysis-plan]
status: open-question
---

# Kubona

**Kubona** — from Tonga (Zambia), *"to see"* — is the name of the platform. To see is the founding act of
science: observe, measure, record, understand. The platform surfaces quantum phenomena,
runs experiments, and shows their results systematically — so the name carries the mission
(see [[overview]]).

The platform: **the ultimate, completely open-source quantum-computing
toolset for AI agents — and an **AI-enhanced science discovery platform**: multi-user
labs, user/agent-created workflows & agents, and autonomous experiments with systematic,
reproducible results (see [[science-discovery]]) — unifying [[qiskit]], [[pennylane]], [[microsoft-azure-quantum|Q#]],
[[google-cirq]], and the major quantum cloud providers behind a single, agent-native
[[mcp-and-context-flooding|MCP]] interface (source-gap-analysis-plan).

## Posture

- Greenfield build, licensed **Apache-2.0**.
- **Private by decision, not by accident** (ruled 2026-09-27). While the blueprint is being
  designed, opening the repository invites contributions before the project has anything to
  direct them into — so privacy is *load-bearing design protection*, not an unmet gate. The
  flip is gated on the blueprint becoming **contributor-legible** (a stated role for `platform/`
  per [[questions]] Q-01, and a method layer that says what to build first), not on a date.

> ⚑ Correction: this page previously carried "public repo from Phase 0" as a live rule with an
> ⚑ *Conflict* block, because the Phase-0 gate passed weeks ago and the repository stayed
> private — which read as drift. The operator has since stated the intent, and it was never
> "public when tests pass": the trigger is **direction legible enough to onboard strangers**,
> which the blueprint does not yet have (1 `settled` page against 24 `adopted`). The Apache-2.0
> commitment stands; its timing follows the condition above. Residual hygiene work before any
> flip (pushed `dev-docs/` + `.obsidian/` history) is tracked as [[questions]] Q-27; see
> [[implementation-gaps]] and [[risks]].

- Closed platforms ([[qbraid]], [[strangeworks]]) are design references and *optional,
  swappable* interop connectors — **never** load-bearing dependencies.
- Defensible product = the **agentic control/governance layer** (cost governance,
  provenance, per-call auth, routing) nobody else has shipped; the hardware-abstraction
  foundation underneath it must be genuinely open source.
- **`status: open-question` is deliberate for this page.** The platform's own definition is
  the least-settled page in the wiki: scope, the discovery-layer commitment, and the
  autonomy model are all still being shaped. See [[questions]] Q-17, Q-10, Q-01.

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