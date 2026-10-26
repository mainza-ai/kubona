---
type: entity
name: Kubona
date: 2026-08-16
updated: 2026-10-26
tags: [kubona, platform]
sources: [source-gap-analysis-plan]
status: adopted
---

# Kubona

**Kubona** — Chitonga (Tonga, Zambia), *ku-bona*, "to see" — is the name of the platform.
The name is the Chitonga infinitive: *ku-* is the Bantu Class-15 infinitive
prefix, *-bona* (Proto-Bantu *-bóna*) the root for seeing, observing,
perceiving. To see is the founding act of
science: observe, measure, record, understand. The platform surfaces quantum phenomena,
runs experiments, and shows their results systematically — so the name carries the mission
(see [[overview]]).

The platform: **the ultimate, completely open-source quantum-computing
toolset for AI agents — and an **AI-enhanced science discovery platform**: multi-user
labs, user/agent-created workflows & agents, and autonomous experiments with systematic,
reproducible results (see [[science-discovery]]) — unifying [[qiskit]], [[pennylane]], [[microsoft-azure-quantum|Q#]],
[[google-cirq]], and the major quantum cloud providers behind a single, agent-native
[[mcp-and-context-flooding|MCP]] interface (source-gap-analysis-plan).

## Settled v1 Scope & Negative Scope Boundary (Ruling 2026-10-26, Q-01)

Kubona v1 is the production agent-native platform for **Quantum Circuit Co-Design, Ground-Anchored Molecular Chemistry, Combinatorial Optimization, and 3D Generative Observation**.

- **v1 Executable Hardware Backends (5):**
  1. `local-simulator`: Ideal Aer statevector and shot-sampling simulation.
  2. `qiskit-aer-simulator`: Noisy density-matrix and Clifford gate execution via Qiskit Aer.
  3. `ibm-qiskit`: Real IBM Quantum Qiskit Runtime (`SamplerV2`), non-blocking deferred job handles, status polling, and hardware job recall.
  4. `quantinuum`: Real trapped-ion execution via `pytket` and `qnexus`, non-blocking polling, bitstring normalization.
  5. `dwave-annealing`: Exact/SA local solver and Leap Hybrid Sampler adapter.

- **v1 Science Modalities (4):**
  1. *Molecular Quantum Chemistry:* Ground-state VQE, active orbital space selection, dissociation curves (PES), and classical RHF/FCI anchors via PySCF.
  2. *Combinatorial Optimization:* Exact and heuristic QUBO / Ising formulations, MaxCut, graph partitioning.
  3. *Circuit Co-Design & QIR Compilation:* OpenQASM 2.0/3.0 lowering, Stim Clifford probes, PyZX spider fusion, Pauli propagation, full-radius Reed-Solomon decoding.
  4. *Spatial Observatory & 3D Synthesis:* Gaussian splat rendering, depth-map extraction, and World Labs Atlas RGBD/Chisel synthesis.

- **v1 Negative Scope (What v1 is NOT):**
  1. Kubona v1 does **not** ship execution support for `rigetti`, `ionq`, `aws-braket`, `neutral-atom-quera-pasqal`, or `xanadu-strawberry-fields`. These backends are catalogued with `dispatch_ready: false` and fail-closed with actionable refusal reasons until v2 drivers land.
  2. Kubona v1 does **not** support periodic boundary materials simulation or dynamic crystal spectroscopy (deferred to v2).
  3. Kubona v1 does **not** perform autonomous in-loop LLM weight training (`unsloth` GQE flywheel is deferred to v2).
  4. Kubona v1 is **not a multi-tenant cloud SaaS with federated OIDC** (external IdP and OAuth are deferred to Phase 4; local Ed25519 tenant lab keys and bearer cookies govern v1).

## Posture

- Greenfield build, licensed **Apache-2.0**.
- **Private by decision, not by accident** (ruled 2026-09-27). While the blueprint is being
  designed, opening the repository invites contributions before the project has anything to
  direct them into — so privacy is *load-bearing design protection*, not an unmet gate. The
  flip is gated on the blueprint becoming **contributor-legible** (Q-01 settled above, and a method layer that says what to build first), not on a date.

> ⚑ Correction: this page previously carried "public repo from Phase 0" as a live rule with an
> ⚑ *Conflict* block, because the Phase-0 gate passed weeks ago and the repository stayed
> private — which read as drift. The operator has since stated the intent, and it was never
> "public when tests pass": the trigger is **direction legible enough to onboard strangers**,
> which the blueprint has now achieved with the closure of Section A (Q-01). The Apache-2.0
> commitment stands; its timing follows the condition above. Residual hygiene work before any
> flip (private-remote, never-public `dev-docs/` + `.obsidian/` history) is tracked as [[questions]] Q-27; see
> [[implementation-gaps]] and [[risks]].

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
- **Control plane:** TypeScript/Next.js [[agent-ux-control-plane|read/approve-only console]] (Dart/Flutter retired Q-22).

## Monetization (future)

- [[x402]] payment rail on the cost-governance server (below the human approval
  threshold); algorithm library metering; provider agent-readiness audits — all
  sequenced after a working product.

## Costs & risks

Four-language production stack (Go/TS/Python/Rust); Dart retired. Export-control exposure from
autonomous cross-border routing; MCP credential compromise → runaway spend (mitigated by
[[cost-governance]] being Phase-1 non-optional).
See [[overview]] and [[roadmap]].