# Repository hygiene

## Console dead exports

`scripts/console-dead-exports.py` runs in the console CI job. It fails when
`platform/console/src/lib/api.ts` gains a function export with no non-test
consumer, and it also fails when an allowlisted export becomes referenced
(remove the entry then).

The known deferred entries are deliberate:

- `resumeTribunal`: the live path is `runTribunal`; the resume UI is not mounted.
- `evaluateDiscoveryPipeline`: discovery curation UI deferred.
- `fetch/start/step/pause/resume/stopAutoresearch*`: operator UI deferred by
  `wiki/concepts/remaining-work.md`.
- `planWorkflow` / `runWorkflow`: the Go endpoints are agent-facing; no
  console canvas yet (tracked as C4).
- `requestServerSignedMandate`: the console uses client-minted mandates;
  server-signed stays daemon/CLI-side.
- `resetDedupForTests`: test-only helper.

Deleting or wiring an entry is preferred to extending the list.

## Large tracked assets

These are intentional, not build output:

- `assets/misc/Kubona_Quantum_AI_Discovery_Platform.pdf` (18.8 MB)
- `assets/misc/Kubona_Platform_Overview_Infographic.png` (5.1 MB)
- `platform/console/public/assets/images/Project_Chicken_Soup_Quantum_AI-slide-deck/` (~10 MB)

If clone size becomes a problem, move the PDF and slide deck to release
assets or Git LFS. Do not commit build output: `platform/core/bin/`,
`platform/core/kubonad`, `platform/crates/*/target/`, and
`platform/console/.next/` are gitignored.

## Coverage

Go, Python, and console have hold-the-line coverage floors in CI. At the
2026-10-29 snapshot: Go 64%, Python 55%, console measured 54.5/55.3/52.6/57.1
(statements/branches/functions/lines) with floors 53/54/51/56. The console
floor uses `@vitest/coverage-v8` and is enforced by `pnpm test:coverage`;
raise it after a sustained green run, never lower it to make a red pass.
