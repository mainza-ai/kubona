---
type: concept
tags: [quantum-algorithms, krylov, eftqc, qldpc, neutral-atoms, shadow-tomography]
date: 2026-10-07
updated: 2026-10-07
status: adopted
sources:
  - source-architecture-paper
  - source-frontier-repo-audit-2026
  - source-openai-math-corpus
---

# Early Fault-Tolerant Quantum & Subspace Methods

The algorithmic and hardware paradigm that supersedes heuristic Noisy Intermediate-Scale Quantum (NISQ) Variational Quantum Eigensolvers (VQE), establishing Quantum Krylov Subspace Diagonalization (QKSD), classical shadow tomography, reconfigurable neutral-atom arrays, and bivariate bicycle qLDPC codes as Kubona's primary quantum execution pillars.

## 1. The Demise of Heuristic NISQ VQE

Between 2024 and 2026, rigorous theoretical and empirical benchmarks established that naive Parameterized Quantum Circuits (PQCs) and unmitigated VQE cannot achieve scalable quantum advantage:
1. **The Barren Plateau Crisis:**
   For randomly initialized or deep ansatz circuits, the gradient variance vanishes exponentially with system size:
   $$\operatorname{Var}_{\vec{\theta}}\left[ \frac{\partial \langle H \rangle}{\partial \theta_k} \right] \le \mathcal{O}\left( 2^{-n} \right)$$
   This occurs not only under Haar-random expressibility, but also under local hardware noise (**noise-induced barren plateaus**) and high entanglement. See [[barren-plateau]].
2. **The Measurement Shot-Noise Barrier:**
   Reaching chemical precision ($\epsilon \approx 1.6 \times 10^{-3} \text{ Hartree}$) for molecular Hamiltonians with $M$ Pauli terms demands:
   $$N_{\text{shots}} \ge \frac{\left( \sum_{i=1}^M |c_i| \right)^2}{\epsilon^2} \sim 10^7 - 10^9 \text{ shots}$$
   At typical QPU repetition rates (10 kHz superconducting, 100 Hz neutral-atoms/ions), a single energy evaluation takes hours, rendering multi-step gradient descent computationally and financially prohibitive.
3. **The Classical Tensor Network Counteroffensive:**
   In 2024, Tindall et al. (*PRX Quantum* 2024) and Begušić & Chan (2024) demonstrated that advanced classical tensor networks (combining 1D Matrix Product States, 2D PEPS, and Belief Propagation) could simulate 100+ qubit kicked Ising and many-body dynamics faster, with higher accuracy, and at negligible cost on classical HPC, disproving naive NISQ "quantum utility" claims.

**Kubona's Architectural Position:** Unmitigated NISQ VQE is relegated to a pedagogical/toy simulator mode. Autonomous discovery pipelines default to **subspace diagonalization**, **randomized phase estimation**, and **early fault-tolerant hardware execution**.

## 2. The Quantum Krylov Subspace Revolution

To circumvent non-linear optimization on QPUs, Kubona standardizes on **Quantum Krylov Subspace Diagonalization (QKSD)** and **Orthogonal QKSD (OQKD)**:
- **Subspace Generation:**
  $$\mathcal{K}_m = \operatorname{span}\left\{ |\psi_0\rangle, \, e^{-i H \tau} |\psi_0\rangle, \, e^{-i 2 H \tau} |\psi_0\rangle, \, \dots, \, e^{-i (m-1) H \tau} |\psi_0\rangle \right\}$$
- **Classical Generalized Eigenproblem:**
  The QPU is used exclusively to measure subspace Hamiltonian elements $H_{jk} = \langle \psi_j | H | \psi_k \rangle$ and overlaps $S_{jk} = \langle \psi_j | \psi_k \rangle$. The ground-state energy is resolved **classically**:
  $$\mathbf{H} \vec{c} = E \mathbf{S} \vec{c}$$
  - *Immune to Barren Plateaus:* Eliminates iterative non-linear parameter updates on quantum hardware.
  - *Strict Variational Convergence:* Eigenvalues converge monotonically from above.
  - *Orthogonal QKSD:* Deploys Lanczos recursion directly on quantum state vectors, removing overlap ill-conditioning.

### Classical Shadow Tomography (Huang, Kueng, Preskill)
Predicts $M$ arbitrary observable expectation values $\langle O_i \rangle$ with high probability using only:
$$N \in \mathcal{O}\left( \frac{\log M}{\epsilon^2} \max_i \|O_i\|_{\text{shadow}}^2 \right)$$
randomized Clifford measurements, completely decoupling measurement overhead from commuting Pauli groupings.

## 3. Early Fault-Tolerant Hardware Architectures
```
┌────────────────────────────────────────────────────────────────────────┐
│               FRONTIER HARDWARE & QEC PARADIGMS (2024-2026)           │
├────────────────────────────────────┬───────────────────────────────────┤
│ Reconfigurable Neutral Atoms       │ Bivariate Bicycle qLDPC Codes     │
│ Harvard / QuEra (Bluvstein 2024)   │ IBM Quantum (Bravyi 2024)         │
│ • 280 physical, 48 logical qubits  │ • \[144, 12, 12\] Gross Code       │
│ • Mobile optical tweezer shuttling │ • 12 logical per 288 physical     │
│ • All-to-all logical connectivity  │ • Slashing overhead by >10x       │
│ • Transversal non-Clifford gates   │ • Threshold: ~0.75%               │
└────────────────────────────────────┴───────────────────────────────────┘
```

1. **Reconfigurable Neutral-Atom Processors (Bluvstein et al., *Nature* 2024):**
   - Traps up to 280 physical $^{87}\text{Rb}$ atoms in 2D optical tweezers, operating up to 48 encoded logical qubits.
   - Mobile optical tweezers shuttle atoms across the array during execution, providing dynamic all-to-all connectivity without SWAP overhead.
   - Demonstrated fault-tolerant transversal non-Clifford gates and color-code logical qubits at break-even fidelities. See [[neutral-atom-quera-pasqal]].
2. **Bivariate Bicycle (BB) Quantum LDPC Codes (Bravyi et al., *Nature* 2024):**
   - The \[144, 12, 12\] "gross code" encodes 12 logical qubits into 144 data qubits + 144 syndrome ancillae (288 total), with code distance $d=12$.
   - Slashes physical qubit overhead by **over 10x** compared to standard rotated surface codes (which require 3,456 physical qubits for 12 distance-12 logical qubits).
   - High threshold ($0.75\% - 0.8\%$) under standard circuit noise models.

## Claim shape + acceptance pricing (built 2026-10-11)
`pkg/qec/claim.go`: every "logical beats physical" assertion carries four
numbers or refuses — suppression-per-+2-distance Λ (fit from ≥2 points,
single points cannot define a scaling law), breakeven factor, decoder
latency vs cycle time (backlog check), and acceptance rate. Postselected
gains price at 1/acceptance via `EffectiveShotsMultiplier` on both
estimators (unspecified acceptance = 1.0×, never assumed). A gain quoted
without its acceptance rate is not a scaling result.

## Fit machinery calibrated on real decoder data (2026-10-11)

Stim + PyMatching repetition-code memory (21 rounds, circuit-level
depolarizing p=0.008, 100k shots, distances 3/5/7) yields per-round rates
2.9e-4 → 2.1e-5 → 1.4e-6 and Λ=14.4 through the same log-linear formula
`FitLambda` implements (d=9 unresolvable at 100k shots — reported, not
extrapolated). This validates the *fit machinery*, not the estimator's
vendor constants: surface/BB suppression laws stay analytic heuristics
until the same protocol runs against them. Rerun recipe: scratch venv,
`pip install stim pymatching` (the full Deltakit package has no usable
wheels on Python 3.14 — Stim-direct instead), fixed rounds across
distances, batch decode, fit ≥2 nonzero points. Debugging notes kept:
rounds must be fixed across distances (else distance confounds with
volume), and observable-array shapes must be flattened before comparison
(a silent (N,1)-vs-(N,) broadcast fabricated a 145× error rate).

## Cross-References

- [[barren-plateau]] — vanishing gradient mechanisms and mitigation.
- [[vqa-vqe]] — historical variational methods and pedagogical simulator mode.
- [[neutral-atom-quera-pasqal]] — neutral-atom hardware and analog simulation.
- [[quantum-advantage-realism]] — analytical baselines and anti-quantum-washing policy.
- [[epistemic-continuum-and-ontology]] — confidence continuum across formal and empirical tiers.
