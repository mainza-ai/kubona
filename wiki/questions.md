---
type: catalog
name: Open Questions & Design Forks
date: 2026-09-27
updated: 2026-10-09
tags: [index, questions, blueprint]
sources: [source-gap-analysis-plan, source-architecture-paper, source-frontier-repo-audit-2026, source-agent-protocol-landscape-2026]
status: living
---

# Open Questions & Design Forks

> **This is the work-list for the blueprint.** A page with `status: open-question`
> means the *insight* is incomplete — something here is not yet understood or not yet
> decided. Answering a question is what promotes a page to `adopted` or `settled`;
> writing code does not. Nothing below is a task to implement — it is a thing to
> **understand or choose** before implementation becomes meaningful.
>
> **Rank** = what the answer unblocks. **P0** changes the shape of the platform;
> **P1** changes a subsystem; **P2** can wait without causing rework.

## A · Meta: what is the prototype for?

These come first, because they change how every other question gets answered.

| # | Question | Rank | Known | Missing | Pages |
|---|---|---|---|---|---|
| **Q-02** | **Which claims here are decisions and which are merely plausible?** Only one page is `settled`. Most are *understood and adopted*, which is weaker: adopted means "we would build this way today", not "we chose this and know its cost". | **P0** | 24 `adopted`, 13 `open-question`, 1 `settled` | A pass marking the 5–7 genuinely load-bearing commitments as `settled` with written rationale | [[overview]] |
| **Q-03** | **Is the wiki's unit of insight right?** Today: entities (things), concepts (mechanisms), sources (records). Missing: *claims* ("X yields Y under Z") and *option-sets* (alternatives with a pending choice). Most P0 questions are option-sets, not mechanisms. | **P1** | The 8 philosophy pages resist the concept/entity split | Whether to add a `claim` type carrying `{claim, evidence-class, conditions, falsifier}` | [[quantum-advantage-realism]], [[evaluation-and-benchmarks]] |

## B · Method & epistemics (the differentiator, least grounded)

| # | Question | Rank | Known | Missing |
|---|---|---|---|---|
| **Q-04** | **What is the allowed public vocabulary for results?** [[quantum-advantage-realism]] forbids advantage claims and mandates a classical anchor + recorded backend. What is the *positive* claim Kubona may make about its own output? Without it, every generated report needs a wording review. | **P0** | The prohibition (never claim advantage; always anchor) | A claim grammar for generated reports: permitted verbs, required qualifiers, what a credibility field holds |
| **Q-05** | **What is a credibility wavefunction, operationally?** [[claim-credibility-wavefunctions]] is the most distinctive idea in the wiki and the least shaped for implementation. What are the axes, who computes them, where are they stored, what does a consumer *do* with the value? | **P0** | Motivation + prior art ([[chickensoup]]'s honest-degradation habit) | A concrete data structure and one worked example on a real VQE result |
| **Q-07** | **What does "fidelity accounting" add over error mitigation + provenance?** [[fidelity-accounting]] overlaps [[error-mitigation]] and [[provenance-and-reproducibility]]. Either it is a distinct ledger of fidelity assumptions per claim, or it is those two renamed. | **P1** | Both neighbours are well specified | A one-line difference, or a merge |
| **Q-08** | **Is the five-ring loop the architecture or a metaphor?** [[five-ring-research-loop]], the plan's §13 meta-loop and [[science-discovery]]'s staged loop describe overlapping cycles. Three loop models for one system is a naming problem waiting to become an API problem. | **P0** | Three loop descriptions, differing stage counts and vocabularies | One canonical loop; the others rewritten as views of it |
| **Q-09** | **What may the meta-loop change, unattended?** [[meta-search-and-self-evolution]] §13 is propose-only in the prototype. Where is the line — search space, budget, safety rails? Auto-apply is named as a stage but never bounded. | **P0** | Propose-only today; staged auto-apply planned | The mutable/immutable config set, and who may widen it |

## C · Governance & the trust boundary

| # | Question | Rank | Known | Missing |
|---|---|---|---|---|
| **Q-14** | **Can a provider capability schema lie?** [[provider-capability-schema]] is data self-described by whoever registers a provider. A wrong qubit count or fidelity corrupts both the optimizer and the price estimate. [[model-optimizer]]'s schema-pinned recipes are the nearest prior art — and still self-authored. | **P1** | Schema-as-data; shape validation | Attestation or measurement: can Kubona probe a backend and reject false claims? |


## D · Science discovery (≈40% of concepts, least surfaced)

| # | Question | Rank | Known | Missing |
|---|---|---|---|---|
| **Q-17** | **Is science-discovery core or v2?** It is ~15 concept pages and 1 of 6 phases, and has no human surface. If core, the console is the emergency. If v2, move those pages to a `vision/` layer so the blueprint stops reading as uniformly near-term. | **P0** | The loop runs end-to-end as an engine-level test | A layer assignment for every discovery page |
| **Q-19** | **Which scholarly sources are free enough for autonomous ingestion?** [[science-data-integrations]] lists arXiv, OpenAlex, Semantic Scholar and others. Each constrains bulk fetch, redistribution and derivative indexing. Autonomous fetch is a licensing question, not an HTTP one. | **P1** | Integration candidates enumerated | A per-source permission table (fetch / cache / quote / redistribute) |
| **Q-20** | **What is a domain pack?** [[quantum-domain-packs]] borrows [[model-optimizer]]'s pack discipline, with [[chickensoup]] and [[mainza-milimo-foundation]] as seed material. Undefined: pack contents, versioning, who owns correctness, how a bad pack is revoked. | **P1** | Pack discipline as prior art; packs are the compounding-asset thesis | A pack manifest + a trust/revocation model |
| **Q-31** | **How does Kubona isolate and resource-cap Lean 4 formal verification inside container sandboxes?** [[openai-math]] requires [[lean4]] (`v4.34.1`) and [[comparator]]. Compiling large proofs can exhaust virtual memory mappings (`vm.max_map_count`) and CPU budgets. Needed: container sandbox image with Lean toolchain + memory controls (`-DMMAP=OFF`, `GLIBC_TUNABLES`) + execution timeout limits in the Capability Core. | **P1** | Submodule `submodules/math` vendored; Python evaluator `comparator_eval.py` passes axiom checks | Production Dockerfile with Lean 4 + Comparator runner + cgroup memory limits |

## E · Surface, ecosystem, operations

| # | Question | Rank | Known | Missing |
|---|---|---|---|---|
| **Q-23** | **Is monetization alive?** x402 + library metering are designed-in, fail-closed and unstaffed (Phase 5 dormant). "Dormant by design" is a legitimate third state — but it is not currently labelled as one. | **P2** | The rail refuses every proof unless a real verifier is injected | A one-line status: dormant / research / active |
| **Q-24** | **Who could review a third-party provider PR?** Phase 4's gate needs a public repo *and* a merged outside PR; there are no contributors and one maintainer, while [[roadmap]] loads 5 roles for the remaining work. Either the gate changes or the team plan does. | **P1** | Gate requires external contribution; a provider-registration SDK exists | A realistic contributor model, or a different Phase-4 gate |
| **Q-27** | **What must the git history look like before the repo can go public?** The posture question is now *decided* (private by design — [[kubona]] §Posture); what remains is the mechanical precondition. `dev-docs/` + `.obsidian/` sit in **private-remote history (the repo has never been public)** though untracked today; no secret has been found in pushable refs. Options: publish as-is, rewrite with `git filter-repo`, or start a clean public history and keep this one as the private archive. | **P1** | 0 secrets in pushable refs; a pre-commit guard refuses newly staged `dev-docs/`/`.obsidian/` paths; the residue is source-document mirrors already present in `raw/` | One of the three chosen before the Apache-2.0 flip (and the operator's `credential.helper=store` token rotated — [[risks]]) |
| **Q-26** | **Which eval corpora are admissible?** No third-party quantum+code benchmark corpus is in the tree; [[evaluation-and-benchmarks]] has the mechanism, not the data. Getting volume data is licensing work, not engineering. | **P2** | Corpus loader exists | Licence-clean corpora + a criterion for what counts as a good answer |

## Closed

| # | Closed | Answer (one line) | Where it landed |
|---|---|---|---|
| **Q-01** | 2026-10-04 | `platform/` is the production tripartite implementation: Go Orchestrator (`platform/core`) handles multi-agent tribunals, loops, concurrency, and SQLite persistence; TypeScript/Next.js (`platform/console`) provides the 8 dynamic observatory surfaces and WebCrypto Ed25519 signing; Python bridge (`platform/bridge`) handles PySCF ab-initio quantum chemistry, Qiskit Aer, and QPD circuit cutting via gRPC. | [[overview]], [[roadmap]], [[implementation-gaps]] |
| **Q-06** | 2026-10-08 | Mathematics: Epistemic curvature is formalized via the Riemannian Fisher–Rao information metric tensor $g_{\mu\nu}(\vec{\theta})$ and Ricci curvature tensor $R_{\mu\nu}$, driving autonomous curiosity along epistemic geodesics toward regions of maximal informational entropy reduction. | [[epistemic-curvature]] |
| **Q-15** | 2026-10-04 | Cost pre-flight error & execution protection enforces a 4-tier fallback hierarchy (Local Simulator -> Cloud Aer -> Qiskit Runtime/Braket -> Neutral Atom), capped at a $150 hard ceiling with automatic abort upon estimate divergence. | [[cost-governance]], [[spend-mandates-and-delegation]] |
| **Q-16** | 2026-10-04 | Local LLMs (oMLX port 8787) and simulators run air-gapped without API keys; external provider credentials are strictly isolated via OpenBao sidecar / scoped environment injection, never committed to git or exposed to agents. | [[openbao]], [[integration-topology-and-licence-discipline]] |
| **Q-18** | 2026-10-08 | Experiment is formalized as a typed hypothesis tuple $\mathcal{H}$, compiled through a 5-stage pipeline, bounded by a Fisher information stopping rule ($\mathbb{E}[\mathcal{I}_{\text{Fisher}}] \le \lambda_{\text{AP2}} \cdot \frac{d\text{Cost}}{d\text{Step}}$), with negative results stored as first-class cryptographic refutation certificates. | [[epistemic-continuum-and-ontology]], [[epistemic-information-lifecycle-and-memory]] |
| **Q-21** | 2026-10-04 | First sanctioned runs execute credential-free on local Qiskit Aer density matrix simulator and PySCF FCI classical anchor; real QPU hardware execution is gated behind human AP2 cryptographic mandate cards signed via WebCrypto. | [[quantum-cloud-pricing]], [[quantum-advantage-realism]] |
| **Q-22** | 2026-10-04 | Human surface and orchestrator standardized on **TypeScript (Next.js/React) + Go**: TypeScript delivers React Flow node canvases, Yjs multiplayer presence, and deep DOM paper selection; Go delivers lightweight concurrency (goroutines/channels) for thousands of agent loops and container orchestration; Python handles Quantum/AI runtimes via gRPC. Dart/Flutter prototype scaffold retired. | [[ui-ux-vision]], [[protocol-strategy]] |
| **Q-25** | 2026-09-29 | Privacy is a **decision, not drift**: the repo stays private while the blueprint is being designed, because a public repo without a settled direction invites contributions the project cannot direct — so the flip is gated on the blueprint becoming *contributor-legible* (Q-01 plus enough of the method layer to say what to build first), not on Phase 0 and not on a date. Apache-2.0 at publication stands. | [[kubona]] §Posture; residual history work → Q-27 |
| **Q-10** | 2026-10-09 | Autonomy ladder is **L0 observe/suggest → L1 simulator-only (default) → L2 capped-QPU (mandate per job) → L3 program envelope**, enforced live at every dispatch entry with ledger-sealed admin-signed grants/revocations (no cached grants). | [[cost-governance]] |
| **Q-12** | 2026-10-09 | Delegation chain rules: scopes narrow only (server-resolved, never client-claimed), depth cap 2, revocation propagates (revoked parents spawn nothing, in-flight runs fence), tenant revocation aborts the lab's jobs; execution runs in per-tenant `a2a-delegation-<tenant>` sessions. | [[spend-mandates-and-delegation]] |
| **Q-13** | 2026-10-09 | Tenant := **lab** (billing/credential/ceiling boundary; operator ruling); admin is the lab operator's Ed25519 key, local keypairs first with IdP deferred to Phase 4; human lab membership is Phase-4 work, agents are never tenants. | [[user-identity-and-profiles]] |
| **Q-29** | 2026-10-09 | Target **MCP spec 2026-07-28** (venv already on SDK 2.3.0); tasks-extension opportunistic with polling normative, MRTR for approvals; A2A adapter first, enforcement in Go core rail only. | [[mcp-and-context-flooding]] |
| **Q-30** | 2026-10-09 | Posture: **encapsulate** vendor MCP suites as downstream provider plugins beneath the governance plane (vault-leased creds, admission gate, anchored results); raw agent→vendor paths refused by design. | [[qiskit-mcp-servers]] |
| **Q-11** | 2026-10-09 | Burden-scaled approval packets: full schema (cost bound + budget left, blast radius, reversibility, evidence, alternatives, expiry, single-use nonce) via MRTR elicitation retry in the gateway plus ApprovalPacketCard in the console approve flow; risk-tiered prompting rules. | [[cost-governance]] |
| **Q-28** | 2026-10-09 | No namespace move: coupling measured one-directional (edge → core); contract enforced mechanically by Go + Python architectural fitness tests in CI. | [[protocol-strategy]] |

## Reading order for a new session

1. **This page** — find the question you can actually answer today.
2. [[overview]] for the synthesis; [[roadmap]] for phases read as *acceptance criteria*, not progress.
3. [[integration-topology-and-licence-discipline]] and [[quantum-advantage-realism]] — the two standing policies.
4. The page for your question. Open [[implementation-gaps]] only when the question is about the prototype.

## When a question closes

1. Write the answer **on the concept page it belongs to**, not here.
2. Flip that page `status: open-question → adopted` — or `settled` if it is now a constraint others must obey.
3. Move the row to a `## Closed` section here with date + one-line answer + link.
4. Append `## [YYYY-MM-DD] question | Q-NN — <one line>` to [[log]].

