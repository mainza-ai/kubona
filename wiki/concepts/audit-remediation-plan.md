---
type: concept
name: Audit Remediation Plan (findings F-R1-09 through F-R1-16, F-001, F-006)
date: 2026-10-26
updated: 2026-10-26
tags: [audit, remediation, planning, gaps, production]
sources: [source-gap-analysis-plan, source-architecture-paper]
status: tracked
---

# Audit Remediation Plan

> **What this page is.** The fix plan for every *open* finding in
> [[full-system-audit-2026-10-26]] plus the two standing items (F-001,
> F-006). **Nothing below is implemented.** Each item names the
> decision to make first, the recommended path, and the acceptance test
> that closes it. Rule of the house: a slice lands green or not at all —
> findings fix with a regression pin proven to fail pre-fix, verified
> live where the finding was found (browser/Docker/rail, not just unit).
> Sequencing: **Now** (small, safe, same-bar as round-1 fixes) →
> **Next** (needs a design decision first) → **Gated** (needs an
> operator product call). Do not start Next/Gated items without recording
> the decision on the linked concept page.

## Now (approved pattern — small, safe, directly testable)

### R-09 · Cross-tenant session reads (P1 — from F-R1-09)
- **Deep-dive 2026-10-26.** `handleSessions` (`server.go:1419-1430`)
  calls `ListSessions(50)` with no tenant filter (`session.go:153-176`
  has no `WHERE tenant_id`). Every session-keyed read
  (`/api/epistemics`, `/campaign`, `/claims`, `/campaign/versions`,
  `/events`, `/results/notebooks`, `/sessions/context`,
  `/chat/messages`, `/canvas/layout`, `synthesize`, single-session GET,
  `DELETE /api/sessions/:id`, ledger export) resolves the *session's*
  tenant but never compares it to the *caller's* — none of them call
  `checkAuth`/`sessionPrincipal`. There is no `tenantFromRequest`
  helper anywhere in `core/pkg`. Identity reality: cookie callers carry
  a tenant (`auth_session.go:281-306` returns it; `handleAuthSessions`
  even scopes cookie lists by it), but **bearer tokens carry no
  tenant** (`auth_session.go:305` returns `""`, comment at `:455-468`
  says "nothing to scope to"). The copy pattern is
  `handleJobByID` (`server.go:2852-2880`): require `session_id` scope
  (400 unscoped), resolve both tenants via `GetSession` (404 ghost),
  403 on mismatch (`server.go:2876-2878`).
- **Decision first:** which identity scopes reads — cookie tenant,
  a tenant-bound bearer, or declared single-lab posture. Bearer-tokens
  must gain a tenant binding (or be refused on multi-tenant reads)
  before any check can cover them; cookie-only scoping leaves the
  documented bearer path (`console` sends bearer-or-cookie) half-open.
- **Recommended path:** (1) add the missing caller-tenant helper;
  (2) gate `GET /api/sessions` (list) + all session-keyed reads +
  `DELETE` on caller-vs-session tenant equality, following the
  `handleJobByID` 400/404/403 shape; (3) tenant-bind bearer tokens or
  refuse them on these reads with a named reason. Console sends both
  credentials already — verify every surface still renders (no silent
  blanks; refusal toasts, not empty lists).
- **Alternative (if multi-tenancy is not a deployment target):** declare
  single-lab posture and record it here; the check becomes "list shows
  only the caller's lab" once identity exists.
- **Acceptance:** the round-1 repro (list → replay UUID into another
  tenant's reads) 401s/403s with a named reason; own-tenant reads
  unaffected; console surfaces re-verified (toasts, not blanks).

### R-10 · Restart-orphan loops (P1 — from F-R1-10)
- **Deep-dive 2026-10-26.** `ReconcileLoopsOnBoot`
  (`ledger.go:789-813`, called from `main.go:226-233`) flips every
  non-terminal row to `sleeping` but rehydrates **nothing**: neither
  `Supervisor.loops` (`supervisor.go:147-170`) nor `activeAutoloops`
  (`server.go:297-331`) is repopulated, spend books untouched. Two
  extra traps: `"killed"` is **not** in the skip list (`ledger.go:800`),
  so supervisor-killed rows resurrect to `sleeping`; and `sleeping` is
  deliberately non-terminal (`migrate.go:47-48`, wake resumes), so
  orphans pin their session via `DeleteBlockers` (`migrate.go:617-651`)
  while no DB-keyed settle path exists (actions 404 without an engine
  at `server.go:4254-4256`, the sweeper skips sleeping at `:4194`).
- **Decision first:** rehydrate sleeping DB loops into the supervisor
  on boot, or settle-and-archive them (terminal receipt + event)?
- **Recommended path:** settle-and-archive — a loop whose engine died
  with its daemon cannot safely resume mid-step. Concretely: (1) a
  terminal disposition for `sleeping` added to **all four**
  single-sources (`session.TerminalLoopStatuses` at `migrate.go:52`,
  `supervisor.TerminalLoopStatuses` at `supervisor.go:91`,
  `validLoopTransition` at `supervisor.go:98`, reconciler skip at
  `ledger.go:800` — cheapest reuse is DB-level kill, since supervisor
  `Kill` from `sleeping` is already legal at `supervisor.go:119`);
  (2) release both reservation rails via the `ClaimLoopSpend` →
  `tenantMgr`/`sessionMgr.ReleaseSpend` sequence (`server.go:4104-4129`,
  idempotent through the conditional zero); (3) append a ledger event
  (`autoresearch_loop_settled` + `loop_spend_released` when refund > 0)
  — silent UPDATEs break the hash-chain audit contract; (4) a DB-keyed,
  session-scoped, tenant-checked settle path, since the action handler
  cannot reach engineless rows; (5) fix the `killed→sleeping`
  resurrection in the skip list regardless.
- **Acceptance:** kill daemon mid-loop → restart → reconciler logs the
  disposition; API shows a terminal record (never `[]` while the DB
  says `running`); session delete proceeds after settle.

### R-11 · ModelHub download safety (P2 — from F-R1-11)
- **Deep-dive 2026-10-26.** Single click wires straight to
  `startHFDownload` (`HuggingFaceModelHub.tsx:475-476` → `:135-147`);
  the size is known client-side (`q.size_bytes`, `:468-472`, from HF
  `siblings[].size` via `hub.go:327-330`) but never gated, and the
  server validates only non-empty fields (`server.go:4892-4895`). The
  Cancel button (`:277-284`) lives inside the row re-created by the
  3 s visibility-gated poller (`:93-110` → `setDownloads` → full
  re-render at `:229`), has no pending state, and unmounts the instant
  status flips — server cancel itself is correct and cleans up
  (`.part` target at `hub.go:470`, removed on cancel at `:522`,
  once-only terminal at `:427-433`).
- **Path:** (a) size-confirmation gate before any fetch over a threshold
  (e.g. 100 MB — "4.36 GB — download?"), enforced client *and*
  server-side; (b) a cancel control outside the progress re-render path
  (stable target, pending/disabled state surviving the poll cycle);
  keep the honest terminal-race toast copy (`:152`).
- **Acceptance:** one click never starts a multi-GB fetch; cancel is
  clickable mid-download in automation (Playwright actionability,
  no `force:`); partial file cleaned on cancel (already true
  server-side — keep it).

### R-13 · Import confirmation (P2 — from F-R1-13)
- **Deep-dive 2026-10-26.** There is no toast library (`package.json`
  has no sonner/hot-toast); three hand-rolled patterns exist, and
  `SessionPortability.tsx` uses the weakest — an inline amber
  `role="alert"` paragraph (`:159-163`) that only ever receives
  failure strings, zero `toast` hits in the file. Success drops
  through `onSessionImported(res.session_id, res.replayed)` (`:98-99`)
  while `app/page.tsx:117-122` takes only `sessionId` — **`replayed`
  is silently dropped**, so import-vs-replay is indistinguishable.
- **Path:** set the existing `role="alert"` paragraph with a success
  string naming what imported (session title, event count) *and*
  thread `replayed` through to distinguish "imported N events" from
  "already present (idempotent replay)"; keep the explicit session
  switch as the second signal. Copy the ModelHub `showToast`
  shape (`HuggingFaceModelHub.tsx:47-51`) if a floating toast is
  wanted — do not add a dependency for one string.
- **Acceptance:** import round-trip ends with a visible, accurate
  receipt; silent success is gone.

## Next (needs a design decision recorded first)

### R-12 · Metasearch proposer gap (P2 — from F-R1-12)
- **Deep-dive 2026-10-26 (corrects the register).** Handlers return
  **400**, not 404, on unknown ids (`evolution.go:18`,
  `w2_production_bar_test.go:191-198`); mux 404 is path-typo only. A
  mint path **exists**: `SetStagnationCallback`
  (`server.go:3957-3971`) fires on 5 consecutive discards
  (`engine.go:561-576`) and calls `GenerateStagnationProposal`
  (`evolution.go:206` is its sole production caller) — but **discards
  the return**, so the proposal id never reaches the ledger event
  (which carries only `loop_id/discards/harness_id`) or any SSE
  stream. Console already consumes the rails (`MetasearchPanel.tsx`);
  the dead end is minting visibility. Threshold (5 discards) is rarely
  hit in short runs, and the callback runs in a goroutine.
- **Decision first:** is loop-stagnation the sanctioned minter (no
  anomaly detector exists; supervisor has no stagnation concept), and
  what quorum approves (default `human-quantum-reviewer`, `:4401-4403`)?
- **Path (no new engine):** capture the return at `server.go:3958`
  and append `proposal_id` + `status` to the
  `autoresearch_stagnation_triggered` ledger event + emit on the event
  stream. Then `GET /api/metasearch/proposals` (`:4383`) shows the row
  and the already-coded approve→apply→`loop.Resume()` (`:4474-4477`)
  becomes testable end-to-end (round-1 ghost-refusals are the negative
  half). Do **not** add direct `ProposeSearchRefinement` callers or
  file-writing apply — `ApplyProposal` (`evolution.go:252-274`)
  deliberately writes no files until Phase-5.
- **Acceptance:** propose → approve → apply → loop behavior changes,
  all ledger-sealed; ghost paths still refuse.

### R-14 · ifc-core lattice pins (P3 — from F-R1-14)
- **Deep-dive 2026-10-26 (corrects the register).** `lib.rs:70-118`
  holds **4** embedded tests; copy qir-core's habits (table-driven
  refusals at `qir-core/lib.rs:242-263`, invariant-output assertions,
  boundary tests). Missing pins, in order: (1) **transitivity**
  (`a→b ∧ b→c ⇒ a→c` + negative case); (2) **antisymmetry/reflexivity**
  (only public→public pinned at `lib.rs:82`); (3) **empty `Only({})`**
  — flows everywhere by vacuous subset, while Go fails closed via
  `deny-all` (`lattice.go:100-109`): decide whether the asymmetry is
  intended, then pin it; (4) **join LUB properties** (commutativity,
  idempotence, `x→x∨y`, leastness; empty-intersection tenant-mix —
  Go asserts `tenant/a + tenant/b → deny-all`,
  `lattice_test.go:49-51`).
- **Path:** add the pins to the existing `mod tests`; mirror the new
  denials in `lattice_test.go` where the Go side must agree.
- **Acceptance:** `cargo test` covers the lattice, not just compiles it.

### R-15 · Tribunal turn-2 flakiness (P3 observation — from F-R1-15)
- **Deep-dive 2026-10-26.** Turn 2 is **not** the longest prompt — T4
  is (~1.5× summaries + JSON schema). T2's uniqueness is *semantic
  coupling*: it is the only turn depending on an external async
  measurement (`ComputeClassicalAnchor`, 15 s timeout at
  `collective.go:536-548`, live `PySCF FCI` at `:544` vs canonical
  fallback at `:520`), the only turn whose coverage is
  `anchorCoverage` rather than `authoredCoverage` (`:712` — an
  LLM-authored turn can still be `uncovered` if the bridge was down,
  `:305-312`), and the only turn whose failure aborts the run
  (`:720-736`, skips 3-4, `ConsensusPassed=false`). `CompleteCapped`
  returns `(content, resp, duration, err)` (`llm/client.go:189-191`)
  but all four call sites (`:647,692,758,804`) discard
  duration/usage; success paths log no reply length, duration, or
  prompt size.
- **Decision first:** once measured — prompt problem, sampling
  problem (bounded retry on empty), or accepted variance?
- **Path (zero behavior change):** at the four `CompleteCapped` sites
  capture the discarded returns and log one line per turn —
  `turn_idx, role, prompt_runes, reply_runes, dur_ms, llmTimeout_ms,
  turnCap, authored, anchorLive (T2), breached, err`. That separates
  "T2 prompt is bigger" (currently disprovable) from "T2 serially
  waits on anchor+LLM" from "officer system prompt yields
  short/refused replies tripping the `len>20` gate". Thread through
  the existing `Turn N fell back … (%d chars)` lines
  (`:659,701,767,813`).
- **Acceptance:** empty/timeout turns stay contested (already true);
  only the *rate* may move, with evidence.

## Gated (operator product calls — do not touch without sign-off)

### R-01 · Python exec rails in Docker (standing F-001)
- **Deep-dive 2026-10-26.** `Dockerfile.kubonad` is `debian:bookworm-slim`
  + `ca-certificates curl` only — zero Python by construction. Three
  `runBridgePython` call sites serve **7 deterministically-503** rails
  in prod compose: `handleChemistryADMET` (`:3708`), `handleDiscoveryPipeline`
  (`:3780`), and `proxyCodesign` (`codesign_eval.go:292` → 5 handlers).
  Meanwhile the bridge already carries the deps (`Dockerfile.bridge:26`),
  already imports the functions (`server.py:21,25`), and compose already
  gates kubonad on bridge health — the proto just has no RPCs for them
  (7 exposed: `kubona.proto:186-193`; ADMET at `chemistry.py:252`,
  pipeline at `:404`, codesign snippets at `codesign_eval.go:37-267`,
  fidelity probe at `fidelity.py:100-170` all unwired). No generic
  "run this snippet" RPC — that re-creates the arbitrary-exec hole.
- **Options:** (a) route codesign/chemistry/discovery through the
  bridge gRPC service like krylov (consistent, larger); (b) ship
  python+venv in the kubonad image (simpler, heavier, supply-chain
  surface grows — duplicates the whole bridge tree, breaks slim/nonroot
  minimalism); (c) document codesign as local-rail-only and make
  the console degrade explicitly (smallest, weakest product —
  consecrates 7 dead routes in the supported topology).
- **Recommendation: (a), staged chemistry-first.** ~½–1 day per slice
  (proto + regen + handlers + Go repoint + tests): chemistry 2 RPCs,
  then codesign 5; fidelity-probe RPC can ride along (ties to R-02).
  Keep `runBridgePython` as the dev fallback, not the prod path.
- **Acceptance:** whichever is chosen, the Docker-stack codesign
  battery from [[full-system-test-plan]] goes green on the shipped
  image — or the docs state the limitation where the user meets it.

### R-02 · Fidelity default probe (standing F-006)
- **Deep-dive 2026-10-26.** `handleFidelity` (`server.go:739-757`)
  defaults empty input (`ibm_heron`, depth 12, 8 gates) and passes a
  **literal `0.94`** (`:755`) — no param is read. Math: predicted
  0.982 vs probe 0.94 → drift 0.042 < 0.15, above floor 0.60 → ADMIT
  citing *"aligns with Clifford probe"* when no probe ran. Blast
  radius is small: sole production caller of 4-arg `EvaluateAdmission`
  is `:755`; spend dispatch uses `EvaluateAdmissionFloorOnly`
  (`jobs/admission.go:371`), console never fetches the endpoint,
  and the test asserts only HTTP 200. The bridge has a real probe
  (`probe_clifford_fidelity`, `fidelity.py:120-170`,
  `measured:False` + `PROBE_METHOD_V1`) that Go never calls.
- **Options:** (a) refuse without explicit `measured_clifford` param
  (never ADMIT on defaults; changes the 200 contract; only the smoke
  test depends on it); (b) re-route through the already-tested
  `EvaluateAdmissionFloorOnly` (`accounting.go:125-151`) + emit
  `measured:false` / `probe_status:"not-run"` (one-line logic change,
  preserves 200, kills the false probe sentence; FloorOnly ADMIT on
  defaults is at least honest prediction).
- **Recommendation: (b) now, (a) if the endpoint is ever promoted**
  from display to admission input — a measurement-gated endpoint must
  refuse, not default.
- **Acceptance:** no caller can read `ADMIT` + `0.94 measured` from
  a zero-input call; `/api/fidelity` contract test pins it.

### R-03 · Public session list posture (extends R-09)
- If multi-tenant consoles ever ship, the list endpoint needs the
  tenant identity the R-09 decision introduces. Until then, R-09's
  single-lab posture (if chosen) must be written down, not assumed.

## Explicitly not in this plan

- Paid/QPU execution paths, x402 live rail, third-party review,
  load/chaos, public-repo flows — same exclusions as the test plan.
- Re-litigation of settled constraints ([[overview]] §Settled
  constraints) — propose a new decision with evidence instead.

Related: [[full-system-audit-2026-10-26]] ·
[[full-system-test-plan]] · [[frontend-backend-ui-ux-audit]] ·
[[implementation-gaps]] · [[risks]]
