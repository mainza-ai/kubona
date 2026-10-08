---
name: Lean 4
type: entity
tags: [entity, tool, formal-verification, theorem-prover, programming-language]
date: 2026-10-08
updated: 2026-10-08
status: adopted
sources: [source-openai-math-corpus]
---

# Lean 4

**Lean 4** is an interactive theorem prover and functional programming language developed by Leonardo de Moura and the Lean community. It is based on the calculus of inductive constructions (CIC) and features dependent type theory, meta-programming, and an optimizing ahead-of-time compiler.

- **Toolchain in `openai/math`:** `leanprover/lean4:v4.34.1`
- **Package manager:** Lake (`lakefile.lean`)
- **Mathematical library:** Mathlib4 (and specialized external repos)
- **Role in Kubona:** Formal verification backend for automated proof validation and epistemic certainty in scientific discovery.

---

## Technical Characteristics

1. **Axiom Discipline:**
   The standard Lean 4 axiomatic foundation consists of:
   - `propext`: Propositional extensionality.
   - `Quot.sound`: Quotients.
   - `Classical.choice`: Axiom of choice.
   Proofs that rely on these three axioms are considered fully classical and sound. The use of `sorryAx` (unproven gaps) or arbitrary ad-hoc axioms is strictly tracked and rejected by verification tools like [[comparator]].

2. **Lake Package Management:**
   In `openai/math/lean/lakefile.lean`, Lake coordinates over 20 external community and research libraries, compiling them into a unified verification library (`OAI`).

3. **Memory and Scalability:**
   Large formal libraries can exhaust system memory mappings. Lean 4 provides options such as `-DMMAP=OFF` and `GLIBC_TUNABLES=glibc.malloc.mmap_max=0:glibc.malloc.arena_max=1` to operate reliably in containerized sandbox environments.

---

## Role in Kubona's Architecture

Within Kubona's [[epistemic-curvature]] and [[science-discovery]] stack:
- **Sandbox Execution:** Lean 4 runs inside isolated container sandboxes ([[secrets-and-workload-isolation]]), invoked by autonomous agents to verify lemmas before claim publication.
- **Epistemic Weight:** Claims accompanied by a verified Lean 4 proof achieve an epistemic confidence $\rho = 1.0$ on the [[claim-credibility-wavefunctions|credibility curve]], serving as immutable anchor points for scientific discovery campaigns.

See also [[comparator]], [[openai-math]], [[formal-proof-verification-lean-comparator]], [[claim-credibility-wavefunctions]].
