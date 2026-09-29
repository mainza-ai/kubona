---
type: entity
name: D-Wave
date: 2026-08-16
updated: 2026-09-27
tags: [provider, annealing]
sources: [source-architecture-paper, source-gap-analysis-plan]
status: tracked
---

# D-Wave

Quantum **annealing** infrastructure — for combinatorial optimization, scheduling,
routing that maps badly to gate models (source-architecture-paper). Via the Ocean SDK,
agents formulate problems as QUBO or CQM, and submit through D-Wave Leap Cloud to hybrid
classical-quantum solvers (incl. higher-order HOBO and continuous variables).

## Role in Kubona

- Phase 5 scope: Ocean SDK deepening, per the original spec — see [[roadmap]].
- The annealing path is architecturally distinct from gate-based providers; the provider
  abstraction must carry the modality ([[provider-capability-schema]]).

## Related

[[microsoft-azure-quantum]] · [[google-cirq]] · [[provider-capability-schema]] ·
[[kubona]]