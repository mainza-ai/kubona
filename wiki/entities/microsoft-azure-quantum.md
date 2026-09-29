---
type: entity
name: Microsoft Azure Quantum / Q#
date: 2026-08-16
updated: 2026-09-27
tags: [sdk, microsoft, language]
sources: [source-architecture-paper, source-gap-analysis-plan]
status: tracked
---

# Microsoft Azure Quantum & Q#

Q# is a functional, domain-specific quantum language (gates/reversible semantics are
first-class); the Q# Development Kit (QDK) manages multi-file projects via
`qsharp.json` manifests (source-architecture-paper). Azure Quantum is the provider
surface; Microsoft's in-house hardware direction is topological/Majorana-1.

## Distinctives exposed to agents

- **Azure Quantum Resource Estimator:** before real execution, compute required logical
  qubits, code distance, and **rQOPS** (reliable Quantum Operations Per Second) — pre-flight
  telemetry so agents don't dispatch algorithms beyond current coherence limits.
- Hardware-agnostic provider entry (Rigetti via Azure, etc.) with **AQT token** pricing.

## Kubona notes

- Q# language integration is via the [[qir-mlir-compilation]] path.
- Majorana-1 is a distinct routing option with an early-stage, unverified-error-correction
  risk profile — surfaced in the [[provider-capability-schema]] as such.

## Related

[[qir-mlir-compilation]] · [[qiskit]] · [[provider-capability-schema]]