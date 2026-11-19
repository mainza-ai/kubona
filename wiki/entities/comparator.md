---
name: Comparator
type: entity
tags: [entity, tool, formal-verification, lean4, benchmarking, security]
date: 2026-10-08
updated: 2026-11-18
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

## Version and invocation (verified 2026-11-18)

- Comparator was merged into Lean core at **v4.35.0** (`lake comparator`, with
  `lake check`); the standalone repository is archived. This corpus is pinned to
  Lean v4.34.1, so the sandbox installs the last 4.34-line release
  (`v4.34.0`, commit `d03acab…`) and pins its `lean4export` dependency to the
  v4.34.0 commit `076e8e5…`.
- Comparator is an external binary, **not** a Lake target. The documented
  invocation is `lake env comparator ComparatorChallenges/<Name>.json`; an
  earlier platform call used `lake exe comparator <Name>` and could never work
  (corrected in `platform/kubona/eval/comparator_eval.py`).
- The image installs `landrun` v0.1.17 (inner Landlock sandbox; comparator
  passes `--best-effort`) and `lean4export` on PATH.
- **Live proof (2026-11-18):** `ThreeStateSupercritical` built 8,950 targets
  and the kernel accepted the solution (`Your solution is okay!`); the signed
  attestation (`live_kernel_proven`, proof hash `4762a59c…`) was accepted by
  the API. **Host requirement (measured 2026-11-18):** Docker Desktop's linuxkit kernel
  reports Landlock ABI 8 and a minimal `landlock_*` probe works, but landrun
  v0.1.14 and v0.1.17 both apply an effectively deny-all ruleset there (even
  `--ro /etc` denies a file read), so comparator cannot run inside that kernel.
  The live challenge must run on a standard Linux kernel (Lima/Colima, a Linux
  host, or CI). For macOS VMs, use Colima with the `virtiofs` mount: the sshfs
  mount drops the Docker API tunnel under Lean's write volume. Run the
  verification container as the checkout-owner UID (`--user "$(id -u):$(id -g)"`,
  now emitted by `sandbox_docker_cmd`) with `HOME=/tmp`, so the bind-mounted
  `.lake` directory is writable by the compiler.

> ⚑ Correction (2026-11-18): the counts below previously said "811 challenge
> specifications". A machine count of this checkout gives **405** `.json`
> challenge files and 405 `.lean` challenge modules
> (`ls submodules/math/lean/ComparatorChallenges/*.json | wc -l`). The 811
> figure is retained in [[source-openai-math-corpus]] as the upstream
> document's claim, not as the state of the pinned tree.

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
- Scans and indexes the challenge specifications in
  `submodules/math/lean/ComparatorChallenges/` (405 `.json` files in this
  checkout, measured 2026-11-18; the source document's 811 is superseded above).
- Verifies axiom containment before accepting mathematical assertions into the research collective.
- Emits cryptographic verification records into the Kubona provenance [[provenance-and-reproducibility|ledger]].

See also [[lean4]], [[openai-math]], [[formal-proof-verification-lean-comparator]], [[provenance-and-reproducibility]].
