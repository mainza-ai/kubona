---
type: concept
name: VQAs & VQE
date: 2026-08-16
tags: [algorithms, variational]
sources: [source-architecture-paper, source-gap-analysis-plan]
status: living
---

# Variational Quantum Algorithms (VQAs) & VQE

The paradigmatic NISQ-era algorithm family: a parameterized quantum circuit (ansatz)
optimized by a classical optimizer against a measured cost function — quantum as an
accelerator inside a [[nisq-and-hybrid-orchestration|classical loop]]
(source-architecture-paper).

## Why Kubona is built around the loop

- VQE (variational quantum eigensolver) is the canonical multi-step hybrid job: prepare
  circuit → dispatch → receive measurement statistics → update variational parameters →
  repeat.
- These loops can span **hours and many agent invocations** — the reason for the `job_id`
  session abstraction and the live [[agent-ux-control-plane|loop inspector]].
- The main algorithmic hazard is the [[barren-plateau]] phenomenon.

## The frameworks

- [[qiskit]] V2 primitives (EstimatorV2) for hardware submission.
- [[pennylane]] gives differentiable circuits and `parameter-shift` gradients for
  optimization inside neural networks — the QML path.

Related: [[barren-plateau]] · [[nisq-and-hybrid-orchestration]] · [[cost-governance]] ·
[[karpathy-autoresearch]] (VQE ansatz harness)