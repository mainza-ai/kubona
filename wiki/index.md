---
type: catalog
date: 2026-08-16
tags: [index]
status: living
---

# Index

Catalog of every page in the wiki. Updated on every ingest. When answering a query,
read this first to find relevant pages, then drill in.

## Overview & meta

| Page | Summary |
|---|---|
| [[implementation-gaps]] | Production gap register: what the platform has vs. needs (M0 closures / open structural gaps). |
| [[overview]] | Synthesis: what Kubona is and where it stands. |
| [[roadmap]] | Phased implementation plan (0–5): gates, dependencies, team loading, tracking. |
| [[log]] | Append-only chronological changelog. |

## Sources (3)

| Page | Source doc | Sources this touched |
|---|---|---|
| [[source-architecture-paper]] | *The Ultimate AI-Quantum Agentic Platform* — architectural design & integration strategy | MCP, QIR/MLIR, providers, mitigation, eval |
| [[source-gap-analysis-plan]] | *Gap Analysis, Architectural Expansion, and Implementation Plan* — companion plan | Everything; the operational blueprint (+ §13 self-evolution) |
| [[source-cloudflare-economy]] | *Cloudflare, x402, and the Agent Resource Economy* — podcast insights | [[agent-resource-economy]], [[x402]], startup patterns |

## Entities (26)

| Page | What it is |
|---|---|
| [[kubona]] | The platform itself — scope, posture, naming |
| [[buzz]] | Block/Mainza open-source hive-mind workspace — identities, event log, workflow triggers (design reference) |
| [[unsloth]] | Local LLM run+train: fast LoRA fine-tuning for the GQE flywheel, personas, private lab models |
| [[qbraid]] | Closed commercial hardware-abstraction platform; design reference, not a dependency |
| [[strangeworks]] | Closed enterprise "unconventional computing" orchestrator |
| [[nvidia-cuda-q]] | GPU-accelerated simulator; open-source components usable as default backend |
| [[aws-braket]] | AWS's managed quantum cloud; access path to IonQ/QuEra + SV1/DM1 simulators; Program Sets |
| [[cloudflare]] | The agent-economy infrastructure vendor (Crawl Control, Monetization Gateway) |
| [[x402]] | Payment rail on HTTP 402; sets a price, agent pays, edge verifies |
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

## Concepts (23)

| Page | What it is |
|---|---|
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