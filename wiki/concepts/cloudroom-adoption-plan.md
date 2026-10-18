---
type: concept
name: Cloudroom adoption plan
date: 2026-10-17
updated: 2026-10-17
tags: [plan, cloudroom, prior-art, control, ecosystem, production]
sources: [source-frontier-repo-audit-2026]
status: candidate
---

# Cloudroom adoption plan — production-grade (S1–S8 landed 2026-10-17)

> Implements selected patterns from [[cloudroom]] as independent Kubona code
> (reference topology — no submodule, no vendored code). Production stance:
> every slice below specifies its **full logic** — concurrency, restart,
> validation, retention, and refusal paths included — so nothing lands as a
> stub that must be redone. The `Not in slice` lines name sequenced follow-ups,
> never hidden gaps. All slices S1–S8 landed 2026-10-17 (see `> ✅ LANDED`
> blocks per slice below).

## 0. Decided values (from the 2026-10-17 deep-dive; evidence in §5)

- **Q1 scope:** S1 covers the 4 session-scoped spend mutations (jobs, launch,
  tribunal, autoresearch-start); metasearch-apply gets a proposal
  applied-state guard instead of a key (sessionless `"global"` scope would
  weaken per-session uniqueness). Plus: launch must accept
  `mandate_ref`/`grant_chain`/`caller_jurisdictions` like jobs — today it
  drops them, so hardware launch is dead on arrival (gap G3).
- **Q2 thresholds:** defaults `WARN 2 GB / BLOCK 1 GB / RESUME 1.5 GB`
  (env-overridable, `0 < block < resume < warn` enforced at startup).
  Monitor the resolved set `{state_root, ledger-parent override, db-parent
  override, HF models dir}`; BLOCK if **any** is below threshold.
- **Q3 timeout:** 10 min idle for autonomous loops, 30 min for chat/tribunal
  sessions, **plus** mid-run `checkLLMBudget` per loop iteration (a timeout
  alone never bounds a hot loop: worst case ≈ 25 iters × 3 min ≈ 75 min).
- **Q4 migration scope:** session-content only = the 8 session-scoped tables
  (sessions, chat_messages, jobs, loops, ledger_events, campaign_states,
  campaign_versions, claim_records, canvas_layouts — §5B), with orphan sweep
  (gap G6) and manifest `version: 1` so a future workspace-files v2 cannot
  silently misread v1.
- **Q5 caller binding:** portable session + tenant + lease-scope binding now;
  AF_UNIX/`SO_PEERCRED` as a Linux-only hardening follow-up (no socket
  infra exists; Darwin dev + TCP edge make peercred a second slice, with a
  `caller_binding` field reserved in the lease record so the schema does not
  change later).
- **Q6 order:** S1 → S2 → S3 (money-correctness, then availability, then
  efficiency), with S2 cleared to jump first if a ≤25 GB VPS ships before S1
  (S2 has no ledger migration and is fully independent).

## 1. Gaps and missing functionality (found by the deep-dive)

- **G1 — No idempotency anywhere.** Zero `request_id`/`idempotency` hits in
  `platform/core/pkg/api`. Every retry mints a new UUID job/loop and a fresh
  spend reservation. Highest risk: jobs dispatch (double `ReserveSpend` +
  second, possibly QPU, job).
- **G2 — No disk guard.** Space-measurement grep over Go + Python finds only
  UI strings and `IsDockerAvailable`. State is 152 MB today (147 MB of it HF
  models) on 3.2 TB free — but HF downloads are unbounded GBs per call and
  `jobs.jsonl` (1.8 MB, 2941 lines) is append-only with no rotation.
- **G3 — Launch cannot reach hardware.** `handleChatLaunch` drops
  `mandate_ref`/`grant_chain`/`caller_jurisdictions`, so any metered-hardware
  launch fails admission. The field set must match jobs or the path is dead.
- **G4 — Loop/tribunal spend is unreserved and unbounded mid-run.**
  Autoresearch start reserves nothing; `checkLLMBudget` runs at entry only;
  worst-case loop ≈ 75 min. Costs are heuristic estimates at that.
- **G5 — Loops live in memory only.** The `loops` table has no writer
  (loops live in `activeAutoloops`); a restart loses them. Sleep state has
  nowhere to persist.
- **G6 — `DeleteSession` orphans.** Deletes only chat + campaign_states +
  sessions, orphaning jobs/loops/events/claims/versions.
- **G7 — No queue operations.** Queued work can only be aborted wholesale;
  no edit/cancel/reorder, no steer into a running turn.
- **G8 — Secrets lack caller binding and value hygiene.** No exact-set,
  single-line, or size rules; no test asserting values stay out of
  events/diagnostics; no atomic-write guarantee where env files are written.
- **G9 — No session migration.** Checkpoint resume / lab-to-lab handoff =
  raw DB copying: no validation, no single record, silent divergence.
- **G10 — Diagnostics have no redaction contract.** No fixed taxonomy; raw
  stderr has no bounded, permissioned capture; no absence-scan test.

## 2. Improvements and enhancements (beyond the Cloudroom pattern)

- **I1 — Canonical content hashing** (sorted-key JSON) shared by S1 keys and
  S5 manifest digests: one `CanonicalHash` helper, drift-proof, never two
  copies.
- **I2 — `jobs.jsonl`/`sessions.jsonl` retention** (size-based rotation a la
  the 8 MiB diagnostics pattern): sequenced after S2, designed here so S2's
  thresholds never assume bounded files.
- **I3 — Per-iteration spend reservation for loops** (full accounting):
  designed after S3 lands mid-run checks; S3 must not preclude it (loop row
  carries `reserved_amount`, same columns as jobs).
  > ✅ LANDED 2026-10-17 (`8a49012`): admission reserves worst-case
  > (per-iteration ceiling across coder/critic roles × max iters, tenant
  > + session dual books; unknown metered models fall back to
  > priciest-known, named in the loop_started event); measured usage
  > draws down per iteration (unpriced → pause); overrun pauses;
  > stop/end/sleep release via claim-once (kill racing completion
  > releases exactly once); sleep releases + wake re-reserves remainder
  > (refuses when uncovered — this reverses the W2.10 never-sleep-money
  > skip, which would have frozen sleep for every funded loop);
  > crash-safe direction documented (over-reserved, never under, refund
  > primitive recovers). Suite: llm/ledger/engine/api green.
- **I4 — `DeleteSession` fail-closed cascade**: refuse when non-terminal
  jobs/loops or unreleased reservations exist; else cascade all 8
  session-scoped tables (fixes G6 at the root; S5's export sweep covers
  already-orphaned rows).
- **I5 — Capabilities endpoint** (`/api/capabilities`: feature flags,
  thresholds, harness catalog): Cloudroom's `GET /v1/capabilities` pattern;
  lets the console degrade honestly instead of probing.
- **I6 — Strict request validation** (`deny_unknown_fields`-equivalent on
  spend mutations): typos in `mandate_ref` currently parse as absent.
  > ✅ LANDED 2026-10-17 (`66cef19`): `decodeStrict` on jobs, launch,
  > tribunal, autoresearch-start, mandates/sign, loops (reads stay
  > lenient); unknown fields 400 typed `bad_request` naming the field;
  > console sends verified compatible before landing.

## 3. Implementation plan (full logic per slice)

### S1 — Content-bound idempotency keys (M, first) — ✅ LANDED 2026-10-17

> Landed as specified, plus the metasearch finding: apply was already
> guarded by its applied-state gate (second apply 400s before
> `loop.Resume()`), so no key was added there — proven by
> `TestMetaSearchDoubleApplyRefused`. 11 new tests (replay/conflict/
> ghost/20-way-race/restart/stale-reclaim/hash-stability/ID-table),
> negative-tested (replay test fails on old handlers with a double job),
> race suites green, live matrix proven on `:8080` (201 → replay +
> header + identical body → 409 → keyless 201 back-compat).

- **Threat:** retry double-executes spend (G1). G7 stopped mis-attribution;
  this stops double-execution.
- **Schema:** `idempotency_keys(session_id, request_id, content_hash,
  status_code, receipt_json, created_at)`, PK `(session_id, request_id)`,
  in `platform/core/pkg/ledger/ledger.go` beside the other tables. Spend
  receipts are audit: **no TTL, never deleted**.
- **Logic:** `requireIdempotency(sessionID, requestID, canonicalContent)` in
  `platform/core/pkg/api`, called **after** session resolve (404 ghost
  first — never store keys for ghosts) and **before** any spend call, inside
  one transaction: `INSERT` new key → proceed; key exists + same
  `content_hash` → replay stored `(status_code, receipt_json)` byte-identical
  with header `X-Idempotent-Replay: true`; key exists + different hash →
  409 `request_conflict` naming both hashes. Concurrent duplicates serialize
  on the PK (second writer loses the insert race and replays — test it).
  `request_id` = `1–64 [A-Za-z0-9_-]`, enforced. Content = canonical hash
  (I1) over the spend-binding fields (QASM/backend/shots/budget/mandate or
  prompt/model/iterations — the same fields the signature/mandate binds, so
  a replayed receipt always matches authorized intent). Rail flag: job-rail
  (jobs, launch) replays the admission + job record; LLM-rail (tribunal,
  autoresearch-start) replays the full result object (tribunal runs
  synchronously ≈ 6 min worst case — replay must return the stored result,
  never re-run). Includes the G3 fix (launch accepts the full jobs field
  set). Metasearch-apply excluded: gets an `applied`-state guard on the
  proposal row (repeat apply returns the original receipt).
- **Files:** `pkg/ledger/ledger.go` (table + accessors),
  `pkg/api/*idempotency*.go` (helper + canonical hash I1),
  `pkg/api/server.go` (4 handlers + launch field set + metasearch guard).
- **Tests:** same/same → one job/loop + byte-identical receipt + replay
  header; same/different → 409; ghost → 404 with no key row; concurrent
  duplicate race → exactly one execution; restart → replay from disk;
  tribunal replay returns stored result without re-calling the LLM (mock
  counts calls); negative-tested (all fail on old code).
- **Live proof:** double-POST one ID → one job; mutated body → 409; daemon
  restart → replay.
- **Accept:** zero duplicate spend under retry storms (parallel ×20 same ID
  → 1 execution, 19 replays); 409 names `request_conflict`.
- **Rollback:** drop table + remove checks (receipts already sealed in
  ledger events remain valid history).
- **Wiki:** [[spend-mandates-and-delegation]] landed-note + gap G1/G3 rows.

### S2 — Disk guard with hysteresis + retention design (M) — ✅ LANDED 2026-10-17

> Landed as specified at decided values (WARN 2 GB / BLOCK 1 GB / RESUME
> 1.5 GB, resolved-mount set: DB dir + HF models dir via `hf.DefaultModelsDir`
> single source). 6 `pkg/disk` unit tests (validation, env, hysteresis
> soak, fail-closed measurement, min-wins, live statfs) + 4 API gate tests
> (all-entries 503 + Retry-After, reads stay, ready probe, runner
> defense-in-depth); negative-tested (gate removal fails); live matrix on
> `:8080` (blocked health/ready/503-body → clear 200/ok, plus invalid-env
> boot refusal). Canvas/sub-MB writes deliberately ungated (bounded).

- **Threat:** unmodelled disk-full outage (G2); bursty writers (HF GBs,
  jobs.jsonl growth, PySCF spill).
- **Logic:** poller (15 s, one shared const block:
  `warn=2GB/block=1GB/resume=1.5GB`, env-overridable,
  `0<block<resume<warn` enforced at startup or refuse to boot).
  Resolves the monitored set at boot (Q2 decision: state_root honoring
  `$KUBONA_STATE_DIR`, ledger-parent on `$KUBONA_LEDGER` override, db-parent
  on `--db` override, HF models dir) via `statfs` on each distinct
  filesystem; BLOCK if **any** is below threshold. States with hysteresis
  (warn clears only above warn; blocked clears only above resume — no
  flapping). While blocked: refuse new jobs/uploads/loops/exports with 503
  `storage_blocked` (+ `Retry-After`), freeze dispatch admission; reads,
  health, recovery, and operator `vm-run`-equivalent stay available.
  Measurement failure (unreadable mount) blocks fail-closed. Every
  transition seals a ledger event (no replay on resume — Cloudroom rule).
  Surfaced on `/api/health` (`disk: {level, bytes_available}`) and readiness.
- **Files:** new `pkg/disk/` (measurer + thresholds const + poller),
  `cmd/kubonad/main.go` (wire + startup invariant), `pkg/api/server.go`
  (health fields + 503 paths), `pkg/jobs/admission.go` (block check first).
- **Tests:** transition table incl. hysteresis boundaries; blocked admission
  503s with code; unreadable-mount blocks; multi-mount min-wins; resume
  replays nothing; negative-tested.
- **Live proof:** injected measurer (small tmpfs) → 503s with code →
  recovery → resume, history intact.
- **Accept:** thresholds exactly as decided; no flapping within 100 MB of a
  boundary under a 60-cycle soak.
- **Rollback:** disable poller (admission check compiled out by flag);
  thresholds const remains.
- **Follow-up (designed, not hidden):** I2 rotation for `jobs.jsonl` /
  `sessions.jsonl` (size-based, 8 MiB pattern).
  > ✅ LANDED 2026-10-17 (`90a82e5`): shared `kubona/retention.py`
  > (8 MiB × 3, rotate-before-append under the caller's lock, readers
  > sweep all segments oldest-first); `session.create` takes the lock
  > like every other writer (its unlocked append raced locked ones).
- **Wiki:** [[risks]] + [[implementation-gaps]] rows.

### S3 — Loop sleep + drain + mid-run budget (M, needs S1 keys for wake) — ✅ LANDED 2026-10-17

> Landed as specified with one documented reduction: chat/tribunal sessions
> do NOT sleep (they hold no harness — sleeping them would be theater;
> loops only). Loop-row writer added (`harness_id/last_activity/drain/
> reserved_amount`), boot reconcile sleeps non-terminal rows (proven live),
> budget hook pauses on breach, wake S1-keyed, drain latched. Engine (3) +
> API (2) + ledger (2) tests; negative-tested; race green; live
> sleep/step-refusal/wake/drain/drain-refusal + restart-reconcile matrix.

- **Threat:** idle loops hold harness/bridge resources forever; stop mid-turn
  loses or double-runs work; hot loops burn unbounded LLM (G4/G5).
- **Logic:** `last_activity` updated on every turn/step/event; loop row
  writer added (fixes G5: start/sleep/wake/step persist to the `loops`
  table with `reserved_amount` column reserved for I3). Idle 10 min
  (loops) / 30 min (chat/tribunal) → release harness, state `sleeping`,
  history kept; sleep only when no turn is running and queue is empty;
  next prompt wakes the **same** session (S1 key on the wake op — a retried
  wake never forks a second loop); `drain` flag refuses new work while
  stopping; restart resumes only queued/recent loops (never refills memory).
  **Mid-run `checkLLMBudget` every iteration** (the timeout bounds
  waste-at-rest; the check bounds the 75-min hot loop). No cgroup
  background-process detection — activity = ledger events + in-flight
  jobs/bridge calls, stated on the page so a future cgroup slice extends
  rather than corrects.
- **Files:** `pkg/autoresearch/engine.go` (activity + check hook),
  `pkg/api/server.go` (sleep/wake/drain), `pkg/ledger/ledger.go` (loop row
  writer + `reserved_amount`).
- **Tests:** idle → sleeping, history intact; wake → same session, no second
  loop under wake-retry; drain refuses starts; restart scope; mid-run cap
  breach stops at the next iteration with spend accounted; negative-tested.
- **Live proof:** short-override timeout → sleep → prompt → resume, one loop
  row throughout.
- **Wiki:** autoresearch + gaps rows.

### S4 — Secrets discipline (S) — ✅ LANDED 2026-10-17

> Landed reframed by mapping: no dotenv writers and no secret values cross
> HTTP anywhere (leases are valueless HMAC receipts; `VaultClient` leases
> were tested but unwired), so the dotenv/atomic-write item had no target
> and was dropped. Shipped instead: session/tenant binding on issuance
> (ghosts 404, event carries session/tenant + reserved `caller_binding`
> for a future peercred sidecar); scope alphabet+length caps; TTL capped
> at 24h; server-side lease registry wired (issue/consume/revoke teeth,
> one ID namespace); `POST /api/vault/leases/revoke` (double-revoke 404s);
> both lease routes bearer-gated (existing key test strengthened with a
> 401-without case); seeded-canary absence scan proving the HMAC key never
> enters any ledger payload. Negative-tested; race green; live
> 401/200/400/400/200/404 matrix.

- **Threat:** values leak into history; multiline breaks consumers; caller
  confusion (G8).
- **Logic:** exact-set single-line ≤16 KiB answers (refuse otherwise);
  values never written to ledger/events/diagnostics (seeded-value
  absence-scan test over all emitted records); atomic `0600` dotenv writes
  (tmp + rename, no partials); lease record carries `session_id`,
  `tenant_id`, `scope`, single-use consume, expiry, and a reserved
  `caller_binding` field (Q5: portable binding now, peercred later without
  schema change). Bao lease model unchanged.
- **Files:** `pkg/vault/*`, `pkg/api` secret paths, absence-scan test.
- **Live proof:** request → answer → event-stream grep shows names only.
- **Wiki:** [[secrets-and-workload-isolation]] + runbook note.

### S5 — Session export/import, single-record migration (M, needs S1 keys) — ✅ LANDED 2026-10-17

> Landed: `POST /api/sessions/export|import`, manifest v1 with per-section
> digests (normalized form — typed-vs-map and int64/float64 stable,
> datetimes truncated to DB seconds, event IDs preserved, server-bookkeeping
> `updated_at` excluded), caps enforced (64 MB / 50 versions / 256 KB
> canvas), validation before existence-replay (tampered-existing 400s —
> caught live), ONE `session_imported` record, non-terminal jobs keep rows
> but no engine resumes them. > **Residual (2026-10-17, see [[rea-conflicts-and-remediation]] C2):**
> the manifest covers Go tables only — Python tenant sidecars
> (`ledger.<tenant>.jsonl`: `workflow_step`, `workflow_dry_run`) do not
> migrate yet. S5b follow-up exports them as a ninth digest-pinned section.

I4 `DeleteSession`: refuses live loops /
> non-terminal jobs / outstanding reservations, else cascades all 9 tables
> (deletion receipt survives by design). I1 unified: `ledger.CanonicalHash`
> single source. 5 tests (round-trip digest equality, 5 refusal kinds,
> validate-before-replay, cascade refuse + full); negative automatic (new
> routes 404 on old code); live export/import/replay/tamper matrix.
>
> > ✅ S5b residual closed 2026-10-17 (`671200d`, see [[rea-conflicts-and-remediation]] C2):
> > the manifest carries the Python sidecar as a ninth digest-pinned
> > section with idempotent replay — workflow history migrates with the lab.

- **Threat:** raw-DB copying diverges silently (G9).
- **Logic:** export sweeps all 8 session tables **plus** already-orphaned
  rows (G6 backfill); manifest `version: 1`, per-section sha256, size caps
  (total ≤64 MB; versions ≤50, canvas ≤256 KB enforced on import);
  import validates (exactly-one history, no `..`/absolute paths — content
  rows have none by construction, digest-pinned, version-gated: unknown
  major refuses with `unsupported_manifest_version`); activation seals ONE
  ledger record; chunked upload rejects late/duplicate chunks (offset +
  digest, S1-style key on activate → double-activate replays the original
  receipt). Includes the I4 `DeleteSession` fix (refuse with active
  reservations/non-terminal work, else cascade all 8 tables).
- **Files:** new `pkg/session/migrate.go`, API routes, `DeleteSession`
  rework, manifest-version const (I1 hash reuse).
- **Tests:** round-trip equality; tampered digest/traversal/oversize/unknown
  version refused; double-activate replays; orphan sweep covers
  pre-fix rows; negative-tested.
- **Live proof:** export → fresh DB import → `verify` chain + resume.
- **Wiki:** memory + gaps rows.

### S6 — Observability redaction by construction (S) — ✅ LANDED 2026-10-17

> Landed: `sandbox.BoundOutput` (16 KiB cap, UTF-8-safe, truncation flagged
> with original byte counts) applied at `parseHarnessOutput` — the single
> choke where sandbox streams enter persisted structures (incl. the
> previously unbounded failure-reason string); job result blobs refuse above
> 1 MB (`result_too_large`, auditable) instead of persisting unbounded
> bridge payloads; closed event taxonomy (`KnownEventTypes`, ~60 rows)
> **enforced** at all three append chokes (a typo'd type can never fork the
> trail) with a conformance test over exercised flows; test-only event
> names migrated to registered types (spend-attribution story kept).
> Enforcement immediately caught a missing `proposal_generated` row.
> 8 tests; race green; live job shape intact, emitted types registered.

- **Threat:** PII/prompts/secrets drift into diagnostics (G10).
- **Logic:** fixed taxonomy (route, status, latency, counts — closed field
  set, new fields need a test update); raw stderr bounded 16 KiB, `0600`,
  rotation-capped, excluded from SSE/history/API; seeded prompt/secret
  absence-scan over every emitted record; support-bundle builder (if any)
  reuses the same allowlist (no second code path).
- **Files:** observability/recorder paths, FlightRecorder allowlist,
  absence tests.
- **Wiki:** [[agentic-memory-and-observability]] note.

### S7 — Queue ops + steer (M, needs S1) — ✅ LANDED 2026-10-17 (reframed)

> Mapping found no prompt queue (synchronous turns) and rewrite/regenerate
> already existing — so S7 shipped what was actually missing: `POST
> /api/jobs/{id}/cancel` (QUEUED never executes, RUNNING fenced via abort
> registry, CAS-guarded reservation unwind, terminal replays success,
> ghosts 404; deliberately ungated by disk — blocking cancel traps spend
> behind the outage) and rewrite hardening (ghost session 404s explicitly,
> `expected_content` compare-and-swap 409s on stale reads, disk-gated).
> Steer documented N/A: no background turn exists to steer into
> (synchronous model) — a mid-flight injection point would be engine
> surgery, not a queue op. Tests: cancel transition/replay/ghost,
> CAS match/stale/empty/ghost (+ manager CAS unit tests); negative
> automatic (new route 404s/405s on old code); race green; live
> cancel/replay/ghost + rewrite 200/409/404 (cancel-transition proven
> in-test where no worker races it).

- **Threat:** queued mistakes unfixable short of abort-all (G7).
- **Logic:** `edit`/`cancel`/`reorder` on queued items with
  `expected_revision` (mismatch → 409); `steer` targets the running turn
  where the engine supports it, else queues as next turn (never rewrites
  past turns); every op carries an S1 key.
- **Tests:** each op + revision-mismatch + steer-miss-queues;
  negative-tested.
- **Not in slice:** execution-semantics changes.

### S8 — Console UX (3 independent S slices, needs I5 flags) — ✅ LANDED 2026-10-17 (reduced)

> **S8a** `GET /api/sessions/context` + composer `ContextReadout`
> (BudgetHistory-denominated, fails-silent UI). **S8b** `OverflowCard`
> (pressure-gated, real actions, session-switch wired) + `JobProgress`
> polling wired into MeasuredOutputs (R7 intact). **S8c reduced:** no
> skills/packs/automations backends exist — hub would be theater; the
> wiring above is the real deliverable. 8 vitests, 86/86 + build clean.

- **S8a** context-% + compact tokens in the composer (real usage endpoint,
  no arithmetic stand-ins). **S8b** overflow card
  (compact / new session / escalate) + stepwise progress for long ops.
  **S8c** Tools hub (skills + packs + automations, search) + drawer-over-
  modals for new surfaces. Each: vitest + `pnpm build`, no behavior change
  without its test.

## 4. Verification matrix (every slice)

`go test -race ./...` · `pytest` from both CWDs · `pnpm build` + `pnpm test` ·
`wiki-lint --strict` · live both-directions proof on `:8080` · new tests
fail on old code · race test for every concurrent path (S1 duplicate storm,
S5 double-activate) · restart-persistence test for every stored receipt/row ·
wiki Correction/landed notes, never silent edits.

Related: [[cloudroom]] · [[spend-mandates-and-delegation]] ·
[[secrets-and-workload-isolation]] · [[agentic-memory-and-observability]] ·
[[workflows-and-agent-authoring]] · [[agent-ux-control-plane]] ·
[[implementation-gaps]] · [[risks]]
