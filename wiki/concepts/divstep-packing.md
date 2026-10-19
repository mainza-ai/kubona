---
type: concept
name: Divstep Register Packing
date: 2026-10-19
updated: 2026-10-19
tags: [divstep, registers, packing, widths, two-complement, metrology]
sources: [source-llm-circuit-codesign-2026]
status: open-question
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

Until one of those arrives, the packing stays `open-question`: our
surveys keep the exact union-range counts (honest metrology), and no
claim is made that the paper is wrong — only that its stated
description is insufficient to reproduce the packing.

## Related

[[evaluator-guided-circuit-codesign]] (the loop being packed) ·
[[reversible-co-design-patterns]] (offline integer-exact fitting) ·
[[failure-budget-and-frontier]] (where the integer-size tail belongs) ·
[[implementation-gaps]] (prototype facts only — this page is not one).
