---
name: Formal Proof Verification with Lean 4 and Comparator
type: concept
tags: [concept, formal-verification, lean4, comparator, epistemics, proof-checking]
date: 2026-10-08
updated: 2026-10-08
status: adopted
sources: [source-openai-math-corpus, source-architecture-paper]
---

# Formal Proof Verification with Lean 4 and Comparator

**Formal Proof Verification** is the process of certifying mathematical proofs and algorithmic properties through computer-checked formal logic, where every deductive step is validated down to foundational axioms.

In Kubona, formal verification provides the highest tier of epistemic certainty, creating an unshakeable bridge between continuous physical simulation and discrete mathematical truth ((source-openai-math-corpus)).

---

## 1. The Epistemic Gap: Simulation vs Proof

Scientific discovery platforms typically rely on empirical simulation:
- Quantum circuit execution on QPUs or statevector simulators yields statistical sampling distributions ($N$ shots, fidelity $F$, variance $\sigma^2$).
- Classical numerical algorithms (FCI, DMRG, DFT) yield floating-point approximations subject to truncation and basis-set errors.

While empirical simulation is essential for exploring physics, it cannot establish universal validity. Formal verification bridges this gap:

| Dimension | Empirical Simulation | Formal Proof Verification |
|---|---|---|
| **Substrate** | Qiskit, PennyLane, PySCF | Lean 4, Mathlib, Comparator |
| **Output** | Expectation value $\langle H \rangle \pm \epsilon$ | Axiomatic derivation $\vdash \forall x, P(x)$ |
| **Epistemic weight** | $\rho < 1.0$ (probabilistic, noise-prone) | $\rho = 1.0$ (deductive certainty) |
| **Failure mode** | Barren plateaus, noise bias, local minima | Incomplete proof (`sorryAx`), unpermitted axioms |

---

## 2. Comparator and the Anti-Cheating Invariant

A persistent hazard in automated AI theorem proving is "cheating" through:
1. Re-defining terms so that the theorem becomes vacuously true.
2. Introducing `sorry` or `admit` tactics that bypass proof obligations.
3. Postulating arbitrary unproven axioms.

The **Comparator** harness used in `openai/math` solves this decisively by enforcing three invariants ((source-openai-math-corpus)):
1. **Separation of Problem and Solution:** The challenge specification (`challenge_module`) is completely immutable and defined separately from the model's proof file (`solution_module`).
2. **Declaration Binding:** The theorem name tested must match the exact signature declared in the challenge.
3. **Axiom Whitelist:** The proof's dependency graph is checked against the three standard Lean axioms:
   $$\mathcal{A}_{\text{permitted}} = \{\texttt{propext}, \texttt{Quot.sound}, \texttt{Classical.choice}\}$$
   Any occurrence of unpermitted axioms, inconsistent axioms, or `sorryAx` immediately triggers an execution failure.

---

## 3. Kubona Platform Implementation (`platform/kubona/eval/comparator_eval.py`)

Kubona implements formal verification as a first-class evaluation pipeline:

```python
from kubona.eval.comparator_eval import ComparatorChallengeRegistry
from kubona.provenance import Ledger

registry = ComparatorChallengeRegistry()
ledger = Ledger(tenant="math-verification-lab")

# Validate proof axioms
result = registry.verify_axioms(
    challenge_name="QuasiRiemannHypothesis",
    axioms_used=["propext", "Classical.choice"],
    ledger=ledger
)
assert result.ok
```

When verified, the event is appended to Kubona's tamper-evident hash-chained [[provenance-and-reproducibility|ledger]], providing cryptographic provenance for mathematical discoveries.

See also [[lean4]], [[comparator]], [[openai-math]], [[autonomous-mathematical-discovery]], [[claim-credibility-wavefunctions]], [[epistemic-curvature]].
