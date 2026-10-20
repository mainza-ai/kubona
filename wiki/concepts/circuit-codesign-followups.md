---
type: concept
name: Post-Blueprint Follow-Ups
date: 2026-10-20
updated: 2026-10-20
tags: [planning, follow-ups, production, mbu, eea, divstep]
sources: [source-llm-circuit-codesign-2026]
status: tracked
---

# Post-Blueprint Follow-Ups: Investigated, Sequenced, Not Yet Built

> **What this page is.** The implementation plan (`circuit-codesign-production-readiness`)
> is complete through Phase 4 (Phases 0–3 built and verified; Phase 4 verdicts
> recorded; Phase 5 blocked on external artifacts). This page is the *next*
> layer: every significant follow-up investigated deeply — feasibility,
> savings quantified where measurable, structural fit checked, risks named —
> with an explicit verdict (build / defer / drop) and sequencing. Nothing
> below is implemented; when an item is built it moves into the
> readiness register as its own slice with the same production bar.

## F1. EEA full radius: 17 -> 35 blocks (RECOMMENDED NEXT)

**Finding (measured + proven).** 17 single-term blocks cover only harvested
locators needing <= 17 terms. Over 1,500 uniform-random valid instances,
776 (52%) need 18–35 single-term eliminations (max exactly 35); the rest
converge. The 35-term bound is tight *and* provable (telescoping:
sum of quotient degrees <= deg(E) = 17, plus <= 18 divisions = 35 max),
so N_BLOCKS=35 covers 100% of valid degree-17 inputs with zero margin
needed — and the explicit-refusal flag stays as backstop (fail-closed
if the bound is ever violated).

**Why not keep 17.** A decoder that explicitly refuses 52% of valid
inputs is not bounded-error, it is broken for over half the radius.
The paper's 17 relies on DQI-typical inputs (low errors) plus bounded
error; our uniform-random measurement shows the failure is systematic,
not atypical. A production decoder (general platform, not DQI-only)
must meet the full radius. The DQI nuance is recorded honestly, not
used to excuse the gap.

**Cost.** ~58kT/block measured (linear — identical blocks), so 35 blocks
≈ 2.03MT (vs 985kT now). +1.05MT total decode (+15%). Honest and affordable.

**Structural fit.** Trivial: `N_BLOCKS` 17→35 (layout asserts are generic;
BT regions scale; no emitter changes; flag/extract agnostic). Re-verify
slice-59 suite with new pins (same strength) + confirm previously-flagged
instances now converge. Estimated: one slice, low risk.

**Verdict: BUILD NEXT.** Highest functional value (closes the only
completeness gap in the decoder), lowest risk (parameter change +
re-verification), cost quantified and acceptable.

## F2. Karatsuba+MBU F2^12 multiplier (QUEUED, major follow-up)

**Finding.** Schoolbook 363T dominates plain Karatsuba 432T self-contained
(slice-56 correction stands), but MBU changes the game: Karatsuba has 108
ANDs vs schoolbook's 144 pairs, and MBU runs each AND at 1T (no uncompute)
vs 2T plain. Estimated MBU-kara F2^12: ~108T + corrections/overhead (~125T
conservative) vs 363T schoolbook = ~3x win. Tower12/F24 MBU precedent exists
(88T -> 48T measured at F4); harness supports hmr + conditional CZ +
multi-trajectory verification (slice-11 methodology ports directly).

**Impact (measured components x estimated mult cost).**
EEA elim mults: 54/block x 17 x (363-125)T ≈ 219kT saved (22% of EEA).
BM rounds: 109 mults/round x 70 x 238T ≈ 1.82MT saved (32% of walk).
Omega: 630 x 238T ≈ 150kT saved. Tower F8 (MBU without kara): 144T saved.
Total decode: ~7.2M -> ~5.0M (**~30% reduction**). Largest single T-win available.

**Cost/risk.** New kara-MBU emitter (~300 lines + exhaustive tests) + re-pin
EEA/BM/omega slices (T-counts change; proofs unchanged in structure).
MBU circuits are non-unitary (measure + feedforward) — consistent with E1
(classical inputs) and tower precedent, but needs an explicit bar ruling
(record MBU as acceptable for classical-input decode paths, same standing
as harvested-state proofs). Multi-trajectory verification required
(corrections must hold across sampled measurement outcomes).
Effort: medium-high (one emitter slice + downstream re-pinning). Risk: medium
(precedent exists at small scale; EEA/BM scale is new).

**Verdict: QUEUE as the major T-optimization (after F1).** Do not bundle with
F1 (F1 is correctness; this is optimization — separate slices, separate pins).

## F3. MBU bank recycling (BLOCKED on harness — infrastructure, not circuit)

**Finding.** Ping-pong overwriting is unitarily impossible (slice-58 proof:
erasure is non-unitary, finite pools fail by pigeonhole). Measurement-clear
(measure each bank wire, X-correct to zero) would cut walk Q 5x (93k -> ~3k
banks + temps), T unchanged (measurements/Cliffords free in T-count).
**Hard blocker:** kickmix forbids non-diagonal conditional gates
(`_gate`: "records can control diagonal corrections only"; `x()` takes no
`cond`). Conditional-X (required to zero measured wires) is inexpressible.
No workaround within the harness (diagonal gates can't erase; SWAP just moves
garbage; fresh banks IS the current design).

**Paths.** (a) Extend kickmix with sound conditional-X/reset for classical
states + verify the extension itself (trust-critical simulator change, broad
blast radius). (b) Accept fresh banks (current, proven). The TQ math favors
(a) *if* sound, but (a) is infrastructure work, not a circuit slice.

**Verdict: DEFER to harness-infrastructure track (not a circuit follow-up).**
Fresh banks stand (proven, honest Q). Revisit only with a verified reset
primitive. Do not attempt MBU-bank workarounds in circuit code (impossible
without the primitive — proven above, not assumed).

## F4. Own fixed-width packed divstep layout (C7 — SCOPED, DEFERRED)

**Scope (from divstep-packing.md + divsteps.py structure).** A1-A8 all still
blocked (no new SI), so only the 'our layout' branch is actionable — and it
is a *different deliverable* from verifying the paper's (must be labeled as
such, per C7). Deliverable: paper-scale (P256) divstep walk with OUR fixed
widths (2 shared banks, measured max + margin), proven end-to-end, T/Q vs
naive-1024 (or naive-wider) baseline.
**Why it matters more than it looks:** naive fresh-bank divstep walk would be
~533 x bank-size (600k+ qubits at 256-bit widths) — infeasible where BM's
93k was merely ugly. Packing is NECESSARY for a feasible divstep walk, not
just an optimization. But divstep (modular inversion / point addition) is a
separate thread from the RS decode program.
**Acceptance (from C7 row):** (i) explicit width assignment (per-step or
uniform, measured + margin); (ii) exhaustive small-domain + harvested proof
(E1); (iii) measured T/Q vs naive; (iv) labeled OURS, never the paper's.
**Effort:** large (new walk builder + packing + proofs; >= BM-walk slice).
**Verdict: QUEUED as a dedicated multi-slice program (not Phase-4 filler).
No action until divstep is prioritized or A3 unblocks (either reframes it).**

## F5. Assessed and closed (no action)

- **EEA reversal (uncompute X..V1 to save ~1.3k wires):** costs ~2x T for -1.3k wires;
  TQ favors forward-only at current mult costs (7.9B vs 8.9B). Deviation from SI
  (main.tex:1789) stands as documented Q-vs-T trade, not a gap. NO ACTION
  (revisit only if mult costs collapse, changing the ratio).
- **Karatsuba-F8 (tower internals):** 16 mults x (18T->12T or 9T MBU) = <200T
  saving on 7.2M (0.003%). Negligible standalone; bundle with F2 if MBU-kara
  proceeds (shared machinery). NO ACTION.
- **Recursive kara (6->3-bit leaves):** superseded by MBU-kara (108T beats any
  plain recursion; mirror-compounding makes plain recursion worse than it looks).
  NO ACTION (fold into F2 if pursued).
- **MBU zero-test (C5, 11T vs 42T):** unitary 11T impossible (AND-tree lower
  bound: 11 ANDs x 2T = 22T floor; ours 42T is within 2x). Paper's 11T requires
  MBU — a measured-circuit model change in the proven phase scan for ~125kT
  (1.7%). DEFERRED to queued MBU-scan slice (needs the same bar ruling as F2
  plus scan-specific correction proofs). Accept 42T with numbers.

## Sequencing

1. **F1 (EEA-35) now** — correctness gap, clean change, unblocks full-radius claim.
2. **F2 (MBU-kara mult) next** — biggest T-win, needs F1 stable first (re-pins downstream).
3. **F3/F4/F5** queued behind their triggers (harness primitive / divstep priority / never).
4. Phase 5 watchlist unchanged (A1-A8 still blocked; re-check per ingest).

## What was explicitly NOT pursued (and why)

- Recursive-plain Karatsuba (dominated by MBU-kara; mirror-compounding).
- EEA reversal now (TQ-negative at current costs).
- MBU-scan now (model change for 1.7%; queued).
- C7 now (large separate-thread build; scoped and queued).
- Re-pinning proven slices for the 14%-better readout cover found during C1
  investigation (divstep-thread-local win, ~64T; re-verification cost exceeds
  gain; noted as micro-optimization if divstep T ever dominates).
