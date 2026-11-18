# Active implementation TODOs — 2026-11-18

Local-only execution list. No push until explicitly requested.
Baseline: `d9a0c1ab` on `develop`.
Audit: `docs/production-audit-2026-11-18.md` (K-01…K-14).

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

## Copilot drawer containment (2026-11-17)

- [x] C1: constrain MathText prose/table min-content width; add browser regression.
- [x] C2: Copilot-specific visible splitter, width persistence, reset, and viewport clamp.
- [x] C3: table semantics, numeric alignment/formatting, scroll cue, table copy row.
- [x] C4: full target label/wrap and separate Δ badge.
- [x] C5: mobile/desktop width and resize E2E coverage.
- [x] C6: table copy and Narrow/Wide/Reset presets (full-screen table expansion remains optional).

## Production audit slices (2026-11-18)

Publication pipeline first; each slice closes only with executable acceptance.

- [ ] R0: remove publish/PDF affordances that do not exist; attach the
      convergence figure only when energies exist; label or drop illustrative
      figures; no manuscript metadata may claim unreferenced figures.
- [ ] R1: `manuscripts`, `manuscript_sections`, `manuscript_reviews`,
      `manuscript_artifacts` tables + tenant-scoped Go routes
      (`/api/sessions/{id}/manuscripts…`); draft built server-side from the
      campaign projection.
- [ ] R1: OpenAPI + console contract entries for every new route; cross-tenant
      refusal tests; content hash stable across reads.
- [ ] R2: resolve provenance receipts and Lean proof hashes against the ledger;
      append `peer_review` and `manuscript_signoff` events; forged receipt
      cannot reach ACCEPT.
- [ ] R3: TeX toolchain (Tectonic-class) in an isolated build job with a fixed
      `SOURCE_DATE_EPOCH`; `\input` sections; `\includegraphics` for declared
      figures; resolved `references.bib`; artifact SHA-256; compile test in CI.
- [ ] R3: two builds of the same manuscript are byte-identical (or the
      deterministic exception is documented).
- [ ] R4: Results Studio draft → review → signoff → build → download; persist
      annotations as `manuscript_reviews`; show build hash and signoff state.
- [ ] R4: browser test covers reload persistence, signoff refusal after a
      content change, and PDF download.
- [ ] R5: Zenodo manifest generated from built artifacts with checksums;
      refuses without ACCEPT + valid signoff; human upload only.
- [ ] R6: `KUBONA_BACKUP_REQUIRE_ENCRYPTION=1` with an age recipient, or a
      signed plaintext waiver; `KUBONA_PREFLIGHT_REQUIRE_BACKUP=1`.
- [ ] R6: restore a real `kubona.db.backup-*` file into a scratch ledger (not
      only `restore-drill`).
- [ ] R6: metrics for job queue depth/age, bridge refusals, backup age, chain
      faults, settlement skips; alert on backup staleness.
- [ ] R7: persist literature used by a manuscript; emit stable BibTeX keys;
      bind citations to claims; record novelty distance as measured or
      unmeasured.
- [ ] Operator: fresh-volume reset to restore the declared `local_private`
      posture (documented procedure; preserves a verified snapshot).

## Later

- [x] Hardware parent allowance rails.
- [x] Hardware-in-the-loop VQE/QAOA (bridge implements both; live evidence recorded).
- [x] Plan draft persistence.
- [x] Obligation 1 h ageing escalation.
- [x] Obligation push events for session-owned rows (key-owned still polls).
- [x] Chart-series and campaign-level fact refs.
- [x] Run parts from literature/prover/tool rails — out of scope until a real loop identity exists.
- [ ] Legacy `WidgetStage`/Recorder/Manifold retirement.
- [x] Console: 0 renderer migrations (dependency removed).
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
