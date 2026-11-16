---
type: concept
name: Open Backlog 2026-11-15
date: 2026-11-15
updated: 2026-11-15
tags: [plan, backlog, production, audit]
sources: [source-gap-analysis-plan, source-frontier-repo-audit-2026]
status: tracked
---

# Open Backlog — 2026-11-15

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
> needs, and loop needs carry the structured failure field. See
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
- **Contract modelling.** Inventory is **162 modeled / 14 client-only / 0
  unmodeled**, 139 schemas, 145 OpenAPI paths. The 14 client-only names are
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
| OB-01 | **Mid-run steering is recorded but not consumed by the agent prompt.** The UI can say an instruction was applied at the next boundary, while the agent loop never reads the text. | [[remaining-work]] (C10); `loop_steers`/`loop_steered` exist, prompt wiring does not. | A queued steer changes the next model turn in a deterministic test; the UI only claims “applied” when the prompt consumed it. |
| OB-02 | **No cancel/edit of a queued steer before the boundary.** | [[remaining-work]] (C10). | Typed cancel/edit API + console control; a cancelled steer is never applied; audit event records it. |
| OB-03 | **Feedback reason UI and proposal-dismiss UI are missing.** The APIs accept reason/actor, but the operator cannot enter a reason or dismiss from the proposal card. | [[feedback-loops-and-notifications]]; `dismissProposal` exists but no UI call site. | Downvote reason persists and hydrates; dismiss captures reason and emits `proposal_dismissed`; E2E proves both. |
| OB-04 | **Claim-review policy is undecided.** Reviews are explicitly advisory; the open question is whether that is permanent or whether a written rule changes claim state. | [[feedback-loops-and-notifications]] P0-3. | Decision recorded: either “advisory permanently” with requester notification, or a written transition rule with tests and notifications. |
| OB-05 | **Operator evidence for production claims is incomplete.** Manual VoiceOver/NVDA was skipped; `gho_…` token revocation is not proven; reviewer key configuration must be confirmed on the host; paid-QPU evidence/invoice is not attached; runtime rate card is due before 2026-11-29. | [[remaining-work]] §A; `docs/remaining-todos.md`. | Each item has a dated evidence link or an explicit “not claimed” status. No prose claim outruns the evidence. |
| OB-06 | **Closed 2026-11-15.** Hardware-in-the-loop VQE/QAOA is implemented in the bridge (`run_vqe_hardware_loop`, `run_qaoa_hardware_loop`) with per-objective hardware batches; live QPU evidence is recorded. The old “local optimizer only” note was stale. | `platform/bridge/experiments.py`; [[vqa-vqe]]; live-QPU evidence. | Closed. |

## P1 — incomplete loops and missing functionality

| ID | Gap | Current state | Exit criterion |
|---|---|---|---|
| OB-07 | **Notification preferences and quiet routing.** | No server-synced per-kind/severity delivery; no quiet hours/presence routing. | Server preferences + console controls; inbox/toast/attention routing obeys them; tests cover quiet mode. |
| OB-08 | **Attention Director is not wired to every producer.** Only `SpatialCanvas3D` calls `notify()`. | [[feedback-loops-and-notifications]]. | Terminal jobs/loops, approvals, budget, allowance, auth, and memory events route through one policy; no duplicate announcements. |
| OB-09 | **One polite live-region batch.** 51 `role=alert`, 70 `role=status`, 6 `aria-live` remain; inline + toast + status can announce twice. | Feedback audit. | One batched polite region for non-blocking notifications; alerts only for blocking states; accessibility test. |
| OB-10 | **Live model invocation in `eval-llm`.** The CLI scores recorded predictions only; no live model call; only the copilot role has fixtures. | [[remaining-work]] (B12). | `eval-llm --live` runs a model, records predictions/report with provenance; strict gate uses it; at least one non-copilot role gains fixtures. |
| OB-11 | **Skills editor/installer and per-skill enable/disable.** | Catalogue + `load_skill` exist; no authoring UI or toggle. | Install/edit/validate a skill from Settings; disable excludes it from prompt/tool; rejected files still name their fault. |
| OB-12 | **Memory inventory/retraction UI and S9 edge screening.** | Tombstones + recall defence exist; no list/retract UI; the secret screen is not wired to cloud-LLM request edge or general ledger writes. | Inventory lists/retracts memories; S9 screen refuses secret-shaped payloads at the cloud edge and ledger write path with tests. |
| OB-13 | **Typed need rendering is incomplete.** Budget/approval/anchor refusals now carry typed `need` envelopes, but job/run cards do not render these envelopes and loop needs still lack a recorded failure field. | [[remaining-work]] (C2). | Owning rails emit each need from structured evidence; job/run cards render one sentence + remedy; no free-text inference returns. |
| OB-14 | **Claim provenance panel and citation highlights.** Provenance endpoint exists; console has no panel. Citation check scans prose only, not tables/rendered values, and has no inline highlight. | [[remaining-work]] (B4/B5). | Claim card opens provenance rows; offending numbers are highlighted inline; table/value scanning is covered or explicitly out of scope. |
| OB-15 | **Proposal history/list and priced proposals.** Id-addressed GET exists for operators; no history surface; proposals without numeric price bounds are refused rather than priced. | [[remaining-work]] (B1). | Proposal list/history with decision state; priced proposal path with explicit bound or an honest refusal reason. |
| OB-16 | **Hardware parent allowance rails.** `hardware_experiment`, `qaoa_hardware`, `spin_chain_hardware` remain per-job only. | [[remaining-work]] (C11). | Parent allowance covers hardware child jobs; draw/revoke/suspend tests; taint exclusion preserved. |
| OB-17 | **Plan draft persistence and unresolved tool references.** Plan card is ephemeral; unresolved tool references are facts, not typed needs. | [[remaining-work]] (C8). | Plan draft survives reload or is explicitly disposable; unresolved refs emit typed needs. |
| OB-18 | **Obligation push events and ageing escalation.** Tray polls; no `obligation_*` ledger events; no 1-hour escalation. | [[remaining-work]] (needs-you tray). | Open/close emits events; SSE invalidates tray; ageing escalation policy implemented and tested. |
| OB-19 | **Chart series and campaign-level fact refs.** Per-fact chips exist for job/loop/approval/task; chart series and campaign facts do not carry refs. | [[remaining-work]] (C9). | Chart points/campaign facts carry producing-record refs or explicitly state “unattributed”. |
| OB-20 | **Run parts from remaining rails and copilot tool calls.** Literature/prover/tool rails record no parts; chat turns are not loops. | [[remaining-work]] (C1). | Each rail records parts or documents why it cannot; copilot tool calls render as parts or are explicitly out of scope. |
| OB-21 | **Legacy surface retirement.** `WidgetStage` and the legacy molecule canvas remain; Recorder and Manifold are separate full-page products rather than Workbench views. | [[remaining-work]] (legacy retirement). | Five-widget parity met; old aliases survive one release; rollback path still tested; then remove. |

## P2 — debt, quality, and performance

| ID | Gap | Current state | Exit criterion |
|---|---|---|---|
| OB-22 | **71 grandfathered `react-test-renderer` files.** | Baseline ratchet only shrinks; CitationBadge is the first migrated file. | 94 → 0; browser/real-DOM tests cover focus/layout; no new renderer use. |
| OB-23 | **67 downward `components → lib/api` imports.** | Legal by layer rule, but couples UI to transport. | Move behind domain projections; target agreed per surface; import gate prevents regression. |
| OB-24 | **Closed 2026-11-15.** Workbench lazy-loaded with Suspense; largest first-load chunk ~277 KiB, total 1133.4 KiB raw / 336.2 KiB gzip, still under the route ceiling. | Per-route budgets remain green. | Closed. |
| OB-25 | **LCP/INP/heap/request-count gates.** | Static/DOM budgets exist; runtime metrics do not gate. | Reproducible LCP/INP/heap/request budgets in CI with documented hardware variance. |
| OB-26 | **Final 14 raw-palette baseline occurrences, form primitive migration, raw-button ratchet.** | Token lint holds at 14; forms still use raw utilities outside primitives. | 14 → 0; Field/Select/Toggle migrations complete; raw-button ratchet only shrinks. |
| OB-27 | **Per-component motion review.** | Global CSS collapse works; only the workbench glide consumes the hook. | Each animated surface uses the motion vocabulary or documents why global collapse is sufficient. |
| OB-28 | **Manual screen-reader transcript.** | Automated axe/keyboard green; manual VoiceOver/NVDA not run. | Dated manual walkthrough of every modal/surface with issues filed/fixed. |
| OB-29 | **Single-daemon budget reservation.** | In-process reservations do not coordinate multiple daemons sharing one ledger. | Either multi-daemon coordination or an explicit single-daemon deployment constraint enforced by preflight. |

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

## Implementation plan

### Phase 0 — evidence and decisions (operator; 0.5–1 day)

1. Revoke/confirm the `gho_…` token; record the provider response.
2. Confirm `reviewer_key_configured=true` on the deployment host.
3. Attach paid-QPU instance/run id/invoice or mark the claim unpaid.
4. Record the runtime rate card before 2026-11-29.
5. Decide OB-04 (advisory permanently vs written verdict rule).
6. Run the manual VoiceOver/NVDA walkthrough and file findings.

**Gate:** every production-facing claim has a dated evidence link or is
explicitly marked unclaimed.

### Phase 1 — close feedback and notification loops (2–3 days)

- OB-03 reason UI + proposal dismiss UI/reason.
- OB-04 decision implementation (notification to requester either way).
- OB-07 preferences/quiet routing.
- OB-08 Attention Director policy wiring.
- OB-09 one polite live-region batch; remove duplicate toasts/chips.
- Acceptance: vote → reload preserves; reason persists; dismiss is auditable;
  review decision notifies; quiet mode suppresses only non-blocking delivery;
  one announcement per event.

### Phase 2 — make agent/runtime claims true (3–4 days)

- OB-01/OB-02 steering prompt consumption + cancel/edit.
- OB-13 typed need emitters + loop failure field.
- OB-14 provenance panel + inline citation highlight.
- OB-15 proposal history + priced-proposal rule.
- OB-17 plan draft persistence + unresolved-ref needs.
- OB-10 live eval invocation (can start in parallel; needs provider budget).
- Acceptance: deterministic tests prove each visible claim; no “applied”
  label without prompt consumption.

### Phase 3 — skills, memory, hardware (4–6 days)

- OB-11 skills editor/installer/toggle.
- OB-12 memory inventory + S9 edge screening.
- OB-16 hardware parent allowances.
- OB-06 hardware-in-the-loop VQE/QAOA, or explicitly narrow the claim.
- OB-18 obligation events/ageing; OB-19/OB-20 remaining fact/part refs.
- Acceptance: each surface has a real consumer and an audit event; hardware
  loops record every measured iteration.

### Phase 4 — console quality and performance (3–5 days)

- OB-22 renderer migration; OB-23 coupling reduction.
- OB-24 chunk split; OB-25 runtime budgets.
- OB-26 token/form/button debt; OB-27 motion review.
- Acceptance: 95 → 0 renderer files; chunk under target; LCP/INP/heap gates
  green; token baseline 0; no new raw-button growth.

### Phase 5 — optional infrastructure (trigger-based)

- OB-29 multi-daemon coordination only if multi-daemon is supported.
- OTel/Sentry, MCP broker, IdP, pgvector, sidecar rotation, bundle export only
  when a deployment/consumer exists.
- Acceptance: the trigger is recorded before work starts; no speculative
  platform is built.

## Sequencing rule

Do **not** add new surfaces while OB-01–OB-04 remain: the product would gain
more controls whose claims are not yet true. Phase 1 and Phase 2 are the
critical path; Phase 0 runs in parallel and gates any production wording.
