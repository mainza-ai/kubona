---
type: concept
name: Open Backlog 2026-11-15
date: 2026-11-15
updated: 2026-11-18
tags: [plan, backlog, production, audit]
sources: [source-gap-analysis-plan, source-frontier-repo-audit-2026]
status: tracked
---

# Open Backlog — 2026-11-15

> ⚑ **Update (2026-11-18):** raw palette **0**; raw buttons **368**; coupling
> **50/51**; forms **44/195**. The ratchet values quoted in the 2026-11-15 text
> below are the historical snapshot; form migration and further coupling
> reduction remain open.

Reconciled against `HEAD ce149481` (both `develop` and `main`), the live
queues ([[remaining-work]], `docs/remaining-todos.md`), [[feedback-loops-and-notifications]],
[[ui-ux-vision]], the production audits, and code-level markers. This page is
the **current execution backlog**; it does not replace the durable design
record on the owning pages.

> ⚑ Status update (2026-11-15, local working tree): OB-01 is implemented for
> the **autoresearch LLM loop** (pending steers enter the proposal prompt and
> are marked applied only after a model turn); the supervisor optimizer loop
> has no model prompt and remains a recorded run-lane instruction. OB-02,
> OB-03, OB-04, and OB-07 are closed. OB-11 is closed: enable/disable,
> authoring/install, and removal are wired through Settings. OB-12's inventory UI already
> exists (`MemoryPanel`); the S9 cloud-LLM edge screen and ledger/spool write
> screening have landed. OB-14's
> provenance panel has landed (typed evidence rows + completeness summary);
> inline highlighting of unverified prose and table-cell numbers has landed;
> table cells and math source values are scanned by the checker. OB-15's
> proposal history surface has landed; the copilot prompt now demands a
> numeric metered bound, while absent bounds still refuse.
> OB-17's plan-draft persistence has landed; unresolved refs as typed needs
> remain. OB-18's 1 h ageing escalation has landed; obligation push events
> remain. OB-18 now emits session-owned obligation open/close events; key-owned
> bearer rows still poll. OB-16 hardware parent allowance rails are closed.
> OB-19 chart-series and campaign fact refs have landed. The raw-palette
> baseline is now 0; the raw-button ratchet is enforced at 368 (may only
> shrink); the coupling ratchet is at 54 files / 55 imports; the form-control
> ratchet is at 46 files / 202 controls. Form migration and further coupling
> reduction remain. The last local HF surface toast
> now routes through the single page toast slot. LCP/INP/heap/request-count
> gates now run in the dense runtime budget spec. Proposal-dismiss
> reasons now feed the copilot prompt as a generation signal. OB-20 is closed as
> out of scope: literature/prover/copilot tool paths have no loop identity, so
> no parts are fabricated.
> OB-13 now carries typed need envelopes for budget, approval, and
> anchor refusals, and the global refusal chip renders the sentence/remedy;
> run-card rendering of budget/retry needs and refusal-chip rendering of
> admission needs have landed. OB-13 is closed; job cards already render job
> needs, and loop needs carry the structured failure field.
> **Completion update:** the `react-test-renderer` migration is at **0 files**
> and the dependency is removed; node + browser coverage is merged before the
> unchanged floors; the Workbench lazy split reduced the largest first-load
> chunk from ~400 KiB to ~277 KiB; live console E2E is 16/16. See
> [[feedback-loops-and-notifications]].

## Method and evidence

- **Read:** the full remaining-work queue, the short execution list, the
  feedback/notification audit, the UI/UX status, and the production-readiness
  audit pages.
- **Checked:** current branch state, generated counts, contract inventory,
  and code markers (`TODO`/`FIXME`/`unimplemented`) with generated stubs and
  tests excluded.
- **Classified:** every item as a real gap, an operator-only deed, a debt
  item, or a deliberate deferral. “Deferred” is not the same as “forgotten”:
  each deferral names its trigger.
- **Not re-audited here:** the full 831-file Round-2 audit. Its blockers were
  closed by the Round-3/Phase-0/1/2 evidence recorded in
  [[production-readiness-audit-2026-10-26]]; re-verify the gate on the exact
  commit before citing “production-ready” as current fact.

## What is not open

These were checked and should not be carried as backlog:

- **Feedback P0 and notification foundation landed 2026-11-15.** Durable
  votes, GET hydration, toggle/clear, caller actor, advisory-review labelling,
  proposal decision events, derived inbox, read/resolve state, SSE domain,
  coalescing, and Notification Center. See
  [[feedback-loops-and-notifications]].
- **Contract modelling.** Inventory is **191 modeled / 16 client-only / 0
  unmodeled**, 165 schemas, 166 OpenAPI paths (2026-11-18). The 16 client-only names are
  intentional transport/browser/auth helpers, not missing operations.
- **Arbitrary-colour and size-named scale classes.** The colour class is
  closed (zero arbitrary colour-var utilities); size-named radius/text
  utilities fail the gate.
- **The 2026-10-26 audit P0s.** Every blocker has closure evidence in the
  audit page. Re-run the gate; do not reopen a blocker merely because an old
  note still lists it.
- **Wiki questions.** `wiki/questions.md` has no open rows; durable design
  forks are closed or tracked as decisions. Execution leftovers belong here,
  not there.
- **Prototype-only mechanisms.** Generated gRPC `Unimplemented` methods and
  Python `NotImplementedError` stubs are generated/interface scaffolding, not
  product gaps unless a live path calls them.

## P0 — production-truth blockers

These either make a user-visible claim false or block an external production
claim. Fix the claim or supply the evidence before broadening scope.

| ID | Gap | Evidence / current state | Exit criterion |
|---|---|---|---|
| OB-01 | **Mid-run steering is recorded but not consumed by the agent prompt.** The UI can say an instruction was applied at the next boundary, while the agent loop never reads the text. | **Closed 2026-11-18.** `engine.SetSteerSource` feeds pending steers into the next model turn (`server.go`); `loop_steers`/`loop_steered` remain the audit trail. | A queued steer changes the next model turn in a deterministic test; the UI only claims “applied” when the prompt consumed it. |
| OB-02 | **No cancel/edit of a queued steer before the boundary.** | **Closed 2026-11-18.** `UpdateSteer`/`CancelSteer` APIs plus console Edit/Cancel controls (`LoopPartsLane.tsx`) landed. | Typed cancel/edit API + console control; a cancelled steer is never applied; audit event records it. |
| OB-03 | **Feedback reason UI and proposal-dismiss UI are missing.** The APIs accept reason/actor, but the operator cannot enter a reason or dismiss from the proposal card. | **Closed 2026-11-18.** Feedback-reason input (`CopilotDrawer.tsx`) and proposal dismissal with a reason are wired. | Downvote reason persists and hydrates; dismiss captures reason and emits `proposal_dismissed`; E2E proves both. |
| OB-04 | **Claim-review policy.** | **Closed 2026-11-18:** advisory permanently; reviews are recorded/listed/notified via `peer_review` and never mutate the verdict ([[feedback-loops-and-notifications]]). | Decision recorded and reflected in the UI copy; no silent verdict changes. |
| OB-05 | **Operator evidence for production claims.** | **Operator-owned, not implementation (2026-11-18).** Reviewer key confirmed in the running container; VoiceOver/NVDA, `gho_…` revocation, paid-QPU invoice, and the runtime rate card are **not claimed** until dated evidence exists. | Each item has a dated evidence link or an explicit “not claimed” status; no prose claim outruns the evidence. |
| OB-06 | **Closed 2026-11-15.** Hardware-in-the-loop VQE/QAOA is implemented in the bridge (`run_vqe_hardware_loop`, `run_qaoa_hardware_loop`) with per-objective hardware batches; live QPU evidence is recorded. The old “local optimizer only” note was stale. | `platform/bridge/experiments.py`; [[vqa-vqe]]; live-QPU evidence. | Closed. |

## P1 — incomplete loops and missing functionality

| ID | Gap | Current state | Exit criterion |
|---|---|---|---|
| OB-07 | **Notification preferences and quiet routing.** | **Closed 2026-11-18.** `NotificationPrefs` (quiet + muted kinds) persist; muted kinds leave the derived inbox; quiet zeroes the badge and skips the Attention Director while keeping the inbox (`page.tsx`, `NotificationAttentionBridge.tsx`); browser tests cover the controls. | Quiet/muted state is observable and routing obeys it. |
| OB-08 | **Attention Director is not wired to every producer.** Only `SpatialCanvas3D` calls `notify()`. | **Closed 2026-11-18.** Attention Director wiring for notification severities landed (todos). | Terminal jobs/loops, approvals, budget, allowance, auth, and memory events route through one policy; no duplicate announcements. |
| OB-09 | **One polite live-region batch.** 51 `role=alert`, 70 `role=status`, 6 `aria-live` remain; inline + toast + status can announce twice. | **Closed 2026-11-18.** Single polite live region; duplicate toast/chip announcements removed (todos). | One batched polite region for non-blocking notifications; alerts only for blocking states; accessibility test. |
| OB-10 | **Live model invocation in `eval-llm`.** The CLI scores recorded predictions only; no live model call; only the copilot role has fixtures. | **Closed 2026-11-18.** `eval-llm --live` invokes a model and records predictions/report with provenance (todos). | `eval-llm --live` runs a model, records predictions/report with provenance; strict gate uses it; at least one non-copilot role gains fixtures. |
| OB-11 | **Skills editor/installer and per-skill enable/disable.** | **Closed 2026-11-18.** Skill authoring/installer UI and per-skill toggle landed (todos). | Install/edit/validate a skill from Settings; disable excludes it from prompt/tool; rejected files still name their fault. |
| OB-12 | **Memory inventory/retraction UI and S9 edge screening.** | **Closed 2026-11-18.** Memory inventory/retraction UI, ledger-write screening, and cloud-prompt screening (`screenCloudPrompt`, tested) landed. | Inventory lists/retracts memories; S9 screen refuses secret-shaped payloads at the cloud edge and ledger write path with tests. |
| OB-13 | **Typed need rendering is incomplete.** Budget/approval/anchor refusals now carry typed `need` envelopes, but job/run cards do not render these envelopes and loop needs still lack a recorded failure field. | **Closed 2026-11-18.** Typed need envelopes, refusal chip, and run-card needs landed (todos). | Owning rails emit each need from structured evidence; job/run cards render one sentence + remedy; no free-text inference returns. |
| OB-14 | **Claim provenance panel and citation highlights.** Provenance endpoint exists; console has no panel. Citation check scans prose only, not tables/rendered values, and has no inline highlight. | **Closed 2026-11-18.** `ClaimHistory` provenance panel; table/rendered citation scanning + inline highlights landed. | Claim card opens provenance rows; offending numbers are highlighted inline; table/value scanning is covered or explicitly out of scope. |
| OB-15 | **Proposal history/list and priced proposals.** Id-addressed GET exists for operators; no history surface; proposals without numeric price bounds are refused rather than priced. | **Closed 2026-11-18.** Proposal list/history and the metered-pricing refusal rule landed. | Proposal list/history with decision state; priced proposal path with explicit bound or an honest refusal reason. |
| OB-16 | **Hardware parent allowance rails.** `hardware_experiment`, `qaoa_hardware`, `spin_chain_hardware` remain per-job only. | **Closed 2026-11-18.** Hardware parent allowance rails landed. | Parent allowance covers hardware child jobs; draw/revoke/suspend tests; taint exclusion preserved. |
| OB-17 | **Plan draft persistence and unresolved tool references.** Plan card is ephemeral; unresolved tool references are facts, not typed needs. | **Closed 2026-11-18.** Plan draft persistence and typed unresolved references landed. | Plan draft survives reload or is explicitly disposable; unresolved refs emit typed needs. |
| OB-18 | **Obligation push events and ageing escalation.** **Closed 2026-11-18.** Obligation open/close events, SSE invalidation, and the 1 h escalation marker (`needs-you.ts`) landed. | [[remaining-work]] (needs-you tray). | Open/close emits events; SSE invalidates tray; ageing escalation policy implemented and tested. |
| OB-19 | **Chart series and campaign-level fact refs.** Per-fact chips exist for job/loop/approval/task; chart series and campaign facts do not carry refs. | **Closed 2026-11-18.** Chart-series and campaign-level fact refs landed. | Chart points/campaign facts carry producing-record refs or explicitly state “unattributed”. |
| OB-20 | **Run parts from remaining rails and copilot tool calls.** | **Closed 2026-11-18 as out of scope.** Literature/prover/copilot tool paths have no loop identity; results remain ledger records, and no parts are fabricated (recorded in [[remaining-work]] C1). | The limitation is explicit; no false “part” rows. |
| OB-21 | **Legacy surface retirement.** `WidgetStage` and the legacy molecule canvas remain; Recorder and Manifold are separate full-page products rather than Workbench views. | **Decision 2026-11-18.** Parity-gated product decision: keep until five-widget parity + one release of alias support; `KUBONA_CONSOLE_WORKBENCH=0` is the tested rollback. | Five-widget parity met; old aliases survive one release; rollback path still tested; then remove. |

## P2 — debt, quality, and performance

| ID | Gap | Current state | Exit criterion |
|---|---|---|---|
| OB-22 | **Closed 2026-11-15.** All former `react-test-renderer` tests are migrated; the dependency is removed and the ratchet reports 0 files. | Browser/real-DOM tests cover focus/layout; no new renderer use. | Closed. |
| OB-23 | **Downward `components → lib/api` imports.** | **Closed 2026-11-18 as an accepted ratchet:** 50 files / 51 imports, enforced by `scripts/console-coupling-ratchet.mjs` (may only shrink). Zero is not a completion target; new coupling fails CI. | Import gate prevents regression; per-surface migration stays opportunistic. |
| OB-24 | **Closed 2026-11-15.** Workbench lazy-loaded with Suspense; largest first-load chunk ~277 KiB, total 1133.4 KiB raw / 336.2 KiB gzip, still under the route ceiling. | Per-route budgets remain green. | Closed. |
| OB-25 | **LCP/INP/heap/request-count gates.** | **Closed 2026-11-18.** LCP/INP/heap/request-count budgets gate in the dense runtime spec. | Reproducible LCP/INP/heap/request budgets in CI with documented hardware variance. |
| OB-26 | **Raw-palette, form primitive, raw-button ratchets.** | **Closed 2026-11-18 as accepted ratchets:** palette **0**, forms **44 files / 195 controls**, raw buttons **368** — all CI-enforced and may only shrink. Zero is not a completion gate for the prototype. | Ratchets prevent regression; migration stays opportunistic. |
| OB-27 | **Per-component motion review.** | **Closed 2026-11-18.** Motion review completed; arbitrary duration/ease/animate values are banned by `console-token-lint`. | Each animated surface uses the motion vocabulary or documents why global collapse is sufficient. |
| OB-28 | **Manual screen-reader transcript.** | **Operator-owned, not claimed (2026-11-18).** Automated axe/keyboard gates are green; the manual transcript is not claimed. | Dated manual walkthrough with issues filed/fixed, or an explicit not-claimed status. |
| OB-29 | **Single-daemon budget reservation.** | **Closed 2026-11-18:** the ledger takes an exclusive `flock`; a second read-write daemon refuses (`TestSingleWriterGuard`). Single-daemon is the enforced deployment constraint, not an unstated assumption. | A second daemon cannot start; the refusal names the lock. |

## Conditional / deliberately deferred

Do not schedule these without the named trigger; they are not current gaps.

- **Python↔Go cross-rail mandates** — no flow needs them today.
- **Warm QIR worker** — measured workload does not justify it.
- **Bundle export endpoint** — needs run-archival design; turns cannot be
  reconstructed from events+chat.
- **Sidecar rotation** — needs chain-aware reads; rotation breaks the
  tail-chained ledger assumption.
- **S8c full hub** — needs skills/packs/automation backends; no theater.
- **Autoresearch UI** — engine is eval-driven; no operator workflow designed.
- **Krylov/mitigation/fidelity/ADMET/discovery-pipeline console forms** —
  agent-consumed stages; console already surfaces results.
- **Notebooks endpoint UI** — ResultsStudio already renders campaign content.
- **Canvas layout save/load** — superseded by interactive SpatialCanvas
  persistence; do not re-open the old note.
- **Per-send model override** — no backend contract; composer links to Model
  Hub instead of faking one.
- **OTel/Sentry, containerized stdio MCP broker, IdP/human membership,
  pgvector, sharding, full LLVM lowering, KEMTLS, extra providers, Lean
  image** — optional or blocked on a deployment/toolchain decision.
- **Reviewer-key and paid-QPU claims** — operator evidence, not code; see
  OB-05.

## Execution status (2026-11-18)

**No code-addressable backlog item remains.** Phases 1–4 of the original plan
landed on 2026-11-18 (steering, typed needs, provenance, proposal history,
skills/memory/hardware rails, renderer migration, chunk split, runtime
budgets, token/motion review). The current state is:

- **Closed by decision:** OB-04 (advisory permanently), OB-20 (no loop identity
  for literature/prover/copilot parts), OB-23 and OB-26 (accepted CI-enforced
  ratchets, not zero targets), OB-29 (single-writer `flock` refuses a second
  daemon).
- **Operator-owned evidence, not implementation:** OB-05/OB-28 — VoiceOver/
  NVDA, `gho_…` revocation, paid-QPU invoice, runtime rate card, fresh-volume
  reset. Register: `docs/operator-evidence-2026-11-18.md`.
- **Conditional infrastructure:** only with a named trigger (multi-daemon,
  pgvector pressure, OTel/Sentry, IdP, sidecar rotation, bundle export). No
  speculative platform is built.

## Sequencing rule

Code work stays closed unless a new evidence-backed gap appears. Operator
evidence gates only the production wording it supports; conditional
infrastructure waits for its trigger. Do not reopen a closed row merely
because an older snapshot still says “open”.
