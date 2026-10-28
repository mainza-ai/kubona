---
type: entity
name: IBM Quantum Platform
date: 2026-10-28
updated: 2026-10-28
tags: [provider, ibm, quantum, cloud, sdk]
sources: [source-architecture-paper, source-gap-analysis-plan]
status: tracked
---

# IBM Quantum Platform

IBM's cloud control plane and compute service for its QPU fleet, plus the Qiskit SDK and the
`qiskit-ibm-runtime` client. Kubona's `ibm-qiskit` provider targets
**IBM Quantum Compute Service** (formerly Qiskit Runtime). This page is the **live-document
hub**: facts here are a snapshot, and the linked IBM pages are authoritative.

## Canonical live documentation

Agents must re-check these links before asserting current IBM behavior, prices, backend names,
or limits. Do not hard-code a backend name, qubit count, gate set, or price from memory.

| Topic | Live URL |
|---|---|
| Guide index | https://quantum.cloud.ibm.com/docs/en/guides |
| Compute services overview | https://quantum.cloud.ibm.com/docs/en/guides/compute-services |
| Install the Compute client | https://quantum.cloud.ibm.com/docs/en/guides/install-qiskit-runtime |
| Initialize account / instances | https://quantum.cloud.ibm.com/docs/en/guides/initialize-account |
| Instance management | https://quantum.cloud.ibm.com/docs/en/guides/instances |
| Sampler quickstart | https://quantum.cloud.ibm.com/docs/en/guides/get-started-with-sampler |
| Sampler options | https://quantum.cloud.ibm.com/docs/en/guides/sampler-options |
| Transpilation / ISA | https://quantum.cloud.ibm.com/docs/en/guides/transpile |
| Job limits | https://quantum.cloud.ibm.com/docs/en/guides/job-limits |
| Maximum execution time | https://quantum.cloud.ibm.com/docs/en/guides/max-execution-time |
| Usage / cost | https://quantum.cloud.ibm.com/docs/en/guides/estimate-job-run-time · https://quantum.cloud.ibm.com/docs/en/guides/manage-cost |
| Monitor / cancel a job | https://quantum.cloud.ibm.com/docs/en/guides/monitor-job |
| QPU details / calibration | https://quantum.cloud.ibm.com/docs/en/guides/qpu-information · https://quantum.cloud.ibm.com/docs/en/guides/calibration-jobs |
| Retired QPUs | https://quantum.cloud.ibm.com/docs/en/guides/retired-qpus |
| Local testing mode | https://quantum.cloud.ibm.com/docs/en/guides/local-testing-mode |
| Error registry | https://quantum.cloud.ibm.com/docs/en/errors |

## Kubona integration snapshot (2026-10-28)

Provenance: live reads through `qiskit-ibm-runtime==0.50.0`; **no QPU job or spend** was
submitted. Re-verify before dispatch.

- Channel: `ibm_quantum_platform` (the legacy `ibm_quantum` channel is retired).
- Backend: `ibm_fez`, 156q Heron r2. `ibm_brisbane` is retired (2025-11-03). The provider
  falls back to `least_busy(operational=True, simulator=False)` if a contract name retires.
- Instance: the Open Plan `open-instance` CRN is supplied via
  `KUBONA_IBM_QISKIT_INSTANCE` (gitignored `.env`), avoiding auto-selection.
- Calibration: refreshed from `backend.properties()`; the single source is
  `platform/contracts/fidelity-devices.json` (T1/T2/errors/`calibrated_at`). Do not reuse
  numbers from this page after the timestamp goes stale.
- Rate card: `platform/kubona/schema/providers/ibm-qiskit.json` (per-second, provenance
  fields). Treat vendor pricing pages as authoritative.
- Execution safety: local ISA transpilation before Sampler; `max_execution_time` default
  600 s; local checks for 10M executions, 5M two-qubit gates, backend capacity, 1M shots.
- Open Plan: 10 QPU minutes per 28-day rolling window.

## First live validation (2026-10-28)

- Job `dbgva40te5oc738j0ui0`, backend `ibm_marrakesh`, 100 shots, reported QPU usage
  **2 seconds** (operator cap: 120 s; set via `KUBONA_IBM_MAX_EXECUTION_SECONDS`).
- Counts: `11:52, 10:44, 00:3, 01:1`.
- **Verdict: hardware path works, but this is not a clean Bell result.** The queue,
  transpilation, submission, execution, and result return are proven; the observed
  distribution is far from ideal (ideal ISA simulation: `00/11`). Qubit 1's readout error was
  ~15.7% at job time, but that alone does not explain 44% `10`; investigate layout/calibration
  before any fidelity claim. An earlier `ibm_fez` submission was canceled while queued with
  0 usage.
- No advantage or fidelity claim is made from this run. Total QPU spend: 2 seconds of the
  2-minute operator cap.

## Platform-governed dispatch (verified 2026-10-28)

The Go → Python-bridge → IBM path ran end-to-end. It is **not** a direct provider call:
the Go `/api/quantum/jobs` route enforces session/tenant/mandate/settlement before the
bridge calls the provider.

Sequence (bearer = `KUBONA_API_TOKEN`):

1. `POST /api/tenants` → create the lab; `POST /api/tenants/admin-key` (bearer) bootstraps
   its Ed25519 admin key.
2. `POST /api/sessions` (bearer, `tenant_id`, budget) → session id.
3. `job_hash = sha256("kubona-job-v1\ncircuit_run\n" + session_id + "\n" + qasm)`.
4. Sign the tenant-admin envelope over `mandate_sign` params
   `{job_hash, max_budget_usd, ttl_seconds}`; `POST /api/mandates/sign` mints the reviewer
   mandate. The params hash is Go `json.Marshal` bytes — integer-valued budgets must be
   encoded as integers (`5`, not `5.0`) or the envelope hash differs.
5. `POST /api/quantum/jobs` with `session_id`, `qasm`, `provider=ibm-qiskit`, `shots`,
   a unique `idempotency_key`, and `mandate_ref` = the signed mandate JSON.
6. Poll `GET /api/quantum/jobs/<id>`; the Go row is attributed from its own record, not
   the bridge response.

Prerequisites uncovered during the run: disable the Sovereign Air-Gap through
`/api/llm/settings` for a deliberate hardware dispatch; the bridge must receive the IBM
token/instance/backend/max-execution env (compose now passes them); and a hardware
mandate must be used even though a dry run succeeds without one.

**Observed run:** governed job `6631e996179d48bb` → IBM `dbh02i9t9uhs73c93fk0`,
`ibm_marrakesh`, 100 shots, **2 s usage**, counts `00:52, 11:40, 10:6, 01:2` (a noisy
Bell; no fidelity claim). The pre-dispatch estimate was `$8.000568` (5.000 s billed at
the $1.60/s card); the session/tenant books were reserved at that estimate.

**Gap found and fixed in code (2026-10-28):** completion previously kept the full estimate
reservation. The provider now returns `usage_seconds` and `actual_usd`; the Go completion
path CAS-settles `reserved - actual` back to tenant+session (full release when measured
usage is zero), and the job row records the measured amount. Covered by a Go test; the
already-created validation session was not retroactively adjusted.

**Anomaly investigation (same day):** the direct-run circuit and the governed QASM produce
the same ISA (initial layout q0/q1, `rz/sx/cz/measure`; ideal Aer `00/11`), so this is not a
conversion defect. The governed rerun on the same backend produced 92% `00/11`; the direct
run's 44% `|10>` was not reproduced. Job records do not retain the physical layout, so the
suspected bad-pair/readout cause cannot be proven from the completed jobs. Treat it as an
unresolved device/layout observation. The provider now records the logical→physical
transpiler layout in the job result/meta (and the bridge status payload), so the next
hardware run can distinguish layout/readout from gate errors.

## Agent rules

1. Read the linked IBM page before quoting a limit, price, backend, or API shape.
2. Resolve backends at runtime (`service.backends`/`least_busy`); never assume the name in a
   cached contract still exists.
3. Transpile to the backend's ISA locally; Sampler rejects non-ISA circuits.
4. Set an explicit instance and `max_execution_time`; never rely on the 3-hour service cap.
5. After any hardware result, record the backend, job id, usage, and calibration timestamp in
   the ledger. No advantage claim without a classical anchor.

Related: [[qiskit]] · [[qiskit-mcp-servers]] · [[ibm-quantum-composer]] ·
[[quantum-cloud-pricing]] · [[quantum-advantage-realism]] ·
[[production-readiness-audit-2026-10-26]]
