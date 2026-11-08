# Production Completion Report and Plan — Local-Private Deployment

**Date:** 2026-11-08  
**Current code head:** `bb75068` (docs-only commit; functional code `22d42cb`)  
**Scope:** fully functional, unattended local-private deployment on one trusted host.
Not a public SaaS, not a multi-tenant hosted service, not a demo.

## 1. Executive verdict

The local-private stack is operational: the compose stack boots on a fresh volume,
loopback ports are enforced, the ledger and backup/restore rails work, the H2
VQE/PES simulator experiments produce measured results, the QPU Bell
characterization completed, and CI plus Console E2E are green on the current
head.

It is **not yet complete for the full product promise**. The remaining gaps are
not cosmetic. They are concentrated in five trust boundaries:

1. **Experiment completeness** — the supported scientific portfolio stops at
   H2 simulator VQE/PES, active-space LiH/BeH2, and simulator QAOA. There is no
   production path for a hardware-VQE loop, hardware QAOA, full-space chemical
   accuracy, error-mitigated experiments, or PES for LiH/BeH2.
2. **Workflow ownership** — a Python workflow step is adopted into Go after it
   completes. That removed the visible ghost, but it is not an atomic job
   lifecycle; failed/cancelled/hardware-spend paths can still diverge.
3. **Reproducibility and result truth** — the legacy launch path still hardcodes
   1024 shots and no seed; direct Krylov/mitigation/QIR routes bypass the job and
   claim ledger; the result object is split across job JSON, campaign state, and
   notebook text.
4. **Sandbox availability** — autoresearch is correctly fail-closed by default,
   but it is not usable in the default compose shape. Enabling it requires a
   root-equivalent Docker socket.
5. **Review and operations closure** — claims have history and a peer-review
   request event, but no reviewer decision/reopen state; accessibility manual
   testing, off-host backup, alert routing, and at-rest secret/ledger posture
   remain incomplete.

**Production verdict:** suitable for a controlled local-private run of the
supported simulator/H2 path with an operator who follows the runbook. Not yet
ready to be sold or operated as the complete Kubona platform without explicit
feature limitations in the UI and contract.

## 2. Scope and threat model

### Supported deployment shape

- One operator, one host (or a private SSH/WireGuard tunnel).
- Console `127.0.0.1:3000`; API `127.0.0.1:8080`; no public bind.
- Go `kubonad` is the money and job authority.
- Python bridge is the only component that reaches a QPU vendor.
- Local simulator works offline; local LLM is optional but must be running for
  LLM features.
- QPU and cloud LLM are optional, explicitly configured, and fail closed.

### Explicitly out of scope

- Public ingress, internet-facing auth, multi-tenant SaaS, HA/multi-host.
- A2A/x402 settlement, marketplace sales, public plugin supply chain.
- Public cloud fleet breadth beyond the providers actually declared executable.
- Multi-user role separation unless the deployment declares multi-lab mode.

### Security assumptions that must be stated in the product UI

- The host is trusted and physically controlled; `.env` is 0600.
- The supported sandbox override is root-equivalent and must never be exposed
  to untrusted users.
- The local SQLite ledger is plaintext unless the operator supplies disk
  encryption; `KUBONA_SECRET_KEY` seals UI-entered LLM keys, not the ledger.
- The local open mode is anonymous *for local chat/campaign creation only*; it
  never authorizes cloud spend, QPU dispatch, settings, or mandates.

## 3. Evidence at the current revision

| Evidence | Current state |
|---|---|
| CI | 20/20 green on `develop` and `main` |
| Console E2E | green on both branches |
| Local compose | fresh-volume boot, loopback-only ports, health/ready, console |
| Backup/restore | `kubonad restore` and scheduled backup profile implemented and tested |
| H2 VQE/PES | measured shot energies, FCI anchor, claim, structured campaign fold |
| QPU | completed Bell/characterization job; no hardware VQE/QAOA result |
| LiH/BeH2 | active-space VQE measured; full-FCI gap contested (20.53 / 36.30 mHa) |
| Full-space LiH smoke | 12 qubits, `EfficientSU2`, poor full-FCI result (contested) |
| QAOA | 8-node 3-regular MaxCut simulator; optimized QASM returned |
| Workflow | completed Python job adopted into Go jobs book; non-atomic |
| Claims | history + `peer_review` request event; no decision/reopen state |
| Autoresearch | opt-in Docker socket only; default `available:false` |
| Accessibility | automated axe/keyboard/reduced-motion green; manual VoiceOver/NVDA not run |
| Hardware QPU QAOA | diagnostic submission queued then cancelled; no result claimed |

This evidence is reproducible from the repository and the local compose stack.
It does not upgrade the missing portfolio features.

## 4. Gap register

### P0 — blocks calling the full local-private product "complete"

| ID | Gap | Why it matters | Evidence |
|---|---|---|---|
| PC-01 | **Workflow lifecycle is not atomic.** Python executes the step; Go adopts only completed results after the fact. Failed/cancelled steps, and hardware spend inside a workflow, can diverge. | A crash between Python completion and Go adoption leaves a ghost or double-book; a failed hardware step has no Go settlement. | `platform/core/pkg/api/workflows.go:262-292`; `platform/core/pkg/jobs/jobs.go:489-522` |
| PC-02 | **No hardware VQE or integrated hardware QAOA.** VQE/PES/QAOA runners are Aer only. | The product promise includes real QPU experiments; a hardware Bell run is not a VQE loop. | `platform/bridge/experiments.py:161-259`, `:420-630`; `run_qaoa` has no provider path. |
| PC-03 | **Full-space VQE is not scientifically usable.** `full_space:true` uses `EfficientSU2`; the LiH smoke result is far from full FCI. | It cannot support "full molecule" chemical-accuracy claims and must not be presented as a completed feature. | `platform/bridge/experiments.py:259-350`; live LiH smoke: `-5.1880 Ha` vs FCI `-7.8823 Ha` (2694 mHa). |
| PC-04 | **Launch reproducibility is broken.** `handleChatLaunch` hardcodes `shots:1024` and does not accept/record a seed. | The canonical proposal-launch path cannot reproduce a result from its spec. | `platform/core/pkg/api/server.go:2909-2919` |
| PC-05 | **Direct experiment routes bypass the job/claim ledger.** Krylov, mitigation, and QIR routes call the bridge directly and return a response. | Results from those routes cannot be cited, cost-settled, or folded into a claim by the same book. | `platform/core/pkg/api/server.go:5564-5742` |
| PC-06 | **Autoresearch is unavailable in the shipped default.** The safe default has no Docker socket; the working profile is root-equivalent. | One of the platform's advertised autonomous capabilities is not functional on a stock local install without a security downgrade. | `platform/core/pkg/api/server.go:4790-4804`; `platform/docker/compose.prod.yml` (no socket); `platform/docker/compose.sandbox.yml` |
| PC-07 | **Claims cannot be formally reviewed/reopened.** `peer_review` is a request event; the claim record is immutable and has no decision state. | A researcher cannot accept, contest, or reopen a claim through the product; history is read-only. | `platform/console/src/components/common/ClaimHistory.tsx:37-58`; `claim_records` in `platform/core/pkg/ledger/ledger.go` |
| PC-08 | **No versioned experiment-result contract.** Result truth is split among job JSON, campaign `measurement`/`pes_points`, notebook text, and UI prose. | Hardware metadata, counts, layout, calibration, usage, cost, per-point evidence, and CIs cannot be guaranteed across rails. | `platform/core/pkg/agents/campaign_synth.go:128-180`; bridge result shape; `ResultsStudio` projections |
| PC-09 | **No PES/experiment path for LiH/BeH2, and no error-mitigation integration.** PES is H2-only; VQE/PES do not run ZNE/PEC. | The chemistry portfolio and "measured, mitigated result" story remain partial. | `platform/bridge/experiments.py:513+`; mitigation is a separate route. |
| PC-10 | **Secrets/at-rest posture is not end-to-end.** `.env` is the supported secret store; the ledger is plaintext; Docker secrets/OpenBao are optional. | A lost host or backup exposes ledger/API material; "production local-private" must state and test this boundary. | `docs/local-deployment.md:250-260`; `docs/production-readiness-plan-2026-10-31.md` P1-3 |
| PC-11 | **Recovery is implemented but not operationally closed.** Off-host backup/restore drills and migration rollback are not proven as a routine release gate. | A local production host needs a repeatable disaster-recovery proof, not only unit tests. | Backup profile; restore docs; current CI has backup smoke, not a full restore drill. |

### P1 — required for a complete researcher workflow

| ID | Gap | Improvement |
|---|---|---|
| PC-12 | Result schema lacks a stable `result_version`, CI95, counts, backend layout, calibration, usage, cost, per-point evidence, and signed per-result receipt. | Define one typed `ExperimentResult` shared by bridge, Go, campaign, API, export, and UI. |
| PC-13 | Result UI still lacks PES error bars/FCI curve in the chart, a full optimizer trajectory view, repeat-run variance, and direct claim/job links in every card. | Comparison/diagnostic surface; chart + table + JSON/CSV. |
| PC-14 | Claim review is a request prompt, not a decision. | Add reviewer decision state (`accepted`, `contested`, `reopened`) as an append-only review record; never mutate the measurement. |
| PC-15 | Non-executable providers are visible in the settings vocabulary. | Hide or clearly gate `planned`/`not integrated` providers; make `executable_now` the primary action state. |
| PC-16 | `GenerativeUIStudio`, `WidgetStage`, legacy `SpatialCanvas`, and some illustrative widgets remain in the shipped console. | Decide and document: keep as explicit proposal-only tools, or remove from the production build; no demo surface may look measured. |
| PC-17 | Manual VoiceOver/NVDA pass is still pending. | Execute the script in `docs/manual-accessibility-2026-11-01.md` on the shipped commit; record OS/AT/version per surface. |
| PC-18 | Observability is local-only; external alert routing is limited to preflight/backup; no disk/rate-specific routing; no OTel/Sentry by default. | Add opt-in scrub/sampling exporters and local alert routing policy. |
| PC-19 | Launch/workflow and result surfaces lack full browser E2E coverage for claim history, PES table/CSV, full-space, and workflow adoption. | Extend E2E and contract tests; keep hardware tests opt-in. |
| PC-20 | Role separation and first-run onboarding remain incomplete for multi-lab mode. | Document single-operator as the only complete mode, or finish researcher/admin split. |
| PC-21 | Design-system raw-palette debt is still ~854 occurrences. | Migrate to shared primitives; keep the ratchet, do not lower the bar. |
| PC-22 | A2A/x402, pgvector, GQE learned proposer, and provider breadth are dormant/partial. | Keep out of local-private scope until explicitly activated; never advertise as live. |

### P2 — enhancements, not local-private blockers

- Off-host backup destination automation and retention policy.
- Containerized stdio MCP broker.
- Full QASM 3/MLIR lowering beyond the supported subset.
- Multi-host/HA operation.
- Public plugin marketplace.
- QPU provider breadth beyond IBM.
- Learned GQE transformer/Unsloth and pgvector memory profile.
- Perf/load baselines and chaos testing.

## 5. Detailed analysis

### 5.1 Experiment completeness

The current bridge can run:
- H2 VQE with a hardware-efficient ansatz, exact FCI anchor, shot measurement;
- H2 PES;
- LiH/BeH2 active-space UCCSD VQE;
- LiH/BeH2 full-space `EfficientSU2` (a smoke path, not an accuracy path);
- QAOA MaxCut on a fixed 8-node graph, simulator only.

What is missing:
1. **Hardware VQE**. A correct hardware loop needs per-iteration circuits,
   observable grouping, shots, layout/calibration, budget admission, vendor
   status polling, cancellation, and settlement. The current `run_vqe` never
   selects a provider. The QPU rail can run arbitrary QASM jobs, but there is no
   optimiser that submits the sequence and folds the measured energy.
2. **Hardware QAOA**. `run_qaoa` returns `circuit_qasm` but always measures on
   Aer. A hardware QAOA needs a deliberate "optimise locally, execute final
   state on QPU" mode with explicit cost/approval and a counts-to-cut estimator.
3. **Full-space chemical accuracy**. The full-space LiH smoke is 2694 mHa from
   FCI; it is not a small tolerance issue. A production answer needs a
   tractable ansatz (ADAPT/qubit-ADAPT or a resource-bounded UCCSD) and a defined
   acceptance target. Until then, the UI must keep active-space and full-space
   claims separate and contested.
4. **Experiment families**. No LiH/BeH2 PES; no H2O; no error-mitigated VQE;
   no repeat-run variance; no hardware VQE/QPU comparison.
5. **Reproducibility**. `handleChatLaunch` unconditionally writes `shots:1024`
   and no seed (`server.go:2909-2919`); the proposal object has no spec field
   for these. The experiment API does accept a spec, but the canonical launch
   path is still not reproducible.

### 5.2 Workflow and ledger

The current adoption is a useful repair, not a completed ownership model:
- Python executes `execution/submit_job`, creates a Python job, and returns the
  result.
- Go only writes a job row after the workflow response, and only for
  `status=="completed"` with a result body.
- Go marks the adopted row `completed` with zero reservation; no settlement or
  cost reconciliation runs.
- No failed/cancelled adoption path exists.
- A crash between Python completion and the Go adoption loop leaves a result in
  the Python ledger and a ghost in Go.

A complete local design needs one of:
- **A. Go-owned workflow worker** (recommended): persist workflow + step rows
  before execution; create each job/reservation in Go; pass the job id to
  Python; Python returns a result by job id; Go settles. Non-job tools go
  through the same outbox.
- **B. Go/Python transactional outbox**: Python writes an outbox row in the same
  store as the job; Go consumes it idempotently; a repair command reconciles
  orphan Python jobs and stuck Go rows. This is less invasive but still needs
  reconciliation and a visible `repair_required` state.

The existing `AdoptExternalJob` (`jobs.go:489-522`) should be extended for
failed/cancelled/adopted-with-spend only as a bridge, not as the final design.
No workflow may claim a completed audit while a step is absent from the Go
book.

### 5.3 Result truth and reproducibility

The H2 result is real and shot-measured, but the result truth is spread across:
- bridge job `result_json` (counts, measurements, trajectory, backend),
- campaign `MeasurementSummary`/`PesPoints`,
- notebook strings and manuscript prose,
- UI projections.

The new `MeasurementSummary` is a good start but is not a versioned contract:
no `result_version`, CI95, counts, layout, calibration, usage, cost, or
per-point evidence. The `ResultsStudio` exports JSON/CSV and a PES table, but
the chart still lacks per-point error bars, and the manuscript is a projection
of notebook text rather than a generated view of the structured result.

The direct Krylov/mitigation/QIR routes are a separate trust boundary:
`handleKrylovSolve` and `handleMitigationApply` call the bridge and return JSON
without creating a job, spend reservation, claim, or result timeline
(`server.go:5564-5742`). That is unacceptable for any result a researcher may
cite.

### 5.4 Sandbox and autoresearch

The default compose has no Docker socket, and the Go `SandboxRunner` refuses
when Docker is unavailable (`autoresearch/harness.go:61-73`). The opt-in
`compose.sandbox.yml` mounts `/var/run/docker.sock`, adds group `0`, and shares
`/tmp/kubona-sandbox`. This is correctly documented as root-equivalent, but it
means the default install cannot run the advertised autoresearch loops.

A complete local-private approach must choose:
- a **narrow sandbox sidecar** (UDS API, rootless runtime) so kubonad never
  controls the host daemon; or
- a **rootless Podman/gVisor runtime** with a documented, tested profile; or
- explicitly scope autoresearch to a host-run "trusted lab" profile in the UI
  and release notes, with no pretence that the stock compose supports it.

The current fail-closed behavior is correct; the missing functionality is a
default path.

### 5.5 Claim review and provenance

`ClaimRecord` is immutable by design, which is correct for the measurement.
`ClaimHistory` adds a `peer_review` event and a prompt, but a review has no
durable outcome and cannot change the claim projection. A complete design adds
an append-only review record with `claim_id`, reviewer identity/key, decision,
reason, evidence refs, timestamp, and a current-state projection. The claim
record itself remains immutable; the decision is separate evidence.

### 5.6 Operations, security, and recovery

Implemented: loopback enforcement, read-only containers/cap-drop, preflight,
backup profile, `kubonad restore`, migration snapshots, Trivy, Semgrep,
disk guard, API metrics, health/ready, deterministic local reset.

Still incomplete:
- `.env` is the secret store; Docker secrets/OpenBao is optional.
- The ledger is plaintext SQLite; host/backup compromise exposes it.
- External alerting is a single webhook for preflight/backup, not disk/rate/
  settlement routing.
- The restore path exists but is not a routine release drill.
- The default sandbox boundary is root-equivalent if enabled.
- Manual accessibility is not run.
- Non-executable providers are visible; the UI is honest but the vocabulary is
  broader than the functional product.

### 5.7 Console and accessibility

Automated evidence is strong: axe, keyboard, reduced motion, forced colors,
200% zoom, focus traps, list equivalents, and E2E gates. The manual VoiceOver/
NVDA script is explicitly pending (`docs/manual-accessibility-2026-11-01.md`).
The shipped console still includes the widget gallery, `WidgetStage`, legacy
`SpatialCanvas`, and a large raw-palette surface area. Each is plausibly
proposal-only, but the production build should make the proposal boundary
unmistakable.

## 6. Implementation plan

### Sequencing and relative effort

| Phase | Relative effort | Depends on | Why this order |
|---|---|---|---|
| 0 — Truth/contracts/reconciliation | M | none | Every later feature needs one result/job identity |
| 1 — Go-owned workflow lifecycle | L | Phase 0 | Removes split-brain and hardware-workflow spend gaps |
| 2 — Hardware experiment worker | L | Phases 0–1 | Needs job ownership, settlement, and approvals first |
| 3 — Scientific completeness | L | Phase 2 for hardware; Phase 0 for result schema | Accuracy claims must be measured and comparable |
| 4 — Sandbox/security | M | none (can run in parallel) | Security posture should not block experiment work |
| 5 — Claims/result UX | M | Phases 0, 2 | Review and visualisation need structured results |
| 6 — Release hardening | M | all prior | Final release gate and manual evidence |

### Phase 0 — Freeze truth and make the books reconcilable (P0)

**Goal:** no result, claim, or spend can exist in one rail and be invisible in
another.

1. Define a versioned `ExperimentResult` schema shared by:
   bridge result, Go job result, campaign `MeasurementSummary`/`PesPoint`,
   API response, export, and UI.
   Minimum fields: `result_version`, `job_id`, `claim_id`, `spec`, `method`,
   `ansatz`, `backend`, `shots`, `seed`, `counts`, `measurement_terms`,
   `energy_measured`, `energy_exact`, `uncertainty`, `ci95`, `delta_mha`,
   `converged`, `optimizer`, `trajectory`, `layout`, `calibration`, `usage`,
   `cost`, `feasibility{below_fci, within_ci}`, `provenance_hash`,
   `evidence_refs`.
2. Extend `AdoptExternalJob` only as a migration aid: failed/cancelled paths,
   idempotent repair, and a `repair_required` projection. Add a `kubonad
   reconcile-jobs` command that scans Python `jobs.jsonl` and Go `jobs` and
   reports/repairs orphans. It must never invent results.
3. Make `handleChatLaunch` accept and persist `shots`, `seed`, `backend`,
   `method`, and the effective spec; remove the hardcoded 1024.
4. Route Krylov/mitigation/QIR through the job/result ledger or explicitly mark
   them as non-citable diagnostics in the UI and API. Recommended: create jobs.
5. Add a CI contract test that a workflow step result, a direct route result,
   and a launch job all expose the same result identity.

**Exit criteria:** kill/restart tests leave no ghost; failed/cancelled workflow
steps are visible and settled; every citable result has a job id and versioned
result.

### Phase 1 — Go-owned workflow lifecycle (P0)

**Goal:** one job lifecycle for every workflow step.

1. Persist workflow and step records before execution.
2. For `execution/submit_job` and `submit_qubo_job`, create the Go job and
   reservation before calling Python; pass the Go job id to Python; require the
   Python result to bind to it.
3. For non-job tools, record an idempotent step receipt.
4. Settle each step through the existing terminal settlement rail; no adoption
   after completion.
5. Add a mid-run crash/retry test and a hardware-workflow test with a mocked
   provider.

**Exit criteria:** a Python completion without a Go job id is refused; a Go
crash during a workflow is recoverable without double-spend or ghost jobs.

### Phase 2 — Hardware experiment worker (P0/P1)

**Goal:** real hardware VQE/QAOA with the same governance as circuit jobs.

1. Add a hardware experiment mode that compiles each observable group into
   circuits using the existing `execution.submit_job` QPU rail.
2. Require an approval/mandate and reserve the worst-case shot/circuit cost
   before dispatch.
3. Poll vendor status, persist layout/calibration/usage, settle actual usage,
   and support cancellation.
4. Add a QAOA "optimize locally, execute final state on QPU" mode with a
   counts-to-cut estimator and explicit `simulated_optimizer:true` label.
5. Make QPU experiments explicit: `backend=ibm-qiskit`, `shots`, `seed`,
   `calibration_age`, `plan`, `queue`, `usage`, `actual_usd`.
6. Add a mock-provider CI lane; keep live hardware runs opt-in/manual.

**Exit criteria:** one real hardware VQE and one QAOA run complete end-to-end
with a job id, claim, and settled ledger, or the feature remains marked
"unavailable" in the UI. No simulator result may be labelled hardware.

### Phase 3 — Scientific completeness (P0/P1)

1. Full-space LiH/BeH2: replace `EfficientSU2` with a resource-bounded
   ADAPT/UCCSD implementation; define and test an accuracy target. If the
   target is not met, keep the full-space path opt-in and contested; do not
   claim chemical accuracy.
2. Add LiH/BeH2 PES, repeat-run variance, and error-mitigated VQE/PES.
3. Add a hardware-vs-simulator-vs-exact comparison view.
4. Add result-level CI95 (bootstrap over measurement terms or shots).

**Exit criteria:** every supported molecule has a declared active/full-space
comparison, an accuracy target, and an honest supported/unsupported boundary.
No `chemical_accuracy:true` without measured uncertainty and a passing gate.

### Phase 4 — Sandbox and security closure (P0/P1)

1. Choose the sandbox topology: narrow sidecar, rootless Podman/gVisor, or
   explicit trusted-host scope. Record the decision and threat model.
2. Ensure default `up` either has a working safe sandbox or a clear
   `autoresearch unavailable` surface; never a silent host fallback.
3. Add startup preflight for sandbox availability and a test that the default
   compose cannot execute a harness on the host.
4. Secret custody: Docker secrets or OpenBao for QPU/LLM keys; encrypted
   off-host backup; documented key rotation and restore.
5. Ledger-at-rest: optional SQLCipher/OS-level encryption with a documented
   custody model; no claim of encryption without it.

**Exit criteria:** sandbox functional under the supported topology; default
host execution impossible; key rotation and restore tested.

### Phase 5 — Claims and result UX (P1)

1. Add append-only claim review decisions; project current state; keep the
   claim immutable.
2. Add claim timeline, reviewer identity/key, reason, evidence links, and
   reopen.
3. Add PES error bars/FCI curve, optimizer trajectory chart, repeat-run
   variance, and a full job/result detail view.
4. Add per-point evidence/claim refs and signed export bundle.
5. Add browser E2E for claim history, review, PES table/CSV, full-space, and
   workflow adoption.

**Exit criteria:** a researcher can go from job to result to claim to review
without leaving the product or reconstructing the story from raw JSON.

### Phase 6 — Release hardening (P1/P2)

1. Backup: off-host destination, restore drill, encrypted artifacts.
2. Observability: opt-in OTel/Sentry with scrub/sampling; alert routing for
   disk/rate/settlement.
3. Accessibility: execute the manual VoiceOver/NVDA script; record results.
4. Console: remove or gate demo/legacy surfaces; continue design-system
   migration.
5. Non-executable providers: hide from the default picker or label "planned".
6. Run a fresh-volume release acceptance and a restore drill on the exact
   shipped commit.

**Exit criteria:** a fresh host can be installed, run, backed up, restored,
upgraded, and operated by one person without hidden shell steps.

## 7. Definition of done (local-private)

A deployment is complete only when all of the following are true:

1. Fresh install works offline except explicitly configured QPU/cloud LLM.
2. No result is citable unless its job, result, claim, and provenance are
   visible in the same session book.
3. Every advertised experiment is either functional with a real backend or
   explicitly marked unavailable with a reason; no simulator result can be
   mistaken for hardware.
4. Full-space and active-space claims are separated; every chemical-accuracy
   claim has an uncertainty and a passing gate.
5. Workflow failures, cancellations, and spend are visible and reconciled.
6. The default sandbox either works safely or is clearly unavailable; no root
   equivalent is required without an explicit operator acknowledgement.
7. Backup, restore, migration rollback, and key custody have executed drills.
8. Automated accessibility passes and the manual screen-reader pass is
   recorded on the shipped commit.
9. CI/E2E green on the shipped commit, plus a fresh-volume local acceptance.
10. Documentation matches code; stale "open" claims are corrected or linked to
    this report.

## 8. Operator decisions required

| Decision | Why | Recommended default |
|---|---|---|
| Local-private vs multi-lab | Determines auth, roles, and test scope | Local-private single operator |
| Hardware experiment scope | Cost, queue, and safety | IBM-only, opt-in, one pre-approved pilot |
| Full-space accuracy target | Scientific claims | 1.6 mHa at declared basis/ansatz or "unsupported" |
| Sandbox topology | Security vs functionality | Narrow sandbox sidecar or explicit trusted-host scope |
| At-rest encryption | Host-loss threat | OS disk encryption minimum; optional SQLCipher |
| Manual accessibility | Release sign-off | Run and record before calling it production |
| Off-host backup | Disaster recovery | Encrypted copy plus quarterly restore drill |

## 9. Risk register

| Risk | Impact | Mitigation |
|---|---|---|
| Python/Go split-brain during workflow | Ghost jobs, double spend, uncitable results | Phase 0 repair + Phase 1 Go-owned lifecycle |
| QPU hardware loop exceeds Open Plan/time | Incomplete experiment, queue uncertainty | Budget admission, cancellation, plan/queue UI, opt-in |
| Full-space ansatz does not converge | False scientific claim | Keep contested; implement ADAPT; explicit target |
| Docker socket profile exposed | Host compromise | Sidecar/rootless; explicit warning; no public bind |
| `.env` or ledger copied off-host | Secret/claim exposure | Docker secrets/OpenBao, encrypted backups, disk encryption |
| Demo/proposal surfaces mistaken for measurements | Trust failure | Provenance badges, remove from production, contract tests |
| Missing manual a11y | Release blocker for some operators | Execute script; record; no waiver without sign-off |

## 10. Verification and rollback

### Gate per phase

- **Unit/contract:** `go test -count=1 ./...`, `go vet ./...`, `platform/.venv/bin/python -m pytest -q`, `pnpm test:coverage`, `pnpm lint`, `pnpm exec tsc --noEmit`.
- **Contract drift:** `python3 scripts/console-contract-inventory.py --check`, `node scripts/console-openapi.mjs --check`, `python3 scripts/repo-counts.py --check`.
- **Stack:** fresh-volume `docker compose ... up --build -d`; `/api/ready`; loopback check; console boot; `scripts/local-deployment-check.sh`.
- **Recovery:** create/verify backup; restore into a scratch state root; `kubonad verify`; restart; compare campaign/job/claim counts.
- **Browser:** `scripts/e2e-isolated.py --build-console` plus the new claim/PES/full-space/workflow E2E.
- **Security:** Trivy critical gate; Semgrep; sandbox refusal test with no Docker socket; explicit sandbox test with the opt-in profile.
- **Hardware (manual/opt-in):** a single approved pilot run with vendor id, layout, calibration, usage, actual cost, cancellation, and settlement; never run from CI by default.

### Rollback rules

- Workflow/result changes must be additive and versioned; keep the old projection readable for one release.
- A failed result-schema migration must leave the previous campaign projection intact and refuse writes, not silently rewrite.
- Hardware modes default off; a failed pilot must be disabled by configuration, not by deleting evidence.
- Sandbox profile changes must be reversible with a documented operator action; no host fallback on failure.

## 11. File/evidence map

- Experiment bridge: `platform/bridge/experiments.py`; `platform/bridge/server.py`
- Jobs/ledger: `platform/core/pkg/jobs/jobs.go`; `platform/core/pkg/ledger/ledger.go`
- Workflow: `platform/core/pkg/api/workflows.go`; `platform/kubona/workflows/engine.py`
- Experiment API: `platform/core/pkg/api/experiments.go`; `platform/core/pkg/api/server.go`
- Launch hardcoding: `platform/core/pkg/api/server.go:2909-2919`
- Direct routes: `platform/core/pkg/api/server.go:5564-5742`
- Result model: `platform/core/pkg/agents/campaign_synth.go`; `platform/core/pkg/agents/replan.go`
- Claim history: `platform/console/src/components/common/ClaimHistory.tsx`; `/api/sessions/{id}/claims`
- Results UI: `platform/console/src/components/surfaces/ResultsStudio.tsx`
- Workbench projection: `platform/console/src/components/workbench/workbench-model.ts`
- Sandbox: `platform/core/pkg/sandbox/docker.go`; `platform/docker/compose.sandbox.yml`
- Autoresearch: `platform/core/pkg/autoresearch/harness.go`; `platform/core/pkg/api/server.go:4790`
- Provider truth: `platform/core/pkg/api/quantum_settings.go:65-76`, `:196-243`
- Deployment: `platform/docker/compose.prod.yml`; `docs/local-deployment.md`
- Recovery: `docs/backup-restore.md`; `platform/core/cmd/kubonad/restore.go`
- Accessibility: `docs/manual-accessibility-2026-11-01.md`
- Existing production plan: `docs/production-readiness-plan-2026-10-31.md`
- Real-experiment audit: `docs/real-experiment-production-audit-2026-11-08.md`
- Results audit: `docs/results-presentation-audit-2026-11-08.md`

## 12. Recommended first slice

The highest-value first slice is **Phase 0 + Phase 1**:

1. versioned result schema;
2. launch shots/seed/backend persistence;
3. direct-route ledger closure;
4. Go-owned workflow step jobs with a repair command;
5. tests for crash/retry, failed/cancelled adoption, and hardware mocked spend.

This removes the class of "result exists but cannot be cited" defects before
adding more experiment surface. It is also the prerequisite for hardware VQE,
hardware QAOA, and claim review.
