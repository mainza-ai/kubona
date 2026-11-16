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
- [x] Remaining per-surface toast/chip dedupe.
- [x] Proposal-dismiss reasons consumed by proposal generation (prompt signal).
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
- [x] Run parts from literature/prover/tool rails — out of scope until a real loop identity exists.
- [ ] Legacy `WidgetStage`/Recorder/Manifold retirement.
- [ ] Console: 17 renderer migrations (…, settings/shell batch, DataChart, MeasuredOutputs, CognitiveMesh migrated).
- [x] Console: coupling ratchet at 54 files / 55 imports (may only shrink).
- [ ] Console: continue reducing direct `components → lib/api` imports.
- [x] Console: split the ~395 KiB chunk (largest now ~277 KiB).
- [x] Console: LCP/INP/heap/request-count gates.
- [x] Console: raw-palette token baseline is 0.
- [x] Console: raw-button ratchet (368 baseline, may only shrink).
- [x] Console: form-control ratchet at 46 files / 202 controls (may only shrink).
- [ ] Console: continue migrating forms to Field/Select/Toggle.
- [ ] Console: per-component motion review.

## Operator-only

- [ ] Manual VoiceOver/NVDA walkthrough.
- [ ] Revoke/confirm the `gho_…` token.
- [ ] Confirm reviewer key on the deployment host.
- [ ] Attach paid-QPU instance/run id/invoice or mark unpaid.
- [ ] Record runtime rate card before 2026-11-29.
