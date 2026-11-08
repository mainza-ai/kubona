# Results Presentation & Research UX Audit — 2026-11-08

**Scope.** How Kubona presents experimental results to a researcher: campaign
cards, measurement/proof/evidence panels, charts, job records, manuscript and
Results Studio, tribunal output, codesign output, operations telemetry, and the
machine-readable export path. Audited against the live Docker stack at
`4cb5e7c` with a real H2 VQE run, H2 PES scan, tribunal, codesign run, and
QPU/Open-Plan probes.

**Verdict.** The result **data exists** and is mostly ledger-bound, but the
researcher-facing presentation is not yet production-grade. The same live
result is presented with contradictory labels, missing uncertainty, a false
convergence claim, a mathematically invalid "best" PES point, and several
placeholder zeros rendered as measured values. The biggest gap is not the
backend; it is the **result contract and its projection into the UI**.

The implementation report is
[`real-experiment-production-audit-2026-11-08.md`](real-experiment-production-audit-2026-11-08.md).

> ⚑ Implementation update (later 2026-11-08): R0 is implemented. The campaign
> now carries a typed `measurement` and `pes_points`; the workbench shows
> uncertainty, convergence, backend, shots, seed, claim, and a below-FCI
> admissibility flag, and renders a VQE/FCI PES chart. PES summaries separate
> minimum energy from closest-to-FCI. Placeholder zeros use the
> not-evaluated sentinel. The Proof DAG labels a synthesized proposal as
> proposed rather than measured. Results Studio generates uncertainty-aware
> prose and offers a result-JSON export. Live acceptance session:
> `de02c225-5a8d-4341-bc3b-a08c14ad6218` (measurement
> `below_fci:true`, uncertainty `1.384 mHa`, claim `clm-de02c225-0001`,
> spend `$0.00`; PES minimum `R=0.7 Å`, closest-to-FCI `R=0.9 Å`).
> The remaining R1 items are structured CSV/PES export, claim-history/review
> UI, optimization-history retention, and generated-prose numeric guards.

---


> ⚑ R1 implementation (later 2026-11-08): Results Studio now renders the
> session claim history and can seal a `peer_review` request; it exports the
> measurement or PES points as CSV in addition to JSON; the campaign retains
> the optimizer trajectory (`measurement.optimizer_history`) and renders the
> point count; and a deterministic numeric-claim guard withholds generated
> prose if it contains a number absent from the result record. The PES table
> shows R, measured energy, FCI, Δ, uncertainty, and chemical-accuracy status.
> Remaining R1: claim-history pagination/filtering and a reviewer decision
> state (the claim record itself remains immutable).

## 1. Live result under test

Final H2 VQE, session `69df2967-8599-476f-9274-f921afcfa777`:

| Field | Value |
|---|---|
| VQE job | `61556f56-7afe-4310-8ef4-5564b330ec24` |
| Measured energy | `-1.1377829714403882 Ha` |
| Exact optimizer energy | `-1.137207003000865 Ha` |
| FCI anchor | `-1.137270174660903 Ha` |
| Shot uncertainty | `0.0013835463 Ha` (`1.384 mHa`) |
| Measured Δ vs FCI | `0.5127967795 mHa` |
| Exact optimizer Δ vs FCI | `0.0631716600 mHa` |
| Shots | `16384` |
| Backend | `qiskit-aer-simulator` |
| Optimizer | COBYLA, 80 evaluations, `success:false` |
| Claim | `clm-69df2967-0001` (`verified`) |
| Provenance | `sha256:bd494b23…` |

**Critical scientific observation:** the *measured* point is **below** FCI by
`0.513 mHa`, which is impossible for an exact variational expectation. It is
within the `1.384 mHa` shot uncertainty, so it is a statistical excursion, not
proof of a Hamiltonian error — but the UI does not show the uncertainty and
therefore turns a noisy point into an apparent variational-principle violation.
The tribunal then escalated that to “variational principle violation
confirmed.” That is not a valid conclusion from a single shot estimate.

H2 PES, job `1680c6fb-9e11-4c44-9364-b4b6bc8856ae`:

| R (Å) | Measured E (Ha) | Δ vs FCI (mHa) | Uncertainty (mHa) |
|---|---|---|---|
| 0.6 | -1.115293 | 0.993 | 1.929 |
| 0.7 | -1.137136 | 0.946 | 1.867 |
| 0.8 | -1.136001 | 1.854 | 1.921 |
| 0.9 | -1.121251 | 0.691 | 1.996 |

The **minimum energy** is at `R=0.7 Å`; the **closest-to-FCI** point is
`R=0.9 Å`. The UI selected `R=0.9` and labels it “best”, which conflates
“smallest error at this geometry” with “best energy on the PES.”

Only the final VQE point (and each PES point) is folded into the campaign
trajectory. The job result contains **80 optimizer evaluations**; the campaign
shows one. The full optimization history is lost from the primary research
surface.

---

## 2. Surface-by-surface findings

### 2.1 Workbench campaign card

Observed:

- `Status: COLLAPSED_VERIFIED`
- `Anchor: computed`
- `Spend: $0.00`

**Good:** claim state, anchor status, and settled spend are visible and
truthful.

**Gaps**

1. No result headline with uncertainty, backend, shots, or claim ID. A reader
   must open five separate cards to reconstruct what ran.
2. No “last experiment” time or job link in the campaign card.
3. `Est. spend` remains a proposal field; it is not labelled as an estimate
   versus settled spend in context.
4. No method/ansatz/active-space summary at the campaign level.

### 2.2 Hypothesis card

Observed:

- Statement: `VQE measured -1.137783 Ha vs FCI -1.137270 Ha (Δ=0.513 mHa).`
- `State: COLLAPSED_VERIFIED`
- `Corroborated α: 0.000`, `Contested β: 0.000`, `Unverified γ: 0.000`

**Gaps**

1. **Zero amplitudes are rendered as measured data.** The campaign
   `EpistemicState` fields default to zero; the claim record uses the `-1`
   “not evaluated” sentinel. `0.000` is not a measured credibility amplitude
   and reads as certainty of “no corroboration/contestion.”
2. The card does not show that the measured point is below FCI nor that the
   uncertainty covers it.
3. It does not cite the claim record (`clm-69df2967-0001`) or the ledger entry
   hash.
4. It does not distinguish **measured** energy from **exact optimizer** energy.
5. `converged:false` is nowhere visible.

### 2.3 Circuit card

Observed:

- `UCCSD Parameterized`
- proposed QASM is the campaign template (`h`, `rz`, `cx`) and is not the VQE
  ansatz actually executed.

**Gaps**

1. **The executed VQE ansatz is not displayed.** The card shows the proposal
   template and calls it `UCCSD Parameterized` even though the runner used a
   hardware-efficient RY/RZ/CX ansatz.
2. No parameter vector, layer depth, entangler map, or optimizer settings.
3. No backend/transpilation details.
4. No distinction between “proposed” and “executed” circuit.

### 2.4 Measurement card

Observed:

- `-1.13778 Ha VQE against -1.13727 Ha FCI`
- `Δ 0.51 mHa`
- `Accuracy PASS`
- `UNCERTAINTY: not recorded`
- Basis text: *“Δ is the recorded VQE−FCI discrepancy in mHa, not an
  uncertainty.”*

**Gaps**

1. **Uncertainty is present in the job result but hardcoded away** in
   `workbench-model.ts` (`value: 'not recorded'`).
2. No error bar or confidence interval; no indication that the measured point
   is below FCI.
3. No backend, shots, seed, execution time, claim ID, or job ID.
4. No separation of exact optimizer energy (`-1.137207`) from shot-measured
   energy (`-1.137783`).
5. `Accuracy PASS` is shown for a measured point that is **below** the
   variational lower bound; the label should be qualified by uncertainty and
   lower-bound admissibility.
6. “Source: pyscf-live” describes the anchor, not the VQE measurement; the
   measurement source/claim record is absent.

### 2.5 Evidence card

Observed:

- `COLLAPSED_VERIFIED`
- `Corroborated α 0.000`, `Contested β 0.000`, `Unverified γ 0.000`
- `P(correlated) 0.000`
- `Literature drift 0.00 mHa`
- `Sources 1`
- Source: `bridge:vqe_run:61556f56…`

**Gaps**

1. **Zeroes and 0.00 mHa are placeholders, not measurements.** They must render
   as “not evaluated”/“unavailable” unless a real evaluator produced them.
2. `P(correlated)` has no evaluated input; showing `0.000` is a fabricated
   confidence.
3. `Literature drift` was never computed; `0.00 mHa` is an invented measured
   value.
4. The source is a bridge/job reference, not a resolvable claim/ledger record.
5. No claim history, contest/reopen control, or evidence consumers.

### 2.6 Proof card

Observed:

- `Provenance recorded`
- `sha256:bd494b23…`
- `Verdict VERIFIED`, `Chemical accuracy PASS`

**Good:** the provenance hash and verdict are present; “no provenance receipt”
is no longer shown after a measured run.

**Gaps**

1. No claim record ID, ledger entry hash, job ID, or evidence reference.
2. Hash is a single campaign-level hash; no per-measurement/per-point hashes.
3. “Chemical accuracy PASS” is not qualified by uncertainty or the
   below-FCI excursion.
4. No link to the ledger verification view or export bundle.

### 2.7 Notebook card / manuscript

Observed:

- Notebook sections are strings:
  - `Job 61556f56 (vqe_run): VQE measured E=-1.137783 Ha ± 0.001384 Ha vs FCI -1.137270 Ha (Δ=0.513 mHa; within chemical accuracy). [clm-69df2967-0001]`
  - `Job 1680c6fb (vqe_pes): VQE PES: 4 points, best E=-1.121251 Ha at R=0.9000 Å (Δ=0.691 mHa vs FCI).`
- Manuscript abstract: *“the optimization converged to an energy of …”*
- Results Studio shows `PROJECTED RESEARCH DRAFT` and
  `Formal Verification & Provenance: Unverified`.
- `converged:false` in the job result.

**Gaps**

1. **False convergence prose.** The manuscript says “optimization converged”
   whenever `COLLAPSED_VERIFIED`, regardless of the optimizer result. The
   actual result is `converged:false` (COBYLA evaluation cap).
2. **Notebook sections are unstructured strings.** The UI cannot render an
   error bar, table, or reproducible method from them.
3. **PES “best E” is semantically wrong**: it selects smallest per-point Δ
   (0.9 Å) and calls it “best E”; the energy minimum is 0.7 Å.
4. The notebook has no claim ID field; the ID only appears inside a string.
5. No uncertainty, backend, shots, seed, method, or convergence in the
   manuscript's methods/results.
6. “PROJECTED RESEARCH DRAFT” and `COLLAPSED_VERIFIED` can appear together;
   the labels are not explaining what is projected versus verified.
7. `Lean 4` is correctly labelled unverified, but the Results Studio tagline
   (“Lean 4 AST synchronization”) can imply a formal proof path that has not
   run.

### 2.8 Charts

Observed:

- `Energy trajectory` x-axis is `Iteration` (1–5), mixing one VQE final point
  with four PES points.
- FCI line is flat; no error bars.
- PES points have no bond-distance axis and no FCI curve by geometry.

**Gaps**

1. **Wrong x-axis for PES.** A PES is energy vs bond distance, not iteration.
2. **VQE optimizer history is lost.** The job has 80 evaluations; the campaign
   folds one.
3. No uncertainty bands/error bars, no shots/seed/backend annotations.
4. The chart description says “measured trajectory”; the PES points and VQE
   point are not the same experiment and should not share one iteration axis.
5. No per-point residual or table view.
6. No CSV/JSON download of the plotted series.

### 2.9 Jobs / measured outputs

Observed:

- `vqe_run`: `VQE -1.137783 Ha vs FCI -1.137270 Ha · Δ 0.513 mHa ·
  qiskit-aer-simulator · job 61556f56`
- `vqe_pes`: `VQE PES 4 points · best R=0.9000 Å · job 1680c6fb`
- Workbench job cards show `Backend: not recorded` for both VQE jobs.

**Gaps**

1. `MeasuredOutputs` shows backend for VQE but not uncertainty, shots, seed,
   exact energy, or claim ID.
2. The Workbench job card reads backend from `params.requested_backend`; VQE
   params do not contain it, so a real backend in `result_json` is shown as
   “not recorded.”
3. PES “best R” uses per-point Δ, not the energy minimum.
4. No click-through to full job result/terms/theta from the summary row.
5. No job comparison or repeated-run variance.

### 2.10 Proof DAG

Observed:

- `MEASURED · Measured campaign DAG · 4 nodes · 2 edges`
- Nodes are `q[0]: |0⟩`, `[Rz(θ1)]`, `q[1]: |0⟩`, `[Rz(θ2)]` — the campaign
  proposal template, not the executed VQE ansatz.
- Empty `Compile receipt (QASM → QIR via backend)`.

**Gaps**

1. **Proposed circuit is presented as measured.** The badge says MEASURED but
   the nodes are the proposed template; no VQE energy, job, or claim node.
2. No anchor/compiler/execution/measurement/claim node lineage.
3. No layout/backend/calibration/ledger hashes on edges.
4. The compile receipt is empty even though a native Rust QIR compiler and a
   Qiskit execution exist for the VQE path (the VQE path currently bypasses
   the QIR compiler).
5. “Select a node to trace its record” is offered but the current nodes do not
   carry record IDs.

### 2.11 Tribunal / transcript

Observed:

- Four authored turns, consensus true, claim `clm-69df2967-0002`.
- Device Auditor transcript says the measured VQE is below FCI by 0.513 mHa
  and calls it a **variational principle violation confirmed**.
- Epistemic Scribe says `COLLAPSED_VERIFIED` applies only to workflow
  execution, not physical correctness.

**Gaps**

1. The tribunal treats a point estimate as exact and ignores the 1.384 mHa
   shot uncertainty. “Confirmed violation” is not supported.
2. It correctly notes **no quantum advantage** and that the simulator is
   classical; that should be promoted to the main result card.
3. The tribunal’s consensus/claim can be read as validating physics even when
   it is validating workflow/process.
4. No explicit claim status for “statistically consistent with FCI” versus
   “below FCI point estimate” versus “physically inconsistent.”
5. No human-readable uncertainty/error bar in the transcript table.

### 2.12 Results Studio tabs and export

Observed:

- Tabs: Manuscript, LaTeX Source, Lean 4 Proof AST, Literature, Cite BibTeX,
  Export.
- Lean tab honestly reports no proof receipt.
- Export downloads Markdown only.
- Session export API returns a signed manifest (`claims`, `events`,
  `campaign_state`, `python_ledger`, signature).
- LaTeX/BibTeX are generated from campaign/notebook values.

**Gaps**

1. No structured result export: JSON/CSV count tables, per-point measurements,
   uncertainties, term expectations, theta history, or raw job results.
2. Markdown export omits uncertainty, backend, shots, seed, method, optimizer
   result, claim ID, and evidence refs.
3. BibTeX uses a provenance hash as `eprint`; no claim/record ID, no author
   attribution from the actual run.
4. `Lean 4` tab cannot link to a proof receipt because no prover run exists;
   the UI is honest but should not imply formal verification is part of the
   current results pipeline.
5. No “download signed manifest” button in Results Studio; the API exists but
   the researcher surface does not expose it.
6. No “export selected result bundle” that bundles result JSON, claim record,
   ledger entries, notebook, and manuscript.

### 2.13 Operations / fleet

Observed:

- Local simulators show `DEGRADED` despite `dispatch_ready:true`.
- IBM shows `READY` with `qubits —`, `queue —`, `T1 —`.
- Python bridge shows `OFFLINE / DISCONNECTED` because radar probes TCP
  `:50051`; the production bridge is UDS-only and healthy.
- Calibration age/queue are not surfaced.

**Gaps**

1. **Bridge health is wrong in production-shaped deployment.** UDS bridge
   cannot be discovered by a TCP port check; show UDS health instead.
2. Local simulator “DEGRADED” mislabels a usable, zero-cost simulator.
3. IBM readiness lacks queue depth, calibration age, and plan (Open Plan).
4. No link from fleet status to the last calibration refresh or QPU job.
5. “Tier 2 AWS Braket/CUDA-Q” is listed as a declared continuum although the
   repository says those are not integrated; verify labels/link to status.

### 2.14 Machine-readable API

Observed:

- `/api/jobs/{id}` returns full VQE result with `uncertainty`, `energy`,
  `energy_exact`, `theta`, `trajectory`, `measurements`, `backend`, `shots`,
  `seed`, `converged`.
- `/api/campaigns/{id}/flight-recorder` returns campaign state and claims.
- `/api/epistemics` now prefers claim records.
- `/api/sessions/export` returns a signed migration manifest including claims
  and events.

**Gaps**

1. No `/api/experiments/{id}` canonical result resource; result is embedded in
   the job row.
2. Campaign state does not persist uncertainty/backend/shots/seed/method/
   convergence; only strings in the notebook.
3. No claim-detail endpoint; claims are available only via flight recorder
   `last_claims` or the export manifest.
4. No versioned result schema; the UI and API each reinterpret `result_json`.
5. No `converged` field in campaign/notebook/manuscript; UI infers wrongly.
6. No `variational_admissible`/`below_fci`/`within_ci` field.
7. No structured `measurement` table or `per_term` data for the UI.
8. No per-point PES `bond_distance` in the campaign trajectory; only job
   result points.
9. Export manifest is comprehensive but not discoverable from the Results UI.

---

## 3. Gap register

### P0 — scientific truth and researcher trust

| ID | Gap | Evidence |
|---|---|---|
| R-01 | Measured VQE point below FCI is presented as `Accuracy PASS` without uncertainty/lower-bound warning | Measurement card; `-1.137783` < `-1.137270`; uncertainty omitted |
| R-02 | Manuscript asserts “optimization converged” while result is `converged:false` | Results Studio abstract; VQE result `success:false` |
| R-03 | Uncertainty present in result is shown as “not recorded” and omitted from claim/proof/manuscript | `workbench-model.ts`; UI measurement card |
| R-04 | Epistemic amplitudes/P(correlated)/literature drift render placeholders as measured zeros | Hypothesis/Evidence cards |
| R-05 | PES “best E” selects smallest per-point Δ (`R=0.9`) not minimum energy (`R=0.7`) | Notebook/MeasuredOutputs; PES points |
| R-06 | Proof DAG labels proposed template as measured and has no result/claim nodes | Proof DAG body |
| R-07 | Tribunal states “variational principle violation confirmed” from a shot point estimate | Tribunal transcript |

### P1 — missing result functionality

| ID | Gap |
|---|---|
| R-08 | No structured result contract (energy, exact, uncertainty, backend, shots, seed, method, convergence, claim, feasibility flags) |
| R-09 | Campaign/notebook lose full VQE optimizer trajectory (80 evaluations → 1 point) |
| R-10 | PES chart lacks bond-distance axis, FCI curve, per-point error bars |
| R-11 | Measurement/Proof/Job cards omit claim ID and ledger entry hash |
| R-12 | Job cards show “Backend: not recorded” although result has backend |
| R-13 | Circuit card shows UCCSD proposal, not executed hardware-efficient ansatz |
| R-14 | No raw result/export bundle (JSON/CSV/manifest) in Results Studio |
| R-15 | No claim-detail UI/API surface; claims only in flight recorder/export |
| R-16 | Operations reports UDS bridge as offline and local sim as degraded |
| R-17 | No per-measurement/per-point provenance; one campaign-level hash |
| R-18 | No repeat-run variance/confidence interval or optimizer diagnostics surface |
| R-19 | No experiment submission form in console (API only) |
| R-20 | No human review/accept/reopen flow for claims beyond frame annotations |

### P2 — research UX and enhancements

- Comparison view: VQE vs FCI vs exact optimizer vs hardware, with residuals.
- Error-bar chart component and table of per-term expectations.
- PES explorer with R slider, point table, CSV export, and minimum-energy vs
  closest-accuracy distinction.
- Manuscript sections: methods, results with uncertainty, limitations,
  reproducibility, claim/evidence IDs, no unverified convergence prose.
- Claim timeline (sealed → evaluated → asserted/verified/contested → reopened).
- “What ran” summary: backend, shots, seed, layout, calibration, cost, usage.
- Result badges for `measured`, `inferred`, `projected`, `simulated`, `QPU`.
- Direct link to `/api/sessions/export` signed manifest and ledger verification.
- Repeat experiment button with same spec/seed/different seed.
- Result review comments tied to chart points/table rows.
- Accessibility: chart data table fallback, color-independent uncertainty
  encoding.

---

## 4. Implementation plan

### Phase R0 — Truth before beauty (P0)

1. **Define a versioned `ExperimentResult` schema** shared by bridge, Go,
   campaign state, notebook, API, and UI:
   ```
   result_version, job_id, claim_id, spec, method, ansatz, backend, shots,
   seed, energy_measured, energy_exact, uncertainty, delta_mha, ci95,
   converged, optimizer, theta, trajectory, measurements[], counts,
   layout, calibration, usage, cost, feasibility{below_fci, within_ci},
   provenance_hash, evidence_refs[]
   ```
   Persist it as a structured object, not only notebook strings.
2. **Claim gate with uncertainty**:
   - `chemical_accuracy` requires `|Δ| <= 1.6 mHa` **and** a documented
     confidence interval.
   - If `energy < fci - 3σ`, mark `CONTESTED`/`INADMISSIBLE` and refuse
     “verified”; if within noise, label “within uncertainty; point estimate
     below FCI”.
   - Never render `PASS` without the uncertainty and the feasibility flag.
3. **Convergence truth**: store `converged` and `optimizer.success`; UI and
   manuscript must say “stopped at evaluation cap” when false. Remove the
   unconditional “converged” template.
4. **Zero/placeholder discipline**: render `not evaluated`/`unavailable` for
   α/β/γ, P(correlated), literature drift, uncertainty, backend, queue, T1,
   etc. Never render `0.000` as a measured value.
5. **PES semantics**: choose `minimum_energy_point` and
   `closest_to_fci_point` separately; labels must say which. Add bond distance
   to trajectory/measurement; plot energy vs R with an FCI curve.
6. **Correct operations health**: UDS bridge health from the same socket
   healthcheck; local simulators `READY`, not `DEGRADED`; surface calibration
   age and queue.

### Phase R1 — Researcher-facing result surface (P1)

1. **Campaign result header**: one card with claim status, measured energy ±
   uncertainty, Δ with CI, backend, shots, seed, claim ID, job link.
2. **Measurement/proof/evidence cards**: structured facts, not prose; claim ID;
   ledger hash; `not evaluated` semantics; explicit exact-vs-measured.
3. **VQE convergence chart**: full job trajectory, error bars, FCI band,
   optimizer stop reason; theta/gradient optional.
4. **PES explorer**: energy vs R, FCI curve, error bars, table, CSV, minimum
   energy and closest-accuracy labels.
5. **Proof DAG**: anchor → proposed ansatz → compiled/executed job →
   measurement → claim; proposed vs executed styles; record IDs on nodes; no
   “measured” badge on a proposal.
6. **Job detail**: backend, shots, seed, method, ansatz, compiler, calibration,
   layout, usage, cost, raw result access.
7. **Export**: signed session manifest + result JSON/CSV + Markdown/LaTeX/
   BibTeX, with claim/evidence IDs and uncertainty.
8. **Operations**: UDS health, calibration age, queue/plan, simulator ready.
9. **UI**: experiment submission form (VQE/PES/QAOA) with spec preview and
   budget; no hidden API-only path.

### Phase R2 — Scientific comparison and review (P1/P2)

1. Repeat-run and seed-variance view; bootstrap CI.
2. VQE vs exact optimizer vs FCI vs hardware comparison.
3. Error-mitigation raw/mitigated/inferred labels with fit residual.
4. Claim timeline and human review (accept/reopen/annotate) tied to result
   points.
5. Manuscript generator from structured results, with explicit limitations
   and no claim without evidence.
6. Literature/DOIs linked to claim IDs.

### Phase R3 — Agent/AI guardrails

1. Tribunal/manuscript prompts must receive uncertainty, convergence, backend,
   and `below_fci` flags; they must not assert “violation confirmed” from a
   point estimate.
2. Post-generation validator rejects numeric claims not present in the result
   record.
3. Add tests that a below-FCI/no-uncertainty result cannot produce a
   “verified” prose or a “converged” abstract.
4. Keep `Lean 4` unverified until a real proof receipt exists; no tagline that
   implies formal verification by default.

---

## 5. Acceptance tests

1. H2 VQE result card shows:
   `-1.137783 ± 0.001384 Ha` measured, exact `-1.137207 Ha`, FCI
   `-1.137270 Ha`, `Δ 0.513 mHa`, CI, backend, shots, seed, claim ID.
2. Because the point is below FCI, the card shows either
   `within uncertainty; point estimate below FCI` or `CONTESTED`; it never
   shows an unqualified `PASS`.
3. `converged:false` is visible; manuscript says “optimizer stopped at 80
   evaluations” and does not say “optimization converged.”
4. PES chart x-axis is `R (Å)`, displays all four points with error bars and
   FCI curve; `R=0.7` is labelled minimum energy; `R=0.9` is labelled closest
   to FCI.
5. Proof DAG shows anchor/ansatz/job/measurement/claim; proposed ansatz is not
   labelled measured; compile receipt links to native QIR.
6. Evidence card shows `not evaluated` for α/β/γ/P(correlated)/drift, not
   `0.000`.
7. Results Studio can download result JSON/CSV and the signed session manifest;
   exports include uncertainty, claim ID, and evidence refs.
8. Operations shows UDS bridge healthy and local simulators ready; IBM shows
   plan/calibration/queue.
9. A below-FCI result cannot mint a “verified” claim or a “converged”
   manuscript without an explicit uncertainty/feasibility resolution.
10. Tribunal transcript does not claim a variational violation without
    statistical support.

---

## 6. Evidence file inventory

- Live UI DOM captures: Workbench, Evidence/Results Studio, Operations, Proof
  DAG, Tribunal, CodeSign.
- API: `/api/jobs/61556f56…`, `/api/campaigns/69df2967…/flight-recorder`,
  `/api/epistemics?session_id=69df2967…`, `/api/results/notebooks`,
  `/api/sessions/export`.
- Result values: as table in §1.
- Code paths: `workbench-model.ts`, `ResultsStudio.tsx`, `FlightRecorder.tsx`,
  `MeasuredOutputs.tsx`, `ProofDagView.tsx`, `replan.go`, `memory_writeback.go`.
