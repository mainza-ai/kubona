---
type: entity
name: Quantinuum
date: 2026-08-16
tags: [provider, trapped-ion]
sources: [source-gap-analysis-plan, source-architecture-paper]
status: tracked
---

# Quantinuum

Trapped-ion provider. Absent from the architecture paper except a QIR mention — elevated to
first-class in the gap analysis (source-gap-analysis-plan).

## Routing-relevant facts

- **tket** compiler (pytket) for circuit optimization/synthesis.
- **Helios** system: 48 error-corrected logical qubits at ~2:1 encoding — currently the
  **highest logical-qubit-density platform available**; exactly the kind of signal a
  routing decision needs.
- Accessible via QIR (Quantinuum Nexus) — see [[qir-mlir-compilation]].
- Per the provider schema example in the source: `modality: trapped_ion`, 98 physical qubits,
  native gates `[rzz, rz, rx]`, `jurisdiction: US/UK` (source-gap-analysis-plan).

## Kubona notes

Second-wave direct integration (Phase 2) via pytket. Related: [[ionq]], [[rigetti]],
[[qbraid]], [[provider-capability-schema]].