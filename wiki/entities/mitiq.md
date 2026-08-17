---
type: entity
name: Mitiq
date: 2026-08-16
tags: [tool, error-mitigation, open-source]
sources: [source-architecture-paper, source-gap-analysis-plan]
status: tracked
---

# Mitiq

Open-source (Apache-2.0) Python toolkit for **quantum error mitigation**, usable as a
compiler-level wrapper on any backend (source-architecture-paper). Functions as a loaded
tool for the agent's post-processing step.

## Techniques (see [[error-mitigation]] for the mechanics)

- **ZNE** — zero-noise extrapolation (unitary folding + extrapolation to zero noise). Highest
  value-to-effort ratio → **first mitigation shipped in Phase 1**.
- **PEC** — probabilistic error cancellation (quasi-probability sampling; exponential shot
  overhead).
- **CDR** — Clifford data regression (train a mapping from noisy to simulated outputs).

## Kubona notes

Wrong-choice ZNE/PEC/CDR config per backend = wasted spend + fidelity loss. That's why the
mitigation strand of the [[karpathy-autoresearch]] harness searches the config space
(mutable artifact = mitigation config; metric = distance from noiseless reference).

Related: [[qiskit-circuit-knitting]] · [[error-mitigation]].