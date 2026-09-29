---
type: concept
name: Circuit Cutting
date: 2026-08-16
updated: 2026-09-27
tags: [circuit-cutting, distributed, scaling]
sources: [source-architecture-paper, source-gap-analysis-plan]
status: adopted
---

# Circuit Cutting (Circuit Knitting)

Decomposing a circuit too wide/deep for any available QPU into smaller independent
subcircuits, executing those on high-fidelity hardware, and reconstructing expectation
values classically via quasi-probability decomposition (source-architecture-paper).

## Overhead math

Each severed CNOT introduces an **exponential measurement overhead** (the source's formula
images: a superpolynomial shot-cost multiplier without classical communication, reducible
with LOCC — local operations and classical communication). This is why cut placement
matters.

## Optimizing the cut

**Integer Linear Programming (ILP)** selects cut placement to maximize noise fidelity while
keeping execution time inside the cloud budget — an optimization that interacts directly
with [[cost-governance]] and the [[karpathy-autoresearch]] harnesses.

## Kubona notes

- Implemented via the [[qiskit-circuit-knitting|Qiskit Circuit Knitting Toolbox]] (Phase 2).
- Complements [[error-mitigation]] as a second strategy for width/depth-limited runs,
  alongside [[nisq-and-hybrid-orchestration|distributed orchestration]].

Related: [[qiskit-circuit-knitting]] · [[error-mitigation]] · [[nisq-and-hybrid-orchestration]]