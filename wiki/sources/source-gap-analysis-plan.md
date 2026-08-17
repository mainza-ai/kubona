---
type: source
title: Kubona — Gap Analysis, Architectural Expansion, and Implementation Plan
date: 2026-08-16
tags: [source, plan]
status: ingested
---

# Source: Gap Analysis & Implementation Plan

> **File:** `raw/Quantum_AI_Platform_Gap_Analysis_and_Implementation_Plan.md`
> Companion to [[source-architecture-paper]]. The operational blueprint.

## What it adds

- Names the platform **Kubona** (Tonga (Zambia) — *to see*) and commits to a fully
open-source (Apache-2.0)
  posture with no closed-platform dependency.
- **10 gap findings** (major): provider coverage 4→11+; [[qbraid]]/[[strangeworks]] prior
  art; [[security-and-pqc]] underspecified; no orchestration-compat layer; no cost
  governance system; no provenance layer; evals are code-gen-only; thin client scope;
  licensing/export control unaddressed; OSS defensibility undefined.
- **Expanded architecture:** four federated [[mcp-and-context-flooding|MCP servers]]
  (compiler / execution / cost-governance / eval-provenance) behind an MCP Gateway
  (OAuth 2.1 + PQC-hybrid); `job_id` session abstraction; [[provider-capability-schema]]
  as data, not code; content-addressed [[provenance-and-reproducibility|experiment ledger]].
- **UI/UX section (§5):** six console surfaces, grounded in circuit-viz prior art
  ([[qflow-studio]], IBM Quantum Composer) and 2026 agent-UX practice; read/approve-only
  posture; generative UI deferred to v2 — see [[agent-ux-control-plane]].
- **Stack update:** Rust gains cost-governance + audit-ledger; Go is optional for canary CI.
- **Roadmap:** Phases 0–5 with exit criteria and team shape — see [[roadmap]].
- **[[karpathy-autoresearch]] engine (§11):** a pluggable harness registry
  (fixed_setup / mutable_artifact / metric / validity_check) for VQE ansatz search,
  QAOA, mitigation-strategy, compilation-pass and correctness-regression harnesses, with
  structural write-scope + simulator-only defaults, producing a validated algorithm library.
- **Agent resource economy (§12):** adopts [[x402]] as an optional payment rail on the
  cost-governance server, positions the algorithm library as a niche data refinery, and
  flags an agent-readiness-audit offering for providers.
- **Meta-search & true self-evolution (§13, rev 2):** a second tier above §11's object-level
  loop that revises *how* Kubona searches — proposal strategy, `search.md` instruction
  files, and harness-registry drafts — with the §13.3 flywheel (algorithm library → retrain
  GQE → better proposal strategy → better candidates) as the compounding mechanism. Stricter
  safety tier: versioned/reversible meta-changes, shadow-mode staged rollout, capability-level
  human sign-off distinct from the dollar gate, and write scope that never reaches the
  compiler/MCP/security layer. See [[meta-search-and-self-evolution]]; Phase 4 stretch goal
  gated on §11 shipping.
- **Science discovery & collaboration (rev 3, §14):** the platform is defined as, in
  addition to a quantum toolset, an **AI-enhanced science discovery platform** — keypair identities for every
  actor with instantly-revocable delegated authority ([[user-identity-and-profiles]]),
  user/agent-created workflows & agents behind versioned, approval-gated authoring
  ([[workflows-and-agent-authoring]]), the 7-stage discovery loop with autonomous
  experiment campaigns and a results contract (ledger record + structured report,
  [[science-discovery]]), Tier-1/Tier-2 scholarly & domain data shortlist from the
  public-apis catalog ([[science-data-integrations]]), and Unsloth as the GQE-flywheel /
  persona training substrate ([[unsloth]]). Adds Phase 6 (Weeks 37+) to the roadmap.
- **Naming (rev 4):** the project is named **Kubona** (Tonga (Zambia) — *to see*). See
  [[kubona]] for the name's meaning and mission fit.

## Pages this source seeded or expanded

[[kubona]] · [[qbraid]] · [[strangeworks]] · [[arxiv-mcp-quantum]] · [[security-and-pqc]] ·
[[cost-governance]] · [[provenance-and-reproducibility]] · [[provider-capability-schema]] ·
[[agent-ux-control-plane]] · [[ibm-quantum-composer]] · [[evaluation-and-benchmarks]] ·
[[karpathy-autoresearch]] · [[x402]] · [[agent-resource-economy]] ·
[[mainza-milimo-foundation]] · [[quantum-cloud-pricing]] · [[design-evolution]] ·
[[risks]] · [[meta-search-and-self-evolution]] · [[science-discovery]] ·
[[user-identity-and-profiles]] · [[workflows-and-agent-authoring]] ·
[[science-data-integrations]] · [[buzz]] · [[unsloth]] · [[roadmap]]