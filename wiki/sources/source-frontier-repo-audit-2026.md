---
name: Frontier Repo Audit (Sept 2026)
type: source
tags: [audit, integration, licensing, governance, frontier]
date: 2026-09-27
updated: 2026-09-27
status: source
origin: first-party maintainer inspection — no raw/ document (see Evidence class below)
---

# Frontier Repo Audit — Sept 2026 (`repo-audit-2026`)

**Evidence class.** This is *not* a `raw/` document. It is a first-party maintainer
inspection of 25 local working copies under `dev-docs/` (gitignored; `raw/` remains
the only versioned source layer). Every claim is anchored to `path:symbol` or
`path:line` so any reader can re-verify against their own checkout. Claims that come
from a README rather than from code are marked *(doc claim)*. Where the code and a
README disagree, the code wins and the disagreement is flagged.

> ⚑ Convention: `(repo-audit-2026)` is a weaker evidence class than a `raw/` paper.
> It is sufficient to justify *engineering* decisions (what to build, how to wire it)
> and never sufficient to justify a *scientific* claim about the world.

**Method.** Per repo: `git remote -v` + `git log -1` (provenance and staleness),
`LICENSE*` header scan, directory inventory, then symbol-level reading of exactly the
modules Kubona would touch. Total: 25 repos, ~1.4 M files, 5 read in depth
(`milimoquantum`, `chickensoup`, `Milimo-QDD`, `buzz`, `paperclip`), 20 inventoried.
**Path anchors and their shelf-life.** Every `path:line` above resolves inside `dev-docs/`
— a gitignored scratch area (`.gitignore:11 dev-docs/`) holding two kinds of working copy:
`dev-docs/gather_insights_from/` (blueprint-only repos, read for patterns) and
`dev-docs/submodules/` (candidate git submodules, each a clone of the live GitHub URL
recorded in the table). **`dev-docs/` is deleted when development ends and is never pushed
to GitHub.** The durable artifact is therefore this page, not the checkout: re-verification
happens against the upstream URL at the recorded HEAD date, and any decision that must
outlive the audit is restated in the concept page it justifies.

**Ownership makes the licence question moot for our purpose.** `chickensoup`, `milimoquantum`,
`Milimo-QDD` and `mainza-consciousness` are the maintainer's own repositories (orgs `mainza-ai`
and `Milimo-Quantum`), and the audit's purpose is **insight gathering for implementation**, not
acquisition. For own repos, "no `LICENSE` file in-tree" is not a bar and not even a task: the
copyright holder is the same person who owns Kubona, so the grant exists the moment it is needed —
one commit, owed to nobody but the owner. Several of those repos are also **unfinished work**, which
is a second, independent reason not to treat them as supply: incomplete research code is read for
method, not lifted. What the licence question actually protects against is narrower and still real:
**third-party** trees, and the day a *derived* artefact of own code ships inside an Apache-2.0 Kubona
release (then add the file, before the release, not now). Third-party trees are bound by
what is in *their* tree, and the HEAD date recorded per row is the date each finding
applies to — `block/buzz` (Apache-2.0), `paperclipai/paperclip` (MIT),
`bilawalsidhu/gods-eye-view` (MIT), Xanadu `pennylane` (Apache-2.0),
`NVIDIA/Model-Optimizer` (Apache-2.0), `openqasm/openqasm` (Apache-2.0),
`vectorize-io/hindsight` (MIT), `tt-a1i/archify` (MIT), `anthropics/financial-services`
(Apache-2.0), `actions/runner-images`, `dream-num/univer`.



## Why this audit exists

The Phase-0→6 build (see [[kubona]]) produced a governed MCP fabric and a *skeleton*
discovery loop. The open question is no longer "can an agent drive a QPU safely" but
"can the platform discover something nobody asked it to". That question is answered by
deciding what to **grow** and what to **stand on**, and that decision requires knowing
what already exists in source form. This page is that knowledge, frozen.

## The four integration topologies

An integration class is a *liability assignment*, not a dependency choice.

| Class | Definition | Failure ownership | Allowed for |
|---|---|---|---|
| **Vendored submodule** | Pinned upstream tree inside `platform/third_party/`, built from source, hash-pinned, SBOM'd | Kubona owns every byte shipped | Apache-2.0 / MIT / MPL-2.0 with attribution; licence-compatible with a future Apache-2.0 core |
| **External binary** | Installed by the operator, invoked through a pinned subprocess contract; never vendored | Operator owns the binary; Kubona owns the contract + output validation | LLVM/MLIR toolchain, `bao`, container runtimes, `ansible` |
| **Sidecar service** | Independent process with its own storage, reached over MCP/HTTP; graceful-degrade when absent | Upstream owns its bugs | [[hindsight]] memory, [[openbao]], relay/queue services |
| **Blueprint** | Code read for design; re-implemented inside Kubona under our licence, with attribution in this file | Kubona owns the re-implementation | Everything, including unlicensed repos (read-only) |

**The unlicensed rule, and its one exception.** Three of the most instructive repos ship
**no `LICENSE` file** (`Milimo-QDD`, `milimoquantum`, `chickensoup`) — and all three belong to the
maintainer. A third-party repo with no licence is **all-rights-reserved by default**, and vendoring
one would make Kubona unable to ship the Apache-2.0 core promised in `AGENTS.md`; that is the rule
that binds everything from `llvm-project` (Apache-2.0 *with LLVM exception* — my header scan failed
to pattern-match its licence text, so it was checked by eye) downward. It does not bind the
maintainer's own code: reading it, mining it and reimplementing it involves no grant at all, because
there is only one rights-holder. `milimoquantum`'s README badge says MIT but carries no licence file
(README:18) — immaterial while the repo stays private WIP, and the first thing to fix if it is ever
published or shipped as a dependency.

## Audit matrix

Remote/HEAD/licence verified by `git remote -v`, `git log -1`, `LICENSE*` scan.
**URL convention:** the Remote column is `owner/repo`; the durable reference is always
`https://github.com/<owner>/<repo>`, taken from `git remote get-url origin` of each clone. Verified
remotes: `block/buzz`, `paperclipai/paperclip`, `vectorize-io/hindsight`,
`bilawalsidhu/gods-eye-view`, `anthropics/financial-services`, `mobile-next/mobile-mcp`,
`zhaoxuya520/reverse-skill`, `mainza-ai/{chickensoup,milimoquantum}`,
`Milimo-Quantum/Milimo-QDD`, `NVIDIA/Model-Optimizer`, `THU-MAIC/OpenMAIC`,
`MakazhanAlpamys/Soup`, `tt-a1i/archify`, `PennyLaneAI/pennylane`, `Qiskit/qiskit`,
`openqasm/openqasm`, `llvm/llvm-project`, `openbao/openbao`, `actions/runner-images`,
`dream-num/univer`, `tensorflow/{quantum,tensorflow}`.
Never cite a local `dev-docs/…` path as the source of a claim — clones are deleted when
development ends; only the URL and the SHA survive. "Primitive" = the single load-bearing thing
worth citing.

> ⚑ **Provenance defect found while checking URLs (2026-09-27).** The clone at
> `dev-docs/gather_insights_from/milimo-quantum-drug-discovery` has
> `origin = git@github.com:mainza-ai/kubona.git` — i.e. it is a **Kubona clone wearing another
> folder's name**, not a clone of a separate upstream. Everything this page attributes to that
> folder (7-stage deterministic engine, 15 py / 1857 LOC, RDKit + PubChem + SCScore, "no licence")
> is therefore about *our own tree* and must be re-verified against `platform/` before it is used
> as external evidence. It is **not** third-party prior art and must not be cited as such.

| Repo | Remote · HEAD · licence | Verified primitive | Class | Kubona landing |
|---|---|---|---|---|
| `milimoquantum` | mainza-ai · 2026-04-03 · **none** | `ANSATZ_CATALOG` (`backend/app/quantum/vqe_executor.py:92`), `meyer_wallach_score` (:351), `build_noise_model` (`noise_profiles.py:80`), `select_backend` (`hal.py:57`), import-whitelist sandbox (`sandbox.py:104`) | Blueprint | [[autonomous-ansatz-optimization]], [[fidelity-accounting]] |
| `chickensoup` | **mainza-ai** (mine) · 2026-07-18 · **none in-tree** | `ClaimWavefunction` (`src/quantum_credibility/wavefunction.py:159`), `score_claim` (:163) fed by `_source_diversity` (:20) / `_engagement_magnitude` (:28) / `_polymarket_prior` (:36); on quantum-stack failure it **relabels itself**: `vqe_backend = "classical-fallback: …"` (:214-219); real Qiskit `EstimatorV2`/`Statevector` with Aer→`primitives`→legacy fallback chain (`src/spacetime_engine/vqe_runner.py:24-38`); `meyer_wallach` (`src/spacetime_engine/entanglement.py:10`) | Blueprint (owner-relicensable) | [[claim-credibility-wavefunctions]], [[epistemic-curvature]] |
| `Milimo-QDD` | Milimo-Quantum (mine) · 2025-11-05 · **none in-tree** | 12-agent drug pipeline (`types.ts:2-14` `AgentName`); the quantum step is *a prompt asked to return a number*: "perform a **mock** quantum simulation … a single numerical value in kJ/mol", then a regex pulls the float out (`services/geminiService.ts:283-290`) | Blueprint (owner-relicensable) | [[quantum-domain-packs]] |
| `milimo-quantum-drug-discovery` | **⚑ origin is `mainza-ai/kubona`** (not an upstream!) · 2026-04-03 · n/a | 7-stage deterministic engine, 15 py / 1857 LOC, RDKit + PubChem + SCScore — **treat as our own code, re-verify before citing** | *not prior art* | [[quantum-domain-packs]] |
| `mainza-consciousness` | **mainza-ai** (mine) · 2025-09-30 · MIT | "quantum consciousness" = `np.random.dirichlet(np.ones(len(states)))` (`backend/utils/quantum_consciousness_engine.py:331`); **but** a real memory lifecycle: `MemoryType`/`MemoryConsolidationLevel` (`backend/utils/advanced_memory_architecture.py:18,29`), `decay_rate=0.95` (:51), `decay_threshold=0.3` (:73), `create_episodic_memory` (:76), `consolidate_memories` (:273), `_calculate_consolidation_strength` (:431) with time decay `max(0.1, 1-0.01t)` (:441); **plaintext `ghp_…` PAT in the remote URL of `.git/config`**; 6 checked-in `.py.bak` files | Blueprint (caution; owner-relicensable) | [[secrets-and-workload-isolation]], [[agentic-memory-and-observability]] |
| `paperclip` | paperclipai/paperclip · 2026-09-26 · MIT | MCP access governance as **four separated planes** — Application → Connection → Catalog → Profile (`doc/MCP-ACCESS-GOVERNANCE.md:31`), profiles = named allow/deny bundles attached by **Bindings** to `company/agent/project/routine/issue` (:57, :206), smoke test asserts `allow_read_tool / deny_write_tool / audit_written` (:92); `doc/LOW-TRUST-PRESETS.md` for "hostile or prompt-injected input"; `doc/execution-semantics.md` splits **structure / dependency / ownership / execution**; `doc/UNTRUSTED-PR-REVIEW.md` runs review in a container "intentionally separate from the normal dev image"; `doc/TASK-WATCHDOG.md`; `evals/promptfoo/`; `skills/para-memory-files` | Blueprint | [[multi-agent-research-collective]], [[secrets-and-workload-isolation]] |
| `buzz` | block/buzz · 2026-09-26 · **Apache-2.0** | `crates/ifc-core/src/lib.rs`: `can_flow_to` (:53, :172, :364), `ConfidentialityLabel<Universe, Principal>` (:107); `crates/buzz-persona/src/{pack,manifest,persona,resolve,merge}.rs` — versioned persona packs resolved and merged; `docs/agent-management-provenance.md`, `docs/agent-profile-identity.md`, `docs/MCP_DRIVEN_HOOKS.md`; relay = signed append-only event log *(doc claim, `ARCHITECTURE.md`)* | **Sidecar** (Apache-2.0 ⇒ submodule also possible) | [[confidentiality-lattice-provenance]], [[multi-agent-research-collective]] |
| `gods-eye-view` | bilawalsidhu/gods-eye-view · 2026-09-24 · MIT | `server/providers/local-receivers.js`: user-owned endpoints admitted from an env list, `isLocalIpv4` guard (:146, :295), and the feed's own clock decides staleness (:376) + `docs/LOCAL-RECEIVERS.md`; per-layer policies `src/layers/{traffic,vessels,launches,flights,radio,localAdsb}/policy.js`; **machine-checked boundaries**: `scripts/check-import-directions.mjs`, `scripts/check-package-boundaries.mjs`, `docs/CODE-BOUNDARIES.md` | Blueprint | [[agentic-memory-and-observability]], [[secrets-and-workload-isolation]] |
| `hindsight` | **vectorize-io**/hindsight · 2026-09-26 · MIT | retain/recall/reflect over memory banks (`client.retain(bank_id=…)`); memories live in **banks**, split into world-facts vs experiences pathways, stored as entities + relations + time series with sparse/dense vectors (`README.md:286`); `observations`, `mental models`, `knowledge pages` as first-class objects; **MCP server** exposing the three ops at `http://localhost:8888/mcp/{bank_id}/` — bank is part of the endpoint, so the label travels with the URL; LLM wrapper (`wrap_anthropic()`, LiteLLM underneath, per-call `hindsight_*` kwargs incl. **recall budget**); LongMemEval SOTA +41.9% vs RAG *(vendor doc claim)*, arXiv:2512.12818 | **Sidecar** | [[agentic-memory-and-observability]] |
| `financial-services` | anthropics/financial-services · Apache-2.0 | `managed-agent-cookbooks/*/agent.yaml`: `default_config: {enabled: false}` + per-tool allow-list, `mcp_servers` from `${ENV}`, `callable_agents` sub-manifests, `steering-examples.json` | Blueprint | [[quantum-domain-packs]] |
| `mobile-mcp` | mobile-next/mobile-mcp · Apache-2.0 | Four packaging surfaces for one MCP server: `manifest.json` v0.3, `plugin.json` (agent-plugins.org schema 1.0.0), `smithery.yaml`, `.mcp.json` | Blueprint | [[quantum-domain-packs]] |
| `reverse-skill` | zhaoxuya520/reverse-skill · MIT | "route before act" contract; `skills/scripts/case-guard.sh` — a scope gate before ACT (exit 0 ok / 2 not ready) whose `--force` flag prints "does not bypass scope hard gates" and still exits 2 (:122-127); it reads a human `ready_for_act` sign-off field; `Evidence→Finding→Path`; `--verify-hashes --strict` case review | Blueprint | [[multi-agent-research-collective]] |

| `qiskit` | Qiskit/qiskit · 2.6.0-dev · Apache-2.0 | 12 Rust crates plus a `cext` + `cext-vtable` C ABI (`crates/`) | **Submodule** | [[agentic-qir-compilation]] |
| `pennylane` | PennyLaneAI/pennylane · 0.46.0-dev · Apache-2.0 | `qcut` (circuit cutting), `ftqc`, `liealg`, `gradients`, `capture`, `backline`, `templates`, `qchem`; `tach.toml` machine-enforced module boundaries | **Submodule** | [[differentiable-quantum-stack]] |
| `llvm-project` | llvm/llvm-project · Apache-2.0 + LLVM exception | MLIR dialect/ODS/PDLL toolchain (`mlir/include/mlir/Dialect/*`, `mlir/tools/{mlir-tblgen,mlir-opt,mlir-pdll,mlir-reduce}`); **no quantum dialect** — `Dialect/Quant` is fixed-point *quantisation* | **External binary** | [[agentic-qir-compilation]] |
| `openqasm` | openqasm/openqasm · Apache-2.0 | ANTLR `qasm3Lexer.g4` / `qasm3Parser.g4` (`source/grammar/`) + language spec + `examples/*.qasm` | **Submodule** (grammar) | [[agentic-qir-compilation]] |
| `openbao` | openbao/openbao · MPL-2.0 | `internal/builtin/logical/{kv,transit,pki,kubernetes,database,ssh,totp}` + `internal/audit` | **Sidecar** + external binary | [[secrets-and-workload-isolation]] |
| `runner-images` | actions/runner-images · MIT | `images/ubuntu/toolsets/toolset-2404.json`: the declarative runtime-manifest pattern | **External binary** (docker) | [[secrets-and-workload-isolation]] |
| `Model-Optimizer` | NVIDIA/Model-Optimizer · 2026-09-26 · Apache-2.0 | `torch/{quantization,prune,sparsity,distill,nas/autonas,peft,speculative,kernels,export}` + the declarative recipe tree `modelopt_recipes/` (schema-pinned YAML, OmegaConf dotlist overrides) | **Submodule** (candidate) | [[differentiable-quantum-stack]] |
| `OpenMAIC` | THU-MAIC/OpenMAIC · MIT | LangGraph-v1 multi-agent classroom: `lib/{choreography,classroom,agent-runtime/lifecycle.ts}` | Blueprint | [[multi-agent-research-collective]] |
| `univer` | dream-num/univer · Apache-2.0 | `packages/{core,engine-formula,engine-render,docs,drawing,action-recorder,data-validation}` | **Submodule** (candidate) | [[agent-ux-control-plane]] |
| `Soup` | MakazhanAlpamys/Soup · Apache-2.0 | `src/soup_cli/{autodistill,autopilot,bench,eval,experiment,cans,mcp_server}`; extra `fast = ["unsloth>=2024.8"]` | **Submodule** (candidate, selective) | [[differentiable-quantum-stack]] |
| `archify` | tt-a1i/archify · MIT | typed-JSON → validated HTML diagram compiler: `validate --quality showcase` demands 9 artifact checks, 0 composition errors, 0 warnings; bounded repair through `subject`/`evidence`/`supportedFixes`; "a non-zero exit can never be described as success" (`archify/SKILL.md`) | **Submodule** (candidate) | [[agent-ux-control-plane]] |
| `quantum` (TFQ) | tensorflow/quantum · Apache-2.0 | Cirq↔TensorFlow interop (`core/ops`, `core/protocols`); README:124 "**not an officially supported Google product**" | Blueprint | [[differentiable-quantum-stack]] |
| `tensorflow` | tensorflow/tensorflow · Apache-2.0 | Keras 3 + TF core, substrate for GQE proxy-model training | Reference | — |



## Findings that change the architecture

1. **Credential hygiene in the maintainer's own working copies — one dead secret, one live
   sink.** `dev-docs/gather_insights_from/mainza-consciousness/.git/config` carries a `ghp_…`
   **plaintext PAT inside the remote URL**. Checked against the API on 2026-09-27 (headers only, no
   value recorded): **HTTP 401 — the token is already revoked/expired**, so there is nothing to
   revoke and the clone's own history never contained it (`git log --all -S"ghp_"` → empty). The
   finding that *is* open sits one directory up in the machine's config: `credential.helper=store`
   is set globally, so `~/.git-credentials` holds a **live** `gho_…` GitHub CLI OAuth token with
   scopes `read:user, repo, user:email, workflow` — write access to every repo the account reaches,
   in plaintext, and *not* the token the keyring uses (different hash ⇒ a stale gh login that was
   never cleaned up). Both halves are the same lesson and the strongest argument in this audit for
   [[secrets-and-workload-isolation]]: a token that is convenient ends up in a URL or a plaintext
   file somewhere. Actions: kill the `store` helper (keychain/`gh` only), revoke the stale grant in
   the GitHub UI rather than just deleting the file, and use SSH remotes. Risks rows:
   *secret-in-remote* (closed by verification) and *plaintext-credential-sink* (open).
2. **"Quantum" appears in these repos with three different meanings.** chickensoup
   prepares a real `QuantumCircuit` and reports *which backend answered*
   (`vqe_runner.py:171,196,206`); `milimoquantum` runs real VQE/QAOA through Qiskit/qutip
   with an MPS path (`vqe_executor.py:597,687,783`); `mainza-consciousness` draws from
   `np.random.dirichlet` and calls the draw consciousness. Therefore every "quantum"
   number Kubona emits must carry a **quantum-provenance tag** —
   see [[claim-credibility-wavefunctions]] § the provenance rule.
3. **MLIR has no quantum dialect.** `grep -i quantum mlir/include/mlir/Dialect` is empty,
   and `Dialect/Quant` is fixed-point quantisation for ML inference. "Compile through
   MLIR" therefore means **we author the dialect** (ODS + PDLL). That is a scope decision,
   not a dependency — see [[agentic-qir-compilation]].
4. **The best drug-discovery "quantum" step in the family is theatre.** Milimo-QDD's
   Quantum Processing Agent asks an LLM for a binding energy
   (`geminiService.ts:283-290`). Replacing that one prompt with a provenanced VQE run is
   the single highest-value domain breakthrough visible from here — [[quantum-domain-packs]].
5. **Unlicensed ≠ unusable.** The three most instructive repos ship no licence. The
   Blueprint class exists so their ideas still land without infecting the future Apache-2.0
   core: attribution here, re-implementation in our own tree, never a vendored copy.
6. **Governance primitives are converging across unrelated codebases.** `paperclip`
   (deny-by-default tool config), `buzz` (`can_flow_to`), `reverse-skill` (`case-guard`
   exits 2 and `-Force` cannot bypass it), `gods-eye-view` (`policy-*.js` with a
   co-located test), Anthropic cookbooks (`default_config: {enabled: false}`). Same
   invariant in five dialects: **no capability by default; capability is a signed,
   testable object.** Kubona already believes this about money — [[cost-governance]]
   extends the belief to tools, memory, and data flow.
7. **Honest fallbacks beat confident ones.** chickensoup never hides its degradation:
   `AerEstimatorV2 → Statevector → Aer-statevector → numpy-fallback`, and the string
   `classical-fallback: {exception}` is returned *in the result* (`:219`). Same posture as
   Kubona's `engine: "rust-qir-core"` vs `"python-reference"` marker in
   `platform/kubona/qir/native.py:74`. This is the pattern for every dual-path engine we ship.

## Not adopted (and why)

| Candidate | Reason |
|---|---|
| `mainza-consciousness` quantum/EM/entropy modules | Not physics. Draws random numbers and names them quantum; no falsifiable prediction. Its **memory lifecycle** is adopted instead. See [[mainza-consciousness]]. |
| `mainza-consciousness` "simulate a human brain locally" | 32 GB RAM claimed for ~7.5 B parameters; unbounded memory growth (see `wiki/concepts/risks.md`). |
| chickensoup `field_manipulator/`, `prompt_superposition` | Metaphor presented as mechanism; no measured coupling. Kept as *naming* inspiration only. |
| TFQ as a runtime dependency | Self-declared unsupported (`quantum/README.md:124`); Blueprint for Cirq-interop patterns only. |
| Milimo-QDD UI as a template | Proprietary-style single-tenant React SPA; Kubona ships the Flutter console ([[agent-ux-control-plane]]). |
| Vendoring `llvm-project` | Source-build cost dwarfs Kubona itself; external binary with a pinned version contract instead. |
