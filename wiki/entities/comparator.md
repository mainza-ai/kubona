---
name: Comparator
type: entity
tags: [entity, tool, formal-verification, lean4, benchmarking, security]
date: 2026-10-08
updated: 2026-10-08
status: adopted
sources: [source-openai-math-corpus]
---

# Comparator

**Comparator** (`leanprover/comparator`) is an automated formal verification harness for [[lean4]] designed to objectively evaluate candidate mathematical proofs against rigorous problem specifications.

- **Upstream live repository:** `https://github.com/leanprover/comparator`
- **Specification format:** JSON challenge files (e.g. `ComparatorChallenges/<ChallengeName>.json`)
- **Companion utilities:** `landrun`, `lean4export`
- **Role in Kubona:** Proof verification gatekeeper preventing unauthorized axioms, cheat definitions, and incomplete proofs in autonomous science workflows.

---

## Challenge Specification Contract

In `openai/math/lean/ComparatorChallenges/`, each challenge defines an explicit verification boundary:

```json
{
  "challenge_module": "ComparatorChallenges.QuasiRiemannHypothesis",
  "solution_module": "OAI.NumberTheory.DirichletL.Nonvanishing",
  "theorem_names": [
    "OAI.riemannZeta_ne_zero_of_seven_eighths_lt_re"
  ],
  "definition_names": [],
  "permitted_axioms": [
    "propext",
    "Quot.sound",
    "Classical.choice"
  ],
  "enable_nanoda": false
}
```

Key invariants enforced by Comparator:
1. **Module Separation:** The `challenge_module` defines the required theorem statement without proof, preventing the candidate solution from redefining the claim into a trivial tautology.
2. **Declaration Mapping:** The `theorem_names` array specifies the exact declaration in the `solution_module` that must prove the challenge statement.
3. **Axiomatic Whitelist:** Only the specified `permitted_axioms` are accepted. Any reference to `sorryAx` (incomplete proofs), inconsistent axioms, or custom assumptions causes immediate rejection.
4. **Independent Kernel Checking:** Optionally exports proofs via `lean4export` to be verified by external minimal type checkers (e.g., `nanoda`).

---

## Integration in Kubona (`platform/kubona/eval/comparator_eval.py`)

Kubona implements a native Python verification inspector (`ComparatorChallengeRegistry`):
- Scans and indexes the 811 challenge specifications in `submodules/math/lean/ComparatorChallenges/`.
- Verifies axiom containment before accepting mathematical assertions into the research collective.
- Emits cryptographic verification records into the Kubona provenance [[provenance-and-reproducibility|ledger]].

See also [[lean4]], [[openai-math]], [[formal-proof-verification-lean-comparator]], [[provenance-and-reproducibility]].
