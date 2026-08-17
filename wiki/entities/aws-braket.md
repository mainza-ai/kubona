---
type: entity
name: AWS Braket
date: 2026-08-16
tags: [provider-access, aws, cloud]
sources: [source-architecture-paper, source-gap-analysis-plan]
status: tracked
---

# AWS Braket

AWS's managed quantum cloud service — in the architecture paper one of the three cloud
backbones (with IBM Quantum and Azure Quantum) whose authentication, job-queuing, and API
mechanics the platform abstracts behind unified MCP resources (source-architecture-paper).
For Kubona it is the **access path** for providers without a direct integration.

## What it's used for

| Target | Notes |
|---|---|
| **IonQ (Forte, Aria)** | Flat per-task + per-shot; **minimum 2,500 shots for error mitigation**; per-shot cost restricts massive sampling (source-architecture-paper) |
| **QuEra (Aquila)** | Flat per-task + per-shot; exclusively for **analog Hamiltonian simulation** on 2D Rydberg arrays (source-architecture-paper) |
| **SV1 / DM1 simulators** | Managed statevector / density-matrix simulators — the simulator-first pre-flight tier |

## Pricing mechanics agents should know

- **Per-task + per-shot metering** — the flat per-task fee makes small job counts
  disproportionately expensive.
- **Program Sets** — batch multiple circuits into a single task, amortizing the flat fee;
  a big lever for highly iterative VQAs (source-architecture-paper).
- Feeds the routing logic in [[quantum-cloud-pricing]] and the pre-flight estimator in
  [[cost-governance]].

## Kubona notes

- Braket was the paper's only pipeline to IonQ and QuEra; the gap analysis upgrades IonQ to
  its own direct API (see [[ionq]]) and gives QuEra/Pasqal a Pulser-native toolchain with
  Braket as one execution surface ([[neutral-atom-quera-pasqal]]).
- [[qbraid]] also fronts AWS among its 34+ devices — interop reference only, never a
  dependency.

Related: [[quantum-cloud-pricing]] · [[cost-governance]] · [[neutral-atom-quera-pasqal]] ·
[[ionq]] · [[mcp-and-context-flooding]] · [[qbraid]]
