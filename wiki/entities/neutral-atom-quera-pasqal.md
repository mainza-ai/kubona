---
type: entity
name: Neutral-Atom Providers (QuEra / Pasqal / Infleqtion)
date: 2026-08-16
updated: 2026-09-27
tags: [provider, neutral-atom]
sources: [source-gap-analysis-plan]
status: tracked
---

# QuEra · Pasqal · Infleqtion (Neutral Atoms)

A distinct quantum paradigm the architecture paper reduced to a pricing footnote
(QuEra inside a [[aws-braket]] row). The gap analysis promotes it to a dedicated toolchain
(source-gap-analysis-plan).

## Why it's architecturally distinct

- **Analog Hamiltonian Simulation (AHS)** and multi-body physics map directly onto 2D
  Rydberg arrays — a different programming model than gate circuits (QuEra's *Aquila* via [[aws-braket]]).
- **Pasqal's Pulser SDK** is the native toolchain for neutral-atom AHS design.
- Requires a dedicated agent toolchain, not a pricing-table footnote.

## Kubona notes

Phase 2 direct integration via Pulser. Onboarded as machine-readable capability data
(`modality: neutral_atom`) via [[provider-capability-schema]].

Related: [[qbraid]], [[rigetti]], [[quantinuum]].