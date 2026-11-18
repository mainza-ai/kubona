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
- [x] Proposal pricing rule for plans without a numeric bound: the mint
      refuses a metered backend with no numeric `max_cost_usd` (text estimates
      are not a spend bound); covered by
      `TestProposalMintRefusesUnpriceablePlans`.

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
- [x] R4: Publications panel in Results Studio: create draft, re-review,
      build, operator signoff, deposit staging, artifact download; browser tests
      cover the empty/create/build/signoff-refusal/deposit-disabled states.
      Annotations are durable manuscript review notes (`manuscript_annotations`
      table + add/resolve routes), and signoff refuses while any annotation is
      open (A1).
- [x] R5: Zenodo manifest generated from built artifacts with checksums;
      refuses without an ACCEPT review and a valid signoff; human upload only
      (`auto_publish:false`, `doi:null`).
- [x] R6: `/metrics` adds job-status, manuscript-state, bridge-configured, and
      backup count/age gauges; `kubonad` mounts `kubona-backups` read-only;
      `backup-status`/preflight fail when encryption is declared but the newest
      backup is plaintext; `scripts/backup-restore-check.sh` restores the real
      scheduled artifact and re-verifies the chain.
- [x] R6 operator: `KUBONA_BACKUP_AGE_RECIPIENT` + `REQUIRE_ENCRYPTION=1` are
      set, encrypted backups restore clean, and `KUBONA_PREFLIGHT_REQUIRE_BACKUP=1`
      is now set (preflight refuses without a fresh backup).
- [x] R7: citations persist in `manuscript_citations` and become a resolved
      BibTeX file with `\cite` keys; novelty distance is recorded as measured or
      explicitly unmeasured. Citations carry `claim_id`/`source_id`/`verified`,
      a bound claim is checked against the ledger, and the Publication panel
      searches OpenAlex and adds/removes references (A2a/A2b).
- [ ] Operator: fresh-volume reset to restore the declared `local_private`
      posture (documented procedure; preserves a verified snapshot).

## Agentic science frontier (2026-11-18)

Research: `docs/agentic-science-frontier-2026-11-18.md`; wiki
[[agentic-science-frontier-2026]]. Ordered so each slice produces a measured
capability, not a demo.

- [x] AG-1a: benchmark runner + **11-task** catalog + unit tests; disposable
      stack baseline published (8/11 pass@1; TFIM 4/6/8 miss their exact
      anchors at 11–54σ; QAOA 2/3 layers pass after the bit-order fix; H2
      ground state reaches chemical accuracy at 20k shots). See the frontier
      report §8 and `docs/accuracy-gap-investigation-2026-11-18.md`.
- [x] AG-1b: `.github/workflows/agentic-bench.yml` nightly/dispatch run with
      `--allow-failures`, a retained JSON artifact, and failure-only stack logs.
- [x] AG-2a: persisted hypothesis engine (evidence-grounded generation,
      dedupe, ranking, falsifier, literature refs, explicit novelty status,
      shortlist API + tests). 10 hypotheses on the H2 campaign.
- [x] AG-2b: strict-JSON LLM proposer with honest fallback; deterministic Elo
      tournament + derived meta-review; `lexical_containment_v1` measured
      novelty; evidence-gated collapse into a cited claim; shortlist-before-
      plan gate. Schema 7.
- [x] AG-3a: `experiment_plans` table/API; plan requires acceptance metric +
      stop rule + spec; approval gate; execute through the job rail and score
      the observable against the acceptance target.
- [x] AG-3b: `pkg/plans.Synthesize` merges campaign facts + the hypothesis's
      relative `spec_patch`; provenance is persisted (`spec_basis`); ungrounded
      methods/targets refuse with `spec_unavailable`. Schema 8.
- [x] AG-4a: `GET /research-graph` projects campaign, hypotheses, plans, jobs,
      claims, and manuscripts from the ledger.
- [x] AG-4b: `pkg/researchgraph` priority summary inside a byte budget with a
      truncation report; `Budget_bytes` validation; `since_seq`/`event_limit`
      resume cursor over the ledger chain.
- [x] AG-5: `pkg/verify` numeric + citation verification; `POST/GET /verify`
      with a content-hash-bound report; signoff blocks fabricated numbers and
      dead citations (an unavailable resolver requires an explicit, sealed
      acknowledgment); `kubonad verify-calibration` reports 12/12 against
      maintainer labels. Schema 9.
- [x] AG-6a: RO-Crate 1.1 metadata in `manuscript-export` (files, hashes,
      content hash, signoff).
- [x] AG-6b: `pkg/prov` W3C PROV-JSON + CRediT attribution; the export writes
      `prov.json`/`credit.json` and folds PROV-O nodes into the RO-Crate.
- [x] AG-7: `pkg/autonomy` stage modes (suggest/approve/act), v1 risk
      classifier (hardware/dual-use/budget + large-simulation), enforced at
      plan execution, persisted policy, and an operator halt that cancels live
      jobs. Schema 10.
- [x] AG-8a: `kubona_hypotheses{status}` + `kubona_experiment_plans{status}`
      gauges.
- [x] AG-8b: `GET /api/ops/science` with stage taxonomy, cost per accepted
      claim (actual-vs-reserved basis), human intervention counts with human
      minutes explicitly unmeasured, queue/loop health; matching Prometheus
      gauges.
- [x] AG-9: tenant-scoped `lab_knowledge` registry (negative results, claims,
      reports) with fingerprint dedupe, provenance, a prior-negative gate on
      plan creation, and owner-granted sharing. Schema 11.

## Agentic mode and oversight (2026-11-18)

Research/design report: `docs/agentic-mode-toggle-2026-11-18.md`.

- [x] AG-17: session agentic master switch (`agentic_enabled`) + operator lock
      (`agentic_locked`); session `PUT /agentic`; operator policy merge; halt
      lock; gates on loop start/resume/wake/step, plan execution, and restart
      rehydration; Autoresearch panel switch/status; Go and browser tests.
- [x] AG-18: global Manual/Auto/Locked indicator (status bar / command palette).
- [x] AG-19: durable approval inbox (`/approvals`, approve/reject/defer,
      restart-safe) with a console panel and browser tests. Reviewer edits are
      validated and risk-reclassified; rejection feedback persists on the plan;
      undo restores the pre-decision protocol.
- [x] AG-20: `notify` stage mode executes low/medium risk and seals
      `notify:true` receipts; `/agentic/digest` batches them and the console
      digest panel renders them (empty/unavailable states included).
- [x] AG-21: per-tool `allow|ask|block` policy enforced at copilot dispatch.
      `ask` records a durable one-time approval bound to the tool and the
      canonical argument digest; grants are spent once within a 15-minute TTL,
      and deny/spend/expiry all refuse. Schema 14.
- [x] AG-22: approval-fatigue telemetry (queue depth, latency percentiles,
      rubber-stamp proxy).
- [x] AG-23: `/receipts` + `/receipts/{id}/undo` for mode flips, unrun plan
      decisions, and deferrals; double-undo refused. Job/claim undo remains
      intentionally unsupported (immutable evidence).
- [x] AG-24: operator-owned tenant default (`/api/tenants/agentic-default`),
      policy history + sealed rollback, strict unreadable-policy refusal, and an
      expanded dual-use taxonomy. The default is operator-owned, not an
      admin-signed tenant action; document that boundary.

## Accuracy-gap follow-ups (2026-11-18)

Root-cause report: `docs/accuracy-gap-investigation-2026-11-18.md`.

- [x] AG-10: QAOA simulator shot scoring uses Qiskit bit order (shared
      `_counts_cut`, unit + 20k-shot tests); benchmark 11 tasks with
      depth-appropriate QAOA targets and 20k-shot accuracy tasks; runner records
      `uncertainty`/`z`; Inspector header wraps with a real-browser non-overlap
      test.
- [x] AG-11: TFIM multi-start optimizer (3–4 restarts; keep best) and
      gap-based `converged`/`optimizer_status`. Acceptance: n=8 4 layers ≤0.10 Ha
      and 6 layers ≤0.05 Ha statevector gap; small-n exactness preserved.
- [x] AG-12: noise-aware benchmark scoring (`uncertainty`, z, perfect-state
      control). Acceptance: exact-state TFIM control passes at task shots;
      fabricated results still fail.
- [x] AG-13: H2 chemical-accuracy task at 1.6 mHa; evaluate HVA/UCCSD and
      optimizer restarts.
- [x] AG-14: QAOA multi-start and depth tasks; document the p=2 ceiling.
- [x] AG-15: console container-query/width-aware panel controls; layout audit
      for every multi-group panel header.
- [x] AG-16: telemetry semantics (`optimizer_status`, `execution_time_ms`,
      QAOA `energy` naming).

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
- [x] A2b: the Publication panel searches OpenAlex and adds/removes references;
      `PUT /citations` recomposes the draft; source ids persist. (G-05)
- [x] A3 (boundary): operator-run `kubona.eval.comparator_cli` refuses any
      non-kernel result, signs a canonical attestation; the daemon verifies the
      configured comparator key and records `formal_verification`; ACCEPT +
      signoff + deposit exercised end to end with a signed attestation.
      (G-04 boundary; the live Lean run needs the pinned sandbox image)
- [x] A4a: per-session artifact byte ceiling
      (`KUBONA_MANUSCRIPT_ARTIFACT_CEILING_BYTES`, default 256 MiB) enforced on
      build/deposit; delete removes rows + artifacts + events; artifact-bytes
      gauge. (part of G-06)
- [x] A4b: `kubonad manuscript-export --session --out` exports metadata,
      sections, citations, reviews, annotations, signoff, and artifacts
      (hash-checked) from a read-only ledger. Exported evidence is the
      precondition for deleting a lab/session. (G-06)
- [x] Operator gate: `kubonad preflight --strict` refuses an unacknowledged
      plaintext backup (`KUBONA_BACKUP_PLAINTEXT_ACK=1` explicitly accepts).
- [x] Operator: comparator key generated (`~/.kubona/comparator.key`) and the
      public key configured on the daemon (`KUBONA_COMPARATOR_PUBKEY_HEX`).
- [ ] Operator: build/warm the pinned Lean image
      (`scripts/build-lean-sandbox.sh --warm`) and run a real challenge.
      **Attempted 2026-11-18:** the build stops at
      `FROM leanprover/lean4:v4.34.1` with "pull access denied… may require
      authorization". The pinned tag is not publicly pullable from this host, so
      substituting a different tag would void the pinned-kernel guarantee. This
      stays operator-owned until the authorized base image is available.
- [x] Operator: encrypted backups enabled (age recipient + REQUIRE=1), key at
      `~/.kubona/backup-age.key`; the encrypted artifact restored clean and the
      pre-encryption plaintext artifacts were removed.
- [ ] Operator: copy `~/.kubona/backup-age.key` off-host and execute the
      reset script (`scripts/reset-compose-state.sh --yes`) when ready.
      (Requires an off-host destination; destructive local reset stays
      operator-initiated.)
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
- [x] Operator: backup encryption recipient + `REQUIRE_ENCRYPTION=1` and
      `PREFLIGHT_REQUIRE_BACKUP=1`. (G-01)
- [ ] Operator: execute the fresh-volume reset and re-run
      `local-deployment-check.sh --expect-single-lab`. (G-02; destructive,
      operator-initiated.)

## Later

- [x] Hardware parent allowance rails.
- [x] Hardware-in-the-loop VQE/QAOA (bridge implements both; live evidence recorded).
- [x] Plan draft persistence.
- [x] Obligation 1 h ageing escalation.
- [x] Obligation push events for session-owned rows (key-owned still polls).
- [x] Chart-series and campaign-level fact refs.
- [x] Run parts from literature/prover/tool rails — out of scope until a real loop identity exists.
- [x] Legacy-surface decision: `WidgetStage`, the Recorder, and the Manifold
      stay as explicitly proposal-only/scroll products until object cards reach
      five-widget parity and the old aliases complete a release of support
      (recorded in [[remaining-work]]); `KUBONA_CONSOLE_WORKBENCH=0` is the
      tested rollback. This is a decision, not an unfinished migration.
- [x] Console: 0 renderer migrations (dependency removed).
- [x] Console: coupling ratchet at 54 files / 55 imports (may only shrink).
- [x] Console coupling batch: memory/jobs/session transport moved behind
      domain facades; ratchet lowered 54/55 → 50/51 (may only shrink further).
- [x] Console: split the ~395 KiB chunk (largest now ~277 KiB).
- [x] Console: LCP/INP/heap/request-count gates.
- [x] Console: raw-palette token baseline is 0.
- [x] Console: raw-button ratchet (368 baseline, may only shrink).
- [x] Console: form-control ratchet at 46 files / 202 controls (may only shrink).
- [x] Console forms batch: Autoresearch and Memory controls moved to
      Field/Select (compact `hideLabel`); ratchet lowered 46/202 → 44/195.
- [x] Console motion review: 0 `transition-all`, 0 arbitrary duration/ease/
      animate values (now banned outright by `console-token-lint`); UI
      transitions use 150–300 ms and ambient surfaces 500–1000 ms with explicit
      properties. Autoresearch/Memory/Approval surfaces checked in this pass.

## Operator-only

- [ ] Manual VoiceOver/NVDA walkthrough. (Needs assistive-technology hardware.)
- [ ] Revoke/confirm the `gho_…` token. (External GitHub credential.)
- [x] Confirm reviewer key on the deployment host: `KUBONA_REVIEWER_PRIVKEY_HEX`
      is present in the running `kubonad` container
      (`reviewer_key_present`), so manuscript signoff refuses only for other
      reasons.
- [ ] Attach paid-QPU instance/run id/invoice or mark unpaid. (External vendor
      evidence; not available in this repo.)
- [ ] Record runtime rate card before 2026-11-29. (Operator pricing decision.)
