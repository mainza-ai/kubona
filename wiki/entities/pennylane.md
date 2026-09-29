---
type: entity
name: PennyLane
date: 2026-08-16
updated: 2026-09-27
tags: [sdk, qml, framework]
sources: [source-architecture-paper, source-gap-analysis-plan]
status: tracked
---

# PennyLane

Open-source framework for **quantum machine learning / variational algorithms**
(source-architecture-paper): treats quantum operations as differentiable layers inside
classical neural networks (PyTorch/JAX), computing gradients via the **parameter-shift rule**.

## Role in Kubona

- Powers the [[vqa-vqe]] / hybrid-optimization path of the agent's cognitive loops.
- **PennyLang** — an eval corpus of 3,000+ curated quantum code samples; RAG-enhanced
  generation lifts QML code-gen success from single digits to ~84%
  (source-architecture-paper) — see [[evaluation-and-benchmarks]].
- Second provider/DSL integration alongside [[qiskit]] in Phase 0.

## Related

[[qiskit]] · [[vqa-vqe]] · [[barren-plateau]] · [[nisq-and-hybrid-orchestration]] ·
[[evaluation-and-benchmarks]]