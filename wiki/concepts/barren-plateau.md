---
type: concept
name: Barren Plateaus
date: 2026-08-16
tags: [algorithms, optimization, problem]
sources: [source-architecture-paper, source-gap-analysis-plan]
status: living
---

# Barren Plateaus & the Alternatives

For heavily parameterized circuits, cost gradients vanish **exponentially** in qubit count
and depth — classical optimizers stall and the neural network becomes untrainable
(source-architecture-paper).

## Two agent-relevant mitigations

1. **GQE — Generative Quantum Eigensolver.** Treat distinct quantum unitary operations as a
   *vocabulary*; a transformer pre-trained on optimized circuits predicts a gate sequence
   approaching the ground state — replacing quantum gradient evaluation with LLM sampling +
   classical backpropagation. Avoids barren plateaus entirely. In Kubona this is a 4th
   harness of the [[karpathy-autoresearch]] engine, not a separate system.
2. **Weak Barren Plateau (WBP) detection.** At initialization, use **classical shadow
   estimation** to track the second **Rényi entropy** of localized subsystems; when
   entanglement exceeds a critical threshold, the agent auto-restarts with adjusted
   learning rates / initial parameters.

## Kubona requirement

Agent-behavior evals must test the agent's *response* to a detected WBP (does it restart,
adjust, or loop forever?) — not just code-gen correctness. This is a route to wasted
spend if mishandled (see [[cost-governance]]).

Related: [[vqa-vqe]] · [[error-mitigation]] · [[karpathy-autoresearch]] ·
[[evaluation-and-benchmarks]]