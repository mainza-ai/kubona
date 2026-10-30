# Remaining execution TODOs

Updated 2026-10-30. This is the short execution list; the full queue lives in
`wiki/concepts/remaining-work.md`.

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
- [ ] **Nightly thesis-scale green run (pending).** The memory blocker is
      fixed locally; run `slice60-composition` and `slice60-determinism`
      manually on the hosted runner, record the run IDs, and do not weaken
      the tests. Call nightly green only after both jobs pass.
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
- [ ] **Run the manual nightly and record the result.** Previous
      `workflow_dispatch` runs `41000688966` and `41003185385` both passed the
      canary but had both slice60 jobs canceled by the runner after ~18–23 min
      with no test output/assertion. The compact-Builder fix now peaks below
      5 GiB; attach the new run IDs and do not weaken the tests.
- [x] **Host LLM default decided 2026-10-30:** the host's persisted
      `active=nvidia`, `air_gap=off` is treated as an intentional development
      override (NVIDIA keys are present); it is **not** the shipped default.
      The production compose/fresh volume defaults to Strata, and the console
      LLM Settings UI is authoritative per user. Cloud in production requires
      `KUBONA_LLM_CLOUD=1`; local providers (Strata/oMLX/Ollama/LM Studio)
      do not. Switch the host to Strata + air-gap in the UI if that is the
      desired dev default.
- [x] **Commit and push.** Current code head `ccd7797` on `develop` and
      `main`; CI 20/20 green (runs `41003173676` and `41003185157`). Earlier
      green heads: `ddd9d22`, `b318e7e`, `4836f0e`.

## Optional / follow-up

- [ ] Opt-in OTel/Sentry exporters (env-gated, scrubbed, sampled, default
      off; no prompts/keys/session ids). Needs a backend/DSN choice.
- [x] Local release dry-run (`scripts/release-dry-run.sh`): clean-tree
      refusal, counts/wiki/secret/compose gates, three-image build, no tag/
      push/sign.
- [ ] A containerized stdio MCP broker, if a deployment needs one; the
      supported local topology is host-spawned stdio.
