---
type: concept
name: Full-System Audit Round 1 — Findings Register (2026-10-26)
date: 2026-10-26
updated: 2026-10-26
tags: [audit, findings, verification, production, testing]
sources: [source-gap-analysis-plan, source-architecture-paper, source-llm-circuit-codesign-2026]
status: tracked
---

# Full-System Audit Round 1 — Findings Register (2026-10-26)

> **What this page is.** The complete findings record of the first
> execution of [[full-system-test-plan]] (Phase 1 only; Phase 2 and the
> out-of-scope list were not touched). Every item was found live —
> real Docker stack, real browser, real Strata LLM — and every fix was
> verified live with a regression pin proven to fail pre-fix. Fixes
> landed in `ed2d914` + `80259c3`; the round-1 plan execution is closed.
> Remediation of the *open* items lives in [[audit-remediation-plan]];
> nothing there has been implemented.

## Scope & method (so the register is auditable)

- Fresh compose stack from a clean checkout (`down -v`, rebuild),
  throwaway bearer token, all health gates green; isolated local rail
  (`kubonad --db /tmp/…`, host bridge, `STRATA_MODEL=iq3_s`); baselines
  green (Go 28/28, console 41 files / 194 tests, `tsc`, lint clean;
  Python tree untouched so the 1011-passed baseline stands).
- Diagram snapshot under test: `9c39c1b`. Findings cite
  `file:line` (post-fix tree) or command output; no finding without a
  repro. Severity: P0 stop-the-line · P1 broken feature / trust-boundary
  hole · P2 wrong-but-contained behavior · P3 polish/observability.

## Fixed this round (verified live, all green)

| ID | Finding | Evidence | Fix (commit) | Verification |
|---|---|---|---|---|
| F-R1-01 | `PUT /api/llm/settings` validation refused with HTTP 500 | `curl -X PUT …/settings -d '{}'` → 500 `active_model must not be empty` | `server.go` Update errors → 400 `bad_request` (`ed2d914`) | Live 400 + `TestLLMSettingsWriteGated` full-object PUT 200; pin fails pre-fix with the exact message |
| F-R1-02 | Unknown supervisor loop ids (`kill`/`pause`/`step`) → HTTP 500 | `POST /api/loops/no-such-loop/kill` → 500 `loop not found` | Same 500→400 class fix (`ed2d914`) | Live 404 on all three verbs; `TestUnknownLoopIDIs404` fails pre-fix on all three |
| F-R1-03 | Empty `sessions`/`jobs`/`autorefresh-loops` lists serialize as `null` | `{"jobs":null}` etc. on empty stores | Nil→`[]` normalization (`ed2d914`) | Live `[]`; per-endpoint pin tests |
| F-R1-04 | Garbage `preferred_model` ("@@@!!!###") routed as omlx-cloud $0.04 | Browser mesh route → "CLOUD ESCALATION ... Provider: omlx, $0.04" | Model-ID charset gate in `RouteMultimodal` (`ed2d914`) | Live refusal naming the malformation; well-formed bare ids keep fail-closed cloud verdict; mesh `iq3_s`→strata unaffected |
| F-R1-05 | Tribunal short replies fell back with zero log trace | Turn 2 produced 0 chars, no log line (found by elimination) | Log the length on short-reply fallback, all 4 turns (`80259c3`) | New log line observed live on the next contested run |
| F-R1-06 | Swarm Critic showed bare `canary 0.984` (template constant as measurement) | First-paint screenshot | `(template)` qualifier until `COLLAPSED_VERIFIED` + pin (`b79e159`, prior slice, re-verified this round) | Live screenshot; pin fails pre-fix |
| F-R1-07 | Radar `50051 OFFLINE` for a healthy bridge | Fleet Radar screenshot, loopback dial in `handleRadar` | Probe the configured `--bridge` addr (prior slice, re-verified: both ports LISTENING live) | — |
| F-R1-08 | HF/agent-registration refusals swallowed to `[]`/`null` (silent UX) | Browser: "No models found" on 401; modal closed with roster unchanged | Throw-on-refusal + in-modal `role=alert` (prior slices, re-verified live incl. honest toast) | — |

## Filed — open, not fixed (see [[audit-remediation-plan]] for paths)

| ID | Severity | Finding | Evidence |
|---|---|---|---|
| F-R1-09 | **P1** | **Cross-tenant session reads.** `GET /api/sessions` lists every lab's session IDs publicly; epistemics/campaign/events/messages for another tenant's session read with no auth and no tenant check. Memory recall *is* tenant-scoped and job reads 404 cross-tenant — the hole is specifically the session list + session-keyed reads. | `curl` matrix against `mandlab`/`envlab` sessions; `{"sessions":null}`-then-listed UUIDs replayed into `/api/epistemics`, `/api/campaign`, `/api/events` → 200 with foreign data |
| F-R1-10 | **P1** | **Restart-orphan loops.** DB `sleeping` rows survive restarts; the reconciler logs them but never rehydrates the supervisor registry, so they are unkillable by id (404 after fix) *and* block session delete ("live loops"). | Volume DB shows `status=running/sleeping` rows; API shows `[]`/404; delete refuses. Discovered while cleaning audit residue |
| F-R1-11 | **P2** | **ModelHub starts multi-GB downloads with no size confirm**, and the in-progress **Cancel button never stabilizes** (progress re-renders defeat actionability). API cancel works and cleans up. | 4.36 GB download started on one click; `Cancel` click times out; `POST …/cancel` → `cancelled`, 12K residue |
| F-R1-12 | **P2** | **Metasearch approve/reject/apply rails have no proposer.** `ProposeSearchRefinement` has zero callers outside its own tests, so the mutation rails can only ever 404. Fail-closed and correct — but dead. | `grep` shows no producers; ghost approve/reject refuse with named reasons *(see ⚑ Correction below — 400, and a mint path exists with discarded return)* |
| F-R1-13 | **P2 (process)** | **Import session succeeds silently** — 200 + `replayed:true`, zero UI confirmation of what changed. | Export→import round-trip verified byte-complete; no toast, diff, or session-switch signal |
| F-R1-14 | **P3** | **`ifc-core` lattice properties unpinned** (4 embedded tests exist — public-flows, private→public denial, one narrowing direction, cross-universe denial — but no transitivity, antisymmetry/reflexivity, empty-`Only({})` vacuous-subset, or join-LUB pins; qir-core has 13). | `lib.rs:70-118` 4 tests; Go pins subset in `lattice_test.go:9-73` |
| F-R1-15 | **P3 (observation)** | **Tribunal turn 2 is the flaky one** (timeout once, empty reply twice, authored once across 4 runs) while turns 1/3/4 are stable. Caps + session-id + logging all hold; the variance is model-side (Strata iq3_s). | Four runs: authored/timeout/empty/authored-fallback mix, every branch covered by coverage + obligation records |
| F-R1-16 | **P3 (noted, not pursued)** | **Sandbox timeout path untested live** (would need a >60s job through the container runtime); covered by unit tests + fail-closed review. **Paid/QPU paths** untested (no creds — correct). **Literature offline** untested (would need network isolation). | Code review + unit suite |

## Standing findings re-confirmed (not re-litigated)

- **F-001** — Python-backed rails 503 in the Docker image (no interpreter;
  `runBridgePython` execs a local venv). Unchanged.
- **F-006** — `/api/fidelity` ADMITS on empty input with a hardcoded
  `0.94` probe. Unchanged, still callerless.
- **Turn-bundle resume has no HTTP route** (Go API only) — recorded as
  the missing surface for a future console flow.

> ⚑ Correction (2026-10-26 deep dive — F-R1-12): the "404" above is wrong.
> Unknown proposal ids return **400** (`meta-search proposal not found`,
> `evolution.go:18`), asserted in `w2_production_bar_test.go:191-198`.
> More importantly a mint path **exists**: `SetStagnationCallback`
> (`server.go:3957-3971`) calls `GenerateStagnationProposal`
> (`evolution.go:206` is its sole production caller) but **discards the
> return**, so the proposal id never reaches the ledger event or the
> operator. The dead end is minting visibility, not UI/routes — the
> console already consumes the rails (`MetasearchPanel.tsx`).
>
> ⚑ Correction (2026-10-26 deep dive — F-R1-14): "zero tests" was stale
> at write time. `crates/ifc-core/src/lib.rs:70-118` holds **4** embedded
> tests; the row above now states the real gap (unpinned lattice
> properties). Note the Rust/Go asymmetry this exposed: Go fails closed
> on empty reader sets via `deny-all` (`lattice.go:100-109`) while Rust
> `Only({})` flows everywhere by vacuous subset — the new pins must
> decide whether that asymmetry is intended.

## Coverage achieved (battery → result)
- HUMAN: all surfaces/tabs/buttons/forms exercised; chat→tribunal→
  mandate→launch→COMPLETED loop twice (UI + server-verified `status: 3`);
  refusal toasts/modals; kill-tests both daemons with recovery;
  390px zero-overflow; keyboard 41/41 named stops; recorder export/
  download/import; air-gap toggle revert; session pills; guardrails
  approve-flow (Deny path verified with honest local toast).
- EDGE: 93/93 routes swept (1 new 500 found → F-R1-01); A2A card/
  `-32001`/402-challenge; auth matrix incl. full Ed25519 login/logout/
  revoke + tampered/expired/wrong-key; CORS allow/deny; rate-limit
  429s; TLS serve + plaintext refusal; SSE live; idempotency replay +
  409 conflict; canvas save/get; strict-vs-lenient recorded as-is.
- COG: tribunal consensus TRUE on default stack; template fallback;
  contested paths; mesh matrix incl. garbage refusal; agents CRUD;
  grants full cycle (mint→register→revoke with ledger receipts);
  mandates sign-gating (reviewer key required); replan MW fold
  (0.82→1.000 measured + notebook line); autoresearch verbs incl.
  disconnect-safe step abort.
- TRUST: ledger export both formats + `verify --db` clean; tenant
  envelopes end-to-end (bootstrap→ceiling→tamper/wrong-key/expired);
  disk guard BLOCK (+Retry-After, reads alive) via env thresholds;
  contracts catalog match; cross-tenant matrix (see F-R1-09).
- RUST/MCP/MEM/BRIDGE/BACKENDS/MODELS/SECRETS/CODESIGN: qir-core CLI
  vectors + exit codes; MCP 5 servers + 29-tool digest match;
  retain/recall + dormant pgvector contract; all 7 bridge RPCs
  (execute/cut/anchor/krylov/mitigate/wavefunction/literature);
  QPU fail-closed (jurisdiction→mandate→credential layers, each
  named); Strata default path incl. spend silence and mesh
  qualification; vault issue/revoke + no-key/charset/TTL-cap
  refusals; full codesign battery (TQ golden, evaluate family /
  spec_table / invalid, frontier best-pick, batch + 64-cap,
  families lineage, budget union bound, packs registry, verified
  JSON regen, per-trial recipes).

Related: [[full-system-test-plan]] · [[audit-remediation-plan]] ·
[[platform-architecture-diagram]] · [[frontend-backend-ui-ux-audit]]
