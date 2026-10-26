---
type: overview
date: 2026-08-16
updated: 2026-10-23
tags: [kubona, synthesis]
sources: [source-architecture-paper, source-gap-analysis-plan, source-cloudflare-economy,
  source-frontier-repo-audit-2026, source-agent-protocol-landscape-2026, source-openai-math-corpus,
  source-leanrea-investigation-2026]
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

Kubona — Chitonga (Tonga, Zambia), *ku-bona*, "to see" — is the name for an **open-source, MCP-native
quantum-computing platform for autonomous AI agents** — a toolset that lets an LLM agent author, optimize, and
execute quantum algorithms across a federation of hardware providers
(IBM, IonQ, Quantinuum, Rigetti, D-Wave, Azure, Google, QuEra/Pasqal, Xanadu, and more)
behind a single agent interface (source-gap-analysis-plan). The core ambition: turn
quantum processors from isolated scientific instruments into co-processors that an
AI agent can navigate directly (architecture-paper). Kubona is also an **AI-enhanced
science discovery platform**: multi-user labs, user- and agent-created workflows and
agents, and the ability to conduct its own experiments and return organized,
reproducible results — see [[science-discovery]].

The name is the Chitonga infinitive *ku-bona*: *ku-* is the Bantu Class-15
infinitive prefix, *-bona* (Proto-Bantu *-bóna*) the root for seeing,
observing, perceiving. The platform is held to it literally — a claim the
system surfaces must trace to something observed (a measured result, a
ledger receipt, a cited anchor), never to an assertion.

## The nine sources ingested

| Source | What it provides |
|---|---|
| [[source-architecture-paper]] | Theoretical foundation: polyglot stack, [[mcp-and-context-flooding]], [[qir-mlir-compilation]], provider/mitigation/eval survey |
| [[source-gap-analysis-plan]] | Buildable plan: gaps, prior art, federated architecture, phases 0–5, [[karpathy-autoresearch]] engine, x402 monetization |
| [[source-cloudflare-economy]] | The agent resource economy: why Kubona's outputs are monetizable, [[x402]] rail, startup patterns |
| [[source-frontier-repo-audit-2026]] | 25 live repos read end-to-end (remote · HEAD · licence · `path:line`): [[integration-topology-and-licence-discipline]], [[quantum-advantage-realism]], prior-art primitives for memory, governance and packs |
| [[source-agent-protocol-landscape-2026]] | 2026 standards audit: MCP 2026-07-28 & SDK 2.x, AAIF, A2A delegation, AP2 mandates, Qiskit MCP servers, [[protocol-strategy]] |
| [[source-openai-math-corpus]] | 722 manuscripts in 372 families, 162 Lean 4 formalizations, 811 Comparator challenges: [[autonomous-mathematical-discovery]], [[formal-proof-verification-lean-comparator]], [[mathematical-physics-anchors]] |
| [[source-leanrea-investigation-2026]] | Live investigation of `aicodeking/leanrea` (`09ba4aa`, MIT): router + traps skill (`skills/kubona/SKILL.md`), benchmark harness (`bench/`), and 29-tool MCP catalog discipline ([[leanrea]], [[leanrea-adoption-plan]]) |
| [[source-llm-circuit-codesign-2026]] | Gu et al. `arXiv:2610.22367v1` (15 Oct 2026, unreproduced): evaluator-guided LLM search, kickmix validity, TQ-cost, routing/history-decode/failure-budget, 7 wins + divsteps: [[evaluator-guided-circuit-codesign]], [[reversible-co-design-patterns]], [[failure-budget-and-frontier]] |
| [[source-magic-state-cultivation-2026]] | Google Willow cultivation (Science Oct 2026, tracked): `~40×` cut, `99.99%` T-fidelity, d3-color→d5-surface graft; magic-state economics for costing |

## The synthesis (current best understanding)

1. **Build the open layer first.** Kubona's defensible contribution is a fully
   open-source (Apache-2.0) hardware-abstraction + governance layer. Closed platforms
   ([[qbraid]], [[strangeworks]]) are design references and optional interop targets,
   never load-bearing dependencies (source-gap-analysis-plan). The genuinely novel piece
   is the **agentic control/governance layer** — nobody has shipped agent-facing cost
   governance, provenance, or per-call auth for quantum hardware yet.

2. **The architecture decouples peripheral protocols from a Capability Core** — [[protocol-strategy]]:
   the core houses hardware cost governance, provider federation, QIR compilation, and the
   provenance ledger. External access is provided via thin edge adapters: [[mcp-and-context-flooding|MCP]]
   (2026-07-28 stateless core) for host/tool invocation, [[a2a|A2A]] for inter-agent delegation, and
   [[ap2|AP2]] signed mandates for hardware spend authorization. In code-execution mode, agents write
   Python/Rust scripts against the Capability Core inside a sandbox, reducing token overhead by up to 98.7%.

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

5. **The production architecture standardizes on Go + TypeScript/Next.js + Python + Rust:**
   The human-facing surface is a read/approve-only control plane implemented in **TypeScript + React/Next.js**
   (with node-based workflow canvases via React Flow, multiplayer presence via Yjs, and deep DOM paper selection),
   talking via WebSockets/gRPC-Web to a high-throughput **Go Backend Orchestrator** (managing thousands of concurrent
   agent loops via goroutines/channels and container sandbox lifecycles). Heavy quantum execution (Qiskit, PennyLane,
   PySCF) runs in dedicated **Python gRPC microservices**, while high-performance compilation runs in a **Rust core**.
   See [[ui-ux-vision]], [[protocol-strategy]], and [[agent-ux-control-plane]].

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
   cleanly compatible with an Apache-2.0 container as long as its files stay distinct.

8. **Cognitive Mesh, Sovereign Air-Gap, and Local-First Quantum Validation:**
   The multi-agent research collective operates across a hybrid LLM fabric configured through an environment- and
   settings-driven plane (`KUBONA_LLM_PROVIDER`, `*_MODEL`, `/api/llm/settings`, with zero hardcoded model IDs in
   code): local inference engines (oMLX on port 8787, Ollama on 11434, LM Studio on 1234) alongside cloud frontier
   providers (NVIDIA NIM, OpenCode, Anthropic, OpenAI). A Sovereign Air-Gap hardware toggle guarantees zero outbound
   network egress. Every quantum algorithm and ansatz must pass a strict **local classical-quantum testing gate**
   (classical ab-initio PySCF RHF/FCI anchors, Qiskit Aer simulation, Clifford canary probes, and Meyer-Wallach
   expressibility checks) before any paid QPU dispatch. See [[ui-ux-vision]], [[fidelity-accounting]], and
   [[implementation-gaps]].
   Paired with [[quantum-advantage-realism]]: the platform records the backend that actually ran,
   always computes a classical anchor, and never claims advantage.

9. **Autonomous Mathematical Discovery and Formal Verification Anchors:**
   The sixth source, [[source-openai-math-corpus]], provides 372 result families, 162 [[lean4]] formalizations,
   and 811 [[comparator]] challenges integrated as an adopted submodule ([[openai-math]], `submodules/math`,
   Apache-2.0). Mathematical physics proofs (Heisenberg ferromagnet spontaneous magnetization, QAOA optimality on
   the SK spin glass, Haldane gap, MUB dimension 6) establish exact analytical anchors for quantum simulation
   ([[mathematical-physics-anchors]]), while formal proof verification provides an epistemic confidence $\rho = 1.0$
   pathway for autonomous scientific exploration ([[autonomous-mathematical-discovery]],
   [[formal-proof-verification-lean-comparator]], [[frontier-benchmark-saturation]]).

10. **The Epistemic Continuum, Quantum Triage, and Post-NISQ Krylov Methods:**
    Scientific truth is modeled along an **Epistemic Continuum** parameterized by confidence $\rho \in [0, 1]$,
    synthesizing formal machine proof ($\rho = 1.0$) with certified classical bounds ($\rho \to 1.0^-$) and
    stochastic QPU execution ($\rho < 0.90$) ([[epistemic-continuum-and-ontology]]). The platform navigates the
    demise of heuristic NISQ VQE by pivoting to **Quantum Krylov Subspace Diagonalization (QKSD)**, classical
    shadow tomography, reconfigurable neutral-atom arrays (Harvard/QuEra 48 logical qubits), and high-rate
    bivariate bicycle qLDPC gross codes (IBM \[144, 12, 12\]) ([[early-fault-tolerant-quantum-and-subspace-methods]]).
    The automated [[quantum-triage-engine]] uses Family 278 (provable Kohn–Sham representability failure) and
    Family 275 (continuum Coulomb QMA-hardness) to mathematically route only multi-reference, strongly correlated
    systems to paid QPUs, preventing wasteful quantum-washing.

11. **Bilingual Manuscript Synthesis, Verified Peer Review, and Knowledge Lifecycle:**
    Scientific discoveries are synthesized into both machine-checked Lean 4 code and publication-grade modular
    arXiv LaTeX preprints, graded by an automated adversarial peer-review tribunal across mathematical rigor,
    novelty, empirical grounding, axiomatic hygiene, and reproducibility ([[autonomous-manuscript-synthesis-and-peer-review]]).
    High-throughput scientific data is governed by a **4-tier storage architecture** (Tier 0 transient tmpfs,
    Tier 1 Merkle DAG ledger, Tier 2 HippoRAG GraphRAG associative network, Tier 3 reflective strategy bank)
    with first-class negative certificates and an RLVR verifiable flywheel ([[epistemic-information-lifecycle-and-memory]]).

12. **The Scientific Agentic Experience (AX) Triad and Multimodal Mission Control:**
    A comprehensive 3-axis interaction architecture: User $\leftrightarrow$ Agent (calibrated friction Ed25519 cards,
    flight recorder scrubbing, counterfactual forking), Agent $\leftrightarrow$ Agent (Epistemic Tribunal Protocol with
    quorum voting), and Agent $\leftrightarrow$ Interface (A2UI generative scientific widgets) across nine core
    observatory surfaces ([[scientific-agentic-experience-triad]]). Local-first multimodal mission control operates
    with 0 outbound bytes by default while permitting cryptographically budgeted cloud escalations, complemented
    by an in-platform model explorer and direct streaming downloader via [[hugging-face-hub]]
    ([[multimodal-mission-control-and-custom-agents]]).

- **Provenance hygiene (2026-09-27):** the `ghp_…` PAT in a gitignored clone's remote URL is
  **verified dead** (401), but the same sweep found a *live* `gho_…` token sitting in plaintext in
  `~/.git-credentials` because `credential.helper=store` is set globally — an operator-machine
  problem, not a repo problem, and the one secret still open
  ([[risks]], [[secrets-and-workload-isolation]]). One folder cited as external prior art turned out
  to be a clone of Kubona's own repository ([[source-frontier-repo-audit-2026]]). The clone folder is scratch and gets deleted; claims must
  resolve to a live URL + SHA + `path:line`. Own repos with no `LICENSE` are **not** on this list:
  insight mining needs no grant, and the only check is at release time.
## Where the blueprint is unsettled

The full ledger of undecided and under-understood items is [[questions]] — 41 closed,
8 open rows (7 unnumbered `open-question` pages in Sections B–C plus Q-27 in Section E), each with what is known, what is missing, and what the answer unblocks.
Three tensions are worth naming here because they are strategic rather than technical:

- **Open-source vs. qBraid's 34-device lead.** Kubona deliberately starts with 2–3
  providers and differentiates on the agentic governance layer. Is that enough? See
  [[risks]] for the full register, and [[questions]] Q-21 for the provider choice.
- **Export control / jurisdiction** across autonomous cross-border job routing — scoped
  to Phase 5, still unresolved at the level of *who is liable*, not which API is called.
- **Core vs. v2 for science discovery.** Roughly 40% of the concept pages describe the
  discovery platform; roughly one phase and no human surface does. That mismatch is
  [[questions]] Q-17 (closed 2026-10-10 as CORE, not v2; v1 scope boundary settled 2026-10-26 in [[kubona]]).

## Settled constraints (Q-02, closed 2026-10-10)

Five load-bearing commitments, each enforced by tests and depended on by other
subsystems — changing any of them needs a new decision and a log entry:

1. [[integration-topology-and-licence-discipline]] — submodule/sidecar/blueprint/reference topology; nothing unlicensed crosses into the core.
2. [[quantum-advantage-realism]] — never claim advantage; every artefact carries backend, classical anchor, and receipts (enforced: claim grammar validators, tribunal attestation rules).
3. [[spend-mandates-and-delegation]] — no spend without a verified mandate; sub-mandates bounded; revocation propagates (enforced at every dispatch entry).
4. [[cost-governance]] — ceilings, pre-flight, and the L0–L3 ladder with live grant/revoke checks (enforced in Go admission and Python preflight).
5. [[epistemic-continuum-and-ontology]] — tier thresholds and the collapse gate (enforced in both epistemics implementations with tests).

Criterion used: *settled = invariant enforced by tests + depended on by other subsystems*.
Everything else stays `adopted` deliberately — understood, not yet chosen.

> ⚑ Correction: this page previously listed *"six-language stack risk
> (Rust/TS/Dart/Py/JS/±Go)"* as live. Verified 2026-09-27: the prototype contains
> Python (141 files), Rust (2 sources) and Dart (12 files) only — the `.ts/.js` files in
> the tree are venv and Flutter build artefacts. The stack collapsed to three without a
> decision being recorded, which is a win the wiki never claimed. Recorded here so the
> reversal stays visible.
>
> ⚑ Resolution (2026-10-08): Superseded by concrete implementation. The prototype
> has expanded to include the Go 1.24 core orchestrator (`platform/core/`, 24 packages,
> 58 tests) and the Next.js 16 / TypeScript 5.8+ console (`platform/console/`, 9 observatory
> surfaces), concretely realizing the production tripartite architecture described in §5.

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

**Status census (mechanical, 2026-10-26).** 151 pages: `settled` 5 · `adopted` 56 ·
`open-question` 7 · `candidate` 13 · `tracked` 44 · `reference` 9 · `source` 10 ·
`living` 7. Vocabulary: [[index]] §status legend. Read it as *decidedness*, never
as completion: five settled policies against 56 adopted mechanisms means the blueprint is
substantively solidified.

### What the prototype has empirically answered

`platform/` — 676 Python tests (`pytest platform/tests/`) + 41 gRPC bridge tests
(`platform/bridge/test_bridge.py`) + 27 Go packages (`go test ./...`, vet clean) +
149 console tests (Vitest, 32 files) + Rust `qir-core` (13 tests) and `ifc-core`
(4 tests), all green locally and in CI as of 2026-10-17
(see [[full-platform-audit-2026-10-12]] and [[remaining-work]]).
Each phase below had a gate test, and each passes **on simulators**:

> ⚑ Correction: the 2026-10-08 text here read "213 Python + 58 Go + 3 Rust +
> 1 Flutter test". The Flutter/Dart stack was retired 2026-10-04 (stack
> ruling, [[roadmap]]); the counts grew as the Go core became the load-bearing
> implementation (84 routes, single `kubonad` binary).
>
> > ⚑ Correction (2026-10-17, C6 + A1): 71 → 76 → 84. New since the earlier count:
> > `sessions/context|export|import`, `vault/leases/revoke`,
> > `jobs/{id}/cancel`, the S8 console-support routes, `/api/capabilities`,
> > `/api/tenants/refund`, and the 6 Ed25519 session-auth routes
> > (`/api/auth/challenge|login|logout|whoami|sessions|revoke`). Counts are
> > prose and prose rots — recount with
> > `rg -o 'HandleFunc\("[^"]*"' platform/core/pkg/api/server.go | sort -u | wc -l`.

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

> ⚑ Update (2026-10-23, [[observatory-lens-audit-2026-10-23]]): the
> Observatory lens (Surface 1/9 AX triad) has since landed P0–P6 as
> prototype fact — Tier-1 walkable lab (Morse terrain, pavilion,
> Attention Director, ambient audio), Vault pre-registration with
> surprise scoring, seven `world.*` MCP tools (catalog 29→36 tools),
> and the opt-in Atlas sidecar (snapshot posing, depth-conditioned
> chisel with a live warp-route consumption proof, v1a webhooks,
> reservation ledger). Live-verified against the beta API: pose, chisel
> and generate succeed; text-to-image is in sustained upstream outage
> (0/4, `TRANSIENT_UPSTREAM`). Current suites: Go `pkg/api` full green,
> console 48 files / 229 tests green, `tsc` + eslint clean, wiki lint
> clean (149 pages). Browser-run P1 end-to-end and burn-dashboard
> numbers remain operator-side.

### What no prototype test can answer

These need a human, a wallet, or the outside world — which is why the roadmap's remaining
work is not "finishing the code": a **real QPU run** (P1/P2 gates say *real QPU*); a
**third-party security review** (P3); a **public repo plus a merged outside PR** (P4 — which the Q-25 ruling now also defers, since it
cannot be satisfied while the repository is private by decision);
**business sign-off** on monetization (P5); and the **human surface**, whose spec is the
observatory surfaces ([[agent-ux-control-plane]]) and whose build is the Next.js console
(P6, [[questions]] Q-22). Also outstanding: volume eval
corpora, a real payment rail, GQE retraining on a GPU host, and a `ci.yml` push pipeline that
had **never passed** (17 recorded runs, every one a failure) while `scripts/ci.sh` ran green
locally — the release gate the roadmap treats as load-bearing was decorative on the machine that
ships code. It went green for the first time on 2026-09-29: the gate's pass/fail rule existed in
two files and had silently diverged, and one function (`release_gate()`) now decides
([[implementation-gaps]]). **M0 closures landed:** single non-`/tmp` state root,
fail-closed production auth, `kubona serve`/`doctor` entrypoints, x402 fail-closed unless
a real verifier is injected. Full register: [[implementation-gaps]].

**Repo posture.** `github.com/mainza-ai/kubona`, **private by decision**: the operator keeps it
closed while the blueprint is still being designed, because a public repo without a settled
direction attracts contributions the project cannot direct. The flip is therefore gated on the
blueprint becoming *contributor-legible* — Q-01 answered (what `platform/` is for) plus enough
of the method layer to say what to build first — not on a date and not on Phase 0, which passed
weeks ago. Apache-2.0 at publication remains committed; the residue in private-remote history
(the repo has never been public: `dev-docs/`, `.obsidian/`) is the last mechanical prerequisite ([[questions]] Q-27,
[[risks]]).
