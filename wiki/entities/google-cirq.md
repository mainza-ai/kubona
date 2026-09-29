---
type: entity
name: Google Cirq
date: 2026-08-16
updated: 2026-09-27
tags: [sdk, google, framework]
sources: [source-architecture-paper]
status: tracked
---

# Google Cirq

Google's SDK for **fine-grained control** over circuit construction — pulse-level
precision, custom noise models, specific gate scheduling, optimized for Sycamore-class
architectures (source-architecture-paper). Integrates with Google Cloud Compute for
large-scale tensor-network simulation.

## Role in Kubona

- The agent's toolchain of choice when a task needs: pulse sequences, custom noise
  modeling, or architecture-specific scheduling that [[qiskit]]/[[pennylane]] bake away.
- Routed via [[qir-mlir-compilation]] like every other DSL.

## Related

[[qiskit]] · [[nisq-and-hybrid-orchestration]] · [[error-mitigation]]