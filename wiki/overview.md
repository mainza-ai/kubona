---
type: overview
date: 2026-08-16
tags: [kubona, synthesis]
sources: [source-architecture-paper, source-gap-analysis-plan, source-cloudflare-economy]
status: living
---

# Kubona — Overview

> The synthesis page. Everything here must trace to a source; last ingest
> is noted in [[log]]. The wiki's job is to make this a compounding artifact,
> not a re-derived answer.

## What Kubona is

Kubona — Tonga (Zambia), *"to see"*; formerly **QAgent** (historical working name) — is the
name for an **open-source, MCP-native quantum-computing platform
for autonomous AI agents** — a toolset that lets an LLM agent author, optimize, and
execute quantum algorithms across a federation of hardware providers
(IBM, IonQ, Quantinuum, Rigetti, D-Wave, Azure, Google, QuEra/Pasqal, Xanadu, and more)
behind a single agent interface (source-gap-analysis-plan). The core ambition: turn
quantum processors from isolated scientific instruments into co-processors that an
AI agent can navigate directly (architecture-paper). The platform is expanding into an
**AI-enhanced science discovery platform** (maintainer synthesis): multi-user labs,
user- and agent-created workflows and agents, and the ability to conduct its own
experiments and return organized, reproducible results — see [[science-discovery]].

## The three sources ingested

| Source | What it provides |
|---|---|
| [[source-architecture-paper]] | Theoretical foundation: polyglot stack, [[mcp-and-context-flooding]], [[qir-mlir-compilation]], provider/mitigation/eval survey |
| [[source-gap-analysis-plan]] | Buildable plan: gaps, prior art, federated architecture, phases 0–5, [[karpathy-autoresearch]] engine, x402 monetization |
| [[source-cloudflare-economy]] | The agent resource economy: why Kubona's outputs are monetizable, [[x402]] rail, startup patterns |

## The synthesis (current best understanding)

1. **Build the open layer first.** Kubona's defensible contribution is a fully
   open-source (Apache-2.0) hardware-abstraction + governance layer. Closed platforms
   ([[qbraid]], [[strangeworks]]) are design references and optional interop targets,
   never load-bearing dependencies (source-gap-analysis-plan). The genuinely novel piece
   is the **agentic control/governance layer** — nobody has shipped agent-facing cost
   governance, provenance, or per-call auth for quantum hardware yet.

2. **The architecture is four federated MCP servers** — compiler, execution,
   cost-governance, eval/provenance — behind an [[mcp-and-context-flooding|MCP Gateway]]
   with OAuth 2.1 + PQC transport, a `job_id` session abstraction, and a data-driven
   [[provider-capability-schema]]. Compilation standardizes on [[qir-mlir-compilation]]
   with a Rust core.

3. **Cost governance and provenance are non-optional rail, not add-ons.** A single
   mis-configured VQE run can burn thousands of dollars in minutes. Mandatory simulator
   pre-flight, hard budget ceilings, a two-stage approval gate, immutable audit ledger:
   these are Phase-0/1 features (source-gap-analysis-plan).

4. **The algorithm library is the compounding asset.** The [[karpathy-autoresearch]]
   harness engine autonomously searches ansätze, mitigation configs, and compilation
   passes, accumulating a provenance-backed library of validated configurations — which
   doubles as Kubona's monetizable resource under the [[agent-resource-economy]] thesis
   (source-cloudflare-economy). A planned **meta-level tier** (§13 of the gap-analysis plan)
   closes the flywheel: the library becomes training data that periodically retrains GQE,
   and the improved model becomes the next ansatz-search proposal strategy — under a
   stricter safety tier (versioned/reversible changes, shadow-mode rollout, capability-level
   sign-off beyond the dollar gate, no write access to platform code). See
   [[meta-search-and-self-evolution]].

5. **The human-facing surface is a read/approve-only control plane** — circuit
   visualizer, cost/approval console, live loop inspector — grounded in both
   [[agent-ux-control-plane|agent UX]] practice — including the
   circuit-visualization design space.

6. **Kubona is also a science discovery platform.** The same governance rail that makes
   hardware spend safe makes literature grounding, experiment design, and structured
   reporting safe: keypair identities for every actor ([[user-identity-and-profiles]]),
   user/agent-created workflows & agents behind versioned, approval-gated authoring
   ([[workflows-and-agent-authoring]]), scholarly/data integrations as governed MCP
   resources ([[science-data-integrations]]), and an experiment notebook that turns every
   run into an organized, citable result ([[science-discovery]]). The [[karpathy-autoresearch]]
   engine already gives the platform the ability to run autonomous experiment campaigns;
   Phase 6 assembles the collaboration layer on top ([[roadmap]]).

## Open questions / tensions

- **Open-source vs. qBraid's 34-device lead:** Kubona deliberately starts with 2–3
  providers and differentiates on the agentic governance layer. Is that enough?
  See [[risks]] for the full register.
- **Six-language stack risk** (Rust/TS/Dart/Py/JS/±Go); Go is optional, Rust can absorb it.
- **Export control / jurisdiction** across autonomous cross-border job routing — scoped to
  Phase 5 ([[risks]]).
- **When to activate x402**: designed for in Phase 1, activated Phase 5+; the "functional
  product first" sequencing is deliberate (source-gap-analysis-plan).
- **Mainza/Milimo reuse claims** are single-source and unverified — validate before building
  on them ([[mainza-milimo-foundation]]).
- **User/agent-created workflows widen the trust surface** — how much autonomy do
  agent-created agents get, and who approves capability changes? See the §13.4-style safety
  rail in [[workflows-and-agent-authoring]].
- **Science-data licensing/TOU** — fetched literature and datasets must carry license flags
  and citation provenance ([[science-data-integrations]]); which sources are free enough
  for autonomous ingestion?

## How design moved from paper to plan

See [[design-evolution]] — the full §10 "what changed" table (4→11+ providers, RBAC→OAuth
+PQC, monolith→federated MCP, heuristic→hard cost governance, no provenance→content-addressed
ledger, code-gen evals only→canary CI + behavior evals).

## Next steps (see also [[roadmap]])

- Phase 0 foundations: federated MCP skeleton, OAuth 2.1, open-source provider
  abstraction (Qiskit + local/GPU simulator), Apache-2.0 repo.
- A lint pass and a first compilation-track web search for competitive signals in
  the MCP-for-quantum space ([[arxiv-mcp-quantum]] referenced as prior art).
- Validate the [[mainza-milimo-foundation|Mainza/Milimo]] reuse claims against the real
  codebases before lifting HAL/agent-mesh/approval patterns.
- Draft the workflow-authoring DSL and identity/delegation model (Phase 6 groundwork) and
  shortlist Tier-1 scholarly integrations (arXiv, OpenAlex) — see
  [[workflows-and-agent-authoring]] and [[science-data-integrations]].