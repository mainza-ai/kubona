---
type: entity
name: qBraid
date: 2026-08-16
tags: [platform, competitor, commercial]
sources: [source-gap-analysis-plan]
status: tracked
---

# qBraid

Closed/commercial hardware-abstraction platform — the dominant **prior-art reference**
for Kubona's provider-abstraction layer (source-gap-analysis-plan).

## What it actually is (2026 snapshot)

- Hardware-agnostic runtime with cross-framework transpilation across **18+ quantum SDKs**
- Centralized access to **34+ devices** (IonQ, Rigetti, IQM, QuEra, Pasqal, Quantinuum, AWS, Azure)
- CUDA-Q remote-target integration, on-demand GPU fleet
- Lab environment (Jupyter/VS Code) — human/notebook-first

## Relationship to Kubona

- **Not a dependency.** Kubona is postured as genuinely open source; its own provider
  layer must be built and shipped in the open (~the way [[qiskit]], [[pennylane]], and
  [[mitiq]] are), per [[overview]].
- Optional interop connector for users who already hold a qBraid account.
- Use as a design reference for what a mature hardware-abstraction API surface looks like.

## Signal

qBraid's breadth (34+ devices) is a head start Kubona deliberately does not try to match
day-one; it differentiates on the agentic governance layer the commercial platforms haven't
built. Related: [[strangeworks]], [[provider-capability-schema]].