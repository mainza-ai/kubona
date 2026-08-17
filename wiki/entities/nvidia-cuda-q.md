---
type: entity
name: NVIDIA CUDA-Q / cuQuantum
date: 2026-08-16
tags: [tool, simulator, gpu]
sources: [source-gap-analysis-plan]
status: tracked
---

# NVIDIA CUDA-Q / cuQuantum

GPU-accelerated statevector/tensor-network simulation plus a unified kernel language
(`@cudaq.kernel`) with multi-vendor QPU targets (source-gap-analysis-plan).

## Role in Kubona

- The **GPU-simulation components are open source (Apache-2.0)** and can be depended on
  directly as the default simulator backend without compromising Kubona's open-source
  posture — consistent with the "open-source components only" hardware-abstraction rule.
- `cuQuantum` (SDK) usable for accelerating the simulator tier.

## Related

[[qbraid]] exposes CUDA-Q as a remote target (interop reference).
Provider-capability model: see [[provider-capability-schema]].