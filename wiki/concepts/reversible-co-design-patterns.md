---
type: concept
name: Reversible Co-Design Patterns
date: 2026-10-18
updated: 2026-10-18
tags: [qir, reversible, routing, uncomputation, mbu, compiler-passes]
sources: [source-llm-circuit-codesign-2026, source-gap-analysis-plan]
status: adopted
---

# Reversible Co-Design Patterns

Reusable `qir-core` + Python-fallback passes implementing the paper's three principles as compiler mechanics (source-llm-circuit-codesign-2026).

## 1. Conditional routing

Detect `2× controlled-K` → `swap-K-swap` canonicalization (`Ub=Rb⁻¹KRb`); share one uncontrolled kernel. Requires canonicalizing branch states to a shared representation — the hard part is deciding which differences matter, not the swaps.

## 2. History decoding (no history stack)

Recover branch bits during uncompute from reachable-state invariants: BM `σ0δ=1`, `δold=σ0τ0⁻¹δnew`; divstep `b=bit_{j−1}(g'vq⁻¹ mod 2^j)` (App A Lemmas A1-A5, Thm A6, Cor A7-A8); Lanczos recompute + 1-bit/round stored decision. **Every decode ships with a Lean proof obligation** via [[formal-proof-verification-lean-comparator]] (`ρ=1.0` path); comparator must pass.

## 3. Supporting passes

- **MBU accounting** (`2T→1T+CZ`); never count a measured clear as free without its `CZ` correction.
- **In-place accumulation** (never materialize full product/value); direct-into-destination adds.
- **Offline integer-exact fitting** (widths/truncations/roundings share one error budget).
- **Evaluation-interpolation** `Au=Φᵀdiag(η)Φu` (evaluate → pointwise-mult → linear-map-back).
- **Scan merging** (compare+subtract one pass, renumber-not-shift, measured carries).

All passes are exact-or-budgeted, never silent. Budget use is ledger-recorded and claim-grammar-checked. Fix the class, not the instance: every conditional-arithmetic site gets routing audit; every history stack gets decode-or-budget audit.

See [[evaluator-guided-circuit-codesign]], [[failure-budget-and-frontier]], [[qir-mlir-compilation]], [[agentic-qir-compilation]].
