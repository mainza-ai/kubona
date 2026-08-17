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

Kubona — Tonga (Zambia), *"to see"* — is the name for an **open-source, MCP-native
quantum-computing platform for autonomous AI agents** — a toolset that lets an LLM agent author, optimize, and
execute quantum algorithms across a federation of hardware providers
(IBM, IonQ, Quantinuum, Rigetti, D-Wave, Azure, Google, QuEra/Pasqal, Xanadu, and more)
behind a single agent interface (source-gap-analysis-plan). The core ambition: turn
quantum processors from isolated scientific instruments into co-processors that an
AI agent can navigate directly (architecture-paper). Kubona is also an **AI-enhanced
science discovery platform**: multi-user labs, user- and agent-created workflows and
agents, and the ability to conduct its own experiments and return organized,
reproducible results — see [[science-discovery]].

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
- **Mainza/Milimo reuse claims** — repo-level verified 2026-08-16 ([[mainza-milimo-foundation]]:
  milimoquantum, MilimoClaw, afreval are public and match the headline claims); code-level
  specifics still need inspection before lifting patterns.
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

## Implementation status & next steps (see also [[roadmap]])

**As of 2026-08-16, Phases 0–6 are implemented at the skeleton/engine level**
under `platform/` (105 Python tests + 3 Rust + 1 Flutter widget test, all green;
repo `github.com/mainza-ai/kubona`, currently private per maintainer decision):

- **P0** federated MCP servers + gateway + provider trait + local simulator +
  capability schema + Apache-2.0 license — gate proven end-to-end over stdio MCP.
- **P1** sessions + cost governance + x402-compatible payments + ZNE + QIR
  (Rust `qir-core` crate) — gate proven (budget-enforced VQE loop).
- **P2** providers to 9, wire cutting, identity/delegation, loop supervisor +
  control MCP server, LangGraph shim, Dart/Flutter console (web) — gate proven
  (watch + interrupt a loop over MCP).
- **P3** hybrid PQC transport (X25519+ML-KEM768), per-call identity-aware auth,
  immutable ledger audit, canary CI (release-gating correctness harness),
  provenance-as-MCP-resource, 4-tab console.
- **P4** autoresearch engine (four-part harness contract + orchestrator), Search
  Mode (beats a hand-designed baseline), GQE 4th harness, algorithm library,
  §13 meta-loop, eval corpus + agent-behavior evals, provider-registration SDK.
- **P5** xanadu + dwave breadth, export-control routing, multi-tenant scale,
  x402 activation + library metering, meta-loop auto-apply (staged).
- **P6** workflow engine, agent registry/personas (no escalation), experiment
  notebook, scholarly integrations, full discovery loop — **P6 gate passes**
  (a tenant-scoped lab runs the loop; every claim traces to a ledger/citation).

**Remaining (production / hardening, not core-engine):** the async persistent
job model; real provider SDK execution (IBM Runtime, pytket, pyquil, dimod+Leap,
strawberryfields) — only local-simulator + qiskit-aer execute today; the Rust
`qir-core` core on the compiler call path; streamable-HTTP MCP + ML-KEM on the
wire + secrets management; volume eval corpora (QuantumKatas/HumanEval/
PennyLang); a real third-party provider PR merged; the third-party gateway
security review; real x402 on a payment rail; GQE/Unsloth retraining on a GPU
host. **M0 closures landed:** a single non-`/tmp` state root (all stores via
`kubona/config.py`), production fail-closed token auth (`KUBONA_ENV=production`
+ `aud`/`nbf` signed tokens via `kubona/run.py`), `kubona serve`/`doctor`
entrypoints, CI (`ci.yml` python/rust/canary-correctness-gate/Flutter +
`nightly-canary.yml`), and x402 **fail-closed** unless a real verifier is
injected. Full register: [[implementation-gaps]].