---
type: concept
name: QIR / MLIR Compilation
date: 2026-08-16
updated: 2026-11-10
tags: [compiler, qir, mlir]
sources: [source-architecture-paper, source-gap-analysis-plan]
status: adopted
---

# QIR / MLIR Compilation Pipeline

The hardware-agnostic compilation core that lets one algorithm run on any provider.

> **Register note (design vs. prototype):** Sections below describe the target compilation
> architecture from the architecture paper and gap-analysis plan. In the prototype at
> `2026-10-17`, `platform/crates/qir-core` was reported as parsing only an OpenQASM 2.0
> subset (`OPENQASM 3` refused); MLIR (`QUIR`/`Pulse` dialects), `qubit linearity`
> borrow-checking, WASM builds, and PyO3 bindings remain blueprint targets (see
> [[agentic-qir-compilation]] and [[deep-implementation-audit-2026-10-12]]).
>
> > ⚑ Update (2026-11-10, `119749e` / `de3428c`): `platform/crates/qir-core` (`23` Rust unit
> > tests; `32` total across `qir-core` + `ifc-core`) now parses both **OpenQASM 2.0**
> > (`qreg`/`creg`) and an **OpenQASM 3.0 subset** (`OPENQASM 3.0;`, `qubit[...]`,
> > `bit[...]`, `reset`; tested in `parses_openqasm3_subset`), decomposes compound gates
> > (`ccx`/`toffoli`) while refusing unsupported control flow (`opaque`/`if`), and emits QIR
> > `BaseProfile` LLVM text IR (`--llvm`) or JSON (`--json`). `Dockerfile.kubonad` builds and
> > bundles `/usr/local/bin/qir-core` (`KUBONA_QIR_CORE_BIN`); the Python package keeps
> > `kubona/qir/pipeline.py` explicitly labelled as `python-reference` (never compiler-verified)
> > when the Rust binary is absent. Multi-dialect MLIR lowering remains formally rejected
> > (`Q-43`).

## Quantum Intermediate Representation (QIR)

- Maintained by the **QIR Alliance**; based on classical **LLVM IR**, extended with opaque
  quantum types (Qubit, Result) and specialized runtime library declarations
  (source-architecture-paper).
- Unifies classical + quantum code in one intermediate framework → standard LLVM passes
  (constant folding, loop unrolling, dead-code elimination) apply to both.

## MLIR progressive lowering (blueprint target)

OpenQASM 3.0 → specialized MLIR dialects (**QUIR** for intermediate representation, **Pulse**
for microwave control). Agents can inspect use-define chains to see how quantum data flows.

## Pipeline (blueprint target)

Front-end (Rust) parses AST (OpenQASM 3.0 / Q# / Qiskit output) → QIR bitcode → MLIR
dialects → per-provider deploy (Quantinuum H-Series, Rigetti Cepheus, Braket, etc.) without
manual refactoring.

## Stack justification

Rust core: memory safety, `qubit linearity` compile-time enforcement (target), ~100× faster than the
legacy (pre-Rust) QDK engine, **WASM** compilation for in-browser/edge simulation (target),
**PyO3** bindings for Python agent code (target; invoked via CLI/Go bridge in prototype).
Ecosystem anchors: **qe-compiler** (OpenQASM's MLIR-based quantum compiler for quantum
engines) and QIR support in the Munich Quantum Toolkit (MQT) are the pipeline's reference
points for progressive lowering into QUIR/Pulse dialects.

Related: [[qiskit]] · [[microsoft-azure-quantum]] · [[google-cirq]] ·
[[evaluation-and-benchmarks]] (correctness-regression harness gates this pipeline).