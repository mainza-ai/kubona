---
name: Mathematical Physics Anchors
type: concept
tags: [concept, quantum-advantage-realism, anchors, mathematical-physics, benchmarks]
date: 2026-10-08
updated: 2026-10-08
status: adopted
sources: [source-openai-math-corpus, source-frontier-repo-audit-2026]
---

# Mathematical Physics Anchors

**Mathematical Physics Anchors** are exact, analytically proven theorems that provide ground-truth baselines for quantum algorithms and physical simulations in Kubona.

Under the [[quantum-advantage-realism]] policy, Kubona forbids ungrounded claims of quantum advantage: every quantum calculation (VQE, QAOA, quantum state tomography) must be compared against rigorous classical and analytical baselines. The `openai/math` repository supplies a suite of frontier analytical proofs that serve as non-negotiable anchor points ((source-openai-math-corpus)).

---

## 1. Key Mathematical Physics Anchors from `openai/math`

| Family ID | System / Model | Analytical Proven Quantity | Benchmark Application in Kubona |
|---|---|---|---|
| **271** | 3D Heisenberg Ferromagnet | Ground state energy density $E_0/(NJ) = -1.5 J$; Bloch $T^{3/2}$ spontaneous magnetization | VQE lattice benchmarks; spin-wave dispersion bounds |
| **281 & 221** | Sherrington–Kirkpatrick & Diluted Spin Glass | Parisi RSB ground state energy limit $E_{\text{SK}} \approx -0.763166$; Mézard–Parisi cavity formula | QAOA optimization benchmarks; Max-Cut asymptotic limits |
| **268** | Spin-1 Heisenberg Chain | Haldane spectral gap $\Delta / J \approx 0.410479$ | Many-body VQE spectral gap estimation; topological phases |
| **266** | Mutually Unbiased Bases | $N(6) = 3$ (Zauner's conjecture) | Quantum state tomography; dimension-6 state reconstruction limits |
| **269** | Laughlin Fractional Quantum Hall | Spectral gap under $V_1$ pseudopotential at filling $\nu = 1/3$ | Topological quantum memory simulation; anyonic state preparation |
| **275** | Electronic Coulomb Potential | QMA-hardness of continuum 3D Coulomb ground energy | Hardness boundaries for molecular active space algorithms |
| **278** | Coulomb Molecule Ensemble | Non-representability of ground state density by Kohn–Sham systems | Justification for quantum VQE where classical DFT provably fails |
| **279** | Quantum Integer Factoring | Polynomial-size uniform circuits over fixed gate set without ancillas | Compilation passes for fault-tolerant Clifford+T architectures |

---

## 2. Enforcing Quantum Advantage Realism

When an autonomous agent runs a VQE or QAOA campaign in Kubona:
1. **Anchor Retrieval:** The agent queries `kubona.anchors.math_physics.MathPhysicsAnchorEngine` to retrieve the analytical baseline for the targeted system.
2. **Variational Check:** By the Rayleigh–Ritz variational principle, the computed expectation value must obey $\langle H \rangle \ge E_{\text{exact}} - \epsilon$. Any result with $\langle H \rangle < E_{\text{exact}} - \epsilon$ indicates unmitigated hardware drift or non-Hermitian operator formulation.
3. **Accuracy Accounting:** The platform records the relative error $\delta = |\langle H \rangle - E_{\text{exact}}| / |E_{\text{exact}}|$ into the [[provenance-and-reproducibility|ledger]].
4. **Honest Provenance:** Reports explicitly state the backend used (simulator vs physical QPU) and benchmark against the analytical anchor. The platform never claims quantum advantage when the result simply approximates an already proven mathematical closed form.

---

## 3. Platform Integration

The anchors are exposed through:
- **Python Engine:** `platform/kubona/anchors/math_physics.py`
- **Domain Pack Manifest:** `platform/domain_packs/math_physics_v1/pack.yaml`
- **REST Surface:** `GET /api/science/math/anchors` in `platform/kubona/console_api.py`

See also [[quantum-advantage-realism]], [[source-openai-math-corpus]], [[openai-math]], [[science-discovery]], [[fidelity-accounting]].
