# Active implementation TODOs — 2026-11-15

Local-only execution list. No push until explicitly requested.
Baseline: `37103aad` on `develop`.

## Now

- [x] Proposal history/list surface (server list + console panel).
- [x] Typed need envelopes: `budget_exceeded`, `approval`, `anchor_missing`.
- [x] Refusal chip renders the typed need sentence and remedy.
- [x] Run card renders typed budget/retry needs; resume clears them.
- [x] Admission needs render in the refusal chip (no job exists).
- [x] Table citation scanning + inline table highlights.
- [x] Rendered math-source citation scanning.
- [x] Attention Director wiring for notification severities.
- [x] Live-run toast duplication removed (single polite region).
- [ ] Remaining per-surface toast/chip dedupe.
- [ ] Proposal-dismiss reasons used in ranking.
- [x] Skill authoring/installer UI.
- [x] Live `eval-llm` model invocation (explicit provider/model).
- [x] General ledger-write secret screening.
- [ ] Proposal pricing rule for plans without a numeric bound.

## Later

- [x] Hardware parent allowance rails.
- [x] Hardware-in-the-loop VQE/QAOA (bridge implements both; live evidence recorded).
- [x] Plan draft persistence.
- [x] Obligation 1 h ageing escalation.
- [x] Obligation push events for session-owned rows (key-owned still polls).
- [x] Chart-series and campaign-level fact refs.
- [ ] Run parts from literature/prover/tool rails.
- [ ] Legacy `WidgetStage`/Recorder/Manifold retirement.
- [ ] Console: 95 renderer migrations.
- [ ] Console: reduce 67 `components → lib/api` couplings.
- [ ] Console: split the ~395 KiB chunk.
- [ ] Console: LCP/INP/heap/request-count gates.
- [ ] Console: final 14 token baseline, form primitives, raw-button ratchet.
- [ ] Console: per-component motion review.

## Operator-only

- [ ] Manual VoiceOver/NVDA walkthrough.
- [ ] Revoke/confirm the `gho_…` token.
- [ ] Confirm reviewer key on the deployment host.
- [ ] Attach paid-QPU instance/run id/invoice or mark unpaid.
- [ ] Record runtime rate card before 2026-11-29.
