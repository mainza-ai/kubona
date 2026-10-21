---
type: concept
name: Audit Remediation Todo List (sequenced, file-anchored)
date: 2026-10-26
updated: 2026-10-26
tags: [audit, remediation, planning, todos, production]
sources: [source-gap-analysis-plan, source-architecture-paper]
status: tracked
---

# Audit Remediation Todo List (sequenced, file-anchored)

> Execution list for [[audit-remediation-plan]], built from the
> 2026-10-26 file-level deep dive. Every item names its files, its
> regression pin (proven to fail pre-fix), and its live verification —
> the same bar as round 1. **Nothing below is started.** Phase 0
> decisions gate everything; Phase 3 is operator-gated and must not be
> picked up on initiative. Slices land green or not at all; each slice
> is one commit, pushed to both branches, wiki-linked on landing.

## Phase 0 — Decisions (no code until each is recorded on its concept page)

- [ ] **D-09 Identity model for reads.** Cookie tenant exists
  (`api/auth_session.go:281-306`); bearer carries none (`:305`).
  Options: tenant-bound bearer, cookie-only scoping, or declared
  single-lab posture. Record in [[audit-remediation-plan]] R-09.
- [ ] **D-10 Settle vs rehydrate.** Settle-and-archive recommended;
  rehydrate risks double-execution of non-idempotent steps. Record
  the terminal disposition choice (new `archived` status vs DB-level
  kill — kill-from-sleeping is already legal at
  `supervisor.go:119`). Record in R-10.
- [ ] **D-12 Minter + quorum.** Confirm loop-stagnation (5 discards,
  `engine.go:561-576`) as the sanctioned minter; confirm default
  `human-quantum-reviewer` quorum (`server.go:4401-4403`).
- [ ] **D-15 Variance posture.** After T-15 ships its log line: prompt
  problem, bounded retry, or accepted variance.
- [ ] **D-01/D-02/D-03 Operator sign-off.** (a) vs (b) vs (c) for
  Docker python (recommendation: (a) chemistry-first); (b)-now vs
  (a)-if-promoted for fidelity; single-lab posture statement for R-03.

## Phase 1 — Now (approved pattern: small, safe, directly testable)

### T-09a · Caller-tenant helper + session-list scoping
- Files: `core/pkg/api/auth_session.go` (new helper beside
  `checkAuth`/`sessionPrincipal`), `server.go:1419-1430`,
  `session/session.go:153-176`.
- Steps: add the missing caller-tenant resolver; gate `GET
  /api/sessions` on it (cookie lists stay scoped as today at
  `auth_session.go:467`; bearer behavior follows D-09).
- Pin: two-tenant test — lab B token listing sees zero lab-A
  sessions (fails pre-fix: sees all 50).
- Live: curl matrix from round 1 rerun green.
- Depends: D-09. Effort: S.

### T-09b · Session-keyed read checks (13 handlers, one shape each)
- Files: `server.go:759-773` (epistemics), `:1592-1735`
  (by-id/campaign/claims/versions/messages/single),
  `:1615-1642` (synthesize), `:2991-3020` (events),
  `:1286-1315` (notebooks), `:1534-1548` (context),
  `:1762-1780` (chat), `:3167-3211` (canvas), `:1718-1728`
  (DELETE), `provenance.go:150-200` (ledger export).
- Steps: per handler, copy the `handleJobByID` shape
  (`server.go:2852-2880`) — require scope (400), resolve both
  tenants via `GetSession` (404 ghost), 403 on mismatch. Ledger
  export keeps its record-label check *and* gains the caller check.
- Pin: per-handler cross-tenant test (own-tenant 200, foreign 403,
  ghost 404, unscoped 400) — extend the round-1 matrix into
  `server_new_routes_test.go` style pins.
- Live: full 13-handler curl matrix + console surface re-verification
  (every surface that reads sessions; refusal toasts, never blanks).
- Depends: T-09a. Effort: M (mechanical, wide).

### T-10a · Reconciler skip-list fix (`killed→sleeping` resurrection)
- Files: `ledger/ledger.go:789-813` (add `killed` — and audit
  `paused/stalled/diverged/rescoped` — to the line-800 skip or map
  them explicitly).
- Pin: seed `killed` row → boot reconcile → still `killed`
  (fails pre-fix: flips to `sleeping`).
- Live: restart drill, DB inspected before/after.
- Depends: none (lands alone). Effort: XS.

### T-10b · Settle-and-archive path
- Files: `migrate.go:52` + `supervisor.go:91` + `supervisor.go:98` +
  `ledger.go:800` (four single-sources for the disposition, per D-10);
  `server.go:4104-4129` (release sequence to reuse);
  new DB-keyed handler (actions at `:4254-4256` and sweeper at
  `:4194` cannot reach engineless rows).
- Steps: terminal disposition; `ClaimLoopSpend` →
  tenant/session `ReleaseSpend`; append
  `autoresearch_loop_settled` (+ `loop_spend_released` on refund);
  session-scoped, tenant-checked settle route.
- Pin: orphan-sleeping + nonzero reservation → settle → terminal
  record, zeroed books, `DeleteBlockers` clear; kill-mid-loop →
  restart → settle → delete proceeds. Fails pre-fix at every step.
- Live: daemon-kill restart drill end to end.
- Depends: D-10, T-09a (tenant check on the new route). Effort: M.

### T-11 · Download confirm + stable cancel
- Files: `HuggingFaceModelHub.tsx:475-476` (gate),
  `:277-284` (cancel), `:93-110` (poller), `lib/api.ts:862-879`,
  `server.go:4881-4911` (server-side size cap).
- Steps: confirm dialog over threshold (e.g. 100 MB — client knows
  `q.size_bytes`); server enforces the same cap (never trust the
  client); cancel control with pending/disabled state outside the
  poll-churned row; keep `:152` honest-race copy.
- Pin: new `hfDownloadConfirm.test` — click with 4.36 GB size asserts
  confirm (fails pre-fix: POST fires); cancel-button actionability
  without `force:`.
- Live: Playwright — confirm appears, cancel clickable mid-download,
  partial cleaned.
- Depends: none. Effort: S.

### T-13 · Import receipt + `replayed` threading
- Files: `SessionPortability.tsx:74-104` (set success string on the
  existing `role="alert"` paragraph `:159-163`), `app/page.tsx:117-122`
  (accept `replayed`), copy `showToast` shape from
  `HuggingFaceModelHub.tsx:47-51` if a floating toast is wanted (no
  new dependency).
- Steps: success string names session title + event count;
  distinguish "imported N events" from "already present (replay)".
- Pin: extend `SessionPortability.test.tsx` — success sets receipt
  (fails pre-fix: nothing set); replayed-true copy asserted.
- Live: export→import round-trip shows receipt + switches session.
- Depends: none. Effort: XS.

### T-15 · Per-turn tribunal telemetry (zero behavior change)
- Files: `collective.go:647,692,758,804` (capture discarded `resp,
  dur`); `:659,701,767,813` (thread through); anchor source at
  `:520,544-547`.
- Steps: one log line per turn — `turn_idx, role, prompt_runes,
  reply_runes, dur_ms, llmTimeout_ms, turnCap, authored, anchorLive,
  breached, err`. No prompt, timeout, or fallback change.
- Pin: run tribunal with instrumented capture in test — line emitted
  with all fields (fails pre-fix: duration/prompt size absent).
- Live: contested run shows the T2 line; collect a week, then D-15.
- Depends: none. Effort: XS.

### T-14 · ifc-core lattice pins
- Files: `crates/ifc-core/src/lib.rs:70-118` (extend `mod tests`);
  `core/pkg/ifc/lattice_test.go` (mirror new denials).
- Steps, in order: transitivity (+ negative case);
  antisymmetry/reflexivity; empty-`Only({})` — first decide the
  Rust/Go asymmetry (Go `deny-all` at `lattice.go:100-109` vs Rust
  vacuous-subset flow) and pin the decision; join LUB
  (commutativity, idempotence, `x→x∨y`, leastness,
  empty-intersection tenant-mix mirroring `lattice_test.go:49-51`).
- Pin: the tests themselves (fail pre-fix by construction where the
  property is violated — especially the empty-set case).
- Live: `cargo test` + Go suite green together.
- Depends: asymmetry decision recorded (add to R-14). Effort: S.

## Phase 2 — Next (needs its Phase-0 decision first)

### T-12 · Stagnation-proposal visibility wiring
- Files: `server.go:3957-3971` (capture the discarded return at
  `:3958`); ledger event `autoresearch_stagnation_triggered`
  (+ `proposal_id`, `status`); SSE/event stream emission.
- Steps: **no new engine, no new callers, no file-writing apply.**
  Proposal surfaces in `GET /api/metasearch/proposals`; existing
  approve→apply→`loop.Resume()` (`:4474-4477`) becomes the positive
  half, round-1 ghost-refusals the negative half.
- Pin: force stagnation (5 discards in test) → proposal row listed
  with id (fails pre-fix: event carries no id); approve→apply→resume
  lifecycle test against the surfaced id.
- Live: slow-burn loop run to stagnation; console
  `MetasearchPanel` shows the proposal; approve→apply→resume
  observed; ghost ids still 400.
- Depends: D-12. Effort: S.

## Phase 3 — Gated (operator sign-off required; do not start)

### T-01a · Chemistry RPCs via bridge (staged first)
- Files: `kubona.proto:186-193` (+2 RPCs), `server.py` handlers
  calling `chemistry.py:252` + `:404`, regen `core/proto/*.pb.go`,
  repoint `server.go:3708` + `:3780`, keep 503 envelope.
- Pin: Docker-stack ADMET + pipeline green (currently 503 by
  construction); local-rail unchanged.
- Live: compose codesign battery, chemistry slice.
- Depends: D-01 (option a). Effort: ½–1 day.

### T-01b · Codesign RPCs via bridge (staged second)
- Files: same pattern for the 5 `proxyCodesign` ops
  (`codesign_eval.go:37-267`, `:292-340`); keep `runBridgePython`
  as dev fallback.
- Pin + live: Docker-stack codesign battery green per
  [[full-system-test-plan]].
- Depends: T-01a pattern proven. Effort: ½–1 day.

### T-02 · Fidelity honest default
- Files: `server.go:739-757` → `EvaluateAdmissionFloorOnly`
  (`accounting.go:125-151`) + `measured:false` /
  `probe_status:"not-run"`; update the 200-only test to pin the
  honest copy (stronger, not weaker).
- Pin: bare `GET /api/fidelity` → 200 with no probe-alignment
  sentence (fails pre-fix: "aligns with Clifford probe").
- Depends: D-02 ((b)-now; escalate to refuse-without-params if the
  endpoint is ever promoted to admission input). Effort: XS.

### T-03 · List-posture statement
- Write down R-09's posture; if multi-tenant consoles ever ship,
  the list endpoint takes the R-09 identity. Docs-only.
- Depends: D-09. Effort: XS.

## Landing checklist (every slice)

1. Pin fails pre-fix (stash-proven), passes post-fix.
2. Full affected suite green (`go test`, console tests, `tsc`,
   lint; `cargo test` for T-14).
3. Live verification in the topology where the finding was found
   (compose / rail / browser) + reverse check where behavior
   branches (refusal still refuses, ghost still 404/400).
4. One commit per slice, both branches, wiki row updated
   ([[full-system-audit-2026-10-26]] item → fixed with commit).

## Execution record (2026-10-26 — all landed except T-01a/b)

Phase 0 decisions recorded above; implementation ran slice-by-slice,
each with a stash/mutation-proven pin, full suites, and live drills
where the finding was found. Commits (develop, all pushed both
branches with the wiki batch):

- T-10a `8463f25` · T-15 `e5d2dd8` · T-13 `00ed7e8` · T-14 `7889641`
- T-11 `abcf0a9` · T-09a/b `98f6ad0` (incl. 7 fixture tests re-pinned
  to operator minting — anonymous named-lab creation was the spoof)
- T-10b `9ac485a` + `6f298ee` (live drill forced the terminal-skip
  refinement) · T-12 `a54ce0c` · T-02 `f6f298ee` · T-16 `a54ce0c`
- T-03 closed by D-09 (posture in `tenant_reads.go` header + R-09).
- T-09c filed (session-keyed mutations need separate threat analysis).
- T-01a/b open: toolchain verified (exact plugin versions, regen
  recipe above), wire change + Docker rebuild kept as its own gated
  slice.

Live drills (fresh rail, `live-token`): single-lab anon 200 →
multi-lab anon 401 → bearer 200 → spoof-create 403; killed row
survived restart, sleeping settled to aborted, session deleted;
fidelity bare call honest ADMIT with `probe_run:false`; tribunal run
emitted `turn-telemetry` lines (turn 2's 71 s serial wait visible).
Suites: Go 29/29, console 42 files / 200 tests, tsc, wiki lint,
Python taxonomy 4/4, ifc-core 9/9.

Related: [[audit-remediation-plan]] ·
[[full-system-audit-2026-10-26]] · [[full-system-test-plan]] ·
[[implementation-gaps]]
