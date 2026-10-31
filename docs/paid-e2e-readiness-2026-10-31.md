# Paid E2E readiness report — 2026-10-31

## Status

A real local compose run was executed with one Atlas generation and one IBM
job. The stack is healthy; the paid run exposed blockers that must be fixed
before another paid attempt.

## Budget consumed

- Atlas: **1,500 / 9,000 credits** (one `images2PosedRGBD` generation).
- IBM: **2 QPU seconds** on `ibm_kingston`; the earlier `ibm_fez` job was
  canceled while queued with **0 vendor usage**.
- No further paid calls are permitted until the fixes below are validated.

## Confirmed passing

- Fresh compose stack, preflight, health/ready, console.
- Local simulator: 256 shots, counts `00:126`, `11:130`.
- Atlas operation `8edf2aaa6d6144468e7961534b6c3555`: `SUCCEEDED`, one
  113 KB image, `$1.20` reservation.
- IBM vendor job `dbj2lrpffo6c73b8qdj0`: `DONE` on `ibm_kingston`, 100 shots,
  2 QPU seconds, counts `00:44`, `11:55`, `10:1`, vendor `$3.20`.

## Blockers found and fixes staged

1. **Terminal hardware status never propagated to Go.**
   - Root cause: deferred hardware jobs are created `queued`; the bridge only
     polled jobs in `running`, and `queued -> completed` is an illegal
     transition.
   - Fix: poll queued hardware jobs, promote `queued -> running` on a terminal
     vendor poll, then settle. Added a deferred-status regression test.

2. **Cancel/terminal usage was missing from the status envelope.**
   - A queued `ibm_fez` cancel (vendor usage 0) kept an `$8.000568` charge.
   - Fix: terminal status now carries `usage_seconds`/`actual_usd` from the
     vendor, including a one-time poll for fenced cancelled jobs; Go can now
     release to measured zero. Python `cancel_job` also persists measured usage.

3. **Completed vendor job settled to the estimate, not actual.**
   - Vendor billed `$3.20`; local reservation was `$8.000568`.
   - Same missing-usage root cause as #2; the status envelope fix is staged.

4. **Atlas cached resolve dropped outputs.**
   - First resolve returned an asset URL; the cached resolve returned
     `asset_urls` absent and `prompt_used` empty.
   - Fix: resolve rows persist `asset_urls`/`local_paths`; cached reads enrich
     from the live Atlas operation; dispatch rows persist the requested prompt.

5. **Atlas images on GCS were refused by the proxy.**
   - Atlas returns signed `storage.googleapis.com/wlt-marble-outputs-prod/...`
     URLs; the proxy allowed only `worldlabs.ai`.
   - Fix: allow only that GCS bucket path (not all GCS) and re-validated the
     proxy: HTTP 200, `image/jpeg`, 115,618 bytes. Unit test added.

6. **Compose omitted Atlas and IBM credentials from kubonad.**
   - Atlas was disabled and IBM showed `credentialless` in fleet despite a
     valid token.
   - Fix: pass `KUBONA_ATLAS_ENABLED`, `WLT_API_KEY`, `WLT_API_BASE_URL`, and
     the IBM credential variables to kubonad.

7. **IBM calibration was stale at 84.7 h.**
   - Admission refused before dispatch.
   - Fix: refreshed `fidelity-devices.json` from the live `ibm_fez` properties;
     admission then passed at predicted fidelity 0.9606.

## Known test-data caveat

The first throwaway bridge store was manually transitioned during diagnosis,
so the local result for job `3a07f8b100954d9d` is not valid evidence. The
vendor job `dbj2lrpffo6c73b8qdj0` remains authoritative. The `kubona-e2e`
volume is disposable; use a fresh project for the next paid run.

## Required validation before another paid run

1. Run the bridge/Go terminal-status and cancel-settlement tests; pass
   `tests/test_execution_discipline.py`, `tests/test_qiskit_provider.py`,
   and the Go observatory tests.
2. Restart a **fresh** compose project with the staged fixes.
3. Verify **free** paths: preflight, login, local simulator, SSE replay,
   backup/restore, Atlas **cached resolve** (no new generation), and image
   proxy.
4. Only then run one controlled hardware validation:
   - one 2-qubit, 100-shot job on an empty-queue backend (`ibm_kingston`,
     `ibm_marrakesh`, or `ibm_fez` only when pending is low);
   - wait for terminal state through the normal status endpoint, not manual
     store writes;
   - assert vendor usage, local `usage_seconds`, settlement, and session/tenant
     spend all agree.
5. Do not run another Atlas generation unless the cached-resolve test fails
   for a code reason and a second generation is explicitly approved.

## Remaining product work after paid validation

- Reconcile the historical IBM row with operator-approved evidence.
- Reconcile the vendor runtime rate card.
- Finish schemas for the 68 unmodeled API functions.
- Contrast/screen-reader accessibility audit.
