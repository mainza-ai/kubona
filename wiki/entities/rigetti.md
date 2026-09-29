---
type: entity
name: Rigetti
date: 2026-08-16
updated: 2026-09-27
tags: [provider, superconducting]
sources: [source-gap-analysis-plan, source-architecture-paper]
status: tracked
---

# Rigetti

Superconducting provider. Mentioned once in the architecture paper as a QIR target; the gap
analysis makes it a real integration via **pyQuil** and its **Cepheus multi-chip**
architecture (source-gap-analysis-plan).

## Kubona notes

- Direct pyQuil SDK integration (Phase 2) — not just "a QIR target."
- Also reachable via [[microsoft-azure-quantum]] (provider-agnostic entry).
- Onboarded via [[provider-capability-schema]].

## Related

[[quantinuum]] · [[qiskit]] · [[qir-mlir-compilation]] · [[kubona]]