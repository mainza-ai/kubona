---
type: concept
tags: [mission-control, multimodal, local-llm, cloud-escalation, custom-agents, routing]
date: 2026-10-07
updated: 2026-10-07
status: adopted
sources:
  - source-architecture-paper
  - source-frontier-repo-audit-2026
---

# Multimodal Mission Control & Custom Agents

The central nervous system of Kubona, defining how Large Language Models (LLMs) and Vision-Language Models (VLMs) drive scientific exploration, synthesize diverse physical modalities, enforce sovereign air-gap isolation, and coordinate both built-in system agents and user-defined custom personas.

## 1. Architectural Philosophy: LLMs as Mission Control

In Kubona, LLMs are not passive chat assistants or simple text wrappers; they function as **Mission Control**:
- **Continuous Executive Orchestration:** Directing multi-hour exploratory campaigns, setting boundary conditions, decomposing mathematical conjectures into Lean 4 formalization cavity goals, dispatching classical simulation anchors (PySCF FCI), compiling quantum circuits (QIR bitcode), and synthesizing publication-grade manuscripts.
- **Multimodal Unification:** Bridging disparate scientific representations into a coherent epistemic graph:
  1. *Visual & Structural:* 2D chemical reaction diagrams, 3D molecular conformations (PDB/CIF/SDF), crystal lattices, NMR/UV-Vis spectroscopic curves, band structure diagrams, and whiteboard sketches.
  2. *Symbolic & Formal:* Lean 4 ASTs and goal states, QIR bitcode, OpenQASM 3.0 circuits, Jordan-Wigner Pauli strings, and Git-compatible unified diffs.
  3. *Temporal & Auditory:* Real-time QPU telemetry, coherence decay curves, microwave pulse waveforms, and hands-free laboratory voice instructions.

## 2. Local-First Default & Sovereign Air-Gap Preservation

To safeguard proprietary intellectual property, novel molecular formulations, and unannounced quantum circuit ansatzes, Kubona operates **local-first by default**:
- **Supported Local Runtimes:** Apple Silicon Unified Memory via MLX/oMLX (Port 8787/8080), Ollama (Port 11434), LM Studio (Port 1234), and vLLM (Port 8000).
- **Physical Air-Gap Guarantee:** When `air_gapped: true`, the system strictly forbids outbound internet communication for model inference (0 bytes egress). Any attempt by an agent or user to route queries to cloud frontiers is intercepted by `HybridModelRouter` and redirected to the strongest local multimodal model (e.g. `Qwen2.5-VL-72B`, `DeepSeek-R1-Distill-Qwen-32B`, or `Llama-3.3-70B-Instruct`).
- **In-Platform Model Acquisition:** Through [[hugging-face-hub]], users search and download GGUF/MLX quantized checkpoints directly from Hugging Face into `~/.kubona/models`, keeping the entire model lifecycle within private infrastructure.

## 3. Cryptographically Gated Cloud Escalation

When extreme reasoning horizons (~3 hours per cavity proof) or massive multi-million-token literature corpuses exceed local hardware limits, Kubona permits **cloud escalation**:
- **Frontier Endpoints:** Anthropic Claude 3.7 Sonnet (hybrid thinking compute), OpenAI o-series/o3 (deep mathematical cavity search), and Google Gemini 2.5 Pro (million-token context windows).
- **AP2 Cryptographic Mandate:** Cloud calls cannot be triggered automatically without explicit human authorization. The principal must sign an Ed25519 cryptographic spend envelope defining:
  - Strict dollar spend cap (`spend_budget_usd`).
  - Max token ceiling per generation.
  - Whitelist of approved domain objectives.

## 4. Agent Ecosystem: Standard System Roles & User-Defined Personas

Kubona balances hardened, formal system agents with a declarative user-defined agent extension framework:

### Standard System Agents
1. `lead_theorist`: High-level conjecture formation, literature synthesis, and campaign planning.
2. `lean4_proof_engineer`: Formal proof synthesis in Lean 4 under strict axiom containment (`comparator` protocol).
3. `classical_anchor_solver`: Exact numerical ground-truth calculation (PySCF FCI, DMRG, CC) to prevent quantum-washing.
4. `quantum_circuit_compiler`: Hardware-efficient ansatz design, Clifford+T synthesis, wire cutting, and ZNE mitigation.
5. `adversarial_falsification_critic`: Relentless red-teaming, counterexample search, and noise stress-testing.
6. `sentinel_spend_governor`: Enforcing spend caps, QPU shot allocations, and physical air-gap integrity.
7. `multimodal_spectroscopist`: Interpreting NMR/UV-Vis spectra, XRD diffraction patterns, and 3D molecular coordinates.

### User-Defined Custom Agents
Users can author specialized scientific personas via declarative manifests (`UserDefinedAgentManifest`):
- **Declarative Schema:** ID, name, role, avatar emoji, system instructions, capability tags, supported modalities, preferred model (local vs. cloud), MCP tool whitelists, spend ceilings, and scoped memory bank access.
- **Sandboxed Capability Gating:** User-defined agents can only invoke MCP tools explicitly declared in their manifest.
- **Dynamic Hot-Reloading:** Agents are saved to local persistent storage (`~/.kubona/custom_agents.json`), exposed via `/api/agents/custom`, and rendered dynamically in the Cognitive Mesh cockpit surface.

## Cross-References

- [[hugging-face-hub]] — model hub search, quantization inspection, and streaming downloader.
- [[agent-ux-control-plane]] — user interaction and flight control surfaces.
- [[claim-credibility-wavefunctions]] — claim amplitude vectors and verification standards.
- [[quantum-advantage-realism]] — analytical anchors and classical ground truth.
