---
type: concept
name: Full Wiki-vs-Code Audit 2026-10-12
date: 2026-10-12
updated: 2026-10-12
tags: [audit, wiki, gaps, plan, implementation]
sources: [source-gap-analysis-plan, source-architecture-paper, source-frontier-repo-audit-2026, source-agent-protocol-landscape-2026, source-openai-math-corpus, source-cloudflare-economy]
status: tracked
---

# Full Wiki-vs-Code Audit (2026-10-12) — 113/113 pages read, every plan claim checked

**Method.** All 113 wiki pages read in full (5 meta, 6 sources, 43 entities,
59 concepts) — no skimming. Every code claim checked against the tree
(`platform/`, Go 28 packages green, 401 Python green, 73 console vitest
green, CI green 38216776061); live behavior probed on the parked stack
(Go `:8080`, bridge `:50051`, console `:3000`); dead-file claims checked
with git history. Standard: production, not demo.

**Headline.** The core build is real and the wiki is unusually honest —
but the wiki has drifted in ~15 small places, 7 open-question pages have
no work-list rows, 2 cited files never existed, 1 dependency is dead
weight, and the plan-vs-code comparison surfaces 1 live operational gap
(oMLX down), 1 local-dev gap (no operator env on the parked stack), and a
ranked list of genuine next builds. Nothing below invents work.

## 1. Verified real (wiki accurate, code proves it)

- **Governance rail:** tenant L0–L3 ladder + admin envelopes + cumulative
  reserve/release (tenant + session), AP2/Ed25519 mandates, UCAN grants +
  revocation propagation, admission gate on every dispatch entry,
  per-tenant A2A sessions (`task_executor.go:39-40`), bearer + limiters on
  spend-adjacent routes, single-writer flock,369+ tests across rails.
- **Epistemics:** conditional sources, withheld collapse, anchor repair,
  claim records + citations, contradiction check, claim grammar +
  EvidenceTier, wavefunction + curvature (`epistemics/curvature.py`) +
  continuum + Fisher evaluator + NegativeCertificate, all tested.
- **Compute:** 14/14 live PySCF family matrix (re-measured this session),
  Qiskit Aer dispatch, PySCF-anchored tribunal with earned consensus,
  PES sweep, KaHyPar/QPD cutting via real PennyLane `qcut`, ZNE + refit,
  QAOA harness, portfolio compiler with receipts, QIR 400-guard.
- **Evals/corpora:** Katas transcribed, HumanEval 136 golden + named
  exclusions, PennyLang refused-named, codeexec sandbox live-proven,
  truncation/schedule/critics, advantage pre-checks, parity both rails.
- **Protocols:** MCP 2026-07-28 (SDK 2.3.0, MRTR approvals, polling
  normative), A2A internal + revoke propagation, x402 dormant-by-design
  (`simulated:true`), Bao sidecar client + per-call provider sourcing.
- **Console:** read/approve-only, Illustrative badges where earned,
  CONTESTED tribunal badge, COMPLETED-status measured table, emergency
  abort wired, Action Gate, WidgetStage, LabsView, literature tab.

## 2. Wiki hygiene findings (concrete, fix directly)

| # | Page | Finding | Fix |
|---|---|---|---|
| H1 | `index.md` | Census stale: "106 pages … 52 concepts" — actual **113 / 59** (43 entities, 6 sources, 5 meta verify) | Recount + fix headers |
| H2 | `questions.md` + AGENTS.md | **7 open-question pages, zero rows** in A–E tables; the "must have a row" rule is prose only — lint checks presence in index OR questions, so it passes green while the work-list is empty | Lint-enforce (Q-32) or reclassify |
| H3 | `kubona.md` §Architecture | "Control plane: Dart/Flutter" — retired Q-22 | TypeScript/Next.js |
| H4 | `risks.md` | "Six-language stack … Treat Go as optional" — Go is the core orchestrator | Rewrite row to the standardized stack |
| H5 | `design-evolution.md` | "What stayed constant … Python/TypeScript/Dart split" | Drop Dart |
| H6 | `cost-governance.md` | "remains questions Q-11" — Q-11 closed 2026-10-09 | Point at landed section |
| H7 | `implementation-gaps.md` | Stale: corpora 🟡 (HumanEval ingested), "live arXiv default" deferred (live-wired), pre-restructure `path:line` anchors, "Perf/load baselines 🔵" (G12 done) | Refresh or archive the page |
| H8 | `production-agentic-audit-2026-10-11.md` | Phase 3 "Remaining" all landed (Hindsight HTTP, replan, versions, compile gates) | Mark done |
| H9 | `production-completion-audit-2026-10-11.md` | Evidence notes "bridge covers 10" → 14 | One-line fix |
| H10 | `epistemic-curvature.md`, `five-ring-research-loop.md` | Cite `platform/kubona/observability/epistemics.py` — substance lives at `platform/kubona/epistemics/curvature.py` (κ + refusal ratio + tests verified) | Fix paths |
| H11 | `agentic-memory-and-observability.md` | Landing cites `kubona/mcp/servers/collective.py` — **never existed** (empty git history); "lifecycle inside the Rust core" unbuilt; "one console view per campaign" absent (Q-17 confirms CORE-ENGINE) | Correct to what exists (Go collective + ledger events + Hindsight HTTP) |
| H12 | `log.md` 2026-10-04 | "Published … in `wiki_89_pages_comprehensive_audit_report.md`" — file never existed; artifact is `forensic-audit-2026.md` | Correct the pointer |
| H13 | `platform/console/package.json` | `@xyflow/react` depended, **zero imports** in `src/` | Remove or use |
| H14 | `questions.md`, `overview.md` | "26 ranked open questions" — stale (30 closed, 1 open row + 7 row-less pages) | Recount |
| H15 | `five-ring-research-loop.md` | Ring table "Ring 0 absent, Ring 1 absent" — supervisor + abort rail (Ring 0) and claim gating (Ring 1) exist and are tested | Refresh to partial |

## 3. Plan-vs-code gaps

### 3a. Outside-world gates (operator queue — correctly tracked, not code gaps)

Real QPU run (runbook ready, no spend) · third-party review (brief
exists, not commissioned) · public flip + Q-27 filter-repo · vendor
conformance grading (suite built, no external sidecar graded) · GPU host
(Unsloth retrain, personas) · Lean image build + `vm.max_map_count` ·
`KUBONA_API_TOKEN` / `KUBONA_REVIEWER_PRIVKEY_HEX` /
`KUBONA_CONTACT_EMAIL` env · EPO OPS registration · Promethium trial ·
commercial-suppression account · HF-token rotation + `store`-helper
revocation (oldest open item, 2026-09-27).

### 3b. Live operational gaps (found this session)

- **oMLX down.** `:8787` returns nothing — the local-first LLM is dark,
  so tribunals run on cloud fallback (verified working: long
  anchor-citing turns + DSML tool calls, earned consensus TRUE is
  legitimate here; 1s-timeout fallback→false still holds). Local-first
  posture is degraded until restart. Needs a restart runbook + a
  console-visible engine indicator.
- **Parked daemon has zero operator env.** No `KUBONA_API_TOKEN`
  (bearer routes 503), no reviewer key (sign refuses), so mandate flows
  are dead locally. Fail-closed is correct; but there is no documented
  local bring-up (key generation → env → verify) so every fresh machine
  rediscovers it. Needs a `doctor`-checked local-keys procedure.
- **No `kubona-lean` image** (quantum-runner 1.0.0/1.1.0 present) —
  comparator runs AST fallback until the operator builds + warms it.

### 3c. Dead states & staged-not-wired

- `suspended` tenant status: field + enforcement exist, **no mutator**
  anywhere — unreachable state (Q-33: add mutator or remove field).
- `pkg/memory/postgres.go`: zero importers — PG/HNSW staged, unwired
  (Q-34: wire or mark dormant like x402).
- `useTelemetry` hook: staged, unconsumed (fleet-radar pending) — either
  wire or note as staged.
- Quantum multi-geometry scans: classical `pes_sweep` done, quantum
  counterpart absent (acknowledged in the claim table; needs design).
- Load/chaos tests on the agentic loop: absent (Phase 4 leftover).
- A2A external transport: Go-internal only; external collectives cannot
  reach it (Q-35: expose JSON-RPC or downgrade the claim).

### 3d. Unbuilt vision (correctly deferred — confirm, don't drift)

Yjs presence · Mol* viewers · WebAuthn/passkeys · 3D Fisher–Rao terrain
(orb exists) · counterfactual forking (branches exist, forks partial) ·
KEMTLS · per-tenant DB sharding (decision pending) · syndrome pipelines
· human lab membership (P4) · IdP (P4) · multi-tenant horizontal scale ·
photonic/ocean credentials.

### 3e. Design-doc mismatches (register corrections, §2 mechanism)

H10/H11 paths · H15 ring table · `workflows` route served on the Python
API with no console view and no Go registry (log's "serving decision"
still open) · `milimo-quantum-drug-discovery` must never be cited as
external (already flagged — repeating because it recurs).

## 4. Improvements & enhancements (ranked)

- **P0:** Q-16 first paid QPU run (closes the P1 gate — highest-value
  evidence event) · oMLX restart + engine indicator · local-keys
  bring-up procedure.
- **P1:** H1–H15 wiki refresh batch · Q-32 lint rule for question rows ·
  Q-33/Q-34/Q-35 decisions · TLS-bind config test (promised in
  "done looks like") · Lean image build · workflows view/registry
  decision · remove-or-use `@xyflow/react`.
- **P2:** Quantum multi-geometry design · loop load/chaos tests ·
  Yjs/Mol*/WebAuthn marked VISION where implied · nightly-canary
  "caught a real regression" evidence watch (P3 needs it) ·
  per-tenant sharding decision · KEMTLS re-evaluation trigger.

## 5. Implementation plan

- **Phase G (wiki hygiene, direct):** H1–H15 fixes + Q-32/Q-33/Q-34/Q-35
  rows in `questions.md` + index recount + log. Acceptance: lint clean,
  zero stale counts, every open-question page rowed or reclassified.
- **Phase H (dead states + small builds, ordered):** suspended mutator
  or field removal (test either way) · PG/HNSW wire-or-dormant ·
  `useTelemetry` wire-or-note · `@xyflow/react` remove-or-use ·
  TLS-bind config test · local-keys procedure + `doctor` check ·
  workflows view/registry decision record. Acceptance: tests + live
  proof per item, CI green per slice.
- **Phase I (operator queue, in order):** oMLX restart (or documented
  cloud-default) · Lean image + warm run · reviewer/API/contact env ·
  EPO registration · vendor grading run · third-party review ·
  paid QPU run · Q-27 flip. Acceptance: receipts, not claims.
- **Non-goals:** Yjs/Mol*/WebAuthn builds · KEMTLS · sharding ·
  syndrome pipelines · IdP · horizontal scale — stay VISION until a
  question row promotes them.
