---
type: entity
name: IonQ
date: 2026-08-16
updated: 2026-09-27
tags: [provider, trapped-ion]
sources: [source-architecture-paper, source-gap-analysis-plan]
status: tracked
---

# IonQ

Trapped-ion hardware provider (Forte, Aria). In the architecture paper IonQ appears only
inside the [[aws-braket|AWS Braket]] pricing table; the gap-analysis upgrades it to a **first-class SDK
target** via its own direct API (source-gap-analysis-plan).

## Routing-relevant facts

- Runs a **direct API** and an **EQC (error-mitigated) execution mode**.
- Reports ~**99.99% two-qubit fidelity** (source-gap-analysis-plan) — e.g. best for high
  gate fidelity and all-to-all connectivity; high per-shot cost restricts naive sampling
  workloads (source-architecture-paper).

## Notes

The agent should know the EQC mode directly, not infer it from Braket pricing. Onboarded
as data via [[provider-capability-schema]]. Related: [[quantinuum]] (also trapped ion),
[[qbraid]].