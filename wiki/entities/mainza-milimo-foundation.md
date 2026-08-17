---
type: entity
name: Mainza / Milimo Foundation (existing codebase)
date: 2026-08-16
tags: [foundation, prior-work, reuse]
sources: [source-gap-analysis-plan]
status: cross-project-reference
---

# Mainza / Milimo Foundation

Cross-project references in the gap-analysis source to **existing, already-built systems**
by the same operator ("Mainza"). Not Kubona code, but directly de-risking for it.

## What exists (per the source)

- **Milimo Quantum** — a foundation with **Qiskit v2.2, 14+ agents, VQE, and an Apple
  Silicon / CUDA HAL already built** (source-gap-analysis-plan §8). The source claims this
  makes **Phases 0–1 of Kubona substantially de-risked**, with the HAL and agent-mesh
  patterns directly reusable for the MCP execution server and cost-governance gate.
- **MilimoClaw Finance Claw** — a **REVIEW→HOLD two-stage approval pattern** already
  validated in a finance domain; the source says it maps *almost exactly* onto Kubona's
  quantum-spend approval gate (§4.5).
- **AfrEval Context Score** — a freshness/trust scoring primitive "already proven out" in
  Mainza's AfrEval project; reused as the per-provider and per-library-entry
  freshness/trust score in the provenance browser (§12.6) — a "certify before you trust it"
  vocabulary shared across both platforms.

## How to treat these claims

> ⚑ Single-source origin, now repo-verified at the surface level (2026-08-16): the referenced
> repositories are public and confirm the headline claims — `mainza-ai/milimoquantum`
> (README: "Qiskit 2.x", 17 specialized agents, VQE-based drug discovery MQDD, an
> `autoresearch-mlx` module on Apple-Silicon MLX, IBM/D-Wave/Braket targets),
> `mainza-ai/MilimoClaw` (README: the Stage 1 REVIEW → Stage 2 HOLD gate for agent-initiated
> spend — the claimed two-stage approval pattern), and `mainza-ai/afreval`
> (an `afreval-context-score` service — the Context Score primitive). Deeper code-level
> specifics (exact Qiskit version, HAL internals, agent-mesh API shapes, licenses) still
> need inspection before any pattern is lifted into Kubona.

## Relevance to Kubona

The reuse thesis: Kubona's [[cost-governance]] approval gate, execution-server
agent-mesh patterns, and provenance freshness scoring should be designed to lift from these
existing systems rather than reinvented. See [[roadmap]] Phases 0–1.

Related: [[kubona]] · [[cost-governance]] · [[provenance-and-reproducibility]] ·
[[karpathy-autoresearch]]