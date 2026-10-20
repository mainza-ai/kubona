---
type: concept
name: Implementation Plan Forward
date: 2026-10-20
updated: 2026-10-20
tags: [planning, implementation, sequencing, decisions]
sources: [source-llm-circuit-codesign-2026]
status: tracked
---

# Implementation Plan: How Best to Proceed (2026-10-20 Deep Dive)

> **What this page is.** The sequenced way forward from the verified baseline
> (Phases 0–4 complete, F1+F2 landed: 3.93M-gate paper-scale RS decoder,
> proven end-to-end). It records (a) what is DONE and needs no further work,
> (b) what is BLOCKED and exactly what would unblock each item (with newly
> discovered unblocking paths that upgrade three blockers to actionable),
> (c) what is DEFERRED by explicit decision (with the trigger that would
> revive each), and (d) the recommended build sequence with acceptance
> criteria. No implementation here — decisions and sequencing only.
> Status as of writing: all code green (1003 passed), lint clean, all pushed.

## 0. Verified baseline (do not rebuild)

Single coherent RS decoder, 3,928,980T, 110,132 qubits, proven end to end
on planted instances (FLAG=1, Chien marks exactly the planted roots,
composed phase == standalone phase, A*E+B*O=1 on composed outputs).
Components: BM walk 2.42M (70×34.6kT MBU), EEA 1.19M (36 blocks, MBU),
omega ~96kT, Chien 86kT, phase scan 217kT. Full-radius EEA (N=36 covers
all valid inputs; max 35 single-term eliminations measured over 1500
instances + tight telescoping bound; flag guards residuals; one scheduling
block of headroom — mechanism of the off-by-one vs the pure bound is an
open micro-question, harmless (correctness proven + flag safety net)).
MBU bar ruling recorded (E2: measured circuits allowed on classical-input
paths; multi-trajectory proving; fail-closed).

## 1. Unblocked by this investigation (actionable now)

Three Phase-5 items turn out NOT to require waiting:

### U1. A1/A2 (Hermitian series + xi seeding) → ingest companion paper
**Finding.** The SI cites Gu & Jordan arXiv:2510.06603 ("Algebraic Geometry
Codes and Decoded Quantum Interferometry", Oct 2025, 23pp) — the Hermitian
DQI companion paper by the same authors. It plausibly contains the true ω
series definition and xi-seeding rule missing from our SI excerpt (or
definitively does not, which itself closes the hunt).
**Action:** ingest arXiv:2510.06603 per the standard workflow
(raw/verbatim → wiki/sources summary → entity/concept updates →
questions.md rows → overview/roadmap/index refresh → log).
**Acceptance:** EITHER (a) A1/A2 unblock (series/seeding found → new slices
to implement Hermitian readout + xi accumulation with the usual bar), OR
(b) confirmed absent (hunt closed, A1/A2 stay blocked with stronger evidence).
**Effort:** one ingest (bounded, mechanical). **Priority: HIGHEST** — it is
the only path that could unblock an entire thread (Hermitian decode) and
it settles a binary question cheaply.
**Risk:** paper focuses on HOPI optimization, not decoding circuits (may lack
the needed details). Mitigated by the bounded cost (an ingest either way
enriches the blueprint; a negative result is still a result).
> **✅ Verdict (2026-10-20): option (b) — CONFIRMED ABSENT, hunt closed.**
> Pulled the source tarball (arXiv:2510.06603v1, 8 Oct 2025, 475-line
> `main.tex`) and searched it exhaustively: `omega`/`residue`/`series`/
> `Krylov`/`null-vector`/`Toffoli` all 0 hits; `locator` 1 hit (taxonomy
> sentence only — "key equation", no construction); `circuit` 1 hit +
> `reversible` 3 hits, all in-principle statements ("can be implemented in
> principle by reversible circuits", :185) with zero constructions and zero
> T-counts; all 34 `xi` hits are English words (maximize/approximate/exist).
> The paper is HOPI optimization theory (semicircle law, Prange comparison,
> list-decoding outlook) — it contains neither the true ω series + residue
> definition (A1) nor the xi-seeding rule (A2). **A1/A2 stay blocked with
> stronger evidence.** No full source-summary ingest: proportionate response
> to a negative result (per the ingest rule, the value here is the closed
> question, recorded in this verdict + `log.md`); full ingest if Hermitian
> work ever revives. Evidence: `https://arxiv.org/abs/2510.06603` (v1).

### U2. A5 (factory numbers) → adopt literature costs, labeled as such
**Finding.** Magic-state factory costs are extensively published independent
of our paper (Litinski surface-code game; Bravyi-Haah block codes; CCZ
factories 2026; active-volume architectures). A5 does NOT require the
paper's numbers — it requires *a* defensible throughput model.
**Action:** build a factory-cost model from literature (Toffoli magic-state
cost, throughput rate, physical-qubit footprint; cite sources + vintage +
uncertainty), wire it into TQ depth/active variants + console kappa slider
(currently queued behind A5), clearly labeled literature-derived (never
paper-measured).
**Acceptance:** TQ variants compute end-to-end on the decoded T-counts;
kappa slider functional; all numbers trace to cited literature (not the SI).
**Effort:** small-medium (model + wiring, no circuits). **Priority: HIGH** —
unblocks product surface (console) and TQ analysis with no paper dependency.
**Risk:** low (additive; literature clearly labeled; no proven code touched
except wiring).
> **✅ LANDED 2026-10-20 (slice 62): A5 CLOSED.**
> `platform/kubona/codesign/factories.py` (pure, dependency-free): single
> `FACTORIES` const with `gf2019_ccz` (72d² patches, 11d half-cycles/state,
> 1e10-state budget — all GF2019 figures); `factory_report()` keyword-only,
> code_distance + cycle_us REQUIRED, exact-int ledgers + labeled float
> estimates, fail-closed on unknown factory / bad inputs / NaN cycle.
> Wired into `KickmixEvaluator.evaluate(factory=...)` as a `"factory"` key
> (default None = explicitly not computed; partial spec refuses; additive —
> no existing pin touched, all callers keyword-safe). Proven: hand-computed
> table pins, 8 refusal probes, decoder-scale end-to-end (3,928,980T /
> 110,132Q → ~583s serial @ d=27/1µs, within budget), determinism
> double-run. tq.py POLICY comment updated (ranking stays TQ, factory is
> diagnostics). TQ variants (`kappa_sweep`) + console kappa slider verified
> (`CodesignCockpit.test.tsx` green) — no console change needed. 23 tests
> pass incl. export-surface audit.

### U3. B4 + sibling case studies (HHL, Coulomb, Gaussian, lattice-gauge full
sim, Fe2S2-real-data) → explicit scope ruling (recommended: OUT)
**Finding.** None of these are called by any production path (verified:
HHL only in its own module+tests; Coulomb/Gaussian/lattice-gauge are
toy/partial prototypes; Fe2S2 scheduler already proven on synthetic data).
The paper uses HHL as a compilation benchmark (not a decoder component);
the other tasks are case studies demonstrating the LLM framework, not
Kubona program requirements. The program that is funded by evidence is
RS decode + method + blueprint.
**Action:** record a general scope ruling (extends the B4 pattern): other
paper tasks stay prototypes until a concrete agent workflow needs them,
at which point each becomes a scoped slice (same bar). Fe2S2-scheduler
stays as proven infrastructure (no action); Fe2S2-real-data stays A4
(blocked on data, validation-only — the scheduler is already proven safe).
**Acceptance:** ruling recorded with the verified non-dependence + program
definition; no code changes.
**Effort:** nil (decision only). **Priority: HIGH (clarity)** — stops scope
creep by default and routes future asks through the slice process.
> **✅ Ruling recorded (2026-10-20): OUT — verified non-dependence.**
> RS decode path (`rs12, bm, eea12, mbu12, tower12, f12, harness, kickmix,
> tq, frontier`) imports **zero** symbols from `hhl/coulomb/gaussian/
> lattice/chemistry/hermitian/hlanczos` (grep-verified this session);
> `hhl.py` is self-contained (stdlib `math` only) with exactly two importers
> (package re-export + its own slice-13 test); `chemistry` (Fe2S2) serves
> `packs.py` + validation tests as proven scheduler infrastructure (no
> action — stays); `hermitian`/`hlanczos` likewise leaf-only (A1/A2-blocked
> thread, already scoped). General rule (extends B4): other paper tasks stay
> prototypes until a concrete agent workflow needs them, then each becomes a
> scoped slice under the same bar. Fe2S2-real-data stays A4 (blocked on data,
> validation-only). Re-litigation needs a caller in a production path, not
> new interest.

## 2. Deferred with triggers (explicitly NOT now)

### D1. F3 MBU bank recycling — BLOCKED on harness (infrastructure, not circuit)
Ping-pong overwriting proven impossible unitarily (1-bit error caught live;
reversibility/pigeonhole proof). Measurement-clear would cut Q 5x (93k→~20k)
at zero T-cost — BUT requires conditional-X (reset), which kickmix FORBIDS
(`_gate`: only diagonal gates take cond; `x()` has no cond path).
No workaround exists within the harness (verified: diagonal can't erase,
SWAP just moves garbage, fresh banks IS the workaround). Trigger to revive:
a verified conditional-reset primitive in kickmix (own trust-critical slice
with proof; broad blast radius — do not attempt as drive-by). Until then:
fresh banks stand (proven, honest Q).

### D2. F4/C7 divstep packing — SCOPED, queued (separate thread)
Our own fixed-width packed divstep bank: necessary for a feasible
paper-scale divstep walk (naive fresh-bank would be ~600k qubits), but
divstep (modular inversion) is a separate program from RS decode.
Acceptance recorded (explicit width assignment, E1 proofs, measured T/Q
vs naive, labeled OURS). Trigger: divstep program prioritized OR A3
unblocks (either reframes the build). Effort: large (≥ BM-walk slice).

### D3. MBU-scan (C5 follow-up), kara-F8, recursive kara, EEA reversal
All assessed: unitary 11T zero-test impossible (AND-tree lower bound 22T);
MBU-scan saves ~125kT (1.7%) at model-change cost — queued behind F2-style
bar ruling + scan-specific correction proofs. Kara-F8 (~6% of tower),
recursive kara (superseded by MBU-schoolbook). EEA reversal loses on TQ.
Trigger: each needs its own cost-benefit at the time (none now).

## 3. Sequencing (recommended build order)

1. **U1 (companion ingest)** — cheapest binary question with the biggest
   upside (unblocks Hermitian thread). Do first; branches on result.
2. **U2 (literature factory model)** — independent of U1; unblocks TQ/console.
   Can run in parallel with U1 (different work: ingest vs modeling).
3. **U3 (scope ruling)** — decision only; record alongside U1/U2 (clears the
   board: everything not explicitly in-scope stays prototype by default).
4. **Phase 5 re-check** — after U1/U2 land, re-evaluate A1/A2 (possibly unblocked),
   A5 (unblocked by U2), A4 (data hunt as follow-up if scheduler needs it).
   A3/A6/A7/A8 remain blocked (no new paths found; A6/A7 accepted via C1/C2).
> **✅ Re-check DONE 2026-10-20.** A1/A2 blocked (stronger evidence, hunt
> closed); A5 closed slice 62; A4 hunt ruled out (pipeline output, no
> citable source — revive trigger recorded); A3/A6/A7/A8 blocked as
> stated (A8 statement verified against SI :3404-3411). Full record on
> circuit-codesign-production-readiness.md Phase-5 re-check block.
5. **F3/F4/D3 only on trigger** (harness primitive / divstep prioritization /
   cost-benefit shift). Not now.

## 4. What "done" means (proposed acceptance for the program)

The blueprint is contributor-legible when: (i) RS decoder complete + verified
at full radius (DONE — F1/F2 landed, 3.93M T, 60/60 converge + flag safety);
(ii) Hermes/divstep threads either unblocked or explicitly scoped (U1/U3
decide this); (iii) TQ analysis wired to literature factories (U2);
(iv) public-repo decision recorded (Q-27, gated on contributor-legibility —
*not* on Phase-0, which passed). Publication is a separate decision from
technical completeness; do not conflate them.

## 5. Explicitly not pursued (with reasons — re-litigation needs new evidence)

- Recursive-plain Karatsuba (MBU-schoolbook dominates: 144T vs 216T+).
- EEA reversal (TQ-negative at current costs: 7.9M vs 8.9M).
- MBU-scan now (model change for 1.7%; queued with bar ruling).
- C7 now (large separate-thread build; scoped + queued).
- Single-term EEA at 17 blocks (proven insufficient: 52% fail; N=36 landed).
- F3 without harness primitive (proven impossible in-circuit).
- Re-pinning proven slices for the 64T readout micro-win (re-verification
  exceeds gain; noted as queued micro-optimization).
- Any LLM-in-the-loop proving (deterministic harness only — standing rule).
- 71st-position fudging (refuse, don't invent — B8 metrology stands).
