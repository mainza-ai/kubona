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

## B · Method & epistemics (the differentiator, least grounded)

| # | Question | Rank | Known | Missing |
|---|---|---|---|---|

## C · Governance & the trust boundary

| # | Question | Rank | Known | Missing |
|---|---|---|---|---|


## D · Science discovery (≈40% of concepts, least surfaced)

| # | Question | Rank | Known | Missing |
|---|---|---|---|---|
| **Q-17** | **Is science-discovery core or v2?** It is ~15 concept pages and 1 of 6 phases, and has no human surface. If core, the console is the emergency. If v2, move those pages to a `vision/` layer so the blueprint stops reading as uniformly near-term. | **P0** | The loop runs end-to-end as an engine-level test | A layer assignment for every discovery page |

## E · Surface, ecosystem, operations

| # | Question | Rank | Known | Missing |
|---|---|---|---|---|
| **Q-23** | **Is monetization alive?** x402 + library metering are designed-in, fail-closed and unstaffed (Phase 5 dormant). "Dormant by design" is a legitimate third state — but it is not currently labelled as one. | **P2** | The rail refuses every proof unless a real verifier is injected | A one-line status: dormant / research / active |
| **Q-24** | **Who could review a third-party provider PR?** Phase 4's gate needs a public repo *and* a merged outside PR; there are no contributors and one maintainer, while [[roadmap]] loads 5 roles for the remaining work. Either the gate changes or the team plan does. | **P1** | Gate requires external contribution; a provider-registration SDK exists | A realistic contributor model, or a different Phase-4 gate |
| **Q-27** | **What must the git history look like before the repo can go public?** The posture question is now *decided* (private by design — [[kubona]] §Posture); what remains is the mechanical precondition. `dev-docs/` + `.obsidian/` sit in **private-remote history (the repo has never been public)** though untracked today; no secret has been found in pushable refs. Options: publish as-is, rewrite with `git filter-repo`, or start a clean public history and keep this one as the private archive. | **P1** | 0 secrets in pushable refs; a pre-commit guard refuses newly staged `dev-docs/`/`.obsidian/` paths; the residue is source-document mirrors already present in `raw/` | One of the three chosen before the Apache-2.0 flip (and the operator's `credential.helper=store` token rotated — [[risks]]) |

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
| **Q-26** | 2026-10-10 | Licence-clean only (_meta licence gate, fail-closed with per-file reasons); good answer = declared invariant satisfied on simulator, no LLM judging. | [[evaluation-and-benchmarks]] |
| **Q-19** | 2026-10-10 | Per-source fetch/cache/quote/redistribute table verified against primary TOUs; arXiv 3s throttle + export host + polite-pool mailto enforced in fetchers; licenses flow into ledger records. | [[science-data-integrations]] |
| **Q-20** | 2026-10-10 | Manifest kubona-pack/v1 (semver, anchor-required, capabilities, evidence reqs); Ed25519 owner signatures; kubona.*-only generator allowlist; registry-file revocation. | [[quantum-domain-packs]] |
| **Q-31** | 2026-10-10 | Containerized lake under memory/swap/CPU/PID caps + timeouts, pinned toolchain image, AST fallback with reason; proof failures never mask as AST passes. | [[lean4]] |
| **Q-14** | 2026-10-10 | Yes where measurable: canary verifies declared qubits + calibration against live plugin reports within tolerance; mismatch fails the canary; unmeasurable reads unknown, never pass. | [[provider-capability-schema]] |
| **Q-02** | 2026-10-10 | Five load-bearing constraints marked `settled` with written rationale (licence, advantage-realism, mandates, cost-governance, continuum); criterion: test-enforced + depended upon. | [[overview]] |
| **Q-03** | 2026-10-10 | No new `claim` page type: claims live as concept sections backed by ledger claim records + ScientificClaim schema; taxonomy cost exceeds benefit. | [[index]] |
| **Q-07** | 2026-10-10 | Kept distinct, not merged: money-gate vs result-repair vs publishability-prediction; fidelity refusal now enforced in both admission paths. | [[fidelity-accounting]] |
| **Q-09** | 2026-10-10 | Mutable set = §13 write scope only, enforced by a versioned registry checked at propose time; widening needs a trusted admin signature, and compiler/MCP/security/ledger are unwidenable by any signer. | [[meta-search-and-self-evolution]] |
| **Q-04** | 2026-10-10 | Allowed vocabulary: 12 permitted reporting verbs, banned hype terms, required qualifiers (anchor, provenance, falsification criteria), credibility field {rho, tier, refs}; enforced fail-closed in `ScientificClaim`. | [[quantum-advantage-realism]] |
| **Q-05** | 2026-10-10 | Operational: 3-state axes, `ClaimCredibilityWavefunction.evaluate` over evidence signals, `ClaimRecord` ledger payloads, rho-gated assertion; worked H2 VQE example with real anchor numbers green. | [[claim-credibility-wavefunctions]] |
| **Q-08** | 2026-10-10 | Not competitors but axes: five rings = architecture, discovery 7-stage = lifecycle traversal, §11/§13 = Ring-2/Ring-4 engines; new loops must state traversed rings. | [[five-ring-research-loop]] |

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

