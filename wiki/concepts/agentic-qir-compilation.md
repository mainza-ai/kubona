---
name: Agentic QIR Compilation
type: concept
tags: [frontier, compiler, qir, mlir, qasm3, autoresearch]
date: 2026-09-27
updated: 2026-09-27
status: open-question
sources: [source-frontier-repo-audit-2026, source-architecture-paper]
---

# Agentic QIR Compilation

**Definition.** Treat the *compiler pipeline itself* as the mutable artifact of an
autoresearch harness: agents propose pass sequences, layout maps and tiling strategies; the
harness scores them on measured depth, two-qubit-gate count, feedforward latency and
predicted fidelity. Extends [[qir-mlir-compilation]] from "a pipeline that exists" to "a
pipeline that is searched".

## Reality check first (this changes the scope)

Upstream **MLIR contains no quantum dialect**. `grep -i quantum mlir/include/mlir/Dialect` is
empty, and `mlir/include/mlir/Dialect/Quant` is fixed-point **quantisation** for ML inference
— a name collision that will bite anyone who assumes otherwise (verified; (repo-audit-2026)).
So "compile through MLIR" means **Kubona authors the dialect** (ODS tablegen + PDLL), and
LLVM/MLIR is an *external binary toolchain*, not a quantum compiler. That is a scope
decision, and it is the honest one. The QUIR/Pulse lowering narrative in
[[qir-mlir-compilation]] comes from the architecture paper and should be read as "dialects we
define", not "dialects we import".

## What we stand on (Apache-2.0, all verifiable)

| Asset | Use |
|---|---|
| `qiskit/crates/{circuit, transpiler, passmanager, synthesis, qasm2, qasm3, qpy, quantum_info, accelerate, circuit_library}` | reference transpilation and synthesis; also the **oracle** agent proposals are scored against |
| `qiskit/crates/cext` + `cext-vtable` | stable C ABI via vtable → Kubona's Rust core can bind libqiskit without PyO3 in the hot path |
| `openqasm/source/grammar/{qasm3Lexer.g4, qasm3Parser.g4}` | canonical ANTLR grammar for OpenQASM 3.1, plus `source/language/*.rst` and `examples/{adder,dd,defcal,gateteleport,ipe}.qasm` as conformance fixtures |
| `mlir/tools/{mlir-tblgen, mlir-opt, mlir-pdll, mlir-reduce, mlir-query, mlir-irdl-to-cpp}` | dialect authoring, pass driver, reducer for counterexample minimisation |
| `crates/qir-core/src/lib.rs` (`parse`, `transpile_to_native`, `emit_qir`) | Kubona's existing Rust lowering core — the seed dialect |
| `kubona/qir/native.py` | dual-path execution: `engine: "rust-qir-core"` vs `"python-reference"` (:74) |

## The harness

`mutable_artifact` = a **pipeline spec**: `[pass_id, params] × N` + layout/tiling map +
routing constraint set. `metric` = weighted
`(two_qubit_depth, total_depth, predicted_fidelity, compile_walltime)`.
`validity_check` = the circuit must be **semantically equivalent** to the input:
1. statevector fidelity ≥ 1 − ε on ≤ 12 qubits (Qiskit `quantum_info`);
2. stabilizer/Clifford path via `stim` where applicable (`milimoquantum/hal.py:8` precedent);
3. PT-identity check for parameterised blocks;
4. `mlir-reduce` minimises any counterexample the moment a check fails, so the agent receives
   a *small* falsifying circuit rather than a 200-qubit diff.

Everything runs inside the existing `Harness`/`Orchestrator` contract
(`platform/kubona/autoresearch/base.py:40-159`) with `budget_steps` bounding the search.

## What is genuinely new

1. **Compile receipts as ledger objects.** Each accepted pipeline emits a receipt:
   `input_sha → [pass_sha…] → output_sha`, engine tag, walltime, equivalence evidence. A
   result can then be traced back through *the compiler that produced it*
   ([[provenance-and-reproducibility]]) — today we trace job → device, not job → pass-sequence.
2. **Pass-sequence transfer.** Accepted pipelines are stored as reusable strategies keyed by
   (gate set, topology, qubit count, target), so a pipeline found on one device seeds the
   search on the next. This attacks the real cost: transpilation search is recomputed from
   scratch everywhere today.
3. **The agent sees use-def chains, not source text.** Because the dialect is ours, agents
   query it (`mlir-query`-style): which qubits are live at op *i*, where non-classical data
   flows. This is the concrete, non-mystical version of "agents understand the hardware":
   structured access instead of guessing from prose. See [[mcp-and-context-flooding]] — the
   dialect query interface is precisely how we keep compiler context *small*.
4. **`defcal`/Pulse as a later dialect, gated the same way.** OpenQASM 3.1 `defcal` blocks
   (`examples/defcal.qasm`) are the seam where control-level lowering starts; adding a Pulse
   dialect must not loosen the equivalence gate.

## Boundary conditions

- MLIR is an **external binary** with a pinned version contract, never a vendored tree
  (`llvm-project` is larger than Kubona itself; (repo-audit-2026)).
- The Rust dialect work is scoped to *one* dialect (Kubona QIR) plus one lowering target
  (OpenQASM 3.1 → provider gate sets). Multi-vendor Pulse is a later phase, not a promise.
- Q# / Braket front-ends remain Blueprint until a dialect exists for them
  ([[microsoft-azure-quantum]], [[aws-braket]]).

See also [[qiskit]], [[evaluation-and-benchmarks]] (correctness-regression harness gates this
pipeline), [[openqasm]]-grammar via [[qiskit]], [[autonomous-ansatz-optimization]].
