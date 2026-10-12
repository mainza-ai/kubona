---
name: Agentic Memory and Observability
type: concept
tags: [frontier, memory, observability, multi-agent, transparency, hindsight]
date: 2026-09-27
updated: 2026-09-27
status: open-question
sources: [source-frontier-repo-audit-2026]
---

# Agentic Memory and Observability

**Definition.** One inspectable surface where every agent thought, tool call, refusal and
revision is *simultaneously* remembered (so the collective learns) and visible (so a human can
audit it in minutes). These are the same pipe: a fact that is not retrievable later is not
observable now, and a log nobody can query is not memory. Substrate for Rings 0-1 of
[[five-ring-research-loop]].

## Three layers, one store

| Layer | Contents | Substrate | Purpose |
|---|---|---|---|
| **Episodic** | append-only records of every run, tool call, refusal, budget event | the existing ledger (`kubona/mcp/servers/provenance.py`) | ground truth; never mutated, never a side DB ([[provenance-and-reproducibility]]) |
| **Semantic** | entities, relations, temporal facts distilled from episodes ("which ansatz failed on which device, when") | [[hindsight]] sidecar | raw logs cannot answer "what did we conclude last month" |
| **Reflective** | strategy revisions, weight deltas | [[meta-search-and-self-evolution]] (GQE) | learning that is not written down is drift |

## Why a memory sidecar, and why this one

`hindsight` (MIT) is the only audited component shipping the full loop as a service:
**retain / recall / reflect** over named *memory banks* (per agent, per user, per domain),
entity + relation extraction, temporal reasoning, batch operations, PostgreSQL + pgvector, and —
decisive for Kubona — an **MCP server and an OpenAI-compatible proxy** (arXiv:2512.12818;
LongMemEval +41.9% vs its RAG baseline *(doc claim)*). Its `reflect` op is a *budgeted*
background synthesis with `max_tokens`/`max_calls` ceilings — the [[agent-resource-economy]]
shape — and MCP `retain` carries `density`/`importance` metadata, so the ledger decides what is
worth remembering rather than the LLM. Adopt as a **sidecar** (degrade to "no memory" when
absent), never inside the Rust core. Then enforce the two rules its docs make easy to violate:

1. **Memory banks are label-scoped.** A bank per (tenant × campaign) via
   [[confidentiality-lattice-provenance]]: `recall` filters by readership *before* tokens are
   spent, or the memory service is a cross-tenant context leak with good documentation
   ([[mcp-and-context-flooding]]).
2. **Derived memories cite episodic records.** A semantic fact with no ledger backing is a
   hallucination with an index — the provenance rule of [[claim-credibility-wavefunctions]].

## The lifecycle to copy — and the half to refuse

[[mainza-consciousness]] (MIT, the maintainer's own repo) has defensible memory engineering
buried inside a package whose quantum claims are `np.random.dirichlet` draws
(`backend/utils/quantum_consciousness_engine.py:331`). The half worth copying is
`backend/utils/advanced_memory_architecture.py` (655 LOC): `MemoryType` (:18) and
`MemoryConsolidationLevel` (:29) as enums; `AdvancedMemory` carrying `decay_rate: 0.95` (:51)
against an architecture-level `decay_threshold = 0.3` (:73); separate write paths
`create_episodic_memory` (:76), `create_emotional_memory` (:119), `create_consciousness_memory`
(:162); typed retrieval `retrieve_memories_by_type` (:202, bounded by `LIMIT $limit`);
`consolidate_memories` (:273); `create_memory_associations` (:339) backed by a Neo4j graph
(`neo4j_production`, imported at :13); and scoring hooks `_calculate_episodic_importance`
(:384) / `_calculate_consolidation_strength` (:431) where time decay is `max(0.1, 1 − 0.01t)`
(:441). That maps onto our three layers almost one-for-one, and re-implements cleanly inside our
licence as a **memory lifecycle**: activate on recall, decay what is unused, consolidate into
semantic banks. Two gaps it leaves open, which Kubona must close: retrieval is `LIMIT`-bounded
but **no path deletes or evicts** (grep for `DELETE`/`evict` returns nothing) — so hard caps with
LRU eviction are ours to add, as `wiki/concepts/risks.md` already demands; and the consolidation
score is a heuristic with no held-out check, so it must be gated by [[evaluation-and-benchmarks]] before
it is allowed to promote anything. The theatrical half (EM-field "memories", Dirichlet
"consciousness") stays refused: a mechanism with no falsifiable consequence does not earn a slot
in a research platform.

## Observability: watch the collective, not the container

[[gods-eye-view]] (MIT, `bilawalsidhu/gods-eye-view`, HEAD 2026-09-24) is the strongest audited
control-room pattern, and its lessons are architectural:

- **Policy per layer, seam enforced.** One `policy.js` per data layer
  (`src/layers/{traffic,vessels,launches,flights,radio,localAdsb}/policy.js`) plus
  machine-checked boundaries: `scripts/check-import-directions.mjs`,
  `scripts/check-package-boundaries.mjs`, `docs/CODE-BOUNDARIES.md`. Policy that only lives in
  prose is not policy; policy that nothing checks drifts.
- **Cost tiers decided *before* the call.** `src/voice/voiceCost.js` resolves a tier
  (`resolveVoiceModel`, :101), normalises limits (`normalizeCostLimits`, :192), splits usage
  tokens for settlement (:252), and — the part that matters — `mostExpensiveVoiceModel()`
  (:122-125) picks the priciest entry by `rates.audioOutput` so an unknown model is priced at
  its **worst case**, with `voiceCost.test.mjs` beside it. That is [[cost-governance]]'s
  estimate → pre-flight → settlement applied to an expensive modality.
- **Budgets as CI tests.** `scripts/focus-allocation-check.mjs` runs 100 000 calls × 9 trials
  under `node --expose-gc` and asserts a median against named budgets
  (`PURE_CALL_BUDGET = 16`, `PRODUCTION_SPRITE_TICK_BUDGET = 212`) — a performance budget that
  fails a build, which is how Kubona should gate its own hot paths (ledger write, compile).
- **Federated user-owned feeds, guarded.** `server/providers/local-receivers.js` admits only
  env-listed endpoints, gates them through `isLocalIpv4` (:146, :295) and lets the feed's own
  clock decide staleness (:376), documented in `docs/LOCAL-RECEIVERS.md`. Real instruments that
  cannot be aimed at the network ([[secrets-and-workload-isolation]]).

Add the **signed event log** from [[buzz]] — `buzz/ARCHITECTURE.md:503`: *"Tamper-evident
append-only log with SHA-256 hash chaining"*, with agent identity handled as keys
(`docs/agent-profile-identity.md`, `docs/agent-management-provenance.md`) — with workflow
triggers fired *fire-and-forget* so a notification never blocks the transition it reports.
Kubona's equivalent is a `collective`
server emitting signed events (proposed / refused / spent / revised) into the ledger. **Refusals
and budget denials are events with equal rights to successes**; a dashboard that shows only wins
is how a fleet learns to hide failure.

## Landing

`platform/kubona/epistemics/curvature.py` (κ/drift diagnostics, tested) ·
Go `pkg/agents/collective.go` tribunal + ledger-signed events (`AppendSignedEvent`,
tribunal turns/audits) · [[hindsight]] behind `memory_retain`/`memory_recall`
with per-tenant banks + HTTP recall/retain (`/api/memory/*`) + `MemoryPanel`
> ⚑ Correction (2026-10-12): this section previously cited
> `platform/kubona/observability/` and `kubona/mcp/servers/collective.py`
> — neither path ever existed. Signed events live in the Go ledger;
> memory lives in `pkg/memory/hindsight.go` + HTTP surface. The
> lifecycle-in-Rust-core and one-console-view-per-campaign items remain
> open (see Q-17 CORE-ENGINE).

**Acceptance test:** a cold human auditor answers "why did the agent stop?" and "what did it
spend?" in under 60 seconds — a measurable gate, added to [[evaluation-and-benchmarks]].

## pgvector sidecar: dormant by design (Q-34, closed 2026-10-12)

`platform/core/pkg/memory/postgres.go` implements a complete PostgreSQL + pgvector sidecar
(HNSW semantic recall, distributed locks) that **nothing constructs** — verified zero
non-test references to `NewPostgresVectorStore`/`Postgres*` outside the file; the live
recall path is the SQLite hindsight store in the same package. Labelled dormant in-code
(Q-34 banner) on the Q-23/x402 precedent: real code, honestly marked, never half-wired.
Activation needs an operator-provisioned PG + pgvector instance, a driver dependency, and
recall-path wiring. Pinned by `TestPostgresStoreDormantContract`: unset `KUBONA_PG_DSN`
means unconfigured, and every method refuses a nil DB fail-closed.
