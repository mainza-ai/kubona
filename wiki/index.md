---
type: catalog
date: 2026-08-16
updated: 2026-10-04
tags: [index]
status: living
---

# Index

Catalog of every page in the wiki (89 pages), updated on every ingest. When answering a
query, read this first to find relevant pages, then drill in.

> **What this wiki is.** The **blueprint** for Kubona — the accumulated insight used to
> shape the platform *before and while* it is implemented. It is not a build log, and the
> state of the code is not the state of the design. Start at [[questions]] for what is
> still unresolved; read [[implementation-gaps]] only for prototype facts.

## The blueprint layers

Not a directory split — a reading map. Any page belongs to one layer; when a new idea
arrives, place it here first and it will find its page.

| Layer | What belongs | Pages | Decidedness |
|---|---|---|---|
| **L0 · Method** | How Kubona is allowed to *know* and *claim*: evidence classes, anchors, credibility | [[quantum-advantage-realism]], [[evaluation-and-benchmarks]], [[provenance-and-reproducibility]], [[claim-credibility-wavefunctions]], [[epistemic-curvature]], [[fidelity-accounting]] | mostly `open-question` — the least grounded, most differentiating layer |
| **L1 · Control** | The governance rail that makes agent spend survivable | [[cost-governance]], [[spend-mandates-and-delegation]], [[security-and-pqc]], [[secrets-and-workload-isolation]], [[user-identity-and-profiles]], [[provider-capability-schema]], [[quantum-cloud-pricing]] | `adopted`, with AP2 mandates anchoring the approval gate |
| **L2 · Compute** | Compilation, providers, mitigation, the machine itself | [[qir-mlir-compilation]], [[protocol-strategy]], [[mcp-and-context-flooding]], [[nisq-and-hybrid-orchestration]], [[error-mitigation]], [[circuit-cutting]], [[vqa-vqe]], [[barren-plateau]] + provider entities, [[qiskit-mcp-servers]] | `adopted` mechanisms, `tracked` vendors |
| **L3 · Research** | Autonomy that compounds: search, meta-search, packs, the discovery loop | [[science-discovery]], [[karpathy-autoresearch]], [[meta-search-and-self-evolution]], [[five-ring-research-loop]], [[multi-agent-research-collective]], [[quantum-domain-packs]], [[agentic-memory-and-observability]], [[autonomous-ansatz-optimization]], [[differentiable-quantum-stack]] | mostly `open-question` |
| **L4 · Surface** | Where a human sees and limits the agent | [[ui-ux-vision]], [[agent-ux-control-plane]], [[workflows-and-agent-authoring]], [[confidentiality-lattice-provenance]] | `open-question` — spec far ahead of build |
| **L5 · Ecosystem** | How it gets used, funded, and extended by others | [[agent-resource-economy]], [[x402]], [[cloudflare]], [[aaif]], [[a2a]], [[ap2]], [[integration-topology-and-licence-discipline]] | 1 `settled` policy; standard delegation and payment rails |

## Overview & meta

| Page | Summary |
|---|---|
| [[questions]] | **The work-list.** Open questions and design forks (Q-01, Q-15, Q-16, Q-21, Q-22, Q-25 closed) — what is known, what is missing, what the answer unblocks. Read first. |
| [[overview]] | Synthesis: what Kubona is, what is decided, and what the prototype has actually answered. |
| [[roadmap]] | Phases 0–6 read as **acceptance criteria**, not progress: gates, dependencies, team loading. |
| [[implementation-gaps]] | Prototype gap register: what the scaffold does vs. what production requires. Code facts only. |
| [[log]] | Append-only chronological changelog. |

## Status legend (frontmatter `status:`)

Describes **how decided** something is — never how finished.

| Value | Meaning | Used by |
|---|---|---|
| `settled` | A decision or standing policy. Treat it as a constraint; changing it needs a new decision and a log entry. | [[integration-topology-and-licence-discipline]] |
| `adopted` | Understood and relied upon; we would build this way today. Weaker than `settled` — the cost may not be known yet. | core mechanism concepts |
| `open-question` | The insight is incomplete or the fork is undecided. Indexed in [[questions]]. | philosophy pages, autonomy model, discovery layer |
| `candidate` | Proposed for integration or reuse; integration topology not yet committed. | [[openbao]], [[univer]], [[openqasm]], [[model-optimizer]], own repos |
| `tracked` | External landscape (vendor, SDK, standard) observed for change. No position taken. | provider entities, [[x402]], [[aaif]], [[qiskit-mcp-servers]] |
| `reference` | Prior art or another project mined for *method* only. Never load-bearing supply. | [[qbraid]], [[strangeworks]], [[hindsight]], own WIP |
| `source` | An ingested source record. Immutable once written. | the 5 pages under `sources/` |
| `living` | Meta pages that never settle. | overview, roadmap, index, log, questions |

**Census (89 pages = 39 entities · 40 concepts · 5 sources · 5 meta):**
`settled` 1 · `adopted` 29 · `open-question` 12 · `candidate` 6 · `tracked` 26 ·
`reference` 5 · `source` 5 · `living` 5. One `settled` against 29 `adopted` is the honest
reading: the method is **understood**, not yet **chosen**. `settled` is meant to stay small —
if it grows much faster than `adopted`, check that constraints are being recorded as `settled`
correctly rather than decisions being rushed into it.

**Machine-checkable:** `python3 scripts/wiki-lint.py --strict` enforces this vocabulary, the
canonical `sources` keys, link and index integrity, and the no-`dev-docs`-anchors rule.
Run it after any batch edit; exit 0 as of 2026-09-27.

## Sources (5)

| Page | Source doc | Sources this touched |
|---|---|---|
| [[source-architecture-paper]] | *The Ultimate AI-Quantum Agentic Platform* — architectural design & integration strategy | MCP, QIR/MLIR, providers, mitigation, eval |
| [[source-gap-analysis-plan]] | *Gap Analysis, Architectural Expansion, and Implementation Plan* — companion plan | Everything; the operational blueprint (+ §13 self-evolution) |
| [[source-cloudflare-economy]] | *Cloudflare, x402, and the Agent Resource Economy* — podcast insights | [[agent-resource-economy]], [[x402]], startup patterns |
| [[source-frontier-repo-audit-2026]] | *Frontier repository audit* — 25 live repos read end-to-end (remote · HEAD · licence · path:line) | Integration classes, licences, prior-art primitives |
| [[source-agent-protocol-landscape-2026]] | *Agent Protocol Landscape & Standards Audit* — 2026 primary-source audit of MCP, A2A, AAIF, AP2, Qiskit MCP | [[protocol-strategy]], [[spend-mandates-and-delegation]], [[mcp-and-context-flooding]] |

## Entities (39)

| Page | What it is |
|---|---|
| [[kubona]] | The platform itself — scope, posture, naming |
| [[buzz]] | Block/Mainza open-source hive-mind workspace — identities, event log, workflow triggers (design reference) |
| [[hindsight]] | vectorize-io memory sidecar: banks, retain/recall/reflect, budgeted recall over MCP |
| [[paperclip]] | paperclipai: four-plane MCP access governance, low-trust presets, task watchdog |
| [[gods-eye-view]] | bilawalsidhu: per-layer policy modules with machine-checked import/package boundaries |
| [[chickensoup]] | Maintainer's own credibility-wavefunction + real-Qiskit VQE repo — own WIP, mined for method and honest-degradation habit |
| [[mainza-consciousness]] | Maintainer's own repo: testable memory lifecycle worth reimplementing, next to Dirichlet-as-quantum theatre; source of the PAT incident |
| [[openqasm]] | OpenQASM 3 ANTLR grammars — the parse contract for agent-authored circuits (grammar-only submodule) |
| [[openbao]] | MPL-2.0 secrets engine, run as a sidecar so the Apache-2.0 core stays unencumbered |
| [[univer]] | Apache-2.0 office engine: formula engine, action recorder, data validation for the console |
| [[model-optimizer]] | NVIDIA: schema-pinned recipe tree (`modelopt_recipes/`) — the pack discipline Kubona copies |
| [[unsloth]] | Local LLM run+train: fast LoRA fine-tuning for the GQE flywheel, personas, private lab models |
| [[qbraid]] | Closed commercial hardware-abstraction platform; design reference, not a dependency |
| [[strangeworks]] | Closed enterprise "unconventional computing" orchestrator |
| [[nvidia-cuda-q]] | GPU-accelerated simulator; open-source components usable as default backend |
| [[aws-braket]] | AWS's managed quantum cloud; access path to IonQ/QuEra + SV1/DM1 simulators; Program Sets |
| [[cloudflare]] | The agent-economy infrastructure vendor (Crawl Control, Monetization Gateway) |
| [[x402]] | Payment rail on HTTP 402; sets a price, agent pays, edge verifies; x402 Foundation |
| [[aaif]] | Agentic AI Foundation: Linux Foundation body stewarding MCP, goose, AGENTS.md |
| [[a2a]] | Agent2Agent protocol: Linux Foundation standard for opaque inter-agent delegation |
| [[ap2]] | Agent Payments Protocol: Google/industry open standard for signed spending mandates |
| [[qiskit-mcp-servers]] | IBM's official MCP server collection for Qiskit and IBM Quantum |
| [[qiskit]] | IBM's SDK; V2 primitives (Sampler/Estimator), transpilation to ISA, runtime |
| [[pennylane]] | Quantum ML / VQA framework; differentiable circuits, parameter-shift |
| [[microsoft-azure-quantum]] | Q# language + QDK + Resource Estimator; topo/Majorana target |
| [[google-cirq]] | Pulse-level, fine-grained gate control; Google hardware |
| [[dwave]] | Quantum annealing; Ocean SDK; QUBO/CQM; Leap cloud |
| [[ionq]] | Trapped-ion provider; direct API, EQC, ~99.99% 2Q fidelity |
| [[quantinuum]] | Trapped-ion, tket compiler, Helios; highest logical-qubit density |
| [[rigetti]] | Superconducting; pyQuil, Cepheus multi-chip |
| [[neutral-atom-quera-pasqal]] | QuEra/Pasqal/Infleqtion; analog Hamiltonian simulation, Pulser |
| [[xanadu-strawberry-fields]] | Photonic/CV quantum computing |
| [[mitiq]] | Open-source error-mitigation toolkit (ZNE, PEC, CDR) |
| [[qiskit-circuit-knitting]] | Circuit cutting for width/depth-limited hardware |
| [[karpathy-autoresearch]] | Reusable LLM self-improvement loop; Kubona's object-level engine + §13 meta-tier |
| [[qflow-studio]] | Credential-safe quantum evidence-sharing UI precedent |
| [[ibm-quantum-composer]] | Reference circuit-editor UI; bidirectional code↔diagram sync |
| [[arxiv-mcp-quantum]] | Prior-art paper proving MCP-for-quantum is a real, non-dependency field |
| [[mainza-milimo-foundation]] | Existing codebase (Milimo Quantum, Finance Claw, AfrEval) cited as reusable de-risking |

## Concepts (40)

| Page | What it is |
|---|---|
| [[protocol-strategy]] | Decoupling the Capability Core from peripheral edge adapters (MCP, A2A); code mode vs tool calls |
| [[spend-mandates-and-delegation]] | AP2-style signed mandates for QPU spend authorization and A2A inter-agent delegation |
| [[mcp-and-context-flooding]] | Model Context Protocol; Tools/Resources/Prompts; code-execution mode |
| [[qir-mlir-compilation]] | Hardware-agnostic intermediate representation & LLVM-based pipeline |
| [[vqa-vqe]] | Variational algorithms; hybrid classical–quantum training loops |
| [[barren-plateau]] | Vanishing-gradient phenomenon; WBP detection; GQE alternative |
| [[error-mitigation]] | ZNE / PEC / CDR; Mitiq's toolkit |
| [[circuit-cutting]] | Partitioning large circuits; ILP cut placement; overhead math |
| [[nisq-and-hybrid-orchestration]] | Noisy intermediate-scale reality; Qiskit Serverless / Pilot-Quantum |
| [[cost-governance]] | Pre-flight, budget ceilings, approval gate, burn-rate telemetry |
| [[provenance-and-reproducibility]] | Content-addressed experiment ledger; queryable by agents |
| [[security-and-pqc]] | OAuth 2.1, ML-KEM transport, per-call auth, audit logging |
| [[provider-capability-schema]] | Data-driven provider onboarding; capability/json contract |
| [[quantum-cloud-pricing]] | Provider pricing models + financial telemetry (per-second, per-shot, AQT, Program Sets) |
| [[agent-resource-economy]] | Attention→resources reframe; 5-layer stack; startup patterns |
| [[agent-ux-control-plane]] | Human oversight: circuit viz, loop inspector, approval console, experiment notebook, labs/workflows view (→ [[ui-ux-vision]] for the full spec) |
| [[evaluation-and-benchmarks]] | Code-gen evals + hardware canary CI + agent-behavior evals |
| [[meta-search-and-self-evolution]] | The §13 second tier above the autoresearch loop: revises *how* Kubona searches; GQE flywheel, stricter safety gate |
| [[science-discovery]] | Kubona as an AI-enhanced science discovery platform: the 7-stage discovery loop, self-experimentation, systematic results |
| [[user-identity-and-profiles]] | Every actor (human or agent) is a keypair identity: profiles, labs, delegable+revocable authority |
| [[workflows-and-agent-authoring]] | User- and agent-created workflows & agents; triggers, gates, registry, safety rail |
| [[science-data-integrations]] | Literature & domain data (arXiv, OpenAlex, MPDS, …) as governed MCP resources |
| [[ui-ux-vision]] | The full console/UX specification: personas, 8 surfaces, visual language, mobile, acceptance criteria |
| [[design-evolution]] | Comparison: what the paper got right vs. what the plan expanded |
| [[risks]] | Risk register with mitigations and sequencing decisions |
| [[quantum-advantage-realism]] | The anti-quantum-washing policy: never claim advantage; backend, classical anchor and interval travel with every artefact |
| [[five-ring-research-loop]] | The outer discovery loop as five rings (Observe → Hypothesise → Compute → Audit → Communicate) plus Ring 0's watchdog discipline |
| [[claim-credibility-wavefunctions]] | Claims as amplitude vectors: credibility scored from source diversity + engagement + priors, collapsed by verification, audited by a tribunal |
| [[epistemic-curvature]] | Curvature of the belief manifold as the signal for where research effort is actually informative |
| [[fidelity-accounting]] | Every result carries a noise/fidelity budget; classical anchors and simulators before any real-QPU spend |
| [[autonomous-ansatz-optimization]] | Ansatz genomes searched as data (catalogs, MEW scores, noise-aware selection) by an `ansatz-scout` role |
| [[differentiable-quantum-stack]] | PennyLane as the differentiable engine: workflow, noise, circuit cutting, resource specs — wrapped, never forked |
| [[agentic-qir-compilation]] | QIR/MLIR compilation driven by agents: capability-aware lowering, resource passes, cost from the same compiler |
| [[quantum-domain-packs]] | Domain science as versioned, schema-pinned packs (recipes + evidence requirements + classical anchors) |
| [[multi-agent-research-collective]] | Roles, signed events, trust ladders and watchdogs: how many agents share one research ledger safely |
| [[agentic-memory-and-observability]] | Memory lifecycle (type, decay, consolidation) and the observability that makes agent claims checkable |
| [[confidentiality-lattice-provenance]] | Labels that flow with data: information-flow control as part of the provenance record |
| [[secrets-and-workload-isolation]] | Vault sidecar, scoped tokens, sandboxed workloads — and the credential incidents that taught us why |
| [[integration-topology-and-licence-discipline]] | Submodule / sidecar / blueprint / reference + licence rules; submodule is a shipping decision, not a compliment |