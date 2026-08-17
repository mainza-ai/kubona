---
type: concept
name: NISQ & Hybrid Orchestration
date: 2026-08-16
tags: [nisq, orchestration, hybrid]
sources: [source-architecture-paper, source-gap-analysis-plan]
status: living
---

# NISQ Reality & Hybrid Quantum–Classical Orchestration

In the Noisy Intermediate-Scale Quantum (NISQ) era a QPU is not a standalone machine — it is
a specialized accelerator inside a **classical loop** (prepare circuit → dispatch → measure →
update parameters) (source-architecture-paper). Think: [[vqa-vqe]].

## Orchestration middleware

To stop network latency killing iterative algorithms, the classical half of a hybrid job
runs in serverless containers placed **physically near the QPU**:

- **Qiskit Serverless** — spawn distributed tasks across CPUs/GPUs/QPUs; parallel molecular
  ansatz evaluation; turns the agent into a macro-scheduler.
- **Pilot-Quantum** — application-level management of hybrid environments; task parallelism
  for variational approaches and circuit-cutting workloads.

## Kubona notes

This is the layer that motivates the `job_id` session abstraction and the live loop
inspector in [[agent-ux-control-plane]] — a multi-hour VQE loop is the canonical workload.
Providers are onboarded with queue-depth and modality data via
[[provider-capability-schema]].

Related: [[vqa-vqe]] · [[circuit-cutting]] · [[cost-governance]] · [[mcp-and-context-flooding]]