# Core scientific capabilities — audit, gaps, and implementation plan

**Date:** 2026-11-09  
**Audited head:** `386f857`; **implementation head:** `b4b3a3e` (hardware spin-chain, excited/reaction chemistry, measured discovery protocol; CI/E2E green)  
**Scope:** production local-private Kubona. One trusted host; no demo paths;
no invented measurements, claims, or scientific results.

> ⚑ **Correction pass (2026-11-09):** a follow-up audit found and fixed a
> water reaction sign error, a spin-hardware Pauli-order defect, a ZNE
> `claimable:true` path, a below-anchor claim leak, exact-state hardware
> admission at the Go layer, H2O hardware admission drift, missing source-job
> status/type checks, a missing source id in VQE/QAOA parent bindings, and
> several UI/result-projection gaps. The full finding register and updated plan
> are in [`production-audit-2026-11-09.md`](production-audit-2026-11-09.md).
> These corrections do not upgrade any historical result.

## 1. Executive verdict

Kubona is currently a **quantum-chemistry and combinatorial-optimization runner
with a governed evidence rail**, not yet a general quantum science discovery
platform. The working scientific spine is:

- H2, active-space LiH/BeH2, and full-space LiH/BeH2 VQE;
- H2/LiH/BeH2 PES;
- 8-node QAOA MaxCut;
- exact PySCF anchors and live Qiskit Aer measurements;
- QPU final-state and hardware-in-the-loop runs through a signed, governed rail;
- Krylov, ZNE, circuit cutting, and claim/notebook folding.

That is real and useful. It is not the same as being able to **discover** new
mathematics, chemistry, or physics. The missing capabilities are structural:

1. **Physics simulation is declared but absent.** The
   `materials_condensed_matter_v1` pack names
   `kubona.science.lattice.HubbardModel`; that module does not exist. The pack
   loader test even asserts the generator is currently unresolved. There is no
   spin-chain, lattice, spectral-gap, magnetization, correlation, or
   time-evolution experiment.
2. **Mathematical discovery is a catalog, not an engine.** The `openai/math`
   corpus and Comparator/Lean verification utilities exist, but there is no
   API/job that generates a conjecture, searches a law, finds a counterexample,
   or records a verified finite-domain result. `autonomous-mathematical-discovery`
   is a design page, not a live capability.
3. **Chemistry is limited to three curated molecules.** There is no general
   molecule input, no H2O/N2/CO support, no excited-state experiment, no
   reaction-energy workflow, no geometry optimization, and no property suite.
   Active-space VQE is real; the chemistry portfolio is not.
4. **Discovery is not a closed loop.** The campaign/notebook/claim rails fold
   measured results, but the campaign engine still runs a toy ansatz search
   rather than invoking the real VQE/PES/QAOA job rail. A researcher can run an
   experiment and see it folded; the platform cannot yet choose, sequence, and
   report an autonomous scientific campaign end-to-end.
5. **Evidence breadth is narrow.** There is no physics anchor beyond tabulated
   constants, no exact finite-domain theorem certificate, no novelty boundary,
   and no physics-specific claim grammar.

The production verdict is therefore: **the chemistry/QAOA spine is usable for a
controlled local-private run; the science-discovery product is not complete.**
The gap is capability breadth and closed-loop scientific method, not merely UI
or operations.

## 2. Capability matrix

| Capability | Current state | Evidence | Verdict |
|---|---|---|---|
| H2 VQE / PES | live simulator + governed hardware | `platform/bridge/experiments.py`; docs/live-qpu | working |
| LiH/BeH2 active-space VQE | live simulator | `_NATURE_ACTIVE`; job folds | working, limited |
| Full-space CEO/ UCCSD ADAPT | live simulator | `platform/bridge/adapt.py` | working |
| QAOA MaxCut | simulator + governed hardware | `run_qaoa*` | working, fixed graph |
| Exact classical anchors | PySCF live for curated molecules | `platform/bridge/chemistry.py` | working |
| Chemistry breadth | H2/LiH/BeH2 only (VQE); H2O/N2/CO/HF/etc. anchors only | `solve_classical_anchor` | **gap** |
| Excited states / reaction paths / geometry optimization | absent | no code | **gap** |
| Physics lattice simulation | **missing module** referenced by pack | `domain_packs/materials.../pack.yaml`; `test_domain_packs.py` | **P0 gap** |
| Spectral gaps / magnetization / correlations | absent as experiments | no code | **gap** |
| Time evolution / quench dynamics | absent | no code | **gap** |
| Math corpus catalog | static index/search | `science/math_catalog.py` | working catalog |
| Lean/Comparator verification | static AST + optional live kernel utility | `eval/comparator_eval.py` | utility, not an experiment |
| Conjecture generation / law discovery | absent | no code | **P0 gap** |
| Counterexample search | absent | no code | **gap** |
| Autonomous campaign over real experiments | toy harness only | `discovery/campaign.py` | **gap** |
| Provenance/claims/results | typed result + signed receipt + campaign fold | jobs/ledger/UI | working, partial |
| Domain packs | 3 manifests; materials generator unresolved | `science/domain_pack.py` | **P0 gap** |

## 3. Why these are production gaps, not feature requests

The production bar says a capability either performs real work or refuses. A
science-discovery platform that cannot run a physics model, cannot discover a
mathematical law, and cannot generalize chemistry is incomplete at the level of
its own purpose. Adding UI before the engines would create the exact
"demo-shaped" surface the project has rejected.

The correct order is:

1. **Executable models first** — each with an exact classical anchor and a
   measured result.
2. **A discovery operation second** — conjecture → finite verification →
   counterexample search → ledger record.
3. **A closed campaign third** — select a model, run the real experiment,
   compare to anchor, mint/withhold a claim, write a report.

## 4. Ranked gap register

### P0 — required for a science-discovery product

| ID | Gap | Required closure |
|---|---|---|
| SC-01 | `kubona.science.lattice` missing; materials pack unresolved | Implement exact spin-chain/lattice Hamiltonians, observables, and tests; make the pack resolve. |
| SC-02 | No physics experiment kind in bridge/Go/API/UI | Add `spin_chain` with exact diagonalization anchor, VQE, shot measurement, gap/magnetization/correlations, and campaign fold. |
| SC-03 | No mathematical law/conjecture discovery engine | Add exact rational law discovery with train/holdout verification, counterexample reporting, and a ledger-bound `conjecture` experiment. |
| SC-04 | No general chemistry beyond 3 molecules | Add H2O active-space VQE with full-FCI/CASCI anchors, geometry builder, and limits. |
| SC-05 | Campaign does not invoke the real experiment rail | Add a typed campaign protocol that selects an experiment spec, runs the real job, folds the result, and records negative results. |

### P1 — required for a complete researcher workflow

| ID | Gap | Required closure |
|---|---|---|
| SC-06 | No excited-state or reaction-energy experiment | Add deflation/QKSD excited-state receipt or reaction-energy protocol with exact anchors. |
| SC-07 | No physics hardware rail | Reuse the governed hardware parent for a spin-chain final-state measurement. |
| SC-08 | No finite-domain theorem certificate format | Add a signed certificate with scope, exhaustive domain, and falsifier; never call finite verification a proof. |
| SC-09 | No novelty/citation boundary for discovered laws | Require literature/catalog comparison and a "known/unknown" field before any novelty statement. |
| SC-10 | No UI for physics/math experiments | Add controls/results to the existing ExperimentPanel and MeasuredOutputs. |
| SC-11 | No typed Result variant for physics/math | Extend the shared result contract and campaign projections. |

### P2 — enhancements

- 2D lattice / Fermi-Hubbard with DMRG cross-check.
- Time-dependent quench dynamics and Loschmidt echo.
- General molecule geometry optimization with classical optimizer and quantum
  final-state check.
- Learned conjecture proposer over the math corpus.
- Hardware spin-chain calibration-aware error mitigation.
- Multi-experiment campaign scheduler and novelty search.

## 5. Implementation plan

### Phase S1 — physics engine (first slice)

- Add `platform/kubona/science/lattice.py`:
  - 1D transverse-field Ising and Heisenberg/XXZ Hamiltonians;
  - sparse exact diagonalization for 2–12 sites;
  - ground energy, gap, magnetization, ZZ correlations, entanglement entropy;
  - Pauli-term export for Qiskit measurement.
- Add bridge `spin_chain` runner:
  - local exact anchor;
  - hardware-efficient VQE on Aer;
  - shot-measured energy and magnetization;
  - explicit `measured`, `claimable`, uncertainty, backend, and limits.
- Add Go job type `spin_chain`, `/api/experiments` kind mapping, settlement,
  result-version gate, and campaign fold.
- Add console controls/results.
- Make the materials pack generator resolve and test it.

### Phase S2 — mathematical discovery engine

- Add `platform/kubona/science/conjecture.py`:
  - exact rational polynomial and linear-recurrence discovery;
  - train/holdout verification;
  - counterexample prediction/falsifier;
  - deterministic candidate IDs; no novelty claim.
- Add bridge `conjecture` runner and Go job `conjecture_run`.
- Record candidates in the campaign notebook as conjectures, never as claims.
- Add tests for true laws, overfit rejection, and counterexample behavior.

### Phase S3 — chemistry breadth

- Add H2O geometry and active-space VQE (`ncas=4`, `nelecas=4`).
- Add full-FCI/CASCI anchors and a 4-qubit/8-qubit comparison.
- Refuse full-space H2O above 12 qubits.
- Add tests for geometry and anchor consistency.

### Phase S4 — closed discovery campaign

- Add a typed `CampaignProtocol` that:
  1. selects a model/problem;
  2. runs the real experiment job;
  3. compares measured vs exact anchor;
  4. records a claim only when the evidence gate passes;
  5. stores negative results and a report.
- Do not use the old toy harness for measured science.

### Phase S5 — surface and contract

- Add physics/math result types to the shared schema and UI.
- Add browser tests for the new experiment kinds.
- Update wiki/roadmap/status.

## 6. Acceptance criteria

- `kubona.science.lattice` imports and its exact TFIM/Heisenberg energies are
  checked against known small-system values.
- `/api/experiments` accepts `spin_chain` and `conjecture`, creates a
  ledger-bound job, and returns `result_version:"experiment-result-v1"`.
- A spin-chain result is measured by shots, carries an exact anchor, and
  reports a finite uncertainty; a failed or out-of-range model refuses.
- A conjecture result is exact on all supplied terms, validates on a holdout,
  or is explicitly reported as unsupported; it is never called a theorem.
- H2O active-space VQE returns a measured energy, a CASCI reference, and a
  full-FCI reference with the active-space/full-space distinction preserved.
- No new result is produced by an LLM; no result is promoted to a claim by
  prose; no missing anchor is silently replaced.

## 7. Execution status (2026-11-09)

Implemented and locally tested:

- **S1 physics engine:** `platform/kubona/science/lattice.py` provides exact
  1D TFIM, Heisenberg, and XXZ sparse Hamiltonians, gap/magnetization/ZZ
  correlations/half-chain entropy, and Pauli export. `HubbardModel` provides a
  real 1D Fermi-Hubbard exact anchor. The materials pack now resolves
  `kubona.science.lattice.HubbardModel`; the phantom-generator test was
  corrected.
- **S1 experiment rail:** bridge `spin_chain` runner uses the exact anchor,
  local VQE, and shot-sampled Pauli measurements on Aer; Go knows
  `spin_chain`, `/api/experiments` accepts it, settlement/version-gate/campaign
  fold are wired, and the console launcher exposes it.
- **S2 math engine:** `platform/kubona/science/conjecture.py` performs exact
  rational polynomial/affine-recurrence search with a train/holdout split,
  explicit falsifier, and `conjecture` (never `theorem`) status. A
  `conjecture_run` job is ledger-bound and folded as a notebook record, never a
  claim.
- **S3 chemistry:** H2O active-space VQE (`ncas=4`, `nelecas=4`, 8 qubits) with
  live full-FCI and CASCI references; H2O is accepted by PES and the console
  form. Full-space H2O refuses above the 12-qubit CPU cap.
- **Evidence:** `platform/tests/test_lattice.py` (7), `test_conjecture.py` (5),
  `test_domain_packs.py` (7), bridge `test_lattice_experiment.py` (5), and the
  H2O bridge test pass locally; full Go suite passes.
- **Physics accuracy options:** the `spin_chain` result supports `state_mode:"vqe"`
  (a real variational attempt that may miss the ground state) and
  `state_mode:"exact"` (prepare the exact finite-size ground state on the
  circuit and shot-measure it; no variational claim). A 4-site TFIM exact-state
  run returned `-5.2188` vs exact `-5.22625` with uncertainty `0.0152`; the
  result is labeled as exact-state readout and `claimable:false`. A live local
  6-site critical TFIM VQE run returned
  exact `-7.7274066103` and shot-measured `-5.9992` (delta `1.728`, uncertainty
  `0.0173`, `claimable:false`, `ground_state_reached:false`). The measured
  magnetization belongs to the optimized VQE state, not the exact ground state;
  the result carries `observable_state` and the notebook does not project it as
  a molecular VQE trajectory. Higher-depth/adaptive ansätze are required before
  a ground-state accuracy claim.
- **Math evidence:** Fibonacci `[1,1,2,3,5,8,13,21,34]` yields the exact
  recurrence `a(n)=a(n-1)+a(n-2)` with predictions `55,89`; squares yield
  `a(n)=n^2`; an unpredictable sequence yields zero candidates. Results carry
  `novelty_status:"known"` for an explicit known-law registry or
  `"unclassified"` otherwise; external novelty is never claimed.

### S4/S5 execution status (continued)

- **S5b hardware spin-chain:** implemented and live-validated. Source spin
  job `dd945888-7e09-4393-8e20-1c6d10e02e2e` measured `-2.614453125` vs exact
  `-3.0090221198` (not claimable); prepared binding
  `91ab033ab0ccecfc9662941435b6ebb91ea134774b600747386fa571cc0956b3`; parent
  hardware job `1ad0e5fb-9bab-4c9e-86df-4ec92b2aceb6` completed on `ibm_fez`
  with vendor `dbp4io44148s739tu8mg`: `-2.278125` vs exact `-3.00902212`,
  uncertainty `0.14599`, `claimable:false`, usage `0.0`, actual `$0.00`,
  signed receipt present. The exact-state source is simulator-only: its QASM
  contains `reset` inside a gate body and the API refuses it for hardware
  rather than dispatching an invalid program.
- **S5c excited-state/reaction chemistry:** implemented H2 deflation VQE
  (`ground_energy`, `excited_energy`, `gap`, overlap, exact 4x4 spectrum),
  active-space deflation for LiH/BeH2/H2O with full-FCI/CASCI references, and
  reaction energy for H2 → 2H and H2O → H2 + O. All are measured simulator
  results; unsupported molecules refuse rather than borrowing another system's
  numbers.
- **S4 measured discovery protocol:** `POST /api/discovery/plan` is a
  deterministic goal-to-experiment planner (excited / spin-chain / PES / H2O /
  H2 VQE; refuses unbounded conjecture). `POST /api/discovery/run` executes
  1–5 real jobs sequentially through the normal admission/settlement rail,
  bounds each step with a timeout, records `discovery_protocol_*` and
  `discovery_step` ledger events, and returns the stored job results with a
  measured/claimable summary. Real-stack E2E covers plan + run.
- **Result projection:** the shared schema/types and `MeasuredOutputs` cover
  `excited_state`, `reaction`, `spin_chain`, `spin_chain_hardware`, and
  `conjecture`; console launcher controls cover the new simulator kinds.

Still queued after the correction pass:

- no live hardware spin-chain *accuracy* claim: the live VQE run was executed
  but missed the exact anchor, and exact-state hardware is refused by design;
- broader reaction families beyond H2/H2O, geometry optimization, and general
  molecule input;
- 2D lattice/Hubbard quantum experiments, time evolution, and DMRG cross-checks;
- external literature/catalog novelty search and a counterexample search beyond
  the holdout falsifier (SC-09 remains an external-validation gap);
- PES/hardware mitigation per point, repeated-run variance, and calibration-
  aware spin runs;
- manual VoiceOver/NVDA evidence remains human-only.

The governed hardware launch UI, protocol history, generic experiment
report/export, and measured-experiment visibility before a molecular notebook
were completed in the correction pass.
