---
type: overview
date: 2026-08-16
updated: 2026-09-27
tags: [kubona, synthesis]
sources: [source-architecture-paper, source-gap-analysis-plan, source-cloudflare-economy,
  source-frontier-repo-audit-2026]
status: living
---

# Kubona — Overview

> **The blueprint, not the build log.** This wiki exists so that Kubona can be *shaped*
> correctly before and while it is built: insights gathered, options compared, decisions
> recorded with their cost. Everything here must trace to a source; last ingest is noted
> in [[log]]. Unsettled items live in [[questions]] — that page, not the code, is the
> work-list. The wiki's job is to make this a compounding artifact, not a re-derived answer.
>
> Code may exist without the design being decided. Where a statement below is really a
> fact about the prototype rather than a design position, it says so.

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

## The four sources ingested

| Source | What it provides |
|---|---|
| [[source-architecture-paper]] | Theoretical foundation: polyglot stack, [[mcp-and-context-flooding]], [[qir-mlir-compilation]], provider/mitigation/eval survey |
| [[source-gap-analysis-plan]] | Buildable plan: gaps, prior art, federated architecture, phases 0–5, [[karpathy-autoresearch]] engine, x402 monetization |
| [[source-cloudflare-economy]] | The agent resource economy: why Kubona's outputs are monetizable, [[x402]] rail, startup patterns |
| [[source-frontier-repo-audit-2026]] | 25 live repos read end-to-end (remote · HEAD · licence · `path:line`): [[integration-topology-and-licence-discipline]], [[quantum-advantage-realism]], prior-art primitives for memory, governance and packs |

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

7. **Nothing enters the tree on vibes.** The frontier audit forced an explicit licence/integration
   discipline: submodule / sidecar / blueprint / reference, with **"submodule" a shipping decision,
   not a compliment** — reading a repo yields a citation plus an independent implementation, not a
   dependency ([[integration-topology-and-licence-discipline]]). The audit's real subject is
   *third-party* code: the three unlicensed repos it found are the maintainer's own unfinished WIP,
   which is insight material rather than a licence problem. MPL-2.0's file-level copyleft is
   precisely why [[openbao]] runs as a sidecar. Paired with [[quantum-advantage-realism]]: the
   platform records the backend that actually ran, always computes a classical anchor, and never
   claims advantage.

- **Provenance hygiene (2026-09-27):** the `ghp_…` PAT in a gitignored clone's remote URL is
  **verified dead** (401), but the same sweep found a *live* `gho_…` token sitting in plaintext in
  `~/.git-credentials` because `credential.helper=store` is set globally — an operator-machine
  problem, not a repo problem, and the one secret still open
  ([[risks]], [[secrets-and-workload-isolation]]). One folder cited as external prior art turned out
  to be a clone of Kubona's own repository ([[source-frontier-repo-audit-2026]]). The clone folder is scratch and gets deleted; claims must
  resolve to a live URL + SHA + `path:line`. Own repos with no `LICENSE` are **not** on this list:
  insight mining needs no grant, and the only check is at release time.
## Where the blueprint is unsettled

The full ledger of undecided and under-understood items is [[questions]] — 26 ranked
open questions, each with what is known, what is missing, and what the answer unblocks.
Three tensions are worth naming here because they are strategic rather than technical:

- **Open-source vs. qBraid's 34-device lead.** Kubona deliberately starts with 2–3
  providers and differentiates on the agentic governance layer. Is that enough? See
  [[risks]] for the full register, and [[questions]] Q-21 for the provider choice.
- **Export control / jurisdiction** across autonomous cross-border job routing — scoped
  to Phase 5, still unresolved at the level of *who is liable*, not which API is called.
- **Core vs. v2 for science discovery.** Roughly 40% of the concept pages describe the
  discovery platform; roughly one phase and no human surface does. That mismatch is
  [[questions]] Q-17, and it is the single largest shape decision left.

> ⚑ Correction: this page previously listed *"six-language stack risk
> (Rust/TS/Dart/Py/JS/±Go)"* as live. Verified 2026-09-27: the prototype contains
> Python (141 files), Rust (2 sources) and Dart (12 files) only — the `.ts/.js` files in
> the tree are venv and Flutter build artefacts. The stack collapsed to three without a
> decision being recorded, which is a win the wiki never claimed. Recorded here so the
> reversal stays visible.

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

## Blueprint status & prototype evidence

**Two registers, kept apart deliberately.** This wiki is the **blueprint**: insights,
options, and decisions that shape Kubona. `platform/` is a **prototype scaffold** written
*before* most of the insights here were ingested. Its passing tests are evidence about
mechanisms — what is easy, what is hard, what a real SDK actually demands — and are *not*
evidence that a design decision has been made. [[questions]] Q-01 asks what the
prototype's role should be; until that is answered, no phase gate here counts as a
blueprint milestone.

**Status census (mechanical, 2026-09-27).** 82 pages: `settled` 1 · `adopted` 25 ·
`open-question` 13 · `candidate` 6 · `tracked` 23 · `reference` 5 · `source` 4 ·
`living` 5 (meta). Vocabulary: [[index]] §status legend. Read it as *decidedness*, never
as completion: one settled policy against 25 adopted mechanisms means the blueprint is
still mostly "understood", not "chosen".

### What the prototype has empirically answered

`platform/` — 154 Python + 3 Rust + 1 Flutter widget test, all green locally as of
2026-09-27. Each phase below had a gate test, and each passes **on simulators**:

- **P0** federated MCP servers + gateway + provider trait + local simulator + capability
  schema + Apache-2.0 → the four-server split is workable over stdio MCP.
- **P1** sessions + cost governance + x402-compatible payments + ZNE + Rust `qir-core` →
  a budget ceiling genuinely stops a runaway VQE loop rather than merely logging it.
- **P2** 8 capability-schema entries (a phantom no-plugin `cuda-q` entry was removed),
  wire cutting, identity/delegation, loop supervisor + control server, LangGraph shim,
  4-tab Flutter console → a loop can be watched and interrupted over MCP.
- **P3** hybrid PQC transport (X25519+ML-KEM768), per-call identity-aware auth, immutable
  ledger audit, canary CI, provenance-as-resource → tampering is rejected on the wire.
- **P4** autoresearch engine (four-part harness contract), Search Mode beating a
  hand-designed baseline, GQE 4th harness, algorithm library, §13 meta-loop, behaviour
  evals, provider-registration SDK → the compounding-asset idea runs as code.
- **P5** xanadu + dwave breadth, export-control routing, multi-tenant scale, x402 +
  library metering, meta-loop auto-apply (staged).
- **P6** workflow engine, agent registry/personas (no escalation), experiment notebook,
  scholarly integrations, full discovery loop → a tenant-scoped lab completes
  ground→experiment→report→archive with every claim traced to a ledger or citation.

### What no prototype test can answer

These need a human, a wallet, or the outside world — which is why the roadmap's remaining
work is not "finishing the code": a **real QPU run** (P1/P2 gates say *real QPU*); a
**third-party security review** (P3); a **public repo plus a merged outside PR** (P4);
**business sign-off** on monetization (P5); and the **human surface**, whose spec is 8
surfaces and whose build is 4 tabs (P6, [[questions]] Q-22). Also outstanding: volume eval
corpora, a real payment rail, GQE retraining on a GPU host, and a `ci.yml` push pipeline
that has **never passed** (all 17 recorded runs fail, HEAD included) while `scripts/ci.sh`
runs green locally — so the release gate the roadmap treats as load-bearing is decorative
on the machine that ships code
([[implementation-gaps]]). **M0 closures landed:** single non-`/tmp` state root,
fail-closed production auth, `kubona serve`/`doctor` entrypoints, x402 fail-closed unless
a real verifier is injected. Full register: [[implementation-gaps]].

**Repo posture.** `github.com/mainza-ai/kubona`, **private**, Apache-2.0 planned at the
Phase-0 gate. That gate passed weeks ago and the flip has not happened — an open decision,
not an oversight, with two prerequisites: the `dev-docs/` + `.obsidian/` residue in pushed
history, and a live GitHub token in `~/.git-credentials` on the operator machine
([[questions]] Q-25, [[risks]]).
