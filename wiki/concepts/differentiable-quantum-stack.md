---
name: The Differentiable Quantum Stack
type: concept
tags: [frontier, differentiation, jax, autodiff, gradients, hybrid]
date: 2026-09-27
updated: 2026-10-26
status: adopted
sources: [source-frontier-repo-audit-2026]
---

# The Differentiable Quantum Stack

**Definition.** Put autodiff at the centre of the architecture, not in a bolt-on module: a single
tape where the **quantum** block, the **classical** block, and the **per-layer
noise/uncertainty** model are all differentiable and share one cost. Two different tapes must
never be reconciled by hand — that is precisely the seam where "quantum ML" results become
unreproducible. Maps to Ring 3 of [[five-ring-research-loop]].

## Why this is an architecture decision and not a feature

Kubona today dispatches a circuit, gets a number, and hands the number to an optimiser.
[[pennylane]] demonstrates that the whole NISQ loop is better expressed as one gradient graph:
VQE, QNN, QMC, and differentiable noise are all one library away — and, decisively for us, its
`default.mixed` device gives **parameter-shift through a noisy channel**, which is the only way a
noise model can be *trained* rather than assumed.

## Verified primitives

PennyLane 0.44.0 (`pennylane/_version.py:18`, Apache-2.0, [[pennylane]]) —

- **Gradients**: `qnodes/jacobian.py:231` `jacobian`, `:421` `hessian`,
  `interfaces/{autograd,tensorflow,torch}_interface.py`, `optimize/{adagrad,rotosolve,adaptive_sgd}.py`.
- **VQE and the loop**: assembled from `pennylane/workflow/` (`execution.py`,
  `construct_tape.py`, `get_best_diff_method.py`, `get_compile_pipeline.py`,
  `jacobian_products.py`) over `pennylane/optimize/`. Kubona must **not** fork a second
  workflow engine; the ledger wraps this one.
- **Differentiable noise**: `pennylane/noise/{noise_model,add_noise,insert_ops,mitigate,conditionals}.py`
  — noise-model construction, insertion and mitigation are first-class modules, and the
  mixed-state simulator that makes a channel *differentiable* is `pennylane/devices/qubit_mixed/`
  (alongside `qubit/{simulate,apply_operation,adjoint_jacobian}.py`).
- **Structure-preserving tools**: `pennylane/liealg/{cartan_decomp,horizontal_cartan_subalgebra,involutions,center}.py`
  plus `pennylane/labs/dla` (Hamiltonian-reduction by Lie algebra) and
  `pennylane/labs/trotter_error` (predictive Trotter error) — Apache-2.0, but *labs is volatile*:
  pin the SHA and expect APIs to move.
- **Circuit cutting**: `pennylane/qcut/` — `cutcircuit.py`, `cutcircuit_mc.py`,
  `cutstrategy.py`, `kahypar.py` (with the shipped `_cut_kKaHyPar_sea20.ini` partitioner config),
  `tapes.py`, `processing.py`. Module docstring: transforms "for 'cutting' (or splitting) of large
  circuits into smaller circuits, to allow them to be executed on devices that have a restricted
  number of qubits" (`qcut/__init__.py`). This is the mature "more qubits than you have" story,
  and Apache-2.0 lets us use it directly as a budget-aware fallback.
- **Resource accounting**: `pennylane/resource/{expression,specs,resource,mlir_specs}.py` —
  `mlir_specs.py` compiles a QNode with a *resource analysis pass* inserted and reads back
  pass-by-pass specs from JSON. That is the seam between a differentiable stack and
  [[agentic-qir-compilation]]: cost estimates produced by the same compiler pass that will
  compile the job.
- **Declared capability, checked in CI**: `pennylane/devices/{capabilities.py,toml_check.py}` —
  devices advertise what they support in TOML and a CI check validates it; 28 files in
  `.github/workflows/`. That is the declarative form [[workflows-and-agent-authoring]] and
  [[provider-capability-schema]] need, already proven in a quantum repo, and the maturity bar our
  own CI must meet.

## The single-tape contract

```
z = U(θ) |0⟩                      quantum block   (PennyLane QNode, PS or adjoint)
N = channel(z; φ)                 noise block      (Kraus/Choi, differentiable in φ)
ŷ = f(N; ψ)                       classical head   (JAX / PyTorch)
L = ℓ(ŷ, y) + λ·Ω(circuit)        one cost; ∂L/∂{θ,φ,ψ} on one tape
```

Because `default.mixed` differentiates through noise, **φ becomes learnable**: fit the channel
from calibration data instead of trusting a vendor PDF. That is how
[[fidelity-accounting]]'s `F_pred` gets *calibrated* rather than asserted — the platform learns
its own error model and logs the fit, and a drifting φ is a calibration-drift alarm
([[epistemic-curvature]]).

## What is genuinely new

1. **Noise as a trained layer, audited against the ledger.** The fitted φ is versioned with the
   run; predictions and measurement disagree → `F_pred` is void, not quietly ignored.
2. **Cutting as a first-class fallback with a budget.** QPD sampling trades shots for qubits, so
   cost is *computable in advance*: `shots × overhead` feeds [[cost-governance]] and the fidelity
   floor, making "split the circuit" an admissible option in [[autonomous-ansatz-optimization]]'s
   search rather than a human afterthought.
3. **One gradient space for the whole hybrid.** `θ` (ansatz), `φ` (channel), `ψ` (head), plus the
   continuous relaxation of discrete choices → the search space of
   [[autonomous-ansatz-optimization]] becomes one optimisation problem instead of three scripts.
   This is the concrete, defensible version of "quantum + AI", and the honest counterweight to the
   numerology in [[mainza-consciousness]] and [[chickensoup]].

## Boundary conditions

Gradients through a noisy channel are only as trustworthy as the calibration behind them;
`F_pred` and fitted φ are reported as intervals with timestamps and are never promoted to
correctness claims ([[quantum-advantage-realism]], `wiki/concepts/risks.md`). Parameter-shift
cost scales with trainable parameters, so gradient strategy is itself a budget decision
([[agent-resource-economy]]). `retworkx` (Apache-2.0, `pennylane/pylib`) is a legal dependency;
pennylane's `diagrams` extras are not required.

## Settlement: Unified Single-Tape Gradient Flow (Adopted 2026-10-26)

Autodiff routing is resolved in favor of the **single unified tape** architecture centered in the Python Bridge (`platform/kubona/diff_stack/tape.py`):

1. **Analytical Mixed-State Differentiation:**
   `DifferentiableQuantumTape` uses PennyLane's `default.mixed` device with exact parameter-shift rules to compute the Jacobian of expectation values with respect to both circuit parameters $\vec{\theta}$ and channel noise parameters $\phi$ (e.g. depolarizing error rate $p$):
   $$\nabla_{\theta, \phi} \langle H \rangle = \left[ \frac{\partial \langle H \rangle}{\partial \theta_1}, \dots, \frac{\partial \langle H \rangle}{\partial \theta_m}, \frac{\partial \langle H \rangle}{\partial \phi_1}, \dots, \frac{\partial \langle H \rangle}{\partial \phi_k} \right]$$
2. **Single Computational Graph:**
   Circuit ansatz execution and noise channel characterization are not separated into independent steps. Both appear on one tape, ensuring joint optimization of ansatz weights and calibrated error parameters without manual reconciliations.
3. **Ledger Calibration Provenance:**
   Gradients evaluated through the noisy channel are logged directly into the provenance ledger, bounding learning rates by the [[fidelity-accounting]] floor.

Verified in `platform/tests/test_differentiable_quantum_stack.py` (simultaneous analytical parameter-shift gradient computation of $\nabla_\theta$ and $\nabla_\phi$ with zero numerical finite-difference drift).

See also [[pennylane]], [[qiskit]], [[error-mitigation]], [[circuit-cutting]],
[[nisq-and-hybrid-orchestration]].
