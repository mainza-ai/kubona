# Remaining execution TODOs

Updated 2026-10-30. This is the short execution list; the full queue lives in
`wiki/concepts/remaining-work.md`.

## Console/backend integration execution (2026-10-30)

- [x] Multi-lab login fallback: free-text lab ID when anonymous lab listing
      is refused; first-run lab-admin bootstrap accepts the operator token
      once and mints the browser key explicitly.
- [x] Action Gate and Guardrails approval now pass the signed mandate to
      `/api/chat/launch` and report a real job id (previously verify-only).
- [x] Fleet telemetry is projected from `/api/quantum/backends`; invented
      queue/coherence/cost numbers removed (null = unknown).
- [x] Browser E2E lane added (`.github/workflows/console-e2e.yml`,
      `platform/console/e2e/`): admin bootstrap, login, local dispatch,
      codesign, refusal state against the real compose stack.
- [x] Visible API-refusal banner; session/campaign/job polling; measured
      outputs and flight recorder refresh.
- [x] Governance surfaces: lab creation, autonomy, obligations, ledger
      verify; canary results; honest fallback removal; LLM key sealing with
      `KUBONA_SECRET_KEY`; same-origin Atlas image proxy; system-agent,
      results-notebook, batch-evaluation, autoresearch, workflow plan/run,
      and short-lived vault-lease panels; fidelity contract and provider
      schemas packaged into the bridge image.
- [x] Tenant grants UI; browser admin-key recovery/rotation with an
      encrypted offline kit; route-level console/backend contract gate
      (109 paths); structural response contracts for the live session and
      operational payloads (sessions, events, loops, jobs, messages, fleet,
      capabilities, canvas, obligations, quantum backends, agents, labs,
      cost, memory, workflows, QIR, auth); SSE telemetry with polling
      fallback; canvas layout persistence; narrow-screen canvas density;
      keyboard skip link, live-region toasts, and dock aria-current.
      Response contracts now cover chat/tribunal, autoresearch/metasearch,
      codesign, LLM/HF, vault/predictions, lens, ledger, and settings.
- [x] Mobile/keyboard smoke gate: one skip target, one main landmark,
      `aria-current` on the dock, live-region toasts, and a narrow-viewport
      E2E proving the copilot starts closed and opens as an overlay.
- [x] Run-state live region and a named, pressed-state canvas mode group.
- [ ] Still open: deep accessibility audit across every modal/surface
      (contrast, focus order, reduced motion, screen-reader transcripts).

## Production readiness plan (2026-10-31)

The full gap register, phased plan, and definition of done are in
[`production-readiness-plan-2026-10-31.md`](production-readiness-plan-2026-10-31.md).

- [x] `kubonad restore` verified atomic restore + rollback tests.
- [x] Scheduled `backup` compose profile with optional age encryption,
      retention, sha256 receipts, and `backup-status` freshness/health.
- [x] Verified pre-migration snapshot gate for older schemas.
- [x] Restore/backup/upgrade documentation truth pass.
- [ ] Operator: runtime vendor rate card before 2026-11-29.
- [ ] Operator: paid IBM evidence + historical settlement reconciliation.
- [x] P1: SSE event IDs + Last-Event-ID cursor replay (server + client native reconnect).
- [x] P1: canvas layout now restores the 3D camera position and orbit target.
- [x] P1: `doctor` reports configured backup freshness and newer failure markers.
- [x] P1: structural a11y E2E (names, labels, img alt, one main landmark).
- [x] P1: generated response-contract inventory (61 modeled / 68 explicit
      unmodeled / 58 schemas), OpenAPI 3.1 (60 paths), CI freshness checks.
- [x] P1: optional external JSON webhook on preflight and backup failure.
- [x] P1: `kubonad preflight` gate before kubonad starts (rate card, ledger
      chain/schema, FK, optional backup freshness).
- [ ] P1: strict runtime validation for modeled responses; full OpenAPI
      coverage for the 68 unmodeled functions.
- [ ] P1: disk/rate-specific external alert routing beyond preflight/backup.
- [ ] P1: contrast/modal/reduced-motion/screen-reader audit.

## Production blockers

- [x] **Resolve the 606 pre-existing ledger chain faults — by disposition
      (2026-10-30).** The host DB contains 258 unhashed legacy v1 events plus
      unsigned commit-class events and their link cascade. Backfilling hashes
      or retro-signing would rewrite/launder history, so it is not done. The
      host DB is forensic/dev-only and is **not** a verified restore source;
      production must start on a fresh ledger. The raw snapshot remains
      `~/.kubona/backups/kubona-pre-fk-20261030T024721Z.db` (sha256
      `c15bffd6…146f3b`), SQLite-consistent but chain-faulted.
- [ ] **Vendor runtime rate-card reconciliation before 2026-11-11.** The
      embedded bootstrap was refreshed (retired models removed; current model
      prices/aliases) and production now requires a runtime
      `KUBONA_RATE_CARD_PATH`/`URL`; `kubonad rates-audit` verifies assigned
      models against live `/models` and can write a used-model-only card.
      Still needs the vendor price sheets/invoices and Atlas-lens evidence for
      the production card. Follow `docs/rate-cards.md`.
- [x] **Nightly thesis-scale memory fix (2026-10-30).** `Builder` stores ops
      in compact typed columns and the slice-60 tests stream them instead of
      materialising a 126M-object list. Local peaks: composition 3.4 GiB /
      595 s, determinism 5.0 GiB / 206 s (previously ~37 GiB). The full
      circuit and every assertion are unchanged.
- [x] **Nightly thesis-scale green run (2026-10-30).** Manual run
      `41041244412` is green: canary, slice60 composition, and slice60
      determinism all passed at commit `c7d98f6`. The first green was
      `41033108173` at `5514997`; earlier red runs (`40854405074`,
      `40666624246`, `40478035440`, `41000688966`, `41003185385`) are
      history. Do not weaken the tests.
- [x] **Attach QPU run evidence (with a paid-status caveat).** IBM job
      `dbha28pq260s73busu70` (`ibm_fez`, created 2026-10-29T01:28:03Z,
      DONE, 2 QPU seconds, 0.028795 RU, 100 shots, counts 00:58/11:38/10:1/
      01:3) maps to local job `39b5e6b832cd4ae3` (estimate $8.000568, local
      status cancelled, settled $0.00). The configured IBM instance reports
      Open Plan limits (600 s/month; 20 s consumed, 0.263828/10.0002 RU), so
      this repo cannot prove a paid invoice from that instance. Attach the
      paid instance/run id or invoice if it was a different account.
- [ ] **Reconcile the completed-vs-cancelled settlement.** The cancel path
      now queries the vendor status and settles measured usage
      (`quantum_jobs.go`, regression test). The historical host row was not
      rewritten: its vendor job completed and charged 2 s while the local
      ledger released $8.00. Reconcile/repair that row only with operator
      approval and the vendor invoice.
- [x] **Run the manual nightly and record the result.** Run
      `41041244412` is green at `c7d98f6` (canary + both slice60 jobs). The
      earlier canceled runs `41000688966` and `41003185385` remain history;
      the compact Builder + streaming evaluator now peak below 5 GiB.
- [x] **Host LLM default decided 2026-10-30:** the host's persisted
      `active=nvidia`, `air_gap=off` is treated as an intentional development
      override (NVIDIA keys are present); it is **not** the shipped default.
      The production compose/fresh volume defaults to Strata, and the console
      LLM Settings UI is authoritative per user. Cloud in production requires
      `KUBONA_LLM_CLOUD=1`; local providers (Strata/oMLX/Ollama/LM Studio)
      do not. Switch the host to Strata + air-gap in the UI if that is the
      desired dev default.
- [x] **Commit and push.** Current code head `c7d98f6` on `develop` and
      `main`; push CI 20/20 green (runs `41039225255` and `41039230782`);
      manual nightly `41041244412` green. Earlier green heads: `3e8b45e`,
      `5514997`, `ccd7797`, `ddd9d22`, `b318e7e`, `4836f0e`.

## Optional / follow-up

- [ ] Opt-in OTel/Sentry exporters (env-gated, scrubbed, sampled, default
      off; no prompts/keys/session ids). Needs a backend/DSN choice.
- [x] Local release dry-run (`scripts/release-dry-run.sh`): clean-tree
      refusal, counts/wiki/secret/compose gates, three-image build, no tag/
      push/sign.
- [ ] A containerized stdio MCP broker, if a deployment needs one; the
      supported local topology is host-spawned stdio.
