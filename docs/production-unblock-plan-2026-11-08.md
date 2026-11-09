# Production unblock plan — 2026-11-08

Status: investigation complete; implementation sequencing open. This page turns the
remaining "not complete" list into concrete blockers, decisions, and executable
slices. It is subordinate to
[`production-completion-plan-2026-11-08.md`](production-completion-plan-2026-11-08.md):
that page is the gap register, this page is the unblock plan.

Current baseline: `d016e41` + the 2026-11-09 PES/fleet slice (CI + Console E2E green on `develop` and `main`). See
[`production-gap-audit-2026-11-09.md`](production-gap-audit-2026-11-09.md) for the verified re-audit; earlier rows that said launch shots/seed, direct Krylov/mitigation, or claim review were open are stale.
The next slices are deliberately ordered so provenance and spend truth land before
more experiment surface.

## 1. What is actually blocked

### 1.1 Governed hardware VQE (blocked by missing experiment-level admission)

The circuit rail is not the problem. `POST /api/experiments/qaoa/hardware` already
loads a local QAOA circuit and submits it through `SubmitJob("circuit_run", ...)`;
`planAdmission` enforces mandate, jurisdiction, fidelity, reservation, and
settlement. The blocker is that VQE is not one circuit:

- A VQE energy is a sum of Pauli expectations over several measurement bases.
  Each basis is a separate physical circuit, and each needs its own QASM.
- A per-circuit mandate binds `circuit_run` to one QASM via
  `mandates.CanonicalJobBinding("circuit_run", sessionID, qasm)`. Asking one
  mandate to authorize N term circuits is not truthful; asking the operator for N
  separate mandate approvals is not a usable experiment loop.
- The local VQE result already carries `theta`, the Pauli coefficients, and the
  active-space/FCI anchors, but nothing turns that state into a set of
  measurement-basis circuits with a parent experiment identity.
- The Go worker currently knows `vqe_run` only as a simulator bridge call
  (`platform/core/pkg/jobs/jobs.go`, `RunExperiment`). It has no parent experiment
  row, no child circuit rows, no aggregate result, and no claim-evidence object.

The hardware diagnostics are therefore not evidence for a governed result:
they were run with a local optimizer and did not pass through a Go job,
reservation, settlement, parent experiment, or claim. They must stay labelled
`diagnostic` and must not be cited as production results.

**Unblock decision (recommended, revised after IBM Runtime research):** add a
parent `hardware_experiment` job type with one experiment-level spend mandate and
execute the term measurements as **one native Qiskit Runtime batch/PUB job**, not
N independent `circuit_run` jobs. IBM's current guidance is to prefer batch mode
for multi-job workloads, to combine sub-minute jobs rather than pay fixed
per-job overhead, and to batch parameter/observable evaluations into PUBs; Open
Plan cannot use sessions and has a 10-minute batch TTL. A parent mandate binds the
canonical experiment spec (molecule, geometry, ansatz hash, backend, shots, seed,
Pauli groups, worst-case cost), and the batch is the unit of vendor usage and
settlement. The parent owns the aggregate energy/uncertainty, vendor job id,
usage, actual USD, and claim evidence. No child circuit is separately mandated
or separately settled.

### 1.2 Full-space LiH/BeH2 accuracy (blocked by physics/resource budget, not by code)

The current `_run_full_space_vqe` path is not an accuracy path. Measured locally
on LiH at 1.5 Å (STO-3G, 12 qubits, full FCI `-7.882362 Ha`):

| ansatz | iterations | energy (Ha) | error vs FCI |
|---|---:|---:|---:|
| `EfficientSU2` layers=2 | 30 | `-5.227017` | `2655 mHa` |
| `EfficientSU2` layers=4 | 30 | `-4.406748` | `3476 mHa` |
| `EfficientSU2` layers=6 | 30 | `-6.023547` | `1859 mHa` |
| `EfficientSU2` layers=2 | 200 | `-5.478579` | `2404 mHa` |
| `EfficientSU2` layers=3 | 200 | `-6.203818` | `1679 mHa` |
| `EfficientSU2` layers=4 | 200 | `-5.906832` | `1976 mHa` |

More layers and more COBYLA iterations do not close the gap; this is an ansatz /
optimization-landscape problem. Full-space UCCSD is worse operationally: LiH is
12 qubits, 92 parameters, and one `Statevector.from_instruction` preparation took
about 203 seconds before any optimizer evaluation. That is why the old comment
records a >30-minute first pass.

The target (`1.6 mHa`) is not reachable with the current default path. The honest
unblock is a **resource-bounded ADAPT/qubit-ADAPT implementation with a declared
budget**, plus a hard result gate: if the target is not met, the result is
`full_space_status:"contested"` and `claimable:false`. Full-space accuracy must not
be a release blocker for the rest of the product; presenting it as accurate is.
BeH2 full-space should be declared unsupported until the same target is
demonstrated; active-space BeH2 remains supported and labelled.

**Unblock decision (implemented):** full-space uses real ADAPT-VQE with
explicit operator pools and a wall-clock/operator cap. The CEO pool is
implemented and selectable (`full_space_pool:"ceo"`) but measured to stall on
the LiH benchmark; the validated default is the fermionic UCCSD ADAPT pool.
`EfficientSU2` is removed from the full-space path. Claims require measured
delta, shot uncertainty, and exact statevector delta all <= 1.6 mHa. BeH2 is
supported as a job but remains contested until it reaches the target.

### 1.3 Mixed Python/Go workflow atomicity (blocked by split ownership)

`handleWorkflowRun` currently reserves a workflow ceiling in Go, calls the Python
`WorkflowRun` RPC, then mirrors each completed step and adopts completed jobs into
the Go book. Job-only workflows take the Go branch and are much closer to atomic,
but mixed workflows still have the original split-brain window:

- Python can complete a step and write its job before Go adopts it; the current
  Python `jobs.jsonl` reconciliation is recovery, not atomicity.
- A failed or cancelled step has no Go job row and no reservation/settlement path.
- A crash between Python completion and the Go adopt loop leaves a ghost; a retry
  can double-book.
- Hardware spend inside a mixed workflow has no parent/child ownership, so it
  cannot safely become a governed hardware experiment.

**Unblock decision (recommended, verified against durable-execution practice):**
finish the Go-owned workflow worker rather than extending adoption. Persist
`workflow_runs` and `workflow_steps` before dispatch; Go creates each
job/reservation and passes its id to the executor; add a single-step bridge RPC
for non-job tools; settle every terminal step through the existing terminal
funnel. This is the local SQLite equivalent of the durable-execution/outbox
model: transactional workflow enqueue is exactly-once, each external step is
at-least-once and must be idempotent, and recovery replays from the last
committed step. Do not add a Temporal/DBOS server for a one-host local install
unless the workflow surface outgrows a single process. Keep the Python
`jobs.jsonl` reconcile as a bounded repair path, not the design. If the Go worker
cannot land first, use a transactional outbox with a visible `repair_required`
state; do not call adoption atomic.

### 1.4 Safe default autoresearch sandbox (blocked by topology choice)

The default compose has no Docker socket, and `SandboxRunner` correctly refuses
when Docker is unavailable (`autoresearch unavailable`). The working profile
mounts `/var/run/docker.sock` into kubonad, adds group 0, and shares a scratch
path; that is root-equivalent if the harness or sandbox path is compromised.
Docker daemon filtering alone cannot make this safe: the daemon API permits host
binds, privileged mode, devices, and namespace changes.

**Unblock decision (revised after 2026 sandbox research):** a Docker socket
sidecar alone is not a security boundary. The 2026 consensus is that a
shared-kernel Docker/runc container is inadequate for model-generated code;
gVisor or a microVM is the boundary. For Kubona:

- **Supported production (Linux host):** `sandboxd` sidecar owns the daemon
  socket, launches the fixed harness image through **gVisor (`runsc`)** with
  `net:none`, read-only rootfs, writable scratch, non-root, seccomp, pids/memory/
  time caps. Its only API routes are `/health` and `/v1/run`; image, command,
  mounts, runtime, and network are fixed at boot, so bind/privileged/device/
  cap-add/host-network/image/runtime overrides are not expressible. kubonad
  gets only a private UDS, never the Docker socket. `compose.sandbox.yml`
  implements this topology; absent `runsc`, `/health` is 503 and runs refuse.
- **macOS local development:** Docker Desktop's Linux VM is the outer boundary,
  but a mounted socket can still reach shared host paths. Do not call this a
  hostile-code sandbox; either use a dedicated Linux VM with gVisor for
  autoresearch, or keep autoresearch explicitly unavailable. Apple's
  per-container VM is a promising macOS path but is not a default Compose
  dependency.
- **Trusted-host override:** keep `compose.sandbox.yml` only for an explicitly
  trusted lab; never silently fall back to host execution.

If the gVisor/Linux path is not available, the default must say
"autoresearch unavailable" and the feature must not be advertised as available.

### 1.5 At-rest, recovery, alerts, claim paging, manual accessibility

- **LLM provider keys:** already sealed with AES-256-GCM when
  `KUBONA_SECRET_KEY` is set. This slice adds preflight auditing and refuses new
  plaintext writes in production; existing plaintext rows must be re-saved.
- **Ledger:** still plaintext SQLite. `kubonad seal-secrets` now migrates
  existing plaintext provider credentials to the AES-GCM secretbox after
  `KUBONA_SECRET_KEY` is set; the ledger itself still relies on OS-level disk
  encryption. The recommended local-private posture is
  **OS-level full-disk encryption (FileVault/LUKS/BitLocker) plus encrypted
  `age` backups**, not SQLCipher by default: SQLCipher adds 5–15% write
  overhead, the Go driver is a maintained fork rather than the upstream
  `mattn/go-sqlite3` path, and a key stored next to the daemon does not protect a
  stolen host. SQLCipher becomes a separate requirement only if schema/metadata
  confidentiality inside an already-unlocked volume is required, with the key
  in the OS keychain/TPM and a tested `PRAGMA rekey` migration.
- **Backup/recovery:** verified backup, restore, retention, and `age`
  encryption exist. `kubonad offhost-drill` now performs the repeatable
  encrypted drill: verified backup, `age` encrypt to an operator-supplied
  off-host mount, decrypt that artifact, restore to scratch, compare
  `sessions/jobs/ledger_events/claim_records`, and print the artifact hash.
  The operator must supply the genuinely separate mount and identity file;
  the command cannot manufacture physical separation. Local scratch drill:
  `kubonad restore-drill`.
- **Alerts:** preflight/backup plus disk guard, settlement-skipped, and named
  rate-limit refusals route through `KUBONA_ALERT_WEBHOOK_URL`. Rate alerts
  are cooldown-limited once per minute per limiter and never change the
  admission decision.
- **Claim pagination:** API/UI paging is implemented with
  `offset`/`next_offset`/`has_more` and "Load older claims".
- **Manual VoiceOver/NVDA:** cannot be produced by code. Automated
  axe/keyboard/reduced-motion/forced-colors/200%-zoom evidence remains green;
  the manual script in `docs/manual-accessibility-2026-11-01.md` still needs a
  human run or an explicit residual-risk waiver.

### 1.6 Hardware diagnostics are not production results

There is no migration that makes a diagnostic a governed result. Re-running it
through the parent experiment rail is the only path. Until then, the diagnostic
job ids/energies are labelled external/diagnostic evidence and cannot mint a
claim. The production acceptance test must use a new parent job with a mandate,
child circuits, settled usage, and a claim.

## 2. Research basis (checked 2026-11-08)

These are the external practices that changed the recommendations above. URLs
are live references; vendor docs move, so pin the version/date when implementing.

### Hardware VQE and spend authority

- IBM Quantum execution modes: batch is the recommended mode for multi-job
  workloads; sessions are dedicated/expensive and **Open Plan cannot submit
  session jobs**; batch has a 10-minute TTL on Open Plan. Combine sub-minute jobs
  to avoid fixed per-job overhead. `https://quantum.cloud.ibm.com/docs/en/guides/choose-execution-mode`
  `https://quantum.cloud.ibm.com/docs/en/guides/run-jobs-batch`
- Qiskit Estimator accepts multiple circuit/observable PUBs in one job and
  returning one result per PUB; commuting observables in the same PUB share a
  measurement. `https://quantum.cloud.ibm.com/docs/en/guides/estimator-input-output`
  `https://quantum.cloud.ibm.com/docs/en/guides/estimator-examples`
- Job usage is available from `job.usage()`; failed/cancelled jobs can still
  count usage. `https://quantum.cloud.ibm.com/docs/guides/estimate-job-run-time`
- Bounded capability receipts (IETF draft, 2026) formalize what Kubona needs:
  one signed parent capability with scope, budget, expiry and holder commitment;
  child allocations are funded from the parent and their aggregate cannot exceed
  it; parent/child registration must be atomic or explicitly reconciled.
  `https://www.ietf.org/archive/id/draft-schrock-ep-bounded-capability-receipts-04.html`
  `https://www.ietf.org/archive/id/draft-saha-aadp-01.html`

**Kubona recommendation:** one `hardware_experiment` parent mandate, one Runtime
batch, one settlement, one claim evidence object. This is both cheaper and more
truthful than N circuit jobs or N approvals.

### Full-space chemistry

- CEO-ADAPT-VQE (npj Quantum Information 11, 86, 2025) is the current
  resource-efficient state of the art for exactly this size range: LiH (12
  qubits) and BeH2 (14 qubits). It reports large reductions versus GSD-ADAPT
  (LiH CNOT count 1,698→427; depth 1,374→110; measurement cost 3.8×10^6→1.5×10^4
  to reach chemical accuracy). `https://doi.org/10.1038/s41534-025-01039-4`
  `https://arxiv.org/abs/2407.08696`
- K-ADAPT-VQE (2025) independently shows operator chunking reduces quantum
  function evaluations for LiH/BeH2. `https://arxiv.org/abs/2506.09658`
- ADAPT still scales poorly with operator-pool size and global re-optimization;
  the 2026 literature is exploring learned circuit generation, not assuming
  vanilla ADAPT is a silver bullet.
  `https://arxiv.org/html/2607.22468v1`

**Kubona implementation:** ADAPT-VQE with explicit UCCSD and CEO pools behind
an operator/time budget; the UCCSD pool is the validated default, the CEO pool
is selectable and experimental. Do not use full-space `EfficientSU2` as an
accuracy claim. If the target is missed, keep full-space contested and
active-space supported. BeH2 full-space is measured but remains contested.

### Sandboxing

- 2026 field guidance is consistent: shared-kernel Docker/runc is not an
  adequate boundary for model-generated code; gVisor or a microVM is the
  boundary. gVisor has ~50 ms startup and 20–50% syscall/I/O overhead; Firecracker
  ~125 ms with KVM. `https://luiscardoso.dev/blog/sandboxes-for-ai`
  `https://vdf.ai/blog/sandboxing-ai-agents-code-execution-on-premises/`
  `https://iotdigitaltwinplm.com/ai-agent-sandbox-firecracker-vs-gvisor-vs-kata-2026/`
- Rootless/user namespaces reduce accidental privilege but do not remove kernel
  bugs; a Docker socket remains effectively root for that daemon.
  `https://luiscardoso.dev/blog/sandboxes-for-ai`
- gVisor is OCI-runtime compatible and fits Docker; Firecracker/Kata need KVM
  and more platform work. macOS cannot run gVisor/rootless Docker natively; use a
  Linux VM or an explicitly unavailable feature.
  `https://gvisor.dev/docs/tutorials/docker-in-gvisor/`

**Kubona recommendation:** Linux production = `sandboxd` + gVisor; macOS =
autoresearch unavailable (or dedicated Linux VM); trusted override only. Do not
market a root-socket profile as a sandbox.

### Durable workflows

- Durable execution/outbox practice: transactional enqueue is exactly-once;
  external steps are at-least-once and must be idempotent; recovery replays from
  the last committed step. DBOS documents transactional enqueue and exactly-once
  transactions; Temporal documents replay/durable execution.
  `https://docs.dbos.dev/python/examples/outbox`
  `https://docs.dbos.dev/golang/tutorials/transaction-tutorial`
  `https://docs.temporal.io/workflow-execution`
- For a single-host SQLite deployment, a DBOS/Temporal server is unnecessary
  operational weight; the same invariants can be implemented in the existing Go
  ledger/state database.

**Kubona recommendation:** Go-owned `workflow_runs`/`workflow_steps` journal with
transactional create, idempotent steps, and replay/repair; no external workflow
server in v1.

### Ledger at rest

- SQLCipher is the mature full-page encrypted SQLite option, but adds 5–15% write
  overhead and needs key custody/rotation/migration; Go requires a maintained
  SQLCipher fork because upstream `mattn/go-sqlite3` has no encryption support.
  `https://www.zetetic.net/sqlcipher/comparison`
  `https://github.com/mattn/go-sqlite3/issues/452`
  `https://pkg.go.dev/github.com/omnilium/go-sqlcipher`
- OS full-disk encryption is the baseline defense for device theft; backups must
  be encrypted and restore-tested. `https://blackhawk.sh/en/blog/best-practices-for-securing-sqlite`
  `https://turso.tech/blog/sqlite-encryption-in-production`

**Kubona recommendation:** OS-level full-disk encryption + encrypted `age`
backups + owner-only permissions are the v1 requirement; SQLCipher is an opt-in
post-1.0 enhancement only when metadata confidentiality inside an unlocked
volume is required.

## 3. Implementation sequence

### Phase A — truth and provenance (now)

Deliverables:

1. Claim pagination API/UI (this slice).
2. Production secret-at-rest preflight and production plaintext-write refusal
   (this slice).
3. Full-space result status (`full_space_status`, `claimable`) and no
   `chemical_accuracy:true` without a passing uncertainty gate (this slice).
4. Hardware diagnostics remain explicitly external; add no claim/import path.
5. Add contract/E2E coverage for claim paging and the full-space status.

Exit: paginated claims E2E; production preflight fails a plaintext stored key;
full-space result cannot claim accuracy without the gate.

### Phase B — parent hardware experiment rail (P0)

Deliverables:

1. New job type `hardware_experiment` and canonical experiment mandate binding
   (molecule, geometry, ansatz hash, backend, shots, seed, Pauli groups, worst-case
   cost). The mandate is a bounded parent capability: child groups draw from its
   committed budget and cannot exceed it.
2. CPU-only bridge helper to prepare the ansatz and measurement PUBs from a local
   VQE result; no vendor spend. Prefer Qiskit Estimator PUBs for grouped
   observables and Sampler PUBs only for the final counts; do not invent a
   per-termination job.
3. One Runtime **Batch** (Open Plan compatible; no session) carrying the prepared
   PUBs, with the parent job recording the vendor job id and one usage reading.
   Parent result: `energy`, `uncertainty`, `measurements`, `hardware:true`,
   `backend`, `vendor_job_id`, `usage_seconds`, `actual_usd`, `plan`, and
   `simulated_optimizer:true` when parameters were optimized locally.
4. One settlement from the parent result; cancellation recalls the vendor job;
   crash recovery reads the parent row and vendor status, never re-dispatches a
   terminal job.
5. A mock-provider test lane and one opt-in live H2 VQE run; then QAOA via the
   same parent contract (the current QAOA endpoint becomes a parent special case
   or is replaced).
6. Claim evidence links the parent hardware job id; no claim from a diagnostic.
   The old per-term diagnostic ids are external evidence only.

Exit: one real hardware VQE and one QAOA end-to-end with parent job id, claim,
settled ledger; or UI says `hardware experiment unavailable` and no fake result.

### Phase C — Go-owned workflow lifecycle (P0/P1)

Deliverables:

1. `workflow_runs` / `workflow_steps` schema and transactional create.
2. Go executor for all steps: job steps via `SubmitJob`, non-job steps via a new
   single-step bridge RPC; step idempotency key passed to Python.
3. Persist each terminal step before advancing; reservations/settlement on the
   Go rail; a failed step leaves a visible failed run with no completed claim.
4. Boot recovery: stale running rows become `repair_required` or resume
   idempotently; never silently adopt a Python completion without a step row.
5. Crash/retry test: kill after Python completion, restart, no ghost/double-book;
   hardware step failure releases the reservation.

Exit: Python completion without a Go step is refused; a crash is recoverable with
no double-spend and no ghost.

### Phase D — scientific completeness (P1, opt-in)

Deliverables:

1. CEO-ADAPT/qubit-ADAPT full-space mode with a wall-clock/parameter cap and a
   declared target; no default full-space run.
2. LiH/BeH2 PES, repeat variance, and a hardware-vs-simulator-vs-exact view.
3. CI95/bootstrap result uncertainty; no `claimable` without measured error.
4. If ADAPT misses target: `full_space_status:"contested"`, UI warning, and
   active-space remains the supported scientific surface. BeH2 full-space is
   `unsupported` until demonstrated.

Exit: declared supported/unsupported boundary per molecule; no false accuracy.

### Phase E — sandbox/security (P1)

Deliverables:

1. `sandboxd` sidecar + gVisor `runsc` runtime + UDS client; default compose uses
   it on a supported Linux host, kubonad has no socket.
2. Runtime request allow-list and malicious-request tests (bind/privileged/
   device/host-network/cap-add/image/runtime override all refuse).
3. Startup preflight reports `sandbox: available|unavailable`; default cannot
   execute on the host. macOS default is unavailable unless a Linux VM is
   supplied.
4. Keep `compose.sandbox.yml` only as the explicit trusted-host override.

Exit: default autoresearch either works through the narrow sidecar or is
honestly unavailable; no root socket in kubonad.

### Phase F — operations, recovery, accessibility (P1/P2)

Deliverables:

1. Off-host encrypted backup destination + OS-level encrypted volume + documented
   key rotation. `kubonad offhost-drill` now executes the encrypted copy →
   decrypt → restore → count-comparison path; the operator must supply the
   separate mount and identity. `kubonad restore-drill` remains the local
   scratch proof.
2. Alert routing for disk/settlement/rate-limit is wired through
   `KUBONA_ALERT_WEBHOOK_URL` (preflight/backup plus disk_guard,
   settlement_skipped, and cooldown-limited rate_limit events).
3. Manual VoiceOver/NVDA run on the shipped commit, recorded per surface
   (human-only).
4. Claim/results browser E2E (paging, review, PES table/CSV, full-space status,
   workflow adoption). Paging and full-space have unit/API coverage; browser
   coverage remains a release gate.
5. Fresh-volume release acceptance and restore drill on the exact commit.

Exit: install/backup/restore/upgrade/operate one-person runbook without hidden
shell steps; manual evidence or explicit waiver recorded.

## 4. Current slice (landed here)

- Claim pagination: `ListClaimRecordsPage`, `offset`/`next_offset`/`has_more`,
  `fetchClaimsPage`, "Load older claims".
- Secret posture: `llm.AuditSecretsAtRest`, production refusal of new plaintext
  provider-key writes, `kubonad preflight` audit.
- Full-space implementation: `platform/bridge/adapt.py` now runs real
  ADAPT-VQE with explicit UCCSD and CEO pools, exact gradients, monotonic
  optimization, operator/time caps, and shot-measured Pauli groups.
  `chemical_accuracy`/`claimable` require measured delta, shot uncertainty,
  and exact statevector delta all <= 1.6 mHa. LiH (12 qubits) reached
  0.706 mHa measured / 0.213 mHa exact in the validated run; BeH2 remains
  resource-contested and is not claimed. See
  `docs/full-space-chemistry-2026-11-08.md`.
- Approval/workflow: agent-created and approval-required workflows now use
  a signed, single-use approval token (`POST /api/workflows/approve`) and the
  Go mixed-workflow rail; the Python compatibility engine is no longer the
  path for approvals.
- Sandbox: `sandboxd` + gVisor `runsc` topology in `compose.sandbox.yml`;
  kubonad has no Docker socket, and `/health` reports unavailable without
  `runsc`.
- Hardware VQE parent rail (H2 first):
  - `hardware_experiment` job type with a parent-only mandate binding
    (`CanonicalHardwareExperimentBinding`), positive hardware cost, jurisdictions,
    and fidelity admission.
  - `POST /api/experiments/vqe/hardware/prepare` reads a local H2 VQE job and
    returns the canonical parent mandate binding; it spends nothing.
  - `POST /api/experiments/vqe/hardware` consumes the completed local H2 VQE
    job's `theta`, Pauli coefficients, FCI anchor, and `ansatz_qasm`; it never
    dispatches the bridge directly.
  - The bridge prepares all measurement-basis circuits and the provider submits
    them as **one Runtime batch job**; one parent result carries `hardware_job_id`,
    `usage_seconds`, `actual_usd`, aggregate energy/uncertainty, and
    `simulated_optimizer:true`.
  - The parent result folds into the campaign/claim path; hardware claims require
    measured uncertainty inside the accuracy limit.
  - Mock/test lanes cover admission, one-batch preparation, and settlement. A live
    Hardware VQE run still requires an operator mandate/backend and is not yet
    claimed as evidence.
- Durable workflow journal (Phase C first slice):
  - `workflow_runs` and `workflow_steps` are persisted before dispatch; job-only
    workflows create a step row before submission and advance it only after the
    terminal job result.
  - Mixed workflows get a run row before the Python engine call and step results
    are mirrored afterward; interrupted runs become `repair_required` on boot.
  - `GET /api/sessions/{id}/workflows` exposes the journal; no run is presented
    as completed without a terminal receipt.
  - Normal mixed workflows are Go-owned: job steps use the Go jobs rail,
    non-job steps use the single-tool `RunWorkflowStep` RPC, and compensations
    are recorded best-effort.
- Investigation evidence and benchmark numbers recorded above.

- **Live governed hardware evidence (2026-11-08):** one H2 VQE parent and one
  QAOA parent completed end-to-end through the new rails, with operator-signed
  parent mandates, Go jobs, vendor batch ids, settled usage, and minted claims:
  - H2: Go job `d1b6bbcd-dff6-440b-bee8-06cd13233c6a`, vendor
    `dbofapoqtr0c738cpc40`, `ibm_marrakesh`, 1024 shots. Hardware energy
    `-1.1269008956 Ha` vs FCI `-1.1372701747 Ha` → `10.3693 mHa`; measured
    uncertainty `5.9767 mHa`; `$0.00`, `0.0 s` reported usage; claim
    `clm-36f4e358-0002` **contested** (not chemical accuracy). The optimizer was
    local (`simulated_optimizer:true`); only the final state was hardware.
  - QAOA: Go job `135a5978-b30d-49b8-92e2-0dc39cae4b84`, vendor
    `dbofeq44148s739sv130`, `ibm_marrakesh`, 1024 shots. Hardware cut
    `7.9521484` vs exact optimum `10`, ratio `0.7952148`; local exact cut
    `8.9070977`; `$0.00`, `0.0 s`; claim `clm-577501f3-0001` **contested**.
  - Both are single-parent, single-vendor-batch results. They supersede the old
    per-term diagnostics for provenance, but are **not** claims of chemical
    accuracy or quantum advantage.

Phase B update: QAOA now has a prepare route symmetric with VQE
(`/api/experiments/qaoa/hardware/prepare`) and the provider batch-path test is
green. Live H2 and QAOA parent results are recorded above.

Phase C update: normal mixed workflows are now Go-owned. `execution/submit_job`
steps use the Go jobs rail; every other tool executes through the new
`RunWorkflowStep` RPC, one tool per call. The Go orchestrator owns sequencing,
per-step budget checks, journal rows, terminal settlement, and best-effort
compensation records. Approval-required and agent-created workflows deliberately
fall back to the Python compatibility engine.

Still open in Phase C: approval/agent workflow migration, full crash/retry
conformance for mid-step RPC loss, and typed inverse compensation for hardware
jobs (the journal records unsupported job compensation rather than inventing it).

## 5. Decisions required before Phase B/E

1. Hardware mandate model: one experiment-level parent mandate + one Runtime
   batch (recommended) versus per-circuit approval (unusable at VQE scale).
2. Full-space acceptance: fund CEO-ADAPT target/time budget, or declare
   full-space contested/unsupported and release active-space only.
3. Sandbox: Linux `sandboxd` + gVisor (recommended); macOS default stays
   unavailable unless a dedicated Linux VM is supplied.
4. Ledger at rest: OS-level encrypted volume + encrypted backups (recommended);
   SQLCipher only if metadata confidentiality inside an unlocked volume is a
   requirement.

Until those decisions land, do not claim the corresponding feature is complete.
