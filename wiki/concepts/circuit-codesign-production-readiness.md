---
type: concept
name: Circuit Co-Design Production Readiness
date: 2026-10-19
updated: 2026-10-19
tags: [circuits, codesign, production, gaps, planning]
sources: [source-llm-circuit-codesign-2026]
status: tracked
---

# Circuit Co-Design Production Readiness: Gap Analysis + Implementation Plan

> **What this page is.** The production-readiness register for the Kubona
> circuit co-design program (slices 2–54): what is verified, what is
> blocked on the paper, what was built at toy scale and would need
> re-implementation for production, what is simplified today that must be
> hardened, and the sequenced plan to close each class. It is a *code*
> register in the sense of [[implementation-gaps]] — every claim below is
> checkable against `platform/kubona/codesign/`, `platform/tests/`, or a
> cited SI passage — but its audience is the production decision: **what
> still stands between the verified prototype and a shippable stack, and
> in what order to close it.**
>
> Standing rule (from AGENTS.md production bar): **real logic or
> fail-closed refusal; no weakened tests; no scope-dodging; drift-proof
> shared values; line audit before landing.** Anything below that would
> need rework on the path to production is called out explicitly as
> **redo-risk**, not left implicit.

## 0. Verified baseline (what exists, all green at last full-suite pass)

- **Scope:** `platform/kubona/codesign/` — 38 Python files; `platform/tests/test_codesign*.py` — 57 files (slices 2–54 contiguous, no gaps in numbering, plus harness/API/audit suites); 239 slice test functions, 287 codesign-collected items with zero collection errors. Full `platform/tests/` suite executed this session: **976 passed, 3 skipped in ~12.5 min** (skips: optional `langgraph`/`pytket`/`pyquil` imports only, unrelated to codesign). Wiki lint clean (verified this session).
- **Harness (production-grade, not a gap):** kickmix evaluator with alloc/free lifecycle, dirty-free + leak refusal, re-alloc zeroing, record-controlled basis gates; TaskSpec specs with `target_perm` + `target_phase`; cross-language TQ parity (Go `Cost` 1..3 ↔ Python `kappa_sweep`, golden triple pinned both sides). Console (`CodesignCockpit`) is correctly decoupled — read-only scoring only, needs no per-builder updates.
- **Threads verified end-to-end (toy scale unless noted):**
  - Divstep: naive/decoded/full-walk circuits at p=13 and p=17 (792T fwd / 1584T roundtrip, zero-overhead composition); prime-agnostic pipeline; minimized + factored decoders (depths 6–8: 234/960/4568T → 136/424/1606T, 1.7–2.8×).
  - Trace readout: naive 6580T → minimized SOP 1434T (normal) / 1508T (spec) → factored 438T / 370T (3.3–4.1×; spec basis wins under multi-level sharing, not SOP).
  - Gauge loops: 576-loop accumulation (216 plaquettes + 384 rectangles); greedy schedule 3513 mults + 1215 invs; coordinate-descent 2-opt to 3507 + 1213.
  - Reed–Solomon at **paper scale**: BM math (70 syndromes, degree ≤ 35), Chien scan circuit (F₂¹², 36 coeffs, 4095 positions, 85,995T), contracted phase-bit math, mask cancellation (70/70, Vandermonde), full-scan phase circuit (4025 positions, 217,350T = 54T/pos), full decode composition (planted 1/3/10/35-error, exact).
  - Hermitian: striped vs direct scans, scheduled walks, 48-dim paper-scale nullspace walk.
  - F₂¹² arithmetic (slice 54): schoolbook mult 363T (basis-verified), CNOT squaring, Fermat inverter 6897T (exhaustive over all 4096).
  - HHL: toy only (domain [8, 16)).
- **Methodology assets that transfer directly:** `_emit_bm_round` shared-emitter pattern, `verify_cover`/`reduce_anf`/`factor_and_network` minimization stack, `emit_factored` (T = 2·nodes, structural), harvested-state proof discipline, in-code bank-disjointness guards, the `harvest=` model hook pattern.

## A. Blocked on paper artifacts (external — track, do not build around)

These cannot be implemented from the SI text we hold. Each lists exactly what is missing and what would unblock it. Building a guess would be invention, not verification.

| # | Gap | What is missing | What unblocks it |
|---|---|---|---|
| A1 | Hermitian true series + residue | The true ω series; synthesis from syndromes reads 0 at error points (`hermitian.py:35-37`: "true series + residue theorem, queued, never claimed") | Paper's series definition or a derivable equivalent |
| A2 | xi-register incremental accumulation | Seeding is underspecified in the excerpt (`hlanczos.py:379`); the locator currently comes from the proven `null_vector()` path instead | Paper's seeding rule |
| A3 | Divstep packing per-step assignment | Which values occupy the two shared 256-bit banks at which steps, and the integer encoding ([[divstep-packing]]: naive sharing provably overflows from step 1 — f=p needs 257 signed bits; deep excursions need 267/271; unsigned still fails) | Paper's per-step register assignment + encoding spec, or a shown error in the pinned measurements |
| A4 | 41k-row Fe2S2 table | The data is not in the repo (`chemistry.py:127`); synthetic multisets validate the scheduler, never the paper's table | The table itself (or a citable subset) |
| A5 | Factory throughput numbers | Needed for TQ depth/active-volume variants and the console kappa slider (`tq.py`: "queued behind capability data"; "inventing weights without them is theater") | Magic-state throughput, routing, decoder performance data |
| A6 | 20T hand-optimized readout | The SI gives the truth table, never the factored circuit (`group72.py:623`); our best is 370T (18×) | The paper's hand-optimized Boolean structure (unrecoverable from SI) |
| A7 | Gauge loop paper DP + hand structure | Paper: 3136 mults + 480 invs vs ours 3507 + 1213 (greedy + 2-opt plateau; `gaugeloops.py:25`) | The paper's dynamic program (named in SI, no recurrence given) |
| A8 | 51-bit phase-gradient addition | `kickmix` admits only binary phases (`gaugeloops.py:25`); sums stay exact integers instead | A phase-capable execution model or an explicit decision that integer accumulation is the shipped semantics |

## B. Toy→paper scaling gaps (the redo-risk core)

This is the highest-risk category for the stated reason: code that is correct at toy scale but whose *structure* does not survive scaling, so shipping it as-is means re-implementing later. Each item names the structural break, not just the parameter change.

| # | Gap | Toy status | What breaks at paper scale | Production requirement |
|---|---|---|---|---|
| B1 | BM round/walk circuit is F24-only | `build_bm_round`/`build_bm_walk` capped at ≤8 rounds, 4-coeff σ, F₂⁴ (`bm.py:1016-1017`) | Paper needs F₂¹², 70 syndromes, degree ≤ 35. Mult cost jumps 22T→363T per multiply; the single Fermat inversion becomes 6897T (vs 66T toy) — inversion dominates every round. Fresh-bank-per-round at 70 rounds × ~440-bit state ≈ 30k qubits of allocation traffic. | Paper-scale BM round + walk with (a) in-place or small-bank-pool discipline instead of fresh banks, (b) the tower/Itoh-Tsuii inverter from C3, (c) harvested proof over planted paper-scale walks (the toy harness pattern ports directly). **B1a LANDED 2026-10-19 (slice 57): paper-scale round `_emit_bm12_round` (F₂¹², 36-coeff σ, 71-word τ, 8-bit d, schoolbook mults + tower ainv), 82,344T/round pinned, 700 harvested round-sims green (all 3 cases, inputs preserved, temps clean). B1b (70-round ping-pong walk) remains.** |
| B2 | No Euclidean/psi-pass circuit | `reciprocal_psi` is math-only (`psi.py:197`); the 44kT Euclidean pass has no circuit | The scan circuit (slice 51) consumes Ψ as an *input*; without a producing circuit the paper-scale decoder chain has an open joint. F₂¹² arithmetic (slice 54) is the enabler that did not exist before. | **CLOSED 2026-10-19 (slice 59): fixed-schedule 17-block EEA (`eea12.py`), 985,484T pinned, harvested-proven (A,B exact + Bezout on circuit outputs + explicit-refusal flag for non-converging inputs).** |
| B3 | F₂¹² arithmetic is schoolbook-only | 363T mult (verified), 6897T Fermat inverter (exhaustive) — honest baseline, stated non-competitive | At paper scale the inverter dominates every loop that needs division (EEA: ~17 inversions ≈ 117kT naive vs 44kT whole-pass budget). Shipping the naive version into a pipeline and optimizing later is exactly the redo to avoid. | Tower/Itoh-Tsuii inverter (~4–5× cheaper, queued in `f12.py` docstring) and Karatsuba mult, each with the same basis/exhaustive proof discipline, *before* the EEA pass is built on top. |
| B4 | HHL is toy-only (domain [8, 16)) | `build_hhl_toy_task` | Any production HHL use needs the paper-scale instance; the dyadic-select + shift + accumulate pattern is proven only in the toy window. | Paper-scale HHL task circuit, or an explicit scope ruling that HHL stays a toy demonstration (either is a decision; drift is not). |
| B5 | Divstep decoders stop at depth 8 | Depth 9+ refuses (65536 inputs exceed exhaustive-proof scope) | If the paper's 533-step schedule ever needs a depth-9 decision, there is no verified circuit for it — and no non-exhaustive proof discipline has been adopted. The math layer (`decoder_poly_derived`) is already prime-generic to PAPER_P256; only the *circuit* is toy-gated. | A production-bar ruling: either prove depth 9+ is unreachable in the paper schedule (then the refusal is the spec), or adopt a named non-exhaustive proof (sampled + bound, symbolic) with the same refusal discipline elsewhere. |
| B6 | Hermitian scan at 48 dims but no true-series readout | Striped scans, scheduled walk, paper-scale nullspace all land; true series + xi are A1–A2 | The Hermitian decode chain is the least complete thread: geometry ✓, walk ✓, readout ✗. | Unblocked only by A1–A2 resolving. Until then: no Hermitian work should be presented as decoder-complete. |
| B7 | ginv/to_internal inversion queued | `group72.py:316-317` ("inversion queued; the solve is exact and fail-closed") | Group-element inversion in-circuit is unbuilt; anything downstream that needs W⁻¹ must route around it. | Either a verified inverter or an explicit call-graph audit proving no shipped path needs it. |
| B8 | The 71st canceled position | `mask_cancellation` caps at 70 (Vandermonde is exactly determined); paper scans 4024 = 4095 − 71 | One unexplained position. Candidates: position 0, a known-zero mask entry, or paper-side rounding. | Resolve as metrology (find the 71st in the SI or a derived argument), not code: the solver is provably maximal for 70 supplied syndromes. |

## C. Structural simplifications to harden (correct today, costly or heuristic)

These are not bugs — every one is measured, pinned, and stated as non-competitive where applicable. They are listed here because *production use at these costs, or with these heuristics, is a decision*, and the decision has not been recorded.

| # | Simplification | Measured cost of the simple version | What hardening looks like | Standing |
|---|---|---|---|---|
| C1 | Greedy multioutput XOR-cover (`minimize.py`) | Exact QM primes; greedy XOR-cover (stated heuristic) | ~~Exact cover or measured gap~~ **VERDICT 2026-10-19: ACCEPT with numbers.** Exact OR-covers are inapplicable (they overlap -> invalid under XOR-accumulate; measured: optima overlap on 4/6 tables each). Exact ESOP is intractable at 108-minterm scale. Greedy output is locally optimal (exhaustive cube-drop: zero improvement) and permutation spot-checks stable (96-105 cubes vs 100 baseline). Multi-start found one 14%-better factoring (187 vs 219 nodes) — NOT adopted: it sits on divstep decoders (not the RS critical path), would require re-pinning proven slices 45/46 for ~64T, and needs proper validation. Headroom noted as queued micro-optimization if divstep T ever dominates a profile. Even a 2x cover win saves <200T system-wide (<0.003%). No follow-up unless the readout dominates a future cost profile (it does not). | Accepted (measured) |
| C2 | 2-opt local search for gauge scheduling | 3513+1215 → 3507+1213; paper 3136+480 stands | **VERDICT 2026-10-19: ACCEPT with numbers.** 30 random restarts all land ~7045+ total (50% worse than 4720 baseline); random+descent plateaus at ~5750-5800 (22% worse, flatlining after 3 cycles — cannot bridge the 1000-point gap). Different basins; greedy's basin is far superior; no restart will beat it. Gap to paper (3136+480) stands — closing needs the paper's DP + hand structure (queued), not more local search. Thread stays A1/A2-blocked regardless. | Accepted (measured plateau) |
| C3 | Naive Fermat inversion | 66T at F4 (matches paper's trial — fine); 6897T at F12 (dominates) | ~~Tower/Itoh–Tsuii (~4–5×); blocks B1–B2 on it~~ **LANDED 2026-10-19 (slice 55): our-construction F2¹²/F64/F8 tower (`tower12.py`), 684T pinned exhaustive-4096, 10.1× over Fermat — beats the ~1.5kT target. Deployment into BM/EEA is Phase 2/3 work, not a C3 remainder.** | Landed (emitter); blocks B1–B2 now unblocked |
| C4 | Schoolbook F₂¹² mult, 363T | Basis-verified, honest | ~~Karatsuba (~3×); same mechanical proof pattern as C3~~ **CORRECTED 2026-10-19 (slices 56–57): one-level plain Karatsuba built and measured at 432T self-contained — the "1.68×" first claimed was invalid (kara-forward leaves temps dirty; valid comparison is 432T/46-temps vs 363T/zero-temps → schoolbook dominates both ways). Kara kept as comparison-family member + MBU/recursive substrate, NOT the production mult. The paper round (B1a) and EEA (Phase 3) use schoolbook.** | Landed (negative result, documented) |
| C5 | Phase-scan root test: 42T (21T test + 21T mirror) vs paper's 11T | 54T/position total vs 23T; per-position products match (12 = 12) | ~~Measurement-based uncompute or 11T construction~~ **VERDICT 2026-10-19: DEFER (MBU-scan slice, queued).** Unitary 11T is impossible: 12-bit zero-test needs >= 11 ANDs (fan-in), each >= 2T unitarily (compute+uncompute) = 22T floor; ours is 42T (within 2x of floor). Paper's 11T requires MBU (1T/AND, no uncompute) — a measured-circuit model change in the proven scan + feedforward corrections, for ~125kT (1.7% of decode). Correctness risk exceeds saving while the scan is 3% of total T. Accept 42T with numbers; MBU-scan is a labeled follow-up, not Phase 4. | Deferred (MBU-scan queued) |
| C6 | Fresh-bank-per-round walks (BM toy; divstep banks likewise) | Simple, correct, qubit-hungry (fine at toy scale) | ~~In-place updates or a small bank pool for paper scale — required by B1, not optional~~ **CORRECTED 2026-10-19 (slice 58): paper-scale walk ships fresh banks (96,620 qubits); ping-pong overwriting refuted live (1-bit error — emitter needs \|0> outputs) and ruled out unitarily in general; MBU-for-banks (measurement-clear, paper-precedented) queued as the Q-reduction follow-up, a model change not a fix.** | Landed (fresh); MBU queued |
| C7 | Divstep packing: no working circuit at any width | Open question stands ([[divstep-packing]]) | ~~Either ... or our own ...~~ **SCOPED 2026-10-19, DEFERRED to dedicated slice (not this turn).** A3 still blocked (no SI update), so only the 'our own layout' branch is actionable: a fixed-width packed divstep bank (labeled OURS, never the paper's), proven end-to-end with stated overhead vs the naive 1024-qubit baseline. Scoped acceptance: (i) explicit width assignment per step, (ii) exhaustive small-domain proof where feasible + harvested proof at scale (E1), (iii) measured T/Q vs naive, (iv) never presented as the paper's construction. Large build — queued, not Phase-4 filler. | Scoped; queued |

## D. Integration assessment (verified Python ↔ product surface)

Deliberately short: the architecture is correctly decoupled and should stay that way.

- **Go core** (`platform/core/pkg/codesign`: `frontier.go`, `tq.go`) is scoring-only: TQ cost with kappa 1..3 fail-closed bounds, MAP-Elites frontier keeps min-TQ per cell. This is the correct division of labor — Python owns validity (kickmix evaluator), Go owns ranking/serving. **No duplication of builders, no stale numbers**: grep finds no paper T-counts (3136, 6580, 1434…) hardcoded in Go; the API takes (T,Q) as inputs.
- **Cross-language parity is asserted, not assumed**: Python `test_kappa_sweep_pins_triple` (5,10 → 50/500/5000) and Go `TestGoldenTQ` (5,10,1 → 50) pin the same golden; both refuse kappa outside 1..3. The formula is trivial (T·Q^kappa) so drift risk is minimal, but there is **no automated cross-language parity test** — a joint golden-vector check in CI would close this for free.
- **Console** (`CodesignCockpit`) is read-only scoring (TQ, frontier, budget, DQI bound); it needs no per-builder knowledge and has none (zero references to phase_scan/bm_walk/mask_cancellation — verified). Correct as-is; do not wire builders into the console.
- **Auditor's note (real gap, process not code): slices 51–54 are uncommitted** (new `f12.py`, four test files, `bm.py`/`f24.py`/`__init__.py` edits, this log). The 10039cb push landed slices 2–50; the working tree has not been committed since. Production rule: verified work is committed before the next slice starts.

## E. Verification-strength decisions the production bar must rule on

These are not code changes; they are rulings the bar itself needs to record so future work does not re-litigate them.

1. **Harvested-state vs full-domain proof.** The BM round (slice 52) and walk (slice 53) are proven over harvested valid states (all single-error + all double-error + seeded triple toy walks, zero refusals, every temp clean). Out-of-scope inputs (invalid syndromes, degree > 3) are *unspecified by design* — same contract as the divstep harvested proof.
> **✅ Decision (2026-10-19):** harvested-valid-input coverage is the **permanent proof standard**, with the model (`harvest=`) as the validity oracle. Rationale: invalid inputs are unspecified by design (fail-closed refusal at the boundary, never silent wrong answers); full-domain (2^n) proofs are infeasible at paper scale and would weaken the suite by forcing sampling. Future emitters must (a) name their harvested domain explicitly, (b) prove every temp clean + every valid input exact + every out-of-domain input refused-or-unspecified, (c) reuse the `harvest=` hook pattern so the oracle stays swappable. This closes E1; re-litigation requires new evidence, not new unease.
2. **Determinism proofs are double-run equality, not harness-enforced.** Fine and cheap; keep.
3. **Exhaustive-everywhere it matters:** 512-space readouts, 4096-space F₂¹² inverter, 16384-mask decoders, 144-pair bilinear bases — the pattern (exhaust the small domain, prove the large by structure) is the right one. No change; record as the house method.
4. **Fail-closed conventions are load-bearing and tested:** invalid encodings read zero (never don't-cares), refused depths/rounds, draft→`_emit_bm_round` shared emitter, single-home constants (`emit_xor_cover`, `_flag_tables`, `_decoder_layout`). The audit test (`test_all_public_submodule_names_exported`) enforces the export surface. Keep.

## F. Implementation plan (sequenced; each step lands green or not at all)

**Phase 0 — Close out the open loop (now).**
- [ ] Commit slices 51–54 + this page: `feat(codesign): ...` on develop; full suite (976 passed, 3 benign skips — verified 2026-10-19) + lint green before push.
- [x] Update wiki census (136 → 137 with this page) and `index.md` L2 layer map; `wiki-lint.py --strict` clean.
- [x] Record the E1 ruling (harvested-state standard) as a dated decision in §E.1 (2026-10-19) — permanent standard, re-litigation needs new evidence.

**Phase 1 — Arithmetic foundation (unblocks everything paper-scale).**
- [x] C3: tower/Itoh–Tsuii F₂¹² inverter — LANDED slice 55 (2026-10-19): 684T measured (beats ~1.5kT target), exhaustive-4096 + basis proof, exported, full suite green (983 passed). NOTE: ours, not the paper's (SI gives no F₂¹² tower) — labeled as such in `tower12.py` and the slice-55 docstring. Remainder (drop-in inside the BM round, slices 52/53 green unchanged) belongs to Phase 2 B1a, not C3.
- [x] C4: Karatsuba F₂¹² multiplier — CLOSED slice 56, CORRECTED 2026-10-19 (slice 57): one-level plain kara measured 432T self-contained vs 363T schoolbook — schoolbook dominates (negative result, kept as comparison family + MBU/recursive substrate). No existing pins changed. The mult bake-off concluded: schoolbook is the production mult; tower the production inverter.
- [x] Re-measure every downstream T-count that changes — VACUOUS for C4: `emit_f12_mul` has no circuit callers outside Fermat (grep-verified); kara is additive. Re-pinning belongs to Phase 2/3.
- **Phase 1 COMPLETE (2026-10-19).** Arithmetic foundation lands: tower inverter (684T) + Karatsuba mult (432T).

**Phase 2 — Paper-scale BM (the core remaining circuit).**
- [x] B1a: F₂¹² BM round circuit — LANDED slice 57 (2026-10-19): 82,344T/round pinned (schoolbook mults — kara lost the mult bake-off, honestly recorded in C4), 700 harvested round-sims green (10 walks × 70 rounds, one build; replace/zero/retain all covered incl. 2 hunted retain walks; inputs preserved, temps clean). 8-bit incr/neg proven exhaustive; bank sizes exact (τ = rounds+1, never truncating).
- [x] B1b: 70-round paper-scale walk — LANDED slice 58 (2026-10-19): fresh 1316-bit banks per round (ping-pong overwriting ATTEMPTED and REFUTED live — 1-bit round-2 sigma error; the emitter needs |0> outputs, and unitary overwriting of stale banks is impossible by erasure/reversibility, finite pools fail by pigeonhole), one shared 2343-temp block, 5,764,080T = 70×82,344T pinned, 96,620 qubits (93,436 append-only banks). Proven over 10 full planted walks end to end (final bank exact incl. OIDELTA = inv(delta), inputs preserved, temps clean). MBU-for-banks (measurement-clear) queued as the principled Q-reduction — a model change, not a fix.
- [x] B5 ruling: PROVED 2026-10-19 — depth-9 is unreachable in the paper's schedule, so the refusal is the spec. SI evidence: "the cutoff j_store is the depth from which the circuit stops decoding" (main.tex:2298); "j_store=5: it decodes depths two through four" (:2305); "extreme, j_store=9 decodes through j=8" (:2308); the measured family is exactly j_store ∈ {0, 5, 9} (history qubits 198/60/24 — `HISTORY_QUBITS`, paper-measured, no interpolation). No SI configuration uses j_store ≥ 10, so no schedule invokes a depth-9+ decoder (a depth-j decoder is a Boolean function of 2(j−1) bits — depth 9's 65,536 inputs are what our exhaustive proof scope excludes, `divsteps.py:221`). The depths-2..8 boundary stands. Reopens only on new evidence (a source showing j_store ≥ 10 — which would also need the unmeasured d9+ tail split, currently refused).

**Phase 3 — Euclidean/psi-pass circuit (closes the RS decoder).**
- [x] B2: fixed-schedule half-degree EEA circuit — LANDED slice 59 (2026-10-19): 17-block forward-only EEA (`eea12.py`), 985,484T pinned, A·E+B·O=1 verified on circuit outputs, explicit-refusal flag (17 blocks cover harvested locators needing <=17 terms; high-degree locators needing more explicitly refuse — paper-consistent bounded error). ψ·σ′=1 composition is the remaining Phase-3 step below.
- [x] Compose the full paper-scale RS decoder — LANDED slice 60 (2026-10-19): single coherent 7,208,672T circuit (`rs12.py`, 110,132 qubits) chaining walk -> split -> EEA -> psi/omega -> Chien (85,995T pin-verified in place) + phase scan (217,350T pin-verified in place). Proven end to end on planted instances: FLAG=1, Chien marks exactly the planted roots, composed phase == standalone phase, A*E+B*O=1 on composed outputs, inputs preserved, temps clean. **This is the first complete paper-scale RS decoder: it decodes where the EEA converges and refuses explicitly elsewhere. T-count headline: 7,208,672T (~7.21M)** (walk 5.76M + EEA 0.99M + omega 0.23M + Chien 0.086M + scan 0.217M; CNOTs free).

**Phase 3 COMPLETE (2026-10-19).**

**Phase 4 — Close the honest gaps that are closable.**
- [x] C5: VERDICT = DEFER (MBU-scan slice, queued). Unitary 11T impossible (22T floor; ours 42T); paper's 11T needs MBU (model change for ~125kT / 1.7%). Accept 42T with numbers.
- [x] C1: VERDICT = ACCEPT with numbers. Exact OR inapplicable (overlaps); exact ESOP intractable; greedy locally optimal + permutation-stable; even 2x saves <200T. C2: VERDICT = ACCEPT with numbers (2026-10-19). 30 random restarts ~7045+ (50% worse); random+descent plateaus ~5750-5800 (22% worse, flatlining). No restart beats greedy-seeded 3507+1213. Gap to paper stands (needs DP+hand structure, queued); thread A1/A2-blocked.
- [x] C7: SCOPED, DEFERRED to dedicated slice (A3 still blocked; acceptance criteria in C7 row; large build, not Phase-4 filler).
- [ ] B8: RESOLVED 2026-10-19 as metrology (SI main.tex:1706-1707): the paper's linear-in-syndromes part (offline Clifford Z choice) cancels the mask at 71 positions, leaving 4,024 to scan. The 71st is the Clifford-linear one, OUTSIDE the Vandermonde solve — our 70-cap is provably maximal FOR the Vandermonde system (exactly determined). No mystery, no bug: our 4,025-position scan is correct at +54T (one extra position); matching the paper exactly needs the offline linear-part optimization (classical, queued as negligible).

**Phase 5 — Blocked-item watchlist (no work until unblocked; re-check each ingest).**
- A1 (true series), A2 (xi seeding), A3 (packing assignment), A4 (41k rows), A5 (factory numbers), A6 (20T hand structure), A7 (gauge DP), A8 (phase-gradient addition). Each has its unblock condition in the table above. When any unblocks, it enters the plan as its own slice with the same production bar.

**Explicitly out of scope for this program (decisions, not omissions):** console per-builder wiring (decoupled by design); Go-side builders (Python owns validity); HHL beyond a scope ruling (B4); 71st-position fudging (refuse, don't invent); any LLM-in-the-loop proving (deterministic harness only).

## Process notes

- The four live `consumed-def` incidents (slices 51–53: an edit eating `def build_chien_scan` and orphaning its body) are a tooling pattern to name and avoid: never use a bare `def ...:` line as an edit anchor — include the docstring first line or a unique body line in the match.
- Test-writing discipline that paid for itself repeatedly: exact-behavior baselines (naive implementation in-test), refusal probes for every gate, determinism double-runs, and the free-marker dirty-check as the universal temp discipline.
- Suite runtime is the CI long pole (hundreds of simulated circuits dwarf lint and emitter unit tests); the per-slice fast units (emitter tables, basis checks) are the right inner loop. Do not let full-suite time be used to justify sampled (weakened) proofs.
