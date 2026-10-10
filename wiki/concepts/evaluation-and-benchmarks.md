---
type: concept
name: Evaluation & Benchmarks
date: 2026-08-16
updated: 2026-10-10
tags: [evals, testing, benchmarks]
sources: [source-architecture-paper, source-gap-analysis-plan]
status: adopted
---

# Evaluation Framework & Benchmarks

The architecture paper proposed only **code-generation evals**; the gap analysis splits
evaluation into four complementary tracks (source-gap-analysis-plan).

## 1. Code-generation correctness (from the architecture paper)

- **QuantumKatas** + **Qiskit HumanEval** — hundreds of tasks (basic gates → Grover,
  Deutsch-Jozsa) verified deterministically via classical simulation vs canonical
  statevectors.
- **PennyLang** ([[pennylane]]) — 3,000+ curated samples; RAG-enhanced generation lifts QML
  code-gen success from single digits to ~84%.

## 2. Platform integration/regression (new)

- Does the [[qir-mlir-compilation|QIR lowering]] pipeline still produce correct bitcode after
  a compiler change?

## 3. Hardware-in-the-loop CI (new)

- **Hardware-canary** — nightly run of a canary circuit against *every* connected backend to
  catch silent provider API breakage / calibration drift before an agent hits it mid-workflow
  (Phase 3; candidate home: Go service or Rust).
- The correctness-regression harness of the [[karpathy-autoresearch]] engine runs here and
  **gates releases**, not just logging.

## 4. Agent-behavior evals (new, Phase 4)

- Does the agent route simulator-first? Respect budget ceilings? Correctly detect and
  respond to a [[barren-plateau|WBP]] signal instead of looping? Budget-compliance and
  simulator-first compliance are explicit eval dimensions.

Related: [[karpathy-autoresearch]] · [[qir-mlir-compilation]] · [[barren-plateau]] ·
[[cost-governance]] · [[qiskit]]

## Admissible corpora (Q-26, closed 2026-10-10)

- **Licence-clean only.** Every corpus file carries `_meta: {license, source_url,
  retrieved_at}`; the loader admits in-repo authored, CC0/CC-BY(+SA),
  Apache-2.0, MIT/BSD, Unlicense — and refuses whole files that are
  proprietary, unknown, or unlabelled, with a per-file reason
  (`load_corpus_dir_with_report`). An unlicensed benchmark laundered into evals
  corrupts every claim built on it, so the gate is fail-closed, not advisory.
- **Good-answer criterion.** A good answer is QASM satisfying the task's
  *declared invariant* on the simulator (score = fraction passing). No
  LLM judging, no vibes — satisfiability is the entire criterion, verified by
  `evaluate_codegen` (in-repo corpora additionally prove satisfiability at
  load-test time).