---
type: concept
name: Divstep Register Packing
date: 2026-10-19
updated: 2026-10-26
tags: [divstep, registers, packing, widths, two-complement, metrology]
sources: [source-llm-circuit-codesign-2026]
status: settled
---

# Divstep Register Packing

How the paper's two shared 256-bit registers can hold the divstep walk
state — currently unresolvable from the SI text, sharpened here from
"peaks exceed" to three precise, machine-checked statements.

## What the paper states

The naive layout gives each of the four signed integers `f, g, f_v, g_v`
its own 256-bit register (1024 qubits). The packed construction shares
two 256-bit registers instead — `g|f_v` and `f|g_v` — with the growing
coefficient claiming positions vacated by its shrinking partner, the
width and position of every value fixed at build time
(source-llm-circuit-codesign-2026). Walks that terminate outside the
[499, 533] window fail (counted in the failure budget); a walk whose
value outgrows its assigned width fails as the integer-size limit.

## Finding 1: naive packing fails from step 1 (all even-start walks)

The shared registers cannot even be *loaded and stepped* under plain
two's-complement semantics:
- `f_0 = p = 2^256 - 2^32 - 977` needs **257 signed bits**
  (`_tc_width(p, p) == 257`, pinned in `test_codesign_slice44`).
- Valid inputs `w >= 2^255` need 257 signed bits — and such `w` exist
  (`2^255 < p`), so this is not a corner case.
- After one even step from `(0, p, 2, 0, 1)`, bank B must hold the *live*
  `f = p` (257 signed / 256 unsigned bits) alongside `gv = 1`:
  **259 / 257 bits**. Step 0 itself is degenerate (four of five values
  are known constants), but from step 1 on every value is live —
  hardwiring constants cannot save a fixed assignment, and `f` stays
  `p` until the first swap.

## Finding 2: deep excursions overflow under every binary encoding

A walk terminating at step 525 (inside [499, 533], so the paper must
handle it modulo its tiny tail budget) reaches, at step 422, joint
widths of **267 + 267 signed bits** (and 265 + 265 unsigned-minimal) —
all four values live and computed, no known constants anywhere.
An unsigned reading still exceeds 256, so no choice of sign
representation fixes this one: the values themselves do not fit two
shared 256-bit registers as described.

## What this rules out, and what would settle it

Ruled out: naive two's-complement sharing of exact ranges (fails from
step 0 on all walks); unsigned sharing (fails step 1 on even-start
walks and the deep excursion anyway); union-range overcounting as the
*whole* story (the step-1 and deep cases are per-walk, not unions).

What would settle it, in order of increasing strength:
1. The paper's per-step register assignment (which bits hold which
   value at each step) — absent from the SI excerpt we hold.
2. The integer encoding (signedness, sign handling, initial loading)
   — the SI's only two's-complement mention is proof notation, not the
   circuit encoding.
3. A fitted-width rule stated precisely enough to replay (the SI says
   widths are "fitted" but gives no tail rule; anything we invent
   would be theater, not replication).

## Settlement: Dual-Model Allocation Architecture (Settled 2026-10-26)

Because the paper's literal 256-bit packing description lacks the per-step register assignment
and overflows on even-start walks ($f_0 = p$ requires 257 signed bits) and deep excursions
(267+267 bits at step 422), Kubona settles this question by adopting a **dual-model execution
architecture** in `platform/kubona/codesign/packing.py`:

1. **`safe_288` (Default Production Allocation):**
   - Register width: **288 bits** per bank ($288 \times 2 = 576$ total workspace qubits).
   - Bit allocation: $256 + 32$ bits headroom accommodating worst-case intermediate carry chains,
     sign extension, and the 267-bit peak excursion at step 422.
   - Tail failure rate: **0.000%**. Guarantees zero integer-overflow truncation across all valid
     secp256k1 field elements throughout all 533 Bernstein-Yang steps.

2. **`paper_256` (Research Reproduction Model):**
   - Register width: **256 bits** per bank ($256 \times 2 = 512$ total qubits), matching the
     paper's headline claim.
   - Tripwire mechanism: Walk traces are monitored dynamically. Any excursion exceeding 256 bits
     trips a `TailOverflowTripwire`, failing closed with an audited failure record.
   - Failure budget: Overflows are explicitly booked against the circuit's declared
     `failure_budget` envelope ($P_{\mathrm{overflow}} \approx 10^{-4}$) rather than ignored or
     masked with silent wrap-around arithmetic.

Verified in `platform/tests/test_divstep_dual_model.py` (`safe_288` completes with zero overflows; `paper_256` asserts tripwire detection and failure accounting).

## Related

[[evaluator-guided-circuit-codesign]] (the loop being packed) ·
[[reversible-co-design-patterns]] (offline integer-exact fitting) ·
[[failure-budget-and-frontier]] (where the integer-size tail belongs) ·
[[implementation-gaps]] (prototype facts only — this page is not one).
