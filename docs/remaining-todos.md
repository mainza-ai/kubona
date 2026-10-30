# Remaining execution TODOs

Updated 2026-10-30. This is the short execution list; the full queue lives in
`wiki/concepts/remaining-work.md`.

## Production blockers

- [ ] **Resolve the 606 pre-existing ledger chain faults** in
      `~/.kubona/kubona.db` before trusting a restore. The 2026-10-30
      quarantine snapshot is SQLite-consistent but `kubonad backup` refused
      it as a verified provenance backup. Do not treat that snapshot as a
      chain-clean restore source.
- [ ] **Vendor rate-card reconciliation before 2026-11-11.** Follow
      `docs/rate-cards.md` §Reconciliation checklist. Needs the vendor price
      sheets/invoices and Atlas-lens pricing; not a repo-only change.
- [ ] **Nightly thesis-scale green.** Current red runs are runner shutdowns,
      not assertions. Run `slice60-composition` and `slice60-determinism` on
      a runner that can hold the memory/time, record the run IDs, and do not
      weaken the tests. If hosted runners cannot, move to a larger or
      self-hosted runner.
- [ ] **Attach the paid QPU run evidence.** The operator confirmed the first
      paid run on 2026-10-28. Attach the deployment ledger/run id, backend,
      shots, and settlement; the local `~/.kubona/kubona.db` only contains
      the 2026-10-29 free-plan `ibm_fez` probe.
- [x] **Host LLM default decided 2026-10-30:** the host's persisted
      `active=nvidia`, `air_gap=off` is treated as an intentional development
      override (NVIDIA keys are present); it is **not** the shipped default.
      The production compose/fresh volume defaults to Strata, and the console
      LLM Settings UI is authoritative per user. Cloud in production requires
      `KUBONA_LLM_CLOUD=1`; local providers (Strata/oMLX/Ollama/LM Studio)
      do not. Switch the host to Strata + air-gap in the UI if that is the
      desired dev default.
- [x] **Commit and push.** `ddd9d22` on `develop` and `main`; CI 20/20 green
      on both (runs `40995438061` and `40995448632`).

## Optional / follow-up

- [ ] Opt-in OTel/Sentry exporters (env-gated, scrubbed, sampled, default
      off; no prompts/keys/session ids). Needs a backend/DSN choice.
- [ ] Local release dry-run/checksum script (build images + SBOM/checksums,
      verify the tag workflow without publishing).
- [ ] A containerized stdio MCP broker, if a deployment needs one; the
      supported local topology is host-spawned stdio.
