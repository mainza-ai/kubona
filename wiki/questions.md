---
type: catalog
name: Open Questions & Design Forks
date: 2026-09-27
updated: 2026-10-26
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

All Section A questions are settled: `platform/` tripartite role closed 2026-10-04; v1 scope boundary and negative exclusions closed 2026-10-26 (see `[[kubona]]` and `## Closed` below).

## B · Method & epistemics (the differentiator, least grounded)

All Section B questions are settled: memory/flight-recorder placement, QIR single-dialect scope, refuted-genome transfer priors, dual-model divstep packing, and unified single-tape autodiff closed 2026-10-26 (see `## Closed` below).

## C · Governance & the trust boundary

All Section C questions are settled: 3-seam IFC lattice enforcement with fail-closed unmodelable label refusal and gVisor/runsc workload containment closed 2026-10-26 (see `## Closed` below).

## D · Science discovery (closed via Q-17, Q-18, Q-19, Q-20)

All initial science-discovery forks are recorded in `## Closed` below (Q-17, Q-18, Q-19, Q-20).

## E · Surface, ecosystem, operations

All Section E questions are settled: Q-27 history scrub executed 2026-10-26 (see `## Closed` below).

## F · Production-readiness (opened 2026-10-26, resolved 2026-10-27)

All production-readiness decisions are resolved. Q-50/Q-51/Q-56 were approved
with the recommended defaults; implementation residuals are tracked in
[[production-readiness-audit-2026-10-26]] §Execution record.

## Closed` below).

## B · Method & epistemics (the differentiator, least grounded)

All Section B questions are settled: memory/flight-recorder placement, QIR single-dialect scope, refuted-genome transfer priors, dual-model divstep packing, and unified single-tape autodiff closed 2026-10-26 (see `## Closed` below).

## C · Governance & the trust boundary

All Section C questions are settled: 3-seam IFC lattice enforcement with fail-closed unmodelable label refusal and gVisor/runsc workload containment closed 2026-10-26 (see `## Closed` below).

## D · Science discovery (closed via Q-17, Q-18, Q-19, Q-20)

All initial science-discovery forks are recorded in `## Closed` below (Q-17, Q-18, Q-19, Q-20).

## E · Surface, ecosystem, operations

All Section E questions are settled: Q-27 history scrub executed 2026-10-26 (see `## Closed` below).

## F · Production-readiness (opened 2026-10-26; technical gates resolved 2026-10-27)

The technical gates are closed with the conservative option recorded in
[[production-readiness-audit-2026-10-26]] §Rulings. Three product/finance
choices remain; recommended defaults are named in that table.

| # | Question | Rank | Known | Missing | Links |
|---|---|---|---|---|---|
| **Q-50** | **Which store is the single money authority, and in what exact unit?** | P0 | Go SQLite is the authority for API/human/quantum dispatch; Python mirrors it; Atlas is optional and off in production. Go reserves and Python records, so failures can diverge. | Ruling on Go-as-authority + integer micro-USD migration (or Decimal/Postgres); reconciliation for pre-existing REAL values. | [[production-readiness-audit-2026-10-26]] |
| **Q-51** | **Are failed/pre-vendor jobs refunded automatically, or only by an operator?** | P0 | Python refunds pre-vendor failures; Go releases any failed/cancelled bridge reservation (including post-vendor, which may be too generous); partial refunds are not implemented. | Ruling: auto pre-vendor + keep post-vendor + operator-only partials (recommended) vs another policy; how Go learns the vendor-pin state. | [[production-readiness-audit-2026-10-26]] |
| **Q-56** | **How does the console authenticate without shipping a bearer or losing its admin key on reload?** | P1 | A baked bearer now fails the production build; CSP is on; the Ed25519 key is module-memory only and is lost on reload; cookie sessions exist server-side. | Ruling: cookie sessions as identity + passkey/non-extractable admin key (recommended) vs another custody model; token distribution/rotation. | [[production-readiness-audit-2026-10-26]] |

## Closed` below).

## B · Method & epistemics (the differentiator, least grounded)

All Section B questions are settled: memory/flight-recorder placement, QIR single-dialect scope, refuted-genome transfer priors, dual-model divstep packing, and unified single-tape autodiff closed 2026-10-26 (see `## Closed` below).

## C · Governance & the trust boundary

All Section C questions are settled: 3-seam IFC lattice enforcement with fail-closed unmodelable label refusal and gVisor/runsc workload containment closed 2026-10-26 (see `## Closed` below).

## D · Science discovery (closed via Q-17, Q-18, Q-19, Q-20)

All initial science-discovery forks are recorded in `## Closed` below (Q-17, Q-18, Q-19, Q-20).

## E · Surface, ecosystem, operations

All Section E questions are settled: Q-27 history scrub executed 2026-10-26 (see `## Closed` below).

## F · Production-readiness (opened 2026-10-26 by the Round-2 audit)

These are implementation-shape decisions surfaced by
[[production-readiness-audit-2026-10-26]]; the code cannot choose them. Each
blocks a Phase-1/2/4 slice. Until answered, the platform is not production-ready.

| # | Question | Rank | Known | Missing | Links |
|---|---|---|---|---|---|
| **Q-49** | **What is the bridge trust boundary — mTLS, HMAC interceptor, or UDS-only — and does the bridge ever run off-host?** | P0 | Bridge binds unauthenticated `[::]:50051`; Go dials insecure; UDS mode still binds TCP. It accepts caller-asserted budget/spend/tenant and can submit IBM hardware outside the governed rail. | Transport choice + whether the deploy topology keeps bridge on the same host/network namespace; key distribution/rotation. | [[production-readiness-audit-2026-10-26]] |
| **Q-50** | **Which store is the single money authority, and in what exact unit?** | P0 | Three books (Go SQLite REAL, Python JSONL float, Atlas) with epsilon guards; Go reserves QPU spend while Python also records it. | Choice (Go SQLite vs Python vs Postgres), integer micro-USD vs Decimal, migration and Atlas reconciliation policy. | [[production-readiness-audit-2026-10-26]] |
| **Q-51** | **Are failed/pre-vendor jobs refunded automatically, or only by an operator?** | P0 | Spend is committed at dispatch; failures keep the charge; Python refund is unsigned/unledgered; Go refund requires an envelope but does not verify its signature. | Refund trigger/timing, idempotent replay semantics, partial-vendor-usage policy. | [[production-readiness-audit-2026-10-26]] |
| **Q-52** | **What is the session-import trust model?** | P0 | Import is unauthenticated and inserts caller-supplied spend, events, hashes, and signatures; this can forge the ledger and mandate receipts. | Operator-only vs remove vs sandboxed staging; manifest signing/key distribution; merge/replay semantics. | [[production-readiness-audit-2026-10-26]] |
| **Q-53** | **Does A2A/x402 become real, or stay disabled until a settlement rail exists?** | P1 | Public A2A accepts a self-signed challenge, treats it as payment, and defaults the tenant to `default`; Q-23 records x402 as dormant. | Settlement rail/chain staffing, authenticated tenant binding, whether root tasks may exist publicly. | [[production-readiness-audit-2026-10-26]], [[a2a]] |
| **Q-54** | **What runtime isolates agent-submitted Python, and may numpy/scipy remain allowed?** | P0 | AST filtering is escapable to native code and host filesystem; no `runsc`/container is used for this rail; Q-48 claimed gVisor but the code does not invoke it. | gVisor vs Docker vs WASM; whether the science rail needs native libs; mandatory-vs-optional enforcement. | [[production-readiness-audit-2026-10-26]] |
| **Q-55** | **Is the anchor source live PySCF per request or a curated table, and which reference values are authoritative?** | P0 | The H2 table returns `-1.0718` at every R (true `-1.13727`), ignores geometry, and the suite was edited to bless it; the repo already contains a correct JW anchor in `eval/truncation.py`. | Cost/latency tolerance for live solves; table curation/verification owner; which molecules must be live. | [[production-readiness-audit-2026-10-26]] |
| **Q-56** | **How does the console authenticate without shipping a bearer or losing its admin key on reload?** | P1 | A dev bearer is baked by a normal `next build`; the Ed25519 key is module-memory only and is lost on reload; the server already supports cookie sessions. | Server-session-only vs client key custody (passkey/WebCrypto non-extractable); operator token distribution; rotation. | [[production-readiness-audit-2026-10-26]] |
| **Q-57** | **Which endpoints may stay anonymous in single-lab mode, and how is multi-lab mode detected?** | P0 | Per-handler authz leaves A2A, import, LLM test, loops, memory, and PQC verify open; bearer tokens carry no tenant while cookies do; D-09 is recorded but not applied uniformly. | Explicit public-read allowlist, tenant-bound bearer vs cookie-only, single-lab declaration/flag, route-policy ownership. | [[production-readiness-audit-2026-10-26]] |

## Closed

| # | Closed | Answer (one line) | Where it landed |
|---|---|---|---|
| **Q-50** | 2026-10-27 | Go SQLite is the single money authority for API/human/quantum dispatch; Python mirrors it; ledger v2 moves to integer micro-USD. | [[production-readiness-audit-2026-10-26]] |
| **Q-51** | 2026-10-27 | Auto-refund pre-vendor failures, keep post-vendor charges, operator-only partials; Go uses the vendor-pin state. | [[production-readiness-audit-2026-10-26]] |
| **Q-56** | 2026-10-27 | Cookie sessions are identity; the admin Ed25519 key is optional signing material; no baked bearer; passkey/non-extractable custody next. | [[production-readiness-audit-2026-10-26]] |
| **Q-49** | 2026-10-27 | Bridge stays on the private compose network with a shared token + HMAC session assertion; mTLS is deferred until an off-host topology exists. | [[production-readiness-audit-2026-10-26]] |
| **Q-52** | 2026-10-27 | Session import is operator-only and HMAC-signed; production refuses unsigned manifests, forged links, unknown types, and impossible spend. | [[production-readiness-audit-2026-10-26]] |
| **Q-53** | 2026-10-27 | A2A/x402 is disabled in production by default (`KUBONA_A2A_ENABLED=1` opts in); the self-signed local challenge is not a settlement rail. | [[production-readiness-audit-2026-10-26]] |
| **Q-54** | 2026-10-27 | Container isolation is mandatory in production (Docker; runsc optional); numpy/scipy remain allowed inside the container. | [[production-readiness-audit-2026-10-26]] |
| **Q-55** | 2026-10-27 | Live PySCF is authoritative and geometry-correct for anchors; a fixed table may only be a fallback where a live solver exists. | [[production-readiness-audit-2026-10-26]] |
| **Q-57** | 2026-10-27 | Production defaults to multi-lab auth reads; `KUBONA_SINGLE_LAB=1` explicitly declares the single-lab quickstart. | [[production-readiness-audit-2026-10-26]] |
| **Q-42** | 2026-10-26 | Go Orchestrator provides unified cold-audit flight recorder API (`GET /api/campaigns/{id}/flight-recorder`) returning terminal verdict, realized spend, and halt reasons in <200ms with React `FlightRecorderCard` console card; Rust `qir-core` retains compilation engine role. | [[agentic-memory-and-observability]] |
| **Q-43** | 2026-10-26 | Refused heavy multi-dialect MLIR compilation; adopted single Kubona QIR dialect lowered directly to native Rust `qir-core` with OpenQASM 3.0 parsing and LLVM IR bitcode generation. | [[agentic-qir-compilation]] |
| **Q-44** | 2026-10-26 | 4-tier failure taxonomy (`FailureClass`) with global intrinsic refutation blacklists and device-scoped $T_2$/topology hardware transfer priors implemented in `RefutedGenomeLibrary`. | [[autonomous-ansatz-optimization]] |
| **Q-45** | 2026-10-26 | Centralized single-tape autodiff via Python Bridge `DifferentiableQuantumTape` using PennyLane `default.mixed` for joint analytical parameter-shift gradient computation of $\nabla_\theta$ and $\nabla_\phi$. | [[differentiable-quantum-stack]] |
| **Q-46** | 2026-10-26 | Dual-model register allocation: `safe_288` (288-bit registers, 0.000% overflow guarantee) for production and `paper_256` (256-bit registers with tripwire failure budget accounting) for research reproduction. | [[divstep-packing]] |
| **Q-47** | 2026-10-26 | 3-seam IFC enforcement (query filtering, sub-DAG export join, admission egress) with strict fail-closed refusal (`deny-all`) for unmodelable policy prose and licenses. | [[confidentiality-lattice-provenance]] |
| **Q-48** | 2026-10-26 | gVisor `runsc` container sandbox runtime with dropped capabilities, process group `killpg` on timeout/cancellation, and 0600 Unix Domain Socket IPC attestation. | [[secrets-and-workload-isolation]] |
| **Q-27** | 2026-10-26 | Git history scrub executed via `git filter-repo --path dev-docs --path .obsidian --invert-paths --force`; 0 residual objects across all branches, `credential.helper=store` dropped, rewritten history pushed, and branch ruleset re-enabled. | [[risks]], [[kubona]] |
| **Q-01** | 2026-10-04 | `platform/` is the production tripartite implementation: Go Orchestrator (`platform/core`) handles multi-agent tribunals, loops, concurrency, and SQLite persistence; TypeScript/Next.js (`platform/console`) provides the 8 dynamic observatory surfaces and WebCrypto Ed25519 signing; Python bridge (`platform/bridge`) handles PySCF ab-initio quantum chemistry, Qiskit Aer, and QPD circuit cutting via gRPC. Settled 2026-10-26: v1 scope explicitly bounds to 5 executable backends (local, Aer, IBM, Quantinuum, D-Wave) and 4 core modalities; unwired providers, periodic crystals, and unsloth weight retraining deferred to v2. | [[overview]], [[roadmap]], [[implementation-gaps]], [[kubona]] |
| **Q-38** | 2026-10-26 | Unified single-book session budget ceiling ($150 default) with sub-category spend tagging (`media_atlas` vs `quantum_qpu`); no separate media quota subsystem. | [[observatory-plan-gap-audit-2026-10-23]] |
| **Q-39** | 2026-10-26 | Looking is free for cached views ($0.00 when `depth_asset_id` or manifest view exists for session); fresh synthesis/chisel bills $0.50. | [[observatory-plan-gap-audit-2026-10-23]] |
| **Q-40** | 2026-10-26 | Session-level Ed25519 mandate with high-watermark escalation (jobs > $10.00 or external jurisdiction require single-job mandate card; subcircuits execute under session ceiling). | [[quantum-hardware-gaps-2026-10-24]], [[cost-governance]] |
| **Q-41** | 2026-10-26 | Automatic 100% spend release on system failure; discretionary refunds require Tenant Admin Ed25519 signature within 72h window; partials permitted bounded by reserved amount. | [[quantum-hardware-gaps-2026-10-24]] |
| **Q-06** | 2026-10-08 | Mathematics: Epistemic curvature is formalized via the Riemannian Fisher–Rao information metric tensor $g_{\mu\nu}(\vec{\theta})$ and Ricci curvature tensor $R_{\mu\nu}$, driving autonomous curiosity along epistemic geodesics toward regions of maximal informational entropy reduction. | [[epistemic-curvature]] |
| **Q-15** | 2026-10-04 | Cost pre-flight error & execution protection enforces a 4-tier fallback hierarchy (Local Simulator -> Cloud Aer -> Qiskit Runtime/Braket -> Neutral Atom), capped at a $150 hard ceiling with automatic abort upon estimate divergence. | [[cost-governance]], [[spend-mandates-and-delegation]] |
| **Q-16** | 2026-10-04 | Local LLMs (oMLX port 8787) and simulators run air-gapped without API keys; external provider credentials are strictly isolated via OpenBao sidecar / scoped environment injection, never committed to git or exposed to agents. | [[openbao]], [[integration-topology-and-licence-discipline]] |
| **Q-18** | 2026-10-08 | Experiment is formalized as a typed hypothesis tuple $\mathcal{H}$, compiled through a 5-stage pipeline, bounded by a Fisher information stopping rule ($\mathbb{E}[\mathcal{I}_{\text{Fisher}}] \le \lambda_{\text{AP2}} \cdot \frac{d\text{Cost}}{d\text{Step}}$), with negative results stored as first-class cryptographic refutation certificates. | [[epistemic-continuum-and-ontology]], [[epistemic-information-lifecycle-and-memory]] |
| **Q-21** | 2026-10-04 | First sanctioned runs execute credential-free on local Qiskit Aer density matrix simulator and PySCF FCI classical anchor; real QPU hardware execution is gated behind human AP2 cryptographic mandate cards signed via WebCrypto. | [[quantum-cloud-pricing]], [[quantum-advantage-realism]] |
| **Q-22** | 2026-10-04 | Human surface and orchestrator standardized on **TypeScript (Next.js/React) + Go**: TypeScript delivers React Flow node canvases, Yjs multiplayer presence, and deep DOM paper selection; Go delivers lightweight concurrency (goroutines/channels) for thousands of agent loops and container orchestration; Python handles Quantum/AI runtimes via gRPC. Dart/Flutter prototype scaffold retired. | [[ui-ux-vision]], [[protocol-strategy]] |
| **Q-25** | 2026-09-29 | Privacy is a **decision, not drift**: the repo stays private while the blueprint is being designed, because a public repo without a settled direction invites contributions the project cannot direct — so the flip is gated on the blueprint becoming *contributor-legible* (Q-01 plus enough of the method layer to say what to build first), not on Phase 0 and not on a date. Apache-2.0 at publication stands. | [[kubona]] §Posture; residual history work → Q-27 |
| **Q-10** | 2026-10-09 | Autonomy ladder is **L0 observe/suggest → L1 simulator-only (default) → L2 capped-QPU (mandate per job) → L3 program envelope**, enforced live at every dispatch entry with ledger-sealed admin-signed grants/revocations (no cached grants). | [[cost-governance]] |
| **Q-12** | 2026-10-09 | Delegation chain rules: scopes narrow only (server-resolved, never client-claimed), depth cap 2, revocation propagates (revoked parents spawn nothing, in-flight runs fence), tenant revocation aborts the lab's jobs; execution runs in per-tenant `a2a-delegation-<tenant>` sessions. | [[spend-mandates-and-delegation]] |
| **Q-13** | 2026-10-09 | Tenant := **lab** (billing/credential/ceiling boundary; operator ruling); admin is the lab operator's Ed25519 key (challenge-response `kubona_session` cookie landed in A1–A4, 2026-10-17), local keypairs first with IdP deferred to Phase 4; human lab membership is Phase-4 work, agents are never tenants. | [[user-identity-and-profiles]] |
| **Q-29** | 2026-10-09 | Target **MCP spec 2026-07-28** (venv already on SDK 2.3.0); tasks-extension opportunistic with polling normative, MRTR for approvals; A2A adapter first, enforcement in Go core rail only. | [[mcp-and-context-flooding]] |
| **Q-30** | 2026-10-09 | Posture: **encapsulate** vendor MCP suites as downstream provider plugins beneath the governance plane (vault-leased creds, admission gate, anchored results); raw agent→vendor paths refused by design. | [[qiskit-mcp-servers]] |
| **Q-11** | 2026-10-09 | Burden-scaled approval packets: full schema (cost bound + budget left, blast radius, reversibility, evidence, alternatives, expiry, single-use nonce) via MRTR elicitation retry in the gateway plus ApprovalPacketCard in the console approve flow; risk-tiered prompting rules. | [[cost-governance]] |
| **Q-28** | 2026-10-09 | No namespace move: coupling measured one-directional (edge → core); contract enforced mechanically by Go + Python architectural fitness tests in CI. | [[protocol-strategy]] |
| **Q-24** | 2026-10-10 | Operator chose C: vendor-maintained sidecar providers; SDK conformance suite is the Phase-4 gate artifact. | [[roadmap]] |
| **Q-23** | 2026-10-10 | Dormant by design: x402 machinery exists but settlement is local-only and unstaffed; activation needs a Phase-5 staffing + chain decision recorded here. | [[agent-resource-economy]] |
| **Q-17** | 2026-10-10 | CORE, not v2: loop runs green + 3 surfaces exist; every discovery page assigned CORE-SURFACED / CORE-ENGINE / VISION in place, no file moves. | [[science-discovery]] |
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
| **Q-33** | 2026-10-12 | Mutator, not removal: `POST /api/tenants/status` flips active ↔ suspended; suspension freezes spend-affecting actions while the admin keeps reactivation + key rotation; enforced in `verifyAdmin`, live-proven both directions. | [[user-identity-and-profiles]] |
| **Q-34** | 2026-10-12 | Dormant by design (x402 precedent): pgvector sidecar is complete but constructed by nothing; labelled in-code with activation criteria; `TestPostgresStoreDormantContract` pins unconfigured + fail-closed. | [[agentic-memory-and-observability]] |
| **Q-35** | 2026-10-12 | Premise was stale — already exposed: `POST /api/a2a/v1` serves external JSON-RPC (card, status, x402-gated submit) on the same ledger-bound executor and tenant rules; proven live, claim corrected, no transport work. | [[a2a]] |
| **Q-32** | 2026-10-12 | Lint taught the rule: `status: open-question` requires a row in A–E (index presence insufficient); enforced in `scripts/wiki-lint.py --strict`, negative-tested; rule recorded on [[index]], which no longer enumerates closed rows (Closed table is the single source). | [[index]] |
| **Q-36** | 2026-10-23 | `dev_inspect` for beta (operator ruling; account-level retention control + training opt-out pending operator check); production gets its own review with purge wiring (P1). | [[observatory-lens-audit-2026-10-23]] §6 |
| **Q-37** | 2026-10-23 | Yes: cover-cropped exact-1280×720 canvas snapshot rides inline base64 to a project asset, then an assetId frame ref; missing/unreadable capture still fail-closes. Landed + mock-tested, zero live spend. | [[observatory-lens-audit-2026-10-23]] §5 |

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

