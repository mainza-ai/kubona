---
type: entity
name: Xanadu (Strawberry Fields)
date: 2026-08-16
updated: 2026-09-27
tags: [provider, photonic, cv]
sources: [source-gap-analysis-plan]
status: tracked
---

# Xanadu / Strawberry Fields (Photonic · Continuous-Variable)

Photonic quantum computing — **continuously variable (CV)**, no discrete qubit model.
Not mentioned at all in the architecture paper; flagged in the gap analysis as relevant
for **Gaussian boson sampling** and ML-adjacent workloads (source-gap-analysis-plan).

## Kubona notes

Phase 5 scope: Strawberry Fields integration after [[qir-mlir-compilation]] and core
providers are stable (see [[roadmap]]). The provider capability schema must represent the
CV modality explicitly — it's not a gate-architecture variant.

Related: [[pennylane]] (Xanadu is PennyLane's vendor).