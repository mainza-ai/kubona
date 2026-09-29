---
name: Autonomous Ansatz Optimisation
type: concept
tags: [frontier, vqa, vqe, qaoa, autoresearch, optimization]
date: 2026-09-27
updated: 2026-09-27
status: open-question
sources: [source-frontier-repo-audit-2026, source-architecture-paper]
---

# Autonomous Ansatz Optimisation

**Definition.** An agent-led search over *ansatz genomes* — circuit families plus their
parameter-initialisation and symmetry constraints — driven by measured energy (or objective)
and rejected before execution by cheap structural pre-checks. It is Ring 3 of
[[five-ring-research-loop]], implemented as one ordinary `Harness` subclass. The agent never
*argues* that an ansatz is good; it proposes, and the simulator decides.

## The genome

```
Genome = { family, depth, entangler_topology, initializer, symmetry, hardware }
family      ∈ {hardware_efficient, realampr, efficient_su2, two_local, uccsd, tree, custom}
initializer ∈ {random, ones, particles, mp2-like-charged, mirrored, previous-best-perturbed}
symmetry    ∈ {particle_number, spin, none}
```

`milimoquantum/backend/app/quantum/vqe_executor.py` (1298 LOC) is the closest working
reference in the family and validates every slot above: `ANSATZ_CATALOG` (`:92`) with
`hardware_efficient / realampr / efficient_su2 / two_local / tree`; `build_ansatz` (`:310`);
`initialize_amplitudes` (`:440`); `_estimate_mps_bond_dim` (`:493`, classical-simulation cost
predicted *before* the run); and three interchangeable executors — `execute_vqe_with_qiskit`
(`:597`), `execute_vqe_with_qutip` (`:687`), `execute_mps_vqe` (`:783`) — dispatched by
`execute_vqe` (`:1191`). `qaoa_executor.py:33-34` re-uses the same catalog and dispatcher, so
the genome shape covers optimisation as well as ground states.

## The loop, with the pre-checks that make it cheap

| Stage | Gate | Primitive |
|---|---|---|
| 1 Propose | genome valid? (arity, qubit count, declared symmetry) | `Orchestrator.propose` (`platform/kubona/autoresearch/base.py:87`) |
| 2 **Expressibility prior** | Meyer–Wallach `Q` of the reference state; low `Q` ⇒ cannot express the target ⇒ reject without running | `meyer_wallach_score` (`vqe_executor.py:351`); PennyLane `liealg` for the reachable algebra |
| 3 **Trainability pre-check** | gradient-variance probe on 2–3 layers; barren basin ⇒ reject or shallow [[barren-plateau]] | PennyLane `gradients` (finite-diff/adjoint), Qiskit `EstimatorV2` |
| 4 **Classical-cost pre-check** | predicted MPS bond dimension / statevector memory vs host | `_estimate_mps_bond_dim` (`:493`), `select_backend` (`hal.py:57`) |
| 5 Execute | best available engine, **provenance-tagged** | `execute_vqe` (`:1191`) → Kubona `execution` server |
| 6 **Fidelity gate** | predicted vs probed fidelity ≥ floor | [[fidelity-accounting]], `build_noise_model` (`noise_profiles.py:80`) |
| 7 Mutate + remember | genome + metric + failure reason → algorithm library | [[science-discovery]] |

Steps 2-4 are the whole point. They are *free* (linear algebra, no shots), they are
falsifiable, and they let the agent explore hundreds of genomes per paid job.

## Noise-adaptive and hardware-aware, not hardware-agnostic

Two properties are searched over, not assumed:

- **Depth against coherence.** `noise_profiles.py` enforces `t2 <= 2*t1` (`:24`) and builds a
  real `qiskit_aer` `NoiseModel` (`:80`) from named device profiles (`:10,:17`);
  [[fidelity-accounting]] turns that into a per-genome fidelity prediction. An ansatz that
  wins noiselessly and loses on the noisy profile is recorded as a **negative result**.
- **Engine as a variable.** qutip-MPS vs Aer statevector vs real device trade accuracy for
  scale (`hal.py:8` `PlatformConfig(simulation_method, is_clifford)` selects `stim` for
  Clifford circuits). Kubona records which engine produced each number
  ([[provenance-and-reproducibility]]), exactly as chickensoup records its VQE backend.

## What is genuinely new here

1. **The ansatz genome is a library citizen.** Every genome ever tried — including refuted
   ones — is stored with its metric, its rejection stage, and its device fingerprint, so the
   search can *transfer* priors across devices instead of restarting. Device transfer is the
   actual open problem; a library of refuted genomes is the only honest way to attack it.
2. **The expressibility–trainability frontier is tracked explicitly.** Each trial plots
   `(Q, gradient variance, achieved energy)`. When the frontier stops moving, budget stops —
   slope, not confidence ([[five-ring-research-loop]] rule 2).
3. **Structure-preserving mutation is a hard constraint, not a preference.** `validity_check`
   (`base.py:63`) must reject a genome that breaks particle-number or spin symmetry for a
   fermionic target. This single check removes most garbage the agent will invent.
4. **Differentiable route beside the analytic one.** PennyLane (Apache-2.0; `gradients`,
   `capture`, `backline` JAX backprop, `qaoa`, `templates`, `liealg`; (repo-audit-2026))
   supplies exact parameter-shift and program-capture paths, so the mutation operator can be
   *learned* rather than only sampled. See [[differentiable-quantum-stack]].

## Guardrails and verification

Energy gaps are reported as ranges with shot counts; no advantage claim; every trial bound to
a ledger record; the harness runs simulator-only by default (`Harness(...,
simulator_only=True)`, `base.py:47`) so real QPU time requires a policy grant
([[cost-governance]]). Tests: (a) `validity_check` rejects a symmetry-breaking genome on a
Fermi-Hubbard setup; (b) the `Q`-prior rejects a 1-layer `two_local` on a target requiring
double excitations; (c) gradient-variance pre-check correlates with measured variance on a
synthetic shallow-circuit suite; (d) energies match published reference values for H₂ and
LiH within tolerance, with the reference cited as a `raw/` source.

See also [[vqa-vqe]], [[error-mitigation]], [[nisq-and-hybrid-orchestration]],
[[qiskit]], [[pennylane]], [[karpathy-autoresearch]].
