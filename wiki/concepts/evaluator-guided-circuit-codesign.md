---
type: concept
name: Evaluator-Guided Circuit Co-Design
date: 2026-10-18
updated: 2026-10-18
tags: [search, evaluator, kickmix, tq-cost, map-elites, families]
sources: [source-llm-circuit-codesign-2026, source-gap-analysis-plan]
status: adopted
---

# Evaluator-Guided Circuit Co-Design

The plan adopts the Gu et al. loop as a Kubona harness type, not as vendored code (source-llm-circuit-codesign-2026).

## TaskSpec + KickmixEvaluator + TQScorer

- `TaskSpec {interface π,g, domain, error/failure budget, input distribution D}` — fixed setup, never agent-editable.
- `KickmixEvaluator`: trajectory sim `(bits, phase)`; checks output match, inputs unchanged, ancillas `|0⟩`, phase exact (Methods Eqs 10-16). Dirty-ancilla reset is **not** clean. Record-controlled basis change **refuses**. `ε` stats per Eqs 17-21 + `P_{k,b}`.
- `TQScorer`: `T×Q` default (`κ=1` for paper-comparability); `Q^κ·T`, Toffoli-depth, active-volume variants wired to provider capability (factory throughput, cold storage). Single shared const for weights — never two copies.
- Programs as Python builders (`alloc/free/ccx/MBU`), not flat gates. Evaluator is ground truth; LLM never judges. Fresh session per iteration; `DISCOVERY:` notes travel with program.

> ⚑ Correction (2026-10-18 audit): the first implementation sampled one random
> measurement record per input, which can miss the failing record (false valid),
> and tracked allocs without markers (leaks invisible, re-alloc reuse stale bits).
> The evaluator now exhausts `inputs × 2^m` trajectories when small (cap 1024,
> exactness demanded), enforces alloc→free-or-measure lifecycle (dirty-free and
> leaks refuse), and resets re-allocs to `|0⟩`. Garbage `κ` refuses on every
> surface (Go, Python, console) instead of defaulting to 1.

## Search

- Ideation: ≤5 distinct families from spec alone, blank start, separate maturation, periodic elite sharing, widen-nudge on stagnation (anti-conservatism fix).
- Retention: MAP-Elites `(T,Q)` grid keeps min-TQ per cell; space-efficient circuits survive worse-TQ. Progress need not be monotonic.
- Provenance: every trial ledger-sealed (artifact, `T,Q,ε`, seed, verdict). Library filing requires ledger ref — phantom archive refuses fail-closed.

## Relation to existing engine

Extends `platform/kubona/autoresearch/base.py` four-part contract (`fixed_setup/mutable_artifact/metric/validity_check/run`): `metric` becomes `(T,Q,ε)→TQ(κ)` tuple scorer; `validity_check` becomes trajectory verifier. `search_mode.py` beat-baseline + fail-closed refusal pattern reused. See [[evaluation-and-benchmarks]], [[karpathy-autoresearch]], [[failure-budget-and-frontier]].

## Prototype note

The prototype implements this as a simulator-only harness with Qiskit Aer local sim (no QPU keys). Passing tests prove mechanism feasibility, never design finality.
