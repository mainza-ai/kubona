---
type: entity
name: QFlow Studio
date: 2026-08-16
tags: [tool, ui, prior-art]
sources: [source-gap-analysis-plan]
status: tracked
---

# QFlow Studio

Agent-specific quantum UI precedent: lets users **design circuits visually**, keeps code and
diagram in sync, routes jobs through a pre-flight check, and **shares evidence of a run
without exposing credentials** (source-gap-analysis-plan).

## Why Kubona cares

The **credential-safe evidence-sharing** pattern maps directly onto Kubona's
[[provenance-and-reproducibility]] ledger and the *provenance/experiment browser* console
surface — adopt rather than reinvent. Contrast with [[qbraid]]'s human-first Lab and IBM's
Quantum Composer for the editor-sync pattern (see [[agent-ux-control-plane]]).

Related: [[agent-ux-control-plane]] · [[kubona]] · [[qiskit]]