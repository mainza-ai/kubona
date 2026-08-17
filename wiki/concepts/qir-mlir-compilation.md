---
type: concept
name: QIR / MLIR Compilation
date: 2026-08-16
tags: [compiler, qir, mlir]
sources: [source-architecture-paper, source-gap-analysis-plan]
status: living
---

# QIR / MLIR Compilation Pipeline

The hardware-agnostic compilation core that lets one algorithm run on any provider.

## Quantum Intermediate Representation (QIR)

- Maintained by the **QIR Alliance**; based on classical **LLVM IR**, extended with opaque
  quantum types (Qubit, Result) and specialized runtime library declarations
  (source-architecture-paper).
- Unifies classical + quantum code in one intermediate framework → standard LLVM passes
  (constant folding, loop unrolling, dead-code elimination) apply to both.

## MLIR progressive lowering

OpenQASM 3.0 → specialized MLIR dialects (**QUIR** for intermediate representation, **Pulse**
for microwave control). Agents can inspect use-define chains to see how quantum data flows.

## Pipeline

Front-end (Rust) parses AST (OpenQASM 3.0 / Q# / Qiskit output) → QIR bitcode → MLIR
dialects → per-provider deploy (Quantinuum H-Series, Rigetti Cepheus, Braket, etc.) without
manual refactoring.

## Stack justification

Rust core: memory safety, `qubit linearity` compile-time enforcement, ~100× faster than the
legacy (pre-Rust) QDK engine, **WASM** compilation for in-browser/edge simulation,
**PyO3** bindings for Python agent code.
Ecosystem anchors: **qe-compiler** (OpenQASM's MLIR-based quantum compiler for quantum
engines) and QIR support in the Munich Quantum Toolkit (MQT) are the pipeline's reference
points for progressive lowering into QUIR/Pulse dialects.

Related: [[qiskit]] · [[microsoft-azure-quantum]] · [[google-cirq]] ·
[[evaluation-and-benchmarks]] (correctness-regression harness gates this pipeline).