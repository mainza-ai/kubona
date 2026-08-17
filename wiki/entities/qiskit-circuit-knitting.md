---
type: entity
name: Qiskit Circuit Knitting Toolbox
date: 2026-08-16
tags: [tool, circuit-cutting, open-source]
sources: [source-architecture-paper, source-gap-analysis-plan]
status: tracked
---

# Qiskit Circuit Knitting Toolbox

Open-source (Apache-2.0) toolkit for **circuit cutting**: partition a circuit too wide or
too deep for available QPUs into smaller subcircuits, execute those, and classically
reconstruct expectation values (source-architecture-paper). See [[circuit-cutting]] for
the mechanism and overhead math.

## Kubona notes

- Phase 2 scope (circuit-cutting for width/depth-constrained circuits).
- Cut placement uses Integer Linear Programming to optimize noise-vs-time under budget —
  feeds directly into [[cost-governance]] and the [[karpathy-autoresearch]] mitigation harness.

Related: [[mitiq]] · [[error-mitigation]] · [[nisq-and-hybrid-orchestration]]