---
type: roadmap
date: 2026-08-16
updated: 2026-10-10
tags: [kubona, plan]
sources: [source-gap-analysis-plan, source-openai-math-corpus]
status: living
---

# Roadmap — Kubona Phases & Acceptance Gates

> ## How to read this page (2026-09-27)
> **These are acceptance criteria for the blueprint, not a progress bar.** A phase gate
> marked *proven* means a prototype test exercises that mechanism and passes — most of
> them on simulators, which is a weaker evidence class than the gate text demands. The
> blueprint itself is still mostly undecided: 1 `settled` page against 25 `adopted` and 13
> `open-question` ([[questions]]). Building further will not close a gate that needs a real
> QPU, a third-party reviewer, a public repo, or a human surface — see
> [[questions]] §"Surface, ecosystem, operations".
>
> **Prototype state** (evidence, not decisions): `platform/` — 159 Python + 3 Rust +
> 1 Flutter widget test green locally as of 2026-09-27. Repo `github.com/mainza-ai/kubona`,
> **private by decision** until the blueprint is legible enough to direct contributors
> ([[questions]] Q-25, closed; residual history work in Q-27). **M0 hardened:** single non-`/tmp`
> state root, production fail-closed auth, `kubona serve`/`doctor` entrypoints, CI scaffold,
> x402 fail-closed. The hosted `ci.yml` push pipeline had **never been green (all 17 recorded
> runs failed)** while `scripts/ci.sh` passed locally; fixed 2026-09-27. The earlier diagnosis
> here was wrong: `run_all()` classified skips correctly — the gate's *rule* was written twice,
> once filtering skips (`ci.sh`) and once not (`ci.yml:68`), so legitimate fail-closed providers
> failed the build. The rule now lives in one function, `release_gate()`, run by both paths as
> `python -m kubona.canary --gate`. **Verified green on the hosted runner** (first green in the
> pipeline's history): the gate step reports `release gate: OK (3 executable providers; 5 skipped)`
> — it really executed providers rather than passing on nothing. CI counts differ from local:
> **156 passed + 3 skipped on the runner, 159 passed locally**, so "N tests green" must always
> name the environment. CI had also been watching only `main`, which this repo advances by
> fast-forwarding develop onto it, so no commit was ever gated as it landed; `develop` and
> `workflow_dispatch` are triggers now. ([[implementation-gaps]])
> Credential-free execution: local-simulator, qiskit-aer, dwave-annealing (QUBO);
> IBM/Quantinuum/Rigetti have real SDK paths validated against the real SDKs but need creds;
> Pulser/StrawberryFields are modality-gated and fail closed.

The working implementation plan for Kubona. Phase content, exit criteria, and team shape
trace to §§7–8 of [[source-gap-analysis-plan]], with the §11.5 (autoresearch engine) and
§12.7 (x402) roadmap revisions folded in. **Task ordering, dependency chains, gate
structure, and tracking mechanics below are maintainer synthesis** derived from those
sections. Total ~30 weeks to Phase 4 release-readiness; Phase 5+ is continuous scale +
business-model work, and **Phase 6 (Weeks 37+) adds the Science Discovery & Collaboration
layer** (maintainer synthesis).

## How to use this page

- Every phase ends in an **exit criterion = go/no-go gate**. Do not start the next phase
  until the gate is met; record the result (pass / fail + reason) in [[log]].
- This page stays mirrored to the source plan; when the source's plan changes, update this
  page and log the change.
- Details live on the linked pages ([[cost-governance]], [[qir-mlir-compilation]],
  [[karpathy-autoresearch]], …); this page is the sequencing, not the design.

## Sequencing principles (why the phases are ordered this way)

1. **Functional product first** (§9) — build a working core before process, governance, or
   monetization. This single rule drives every "deferred by design" line below.
2. **Cheap now, expensive later** (§4.7, §12.7) — three designed-in-early, deferred
   activations: PQC transport swappability (design P0 → harden P3), x402-compatible payment
   interface (design P1 → activate P5), plugin provider trait (architecture P0 → new
   providers P2).
3. **Open-first, no closed dependencies** (§2.10) — Apache-2.0 from day one; open-source
   components only; [[qbraid]]/[[strangeworks]] are references, never load-bearing.
4. **De-risk with existing assets** (§8) — Milimo Quantum's HAL/agent-mesh and Finance
   Claw's two-stage approval de-risk P0–P1 — *but* those claims are single-source and must
   be validated against the real codebases first ([[mainza-milimo-foundation]]).
5. **Cost governance is non-optional rail** (§2.5) — exists from Phase 1, never deferred.

## Phase gates at a glance

| Gate | When | Verdict asked (from the source's exit criteria) |
|---|---|---|
| **P0 gate** | Week 4 | Simulator job end-to-end via MCP with a provenance record written, open-source deps only |
| **P1 gate** | Week 10 | VQE loop runs on a real QPU with budget enforced + every step provenance-logged |
| **P2 gate** | Week 16 | A human watches *and interrupts* an in-flight multi-hour hybrid loop from the console |
| **P3 gate** | Week 22 | Third-party gateway security review passes; canary CI caught a real regression in staging |
| **P4 gate** | Week 30 | Public repo + documented plugin API; third-party provider integration merged; Search Mode beats a hand-designed baseline (in simulation) |
| **P5 gate** | 31+ | Multi-tenant scale + monetization rail live (business-model exploration, §12.7) |
| **P6 gate** *(synthesis)* | 37+ | A lab runs the full discovery loop — literature ground → experiment → structured report → archived library entry — using user- and agent-created workflows, every claim tracing to a ledger record or citation |

## Phase 0 — Foundations (Weeks 1–4) → gate P0

> **Status (2026-08-16):** skeleton committed under `platform/` — four MCP servers
> (compiler/execution/cost/provenance, MCP stdio), gateway (auth + swappable transport,
> audit), provider trait + pure-numpy local simulator + 3 capability-schema entries,
> content-addressed hash-chained ledger, cost pre-flight, QASM2-subset IR. **P0 gate test
> green end-to-end over stdio MCP** (`platform/tests/test_phase0_gate.py`, 13 tests).
> Apache-2.0 LICENSE added. OAuth 2.1 interface stubbed (dev auth); real IdP wiring,
> Qiskit adapter, and the streamable-HTTP gateway remain.

Ordered execution (dependencies within the phase):

1. **Apache-2.0 license + public repo** — one-time decision, cheap now, expensive later (§7).
   *(Status 2026-08-16: repo created privately at github.com/mainza-ai/kubona; kept private
   until a working core exists — per maintainer decision. Flips public at the Phase-0 gate:
   simulator job end-to-end via MCP with a provenance record.)*
   > ⚑ Correction (2026-09-29): that flip condition is superseded. The Phase-0 gate passed and the
   > repo did **not** go public — [[questions]] Q-25 ruled the timing turns on the blueprint
   > becoming contributor-legible, not on a gate passing. The Apache-2.0 decision stands; only the
   > trigger moved. See [[kubona]] §Posture.
2. **Federated MCP server skeleton** — compiler / execution / cost-governance /
   eval-provenance, stub tool schemas ([[mcp-and-context-flooding]]).
3. **OAuth 2.1 on the gateway**; transport layer designed **swappable from day one**
   (PQC itself deferred to Phase 3 — cryptographic agility, not a retrofit).
4. **Provider abstraction as a plugin trait** — Kubona's own open-source interface with the
   first two implementations: Qiskit (IBM) and local/GPU simulator ([[nvidia-cuda-q|CUDA-Q]]
   open components). Trait from day one so later providers are pure additions, not core
   rewrites.
5. **[[provider-capability-schema]]** with 2–3 initial entries (IBM via Qiskit, local GPU
   simulator, +1).

**Gate P0:** agent submits a simulator job end-to-end through the MCP chain and gets results
back with a provenance record written, **using only open-source dependencies**.

## Phase 1 — Core execution + cost governance (Weeks 5–10) → gate P1

> **Status (2026-08-16):** implemented under `platform/` — session abstraction
> (persistent, budget-ceilinged), cost-governance service (hard ceilings, two-stage
> REVIEW→HOLD→APPROVED gate, burn-rate telemetry), x402-compatible payment interface
> (quote/verify; free tier live, x402 stub), ZNE mitigation (noise-aware backend +
> Richardson extrapolation), QIR lowering (Python reference + Rust `qir-core` crate,
> cargo-tested), compiler `lower_circuit` tool. **P1 gate test green** — budget-enforced
> VQE loop with step-level provenance + paid approval flow (28 tests total). Real-QPU
> execution still awaits provider credentials; the full mechanism is proven on the
> simulator chain.

- `job_id` session abstraction threaded through every call (§4.3).
- [[cost-governance]] service: mandatory simulator pre-flight, hard budget ceilings,
  two-stage approval gate, real-time burn-rate telemetry.
- Design the cost-gov payment interface **x402-compatible from day one** (§12.7 — cheap now,
  expensive to retrofit; same logic as PQC in Phase 0).
- [[qir-mlir-compilation]] lowering for OpenQASM 3.0 + Qiskit-generated circuits (Rust core).
- [[mitiq]] integration, **ZNE first** (highest value-to-effort of the three techniques).

**Gate P1:** a VQE loop runs end-to-end on a real QPU via direct provider SDK integration,
budget ceiling enforced, every step provenance-logged.

## Phase 2 — Multi-provider + control plane (Weeks 11–16) → gate P2

> **Status (2026-08-16):** implemented under `platform/` — 3 new provider capability
> entries (quantinuum/rigetti/neutral-atom) with optional-dep plugins (registry = 7,
> incl. executable `qiskit-aer-simulator` for credential-free Qiskit testing per
> maintainer direction — open-source first);
> LOCC wire cutting for Z-diagonal expectations (numerically verified vs uncut);
> identity groundwork (Ed25519 keys, owner-delegated authority, instant cascade
> revocation); loop supervisor + `control` MCP server (watch/pause/kill); LangGraph
> shim with a reference VQE StateGraph; Dart console scaffold (models + gateway
> client + two priority surface contracts — widgets require a Flutter machine).
> **P2 gate test green** — watch + interrupt an in-flight loop over MCP; console now
> includes a read/approve-only REST API + a **building Flutter web app** (Cost &
> Approvals, Live Loops) — Flutter SDK installed (Homebrew); 52 Python tests + 1
> widget test + `flutter build web` all green.

- Add [[quantinuum]] (pytket), [[rigetti]] (pyQuil), [[neutral-atom-quera-pasqal]] (Pulser)
  as direct, open-source SDK integrations — validating the P0 plugin trait.
- [[circuit-cutting]] (Qiskit Circuit Knitting Toolbox) for width/depth-constrained circuits.
- Dart/Flutter console, highest-priority surfaces first (§5.3): cost/approval console +
  circuit visualizer; **provenance browser + fleet-status views follow in Phase 3**.
- Compatibility shim for ≥1 orchestration framework — LangGraph recommended (long-running
  stateful graphs map cleanly onto the VQE session model).
- **Identity groundwork** *(synthesis)*: keypair identities for users and agents + owner-
  delegated authority beneath the OAuth/PQC layer ([[user-identity-and-profiles]]) —
  scaffolding for the Phase-6 collaboration layer.

**Gate P2:** a human operator can watch and interrupt an in-flight multi-hour hybrid loop
from the Dart console.

## Phase 3 — Security hardening + provenance depth (Weeks 17–22) → gate P3

> **Status (2026-08-16):** implemented under `platform/` — hybrid PQC transport
> (X25519+ML-KEM768, HKDF-combined, AES-GCM; tested), per-call identity-aware
> authorization (fine-grained scopes, live delegation revocation; tested),
> immutable ledger audit of every gateway call, hardware-canary CI module
> (CLI + ledger + regression-catch test), provenance server as MCP resource
> (`provenance://records`, `provenance://best` aggregation), console REST
> `/api/provenance` + `/api/fleet`, Flutter web console now 4 tabs (builds),
> `docs/security-review.md` package for the third-party review. **65 Python tests
> + 1 widget test green.** Real OAuth 2.1 IdP + gateway-wire ML-KEM + third-party
> review execution remain (hardening/deployment + human gate).

- Hybrid [[security-and-pqc]] transport (ML-KEM) on the gateway — the Phase-0
  swappability pays off.
- Per-call identity-aware authorization (replacing coarse RBAC roles).
- Hardware-canary CI — nightly regression against every connected backend.
- [[provenance-and-reproducibility]] server queryable as an **MCP resource**
  ("what worked best historically" queries).
- Console: provenance/experiment browser + provider/fleet status views (the Phase-2
  follow-ons).

**Gate P3:** third-party security review of the gateway passes; canary CI caught at least
one real provider-side regression **in staging** before it reaches an agent.

## Phase 4 — Evaluation, autoresearch engine, release prep (Weeks 23–30) → gate P4

> **Status (2026-08-16):** implemented under `platform/` — the four-part harness
> contract + Orchestrator (keep-or-discard, write-scope, simulator-only,
> coarse-to-fine search); three harnesses ship (VQE ansatz, mitigation/ZNE,
> correctness-regression which now **gates the canary/release**); **Search Mode
> beats a hand-designed baseline** (tested) and files to the library; GQE as the
> 4th harness (n-gram proposal; Unsloth transformer is the production path);
> provenance-backed algorithm library (simulator → cost-approved → library
> pipeline tested); **§13 meta-loop** (propose-only, shadow-tested, human-gated);
> eval corpus (bell/ghz3/qft2 invariants) + agent-behavior evals (budget,
> simulator-first, WBP-response); provider-registration SDK +
> `docs/contribution-governance.md`. **84 Python tests + widget test green.**
> Remaining Phase-4 gate items: human review (evals done: QuantumKatas 13 at 1.0, HumanEval 136 golden + 15 excluded-named, harness live-proven, PennyLang refused-named under Q-26 — 2026-10-10),
> a first graded sidecar provider (SDK conformance suite built 2026-10-10: 9 checks, fail-closed grading — the Q-24 gate artifact), and the human review — plus the P4
> stretch (GQE retraining pipeline via Unsloth) once a GPU/cloud host exists.
>
> ⚑ Q-24 decision (2026-10-10, **operator chose C**): the gate
> demands a merged outside PR with zero contributors and one maintainer. Four
> ways out — **A.** narrow the gate (an outside *identity*, e.g. a contracted
> reviewer or second operator identity, counts as outside eyes instead of a
> community); **B.** grow the team plan (recruit 1–2 external provider
> reviewers before Phase 4); **C.** vendor-maintained providers — third-party
> code lives in sidecar repos, core reviews only the registration interface,
> and the SDK conformance suite becomes the gate artifact (consistent with the
> settled sidecar discipline in
> [[integration-topology-and-licence-discipline]]); **D.** defer (drop the
> outside-PR requirement to Phase 6). **Recommendation: C**, with A as
> fallback — the only option that works with a one-maintainer team and
> strengthens the supply-chain boundary instead of weakening the gate. Chose C: vendor-maintained sidecars; SDK conformance suite is the gate artifact.

- Eval corpora: QuantumKatas + Qiskit HumanEval + PennyLang; **agent-behavior evals**
  (budget compliance, simulator-first compliance, WBP-detection response)
  ([[evaluation-and-benchmarks]]).
- Build the [[karpathy-autoresearch]] **general engine first** (§11.5): pluggable harness
  interface + orchestrator, shipping with **three harnesses** — VQE ansatz search ·
  mitigation-strategy search · correctness-regression. **QAOA and compilation-pass search
  follow once the engine has proven out** (not in the first ship).
- **Kubona Search Mode** (§11.4): the autonomous ansatz-search MVP, simulator-only by
  default; validates the sandboxed-write-scope pattern before GQE lands.
- **GQE circuit optimization as the 4th harness** (v2) — same four-part contract, not a
  separate system; retraining runs on [[unsloth]] (2× faster LoRA, fits Apple
  Silicon/consumer GPUs) so the §13.3 flywheel stays affordable *(synthesis)*.
- Correctness-regression harness wired into the Phase-3 canary CI — **gates releases**.
- Plugin/provider-registration SDK for third-party backend contributions + contribution
  governance doc for the tool-surface security boundary.
- **Stretch goal — meta-level loop (§13):** a scheduled reflection job that **proposes**
  (never auto-applies) proposal-strategy and harness-registry changes for human review —
  the [[meta-search-and-self-evolution]] tier that makes the engine self-evolving. Gated on
  the object-level engine shipping first (a meta-loop with nothing to reflect on is
  premature); auto-application of meta-changes is Phase 5+.

**Gate P4 (three criteria):** public repo with documented plugin API; ≥1 third-party
provider integration merged (validates the extensibility model); Search Mode produced ≥1
ansatz beating a hand-designed baseline on a benchmark problem, entirely in simulation.
**Plus two §11.5 additions:** the correctness harness has caught ≥1 real regression in
staging; the algorithm library has ≥1 entry through the full **simulator → cost-governance-
approved → real-hardware-validated** pipeline.

## Phase 5 — Scale + breadth (Weeks 31+) → gate P5, then continuous

> **Status (2026-08-16):** implemented under `platform/` — breadth providers
> (xanadu photonic + dwave-annealing with a credential-free QUBO solver);
> jurisdiction/export-control routing enforcement (§2.9); multi-tenant scale
> (tenant-scoped ledger + session stores, atomic+locked chain-safe appends);
> x402 activation + algorithm-library metering (free summary + sub-threshold
> microtransaction, ledger-as-payment-proof); meta-loop auto-application gated
> by safety period + shadow-mode win (§13.5). **96 Python tests green.**
> Remaining: real photonic/ocean credentials + SDK wiring, x402 off-ledger
> crypto verification on a real rail, multi-tenant horizontal deployments, and
> the human business-model sign-off.

- [[xanadu-strawberry-fields]] photonic/CV integration; [[dwave]] Ocean SDK deepening.
- Export-control/jurisdiction enforcement at the routing layer (§2.9; the
  [[provider-capability-schema]] jurisdiction field becomes load-bearing).
- Horizontal scaling of the provenance ledger + cost-governance for multi-tenant/enterprise.
- Activate [[x402]] for sub-threshold transactions and algorithm-library metering (§12.7).
- Auto-application of meta-loop changes via the shadow-mode staged rollout (§13.5) — only
  after the Phase-4 propose-and-review tier has run safely.

**Gate P5:** multi-tenant scale + first live monetization rail; thereafter the roadmap
becomes business-model exploration (agent-readiness audits, library metering), not core
build.

## Phase 6 — Science Discovery & Collaboration (Weeks 37+) *(maintainer synthesis)* → gate P6

> **Status (2026-08-16):** implemented under `platform/` — workflow engine (YAML
> DSL, gates, agent-created-requires-approval, ledger-recorded, library),
> agent registry/personas with no-escalation creation, experiment notebook/
> results studio (structured, claim-traceable reports), governed scholarly
> integrations (arXiv/OpenAlex + offline mock, license/freshness provenance),
> and the **full discovery loop — the Phase-6 gate test passes end-to-end** (a
> tenant-scoped lab runs ground → experiment → report → library archive, every
> claim tracing to a ledger record or citation). **105 Python tests green.**
> Remaining Phase-6 items: live arXiv/OpenAlex network wiring into the console
> (mock/offline path proven), the two new console surfaces (notebook + labs/
> workflows views) in Flutter beyond the current 4 tabs, and unsloth-trained
> personas on a GPU/cloud host.

The human/AI science layer on top of the Phase-0–5 quantum core. Everything here is
scaffolded earlier (identity in P2, engine + meta-loop in P4) and shipped only after a
working, governed core.

- **Profiles & labs:** multi-user identities, lab/team communities with scoped state
  ([[user-identity-and-profiles]]).
- **Workflow & agent authoring:** versioned YAML workflows with triggers + approval gates;
  agent registry with personas & delegated authority; agent-created agents under the §13.4
  safety rail ([[workflows-and-agent-authoring]]).
- **Experiment notebook & results studio:** the two new control-plane surfaces
  ([[agent-ux-control-plane]]) — structured, citable, reproduction-ready reports over the
  provenance ledger ([[science-discovery]]).
- **Scholarly & data integrations:** Tier-1 first (arXiv, OpenAlex), Tier-2 domain data
  (materials, biodiversity) as governed MCP resources ([[science-data-integrations]]).
- **Full discovery loop:** the platform runs autonomous experiment campaigns — literature
  ground → hypothesis → experiment → analysis → report → library → meta-learn —
  inside the existing cost/capability gates.
- **Formal verification & mathematical discovery anchors:** [[openai-math]] integrated as
  adopted submodule (`submodules/math`, Apache-2.0, 372 families); [[lean4]] / [[comparator]]
  formal verification challenge evaluator; exact analytical baselines ([[mathematical-physics-anchors]])
  grounding quantum discovery campaigns with $\rho = 1.0$ epistemic certainty.
- **Personas & fine-tuned domain models** via [[unsloth]] (private, local; ties to the
  Milimo Apple-Silicon HAL, [[mainza-milimo-foundation]]).

## Deferred by design (until a working product exists)

- Fit-for-governance process and formal contribution governance (P4 is *targeted* only if
  outsiders appear), x402 activation, provider agent-readiness-audit offering —
  all sequenced after a working product ("functional product first", §9).
- Generative/agent-materialized UI (MCP Apps / A2UI) → v2+, not for core surfaces.

## Critical path & cross-phase dependencies (maintainer synthesis)

The chain that must not slip — and the debt that must not be left unpaid:

1. **P0 plugin trait + P1 QIR/MLIR (Rust core)** → everything downstream assumes the
   provider abstraction and compiler core. Rust is the highest-leverage hire (§8).
2. **P1 cost-gov + x402-compatible interface** → unblocks P5 monetization.
3. **P0 provenance record → P3 ledger-as-MCP-resource → P4 algorithm library** —
   the compounding asset; never let earlier phases drop the record.
4. **P3 canary CI → P4 correctness harness wiring** (release gate) — the §11.5 exit
   additions depend on P3 being real.
5. **P4 Search Mode → GQE harness** — write-scope pattern must be validated first.

## Team loading (§8)

> ⚑ Architectural Standardization (2026-10-04): The production engineering stack is standardized on
> **Go (Backend Orchestrator)** + **TypeScript/Next.js (Frontend UI)** + **Python (Quantum/Data Bridge)** +
> **Rust (Compiler Kernel)**, retiring Dart/Flutter.

| Role | Phases | Notes |
|---|---|---|
| Go systems engineer (Orchestrator/Agent engine) | 0–5 | Core backend: lightweight goroutines/channels for 1000s of agent loops, state engine, Docker/K8s sandbox lifecycle |
| TypeScript / Next.js frontend engineer | 1–5 | Control plane: React Flow / xyflow node workflow builder, Yjs multiplayer presence, deep DOM paper/math viz |
| Python quantum & data bridge engineer | 0–5 | Direct Qiskit/PennyLane/PySCF/Cirq/Pulser bindings via gRPC microservices |
| Rust systems engineer (compiler/QIR core) | 0–5 | High-performance QIR lowering, OpenQASM AST parsing, PyO3/IPC bindings |
| Security reviewer (contract) | 3 | PQC + MCP + AP2 mandate threat-model review |

**Stack rationale:** Dart/Flutter falls short for deep DOM scientific paper selection, WebGL molecular embedding (Mol*),
and node-based visual workflow canvases (React Flow). Go handles concurrent autonomous agent loops with minimal memory
overhead and manages sandbox infrastructure natively.

## Schedule risks to watch

- Managing gRPC IPC contracts between the Go orchestrator and Python quantum runtimes.
- Provider queue times/availability during P1–P3 hardware work — off-peak scheduling and
  Sessions ([[quantum-cloud-pricing]]).
- Single-source Milimo reuse claims may not survive validation — plan P0–P1 as if they
  might not.
- CUDA-Q open components depend on NVIDIA's OSS posture — keep the plugin trait honest so
  the dependency stays swappable.

## Living-plan mechanics

- Mirrored to [[source-gap-analysis-plan]] §§7–9, 11.5, 12.7; when that source changes,
  update this page and note it in [[log]].
- Phase transitions are recorded in [[log]] as `roadmap | gate P<n> <pass|fail> — <reason>`.
- This page stays `status: living`; [[log]] is append-only.

## Post-Audit Modern Implementation Roadmap (2026-10-04)

Following the forensic gap audit of the 25 source repositories in `dev-docs/`, the roadmap
prioritizes eliminating all shims and establishing local verification before cloud hardware:

### The Local Classical-Quantum Testing Gate (Precondition to Cloud Hardware)
Before any job is dispatched to paid cloud QPUs (IBM, Quantinuum, QuEra, D-Wave):
1. **Classical Anchor Verification:** Calculate exact ground state and dissociation curves using ab-initio
   Restricted Hartree-Fock (RHF) and Full Configuration Interaction (FCI) via PySCF.
2. **Local Simulation & Transpilation:** Execute circuits on `qiskit_aer.AerSimulator` and `Statevector`,
   evaluating Meyer-Wallach expressibility $Q$, 2-qubit gate depth, and Clifford fidelity probes.
3. **Graph-Partitioned Wire Cutting:** Partition circuits exceeding local qubit capacities using PennyLane
   `qcut` and KaHyPar graph partitioning, reconstructing distributions via Quasi-Probability Decomposition (QPD).
4. **Ring 1 Claim Collapse:** Verify whether energy errors fall within chemical accuracy ($|E_{\text{VQE}} - E_{\text{FCI}}| \le 1.6\text{ mHa}$)
   and assert claims only when corroborated ($P_{\text{corr}} \ge 0.75$).

### The Hybrid LLM Configuration Plane
A unified settings engine in Go and Next.js manages:
- **Local Engines:** Automatic zero-config discovery for oMLX (port 8787 hosting `Qwen3.8-27B-Uncensored-OrcaRouter-MLX-8bit`,
  `Llama-3.2-3B-Instruct-bf16`, and `Qwen3.6-35B-A3B-UD-MLX-4bit`), Ollama (port 11434), and LM Studio (port 1234).
- **Cloud Providers:** Configurable endpoints and API keys for Anthropic Claude and OpenAI.
- **Sovereign Air-Gap Switch:** Physical toggle in the control plane that severs external egress and enforces 100% local inference.
- **Agent Persona Routing:** Assigning distinct models to individual roles (`ansatz-scout`, `device-auditor`, `disconfirmation-officer`).

---

*Phase content and exit criteria trace to the source; gates, ordering, critical path, and
tracking conventions are maintainer synthesis, kept deliberately explicit so they can be
disagreed with and revised.*
