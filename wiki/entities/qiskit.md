---
type: entity
name: Qiskit
date: 2026-08-16
updated: 2026-09-27
tags: [sdk, ibm, framework]
sources: [source-architecture-paper, source-gap-analysis-plan]
status: tracked
---

# Qiskit

IBM's open-source quantum SDK — the primary gate-model integration target for Kubona
(source-architecture-paper). For AI agents its modern surface is the Qiskit **Runtime
REST API with V2 Primitives**, which superseded V1.

## V2 Primitives (ISA requirement)

- Circuits must be fully **transpiled to the target QPU's Instruction Set Architecture**
  before submission; hidden server-side compilation is disabled.
- **Sampler** — qiskit-ibm-runtime 0.50 uses the client-side `executor_sampler.Sampler`
  (legacy `SamplerV2` remains an interface); it takes an ISA circuit + optional parameter
  sets and returns per-shot measurement bitstrings.
- **EstimatorV2** — takes an ISA circuit, observables (e.g. Pauli strings), parameter sets,
  and a target precision.
- Agent-facing unit: **Primitive Unified Blocs (PUBs)** — the parameterized submission unit.

## Live IBM documentation

The authoritative, changing IBM facts (backends, limits, prices, channels) live in IBM's
docs, not here. Start at [[ibm-quantum-platform]] for the curated link hub and the current
Kubona provider snapshot.

## Role in Kubona

- First provider integration in Phase 0 (with the local/GPU simulator).
- **Sessions** (Qiskit Serverless, via [[nisq-and-hybrid-orchestration]]) for distributed
  hybrid jobs; per-second Runtime pricing metered via Sessions — see
  [[quantum-cloud-pricing]] for the routing logic (incl. off-peak scheduling when IBM queue
  times are high).
- Qiskit HumanEval / QuantumKatas eval corpora — see [[evaluation-and-benchmarks]].

## Related

[[qir-mlir-compilation]] (OpenQASM 3.0 → QIR) · [[ibm-quantum-platform]] · [[microsoft-azure-quantum]] ·
[[error-mitigation]] · [[qiskit-circuit-knitting]] · [[vqa-vqe]]