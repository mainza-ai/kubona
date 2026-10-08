---
type: concept
name: Agent UX & the Control Plane
date: 2026-08-16
updated: 2026-10-08
tags: [ux, control-plane, hitl]
sources: [source-gap-analysis-plan, source-openai-math-corpus]
status: adopted
---

# Agent UX & the Kubona Control Plane

Kubona's UI sits at the intersection of two disciplines that rarely meet
(source-gap-analysis-plan):

1. **Quantum circuit visualization** — a design space where even 2026 studies call
   platform-agnostic rendering an **open problem**, because visualization is tightly coupled
   to the originating SDK. Kubona inherits this by being multi-SDK from day one. Prior-art
   reference points: [[ibm-quantum-composer]] (bidirectional code↔diagram editor) and
   [[qflow-studio]] (credential-safe evidence sharing).
2. **Agent UX** — an accountability layer: surface *what the system is doing, why it chose an
   action, allow override at any point, recover gracefully from errors.*

No existing tool (quantum or agentic) covers both.

> **Full specification:** the complete UI/UX vision — personas, information architecture,
> per-surface detail, visual language, mobile, open design problems, acceptance criteria —
> lives in [[ui-ux-vision]].
>
> **Implementation stack:** The control plane is implemented in **TypeScript + React/Next.js**
> talking via WebSockets and gRPC-Web to a high-throughput **Go backend orchestrator**, while
> quantum execution runs via isolated **Python gRPC microservices**.

## The nine core observatory surfaces
 
| Surface | Purpose |
|---|---|
| **Spatial research canvas** | Infinite zoomable canvas (Galaxy to Surface); agent co-presence cursors (Buzz pattern); SDK-independent post-QIR universal glyphs & ghost circuits |
| **4D flight recorder** | Replay/scrub multi-hour trajectories; cavity expansion & tree shift replay ([[source-openai-math-corpus]]); speculative counterfactual branching |
| **Epistemic manifold & analytical anchors** | Tripartite credibility wavefunction rings (`{corroborated, contested, unverified}`); deductive certitude ($\rho = 1.0$); inline DMRG / exact analytical anchors |
| **Autonomy cockpit & spend cards** | Calibrated friction architecture; WebCrypto Ed25519 spend mandates (`{tenant_id, job_hash, max_budget_usd, expiry}`); one-glance action gate cards |
| **Execution fleet radar** | Real-time multi-modality fleet status (local simulator, ZNE, IBM, Quantinuum, QuEra, D-Wave); jurisdiction & export-control shield (`policy.py`) |
| **Cognitive LLM mesh** | Zero-config local host port radar (Ollama, OMLX, LM Studio, MLX); sovereign air-gap switch; tiered persona model assignment; arg normalization inspector |
| **Results studio & notebook** | Systematic citable reports with frame-anchored media comments (Buzz pattern); 100% provenance verification links; first-class negative results repository |
| **Epistemic tribunal room** | Dedicated adversarial deliberation chamber; real-time quorum consensus gauges, dissenting arguments, and formal Lean 4 AST citations |
| **Generative scientific UI (A2UI)** | Agent-streamed interactive components: 3D molecular active space (Mol*/Three.js), QIR circuit editor, Lean 4 goal tree, shadow tomography radar |

## Interaction principles

- **Progressive disclosure** — deep circuits default to summarized stats, drill-down on demand.
- **Confidence/uncertainty viz** — distinguish raw vs. error-mitigated values; show technique
  and its variance cost inline.
- **Status & activity** — the loop inspector always answers "what is the agent doing now,
  and why" in one glance.
- **Error handling & recovery** — failed jobs suggest an actionable next step (switch
  backend, request budget, fall back to simulator).

## Two hard rules

- **Read/approve only** — the console is oversight on top of agent actions, never a second
  path to submit jobs (avoids reopening the credential/spend risk).
- **Fixed, audited human surfaces** for approval/circuit/provenance — generative /
  agent-materialized UI (MCP Apps, A2UI) is a v2+ direction for ad-hoc visualizations only,
  given the security sensitivity.

Related: [[qflow-studio]] · [[cost-governance]] ·
[[provenance-and-reproducibility]] · [[qir-mlir-compilation]]