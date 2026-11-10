# Production audit and completion report — 2026-11-09

**Audited baseline:** `1b18ade` (2026-11-09 science/UI closure)
**Correction pass:** `fee14b4` (this document plus the truth/UI/hardware
hardening).
**Scope:** fully functional **local-private** deployment on one trusted host.
Not a public SaaS, not multi-tenant hosting, and not a demo.
**Method:** re-read the current Go/Python/console code and tests, re-ran the
targeted science suites, and checked generated contracts. Prior status prose was
treated as a claim to verify, not as evidence.

## 1. Executive verdict

The local-private stack is operational for the supported science surface:

- H2/LiH/BeH2/H2O molecular VQE and PES on Qiskit Aer with live PySCF anchors,
  plus caller-supplied closed-shell STO-3G geometries with explicit active spaces;
- H2/LiH/BeH2/H2O active-space excited states and H2/H2O reaction energies;
- exact 1D TFIM/Heisenberg/XXZ spin-chain simulation with a shot-measured
  experiment rail;
- exact finite-scope mathematical law discovery with holdout verification;
- governed IBM hardware parents for VQE, QAOA, and spin-chain final states;
- ledger-bound discovery protocols, campaign folding, claims, signed result
  receipts, and real-stack browser E2E.

It is **not** correct to call it a complete general science-discovery platform.
The remaining gaps are capability breadth and external validation, not missing
UI glue: no general molecule input, no geometry optimization, no 2D lattice or
time-evolution experiment, no general reaction network, no external-literature
novelty check, and no human screen-reader sign-off. Hardware VQE executions
exist, but the live spin-chain VQE run missed its exact anchor and remains
non-claimable.

The pass below fixes several concrete truth/UI defects found in the recent
science implementation. It does **not** upgrade any historical result to a
stronger claim.

## 2. Recent implementation audit

| ID | Finding | Severity | Resolution |
|---|---|---|---|
| A-01 | H2O → H2 + O measured `water + hydrogen - oxygen`, which is not a reaction energy (products were added instead of subtracted). | P0 truth | Fixed to `water - hydrogen - oxygen`; exact active-space and full-molecule anchors are now separate, and the claim is scoped to the active-space water model. |
| A-02 | Spin-chain hardware converted Pauli labels with the wrong qubit ordering. The uniform TFIM/Heisenberg set is reflection-symmetric, which hid the defect for the live model but would mis-measure asymmetric coefficients. | P0 truth | The bridge now converts source labels to Qiskit little-endian order for both measurement circuits and coefficient matching; a regression test asserts `X` on qubit 0 prepares `h q[0]`. |
| A-03 | ZNE extrapolation could set `claimable:true` while `measured:false`; an extrapolated point is not a direct measurement. | P0 truth | ZNE is now always `claimable:false` and `chemical_accuracy:false`, even when the fit lands within 1.6 mHa. |
| A-04 | Chemical-accuracy gates accepted a shot value below the exact anchor by more than three standard errors. | P0 truth | Added a shared below-anchor guard used by molecular, hardware, excited-state, and reaction results. |
| A-05 | `SpinChainModel.observables(vector)` silently recomputed the ground state and ignored the supplied vector. | P1 truth | The supplied state is now used for magnetization, correlations, and entropy; the exact spectrum is still the gap anchor. Invalid/non-normalizable vectors refuse. |
| A-06 | Go admission accepted source spin-chain data with `state_mode:"exact"` even though exact-state QASM contains reset and cannot be sent to hardware. | P0 governance | API source conversion and the jobs admission twin reject exact-state hardware before dispatch. |
| A-07 | Hardware VQE source conversion accepted H2O, but the jobs admission twin only allowed H2/LiH/BeH2. | P1 consistency | H2O active-space hardware VQE is now accepted consistently; a regression test reaches the mandate check rather than a molecule-type refusal. |
| A-08 | Hardware source jobs were checked for session/result shape but not for `vqe_run`/`qaoa_run`/`spin_chain` status and type. | P1 governance | Conversion now requires a completed source job of the expected type. |
| A-09 | VQE and QAOA parent bindings did not include `source_job_id`; a mandate could be prepared for one source and reused with another source having the same visible spec. | P0 governance | `source_job_id` is now part of the canonical parent binding for VQE/QAOA (spin already had it). |
| A-10 | Experiment results were rendered only when a molecular `COLLAPSED_VERIFIED` claim existed; spin/excited/reaction/conjecture records were hidden before that. | P1 UI | The measured-experiment table and generic report now render in the pre-claim state. |
| A-11 | `ExperimentPanel` compared `job.status` to lowercase `'completed'` while the API emits `JOB_STATUS_COMPLETED`; measured result details were not displayed. | P1 UI | All status checks use `jobStatusLabel`; stale labels were corrected for LiH/BeH2/H2O excited/reaction paths. |
| A-12 | There was no generic report/export for non-molecular experiments; manuscript export remained chemistry-shaped. | P1 UI | Added a measured experiment report and JSON export built only from stored campaign experiments; no prose-generated numbers. |
| A-13 | Discovery protocol history was ephemeral; only the latest response was visible. | P1 UI | Completed protocol summaries are written into the ledger event and reloaded in the panel; each step carries job id, status, measured/claimable flags. |
| A-14 | Hardware launch did not disclose the absence of a pre-dispatch vendor price. | P1 UI | The panel now explicitly states that the operator cap is enforced and a vendor price is unavailable before dispatch; no invented estimate. |
| A-15 | VQE hardware exposed no ZNE control even though the bridge supports it; loop + mitigation could be requested together and silently ignore mitigation. | P1 UI/truth | Added a VQE ZNE control and a Go/UI guard that refuses loop + mitigation together. |
| A-16 | Conjecture discovery advertised 3 terms but holdout validation effectively required 4. | P2 input | It now explicitly requires 4 terms (3 training + at least 1 holdout) and refuses earlier. |
| A-17 | Campaign projections lost exact excited-state/gap anchors, reaction molecule/reaction labels, conjecture laws/known-as, and result versions. | P1 result truth | Added typed fields and fold logic for all four result families. |
| A-18 | H2O reaction ignored the requested layer count. | P2 reproducibility | `layers` is now forwarded to both endpoints. |
| A-19 | Spin-chain hardware returned energy but not per-term measured expectations or CI95. | P1 result truth | Added per-term measurements, `ci95`, and the label-convention receipt. |
| A-20 | Hardware QASM containing reset was refused only by `state_mode`; a malformed VQE source could still carry reset. | P1 governance | The bridge now refuses any spin hardware QASM containing reset. |
| A-21 | H2O reaction compared an active-space water measurement to a full-FCI reaction anchor without saying so. | P1 truth | It now reports active-space and full-molecule anchors separately and scopes any claim to the mixed active-space/exact-atom model. |
| A-22 | Provenance tenant writes re-read the process environment for their base path while queries used the module ledger's base; a test/state-root change could write and read different files. | P1 test isolation/truth | Tenant shards are now bound to the module ledger base, so a write and its query share one path. |

## 3. Current capability matrix

| Capability | State | Evidence class |
|---|---|---|
| H2, LiH, BeH2, H2O active-space VQE | Working | Shot-measured Aer; PySCF anchors |
| Caller-supplied custom-geometry VQE | Working, bounded | Explicit atom/ncas/nelecas; CASCI + full FCI; 12-qubit active cap |
| Geometry optimization | Working, bounded | One curated internal coordinate; VQE at optimized geometry |
| Repeated VQE variance | Working | Mean shot energy + between-run standard error |
| LiH/BeH2 atomization reactions | Working | Active-space endpoint + exact atomic FCI products |
| 2D TFIM quench dynamics | Working, finite-size | Exact `expm_multiply`; not a QPU measurement |
| Signed experiment export | Working | Server-built payload + Ed25519 envelope/receipts |
| Full-space LiH/BeH2 ADAPT/CEO | Working with target/limits | Shot-measured; contested status is preserved |
| H2/LiH/BeH2/H2O PES | Working | Per-point measured energy/uncertainty/FCI |
| H2/LiH/BeH2/H2O active-space excited state | Working | Shot-measured; active-space anchors |
| H2 → 2H and H2O → H2 + O reaction energy | Working | Measured endpoints; exact atom anchors |
| 1D TFIM/Heisenberg/XXZ spin chain | Working | Exact diagonalization + shot-measured VQE/exact-state readout |
| Mathematically exact finite-prefix law discovery | Working, bounded | Exact rational training/holdout; not a proof |
| QAOA MaxCut | Working on fixed 8-node graph | Aer + governed hardware final state |
| Governed hardware VQE/QAOA/spin | Working | Mandate/fidelity/settlement/vendor receipts |
| Discovery protocol | Working, bounded | Deterministic planner, 1–5 real jobs |
| Full browser science E2E | Working for supported kinds | Real compose stack |
| Manual VoiceOver/NVDA | Not run | Human-only residual |

## 4. Remaining gaps

### P0 — blocks a broad production science claim

1. **Hardware spin-chain accuracy is not established.** The live VQE run was
   executed and settled, but measured `-2.278125` Ha against exact
   `-3.00902212` Ha; `claimable:false`. Execution is complete; an accuracy
   claim requires a better ansatz/optimizer/shot budget and a new measured run.
2. **Chemistry is broader but still bounded.** Curated H2/LiH/BeH2/H2O,
   caller-supplied closed-shell STO-3G geometry, atomization reactions,
   one-coordinate geometry optimization, and repeated VQE are supported. There
   is no SMILES conversion, open-shell/charged generic path, full
   multidimensional relaxation, N2/CO/HF suite, or general reaction network.
3. **Physics breadth is finite-size.** 1D TFIM/Heisenberg/XXZ and exact 2D
   TFIM quench dynamics are implemented. There is no DMRG, thermodynamic-limit,
   spectral-function, or hardware dynamics path.
4. **Mathematical discovery is not a proof/novelty system.** It discovers
   exact finite-prefix polynomial/recurrence laws with a holdout and falsifier.
   It does not prove theorems, search external literature, or establish
   novelty; `unclassified` is the correct boundary.
5. **Result truth is not yet one shared object end-to-end.** The versioned
   JSON envelope, TypeScript interface, signed result receipt, and typed
   campaign projection exist, but campaign state, exports, and UI still carry
   projections. A single canonical `ExperimentResult` should be adopted across
   bridge → Go job → campaign → export → UI.
6. **The host forensic ledger is not a verified restore source.** The existing
   host database has historical chain faults from legacy unsigned deletes.
   Production must start from a fresh volume; the history must not be rewritten.

### P1 — required for a complete researcher workflow

- PES does not request mitigation per point and has no hardware PES path.
- No repeated-run variance/batch campaign across a molecule family.
- Negative/failed discovery steps are recorded in the protocol ledger but are
  not folded into the campaign notebook as negative results.
- Hardware cost preview is a cap disclosure, not a vendor price estimate.
- Manuscript/report generation still centers molecular VQE; the generic
  experiment report is an additional surface, not a replacement.
- Non-executable/planned providers and legacy proposal surfaces still need a
  harder default-picker boundary.
- A single operator is the complete deployment mode; multi-lab roles and
  first-run onboarding are not fully separated.
- Manual screen-reader testing and a signed residual-risk waiver are still
  outstanding.

### P2 — enhancements

- 2D Hubbard/DMRG, time evolution, Loschmidt echo, spectral functions.
- General molecular geometry optimization and larger basis/active-space support.
- Hardware spin-chain calibration-aware mitigation and repeated-shot statistics.
- External math catalog/literature search and counterexample search beyond the
  holdout falsifier.
- OTel/Sentry, chaos/load baselines, multi-host/HA, public plugin marketplace.
  These are outside the local-private scope and must not be advertised as live.

## 5. Improvements landed in this audit pass

- Corrected reaction stoichiometry and separate active/full anchors.
- Corrected spin hardware Pauli basis mapping and added per-term measurements.
- Corrected ZNE claimability and added a below-anchor truth guard.
- Corrected focus-state observables and conjecture input-domain refusal.
- Hardened hardware source status/type/mode checks and added `source_job_id`
  to VQE/QAOA parent bindings.
- Removed the H2O hardware admission mismatch and the loop+mitigation ambiguity.
- Exposed measured experiments before a molecular notebook exists.
- Added a generic measured experiment report/JSON export and protocol history.
- Added a ZNE control and explicit hardware-cap disclosure.
- Preserved result version/labels/anchors in the campaign projection.

## 6. Updated implementation plan

### Phase 0 — Truth and governance (completed in this pass)

Exit criteria met:

- no reaction sign error, no silent ZNE claim, no below-anchor pass;
- exact-state hardware refuses before admission;
- source jobs are completed and type-checked before parent preparation;
- parent bindings include the source job;
- campaign projections preserve exact anchors and result versions.

### Phase 1 — Canonical result adoption (landed in this pass)

1. **Landed:** dependency-free Go `experimentresult` decoder validates version,
   job-kind match, per-kind required fields, finite numbers, and receipt shape
   at the jobs boundary.
2. **Landed:** completed jobs add a campaign `result_receipts` reference
   (job id, kind, result version, ledger receipt); the console renders the
   ledger/result hashes. The campaign still keeps readable projection fields
   for one release.
3. **Landed:** failed experiment jobs record a negative-result campaign entry
   with status, reason, measured=false, claimable=false; duplicate failure folds
   are idempotent.
4. **Landed:** `/api/sessions/{id}/experiment-export` returns a server-built
   payload (campaign + experiment jobs/results) with a canonical SHA-256 and an
   Ed25519 envelope; the console exposes **Signed export**. Underlying
   per-result receipts remain independently signed.

**Gate:** every citable result has one job id, one result version, one receipt,
and one campaign projection; no field is reconstructed from prose.

### Phase 2 — Science breadth

1. **Landed:** caller-supplied PySCF geometry VQE with explicit `ncas`/
   `nelecas`, closed-shell STO-3G, 8-spatial-orbital full-FCI cap, and 12-qubit
   active-space cap. Unsupported geometries refuse.
2. **Landed:** one-coordinate geometry optimization and LiH/BeH2 atomization
   reactions with exact atomic FCI products; full-molecule and active-space
   anchors remain separate.
3. **Landed:** exact finite-size 2D TFIM quench dynamics with explicit
   `measured:false`; DMRG/thermodynamic-limit and hardware dynamics remain open.
4. **Partially landed:** repeated-run variance is implemented. PES mitigation
   opt-in remains open.
5. **Partially landed:** conjecture jobs can opt into an OpenAlex lookup that
   records matching titles/DOIs or an explicit `unavailable` result. It still
   does not establish novelty; `unclassified` remains the default boundary.

**Gate:** every advertised model has an exact anchor, a measured result, a
claimability gate, and a documented unsupported boundary.

### Phase 3 — Hardware and operations

1. Hardware PES/mitigation per point and calibration-aware spin runs.
2. Repeat the live spin-chain run with a better ansatz/shot budget; record the
   result as claimable only if the gate passes.
3. Vendor price preview from the reviewed rate card; keep the signed cap.
4. Fresh-volume release acceptance, restore drill, and secret/ledger custody
   verification on the exact shipped commit.
5. Manual accessibility run or explicit signed waiver.

**Gate:** one operator can install, run, back up, restore, and operate the
supported science surface without hidden shell steps or unsupported claims.

## 7. Verification

| Gate | Result |
|---|---|
| Go full suite | pass (all packages, including canonical decoder/negative folds) |
| Python full suite | 1262 passed, 36 skipped (35:18) |
| Bridge targeted science suite | 48 passed (science/custom/lattice/reaction/conjecture files) |
| Console tests | 605 passed; tsc/eslint/build pass |
| Console tsc/eslint | pass |
| Console production build | pass with `NEXT_PUBLIC_KUBONA_CONSOLE_TOKEN` unset (the build intentionally refuses a browser bearer) |
| Contract inventory / OpenAPI | `140 modeled / 14 client-only / 0 unmodeled`, `128 paths / 121 schemas` |
| Wiki lint | clean |
| Real-stack E2E | green at `fee14b4` |

### 7.1 Live functional checks

The local compose stack was rebuilt from `fee14b4` with the repository `.env`
plus the host secret env, and exercised through the real HTTP API:

- **Custom-geometry VQE job `04ee3265-88bd-446e-a4db-109762cc9b29`**
  completed on the bridge with `molecule:"custom"`, `generic:true`,
  active-space energy `-1.046441054576304 Ha` against full FCI
  `-1.137270174660903 Ha`, uncertainty `10.79 mHa`, `claimable:false`,
  `measured:true`, and a signed `result_receipt`. The campaign response
  contains that job in `result_receipts` with `signed:true`.
- **Refused custom basis job `6a9da3e7-b29f-48e0-ac52-48dfc622f3bf`** reached
  terminal `failed` with `generic VQE currently supports basis=sto-3g only`;
  the campaign contains a negative-result entry with `status:"failed"`,
  `measured:false`, and `claimable:false`.
- **Signed export** returned `experiment-export-v1` with 22 stored jobs,
  `envelope.signed:true`, a payload SHA-256, and an Ed25519 signature.
- **Geometry optimization `19df6850-658b-452f-80bc-27aa9f23bed1`** completed at
  `R=0.734833 Å` with measured `-1.105165 Ha` vs FCI `-1.137306 Ha`, not
  claimable.
- **Repeated VQE `6183b5db-e351-4657-b07c-3e99e2523bd5`** completed two runs;
  mean `-1.115140 Ha`, between-run SE `9.945 mHa`, not claimable.
- **2D quench `ed96a702-728d-4521-88f8-ee048ceea5fd`** completed exact
  finite-size dynamics with three time points and `measured:false`.
- **LiH reaction `a6439e9a-c84b-41b0-a621-05baa4bbacac`** completed
  `LiH -> Li + H` with measured `-0.067674 Ha` against active-space anchor
  `-0.079846 Ha`, not claimable.

## 8. Operator actions

- Do not cite historical live hardware spin accuracy as a pass; it is a
  measured non-claimable run.
- Start production on a fresh state volume; do not rewrite the forensic host
  database.
- Run the manual screen-reader script or sign the residual-risk waiver.
- If a vendor price is required before approval, add it to the reviewed rate
  card; do not infer a price from shots.

## 9. File map

- Science core: `platform/kubona/science/lattice.py`,
  `platform/kubona/science/conjecture.py`
- Bridge runners: `platform/bridge/experiments.py`,
  `platform/bridge/lattice_experiment.py`
- Go API/admission: `platform/core/pkg/api/experiments.go`,
  `platform/core/pkg/jobs/hardware_experiment.go`,
  `platform/core/pkg/api/discovery_protocol.go`
- Campaign projection: `platform/core/pkg/agents/campaign_synth.go`,
  `platform/core/pkg/agents/replan.go`
- Console: `HardwareLaunchPanel.tsx`, `ExperimentPanel.tsx`,
  `DiscoveryProtocolPanel.tsx`, `ResultsStudio.tsx`, `types/index.ts`
- Contracts: `platform/contracts/experiment-result-v1.schema.json`,
  `platform/contracts/console-contract-inventory.json`,
  `platform/contracts/console-openapi.json`
- Tests: `platform/tests/test_lattice.py`, `test_conjecture.py`,
  `platform/bridge/test_experiments.py`, `test_lattice_experiment.py`,
  `platform/core/pkg/jobs/jobs_test.go`,
  `platform/console/src/components/**/*.test.tsx`
