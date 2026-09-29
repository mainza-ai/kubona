---
type: concept
name: Quantum Cloud Pricing & Financial Telemetry
date: 2026-08-16
updated: 2026-09-27
tags: [pricing, cost, cloud, telemetry]
sources: [source-architecture-paper, source-gap-analysis-plan]
status: adopted
---

# Quantum Cloud Pricing & Financial Telemetry

How QPU time is priced across providers, and how the agent navigates it as a
financial optimizer (source-architecture-paper). The governance side of this is
[[cost-governance]]; this page is the pricing-landscape side.

## Provider pricing models (2026, per the source)

| Provider | Model & systems | Pricing structure | Agent routing logic |
|---|---|---|---|
| **IBM Quantum** | Superconducting (Eagle, Heron) | Pay-as-you-go **per second** of Qiskit Runtime execution | Deep circuits / rapid iterative loops via Qiskit Serverless **Sessions**; high queue times → **off-peak scheduling** |
| **[[aws-braket|AWS Braket]] — IonQ** | Trapped ion (Forte, Aria) | **Flat per-task + per-shot**; **minimum 2,500 shots** for error mitigation | High gate fidelity + all-to-all connectivity; per-shot cost restricts massive sampling |
| **[[aws-braket|AWS Braket]] — QuEra** | Neutral atom (Aquila) | Flat per-task + per-shot | Exclusively for **analog Hamiltonian simulation** / multi-body physics on 2D Rydberg arrays |
| **Azure Quantum** | Hardware-agnostic (Rigetti, IonQ) | **AQT token consumption** model | Optimized Q# and [[qir-mlir-compilation|QIR]] pipelines |

## Pre-flight expense mitigation (the paper's heuristics)

- **Simulator-first**: initial iterations route to managed classical simulators — [[aws-braket|AWS Braket]]
  **SV1 (statevector), DM1 (density matrix)**.
- **Dynamic cost estimation**: the agent calculates expected cost from required shots × depth
  before submitting.
- **Program Sets** on [[aws-braket|Braket]]: batch multiple circuits into a single task, amortizing the flat
  per-task fee — big win for iterative VQAs.

## Kubona's upgrade over the paper

The paper describes routing heuristics in prose; Kubona makes them a hard system —
see [[cost-governance]] (mandatory pre-flight, ceilings, approval gate, burn-rate telemetry)
and the data-driven [[provider-capability-schema]] (pricing model as a schema field, not a
table row).

Related: [[cost-governance]] · [[provider-capability-schema]] · [[qiskit]] ·
[[ionq]] · [[neutral-atom-quera-pasqal]] · [[microsoft-azure-quantum]]