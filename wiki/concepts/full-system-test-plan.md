---
type: concept
name: Full-System Test Plan (diagram-complete)
date: 2026-10-21
updated: 2026-10-21
tags: [testing, audit, plan, coverage, scenarios]
sources: [source-gap-analysis-plan, source-architecture-paper, source-llm-circuit-codesign-2026]
status: candidate
---

# Full-System Test Plan — every node on [[platform-architecture-diagram]]

> **Status: PROPOSED + ROUND 1 EXECUTED 2026-10-26 (see [[log]] "full-system
> test round 1").** The batteries below ran once against the Phase-1 tree;
> findings and the Phase-2 queue live in the log entry. Phase 2 remains
> unapproved — do not execute it. The plan is organized by
> diagram node so coverage is checkable: every subgraph on the current
> [[platform-architecture-diagram]] snapshot has a battery, ordered
> basic → complex. Prior verified results are cited as the floor, never
> re-proven on vibes.
>
> **Scenario method (LLM-assisted, human-verified).** Strata (local,
> `iq3_s`) generates scenario *variants* per battery within fixed
> contracts (shapes, budgets, refusal rules from the code). Strata never
> judges outcomes: every assertion is mechanical (status code, ledger
> row, pixel/screenshot, file hash). Deterministic seeds where the rail
> supports them; every run records command + output + evidence.
>
> **Evidence standard.** Each check records: exact command / click path,
> expected vs observed, and `file:line` or output excerpt. A finding
> without a repro is not a finding. Small/safe fixes land only after
> approval, each with a regression pin proven to fail pre-fix.

## 0. Entry criteria (before battery 1 runs)

- [ ] Fresh compose stack from a clean checkout (`up --build -d`), token
  from a local env file (never committed), all three health gates green.
- [ ] Isolated local rail (`kubonad --db /tmp/… --bridge 127.0.0.1:50051`,
  host bridge up, `STRATA_MODEL=iq3_s`) for Python-backed + LLM rails.
- [ ] Baselines recorded: `go test ./pkg/...`, `vitest run`, `tsc`,
  `pytest -q` (Python tree untouched since 1011-passed baseline — rerun
  only if the tree changed), `wiki-lint --strict`.
- [ ] Playwright/Chromium harness in `/tmp` (fresh profile per run).
- [ ] Diagram snapshot hash under test recorded (traceability).

## 1. HUMAN — console (13 surfaces, every control)

Per surface: first paint (0 console errors, 0 failed requests), every
button/input/slider/select/link clicked with outcome recorded, empty vs
error vs loading states distinguished, then the surface-specific battery:

- **Arena + tabs (Orbital/Proof DAG/Hardware QPU):** probe cards open,
  recenter, tabs switch; Proof DAG node-click traces record; QPU device
  "Deep Physics & Pricing" expands.
- **Flight Recorder:** playback play/reset, scrubber drag, step pills,
  Export session / Ledger JSON+Markdown / Import session round-trip,
  Preview Next Step, Inspect Circuit Code Mutation expand.
- **Manifold:** deep-math disclosure, Tripartite Wavefunction expand,
  gauge states (verified vs template).
- **Fleet Radar:** tier expand, device cards, port states vs `/api/radar`.
- **Proofs tabs (Manuscripts/Tribunal/Preview Gallery/Circuit Codesign):**
  manuscript/LaTeX/Lean tabs render; tribunal convene without LLM (honest
  template path) and with Strata (4/4 authored, consensus closes);
  gallery items; **Codesign cockpit: all six actions** (Score, Score
  frontier, Run evaluation, Run families, Sweep κ, Seal) on the venv
  rail — valid + invalid + refusal states.
- **Mesh tabs (Cognitive/Guardrails/ModelHub/Laboratories/Settings):**
  Scan Ports, Air-Gap toggle both directions (verify egress claim),
  Evaluate Route with/without model, Guardrails rules render, ModelHub
  curated categories + search (authed via console token in test env) +
  download tiny file + cancel, Laboratories tenant rows expand +
  ceiling/refund flows (admin envelope), Settings: test-model per role,
  save with/without token (honest refusal), revert.
- **Copilot drawer:** send (Strata), search/export/regenerate/clear,
  attach file, session pills switch, SESSION MEMORY +, tribunal review
  pills (Launchit/Inspect/Tribunal review), mandate card display,
  command palette (⌘K) coverage, close shortcworks.
- **Console-token positive paths** (P0 — refusals alone prove nothing
  about the privileged flows): with `NEXT_PUBLIC_KUBONA_CONSOLE_TOKEN`
  set in the test env, HF search returns models, download starts +
  cancels cleanly, custom agent registers and appears in the roster,
  settings save persists and survives a reload, mandate card flow
  completes. Every one of these is refusal-tested tokenless already;
  this battery proves they *work* when authorized.
- **Cross-surface:** kill kubonad mid-session (banner), kill bridge
  mid-run (fallback + recovery), expired session (logged-out read),
  garbage into every form (inline named errors), 390px pass per
  surface, keyboard-only pass of the five highest-traffic flows.

## 2. EDGE — kubonad :8080 (all 120 routes, valid-path + refusal; count generated in `docs/repo-counts.md`)

- Route census recount; every route gets happy-path + one refusal case.
- Auth matrix: bearerless/wrong-token/admin-without-envelope per gated
  rail; session-cookie login flow with generated Ed25519 keys
  (challenge → login → whoami → revoke).
- Contract checks: strict vs lenient decode inventory, oversize bodies
  (frontier 64/batch 64/families/packs 64/budget 32), wrong methods,
  idempotency replay (same key → cached; mutated body → 409) on all
  four spend mutations, rate-limit behavior (human-paced buckets),
  CORS allowlist (good vs foreign origin), SSE stream connect + events.
- `events/stream` + `canvas/layout` save/load round-trip.
- A2A protocol cycle (own battery — the only external JSON-RPC
  surface): card, unknown method → `-32601`, unknown task → `-32001`,
  submit without payment → HTTP 402 + challenge, paid submit refused
  without a verifier, delegation + `requireDispatchAutonomy` on dispatch.
- TLS posture with a self-signed cert (refuse without material,
  serve with material).

## 3. COG — tribunal, mesh, agents, replan, metasearch

- Tribunal battery with Strata: H2/LiH/BeH2 prompts, all-authored
  consensus + obligation close (proven once — re-prove on the
  post-change tree), template fallback when LLM absent, tool-failure
  contest path, scribe tag contract (`seen/inferred/unknown` +
  proposal block presence recorded, fallback covers absence).
  Turn-bundle resume is Go-level only (no HTTP route exists — verify
  via unit path and record the missing surface as a gap).
- Mesh route matrix: bare/qualified/unknown models × funded/unfunded ×
  air-gap on/off; custom-agent create/delete with and without token
  (honest in-modal errors); triage endpoint.
- Metasearch full cycle (propose → approve/reject → apply) with admin
  envelope; grants register/revoke; mandates sign (generated keys) →
  verify; campaign synthesize + replan folds (measured_krylov,
  anchor recompute, claim verdict).
- Autoresearch: start/step/pause/resume/stop/sleep/wake/drain per
  harness; loop kill-switch; idempotent replay.

## 4. EXEC — jobs, loops, QIR, sandbox, Rust

- Jobs: submit (valid + invalid QASM), get/list/`{id}/cancel`,
  COMPLETED/FAILED folds, `jobs:[]` shape.
- Loops: full lifecycle per action verb; restart-orphan case
  (sleeping DB rows after daemon restart — reported, needs design).
- **Restart persistence (P0):** daemon restart mid-flight, then verify
  reconcile behavior — in-flight loops sleep (not vanish), jobs keep
  terminal state, sessions + campaigns + reservations survive, and the
  previously-observed orphan rows are accounted for (killable or
  documented, never silently stuck *and* undeletable).
- QIR: compile valid/invalid/empty; qir-core CLI vectors
  (`--json`, BaseProfile) via `KUBONA_QIR_CORE`.
- Sandbox: timeout/refusal paths; quantum-runner image contract
  (ro, net-none, unprivileged) present, not breached.

## 5. TRUST — ledger, tenants, cost, auth, disk, contracts

- Ledger append allowlist (tribunal_commit/peer_review/annotation) +
  forgery refusal; export both formats parse; `verify --db` flow.
- Tenants: open provisioning, admin-key bootstrap (first wins),
  ceiling/refund/autonomy/revoke with valid + tampered envelopes.
- Cost: spend summary math vs metered calls; cap enforcement;
  reservation acquire/release on job terminal states.
- Auth: login/logout/whoami/sessions/revoke; stale-cookie fallback
  to bearer; limiter behavior under spray.
- Disk guard via env thresholds (WARN/BLOCK/RESUME small values):
  BLOCK 503s new work with Retry-After, reads stay; no real disk fill.
- Contracts conformance: error-catalog coverage, event-types drift
  check, obligation rules, mcp-catalog digest.
- **Cross-tenant isolation matrix (P0):** lab A session attempting
  lab B rows across sessions/jobs/loops/memory/epistemics/campaigns —
  every read refuses with the tenancy error named, nothing leaks, and
  the ledger attributes the attempt. Tenant scoping per-store is
  implemented; this battery proves the seams hold adversarially.

## 6. RUST + MCP + MEM + BRIDGE + BACKENDS + MODELS + SECRETS + CODESIGN

- Rust: `cargo test` both crates; qir-core CLI golden vectors.
- MCP: gateway boot via `build_gateway`, all five servers' tools
  callable through it (auth on/off), catalog digest match.
- Memory: retain/recall round-trip per bank; tenant scoping;
  pgvector dormant-contract test (no live PG).
- Bridge (all 7 RPCs, enumerated: `ExecuteCircuit`, `CutCircuit`,
  `ComputeClassicalAnchor`, `EvaluateWavefunction`, `GroundLiterature`,
  `ExecuteQuantumKrylov`, `MitigateCircuit` — `proto/kubona.proto:187-193`):
  execute valid/invalid/oversize-RAM, cut (incl. kappa-cap + multi-q
  refusals), anchor per molecule + basis allowlist + variational guard,
  krylov H2 + non-H2 refusal corridor, mitigate ZNE/PEC/CDR +
  unknown-method/device refusals, wavefunction verdict matrix
  (advantage-claimed/verified truth table), literature live +
  offline-empty.
- Backends: simulator tiers live; QPU paths prove fail-closed
  without credentials (no spend, no silent sim).
- Models: role resolution per provider, spend silence for local,
  usage-omitted Strata no-refusal (pinned), caps on tribunal turns,
  session-id in wired prompts (pinned), mesh qualification (pinned).
- Secrets: vault leases with test HMAC key (issue/use/revoke),
  absence without key (503 naming the fix).
- Codesign (venv rail): TQ golden, evaluate valid/invalid/factory,
  frontier best-pick, batch, budget union bound, families
  cross-pollination + widen-nudge, packs registry, verified JSON
  regen reproducibility, per-trial recipe determinism.

## 7. Exit criteria

- Every battery row PASS/FAIL/SKIP-with-reason; zero P0/P1 open;
  every refusal path demonstrated; suites green
  (`go test`, `vitest`, `tsc`, `scripts/ci.sh` modulo hardware keys,
  Python suite if tree changed).
- Findings register with severity + evidence; fixes proposed,
  none applied without a second approval.
- Wiki updated (diagram refresh if topology changed) + lint clean;
  teardown (`down -v`) only after the report is accepted.

## Explicitly out of scope (with reason)

- Real QPU execution (no credentials; fail-closed paths covered instead).
- Paid endpoints / x402 live rail (dormant by design, Phase 5+).
- Third-party security review (human gate, P3).
- Full-loop load/chaos and real disk-fill (destructive; env-threshold
  simulation covers the logic).
- Public-repo / outside-contributor flows (private by decision, Q-25).

## Phase 2 — queued, not approved (do not execute yet)

Deep-dive findings parked here so nothing is lost. Each becomes a
battery only on a second approval:

- **Export-control routing:** declared vs undeclared caller
  jurisdictions → ELIGIBLE / INELIGIBLE / refused-before-simulation
  (§2.9, currently zero live coverage).
- **x402 fail-closed:** settlement refuses without a real verifier
  (the design) — distinct from skipping the dormant rail.
- **Tenant suspend/reactivate cycle** (Q-33: spend freeze, admin keeps
  keys) — ceiling/refund covered, suspend not.
- **`kubonad grants` CLI** (mint/attenuate/verify) — the operator trust
  tool has no battery.
- **Boundary caps:** 64KB ledger append, 256KB canvas, 40-line
  notebook trim, 50-version campaign cap, per-rail oversize bodies.
- **Tamper-evident ledger:** mutate a row on a scratch copy, watch
  `verify --db` fail (the actual point of the chain).
- **Copilot tool-call rounds** (`resolveWithTools` + `tool_called`
  attribution) — chat sends never exercise a tool-invoking turn.
- **Env-var audit:** `.env.example` vs compose vs code defaults
  consistency table (the Strata lesson, mechanized).
- **Codesign negatives:** `spec_table` truth-table variant, factory
  partial-spec refusal at every layer, packs-promotion refusal
  without ledger refs.
- **`go test -race`** on `api/supervisor/ledger`; auth/a2a challenge
  expiry paths; secret rotation drill (procedure-level).

Related: [[platform-architecture-diagram]] · [[audit-topology-map]] ·
[[frontend-backend-ui-ux-audit]] · [[local-simulation-test-scenarios]] ·
[[circuit-codesign-ux-plan]] · [[implementation-gaps]]
