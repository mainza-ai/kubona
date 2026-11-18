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

Executed 2026-11-18; each box is closed only by a test or a live run named in
`docs/production-audit-2026-11-18.md` §11.

- [x] R0: no fabricated/static figures; `compile_latex` refuses SVG; the
      bilingual constructor takes real caller figures; convergence data is a
      table (tests pin empty/no-figure behavior).
- [x] R1: `manuscripts`, `manuscript_sections`, `manuscript_reviews`,
      `manuscript_artifacts`, `manuscript_signoffs`, `manuscript_citations`
      tables; tenant-scoped routes under `/api/sessions/{id}/manuscripts`;
      drafts built server-side from the stored campaign snapshot; immutable
      after review.
- [x] R1: OpenAPI + console contract entries (180 modeled / 15 client-only /
      0 unmodeled, 156 schemas, 157 paths); cross-tenant refusal tests;
      content hash stable across reads.
- [x] R2: receipts resolved against the campaign's signed receipts; proof
      hashes resolved against comparator events (none exist yet, so a
      supplied hash stays unverified); `manuscript_drafted/reviewed/signed/
      built/deposit_staged` events; signoff binds the content hash; forged
      references are named unresolved.
- [x] R3: pdfTeX in the bridge image; `\input` sections; `\includegraphics`
      for declared PNG/PDF/JPEG; `references.bib`; artifact SHA-256;
      byte-identical rebuilds (test + live).
- [x] R3: source-change refusal: a recomposition whose hash differs from the
      signed content hash returns 409 `source_changed`.
- [x] R4 (partial): Publications panel in Results Studio: create draft,
      re-review, build, operator signoff, deposit staging, artifact download;
      browser tests cover the empty/create/build/signoff-refusal/deposit-disabled
      states. **Open:** annotations are still ephemeral chat prompts; persisting
      them as manuscript review notes is the remaining R4 item.
- [x] R5: Zenodo manifest generated from built artifacts with checksums;
      refuses without an ACCEPT review and a valid signoff; human upload only
      (`auto_publish:false`, `doi:null`).
- [x] R6: `/metrics` adds job-status, manuscript-state, bridge-configured, and
      backup count/age gauges; `kubonad` mounts `kubona-backups` read-only;
      `backup-status`/preflight fail when encryption is declared but the newest
      backup is plaintext; `scripts/backup-restore-check.sh` restores the real
      scheduled artifact and re-verifies the chain.
- [ ] R6 operator: set `KUBONA_BACKUP_AGE_RECIPIENT` + `REQUIRE_ENCRYPTION=1`,
      or record a plaintext waiver; set `KUBONA_PREFLIGHT_REQUIRE_BACKUP=1`.
- [x] R7 (partial): citations persist in `manuscript_citations` and become a
      resolved BibTeX file with `\cite` keys; novelty distance is recorded as
      measured or explicitly unmeasured. **Open:** literature lookup does not
      yet feed citations automatically, and claim→citation binding is manual.
- [ ] Operator: fresh-volume reset to restore the declared `local_private`
      posture (documented procedure; preserves a verified snapshot).

## Follow-up audit slices (2026-11-18, second pass)

Findings and evidence: `docs/publication-followup-audit-2026-11-18.md`.
F-01…F-10 (nested artifacts, order-sensitive hash, citation collisions, target
escaping, size checks, build limiter, schema v2, preflight env, figure-hash
verification, receipt cap) are landed with tests.

- [x] A1: `manuscript_annotations` table + routes; annotations survive reload;
      signoff refuses while any annotation is open; panel add/resolve UI and
      browser tests. (closes G-03)
- [x] A2a: citations gain `claim_id`/`source_id`/`verified`; a bound claim is
      verified against the ledger (unknown claim → 400); `citation_distance`
      is accepted, validated, and passed to the review (measured or unmeasured).
      (part of G-05, G-13)
- [ ] A2b: literature selection UI auto-feeds citations from `/api/literature`;
      link citations to the selected records' source ids. (rest of G-05)
- [ ] A3: comparator producer (Lean job/bridge RPC) emits
      `formal_verification`; an ACCEPT + signoff + deposit path is exercised
      end to end. (closes G-04)
- [x] A4a: per-session artifact byte ceiling
      (`KUBONA_MANUSCRIPT_ARTIFACT_CEILING_BYTES`, default 256 MiB) enforced on
      build/deposit; delete removes rows + artifacts + events; artifact-bytes
      gauge. (part of G-06)
- [ ] A4b: session-delete evidence semantics and a retention/export policy
      (builds replace today, so "keep N builds" is a policy for versions, not
      this storage). (rest of G-06)
- [x] A5: `AppendEventWithSpool` for manuscript events (failure refuses);
      scrubbed `build.log` artifact; build success/failure + duration +
      artifact-bytes metrics. (closes G-07, G-08)
- [x] A6a: create idempotency (`request_id`, unique per session) and
      `deposit_ready` state after staging. (part of G-09/G-10)
- [x] A6b: draft title/abstract edit before review (review reset), delete
      (operator for signed/built), list pagination (`limit/offset`). (G-09)
- [x] A7: authenticated blob artifact download; review-detail panel with
      measured/unmeasured dimensions. (closes G-12, G-13 UI)
- [x] A8: report route patterns vs OpenAPI paths/operations separately; count
      bridge tests explicitly. (closes G-11, G-15)
- [ ] Operator: backup encryption recipient + `REQUIRE_ENCRYPTION=1` (or a
      signed plaintext waiver) and `PREFLIGHT_REQUIRE_BACKUP=1`. (G-01)
- [ ] Operator: execute the fresh-volume reset and re-run
      `local-deployment-check.sh --expect-single-lab`. (G-02)

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
