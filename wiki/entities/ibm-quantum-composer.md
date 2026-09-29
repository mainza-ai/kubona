---
type: entity
name: IBM Quantum Composer
date: 2026-08-16
updated: 2026-09-27
tags: [ui, prior-art, tool]
sources: [source-gap-analysis-plan]
status: tracked
---

# IBM Quantum Composer

The **current reference implementation** for the quantum-circuit-editing category
(source-gap-analysis-plan §5.2). It's what "good looks like" for the circuit-visualizer
surface Kubona plans.

## Patterns Kubona borrows

- **Drag-and-drop circuit editor stays in sync with generated code (OpenQASM/Qiskit)
  bidirectionally** — editing either view updates the other.
- **Live visualizations** (Q-sphere, statevector, measurement histograms) run against a fast
  local simulator — decoupled from (and much cheaper than) real QPU execution.
- **Job results pages** show the original circuit *and* the transpiled circuit side by side,
  plus a histogram, so the user sees what the compiler actually did to their intent.

## Why it matters to Kubona

Kubona's circuit visualizer inherits this pattern but must be **platform-agnostic** (post-QIR
lowering, SDK-independent) — see the open problem in [[agent-ux-control-plane]]. Related
prior art: [[qflow-studio]] for credential-safe evidence sharing.

Related: [[agent-ux-control-plane]] · [[qiskit]] · [[qflow-studio]]