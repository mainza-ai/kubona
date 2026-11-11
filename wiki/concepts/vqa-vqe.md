---
type: concept
name: VQAs & VQE
date: 2026-08-16
updated: 2026-11-10
tags: [algorithms, variational]
sources: [source-architecture-paper, source-gap-analysis-plan]
status: adopted
---

# Variational Quantum Algorithms (VQAs) & VQE

The paradigmatic NISQ-era algorithm family: a parameterized quantum circuit (ansatz)
optimized by a classical optimizer against a measured cost function — quantum as an
accelerator inside a [[nisq-and-hybrid-orchestration|classical loop]]
(source-architecture-paper).

## Why Kubona is built around the loop

- VQE (variational quantum eigensolver) is the canonical multi-step hybrid job: prepare
  circuit → dispatch → receive measurement statistics → update variational parameters →
  repeat.
- These loops can span **hours and many agent invocations** — the reason for the `job_id`
  session abstraction and the live [[agent-ux-control-plane|loop inspector]].
- The main algorithmic hazard is the [[barren-plateau]] phenomenon.

## The frameworks

- [[qiskit]] V2 primitives (EstimatorV2) for hardware submission.
- [[pennylane]] gives differentiable circuits and `parameter-shift` gradients for
  optimization inside neural networks — the QML path.

## Shipped VQE & Hardware Loop Scope (2026-11-10, prototype register)

- **Molecular & Geometry VQE (`experiment-result-v1`):**
  - Curated active-space `H2`, `LiH`, `BeH2`, and `H2O` (`vqe`, `pes`,
    `excited_state`, and `H2` / `LiH` / `BeH2` / `H2O` atomization `reaction`) on
    Qiskit Aer backed by live PySCF CASCI/FCI exact anchors and a shared
    below-anchor error guard (`docs/production-audit-2026-11-09.md`).
  - Caller-supplied closed-shell STO-3G custom-geometry VQE (`molecule: "custom"`,
    explicit `ncas`/`nelecas`, 8-orbital full-FCI / 12-qubit active-space cap)
    and multidimensional RHF/STO-3G BFGS `geometry_optimization` (`de3428c`), plus
    `vqe_repeats` variance runs and full-space LiH/BeH2 ADAPT/CEO baselines.
- **Live IBM QPU Loops (`docs/live-qpu-2026-11-09.md`):**
  - **4-iteration SPSA hardware-in-the-loop VQE (`da0b1703`)** on `ibm_fez`
    (`Open Plan`, `$0.00`): 9 QPU batches, `-1.1254 Ha` (`11.9 mHa` delta, honest
    `claimable:false`), alongside live ZNE VQE (`5f0064eb`), active-space LiH
    (`1b6bb0ec`), HILO QAOA (`7014cd0c`), and `spin_chain_hardware` (`1ad0e5fb`).
  - Batched `hardware_pes` (single QPU batch over PES points + optional ZNE) is
    implemented and tested, awaiting its first live QPU run.

Related: [[barren-plateau]] · [[nisq-and-hybrid-orchestration]] · [[cost-governance]] ·
[[karpathy-autoresearch]] (VQE ansatz harness)