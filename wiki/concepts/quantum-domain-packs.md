---
name: Quantum Domain Packs
type: concept
tags: [frontier, plugins, domain-packs, vqe, qml, workflows, dft]
date: 2026-09-27
updated: 2026-10-10
status: adopted
sources: [source-frontier-repo-audit-2026]
---

# Quantum Domain Packs

**Definition.** Declarative, versioned, capability-declaring packages that turn the platform into
a *domain scientist* — Hamiltonian builders, workflow graphs, classical anchors, metrics, and
evidence requirements for one application area. The platform core stays domain-free; everything
application-specific is a pack. This is how Kubona ships VQE, QML, and QCFI without hard-coding
them.

## The mechanism we adopt, and the licence it comes with

NVIDIA's `Model-Optimizer` (Apache-2.0, `github.com/NVIDIA/Model-Optimizer`, HEAD 2026-09-26) is
the most mature plugin contract in the audit — and the interesting half is **not code, it is a
directory of recipes**. `modelopt_recipes/` is organised as method × model-type
(`configs/{auto_quantize,numerics,ptq}`, `general/{auto_quantize,distillation,ptq,qad,speculative_decoding}`,
then `huggingface/`, `model_type/`, `timm/`, `models/`), each level documented by a README. Inside,
every recipe is a YAML that **pins its own schema in a comment** and maps its sections onto the
training script's argument dataclasses:

```yaml
# modelopt-schema: modelopt.recipe.config.ModelOptDFlashRecipe
metadata:
  description: Domino training recipe (DFlash backbone + causal correction head).
# maps to ModelArguments (main.py)
model:
  model_name_or_path:
  trust_remote_code: false
```

Fields are overridden by an **OmegaConf dotlist on the CLI**, so a recipe is a default, not a
lock. What makes this a *contract* rather than a config dump is how variants behave
(`general/speculative_decoding/lilicorr_conv.yaml`): the header states in caps which upstream
variant it requires, warns that it "**raises at model build with a message naming the two keys to
remove**" until that variant lands, notes that the shared convolution is *imported* rather than
copied "so that the two variants cannot drift apart arithmetically", and concedes that "**recipes
do not compose**; keep the two files in sync when editing shared fields". A pack that fails at
build time, naming the offending keys, is exactly the behaviour a research platform needs —
compare `milimoquantum`'s `ANSATZ_CATALOG` (`backend/app/quantum/vqe_executor.py:92`) and
`meyer_wallach_score` (:351), which are the classical-anchor side of the same idea.

> ⚑ Correction: an earlier draft of this page cited
> `modelopt_recipes/configurators/kits/registry.py` / `CustomRestoreConfigKit` and an in-repo
> `experimental/ControlNetToolkit`. Neither exists in this tree (`configurators/` contains only
> `dms/`; there is no `experimental/`). Superseded by the verified reading above
> ([[source-frontier-repo-audit-2026]]).

Kubona's pack manifest therefore carries `name`, `version`, `schema` (a Pydantic model ref, in the
spirit of `modelopt-schema`), `capabilities`, `evidence_requirements`, `classical_anchors`,
`entry_points`, and packs are **not composable** — composition is a graph the platform builds, and
it says so in the manifest. Packs cannot add tool grants: a pack that needs real QPU access is
refused unless the role executing it already holds that capability
([[multi-agent-research-collective]]).

## Contents of a pack

1. **Problem → Hamiltonian** builders. The reference implementation is
   `milimoquantum`'s active-space workflow: `generate_active_space_config` (RDM-based inactive
   space, MP2 frozen-core) → `map_to_qubit_hamiltonian` → parity transform + 2-qubit reduction
   (`backend/app/quantum/vqe_executor.py:225-321`), with an OpenFermion+PySCF path at
   `:747` and a real H2 example at `examples/h2_vqe.py`. **Blueprint-only: unlicensed**
   ((repo-audit-2026)) — reimplemented under Apache-2.0 against [[pennylane]]/[[qiskit]].
2. **Workflow graph, not code.** `univer`'s Flow (`packages/engine-workflow/src/{base,executor,
   trigger,runtime}.ts`) is the reference engine: typed edges, deterministic `base.getExecution`
   ordering, `trigger` for event-driven starts, and a `workflow-observe.service.ts` for live
   state (Apache-2.0, [[univer]]). A pack ships a graph; the platform executes and reports.
3. **Classical anchor.** Every pack declares the *cheap classical method that must be beaten* —
   RHF for VQE, a random forest for QML, plain DFT for QCFI. This makes the pack falsifiable and
   is the practical form of [[quantum-advantage-realism]].
4. **Evidence requirement.** Which observables, how many shots, which collapse mode
   ([[claim-credibility-wavefunctions]]), which fidelity floor ([[fidelity-accounting]]).
5. **Instruments.** `local-receivers`-style guarded endpoints for lab hardware
   ([[secrets-and-workload-isolation]]).

## The pack set

| Pack | Substance | Provenance | Anchor |
|---|---|---|---|
| **vqe-chemistry** | active-space VQE, H2→chain systems, RDM measurement reuse | Blueprint (milimoquantum, Milimo-QDD); [[pennylane]]/[[qiskit]] runtime | RHF / FCI |
| **qml** | state-overlap kernel matrices, differentiable data upload, small classifiers | runtime: `qiskit/primitives/{statevector_estimator,statevector_sampler}.py` + `qiskit/quantum_info` (Apache-2.0); gradients via [[pennylane]]. Note `qiskit-machine-learning` is **not** vendored here, so kernel code is written in-pack | GBT / random forest |
| **qcspectroscopy** | vibronic/vibrational Trotter-error analysis → predicted error *before* the run | `pennylane/labs/trotter_error/` (Apache-2.0): `fragments/vibronic_fragments.py` `vibronic_fragments(states, modes, freqs, taylor_coeffs, scheme="blocks"|"modes")` after arXiv:2411.13669 §III, `vibrational_fragments.py`, `realspace/` | classical DFT / analytic IRC |
| **benchmark-suite** | QMLBench-style tasks + scoring | [[qflow-studio]] spec (Blueprint) | its own documented baselines |

**PennyLane Labs is a pack goldmine and a caution at once.** `pennylane/labs/` ships `dla`,
`estimator_beta`, `profiler`, `tcdq`, `templates`, `transforms`, `trotter_error` under Apache-2.0.
`trotter_error` is the notable one: it derives *predictive* Trotter error from real-space fragment
structure **before** execution — an upstream, citable instance of exactly the
[[fidelity-accounting]] philosophy (a physical pre-prediction that gates the run), which means we
can compare our `F_pred` against a published method instead of only against ourselves. `labs/dla`
(dynamical Lie algebra) offers a real reachability/controllability measure to check
[[autonomous-ansatz-optimization]]'s expressibility heuristic against. But *labs code is
explicitly experimental*: APIs move, so pin the SHA and treat it as a volatile Submodule — legal
to link, not legal to assume stable.

## Genuinely new

Packs become **searchable harness dimensions**: a pack can propose an algorithm swap
(HW-CFFN ↔ hardware-efficient ↔ UCCSD, the search space named in
`milimoquantum/backend/app/ai/nano_ple.py:18`) and let [[autonomous-ansatz-optimization]] decide,
so the *workflow itself* is a mutable artifact under `Orchestrator`
(`platform/kubona/autoresearch/base.py:101`). And because pack execution is event-triggered,
**one run seeds the next run's hypothesis** — the closed-loop chemistry labs sketched in
*Quantum Agents* (arXiv:2506.01536) — which is **bibliography, not source**: reference 16 of
[[source-architecture-paper]], a paper we have never read. Nothing here has been ingested from it and
nothing is queued to be; if a design decision ever comes to depend on it, it gets fetched and
ingested then, and until then treat the closed-loop framing as an ambition with a citation rather
than evidence. We get an execution substrate either way.

## Boundary conditions

Domain packs are **not** claims of chemical or biological insight: a pack that cannot state its
classical anchor does not ship. No docking/scoring claims inherited from the unlicensed
drug-discovery material in the audit — and note that the folder once cited as an external
drug-discovery repo turned out to be **a clone of Kubona's own repository**
([[source-frontier-repo-audit-2026]] §provenance defect), so it is a method blueprint at best and
external evidence never. Graphical pack authoring
must round-trip losslessly through JSON or every pack edit silently corrupts
([[agent-ux-control-plane]], [[univer]]).

## Manifest, trust, and revocation (Q-20, closed 2026-10-10)

- **Manifest** (`schema_version: kubona-pack/v1`): name, X.Y.Z semver version,
  domain, description, author, `classical_anchor_methods` (required — a pack
  that cannot state its anchor does not load), benchmark systems, generator
  path, qubit floor, `capabilities`, `evidence_requirements` (min shots,
  collapse mode, fidelity floor), `entry_points`. Enforced by pydantic in
  `platform/kubona/science/domain_pack.py` (`DomainPackManifest`).
- **Versioning**: strict semver, fail-closed on mismatch (`"v1"` / `"1.0"`
  refuse); schema pin refuses unknown schema versions.
- **Correctness ownership**: optional Ed25519 owner signature over the canonical
  manifest bytes (`owner_pubkey_hex` + `signature_hex`, NaCl-verified).
  Unsigned packs load (author is an unattested claim); signed-but-invalid
  packs refuse (forgery is worse than anonymity).
- **Trust boundary**: `resolve_generator` allowlists `kubona.*` only — packs
  declare capabilities, never supply chains. Unresolvable generators refuse at
  resolve time (this caught the materials pack's phantom
  `kubona.science.lattice.HubbardModel`, which does not exist: its manifest
  data stays valid, but nothing can execute it until implemented).
- **Revocation**: `PackRegistry` with a `revoked.json` file (`revoke(pack_id,
  reason)` requires a non-blank reason); `load_all` refuses revoked packs
  everywhere. Revocation lives in the registry, never in the pack.
