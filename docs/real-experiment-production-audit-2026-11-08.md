# Real-Experiment Production Audit — 2026-11-08

**Scope.** End-to-end investigation of what a *real* quantum experiment means for
Kubona, followed by live execution of the human, agent, simulator, and QPU rails
on a production-shaped local-private Docker stack.

**Verdict.** The core H2 experiment spine now works on the local-private Docker
stack: a real VQE and PES run produce shot-measured energies, fold into the
campaign, mint a verified claim, and render in the proof/UI. The native Rust QIR
compiler and the Docker calibration-refresh path are deployed. The platform is
still **not production-ready for the full experiment portfolio**: LiH/BeH2 VQE,
QAOA, autoresearch, and a completed QPU measurement remain open; the workflow
rail still writes its Python ledger separately from the Go job ledger.

The original audit findings are retained below; the “Implementation executed”
section records what changed and the remaining status.

> ⚑ Status update (later 2026-11-08): the portfolio gaps named in the verdict
> were exercised after the initial report.
>
> - **LiH VQE**: job `8d122fa8…` / session `ea4aa53a…`; 4 qubits,
>   active-space `ncas=2/nelecas=2`; measured `-7.8618628 Ha`; full FCI
>   `-7.8823915 Ha` (Δ `20.53 mHa`, contested); active-space CASCI
>   `-7.8622641 Ha` (Δ `0.401 mHa`, within the active-space target).
> - **BeH2 VQE**: job `68f226a4…` / session `ef1b5ba4…`; 6 qubits,
>   active-space `ncas=3/nelecas=4`; measured `-15.5588153 Ha`; full FCI
>   `-15.5951176 Ha` (Δ `36.30 mHa`, contested); active-space CASCI
>   `-15.5631370 Ha` (Δ `4.322 mHa`).
> - **QAOA**: job `ebda920e…`; 8-node MaxCut, exact optimum `10`, exact
>   optimizer `8.901`, shot-measured `7.870`, ratio `0.787`; the `qaoa_run`
>   settlement path now releases the reservation (re-tested live at `$0`).
>   The simulator result now carries the optimized `circuit_qasm` for the
>   generic hardware rail. A manual diagnostic hardware submission
>   (`dbo7uooqtr0c73ae4u80`) was queued on `ibm_fez` and then cancelled after
>   the queue wait; it produced **no** hardware result and bypassed the ledger
>   (diagnostic only). No hardware QAOA/VQE result is claimed.
> - **Completed QPU**: job `38a0b0642f664c74` / vendor
>   `dbo2egoou4as73b5dfog`, `ibm_fez` layout `144/143`, 100 shots,
>   counts `00:57, 01:2, 10:1, 11:40`, usage `2.0 s`, actual `$0.00`,
>   Go status `completed`; this is a hardware Bell characterization, not a
>   hardware VQE.
> - **Autoresearch**: the hardened runner is available only with the opt-in
>   Docker-socket override (`docker_runner.available:true`); loop
>   `loop-ansatz-search-bd7627a9` completed one sandboxed iteration. The
>   default compose remains fail-closed.
> - **Workflow ledger**: the Go route now mirrors `workflow_step` and
>   `workflow_execution_completed`/`failed` events into the session ledger;
>   live run `9a74fe4d…` produced both event types. Python still owns the
>   step-side job/ledger, so this is a mirror, not one transaction.
> - **Console form**: the `Experiments` settings surface submits VQE/PES/
>   QAOA through `POST /api/experiments`; typecheck/build pass.
> - **Results R0**: measurement summary, per-point PES, uncertainty,
>   below-FCI flag, convergence, proof labeling, negative sentinels, and a
>   result-JSON export are implemented. Live R0 session
>   `de02c225-5a8d-4341-bc3b-a08c14ad6218` produced a structured measurement
>   and a 4-point PES; the result card is unit-covered.
>
> See `docs/results-presentation-audit-2026-11-08.md` for the remaining R1
> work and the presentation acceptance matrix.

**Revision audited:** `a7a8cd2` for the original pass; implementation is at
`4cb5e7c` (CI + Console E2E green on `develop` and `main`). The implemented
result presentation is audited separately in
[`results-presentation-audit-2026-11-08.md`](results-presentation-audit-2026-11-08.md).
**Audit stack:** Docker project `kubona-audit`, current images built from
`a7a8cd2`, console `127.0.0.1:3000`, API `127.0.0.1:8080`, UDS bridge.
**LLM under audit:** NVIDIA `nvidia/nemotron-3-ultra-550b-a55b` for
chat/tribunal roles; codesign left on `nvidia/nemotron-3.5-super-vl-120b-a12b`
because the runtime card does not price Ultra.
**QPU:** IBM Quantum Open Plan instance; credentials present.
**Code changes:** none. The host `fidelity-devices.json` was refreshed, copied
to `/tmp`, and bind-mounted into the audit containers; the repository contract
was restored. No secret is printed in this report.

---

## 0. Implementation executed after the audit

**Implemented and tested**

| Change | Evidence |
|---|---|
| Native Rust QIR in the production image (`qir-core` builder stage, `KUBONA_QIR_CORE_BIN`) | `/api/qir/compile` -> `compiler_used:"rust-qir-core"`, `fallback_estimate:false`, `qir_profile:"BaseProfile"`; `/usr/local/bin/qir-core` present in the image |
| Writable calibration contract in the state volume; entrypoint seeding; `refresh.py` honors `KUBONA_FIDELITY_DEVICES` | `docker compose run --rm --no-deps bridge python -m kubona.fidelity.refresh --provider ibm-qiskit --device ibm-heron --write` returned `written:true` with live `ibm_fez` values; QPU dry run then admitted with `calibration_fresh:true` |
| Plan-aware IBM estimate | Open Plan dry run now returns `estimated_usd:0.0` with `list_price_usd:8.000568`, `plan:"open"`, `pricing_type:"free"` |
| First-class `cancelled` job status | Go job sync + terminal statuses; UI maps `JOB_STATUS_CANCELLED` |
| VQE + PES experiment RPC, Go job types, API, campaign fold, claim mint | VQE job `61556f56…` completed with `energy=-1.1377829714 Ha`, `fci=-1.1372701747 Ha`, `Δ=0.5128 mHa`, uncertainty `1.384 mHa`, measured trace, claim `clm-69df2967-0001`; spend released to `$0.00` |
| Campaign notebook/epistemics/proof integration | Notebook anchor `-1.13727 Ha`, provenance hash populated, verdict `VERIFIED`; `/api/epistemics` canonical anchor/claim record and `VERIFIED` state |
| Workflow session ownership/bridge | `execution/submit_job` workflow against a Go campaign completed (1 step, `$0`); session owner is now the bridge principal |
| Reasoning-model handling and codesign prompt | Codesign rerun with the NVIDIA model returned a valid AND2 candidate (`ccx`, TQ=3) and sealed provenance; tribunal with the device-auditor prompt fix reached unanimous consensus with four authored turns |
| Measured-output UI for VQE/PES | `MeasuredOutputs` renders VQE energy/delta/backend and PES best point; Vitest coverage added |
| Full local gates | Go `./...` green; 115 console files / 576 tests (forks pool); tsc/lint/build green; 33 targeted Python tests green; three Docker images rebuilt from this tree |

**Final live acceptance evidence**

- H2 VQE: job `61556f56…`; measured `-1.1377829714 Ha` vs FCI
  `-1.1372701747 Ha`; Δ `0.513 mHa`; uncertainty `1.384 mHa`; claim
  `clm-69df2967-0001`; campaign spend `$0.00`.
- H2 PES: job `d093f497…`; 3 points, measured trajectory; a 4096-shot point
  was intentionally outside chemical accuracy and did **not** demote the
  verified primary point.
- Codesign: run `30e80d51…`; valid AND2 candidate `ccx`, Tof=1, TQ=3,
  provenance sealed.
- Tribunal: four authored turns, unanimous consensus, claim
  `clm-69df2967-0002`.
- Workflow: `execution/submit_job` against a Go campaign completed one step,
  `$0`, under the bridge principal.
- QIR: `/api/qir/compile` -> native `rust-qir-core`, no fallback.
- QPU: Open Plan dry run -> billable `$0.00`, list `$8.000568`, plan `open`,
  calibration fresh; a live plan-aware submit was accepted and canceled before
  a vendor job id was pinned, with no vendor spend.

**Still open after this pass**

- LiH/BeH2 VQE and QAOA are not implemented; the runner refuses unsupported
  molecules rather than borrowing H2 numbers.
- The workflow rail still uses the Python MCP ledger for its `submit_job`
  side effect; the Go workflow route records its own receipt/reservation but
  the two books are not one transaction.
- Autoresearch still reports `docker_runner.available:false` in the production
  compose image; the five harnesses are not runnable there.
- No completed QPU result was obtained in this audit; dispatch/dry-run/
  cancellation/settlement are proven, but a final hardware measurement remains.
- The VQE/PES API has no dedicated console submission form (the results and
  claims render in the existing workbench/proof surfaces).
- `converged` is the optimizer success flag; the H2 exact optimizer energy is
  already within the target but COBYLA usually stops at its evaluation cap.
- A runtime price entry for `nvidia/nemotron-3-ultra-550b-a55b` is still
  operator-owned; codesign uses the priced VL model with the corrected prompt.


> ⚑ Status update (R1, later 2026-11-08): the workflow rail now **adopts**
> completed Python `execution/submit_job` results into the Go jobs book
> (`AdoptExternalJob`), so `/api/jobs` and the session ledger carry the same
> job id/result instead of a ghost; live workflow `89c324ab…` produced job
> `28dfccf7a2ed4982` with counts and `workflow_step`/`job_settled` receipts.
> Full-space VQE is available as an explicit `full_space:true` option
> (`EfficientSU2`, full orbital Hamiltonian); the LiH smoke result is
> `12` qubits and remains far from full FCI, so it is labelled contested and
> is not claimed as chemical accuracy. QAOA now returns its optimized
> `circuit_qasm` for a hardware rail; the measured QAOA result remains
> simulator-only. Autoresearch reports an explicit fail-closed reason when
> the opt-in Docker socket profile is absent.

## 1. What counts as a real experiment for Kubona

A real experiment is not a proposal, a circuit template, a QPU queue entry, or a
classical anchor. For Kubona it must satisfy all of the following:

1. A target, basis/active space, observable, ansatz, optimizer, shots, backend,
   and stopping rule are recorded before execution.
2. A **classical anchor** is computed for the same geometry/basis.
3. A **quantum measurement** produces counts, energies, or expectation values
   with backend, layout, shots, seed, calibration, and uncertainty recorded.
4. The measured result is folded into the campaign and is reproducible from the
   ledger.
5. A claim is evaluated against the anchor and is visible as a verdict, with a
   notebook/provenance receipt and, where relevant, a manuscript draft.
6. Any hardware claim names the device, job id, usage, calibration time, and
   measured uncertainty; no advantage claim is made without a comparison.

Candidate experiment families for Kubona:

| Family | Concrete experiment | What must be measured |
|---|---|---|
| Molecular ground state | H2 / LiH / BeH2 / H2O VQE with UCCSD or hardware-efficient ansatz | Energy expectation vs FCI, convergence, shots, optimizer path |
| Bond dissociation / PES | H2 0.5–2.5 Å, LiH 1.0–3.0 Å | RHF/FCI anchor curve, VQE points, error bars |
| Quantum Krylov | H2/LiH QKSD subspace diagonalization | Ground-state energy, subspace eigenvalues, overlap/conditioning |
| Combinatorial optimization | QAOA MaxCut on an 8-node 3-regular graph | Approximation ratio vs exact classical MaxCut, depth p |
| Error mitigation | ZNE/PEC/CDR on a calibrated backend | Raw vs mitigated expectation, fit residual, error bound |
| Circuit cutting | 4–8 qubit circuit split to ≤2-qubit fragments | Reconstructed expectation vs uncut simulator |
| Circuit co-design | AND2/XOR2/Kickmix synthesis | Validity, T-count, TQ, peak qubits, frontier |
| Hardware characterization | Bell/GHZ, Clifford canary, readout calibration | Counts, fidelity, calibration timestamp, layout |
| Epistemic experiment | Pre-registered prediction → measurement → verdict | Surprise z, Brier score, claim record |

---

## 2. Audit method and stack

The audit used the supported production shape, not a host-only dev process:

```text
Docker compose project: kubona-audit
  bridge    unix:///run/kubona/bridge.sock
  kubonad   127.0.0.1:8080
  console   127.0.0.1:3000
  state     kubona-audit_kubona-state
```

Environment posture:

- `KUBONA_ENV=production`
- `KUBONA_SINGLE_LAB=1`
- `KUBONA_LLM_PROVIDER=nvidia`
- `KUBONA_LLM_CLOUD=1`
- `KUBONA_LLM_BUDGET_USD=1`
- `KUBONA_RATE_CARD_PATH=/contracts/rate-cards.json`
- Sovereign Air-Gap off only for this cloud-provider audit
- IBM token/instance/channel present; Open Plan

The NVIDIA endpoint was tested both directly and through Kubona:

- `/v1/models`: 200, configured model present.
- Chat completion: 200.
- `/api/llm/test`: 200, `ULTRA-OK` in 1.9 s for Ultra.
- UI chat: served by `nvidia/nemotron-3-ultra-550b-a55b`, 2.6 s / 193 tokens.

A fresh lab admin key was bootstrapped for the audit, then a session cookie was
used for the browser checks. The live operator `.env` remains untracked.

---

## 3. Evidence summary

| # | Scenario | Observed | Verdict |
|---|---|---|---|
| 1 | H2 campaign creation | PySCF anchor computed: RHF −1.1166843870853405 Ha, FCI −1.137270174660903 Ha, source `pyscf-live` | **Pass** |
| 2 | H2 PES anchors | 0.5 Å −1.0551597944706248; 0.7414 −1.137270174660903; 1.5 −0.9981493534714101; 2.0 −0.9486411121761853 Ha | **Pass** (anchor only; no PES workflow) |
| 3 | LiH / BeH2 anchors | LiH −7.882401932290228 Ha; BeH2 −15.57599337297506 Ha | **Pass** |
| 4 | NVIDIA Copilot proposal | Launchable QASM, FCI anchor, explicit “VQE not measured yet”; no `designed_by` | **Partial** |
| 5 | Launch proposed circuit on local simulator | Job completed in ~4 ms; Aer counts `00:590, 01:145, 10:289`; MW=0.1824819; no energy | **Partial** |
| 6 | Campaign replan after circuit | MW folded; notebook `classical_anchor:"not computed"`; provenance hash empty; trajectory empty | **Fail** |
| 7 | Quantum Krylov H2 | −1.1372701746609022 Ha in 28 ms; solver `numerical-krylov`; exact FCI match | **Pass** as computation, **Fail** as workflow (no ledger/campaign fold) |
| 8 | ZNE mitigation | Raw −0.00390625 → mitigated 0.89453125, error bound 0.0442; API drops `measured` labels | **Partial** |
| 9 | QIR compile | 2 qubits, depth 4, T=0, `CustomProfile-Estimate`, `compiler_used:"go-fallback-estimate"`, `fallback_estimate:true` | **Fail** (Rust QIR not in image) |
| 10 | Codesign evaluator | Valid `ccx` AND2: valid, TQ=3, kappa sweep 3/9/27 | **Pass** |
| 11 | Codesign rerun with NVIDIA | 2 steps, empty proposals, `no_valid_candidate`; provenance sealed | **Fail** (reasoning cap/prompt) |
| 12 | Workflow provider registry | Plan + run completed; step record `573c2e46a3a24d26` | **Pass** |
| 13 | Workflow real job submit | `execution/submit_job` with a Go campaign session: HTTP 400 “unknown session_id” | **Fail** (split-brain) |
| 14 | Tribunal H2 | 4 turns, 70.6 s, consensus false; turns 3/4 LLM-empty fallbacks; claim `clm-...0002` contested | **Partial** |
| 15 | Vault prediction | Seal/evaluate/assert: z=−0.000349, Brier 7.29e−6, claim `clm-...0001` | **Pass** |
| 16 | Epistemics / proof after claim | `/api/epistemics` empty; proof PENDING; campaign epistemics None | **Fail** |
| 17 | IBM dry run before refresh | 502 “Stale calibration data (179.7 hours old > 24.0h limit)” | **Fail-closed, but production blocker** |
| 18 | IBM dry run after live refresh | Estimate $8.000568; predicted fidelity 0.9600; device `ibm-heron`; admitted | **Pass** |
| 19 | IBM live submit | Queued as `7b5c54f951a64812` → vendor `dbnvitoqtr0c73ado300`; layout 144/143; readout 0.00195/0.00293; `ibm_fez` | **Partial** (dispatch only) |
| 20 | IBM cancel/settlement | Vendor `CANCELLED`; usage 0; actual $0; local spent 0; Go status `failed`, bridge status `cancelled` | **Partial** (status mismatch) |
| 21 | Autoresearch start | `docker_runner.available:false`; also refused unpriced Ultra | **Fail** |
| 22 | UI chat/campaign | Ultra answered; campaign selected; job/approval cards render | **Pass** |
| 23 | UI proof/status | Proof PENDING; canceled job displays FAILED; “0 loops” after loop registered | **Partial** |

---

## 4. Stage-by-stage findings

### 4.1 Intake and campaign definition

**Works**

- `POST /api/sessions` creates a session and an initial campaign proposal.
- `BuildCampaignProposal` computes the live PySCF anchor; production refuses to
  create a campaign when the bridge cannot compute one.
- H2, LiH, and BeH2 anchors match expected STO-3G values.

**Gaps**

1. **No experiment-specification schema.** A campaign stores a title, target,
   and a synthesized circuit. It does not store a typed experiment object with
   observable, ansatz, optimizer, shots, seed, backend constraints, stopping
   rule, or success criterion. This makes a true experiment unrepeatable even
   when the anchor is correct.
2. **No PES/PES-series intake.** `BuildCampaignProposal` parses one bond
   distance. There is no `r_min/r_max/steps` HTTP path; each point requires a
   new campaign.
3. **`fresh` is accepted but unused** by the session handler. A caller cannot
   request a clean campaign state reliably.
4. **The default proposal is a template, not a VQE ansatz.** The synthesized
   circuit contains `h`, `rz`, `cx`; the campaign reports
   `ansatz_family:"UCCSD Parameterized"`. Those two facts disagree. The
   `Synthesized` flag is honest, but the label is not.
5. **Proposal attribution is absent.** Both NVIDIA models returned
   `proposal_json` without `designed_by`; the fallback proposal path does not
   stamp the served model. Jobs therefore have no LLM attribution.
6. **Target parsing is loose.** Unknown targets fall through to molecule-table
   defaults; the launch rail does not verify the proposal target against the
   campaign target.

### 4.2 Classical anchor

**Works**

- H2 at 0.7414 Å: FCI −1.137270174660903 Ha, RHF −1.1166843870853405 Ha.
- H2 at 0.5/1.5/2.0 Å and LiH/BeH2 all computed live.
- Anchor source `pyscf-live`; production fail-closed without the bridge.

**Gaps**

1. **Anchor never reaches the notebook.** After a circuit run, the notebook
   reports `classical_anchor: "not computed"` and `provenance_hash: ""`.
   `ensureNotebook` creates these placeholders; only a `classical_anchor` job
   fold fills them. Campaign creation already has a computed anchor, so the
   notebook is wrong from first use. Evidence: `campaign_state.notebook` after
   job `62195d55…`.
2. **No anchor chart/PES series in the campaign.** The workbench can show a
   chart only if trajectory points exist; there is no anchor curve.
3. **Anchor not fed to claim evaluation automatically.** A claim must be
   supplied externally; the campaign's own FCI value is not used as the default
   classical comparison.

### 4.3 Circuit proposal and VQE construction

**Works**

- NVIDIA Ultra returns a useful H2 VQE design and a launchable OpenQASM 2.0
  circuit.
- The proposal is extracted and displayed; the local simulator executes it.
- The model explicitly refuses to invent a VQE energy.

**Gaps**

1. **There is no VQE runner.** The proposed QASM is one parameter instance.
   There is no Hamiltonian construction, no Pauli grouping, no parameter
   vector, no optimizer loop, no shot-based `<H>` estimate, and no convergence
   criterion. `/api/chat/launch` can only submit a `circuit_run`.
2. **No energy observable in the job.** `circuit_run` folds only Meyer-Wallach
   entanglement. `counts` are not converted into an energy. The campaign
   `trajectory` remains empty after a real simulator execution.
3. **Shots are hardcoded at launch.** `handleChatLaunch` stores `shots: 1024`
   and does not accept a shots field. A proposal cannot declare 100/4096/8192.
4. **No seed.** The API does not pass a seed; deterministic reproducibility is
   not recorded. The Aer result was non-deterministic in practice.
5. **No QASM validation against the target.** A campaign can launch a circuit
   for another molecule or a non-VQE circuit; attribution/campaign state does
   not bind them.
6. **No parameterized-circuit model.** OpenQASM 2.0 with literal angles is the
   only accepted form; a variational experiment needs parameter symbols,
   bindings, and an optimizer contract.
7. **No QAOA path.** The QAOA harness exists only in the Python autoresearch
   layer; there is no HTTP/MCP experiment route.

### 4.4 Compilation and resource analysis

**Works**

- QASM → QIR-like LLVM text compiles and returns metrics.
- `compiler_used` is surfaced; no silent “success” for a failed lower.

**Gaps**

1. **Rust `qir-core` is not in the production image.** The Docker image copies
   `kubonad`, contracts, and backup script; no `qir-core` binary. The observed
   compiler is `go-fallback-estimate`, and `/api/qir/compile` returns
   `fallback_estimate:true`, `qir_profile:"CustomProfile-Estimate"`.
   The native Rust compiler is a design claim, not a deployed capability.
2. **No per-backend transpilation in the Go rail.** The bridge/Qiskit provider
   transpiles for IBM; the QIR rail does not produce a backend-targeted ISA or
   a resource estimate tied to the selected device.
3. **No compile receipt in the campaign.** `compiler_used` lives on the job row;
   the campaign notebook/provenance does not cite the compiler or its
   equivalence evidence.

### 4.5 Local simulation and execution

**Works**

- `local-simulator` / `qiskit-aer-simulator` execute; Bell canary passes.
- Counts, statevector, and MW are returned; job settles at $0.
- Canary for both simulators passed with balanced counts and correctness checks.

**Gaps**

1. **Wrong degradation label.** A `local_simulator` run reports
   `resolved_backend:"qiskit_aer.AerSimulator"`,
   `degradation_level:2`, and
   `degradation_reason:"Executed Aer noisy density matrix simulator"`.
   The backend is Aer without a noise model; the label is false. It should say
   exact statevector/shot simulation or use a real density-matrix noise model.
2. **`circuit_run` is not an experiment.** It records counts and MW only. No
   expectation value, no variance, no observable, no optimizer.
3. **No seed/backend configuration in the job.** Reproducibility is partial.
4. **`/api/krylov/solve` bypasses the ledger and campaign.** It returns
   `ground_state_energy`, `classical_fci_energy`, `subspace_eigenvalues`, and
   `solver:"numerical-krylov"` but writes no job, event, or campaign fold.
   The UI cannot cite it as a measured experiment.
5. **`/api/mitigation/apply` drops the truth labels.** It returns success,
   raw/mitigated values, error bound, and method; it drops `measured`,
   `device`, and the fidelity-probe receipt from the bridge response. The UI
   cannot distinguish a measured value from an inferred extrapolation.
6. **`/api/krylov/solve` drops labels too.** It omits `method`, `measured`,
   `ground_state_energy_measured`, `subspace_eigenvalues_measured`,
   `meyer_wallach_reason`, and `classical_method` from the bridge payload.
7. **No error bars.** Krylov and mitigation return single values; no confidence
   interval or shot-noise propagation reaches the campaign.

### 4.6 Hardware / QPU

**Works**

- IBM credentials are detected; `/api/quantum/backends` shows `ibm-qiskit`
  ready.
- Live calibration refresh from `ibm_fez` produced a valid snapshot:
  T1 147.76 µs, T2 106.61 µs, readout 0.01074, timestamp
  `2026-11-08T03:23:14Z`.
- Dry run admitted the Bell circuit with predicted fidelity 0.9599 after the
  refresh.
- Live submission reached IBM: local `7b5c54f951a64812`, vendor
  `dbnvitoqtr0c73ado300`, device `ibm_fez`, layout 144/143, readout
  0.00195/0.00293, queue 20k+.
- Cancel reached the vendor; vendor status `CANCELLED`, usage 0, actual $0,
  local session/tenant spend 0.

**Gaps / blockers**

1. **Stale calibration blocks every QPU dispatch after 24 h.** Before the
   refresh, the API returned 502 “Stale calibration data (179.7 hours old >
   24.0h limit)”. This is the correct fail-closed behavior, but there is no
   deployed refresh path.
2. **The refresh tool is not usable in the Docker production shape.**
   `kubona.fidelity.refresh` resolves
   `/usr/local/lib/python3.11/site-packages/contracts/fidelity-devices.json`
   inside the bridge image and fails `No such file or directory`. It ignores
   `KUBONA_FIDELITY_DEVICES`. `/app/contracts` is read-only. We had to refresh
   on the host, copy the file to `/tmp`, and bind-mount it over
   `/app/contracts/fidelity-devices.json` and
   `/contracts/fidelity-devices.json` to test.
3. **No scheduled/refresh worker or UI.** Calibration can go stale without any
   operator-visible warning before a dispatch is refused. `/api/fleet` shows
   IBM `status:"READY"` with unknown queue/qubits and no calibration age.
4. **List-price estimate vs Open Plan.** The dry run estimates $8.000568
   (5 s at $1.60/s) and reserves it, while the instance is Open Plan/free and
   the actual settles to $0. The UI says “Dispatch — $8.00”; an operator can
   be blocked by a ceiling for money that will not be billed.
5. **No queue visibility.** A 20k-job queue looks like `status:"running"` with
   `result.error:"not completed"` and no position/ETA. The operator cannot
   decide whether to wait or cancel.
6. **Cancel maps to `failed` in the Go ledger.** The bridge and `/api/quantum/jobs`
   say `cancelled`; the Go job row and UI show `failed`. Cancellation is not a
   first-class terminal state.
7. **No QPU result in this audit.** The vendor queue exceeded the test window;
   the job was canceled. A current hardware result (counts/fidelity) is not
   evidence until a job completes. Prior wiki evidence is historical.
8. **Estimate vs settlement is split across two rails.** Go reserves the
   pre-dispatch estimate; Python mirrors the session; the provider returns
   actual usage. This worked for cancel/settlement, but the success path needs
   the same live result proof (counts + usage) and a claim fold.

### 4.7 Results, claims, and epistemics

**Works**

- Vault prediction seal/evaluate/assert is real and ledger-sealed:
  prediction `3d5653…`, z −0.000349, Brier 7.29e−6, claim `clm-…0001`.
- `last_claims` exposes both vault and tribunal claims.

**Gaps**

1. **The campaign epistemics do not read claim records.**
   `/api/epistemics` reads `CampaignState.Epistemics`; the vault assertion mints
   a claim record but never updates the campaign. Result: proof PENDING,
   “No provenance receipt,” campaign `UNVERIFIED`.
2. **Notebook remains “not computed”.** Even after the anchor and measured
   Krylov value exist, the notebook has the anchor placeholder and no
   provenance hash.
3. **The latest claim determines the terminal verdict.** The tribunal’s
   contested claim `clm-…0002` overrides the vault’s asserted claim in the
   flight recorder. There is no precedence policy for verified vs asserted vs
   contested claims.
4. **Claim evaluation is not reachable from the human experiment rail.**
   `epistemic_claim_evaluation` is an A2A task mapping; A2A is disabled by
   default and requires x402. A normal campaign cannot submit a claim directly.
5. **The claimed value is not bound to a job.** Vault allows any measured value
   with an evidence reference; it does not require the referenced job to have
   completed or to carry the value. The operator can assert a value the system
   never measured.

### 4.8 Agents, tribunal, and co-design

**Works**

- NVIDIA Ultra Copilot answered correctly in 2.6 s in the UI.
- `get_anchor` tool call was observed; no fake `success:true`.
- Tribunal turns 1–2 were LLM-authored; claim and obligation records exist.
- Codesign evaluator is real and fail-closed; `ccx` AND2 returns TQ=3.

**Gaps**

1. **Reasoning-token caps break structured roles.** Tribunal turn cap is 1500
   (`platform/core/pkg/agents/collective.go:426`). Ultra spent 1500 tokens on
   reasoning and returned `content:null` for turns 3/4; the tribunal fell back
   to “LLM unavailable” and did not reach consensus.
2. **Codesign cap is 1024.** `codesignRerunMaxTokensPerStep = 1024`
   (`platform/core/pkg/api/codesign_rerun.go`). Ultra needed 4,400 completion
   tokens before producing valid JSON at 8192; the rerun returned two empty
   proposals and `no_valid_candidate`.
3. **Reasoning content is discarded.** `CompleteCapped` returns
   `Message.Content` only (`platform/core/pkg/llm/client.go:282`); it does not
   notice `reasoning_content` or surface a reasoning-only refusal. The caller
   sees an empty string, not “model reasoned and hit the cap.”
4. **Codesign prompt is underspecified.** The prompt says “Kickmix” without a
   grammar/few-shot example; the model spends its budget reasoning about the
   term. The evaluator has the closed grammar; the prompt should carry it.
5. **Model pricing is not enforced at assignment time.** Ultra is selectable
   in settings but absent from the runtime card. Autoresearch start refuses
   `unpriced_model`; codesign would too. A production operator can save an
   unrunnable configuration.
6. **Tribunal fallback text can become a claim.** The run minted a contested
   claim despite the device and scribe turns being non-authored fallbacks. That
   is honest as “contested,” but the UI should not present it as a scientific
   verdict.
7. **No per-role model capability gate.** A prose/reasoning model is used for
   strict JSON roles without a `supports_json`/`reasoning`/`max_output`
   contract.

### 4.9 Workflows and orchestration

**Works**

- `/api/workflows/plan` and `/api/workflows/run` execute a no-session tool
  (`execution/list_provider_registry`) and return a step record.

**Gaps**

1. **Session split-brain.** A workflow containing `execution/submit_job` with a
   valid Go campaign session failed:
   `400 unknown session_id '328729f7-…'`. The Go API passes its session id to
   the Python bridge workflow gateway, whose `SessionStore` does not know Go
   sessions. The sample provider-registry workflow works only because it does
   not need a session.
2. **Budget split-brain.** Go reserves a workflow ceiling in SQLite; the Python
   MCP rail has its own governance/session book. A real workflow would be
   accounted twice or not at all.
3. **No TTL/scheduler.** Schedule/event triggers are stored but cannot fire;
   no scheduler exists. For local production, a manual run is acceptable, but
   the plan should say so.
4. **No experiment templates.** The UI sample is `provider-audit`; there is no
   H2-VQE/PES/QAOA workflow template.
5. **Compensation paths were not exercised.** No failure injection was run in
   this audit.

### 4.10 Autoresearch, VQE, and QAOA harnesses

**Observed**

```json
{"docker_runner":{"available":false,"image":"kubona-quantum-runner:1.1.0"}}
```

Starting `ansatz-search` with Ultra returned:
`loop admission refused: role "coder" model "nvidia/nemotron-3-ultra-550b-a55b"
has no ceiling rate`.

**Findings**

1. **The five harnesses cannot run in the Docker production stack.** The
   kubonad image does not contain Docker or a Docker socket; the sandbox runner
   reports unavailable. The README’s VQE/QAOA/autoresearch capabilities are
   host-dev capabilities, not local-private production capabilities.
2. **Reasoning-model assignment is unpriced.** Even with a Docker runner, the
   loop would refuse Ultra before using it.
3. **No fallback to the bridge’s simulator stack.** The harnesses use a
   separate Python sandbox/provider rail, not the production Go/bridge job
   rail.
4. **No VQE/PES HTTP equivalent.** Nothing on the human rail exposes the
   harness result.

### 4.11 Console and operator experience

**Works**

- Login with a bootstrapped Ed25519 lab key; session cookie works.
- Chat with Ultra; campaign selection; job and approval cards render.
- The UI correctly shows the H2 anchor and the hardware job status.
- Workbench save/flicker fix held in this stack.

**Gaps**

1. **Canceled hardware job displays FAILED.** The local job row maps cancel to
   FAILED, so the operator cannot distinguish a deliberate cancel from a
   failure.
2. **Proof PENDING / no provenance receipt** despite a computed anchor, a
   completed simulator job, a Krylov result, and a vault claim.
3. **“0 loops”** in the status strip after a loop was registered and then
   stalled; the flight recorder has loop events but the UI count is zero.
4. **Normal chat is labeled “Kubona Tribunal Salon.”** The chat surface appears
   to use the tribunal salon label for ordinary copilot turns.
5. **No calibration/queue panel.** IBM status is “READY” while calibration is
   stale or a vendor job is in a 20k queue.
6. **Local simulator fleet cards show DEGRADED** while `dispatch_ready:true`
   and `executable_now:true`; status semantics are inconsistent.
7. **Model picker permits unpriced/structured-incompatible models.** The
   operator gets a late refusal at run time, not at settings save.

### 4.12 Operations and deployment

**Works**

- Loopback-only host publish; Docker health; `/api/ready`.
- Production-mode auth and session bootstrap.
- Runtime rate card mounted for this audit; cost summary $0.002844 of $1 cap.
- Backup profile exists (not exercised here).

**Gaps**

1. **Calibration refresh and image contract are incompatible** (see 4.6).
2. **`KUBONA_SECRET_KEY` is absent** from the audited `.env`; UI-entered provider
   keys would use the legacy/plaintext path. The docs require this for
   production. (No UI key was entered during this audit; NVIDIA came from
   `.env`.)
3. **`KUBONA_RATE_CARD_PATH` was absent** from `.env`; the first boot used the
   embedded card and logged a warning. The audit restarted with
   `/contracts/rate-cards.json`. A fresh compose deployment should fail closed
   or seed this path by default.
4. **Rate card does not include the audited model.** Ultra is usable for prose
   but not for priced roles.
5. **No QPU vendor-plan detection in the estimate.** Open Plan free is known to
   the provider (`_detect_plan`), but the pre-dispatch estimate is list price.
6. **No scheduled calibration/queue telemetry or alert** on the host.
7. **Docker bridge contract is baked into a read-only image layer**; refreshing
   it requires a rebuild or a bind mount, not an operator command.

---

## 5. Gap register

### P0 — blocks a real experiment workflow

| ID | Gap | Why it blocks | Evidence |
|---|---|---|---|
| RX-P0-1 | No VQE/PES/QAOA experiment runner on the human/agent rail | The product cannot measure the central chemistry/optimization experiments; only a single circuit runs | `/api/chat/launch` → `circuit_run`; experiment replan has no energy field |
| RX-P0-2 | `circuit_run` folds MW only; no energy/expectation/trajectory | A completed quantum job does not produce a scientific result | job `62195d55…`; campaign trajectory `[]` |
| RX-P0-3 | Workflow session split-brain | Any workflow that submits a real job fails against a Go campaign | `400 unknown session_id` for `execution/submit_job` |
| RX-P0-4 | Stale calibration has no production refresh path | QPU dispatch is blocked after 24h; the operator cannot recover without rebuilding/bind-mounting | 502 stale 179.7h; refresh tool fails in image |
| RX-P0-5 | Rust QIR compiler not in the image | The promised native compiler is absent; all results use Go fallback estimates | `compiler_used:"go-fallback-estimate"`; `fallback_estimate:true` |
| RX-P0-6 | Reasoning models exceed task caps; content is discarded | Codesign and tribunal turns 3/4 return empty; no valid co-design/tribunal result | tribunal turn 3/4 `reply_chars:0`; codesign `empty proposal` |
| RX-P0-7 | Claims/notebook/epistemics not integrated with campaign | The measured result is not citable in the UI; proof stays PENDING | notebook anchor uncomputed; `/api/epistemics` empty |
| RX-P0-8 | Autoresearch sandbox unavailable in Docker | VQE/QAOA harnesses cannot run in the supported production shape | `docker_runner.available:false` |
| RX-P0-9 | No current QPU result; queue/status/plan handling | A real hardware experiment cannot be completed/observed reliably | vendor job queued 20k; canceled; Go `failed` |

### P1 — production hardening

| ID | Gap | Why it matters |
|---|---|---|
| RX-P1-1 | List-price estimate on Open Plan free instance | Operator sees/reserves $8 for a $0 run |
| RX-P1-2 | No queue position/ETA, calibration age, plan in `/api/fleet` or console | Operator cannot choose/wait/cancel with evidence |
| RX-P1-3 | Cancel maps to `failed` in Go | Audit and UI lose the cancellation distinction |
| RX-P1-4 | Direct Krylov/mitigation/QIR routes bypass the ledger | Results cannot be cited or reproduced; spend/claims split |
| RX-P1-5 | Krylov/mitigation responses drop truth labels | Consumers cannot distinguish measured vs inferred |
| RX-P1-6 | Local simulator says “noisy density matrix” when it is exact Aer | Register drift; decisions may be wrong |
| RX-P1-7 | Proposal attribution missing (`designed_by`) | No model/agent authorship on jobs |
| RX-P1-8 | Shots hardcoded at launch, no seed | Experiments are not reproducible as specified |
| RX-P1-9 | Model assignment does not validate pricing/capability | Settings can be saved into a configuration that refuses at run time |
| RX-P1-10 | No PES-series route or anchor curve | The PES scenario is the canonical chemistry demo and cannot be run as one workflow |
| RX-P1-11 | `KUBONA_SECRET_KEY`/rate-card path absent in audited `.env` | Production secret-at-rest and cloud-pricing posture incomplete |
| RX-P1-12 | Fleet local simulators report DEGRADED | Ops status is misleading |

### P2 — enhancements

- QAOA MaxCut route and exact classical ratio in the campaign.
- Circuit cutting exposed as an experiment with reconstruction error.
- ZNE/PEC/CDR as first-class job types with measured/inferred labels.
- Ansatz/baseline comparison and active-space selection.
- Error bars, confidence intervals, and Brier-calibrated claims.
- Calibration-aware resource estimation and queue-aware planning.
- Hardware batching for parameter sweeps.
- Experiment templates and reproducibility manifests.
- Autoresearch sandbox as a separate, network-isolated sidecar.
- Manuscript generation from measured trajectories, not templates.
- Model capability registry and per-role structured-output contracts.

---

### Implementation status against the P0 register

| ID | Status after this pass |
|---|---|
| RX-P0-1 | **Partial** — H2 VQE/PES implemented; LiH/BeH2/QAOA not. |
| RX-P0-2 | **Closed for H2 VQE/PES** — measured energy/theta/trajectory folded; claim minted. |
| RX-P0-3 | **Closed for submit_job** — workflow session ownership fixed; cross-rail ledger still split. |
| RX-P0-4 | **Closed for refresh path** — state-volume contract + Docker command; no scheduler/auto-refresh. |
| RX-P0-5 | **Closed** — native Rust QIR in image, no fallback. |
| RX-P0-6 | **Closed for tested paths** — 8192 caps + prompt; generic capability gate remains future work. |
| RX-P0-7 | **Closed for VQE** — notebook/provenance/claim/epistemics verified. |
| RX-P0-8 | **Open** — autoresearch Docker runner unavailable. |
| RX-P0-9 | **Open** — no completed QPU result in this pass. |

## 6. Implementation plan

### Phase 0 — Unblock and make the existing evidence trustworthy (P0)

**Goal:** a QPU job can be refreshed, dispatched, observed, canceled, and cited
without rebuilding; the campaign’s anchor/claim state is truthful.

1. **Calibration service**
   - Make `kubona.fidelity.refresh` honor `KUBONA_FIDELITY_DEVICES`.
   - Put the contract in a writable state path
     (`~/.kubona/contracts/fidelity-devices.json`) and mount it read-only into
     bridge and kubonad; keep the image copy as bootstrap.
   - Add `kubonad calibration-refresh --provider ibm-qiskit --device ibm-heron
     --apply` and a scheduled pre-dispatch freshness check.
   - Seal `calibration_refreshed` with snapshot hash, source, and timestamp.
   - Add `/api/fleet`/console calibration age and refuse with the exact age.
2. **Campaign truth**
   - Fill notebook anchor/provenance from `CampaignState.ClassicalAnchor` at
     creation and on every anchor fold.
   - Make `/api/epistemics` read the latest claim record (and the campaign
     state) with explicit precedence: verified > asserted > contested >
     pending.
   - Show the claim record ID and evidence references on the proof card.
3. **Status/estimates**
   - Add a first-class `cancelled` status in the Go job table and UI.
   - Detect Open Plan/free instances before estimate; label list price vs
     billable amount, and do not block a free-plan session on list price.
   - Add queue position/ETA and vendor status to the quantum job response when
     the provider can report it.
4. **Compiler deployment**
   - Build `qir-core` in the kubonad image (Rust builder stage), pin the
     binary, set its path, and fail boot if the native compiler was expected
     but is absent.
   - Add an integration test asserting `compiler_used` is native and
     `fallback_estimate:false` for the supported QASM subset.
5. **Truth labels**
   - Fix the Aer degradation label.
   - Preserve `measured`, `device`, `solver`, `classical_method`, and
     uncertainty fields through `/api/krylov/solve` and
     `/api/mitigation/apply`; make them ledger jobs.

**Exit criteria:** fresh QPU dry run after a simulated 48h gap succeeds without
a rebuild; a canceled job reads CANCELED in API and UI; the proof card shows the
computed anchor and a claim record; QIR reports native compilation.

### Phase 1 — Build the experiment spine (P0)

**Goal:** one real H2 VQE experiment and one PES series from intake to claim.

1. **Typed experiment schema**
   - Add `ExperimentSpec` with `target`, `geometry`, `basis`, `active_space`,
     `observable`, `ansatz`, `optimizer`, `shots`, `seed`, `backend`,
     `mitigation`, `stopping_rule`.
   - Persist it in campaign state and the ledger before execution.
2. **Bridge runners**
   - Implement `VQERunner` in the bridge/Python runtime:
     - Build the fermionic Hamiltonian with PySCF for H2/LiH/BeH2.
     - Map to qubits (parity/JW) and expose Pauli terms.
     - Build the ansatz (UCCSD or hardware-efficient) with parameter symbols.
     - Evaluate `<H>` with shot-based grouped measurements on Aer.
     - Optimize with COBYLA/SPSA; record every iteration (theta, energy,
       gradient, shots, backend, seed, wall time).
     - Stop on gradient/energy tolerance and return uncertainty.
   - Implement `PESRunner` using the same Hamiltonian path over `r_min/r_max`.
   - Implement `Q AOARunner` for an 8-node 3-regular MaxCut, with exact
     classical optimum computed by the runner, not hardcoded.
3. **Go jobs**
   - Add `vqe_run`, `pes_sweep`, and `qaoa_run` to the closed job-type set and
     the HTTP `/api/jobs` rail (or a new `/api/experiments`), with session,
     mandate, autonomy, idempotency, and settlement identical to
     `circuit_run`.
   - Extend `ReplanWithJobResult` to fold energies, theta, uncertainty, and
     backend into `trajectory`; create a claim only when the run reports a
     measured energy and a valid convergence state.
4. **Claim and notebook**
   - On successful VQE/PES, mint a claim record with the anchor, measured
     energy, uncertainty, backend, seed, and ledger refs.
   - Render the notebook anchor, trajectory, claim verdict, and provenance
     hash in the console.
5. **Session bridge**
   - Mirror Go session creation/closure into the Python session store, or
     execute workflow tools through a Go-native dispatcher.
   - Add an integration test: create a Go session, run an
     `execution/submit_job` workflow, verify one ledger job and one spend
     reservation.
6. **Workflow templates**
   - Ship H2 VQE, H2 PES, LiH VQE, QAOA MaxCut, and Bell hardware templates.
   - Require the anchor step before the quantum step.

**Exit criteria:** H2 VQE at 0.7414 Å returns an energy within 1.6 mHa of FCI
with 1024 shots, a non-empty theta trajectory, a claim record, a notebook
provenance hash, and a reproducible manifest. The same run is visible in the
UI and re-runnable from the idempotency key.

### Phase 2 — Hardware and operations (P1)

1. Refresh calibration from the live provider on a schedule and before each
   dispatch; refuse with the exact age if refresh fails.
2. Estimate from the actual instance plan; record list price and billable
   amount separately; settle to measured usage.
3. Add queue position/ETA and vendor status to the job model.
4. Implement a real hardware experiment:
   - Bell/GHZ characterization on a free-plan backend.
   - H2 VQE on hardware with parameter batching, shot budget, and mitigation.
   - Record device, layout, calibration, readout error, usage, actual cost, and
     uncertainty; never claim advantage without the classical anchor.
5. Make cancellation first-class in ledger, API, and UI; verify vendor recall
   and settlement.
6. Add a hardware result ingestion test that compares counts to the ideal
   distribution and reports readout/layout diagnostics.

### Phase 3 — Agent and co-design hardening (P1)

1. Add a model-capability contract:
   - `supports_json`, `reasoning`, `max_output_tokens`, `context_tokens`,
     `priced` flag.
   - Reject a role assignment that cannot satisfy the role’s output contract.
2. Reasoning budget:
   - Read `usage.completion_tokens_details.reasoning_tokens`.
   - If `content` is empty and the finish reason is length, retry once with a
     larger cap inside the role budget, or refuse with “reasoning exhausted
     the cap.”
   - Raise codesign to at least 8192 and tribunal to at least 8192 for
     reasoning models; keep the dollar guardrail.
3. Codesign prompt: include the closed grammar, arity table, a valid AND2
   example (`[["ccx",[0,1,2],[]]]`), and “no prose, JSON only.” Test with Ultra.
4. Tribunal: use authored coverage for all four turns; do not mint a
   scientific claim when the audit/scribe turns are fallback-only. Show the
   fallback reason in the transcript.
5. Attribute proposals: stamp `designed_by` with provider/model in the
   server fallback path and preserve it in job params.
6. Add a priced runtime card entry for every enabled model, including
   `nvidia/nemotron-3-ultra-550b-a55b` (vendor price required; do not invent).

### Phase 4 — Autoresearch and workflow breadth (P1/P2)

1. Provide a real production sandbox:
   - Preferred: a separate sandbox sidecar with a controlled Docker socket,
     network-none, read-only root, cap-drop, and a documented threat model.
   - Or run the harnesses on a host worker behind an explicit operator gate;
     never mount the host Docker socket into the main API container by default.
2. Use the same session/ledger rail as the Go jobs; no second book of record.
3. Add QAOA, active-space search, ansatz transfer, and compilation-pass
   harnesses to the experiment templates.
4. Expose circuit cutting, ZNE/PEC/CDR, and resource estimation as first-class
   jobs with measured/inferred labels.
5. Add a scheduler only when a durable queue and approval policy exist.

### Phase 5 — Acceptance and evidence

Run and record:

1. H2 VQE at 0.5, 0.7414, 1.5, 2.0 Å (simulator; energy + uncertainty).
2. LiH and BeH2 active-space VQE.
3. H2/LiH quantum Krylov with classical comparison.
4. QAOA MaxCut ratio vs exact classical optimum.
5. ZNE on a noisy simulator; report raw vs mitigated plus residual.
6. Circuit cutting reconstruction vs uncut.
7. Codesign AND2 with Ultra: valid candidate, TQ, ledger receipt.
8. Tribunal with four authored turns and a contested/verified verdict.
9. IBM Bell/GHZ hardware result with counts, layout, calibration, usage, and
   settlement.
10. Workflow `submit_job` from a Go session with one ledger/spend record.
11. Backup/restore and restart persistence for a completed experiment.

---

## 7. Immediate next actions (ordered)

1. Fix calibration refresh in Docker and prove a QPU dry run after a simulated
   stale period; do not require a rebuild.
2. Add the VQE/PES runner and the `vqe_run`/`pes_sweep` job types; make the
   campaign trajectory the source of truth.
3. Fix the Go/Python session bridge for workflows.
4. Raise/route reasoning-model caps and add a priced card entry for the audit
   model.
5. Fill notebook anchor/epistemics from campaign/claim state.
6. Build `qir-core` into the production image.
7. Decide the sandbox production topology for autoresearch.
8. Re-run the acceptance matrix on a clean Docker stack and attach the run ids.

---

## 8. Evidence appendix

Key artifacts (not secrets):

- Audit stack: `kubona-audit` (bridge/kubonad/console), built from `a7a8cd2`.
- NVIDIA model test: `/api/llm/test` → `ULTRA-OK`, 1.9 s.
- UI chat: ultra, 2.6 s, 193 tokens.
- H2 anchor: FCI −1.137270174660903 Ha, RHF −1.1166843870853405 Ha.
- PES: 0.5 −1.0551597944706248; 0.7414 −1.137270174660903; 1.5
  −0.9981493534714101; 2.0 −0.9486411121761853 Ha.
- LiH −7.882401932290228 Ha; BeH2 −15.57599337297506 Ha.
- Simulator job `62195d55-5abe-4c83-bec9-4b14e4db0311`: 1024 shots,
  `00:590,01:145,10:289`, MW 0.1824819240, 2.73 ms.
- Krylov: −1.1372701746609022 Ha, solver `numerical-krylov`, 28.3 ms.
- ZNE: raw −0.00390625 → 0.89453125, error 0.04419.
- QIR: `CustomProfile-Estimate`, `go-fallback-estimate`, fallback true.
- Codesign: current model empty/no valid; `ccx` evaluator valid TQ=3.
- Tribunal: 4 turns, 70.6 s, consensus false, claim `clm-328729f7-0002`.
- Vault: prediction `3d5653…`, z −0.000349, Brier 7.29e−6, claim
  `clm-328729f7-0001`.
- Workflow: provider registry pass; submit_job `unknown session_id`.
- Autoresearch: `docker_runner.available:false`; Ultra `unpriced_model`.
- QPU: refresh `2026-11-08T03:23:14Z` (`backend-properties:ibm_fez`);
  dry estimate $8.000568; vendor job `dbnvitoqtr0c73ado300`; layout 144/143;
  canceled; usage 0; actual $0.
- Calibration stale refusal: 179.7h > 24h.
- Go/bridge cancel status mismatch: API `cancelled`, Go `failed`.
- Campaign notebook: `classical_anchor:"not computed"`, `provenance_hash:""`.
- Proof: PENDING despite computed anchor/claim.
- Rate card: `nvidia/nemotron-3-ultra-550b-a55b` absent; LLM spend $0.002844/$1.

**Limitation.** No current QPU counts were obtained. The 20k+ Open Plan queue
exceeded the audit window; the job was canceled with verified vendor recall and
zero usage. The historical IBM jobs in the wiki are prior evidence, not
evidence for this commit. A completed hardware result remains the final proof
for the hardware lane.
