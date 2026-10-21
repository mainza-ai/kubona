---
type: concept
name: Frontend–Backend–UI/UX Audit
date: 2026-10-20
updated: 2026-10-20
tags: [ux, console, api, wiring, audit, planning, security]
sources: [source-gap-analysis-plan, source-architecture-paper, source-llm-circuit-codesign-2026]
status: tracked
---

# Frontend–Backend–UI/UX Audit: Wiring, Gaps, and Implementation Plan

> **What this page is.** A full-stack audit of the product surface as of
> 2026-10-20: what the Next.js console calls, what the Go orchestrator
> serves, where the two diverge, and what the UI/UX must become to carry
> the platform's new capabilities (Strata local default, literature
> factory-cost model, verified RS decoder, TQ variants). Two explorer
> passes mapped both sides end-to-end (routes, contracts, auth, gaps);
> every claim below was re-verified against the cited `path:line`.
> Companion plans: [[circuit-codesign-ux-plan]] (codesign slices 2–5),
> [[ui-ux-vision]] (the 10 principles + 9 surfaces), [[agent-ux-control-plane]].
>
> Standing rule: console stays **read/approve-only** ([[ui-ux-vision]] §3.1,
> hard rule #1). Nothing here proposes job submission from the browser.
>
> ## 0. The wiki UI standard this audit is judged against
>
> There is no single "less data dense" page — the standard is distributed,
> and it is stricter than any one line. Assembled from [[ui-ux-vision]] and
> [[agent-ux-control-plane]]:
> - **Calm instrumentation** (principle §3.10): the console is an oversight
>   layer, not a dashboard that shouts; mobile-first check-in presence.
> - **One-glance reviewer** (§2 persona; §5.4 action gates; control-plane
>   §int-principles): a reviewer answers "what/why/cost/trust" in one
>   glance; P2 gate = watch, understand, approve, interrupt in 60 seconds.
> - **Progressive disclosure** (§3.3): deep circuits and long loops default
>   to summarized stats, drill-down on demand — never a firehose.
> - **De-worded shell** (§13–14): icon-only affordances, calm chrome, empty
>   stage by default (WidgetStage renders nothing until summoned).
> - **Calm check-in mode** (§7): glanceable burn-rate, ring status, anomaly
>   notifications — not watching dashboards.
> Net rule: **glanceable first, depth on demand.** The "high semantic
> density" line (§6) serves this only when disclosure-gated — density
> without a summary-first gate is the violation, not the goal. §3 and §5
> below are re-judged against this rule; the density gaps are D1–D6.

## 1. Wiring map (verified 2026-10-20)

**Frontend shape.** Single-page shell (`platform/console/src/app/page.tsx`):
one same-origin route (`/api/config` → backend URL resolution) + client-side
state navigation (`surface` / `arenaView` / `proofsTab` / `meshTab`).
13 surfaces, one API client (`lib/api.ts`: `apiFetch`, bearer-or-cookie,
uniform silent-empty error convention). ~80 Go endpoints exist
(`server.go:413-555` route table); the console calls a working subset
(sessions/chat/loops/jobs/events/ledger/fleet/radar/epistemics/llm/hf/
agents/mesh/tenants/mandates/auth/codesign-tq+frontier, …). Full per-route
tables were produced during this audit and are summarized here by what they
imply — the durable record is the gap register below, not a route dump
(prose rots; recount with `rg 'HandleFunc\(' platform/core/pkg/api/server.go`).

**Backend shape.** Go `kubonad` (:8080) is the sole served API (compose:
`bridge:50051` gRPC + `kubonad --bridge bridge:50051` + console). Reads are
public behind limiters; writes take edge auth (session cookie first, bearer
second) — the reader-vs-writer doctrine holds with deliberate carve-outs.
Python `platform/kubona/console_api.py` (Starlette) is **unserved**: K5
retired its duplicate routes 2026-10-12; what remains (health, approvals,
providers, reports, workflows, agents, labs, models, settings, `science/*`,
`codesign/evaluate|frontier|budget`) has no Go equivalent and no listener
— a tested but unreachable API (see G2).

## 2. Gap register (ordered by severity)

### G1 — HIGH (security): `GET /api/llm/settings` echoes live API keys to any network reader
- `handleLLMSettings` GET returns `GetSnapshot()` verbatim (`server.go:3258`);
  `GetSnapshot` copies `Providers` map untouched (`config.go:419-443`);
  `ProviderConfig.APIKey` serializes as `api_key,omitempty` (`config.go:80`)
  — empty keys omitted, **set keys disclosed**. The GET path is public (only
  PUT/POST takes `checkAuth`, `server.go:3263`).
- The console was *designed* for no-echo ("key rotate-never-echo", untouched
  providers send `""`, "backend keeps stored" — `LLMSettings.tsx:82,145-150`),
  so the UI neither needs nor expects keys back: pure disclosure, zero
  function. Any host reaching :8080 (compose binds `0.0.0.0`) reads
  NVIDIA/Anthropic/OpenAI keys. Strata (usually keyless) is unaffected —
  which is exactly why this survived the Strata migration unnoticed.
- Fix: redact `APIKey` in the GET serializer (server-side only, never over
  the wire); keep the merge-on-update path. Test both directions
  (redacted-read + preserved-update). Phase 0 of the plan.

### G2 — Python `console_api.py` is a tested, unreachable second API
- Unique routes with no Go equivalent: `science/*` (math families/anchors,
  triage, krylov, shadows, manuscripts, memory, tribunal), `providers`,
  `approvals`, `workflows`, `agents`, `labs`, `models`, `settings`,
  `codesign/evaluate|frontier|budget` (`console_api.py:759-781`). Nothing
  serves this module (no uvicorn target in `platform/docker/`, bridge, or Go;
  compose serves bridge + kubonad + console only). Auth model differs too
  (`_authed`, `console_api.py:229` vs Go edge auth).
- Meanwhile the cockpit comment still points at "Python
  `/api/codesign/budget`, pending Go migration via bridge RPC"
  (`CodesignCockpit.tsx:135,191`) — a migration with no owner and no route.
- Decision required (Phase 1): serve the unique science routes behind the Go
  proxy *or* migrate them to Go via bridge RPC, then delete the duplicates
  (single-surface rule, `console_api.py:11`). Until then the science surfaces
  that depend on them (ResultsStudio manuscript/literature paths, tribunal
  deliberate) are wired to Go equivalents or local-only fallbacks — verified
  working, but the dead module invites resurrection-by-mistake.

### G3 — Factory diagnostics (slice 62) have no HTTP path at all
- `factories.py` + `evaluate(factory=…)` are in-process Python only.
  The (unserved) `codesign_evaluate` route predates them and takes no factory
  params (`console_api.py:221-255`); Go has no proxy; the console's budget
  card is local arithmetic explicitly labeled estimate
  (`CodesignCockpit.tsx:132-154`). Net: the A5-closing model is invisible to
  the product surface it was built to unblock (TQ analysis + console).
- Fix (Phase 2): expose `factory_report` through the codesign evaluate path
  (Go proxy → bridge RPC → `evaluate(factory, code_distance, cycle_us)`;
  required-params rule preserved end-to-end: partial spec refuses at every
  layer). Read-only, no spend, same threat model as G-tq reads.

### G4 — Verified RS decoder results are invisible in the product
- The program's headline artifact (3,928,980T full-radius decoder, pins for
  walk/EEA/Chien/scan, convergence + flag proofs) lives in Python test logs
  only. No endpoint serves verified (T, Q, depth, pins); the cockpit's
  reference points are hand-typed (`1348/115` HHL, `1583/139` Gaussian).
- Fix (Phase 2): a read-only **verified-results card** (T/Q per component,
  pin status, proof refs, "Anthropic-claimed until rerun" labeling) served
  from versioned JSON generated at slice-landing time — never hand-typed,
  never a verdict (Go serves numbers, Python owns validity — same split as
  G-tq). This is the single highest-leverage UI addition: it turns the
  blueprint's core result into something a reviewer can see in 60 seconds
  ([[ui-ux-vision]] §12 P2 gate).
> **✅ LANDED 2026-10-20.** `gen_codesign_verified.py` measures every
> figure live from the builders (emitter-speed; proofs cited, not re-run)
> and writes `pkg/codesign/verified.json` (v1: 3,928,980T × 112,168Q, 5
> pinned components, 4 proof refs, non-summation note). Served verbatim by
> `GET /api/codesign/verified` (read-only, shape-checked); console headline
> card renders total + components + labels + collapsible proofs, with a
> refusal state that invents nothing. Side find, corrected same day:
> 112,168 qubits (not the stale 110,132 prose figure) — addressable and
> peak-live agree; the generator asserts it on every run.

### G5 — Strata is the default provider and the console has never heard of it
- Zero `strata` references in `platform/console/src` (grep-verified). Model
  prefetch covers `[active_provider, 'omlx']` (`LLMSettings.tsx:92`) — works
  for strata by accident of `active`, with no local-engine affordance: no
  health/port check (vision §5.6 radar lists 11434/8787/1234/8080, not 47831),
  no serving-panel URL display, no "local = free/unmetered" spend cue.
- Fix (Phase 3): Strata provider card (base URL, model, liveness, local badge);
  add 47831 to the discovery radar; surface the local-bypass spend rule
  ("local inference $0, unmetered") where costs render.

### G6 — Cockpit shows scoring only; ux-plan slices 2–4 still queued
- Landed: TQ scorer, frontier batch, κ-sweep, local budget math, DQI widget,
  packs scaffold, families note (`CodesignCockpit.tsx`, 228 lines). Queued per
  [[circuit-codesign-ux-plan]] slices 2–4: evaluator-runs view (per-trial
  refs, ε-bounds, union-bound table), families view (tracks, elites,
  widen-nudge, lineage), packs view (registry with anchor/evidence reqs).
- Fix (Phase 2, in ux-plan order, now unblocked): slice 2 absorbs G3
  (factory wall-time/spacetime card next to the ε-bound); slice 4 absorbs G4
  (packs carry their verified pins). No new dock icon until sign-off
  (ux-plan slice-2 decision stands).

### G7 — Console has no codesign types; one placeholder looks like a digest
- `types/index.ts` has no `Codesign/TQ/Frontier/Kappa` types; the cockpit
  uses local `FrontierEntry` + `any`-shaped responses (`CodesignCockpit.tsx:11-15,69,103`).
  Contract drift between Go/Python/TS is currently caught by golden tests, not
  types. Separately, `GenerativeUIStudio` falls back to
  `'sha256:lean4-proof-ast-70a69dd'` (`Studio.tsx:116`) — unlike every other
  fallback on the page, a hash-shaped string reads as a real digest.
- Fix (Phase 4): typed codesign contracts shared from the Go/Python goldens;
  re-label or remove the hash-shaped placeholder (provenance-everywhere,
  [[ui-ux-vision]] §3.6, cuts against look-alike receipts).

### G8 — Sweep bypasses the κ validation the other two actions enforce (minor)
- `scoreTQ`/`scoreFrontier` refuse non-1..3 κ without fetching
  (`CodesignCockpit.tsx:53-57,78-82`); `scoreSweep` fires κ=1,2,3 regardless
  of the κ input (`Cockpit.tsx:110-130`). Harmless today (fixed 1..3 by
  construction) but the rail is inconsistent: a typo'd κ gets refused in two
  cards and silently ignored in the third.
- Fix (Phase 4): validate once, share the rail.

### G9 — Dead code with blast radius on the next refactor
- `ObservatoryCockpit.tsx` (full alternate shell, zero importers),
  `useTelemetry` SSE hook (staged `enabled=false`, normative transport is
  3.5s polling), widgets with zero fetches, 405-only
  `GET /api/agents/custom/:id`. None hurt today; all will confuse the Phase-2
  author.
- Fix (Phase 4): delete or document; enable-or-remove the SSE hook (polling
  vs stream is a live inconsistency for loop-watching UX).

### G10 — No spend-visibility endpoint (console cannot answer "what did we spend")
- `llm_spend_estimated` totals, `KUBONA_LLM_BUDGET_USD` remaining, and the
  per-iteration loop bound live server-side only (visible as turn caps in
  `capabilities.budget`, `server.go:703-717`). The mesh router shows
  per-call estimates; nothing shows cumulative/remaining.
- Fix (Phase 3): read-only spend summary (totals, remaining, by-provider)
  behind the existing auth rail; AutonomyCockpit budget slider reads it
  instead of session-local state. Directly serves the P2/P3 acceptance gates
  ([[ui-ux-vision]] §12: reviewer/operator glances, not dashboards).

### G11 — Backend validation rails are inconsistent (hygiene, not breakage)
- `decodeStrict` (unknown-field refusal) only on spend paths; `session_id`
  requiredness, `limit` clamps, and error shapes (`http.Error` vs typed
  `WriteAPIError` vs 200-`{valid:false}` envelopes) vary rail-to-rail
  (full table in the audit working notes). Nothing is exploitable per the
  threat model, but every new endpoint copies a different convention.
- Fix (Phase 4): adopt one rail for new codesign/factory/spend endpoints
  (strict-decode + typed errors + documented limits); backfill opportunistically,
  never as a flag day.

### G12 — Codesign is buried (navigation, behind existing sign-off)
- 4th `proofs` tab (`manuscripts/tribunal/gallery/circuits`, `page.tsx:648-679`),
  no dock icon, unknown ⌘K coverage — the ux-plan slice-2 deferral, still
  reasonable for a scoring-only tool. Once G4 lands (verified-results card),
  the burial stops being proportionate: the decoder is the program's headline
  result and deserves the same one-glance reachability as fleet/tribunal.
- Fix (Phase 5): dock/palette placement behind operator sign-off (existing
  decision, not revisited here), with progressive disclosure per vision §3.3
  (summary stats default, drill-down on demand).

### G13 — Partial provider PUT silently clobbers non-sent fields (found 2026-10-20 during Phase 0)
- `Settings.Update` merges only `APIKey` ("empty keeps stored",
  `config.go:505-513`); every other field is full-struct-replaced per
  incoming entry. A partial `PUT {"providers": {"nvidia": {"api_key": "x"}}}`
  zeroes `Enabled`/`BaseURL`/model — the provider silently disables
  (fail-visible at resolve time, which names the disabled provider, but the
  save itself reports success). Only bearer-authenticated operators can PUT;
  no real client sends partials (the console always spreads full objects) —
  so this is a hardening gap, not a live break. Fix direction: presence-aware
  merge or full-object-or-refuse; must preserve the `{enabled: X}` toggle
  flow. Tracked for Phase 4, not Phase 0 (surgical redaction stays surgical).

## 3. UI/UX assessment vs the vision

Against [[ui-ux-vision]] §3 (10 principles) + §12 (gates), judged by the §0
glanceable-first rule:
- **Held:** read/approve-only (no submission surface anywhere; cockpit header
  rule enforced + tested); fail-closed refusal chrome (`refused (HTTP …)`,
  κ-refusal, UNVERIFIED placeholders, never invented numbers); calm
  *transport* (polling, silent-empty degradation, three-state status).
- **Partially held:** confidence-as-visual (ε-bounds render as numbers, not
  states); provenance-everywhere (ledger links exist for runs, but cockpit
  numbers have no click-through — G4 fixes the highest-value instance);
  recovery-as-guidance (refusals name the rule but offer no next step).
- **Failed — density & attention (new D-gaps, verified 2026-10-20):**
  - **D1 — Everything renders expanded; one `<details>` in the whole app.**
    Tribunal turns default expanded (`CopilotDrawer.tsx:1171`
    `expandedTurns[turnKey] !== false; // Default expanded`); all cockpit
    cards, all fleet tiers, all mesh panels render simultaneously. The only
    disclosure primitive app-wide is a single `<details>` in MetasearchPanel
    (`MetasearchPanel.tsx:135`). LabsView accordions (`LabsView.tsx:82-136`)
    are the exception that proves the pattern. Progressive disclosure (§3.3)
    is currently a wiki line, not a UI behavior.
  - **D2 — No summary-first mode on any surface.** No card shows a one-line
    verdict with drill-down; the reviewer gets the full firehose or nothing.
    The P2 60-second gate ([[ui-ux-vision]] §12) is unpassable as built: no
    surface can answer "what is happening and what needs me" in one glance.
  - **D3 — No calm check-in mode.** Vision §7 specifies it (glanceable
    burn-rate, ring status, anomaly notifications); nothing implements it.
    The HUD spend pill + `OVER CAP` flag (`ArenaHud.tsx:88-107`) is the only
    glanceable element, and it has no burn-rate, no ring status, no anomaly
    feed. Mobile check-in is doubly absent (no mobile layout either).
  - **D4 — Unaggregated alerting.** Page-level polling fires a toast per
    fresh ledger event (3.5s cadence, no priority/mute/aggregate —
    `page.tsx:165-193`); concurrent loops shout concurrently. This is the
    notification-fatigue failure the vision's open problem #4 names.
  - **D5 — Icon-only dock without persistent labels.** The 64px dock is
    correctly de-worded per §13, but meaning lives in hover tooltips only
    (`SovereignDock.tsx:37-44`) — touch/keyboard-first users get nothing,
    and `Activity`/`Network`/`Orbit` are interchangeable waveforms. De-worded
    chrome still needs a first-run gloss + `:focus-visible` tooltips.
  - **D6 — No density budget.** New cards (factory diagnostics, verified
    results, spend summary — G3/G4/G10) added to current surfaces will make
    the firehose worse. Every Phase-2 addition must *replace* a visible
    element with a summary-first gate, not append another card.
- **New-feature fit:** Strata (G5), factories (G3), RS decoder (G4), and spend
  (G10) are the "never before seen" capabilities of this audit cycle; none
  has a surface. The plan below wires each to the principle it serves —
  under the D6 budget.

## 4. Implementation plan (sequenced; each phase lands green or not at all)

**Phase 0 — Secret disclosure (now, before any feature work).**
Redact `APIKey` in the `GET /api/llm/settings` serializer (server-side
only); keep merge-on-update. Tests: redacted-read, preserved-update,
empty-stays-empty, PUT-still-gated. Threat note: reads stay public behind
limiters — redaction is what makes that posture survivable. Acceptance:
`curl GET` shows no `api_key` with keys configured; all existing
`llm_test`/`tools_test` green.
> **✅ LANDED 2026-10-20 (`c4ac28d`).** `GetPublicSnapshot()` serves both
> GET and the PUT response envelope (single `snapshot(redact)` builder —
> also fixes the lock-copy vet shape the naive version had); console
> `enabled`-flag display preserved (tsc clean); HTTP tests for redaction /
> full-object rotation / console-shape save (merge intact); adjacent hole
> G13 recorded, not smuggled into the fix.

**Phase 1 — One served API (decision + migration).**
Decide: serve unique `console_api.py` science routes behind the Go proxy
or migrate to Go via bridge RPC (ux-plan slice 5). Either way: single-surface
rule enforced (delete the loser), `_authed` vs edge-auth reconciled per
route, cockpit budget comment updated to name the real route. Acceptance:
every console-called path served by exactly one backend; dead module gone or
documented; `codesign/budget` resolves to a real endpoint or the comment is
removed. Needs operator sign-off (contract change — existing rule, kept).
> **✅ DECIDED 2026-10-20: migrate stateless to Go.** Serving as-is rejected
> (second bearer token, parallel stores — K5 split-brain returns). Landed
> slice 1: `POST /api/codesign/evaluate|evaluate-batch|budget` via
> `runBridgePython` (Python stays single validator/implementation;
> num_tests HTTP-capped 4096; 120s ctx; public reads like existing
> codesign rails); console budget card rewired to the sealed bound
> (local math deleted); `evaluateCodesignArtifact` helper landed for the
> Phase-2 runs view. Go 28/28, console 170/170 green. Stateful duplicates
> stay retired; `science/*` migration follows the same proxy pattern.

**Phase 2 — Cockpit grows up (ux-plan slices 2–4, updated).**
2a. Evaluator-runs view + factory card (G3, G6-slice-2): per-trial refs,
ε-bound display, union-bound table served (not local math), wall-time /
spacetime / active-volume diagnostics from `factory_report` with source
labels (GF2019/Litinski, literature-derived, never paper-measured).
2b. Families view (G6-slice-3): tracks, elite-sharing events, widen-nudge
(human nudge, never auto-prompt), lineage graph.
2c. Verified-results card (G4) + packs view (G6-slice-4): versioned JSON
generated at slice-landing, pins + proof refs + claimed-labeling; packs
carry anchors/evidence reqs. Ranking stays TQ; factory numbers stay
diagnostics (existing POLICY, both sides).
Acceptance per view: renders live data end-to-end from the new endpoints,
refusal paths tested, no hand-typed numbers, no verdicts from Go.

**Phase 2.5 — Calm-first density pass (new; gates Phase-2 additions).**
Enforce the §0 glanceable-first rule before new cards land (D-budget):
D1: disclosure primitives per surface (collapse-all by default on tribunal
turns, cockpit cards, fleet tiers — LabsView accordion is the pattern);
D2: one-line summary-first header on every card with drill-down
(P2-gate test: reviewer reaches "what needs me" in 60s untrained);
D3: calm check-in strip (burn-rate, ring status, anomaly count — the HUD pill
grows into the vision §7 mode, desktop first); D4: toast aggregation
("3 loop events in 30s — view recorder"), visibility-gating, backoff;
D5: persistent micro-labels at `md+` + `:focus-visible` tooltips + first-run
gloss for the dock; D6: density budget — each Phase-2 card *replaces* a
visible element with a summary gate, never appends. Acceptance: P2-gate
walkthrough per surface (untrained reviewer, 60s, watch/understand/
approve/interrupt); no surface renders more than its summary set by default.
> **✅ LANDED 2026-10-20 (2.5a + 2.5b).** Tribunal turns default-collapsed
> with expand/collapse-all (`CopilotDrawer`); cockpit informational cards
> (families/DQI/packs) are native `<details>` collapsed by default —
> scoring cards stay open (the tool's purpose); page poll aggregates
> toasts ("N new events — view recorder"), skips fetch while hidden,
> backs off to 15s at rest (clock untouched). Deferred: SwarmUniverse
> hover-cards → tap-to-open rebuild (needs state-driven visibility, not
> a label); fleet tier collapse (tiers ARE the surface's purpose —
> D1 narrowed on evidence).

**Phase 3 — LLM + spend visibility.**
Strata provider card + 47831 radar + local=$0 cues (G5); read-only spend
summary feeding AutonomyCockpit + mesh router (G10). Acceptance: operator
can answer "what provider, at what cost, with what remaining" from one
surface (vision §12 P3 shape).

**Phase 4 — Contract hygiene.**
Shared TS codesign types from the goldens; κ-rail parity (G8); hash-shaped
placeholder fixed (G7); SSE enable-or-remove + dead-code sweep (G9);
new-endpoint rail convention (G11); presence-aware provider merge (G13). Acceptance: `tsc` + vitest + lint clean;
no behavior change except the fixed items.
> **✅ LANDED 2026-10-20 (4a + 4b, partial).** Shared `Codesign*` types in
> `types/index.ts` (cockpit + api import, no local duplicates); sweep card
> documents κ-field independence (behavior correct as-built — sweep
> covers all three by construction; refusing it on a typo'd κ would be
> the worse UX); hash-shaped `sha256:lean4-proof-ast-…` fallback deleted
> (honest empty state instead). Dialogs: both custom overlays get
> role/aria-modal/labelledby/Esc/trap/return-focus via shared
> `useFocusTrap` (no DOM test harness exists — node env, no testing-library;
> tsc-verified). Labels batch: all icon-only buttons, placeholder-only
> inputs, and range sliders labeled; div-click selections converted
> (turn headers→real buttons, DAG/orb/anchors/rows/steps keyboardable;
> orb display-only, separate toggle button rules); NewCampaign mode cards
> radio-behaved. Vitest 38 files/170 green. Remainder: SwarmUniverse
> tap-to-open rebuild, SSE enable-or-remove, dead-code sweep, G13 merge,
> new-rail convention — queued, not smuggled into this slice.

**Phase 5 — Navigation (behind existing sign-off).**
Dock/palette placement for circuits once G4 lands (G12); progressive
disclosure pass (summary-first, drill-down) across the cockpit. Acceptance:
P2-gate shaped — a reviewer reaches the verified decoder result in under 60
seconds untrained.

**Explicitly not in this plan:** job submission from the console (hard rule);
Go-side builders / Python duplication (validity stays Python-owned); LLM-in
the-loop proving; any endpoint that mints spend from a read surface.

See [[circuit-codesign-ux-plan]] (lands here as slices 2–5 detail),
[[ui-ux-vision]], [[agent-ux-control-plane]], [[circuit-codesign-production-readiness]].
