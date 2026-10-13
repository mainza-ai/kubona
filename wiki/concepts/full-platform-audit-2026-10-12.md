---
type: concept
name: Full Platform Audit 2026-10-12 — completeness report
date: 2026-10-12
updated: 2026-10-12
tags: [audit, implementation, completeness, verification, production]
sources: [source-gap-analysis-plan, source-architecture-paper, source-frontier-repo-audit-2026, source-agent-protocol-landscape-2026, source-openai-math-corpus, source-cloudflare-economy]
status: tracked
---

# Full Platform Audit 2026-10-12 — what is done, what remains

> **Verdict: ~73% implemented** (simulator-grade production core; see §1 for what
> the number means and §6 for the full score table). This page is a dated finding
> record: wiki read page-by-page (116 pages), code inventoried package-by-package,
> claims checked against `file:line`, suites executed, daemon probed live.

## 1. Methodology — read this before quoting the number

- **Wiki pass:** all 116 lint-counted pages triaged; the 5 `settled` + 43 `adopted`
  concept pages read in full across three verification batches; entities grepped for
  implementation-state verbs and the 8 strongest claims verified; 7 `open-question`
  pages confirmed still open. Machine lint (`scripts/wiki-lint.py --strict`) green;
  0 orphan pages.
- **Code pass:** every Go package (26 + `cmd/kubonad`), all console sources, all
  bridge modules, and the legacy Python tree inventoried with LOC + test counts.
- **Scoring:** each component scored 0–100 on evidence tiers — 100 = live-proven
  **and** test-enforced; ~85 = test-enforced; ~60 = implemented, thin tests;
  ~30 = partial/stub; ~10 = staged but honestly labelled dormant; 0 = absent.
  The headline is the **unweighted mean of 30 components = 73.0%**. It measures
  *implementation completeness of the design as built on simulators* — not
  blueprint decidedness, and not production readiness (which needs §7's deeds).
- **Execution proof (2026-10-12):** Go `go test ./...` — 27 packages ok, 291 test
  funcs, 0 failures, `go vet` clean. Python `pytest tests/` from `platform/` —
  **402 passed, 3 skipped** (run from repo root fails 5 tests on CWD-relative
  fixture paths — a test-hygiene footgun, not a product bug). Console
  `pnpm test` — **78/78 across 16 files**. Rust `qir-core` builds, 0 unit tests.
  Live daemon probes: 15+ routes 200/201/402/404-as-specified, SSE streams.
- **Velocity:** 215 commits since 2026-10-01, 76 since 2026-10-11 (branch `develop`).

## 2. Code inventory (measured, not estimated)

| Tree | Size | Tests | Notes |
|---|---|---|---|
| `platform/core` (Go 1.27.1) | 20,658 LOC, 26 pkgs + `cmd/kubonad` | 59 test files, 291 test funcs | 71 HTTP routes; single `kubonad` binary |
| `platform/console` (Next 16 / React 19) | 13,188 LOC, 48 src files | 16 test files, 78 tests (vitest) | 56 exports in `lib/api.ts`; 12 surface components |
| `platform/bridge` (Python gRPC) | 2,799 LOC | 1 test file (278 LOC) | 7 RPCs in `QuantumBridgeService`; deps unpinned (`>=` floors) |
| `platform/kubona` (legacy Python) | 17,970 LOC, 158 files | 80 test files, 402 tests | `console_api.py` down to 19 Starlette routes (duplicates retired) |
| `platform/crates` (Rust) | 2 crates (`ifc-core`, `qir-core`) | builds clean; 4 + 4 unit tests green |
> ⚑ Correction (2026-10-12, same day): the filed "0 unit tests" was my own
> `cargo test -q | tail` truncation hiding the lib block — both crates carry
> 4 unit tests, all passing (`qir-core`: Bell parse, u3 decomposition, QIR
> emit, QFT-4 analysis; `ifc-core`: 4 lattice tests). Verified direct. builds clean; role shrinking toward text-IR |
| `platform/domain_packs` | 3 `pack.yaml` manifests | covered by `test_domain_packs.py` | chemistry, materials, math-physics |

## 3. What is done (>85 — test-enforced, several live-proven)

- **Spend mandates & delegation (95):** browser Ed25519 canonical mandates,
  Go `/api/mandates/verify`, scope-narrowing + depth-cap-2, atomic CAS spend
  (`spent+?<=ceiling`) with release-on-failure, per-tenant A2A sessions.
- **Provider capability schema (95):** measured-vs-declared canary (qubits,
  calibration ±25%), unknown→`None`, pricing-until-metered.
- **Tribunal collective (90):** 4-turn `ExecuteTribunal`, per-role routing,
  unknown-session 404 (K4 + G7), spend-gated entries.
- **Tenants = labs + suspend (90):** first-write bootstrap, signed
  rotation/ceiling/autonomy, `POST /api/tenants/status` freeze with
  recovery-preserving semantics (Q-33), session-attribution strictness (G7).
- **Jobs dispatch + admission (90):** QASM validation, autonomy gate, fidelity
  floor, grant admission, L0/suspend refusals that name their cause.
- **Policy parity (90):** shared `policy_parity.json` driven green by both rails.
- **A2A transport (90):** external JSON-RPC live (`/api/a2a/v1`: card, `-32601`,
  status, 402-gated submit) on the ledger-bound executor (Q-35 corrected).
- **Replan + claims (90):** pure-fold replan, `clm-` records, campaign versions
  (cap 50), MW expressibility fold proven live.
- **Math-physics anchors (85–90):** 20 families, engine + pack + REST, corpus tests.
- **Cost governance (85):** L0–L3 ladder, ceilings, global LLM cap with 429,
  per-call spend estimation.
- **Metasearch (85):** evolution engine, bearer-gated approve/reject/apply.
- **Bridge chemistry (85):** PySCF FCI anchor, 14-row live family matrix, honest
  ADMET/literature values (no invented citations/counts).
- **Console surfaces + gating (85):** ResultsStudio `COLLAPSED_VERIFIED`-only,
  MeasuredOutputs `COMPLETED`-only, mandate cards, tenant LabsView.
- **Quantum triage (85):** Family-278 routing live, advisory-only by decision.
- **Chat/stream/regenerate/launch (85):** ghost-session 404s, fresh-mint on
  empty, console flows intact (always sends ids).
- **Evals & corpora (88–90):** HumanEval-136 licence-gated, Katas, canary,
  truncation curves, check-critic, SDK conformance suite (Q-24 artifact).

## 4. Substantially there (60–85 — works, thinner proof or scoped edges)

- **Memory recall (80):** Hindsight SQLite retain/recall live + tenant-isolated;
  recall 404s ghost sessions (G7, probed live).
- **Autoresearch loops (80):** 5 harnesses, session-attributed starts;
  load/chaos tests still open.
- **Bridge krylov/cutting/mitigation (80):** emulation labelled, ZNE recovers,
  wire-cut verified vs uncut; quantum multi-geometry absent.
- **Secrets runbook (80):** fail-closed 503/401s, env-only DSN, Bao `from_env`
  without silent fallback; the live `gho_…` token remains an operator deed.
- **Claim wavefunctions (80):** `evaluate` + ledger payloads + worked H2 test;
  provenance-server method names differ from one page's claim.
- **Manuscript synthesis (75):** generator + ≥9.0/hygiene-10.0 gate, tests
  green; bilingual Lean/LaTeX synthesis partially wired.
- **Literature grounding (75):** live arXiv/OpenAlex with honest offline-empty,
  rate limits, licence flags; vendor binding deferred by decision.
- **Fidelity accounting (70):** admission floor + leave-one-out budgets live;
  fidelity ledger table not found (evaluated, not persisted).
- **AuthN model (60):** bearer + limiters + CORS allowlist enforced and tested —
  but the wiki's "OAuth2.1 everywhere" is refuted: local auth is bearer-token,
  IdP is Phase-4 work.
- **QIR compilation (60):** text-IR Analyze gate live; QASM3 refused **by
  locked decision**, full LLVM lowering + QUIR/Pulse absent.
- **Lean comparator (55):** AST-first path live with ledger records; Hub path
  unresolvable, container path partial — $\rho=1.0$ is a pathway, not a tap.
- **Provenance depth (50):** hash-chained ledger + `kubonad verify` real;
  circuit→noise→cost full-chain schema and crash-recovery claims unmapped.
- **Rust crates (50):** build clean; WASM/PyO3/linearity claims refuted —
  the compiler role is shrinking to text-IR, honestly.

## 5. Dormant, staged, or missing (<40 — labelled, not hidden)

- **x402 settlement (10):** `simulated:true`, local-only, unstaffed — dormant
  by decision (Q-23), needs Phase-5 staffing + chain choice.
- **PG/HNSW sidecar (10):** complete code, zero importers, in-code dormant
  banner + contract test (Q-34); needs operator PG + driver + wiring.
- **Workflows & agent authoring (35):** YAML engine validates only;
  triggers/gates/dry-run/library/MCP-registry absent.
- **MCP edge (40):** 4 scoped servers + stdio role + sandbox exist; gateway
  allow-lists deleted (F-5), SDK-2.3.0 consequences partial.
- **Refuted design claims found and corrected during this audit:** "OAuth2.1
  everywhere" (bearer reality), "QASM3→QUIR/Pulse pipeline" (QASM2-subset by
  decision), "Circuit Knitting Toolbox integration" (custom splitter only),
  "per-file CI licence table" (no such table in `ci.yml`/`scripts/`),
  "8 observatory surfaces" (12 components on disk — count removed).

## 6. Full score table (the 73.0%)

| # | Component | Score | # | Component | Score |
|---|---|---|---|---|---|
| 1 | Spend mandates & delegation | 95 | 16 | QIR compilation | 60 |
| 2 | Provider capability schema | 95 | 17 | Lean comparator | 55 |
| 3 | Tribunal collective | 90 | 18 | Provenance depth | 50 |
| 4 | Tenants + suspend + attribution | 90 | 19 | Rust crates | 50 |
| 5 | Jobs dispatch + admission | 90 | 20 | MCP edge | 40 |
| 6 | Policy parity | 90 | 21 | Workflows & authoring | 35 |
| 7 | A2A transport | 90 | 22 | x402 settlement | 10 |
| 8 | Replan + claims + versions | 90 | 23 | PG/HNSW sidecar | 10 |
| 9 | Math-physics anchors | 88 | 24 | Memory recall | 80 |
| 10 | Evals & corpora | 88 | 25 | Autoresearch loops | 80 |
| 11 | Cost governance | 85 | 26 | Bridge krylov/cut/mitigate | 80 |
| 12 | Metasearch | 85 | 27 | Secrets runbook | 80 |
| 13 | Bridge chemistry | 85 | 28 | Claim wavefunctions | 80 |
| 14 | Console surfaces + gating | 85 | 29 | Manuscript synthesis | 75 |
| 15 | Quantum triage | 85 | 30 | Literature grounding | 75 |

Mean: 2191 / 30 = **73.0%**. Batch cross-checks: governance 71.7, mechanisms
69.8, science/surface 73.0 — three independent passes converge within ±2 points.

## 7. What remains (grouped by who can close it)

- **Operator deeds only (nobody else can):** revoke the `store`-helper `gho_…`
  token; Q-27 history flip (`filter-repo`, force-push, re-clone); vendor
  credentials for a paid QPU run (the P1 gate as written); GPU host for GQE
  retraining + Unsloth personas; third-party security review (P3); public repo
  + outside contribution (P4); business sign-off on monetization (P5).
- **Deferred by design (decided, not missing):** PG wiring, x402 activation,
  IdP (P4), human lab membership (P4), per-tenant sharding (needs sign-off),
  full LLVM lowering, KEMTLS re-eval, CKT integration, Bemba/Typst output.
- **Buildable next (no operator needed):** workflow triggers/gates/dry-run,
  fidelity ledger persistence, loop load/chaos tests, quantum multi-geometry
  scans, pytest CWD-robustness (done 2026-10-12: `__file__`-anchored paths,
  403 green both CWDs), bridge dep pinning (done 2026-10-12: `==` pins +
  3.11 marker), ~~Rust unit tests for `qir-core`~~ (struck 2026-10-12: both
  crates already carry 4 green unit tests — see §2 correction).
- **Wiki-refresh debt (fresh findings, done 2026-10-12):** [[overview]] counts
  re-based (34 closed / 18 open, lint census, measured suites) and
  [[roadmap]] state-updated with a Go-core-era block; phase-status blocks
  marked historical.

## 8. Standing of this report

Three subagent verification passes + direct execution evidence agree: the
platform is a **simulator-grade production core at roughly three-quarters
implementation** — the trust rail (spend, tenants, mandates, parity) is the
most complete part, the science-discovery surface the least. Nothing audited
was found to be fictionally claimed as live except items already corrected
in-audit (§5). The remaining quarter is one-third operator deeds, one-third
deferred-by-decision, one-third buildable work listed above.

Related: [[implementation-gaps]] · [[overview]] · [[roadmap]] · [[questions]] ·
[[platform-architecture-diagram]] · [[production-completion-audit-2026-10-11]] ·
[[deep-implementation-audit-2026-10-12]] · [[full-wiki-audit-2026-10-12]]
