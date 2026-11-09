# Production Completion Report and Plan — Local-Private Deployment

**Date:** 2026-11-08 (execution continued 2026-11-09)  
**Current code head:** `c1358e5` (2026-11-09 CEO/hardware-loop/mitigation/
result-version/claimable/sandbox-proof slices; re-verify the exact commit)  
**Scope:** fully functional, unattended local-private deployment on one trusted host.
Not a public SaaS, not a multi-tenant hosted service, not a demo.

> **2026-11-09 correction and re-audit:** the verified gap register and updated
> plan are in
> [`production-gap-audit-2026-11-09.md`](production-gap-audit-2026-11-09.md).
> Several rows below are stale: launch shots/seed, direct Krylov/mitigation
> ledger binding, and claim review decisions are now implemented; PES covers
> active-space LiH/BeH2. CEO-ADAPT now reaches LiH/BeH2 targets after the
> spin-block fix; hardware loops and molecular ZNE are implemented; gVisor
> was proven on a Linux VM; result-version enforcement is in place. Remaining
> P0s are live QPU loop/mitigation acceptance, scheduled calibration/vendor-
> queue operations, full campaign/UI type adoption (the signed receipt is now
> embedded by the job rail), browser breadth, manual accessibility, and the
> forensic host ledger.

## 0. Execution log (2026-11-08)

This plan is being executed incrementally. Landed after the report:

- **Experiment result identity:** bridge results carry
  `result_version:"experiment-result-v1"`; `MeasurementSummary` carries
  version, provider, hardware flag, optimizer/measurement backends, CI95,
  usage, actual USD, layout, calibration, counts, and evidence refs.
- **Reproducible launch:** `/api/chat/launch` accepts and persists `shots` and
  `seed` (defaults are explicit); the console helper forwards both.
- **Direct compute routes:** Krylov and mitigation now create a ledger-bound
  job and wait for the terminal result; anonymous/unattributed calls refuse.
- **Go-owned workflows:** normal mixed workflows now run step-by-step in Go
  (job steps on the Go jobs rail; non-job steps through `RunWorkflowStep`).
  `execution/submit_job` execute on the Go jobs rail and never create a
  Python ghost; mixed workflows retain the Python path/adoption bridge.
- **Claim review:** append-only `claim_reviews` with
  `accepted|contested|reopened` decisions, reviewer/reason/evidence, API, and
  Claim History controls; the claim row remains immutable.
- **Results UI:** measured CSV, PES table, optimizer history, provider/
  hardware/CI/usage facts, and review decisions; numeric-claim guard and
  claim history tests.
- **Build/gates:** CI and Console E2E green at `f1b7f1f`; Go full suite, 592
  console tests/54.35% branches, Python targeted suite, lint/tsc green.
- **Python terminal-job reconciliation:** on boot, `kubonad` adopts terminal
  Python `jobs.jsonl` records that have no Go row (idempotent; queued/running
  rows stay live). This closes the crash window left by the mixed-workflow
  adoption bridge.
- **Governed hardware QAOA launch:** `POST /api/experiments/qaoa/hardware`
  takes a local QAOA job's optimized QASM/graph and submits it through the
  existing circuit-run admission rail: mandate, jurisdiction, fidelity,
  reservation, settlement. It never dispatches directly to the bridge. The
  hardware counts are augmented with `measured_cut`/ratio and shown in the
  measured outputs surface.
- **Hardware QAOA diagnostic (not yet the product rail):** the optimized
  8-node QAOA circuit from the simulator run was executed on `ibm_marrakesh`
  (vendor job `dboak8oqtr0c73ae8dpg`, 256 shots). Hardware cut expectation
  `7.1875` vs classical optimum `10` and local exact optimizer `8.7106`
  (ratio `0.71875`), usage `2.0 s`, actual `$0.00`. The optimizer remained
  local and the run bypassed the session/claim ledger; it is evidence for
  the next hardware-worker phase, not a production result.
- **Hardware VQE diagnostic (not yet the product rail):** the final
  hardware-efficient H2 ansatz was measured on `ibm_marrakesh` across five
  Pauli bases (1024 shots each; vendor jobs `dboartgqtr0c73ae8nh0`,
  `dboarus4148s73927dag`, `dboas00ou4as73b5p500`, `dboas18ou4as73b5p52g`,
  `dboas2gqtr0c73ae8nng`). Including the identity coefficient, the hardware
  energy is `-1.119494 Ha` vs FCI `-1.137270 Ha` (`17.776 mHa`), i.e. not
  chemical accuracy; each job used 2.0 s, actual `$0.00`. The optimizer was
  local and this diagnostic bypassed the session/claim ledger. It proves the
  IBM hardware and observable-measurement path; it is not a production VQE
  result.

> ⚑ Correction (2026-11-08): governed hardware H2 VQE and QAOA parent runs are
> now complete with operator-signed mandates, Go jobs, vendor batch ids,
> settled usage, and claims. They are contested (H2 `10.369 mHa`, uncertainty
> `5.977 mHa`; QAOA ratio `0.7952`), not chemical-accuracy or advantage claims.
> The per-term diagnostics below remain historical external evidence; they were
> not retroactively upgraded.
Still pending from this plan: browser E2E breadth, manual accessibility,
and release hardening. Full-space ADAPT-VQE, Go-owned approval workflows,
the gVisor `sandboxd` topology, local provider-key sealing, disk/settlement/rate
alerts, and the encrypted off-host drill are implemented with the limits
recorded below. The 2026-11-08
unblock slice added claim pagination, a production LLM-secret preflight/write
refusal, and an explicit full-space status gate; those are no longer open as
silent omissions.

> Blocked-work investigation and sequencing: see
> [`production-unblock-plan-2026-11-08.md`](production-unblock-plan-2026-11-08.md).
> It records why each remaining item is blocked, the recommended decisions, and
> the Phase A–F unblock order.

> 2026-11-08 execution: the unblock slice landed claim pagination, production
> provider-secret preflight/write refusal, full-space status/claimability gates,
> the first `hardware_experiment` parent rail (H2, one Runtime batch, mock
> lane), and the durable workflow run/step journal with boot repair. Live
> hardware evidence still requires an operator mandate and run; mixed workflow
> normal mixed steps are Go-owned; approval/agent-created workflows still use
> the Python compatibility engine pending approval migration.

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

## 2.1 Execution log (2026-11-09)

- `3252180` — corrected CEO block-spin pool; LiH/BeH2 reach the 1.6 mHa gate;
  added governed hardware VQE/QAOA loops and molecular ZNE; versioned result
  gate/schema; local queue depth.
- `4c4ea51` — claimable/CI95 PES evidence folded into campaign `PesPoint`;
  live gVisor proof recorded (`docs/sandbox-live-2026-11-09.md`).
- `956deab` — campaign/claim paths honor `claimable` and `measured`; mitigated
  or exact-refused results cannot become "verified" from delta alone.
- `70cb947`, `ead17da` — hardware loop receipts carry layout/calibration;
  result-version gate regression test; BeH2/CEO docs corrected. CI/E2E green
  at `ead17da`.
- `2fa72b3` — live `ibm_fez` SPSA VQE loop, QAOA loop, and molecular ZNE
  completed through the governed parent rail with signed receipts; QAOA
  layer bug found and fixed. Evidence in `docs/live-qpu-2026-11-09.md`.
- Tests: full Go suite, 596 console tests, bridge tests, console tsc/eslint.
  CI/E2E green at `4c4ea51`; re-verify the later docs commit.

## 3. Evidence at the current revision

| Evidence | Current state |
|---|---|
| CI | 20/20 green on `develop` and `main` |
| Console E2E | green on both branches |
| Local compose | fresh-volume boot, loopback-only ports, health/ready, console |
| Backup/restore | `kubonad restore` and scheduled backup profile implemented and tested |
| H2 VQE/PES | measured shot energies, FCI anchor, claim, structured campaign fold |
| QPU | governed H2 VQE + QAOA parent runs complete (contested; OpenAI Plan $0) |
| LiH/BeH2 | active-space VQE measured; full-FCI gap contested (20.53 / 36.30 mHa) |
| Full-space LiH ADAPT | 12 qubits, 16 UCCSD operators, 0.706 mHa measured (0.213 mHa exact) |
| QAOA | 8-node 3-regular MaxCut simulator; optimized QASM returned |
| Workflow | completed Python job adopted into Go jobs book; non-atomic |
| Claims | history + `peer_review` request event; no decision/reopen state |
| Autoresearch | opt-in Docker socket only; default `available:false` |
| Accessibility | automated axe/keyboard/reduced-motion green; manual VoiceOver/NVDA not run |
| Hardware QPU QAOA | governed parent run complete: cut 7.9521/10, ratio 0.7952, claim contested |

This evidence is reproducible from the repository and the local compose stack.
It does not upgrade the missing portfolio features.

## 4. Gap register

### P0 — blocks calling the full local-private product "complete"

| ID | Gap | Why it matters | Evidence |
|---|---|---|---|
| PC-01 | **Approval workflows are Go-owned; job compensation remains partial.** Approval-required/agent-created workflows now require a signed single-use token and run through the Go mixed rail. No explicit inverse exists for job-step compensation. | A crash is surfaced as `repair_required`; job compensation is refused rather than faked. | `platform/core/pkg/api/workflow_approval.go`; `platform/core/pkg/api/workflows_go.go`; `platform/core/pkg/ledger/workflow_journal.go` |
| PC-02 | **Hardware iterative loops live-validated.** `loop:true` made every objective a real QPU batch on `ibm_fez`: SPSA VQE 9 batches (`-1.12536 Ha`, 11.9 mHa delta), QAOA 4 batches (cut 5.953/10, ratio 0.595). Both completed with signed receipts and `simulated_optimizer:false`; neither is a chemical-accuracy/advantage claim. | No code gap; higher shot/iteration tuning is a scientific-accuracy decision, not missing dispatch. | `docs/live-qpu-2026-11-09.md`; `platform/bridge/experiments.py`. |
| PC-03 | **Full-space CEO now reaches LiH/BeH2 targets.** The old CEO measurement was invalid due to an interleaved-spin assumption; the corrected block-spin pool reaches LiH 0.0399 mHa exact / 0.4499 mHa measured and BeH2 0.3933 mHa exact / 0.0146 mHa measured at 262144 shots. CEO is the default; UCCSD remains selectable. | Full-molecule claims remain gated by measured delta, shot uncertainty, and exact diagnostic. | `platform/bridge/adapt.py`; `docs/full-space-chemistry-2026-11-08.md`. |
| PC-04 | **Closed 2026-11-09 verification.** `handleChatLaunch` accepts and persists shots and seed, including proposal-driven launches. | A launch records its sampling spec. | `platform/core/pkg/api/server.go` launch block; `launch_test.go` |
| PC-05 | **Closed for Krylov/mitigation, including labels.** `runDirectComputeJob` creates a ledger-bound JSON experiment job and waits for terminal state; QIR compile remains stateless analysis, not a measured experiment. | Direct compute results have a job id, spend/ledger attribution, and the complete solver/mitigation receipt. | `platform/core/pkg/api/server.go` direct compute jobs; `platform/core/pkg/jobs/jobs.go` JSON cases |
| PC-06 | **Sandbox live on Linux VM; macOS native remains closed.** `sandboxd` + gVisor was proven on a Lima Ubuntu arm64 VM: health available, benchmark UID 1000, network blocked, read-only, cap-drop=ALL. macOS Docker Desktop still has no `runsc`; the API reports unavailable. | Use a Linux VM/host or keep autoresearch off; do not add a weaker macOS fallback. | `platform/core/pkg/sandbox/sidecar.go`; `platform/docker/compose.sandbox.yml`; `docs/sandbox-live-2026-11-09.md`. |
| PC-07 | **Closed.** Append-only `claim_reviews` stores accepted/contested/reopened decisions with reviewer, reason, and evidence; the claim measurement remains immutable and the API/UI expose the history. | A researcher can review without mutating the measurement. | `platform/core/pkg/ledger/ledger.go` claim review methods; `ClaimHistory.tsx` |
| PC-08 | **No versioned experiment-result contract.** Result truth is split among job JSON, campaign `measurement`/`pes_points`, notebook text, and UI prose. | Hardware metadata, counts, layout, calibration, usage, cost, per-point evidence, and CIs cannot be guaranteed across rails. | `platform/core/pkg/agents/campaign_synth.go:128-180`; bridge result shape; `ResultsStudio` projections |
| PC-09 | **Molecular hardware ZNE live-validated.** Gov. H2 vendor `dbos2u44148s739th03g`: scales 1/3/5, mitigated `-1.1441740297 Ha`, 6.904 mHa delta, 12.036 mHa uncertainty, `measured:false`, not claimable. PES still has no mitigation opt-in. | Live mitigation works; a PES opt-in is a product-scope choice, not a correctness gap. | `docs/live-qpu-2026-11-09.md`; `platform/bridge/experiments.py` |
| PC-10 | **Secrets/at-rest posture is not end-to-end.** `.env` is the supported secret store; the ledger is plaintext; Docker secrets/OpenBao are optional. | A lost host or backup exposes ledger/API material; "production local-private" must state and test this boundary. | `docs/local-deployment.md:250-260`; `docs/production-readiness-plan-2026-10-31.md` P1-3 |
| PC-11 | **Recovery is implemented but not operationally closed.** Off-host backup/restore drills and migration rollback are not proven as a routine release gate. | A local production host needs a repeatable disaster-recovery proof, not only unit tests. | Backup profile; restore docs; current CI has backup smoke, not a full restore drill. |

### P1 — required for a complete researcher workflow

| ID | Gap | Improvement |
|---|---|---|
| PC-12 | **Versioned schema and signed ledger receipt exist; campaign/UI adoption remains.** `platform/contracts/experiment-result-v1.schema.json`, a TS `ExperimentResult`, and a Go job-boundary version gate exist. Completed jobs embed `result_receipt` (Ed25519 when a result/reviewer key is configured; otherwise hash-chained `signed:false`). Campaign/UI still use separate structs. | Move campaign/export/UI onto the shared type and commission the signing key in production. |
| PC-13 | Result UI still lacks PES error bars/FCI curve in the chart, a full optimizer trajectory view, repeat-run variance, and direct claim/job links in every card. | Comparison/diagnostic surface; chart + table + JSON/CSV. |
| PC-14 | Claim review is a request prompt, not a decision. | Add reviewer decision state (`accepted`, `contested`, `reopened`) as an append-only review record; never mutate the measurement. |
| PC-15 | Non-executable providers are visible in the settings vocabulary. | Hide or clearly gate `planned`/`not integrated` providers; make `executable_now` the primary action state. |
| PC-16 | `GenerativeUIStudio`, `WidgetStage`, legacy `SpatialCanvas`, and some illustrative widgets remain in the shipped console. | Decide and document: keep as explicit proposal-only tools, or remove from the production build; no demo surface may look measured. |
| PC-17 | Manual VoiceOver/NVDA pass is still pending (human-only; not producible by code). | Execute the script in `docs/manual-accessibility-2026-11-01.md` on the shipped commit; record OS/AT/version per surface, or sign a residual-risk waiver. |
| PC-18 | Observability is local-only; OTel/Sentry is not enabled by default. Preflight/backup, disk guard, settlement-skipped, and cooldown-limited rate-limit alerts now route through the operator webhook. | Keep exporters opt-in and scrubbed; do not claim telemetry leaves the host. |
| PC-19 | Browser E2E for claim paging/full-space/workflow adoption remains a release gate; unit/API coverage exists for paging, full-space status, and approval/workflow execution. | Extend the real-stack E2E fixtures or record a scoped residual-risk waiver; keep hardware tests opt-in. |
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
- LiH/BeH2 full-space ADAPT-VQE (LiH validated; BeH2 resource-contested, see `docs/full-space-chemistry-2026-11-08.md`);
- QAOA MaxCut on a fixed 8-node graph, simulator only.

What is missing:
1. **Hardware VQE**. A correct hardware loop needs per-iteration circuits,
   observable grouping, shots, layout/calibration, budget admission, vendor
   status polling, cancellation, and settlement. The current path optimizes
   locally and measures the final active-space/H2 ansatz on one QPU batch; it is
   explicitly `simulated_optimizer` and is not an iterative hardware loop.
2. **Hardware QAOA**. `run_qaoa` returns `circuit_qasm` but always measures on
   Aer. A hardware QAOA needs a deliberate "optimise locally, execute final
   state on QPU" mode with explicit cost/approval and a counts-to-cut estimator.
3. **Full-space chemical accuracy**. LiH now reaches 0.706 mHa measured
   (0.213 mHa exact) with the UCCSD ADAPT pool; BeH2 remains above target through a 60-operator test; it is not a small
   tolerance issue. A production answer needs a
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
- External alerting is a single webhook, now covering preflight/backup, disk,
  settlement-skipped, and cooldown-limited rate limits; it is not OTel/Sentry.
- `kubonad offhost-drill` executes the encrypted off-host restore drill; the
  operator must still supply the separate mount and age identity.
- `compose.sandbox.yml` now uses a `sandboxd` sidecar with gVisor `runsc`;
  kubonad has no Docker socket. Hosts without `runsc` (including macOS Docker
  Desktop) report sandbox unavailable.
- Manual accessibility is not run; automated evidence is green.
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
