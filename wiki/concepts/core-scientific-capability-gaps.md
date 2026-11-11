---
type: concept
name: Core Scientific Capability Gaps
date: 2026-11-09
updated: 2026-11-10
tags: [science-discovery, quantum-chemistry, mathematical-physics, production, gaps]
sources: [source-gap-analysis-plan, source-openai-math-corpus, source-architecture-paper]
status: tracked
---

# Core Scientific Capability Gaps

> **Register, not ambition.** This page records what the platform can actually
> execute in mathematics, chemistry, and physics, and what it cannot. It is a
> code facts page; the design intent stays on [[science-discovery]] and
> [[autonomous-mathematical-discovery]]. Full report:
> `docs/core-scientific-capabilities-2026-11-09.md`.

> ⚑ **Correction pass (2026-11-09):** a follow-up audit found and fixed the
> water reaction sign, spin-hardware Pauli order, ZNE claimability,
> below-anchor claim leak, exact-state hardware admission, H2O hardware
> admission drift, source status/type/binding checks, and several UI/result
> projection gaps. Full register:
> `docs/production-audit-2026-11-09.md`. Historical live results were not
> upgraded.
>
> ⚑ **Implementation update (2026-11-10, `119749e` / `de3428c`):** Subsequent
> slices closed the remaining November 9 breadth gaps in the shipped rail:
> - **All 19 result kinds enforced (`TestSchemaKindEnumMatchesDecoder`):**
>   `geometry_optimization`, `vqe_repeats`, `hardware_pes`,
>   `thermodynamic_limit`, `lattice_dynamics`, and `qaoa_hardware_loop` are now
>   registered in `platform/contracts/experiment-result-v1.schema.json`, decoded
>   in `pkg/experimentresult`, folded into `CampaignExperiment` state, and
>   included in Ed25519 `/api/sessions/{id}/experiment-export`.
> - **Chemistry breadth closed within STO-3G bounds:** caller-supplied
>   closed-shell STO-3G `vqe`, curated 1D and arbitrary caller-supplied
>   multidimensional RHF/STO-3G BFGS `geometry_optimization`, repeated-seed
>   `vqe_repeats`, active-space `excited_state`, and `LiH`/`BeH2`/`H2O`/`H2`
>   atomization `reaction` are implemented and folded. General SMILES, open-shell/charged
>   species, and arbitrary reaction networks remain out of scope.
> - **Condensed matter, DMRG & 2D quench:** 1D `spin_chain` adds multi-start
>   `L-BFGS-B` warm-up; `kubona.science.dmrg` + `thermodynamic_limit` run
>   open-boundary `quimb` DMRG2 (including the `TNLinearOperator` `N>=12` fix
>   verified through `N=16`) with exact TFIM / isotropic-Heisenberg limits
>   (anisotropic XXZ stays finite-size); `lattice_dynamics` computes exact
>   finite-size 2D TFIM quench trajectories via sparse Krylov `expm_multiply`
>   (`measured:false`, `claimable:false`).
> - **Hardware QPU acceptance & `hardware_pes`:** `docs/live-qpu-2026-11-09.md`
>   records governed `ibm_fez` Open Plan execution of SPSA HILO VQE, ZNE VQE,
>   active-space LiH, HILO QAOA, and `spin_chain_hardware` (`1ad0e5fb`, TFIM
>   `n=3`, `-2.278125` vs `-3.00902212`, `claimable:false` — hardware rail
>   verified, accuracy claim still open). Calibration-aware `hardware_pes` and
>   reviewed rate-card pricing preview are implemented (awaiting live QPU run).
> - **Multi-step discovery, novelty lookup & UI:** `/api/discovery/{plan,run}`
>   supports multi-step goals separated by `" then "`, `" -> "`, or `";"` across
>   all molecules and active kinds; `conjecture` supports opt-in OpenAlex
>   title/DOI lookup (`external_novelty`: `checked` / `unavailable`, never
>   upgrades `novelty_status`); Console adds the `Cmd+E` **Experiment & Discovery Lab**
>   modal and `ResultsStudio` SVG `<Chart>` series plots (`x.kind: 'linear'`),
>   while `api.Server` shares the live `FidelityGate` calibration pointer with
>   `jobs.Runner` and `ledger/obligation.go` locks all DB calls.

## The finding (2026-11-09)

Kubona has a real **measured science spine**: H2/LiH/BeH2/H2O chemistry,
PES, QAOA MaxCut, H2 excited-state deflation, H2 reaction energy, spin-chain
physics, exact finite-scope law discovery, PySCF/Qiskit anchors and shots,
governed QPU hardware rails, signed receipts, and campaign folding.

The first capability gap is closed; the remaining gaps are breadth and
external validation:

- **Physics is executable.** `kubona.science.lattice` provides exact
  1D TFIM/Heisenberg/XXZ anchors, observables, Pauli export, and a real
  Fermi-Hubbard anchor; `spin_chain` runs as a measured simulator job and
  `spin_chain_hardware` has a governed QPU parent rail. The critical 6-site
  VQE is not yet ground-state accurate; exact-state readout is a separate,
  labeled mode.
- **Mathematical discovery is an engine, not only a catalog.** Exact rational
  polynomial/recurrence discovery with a train/holdout split and falsifier is
  live as `conjecture_run`; it is explicitly not a theorem prover and does not
  solve open corpus problems.
- **Chemistry is broader but still bounded.** H2/LiH/BeH2/H2O VQE,
  H2O CASCI/full-FCI, H2/LiH/BeH2/H2O active-space excited states, H2/LiH/BeH2/H2O
  atomization reaction energies, caller-supplied closed-shell STO-3G VQE,
  multidimensional RHF/STO-3G BFGS geometry optimization, and `vqe_repeats`
  variance are implemented as of `2026-11-10`. Broad reaction networks, SMILES,
  and open-shell/charged generic molecules remain absent.
- **The discovery loop is bounded.** `/api/discovery/plan` deterministically
  plans single-step or multi-step (`then` / `->` / `;`) pipelines across all
  supported molecules and experiment kinds; `/api/discovery/run` sequences 1–5
  real jobs through the normal rail and returns stored results plus a
  measured/claimable summary. Completed protocol summaries are retained in the
  ledger and shown in the panel; the old simulator-era `DiscoveryCampaign` is
  legacy and must not be used as measured science.

## Why it matters

A platform whose purpose is discovery must either execute a scientific
capability or refuse. A UI over a missing physics model, or a "conjecture"
generated by prose, would be the same theater the [[quantum-advantage-realism]]
policy forbids. The order must be: executable model → exact anchor → measured
result → falsifiable claim → report.

## Ranked gaps

| ID | Gap | Closure (as of 2026-11-10) |
|---|---|---|
| SC-01 | `kubona.science.lattice` missing; materials pack unresolved | Implemented exact 1D TFIM/Heisenberg/XXZ, observables, Hubbard anchor, open-boundary `quimb` DMRG (`thermodynamic_limit`, verified `N<=16`), and 2D TFIM quench (`lattice_dynamics`); pack resolves. |
| SC-02 | No physics experiment rail | Implemented `spin_chain` (with `COBYLA` + multi-start `L-BFGS-B` warm-up), `thermodynamic_limit`, and `lattice_dynamics` across bridge/Go/API/UI with exact anchors, campaign folds, and signed exports. |
| SC-03 | No math discovery engine | Implemented exact rational law discovery, holdout/falsifier, and ledger-bound `conjecture` job (4+ terms required; explicitly not a theorem). |
| SC-04 | Chemistry portfolio too narrow | Added H2O active-space VQE/CASCI/full-FCI, caller-supplied closed-shell STO-3G VQE, multidimensional RHF/STO-3G BFGS `geometry_optimization`, and `vqe_repeats`. SMILES/open-shell/periodic crystals remain out of scope. |
| SC-05 | Discovery campaign not over the real rail | Implemented: multi-step `/api/discovery/{plan,run}` (`then`/`->`/`;`, 1–5 real jobs) across all supported molecules/kinds, recording ledger events and measured summaries. |
| SC-06 | No excited-state/reaction experiment | Implemented `excited_state` (`H2`/`LiH`/`BeH2`/`H2O` active space) and `reaction` (`H2`, `LiH`, `BeH2`, `H2O` atomization against exact atomic FCI). General reaction networks remain open. |
| SC-07 | No physics hardware rail | Implemented `spin_chain_hardware` + `hardware_pes`. Live `ibm_fez` execution completed for `spin_chain_hardware` (`1ad0e5fb`, `claimable:false`); QPU accuracy claim and live `hardware_pes` run remain open. |
| SC-08 | No finite-domain certificate format | Implemented finite-scope certificate/falsifier + Ed25519 `result_receipt` and `/api/sessions/{id}/experiment-export`; never called a theorem. |
| SC-09 | No novelty boundary | Local known-law registry labels matches `known` else `unclassified`; opt-in OpenAlex lookup records `external_novelty` (`checked`/`unavailable`) without upgrading `novelty_status`. Formal proof novelty remains open. |
| SC-10 | No physics/math UI | **Experiment & Discovery Lab** (`Cmd+E`), `ExperimentPanel`, `HardwareLaunchPanel`, `DiscoveryProtocolPanel`, `MeasuredOutputs`, and `ResultsStudio` (with PES/DMRG/quench SVG `<Chart>` plots, pre-claim table, receipts, and signed export) cover all 19 kinds. |

## Implementation status (2026-11-09)

S1–S4 and the S5b/S5c first slices are implemented in the tree:

- `platform/kubona/science/lattice.py` now provides exact TFIM/Heisenberg/XXZ
  anchors, observables, Pauli export, and a real `HubbardModel`; the materials
  pack resolves.
- A shot-measured `spin_chain` bridge/Go/job/console path exists. It reports an
  exact finite-size anchor, measured energy, uncertainty, gap, magnetization,
  and no advantage claim. The live 6-site TFIM check is honest but not
  ground-state accurate: exact `-7.7274066`, measured `-5.9992`,
  `ground_state_reached:false`; the measured magnetization is explicitly
  labeled as the VQE state's, not the ground state's. The same runner supports
  `state_mode:"exact"` for exact finite-size state preparation plus shot
  readout, which is labeled as such and is not a variational claim.
- `platform/kubona/science/conjecture.py` discovers exact polynomial/linear-
  recurrence laws on a training prefix, validates a holdout, and emits a
  falsifier. The `conjecture_run` job is recorded as a conjecture, never a
  claim or theorem.
- H2O active-space VQE is available with full-FCI/CASCI references; full-space
  H2O refuses above the 12-qubit CPU cap.
- S4 is now a real measured protocol: `POST /api/discovery/plan` selects a
  bounded experiment from a goal (and refuses unbounded conjecture), while
  `POST /api/discovery/run` submits 1–5 real jobs sequentially through the
  normal admission/settlement rail and returns stored results plus a
  measured/claimable summary.
- S5b adds a governed `spin_chain_hardware` parent rail (bridge batch, Go
  prepare/launch, mandate/fidelity/settlement); a live `ibm_fez` execution
  completed with a negative, non-claimable accuracy result. The exact-state
  source remains simulator-only and is refused before hardware admission.
- The 2026-11-09 correction pass fixed the water reaction sign, spin hardware
  Pauli basis order, ZNE claimability, below-anchor claims, focus-observable
  semantics, source-job binding/status checks, and the measured-experiment UI.
  Remaining work is breadth, external validation, and manual accessibility.
- S5c adds H2 deflation excited-state VQE and H2→2H reaction energy against
  exact FCI endpoints; both are measured simulator results.
- Still open (as of 2026-11-09 first pass): a live hardware spin-chain
  *accuracy* claim (execution completed on `ibm_fez`, but the VQE state missed
  the exact anchor), broader reaction families, and external novelty evidence
  beyond the local known-law registry. The hardware/protocol launch UI now
  exists.
- **Added in later 2026-11-09 / 2026-11-10 slices (`119749e` / `de3428c`):**
  caller-supplied closed-shell STO-3G VQE, multidimensional RHF/STO-3G BFGS
  `geometry_optimization`, repeated VQE variance (`vqe_repeats`),
  `LiH`/`BeH2`/`H2O` atomization reactions & active-space excited states,
  open-boundary `quimb` DMRG (`thermodynamic_limit`, verified through `N=16`),
  exact 2D TFIM quench (`lattice_dynamics`), calibration-aware `hardware_pes`
  + reviewed pricing preview, opt-in OpenAlex check (`external_novelty`),
  multi-step `DiscoveryProtocol` (`then` / `->` / `;`), all 19 result kinds
  validated in `pkg/experimentresult` and exported via Ed25519
  `/api/sessions/{id}/experiment-export`, plus the Console `Cmd+E`
  **Experiment & Discovery Lab** modal and `ResultsStudio` SVG `<Chart>` plots.
  What remains open is QPU accuracy (`claimable:false` on `ibm_fez` runs),
  live `hardware_pes` execution, general SMILES/open-shell/periodic chemistry,
  anisotropic XXZ thermodynamic limits, formal Lean proof synthesis, and manual
  screen-reader sign-off.

## Implementation order (acceptance criteria)

1. **S1 — physics:** exact TFIM/Heisenberg energies match known small-system
   values; `/api/experiments` accepts `spin_chain`; result is shot-measured,
   anchored, versioned, and folded; materials pack resolves.
2. **S2 — math:** exact rational laws verify on a holdout; overfit candidates
   are rejected; counterexample/falsifier is recorded; a `conjecture` job is
   never treated as a theorem or a claim.
3. **S3 — chemistry:** H2O active-space VQE returns measured energy, CASCI
   reference, and full-FCI reference; full-space H2O refuses above the qubit
   cap rather than exhausting the host.
4. **S4 — closed campaign:** a real experiment job is selected and run by the
   campaign; a claim is minted only on the evidence gate; negative results are
   first-class.
5. **S5 — surface:** result types, UI, and browser tests; no demo-to-measured
   leakage.

## Relation to settled policy

- [[quantum-advantage-realism]]: every physics/chemistry result carries the
  exact classical anchor, backend, uncertainty, and the absence of advantage.
- [[provenance-and-reproducibility]]: the new experiment kinds must use the
  same job/ledger/result-receipt rail, not a side channel.
- [[epistemic-continuum-and-ontology]]: exact finite verification is not
  $\rho=1.0$ proof; mathematical laws remain conjectures until a formal proof
  exists.
- [[quantum-triage-engine]] remains advisory; no new physics path may claim
  quantum necessity without a classical comparison.

## Cross-references

[[science-discovery]] · [[autonomous-mathematical-discovery]] ·
[[mathematical-physics-anchors]] · [[quantum-advantage-realism]] ·
[[implementation-gaps]] · [[quantum-domain-packs]] · [[openai-math]]
