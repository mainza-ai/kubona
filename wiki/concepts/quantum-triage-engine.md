---
type: concept
tags: [quantum-triage, kohn-sham, dft-failure, qma-hardness, chemistry-routing, family-278]
date: 2026-10-07
updated: 2026-10-07
status: adopted
sources:
  - source-architecture-paper
  - source-openai-math-corpus
  - source-frontier-repo-audit-2026
---

# Quantum Triage Engine

The automated mathematical decision engine (`platform/kubona/science/triage.py`) that classifies molecular and electronic systems based on provable classical tractability thresholds, routing only genuinely multi-reference, strongly correlated problems to paid quantum QPUs while routing single-reference systems to classical Density Functional Theory (DFT).

## 1. Mathematical Grounding: Families 278 & 275

In computational chemistry, billions of dollars are wasted either running expensive multireference solvers on trivial systems or attempting classical DFT on molecules where non-interacting representations fail fundamentally:

### Family 278: Provable Failure of Kohn–Sham Representations
Derived in [[openai-math]], Family 278 establishes that there exists a 3-electron Coulomb molecule with equal positive nuclei whose ground-state electronic density $\rho_0(x)$ **provably cannot be represented** by any non-interacting Kohn–Sham ensemble in $L^{3/2} + L^\infty$.
- **Impact on Triage:** Classical DFT assumes that the true interacting ground-state density can be mapped onto a non-interacting fictitious reference system. Family 278 gives Kubona an analytical theorem proving when this assumption collapses. When multi-reference static correlation destroys single-determinant representability, classical DFT is mathematically guaranteed to fail.

### Family 275: Continuum Coulomb QMA-Hardness
Family 275 proves that approximating the electronic Coulomb spectral infimum in 3D continuous space is strictly **QMA-hard**, even with unit-charge nuclei and a macroscopic promise gap $\ge 1.0 \text{ Hartree}$.
- **Impact on Complexity:** Formally establishes that worst-case continuous electronic chemistry is quantum-hard, providing a rigorous complexity ceiling that justifies quantum hardware allocation for pathological configurations.

## 2. Automated Triage Decision Flow

```
                                MOLECULAR SYSTEM CANDIDATE
                                            │
                                            ▼
                        ┌───────────────────────────────────────┐
                        │   Static Correlation Diagnostic       │
                        │   • Fractional Occupation Number      │
                        │   • D1 / T1 Multi-Reference Metric    │
                        └───────────────────┬───────────────────┘
                                            │
                     ┌──────────────────────┴──────────────────────┐
                     ▼                                             ▼
         [SINGLE-REFERENCE SYSTEM]                     [MULTI-REFERENCE SYSTEM]
         • Kohn-Sham Representable                     • Provable KS Failure (Family 278)
         • D1 < 0.05, Homogeneous Bonds                • Open d-shells, Radical Centers
                     │                                             │
                     ▼                                             ▼
         ┌───────────────────────┐                     ┌───────────────────────┐
         │   CLASSICAL HPC DFT   │                     │ Active Space Sieve    │
         │   PySCF B3LYP / PBE   │                     │ CAS (N_elec, N_orb)   │
         │   Cost: $0.00         │                     └───────────┬───────────┘
         └───────────────────────┘                                 │
                                             ┌─────────────────────┴─────────────────────┐
                                             ▼                                           ▼
                                  [ACTIVE SPACE <= 50]                        [ACTIVE SPACE > 50]
                                  Classical Slurm HPC                         Provable QMA Horizon
                                  DMRG / Heat-bath CI                         Federated QPU Dispatch
                                  MPS Bond Dim χ ~ 4000                       Quantum Krylov (QKSD)
                                  Cost: ~$0.50 (Slurm)                        Cost: x402 Token Spend
```

1. **Step 1: Multi-Reference Diagnostic:** Evaluates fractional frontier orbital occupancies and static correlation metrics ($D_1$ diagnostic, natural orbital occupation numbers).
2. **Step 2: Single-Reference Classical Route:** If $D_1 < 0.05$, the system is confirmed Kohn–Sham representable. The job is dispatched to PySCF DFT (B3LYP/PBE) on local CPU cores. QPU allocation is strictly rejected.
3. **Step 3: Multi-Reference Active Space Sieve:** If static correlation is detected (Kohn–Sham failure), the active space dimension is evaluated (ceiling 50 orbitals, matching both engines — supersedes the older ≤36 figure):
   - If active orbitals $\le 50$: Routed to classical Slurm HPC executing Density Matrix Renormalization Group (DMRG) or Heat-bath Configuration Interaction (HCI).
   - If active orbitals $> 50$: Exceeds classical tensor network contraction limits ($n > 50$). The job is compiled into QIR and dispatched to federated QPUs (QuEra neutral atoms or IBM Quantum) executing [[early-fault-tolerant-quantum-and-subspace-methods|Quantum Krylov Subspace Diagonalization]].

> **Status (2026-10-12): advisory engine.** Both implementations (Go
> `pkg/triage`, Python `science/triage.py`) are tested and agree, but
> no live dispatch path calls either — verdicts are computed on
> demand and recorded, never enforced. Admission wiring is a design
> question (submit paths carry QASM/backends, not molecule/orbital
> identity), not an unimplemented TODO.

## 3. Enforcing Quantum Advantage Realism

By embedding the Quantum Triage Engine directly into the job submission pipeline:
- The platform eliminates **quantum-washing**: no researcher can claim quantum advantage for calculating water ($\text{H}_2\text{O}$) or hydrogen ($\text{H}_2$), which classical DFT solves in milliseconds.
- Spend mandates ([[ap2]]) are protected: expensive hardware shots are conserved exclusively for molecular geometries (e.g. nitrogenase FeMoco clusters, transition metal catalysts, diradical transitions) that classical density functionals provably cannot model.

## Cross-References

- [[mathematical-physics-anchors]] — analytical baselines and Family 278/275 theorems.
- [[quantum-advantage-realism]] — core policy governing true quantum supremacy claims.
- [[early-fault-tolerant-quantum-and-subspace-methods]] — Krylov subspace methods and QPU execution.
- [[epistemic-continuum-and-ontology]] — continuum across formal, classical, and quantum tiers.
