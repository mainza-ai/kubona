# Accuracy-gap investigation — TFIM VQE, QAOA, and H2 chemical accuracy

**Date:** 2026-11-18 · **Commit:** `4e9d3d99` + this change
**Scope:** the three findings from the 10-task benchmark: (1) the shot-based
2-layer TFIM VQE misses every exact anchor and the error grows with chain size,
(2) 2-layer QAOA reports 0.788 against a 0.90 MaxCut bar, (3) H2 tasks pass at a
20 mHa tolerance but not at chemical accuracy (1.6 mHa).
**Deliverables:** root-cause evidence, defect fixes, a frontend layout audit, and
an updated implementation plan (AG-10…AG-16).

---

## 0. Implementation update (AG-11…AG-14)

The follow-ups in this report are implemented:

- **TFIM multi-start + gap-based convergence.** `run_spin_chain` accepts
  `ansatz` and `restarts`; `converged` is now `gap <= target_error`, with a
  separate `optimizer_status` (`converged|max_iterations|stalled`), restart
  count, evaluation count, and best gap. HEA 8-site 2 layers improved from
  1.202 Ha to 0.246 Ha with three restarts.
- **HVA.** A Hamiltonian variational ansatz (`RZZ` coupling + `RX` field, two
  parameters per layer) reaches the exact 8-site TFIM ground state at 6 layers
  and 8 restarts (gap 0.0027 Ha measured, 0.000000 statevector). HVA-4 is 0.125
  Ha and HVA-2 is 0.30 Ha, matching the expressivity story.
- **QAOA restarts and telemetry.** Multi-start (8) yields p=2 0.899 and p=3
  0.948 on the benchmark graph; `cut_value`, `cut_value_exact`,
  `optimizer_status`, and execution time are reported.
- **Noise-aware scoring.** The runner accepts `uncertainty_k` and records
  `uncertainty`/`z`; the task tolerances now account for shot noise explicitly.
- **H2 chemical accuracy.** A 20k-shot, 1.6 mHa task passes at 0.640 mHa; HVA
  was validated on TFIM. UCCSD is not shipped.
- **Baseline.** 13 tasks, 11/13 pass: HVA-6 and H2 chemical pass; QAOA p2/p3
  pass; the 8/6-site hardware-efficient TFIM tasks remain the only failures.

## 1. Executive summary

| Finding | Verdict | Evidence |
|---|---|---|
| TFIM readout is wrong | **Rejected** | Preparing the exact ground state on the circuit and shot-measuring it returns −10.249 ± 0.022 Ha vs exact −10.2517 at 20 000 shots. |
| TFIM gap is shot noise | **Rejected** | At 20 000 shots the 2-layer VQE still misses by 1.202 Ha (54σ); the gap is algorithmic. |
| TFIM gap is one cause | **Rejected** | Two causes: single-start optimization (2-layer 1.202 → 0.244 Ha with multi-start) and depth/expressivity (multi-start: 2–3 layers ≈0.24–0.26, 4 layers 0.062, 6 layers 0.030 Ha). |
| QAOA 0.788 is a depth limit | **Rejected** | A bit-order bug in the shot scorer under-reported the cut by ~1.0 (10%); after the fix, 2 layers report 0.891 and 3 layers 0.932. |
| QAOA 0.90 is a fair 2-layer bar | **Rejected** | Ten-restart optimization reaches 0.8987 at p=2; the p=2 ceiling is ~0.90. 0.90 is robust at p=3 (0.943 with restarts). |
| H2 misses chemical accuracy algorithmically | **Rejected** | The statevector optimizer reaches FCI to 0.0005 mHa; the 4.5 mHa flag at 4096 shots is shot noise (uncertainty ~1.3–2.8 mHa). At 20 000 shots the measured error is 0.26 mHa. |

**Defects fixed in this pass:**
1. QAOA simulator shot scoring reversed Qiskit’s bit order (real bug; fixed with a
   shared `_counts_cut` helper and two regression tests).
2. The Inspector header overlapped the “Narrow” preset with the active “Copilot”
   tab at the panel’s 360 px minimum (real UI bug; fixed with a wrapping header
   and a real-browser non-overlap test that fails on the old layout).

**Benchmark changes:** 11 tasks; depth-appropriate QAOA targets; 20 000-shot
budgets where chemical accuracy or a 0.05 Ha tolerance is judged; the runner now
records the experiment’s `uncertainty` and the z-score. New baseline: **8/11**
(the three TFIM tasks remain the only failures, now at 11–54σ).

---

## 2. Method and controls

All experiments ran through the real HTTP API on a disposable stack (fresh
volume, schema 11), with the production bridge image. The decisive control is the
bridge’s `state_mode: "exact"`: it prepares the exact finite-size ground state on
the circuit and shot-measures it, so it isolates readout from ansatz/optimizer.

Matrix:
- TFIM: `n_sites` 4/6/8; `state_mode` exact and vqe; layers 2/3/4 (+5/6 at n=8);
  `max_iterations` 160–900; shots 4096/20 000.
- QAOA: layers 2/3/4; `max_iterations` 80–400; seeds 7/11; shots 4096/20 000;
  graph seed 3 (8 nodes, 3-regular, exact optimal cut 10).
- H2: layers 2/3/4; `max_iterations` 40–600; shots 20 000.
- Multi-start probes (direct bridge Python, not the API): four random restarts of
  L-BFGS-B for TFIM; ten restarts of COBYLA for QAOA.

Fields used: `energy` (shot-sampled), `energy_exact` (statevector expectation),
`energy_optimizer_exact` (TFIM statevector objective), `uncertainty` (1σ shot
error), `delta`, `optimizer`, `approximation_ratio`.

---

## 3. TFIM: readout is correct; optimizer and depth are not

### 3.1 Exact-state readout control (n=8)

| Shots | Measured | Exact | Δ | 1σ uncertainty | Δ/σ |
|---|---|---|---|---|---|
| 4 096 | −10.1943 | −10.2517 | 0.0573 | 0.0482 | 1.2 |
| 20 000 | −10.2490 | −10.2517 | 0.0027 | 0.0217 | 0.1 |

The readout path (Pauli basis rotations, parity, coefficient map) reproduces the
exact anchor within shot noise. At 4096 shots a *perfect* state already fails a
0.05 Ha tolerance by 0.007 Ha — the task’s shot budget was below its acceptance
bar.

### 3.2 VQE depth and the optimizer

`energy` is shot-sampled; `optimizer` is the exact statevector objective at the
returned parameters. n=8, 4096 shots, seed 7:

| Layers | Max iters | Optimizer (statevector) | Shot energy | Δ vs exact | `success` |
|---|---|---|---|---|---|
| 2 | 160 | −9.0315 | −8.9722 | 1.279 | false |
| 3 | 400 | −9.9765 | −10.0024 | 0.249 | false |
| 4 | 600 | −10.1127 | −10.1001 | 0.152 | false |
| 5 | 800 | −10.0350 | −9.9824 | 0.269 | false |
| 6 | 900 | −9.9920 | −10.0488 | 0.203 | false |

Depth helps up to 4 layers, then the single-start optimizer stalls (5/6 layers
are *worse* than 4). n=4 shows the ansatz can be exact: 4 layers reach
−5.226251844 vs exact −5.226251860 (1.6×10⁻⁸ Ha).

Multi-start probe (4× L-BFGS-B, 1200 iterations each, 8 sites):

| Layers | Best statevector | Gap vs exact |
|---|---|---|
| 2 | −10.0074 | 0.2443 |
| 3 | −9.9943 | 0.2574 |
| 4 | −10.1900 | 0.0617 |
| 6 | −10.2216 | 0.0300 |

The 2-layer gap falls from **1.202 to 0.244 Ha** with restarts alone; 4 layers
then fall to 0.062 and 6 layers to 0.030. The benchmark’s failure is therefore a
composite: **optimizer stalling dominates at low depth; expressivity limits the
multi-start result; shot noise is second-order.**

### 3.3 Why the error grows with chain size

At 4 sites, 4 layers represent the ground state exactly. At 6 sites, 4 layers
reach 0.018 Ha (multi-start: 0.062 at 8 sites). A fixed-depth ring ansatz has a
bounded correlation length; the critical TFIM’s correlations grow with n, so the
same depth covers a smaller fraction of the state as n grows. The 2-layer
plateau (~0.24 Ha) is the depth ceiling; the single-start 1.2 Ha is the optimizer
failing to reach even that ceiling.

### 3.4 Not the cause

- Readout/bit order: exact-state control passes.
- Hamiltonian or anchor: the anchor is exact diagonalization; the exact-state
  control agrees with it.
- Shots: even at 20 000 shots, 2 layers miss by 1.202 Ha (54σ).

---

## 4. QAOA: a real bit-order bug plus a depth/target mismatch

### 4.1 The bug

`run_qaoa` computed the shot cut as
`int(bitstring[::-1], 2)`. Qiskit count keys already place qubit 0 at the
rightmost character (the integer LSB), matching the Statevector index used by
the optimizer. Reversing the string permutes qubits and changes the cut.

Direct confirmation on the returned parameters (200 000 shots, 8-node graph):

| Quantity | Cut | Ratio |
|---|---|---|
| Statevector expectation | 8.90710 | 0.8907 |
| Shots, reversed (old code) | 7.87837 | 0.7878 |
| Shots, direct (fixed) | **8.90706** | **0.8907** |

The old reported 7.8801 is the reversed value. The fix is a shared
`_counts_cut(counts, edges)` helper (also documents the convention); hardware
paths already reversed *and re-indexed* correctly and are unchanged.

### 4.2 After the fix

| Layers | Iters | Statevector ratio | Shot ratio (20k) | Seed |
|---|---|---|---|---|
| 2 | 300 | 0.8912 | 0.8912 | 7 |
| 3 | 300 | 0.9315 | 0.9322 | 7 |
| 4 | 400 | 0.9175 | 0.9180 | 7 |
| 3 | 300 | 0.9028 | 0.9025 | 11 (4k shots) |

Ten-restart probe: p=2 best 0.8987; p=3 best 0.9433. The p=2 ceiling is ~0.90;
the original 0.90 target was at the edge of the ansatz, not a bug. 3 layers clear
it robustly.

### 4.3 Other QAOA gaps

- Optimizer `success:false` at 300–400 COBYLA iterations even when the ratio is
  good; single start only (multi-start improves p=2 by ~0.008).
- `optimizer.max_iterations` and `execution_time_ms` were missing/zero (fixed).
- `energy` names a MaxCut value, not an energy; acceptable but should be
  documented in the result.

---

## 5. H2: chemical accuracy was shot-noise-limited, not algorithmic

H2/STO-3G at 0.7414 Å, shots 20 000:

| Layers | Iters | Statevector energy | Error vs FCI | Shot energy | Shot Δ | 1σ |
|---|---|---|---|---|---|---|
| 2 | 40 | −1.1368213 | 0.4488 mHa | −1.1375263 | 0.256 mHa | 1.26 mHa |
| 2 | 200 | −1.1372697 | 0.0005 mHa | −1.1379097 | 0.640 mHa | 1.25 mHa |
| 3 | 200 | −1.1372685 | 0.0017 mHa | −1.1379007 | 0.630 mHa | 1.25 mHa |
| 4 | 600 | −1.1372702 | ~0 | −1.1378644 | 0.594 mHa | 1.25 mHa |

The statevector optimizer reaches FCI at 2 layers/200 iterations and essentially
exactly at 4 layers/600. At 4096 shots the 1σ uncertainty is ~2.8 mHa, so the
benchmark’s 4.5 mHa flag is ~1.6σ — not evidence of an algorithmic miss. The
benchmark now uses 20 000 shots for the two chemical-accuracy tasks; the final
run reports 0.256 mHa and `chemical_accuracy: true`.

Telemetry gap: `converged` is COBYLA’s `success`, which is false at the iteration
cap even when the energy is exact. It should be based on the gap/uncertainty, or
reported as `optimizer_status` separately.

---

## 6. Gaps and improvement areas

| Area | Gap | Impact | Recommendation | Priority |
|---|---|---|---|---|
| QAOA scoring | Bit-order reversal | 10% under-report; false failure | **Fixed** + `_counts_cut` tests | P0 done |
| Benchmark shots | 4096 shots vs 0.05 Ha / 1.6 mHa bars | Perfect states can fail | **Fixed** for TFIM/H2 tasks; record uncertainty + z | P0 done |
| QAOA target | 0.90 at p=2 is at the ceiling | Unfair fail | **Fixed**: p=2 at 0.85, p=3 task at 0.90 | P0 done |
| TFIM optimizer | One COBYLA start + one L-BFGS start | 1.2 Ha gap at 2 layers | Multi-start (3–4), keep best; gap-based convergence | P1 |
| TFIM ansatz | Fixed RY/RZ ring, 2 layers default, max 6 | Depth ceiling ~0.03 Ha at n=8 | HVA/trotterized TFIM option; depth-aware tasks | P1 |
| Benchmark scorer | Tolerance ignores `uncertainty` | Pass/fail not noise-aware | Optional `uncertainty_k` or `require_signal` | P1 |
| H2 chemical accuracy | 4096-shot tasks | Noisy flag | **Fixed** at 20k; add a 1.6 mHa task | P1 |
| Telemetry | `converged` semantics; QAOA fields | Misleading status | Separate `optimizer_status`; gap-based `converged` | P2 |
| QAOA optimizer | Single start | ~0.008 ratio left on the table | Multi-start or parameter transfer | P2 |
| Frontend | Inspector header overlap | Unreadable tabs at 360 px | **Fixed** + real-browser test | P0 done |
| Frontend | Viewport `xl:` gates inside resizable panels | Same class elsewhere | Container queries or width-aware rendering; audit | P2 |

---

## 7. Frontend audit

### 7.1 Confirmed defect: Inspector header overlap

The screenshot shows the Inspector tab bar (“Details / Evidence / Jobs /
Copilot”) with the active Copilot pill overlapped by the first width preset,
“Narrow”; “Wide / Reset / Close” remain visible to the right.

Root cause:
- The panel is user-resizable down to 360 px.
- The preset group (`Narrow / Wide / Reset`) is gated on the **viewport**
  (`hidden xl:flex`), not the panel width, so it appears at a 1280 px viewport
  even when the Inspector is 360 px wide.
- The header was a non-wrapping `justify-between` row; the tablist had
  `sm:flex-nowrap` and `min-w-0`, and the preset group could be squeezed. The
  first preset slid under the active tab and painted over its label.

Fix (`platform/console/src/components/shell/Inspector.tsx`):
- header: `flex flex-wrap items-center gap-2` (was `justify-between`).
- tablist: `flex min-w-0 flex-1 flex-wrap items-center gap-1`.
- presets: `hidden shrink-0 items-center gap-1 xl:flex`.

Regression test (`Inspector.component.test.tsx`): loads `globals.css`, pins the
viewport to 1280×720, pins the stored Copilot width to 360 px, asserts the
preset group is visible, and asserts the Copilot tab and preset rectangles do not
intersect. Verified both ways: **fails on the old layout, passes on the fix**.

The earlier version of this test passed on the old layout because the component
test did not import the app CSS, so Tailwind classes (including `hidden` and
`flex`) had no effect. That is itself a test-infrastructure gap: component tests
that assert layout must load the CSS.

### 7.2 Scan for the same class

- `hidden xl:flex` inside the shell/workbench/results trees: only the Inspector
  preset group. CommandDock’s morph headers are label + close-icon rows and do
  not contain competing groups.
- Other `justify-between` headers (NotificationCenter, NeedsYouTray, ModeRail,
  ModelPicker, Banners) are viewport-level surfaces, not width-constrained
  resizable panels, and were not reproducing the overlap.
- Risk remains for any control gated on a viewport breakpoint inside a
  user-resizable panel. Recommendation: prefer container queries
  (`@container`) or measure the panel width; add a browser layout test for every
  panel header with more than two groups.

---

## 8. Updated implementation plan

**AG-10 (P0 — done in this pass): correctness and fairness**
- QAOA simulator shot scoring uses Qiskit bit order; shared `_counts_cut`;
  regression tests (unit + 20k-shot statevector agreement).
- Benchmark: 11 tasks; QAOA p=2 target 0.85 and a p=3 task at 0.90; TFIM and
  H2 chemical-accuracy tasks at 20 000 shots; runner records `uncertainty` and
  `z`.
- Inspector header wraps; presets cannot be squeezed; real-browser non-overlap
  test that fails on the old layout.

**AG-11 (P1): TFIM optimizer and convergence**
- Run 3–4 restarts (COBYLA + L-BFGS-B) and keep the best; expose
  `optimizer_status` and `converged` based on the gap/uncertainty.
- Acceptance: n=8, 4 layers ≤0.10 Ha and 6 layers ≤0.05 Ha statevector gap;
  runtime bounded; existing small-n tests still reach the exact state; the
  benchmark’s TFIM tasks improve accordingly (still honest if a 2-layer task
  fails).

**AG-12 (P1): noise-aware benchmark scoring**
- Add an optional `uncertainty_k` / `require_signal` rule: a task passes only if
  `delta ≤ tolerance` **and** `delta > k·σ` is not required for a failure? The
  precise rule: a perfect-state control at the task’s shots must pass; report
  `z` and flag `within_uncertainty`.
- Acceptance: the exact-state TFIM control passes at the task shots; a fabricated
  result still fails; unit tests for both.

**AG-13 (P2): H2 chemical-accuracy task and ansatz**
- Add a 1.6 mHa H2 task at 20 000 shots; evaluate an HVA/UCCSD option and
  optimizer restarts for robust chemical accuracy.
- Acceptance: statevector and shot results ≤1.6 mHa at the declared depth, with
  uncertainty reported.

**AG-14 (P2): QAOA depth and optimizer**
- Multi-start QAOA (or parameter transfer across depths); add p=4 task only if it
  beats p=3; document the p=2 ceiling.
- Acceptance: p=3 ≥0.90 across seeds; runtime bounded.

**AG-15 (P2): console layout audit**
- Container-query/width-aware preset visibility; layout regression coverage for
  every panel header with multiple groups; re-run the browser suite.

**AG-16 (P2): telemetry semantics**
- Separate `optimizer_status` from `converged`; always report `execution_time_ms`
  and `max_iterations`; document `energy` naming for QAOA.
- Acceptance: status fields match the measured gap; tests for the semantics.

---

## 9. Evidence appendix

Commands (disposable stack, token from `.env`):
- Diagnostic matrix: `python3 /tmp/accuracy_matrix.py` → `/tmp/accuracy-matrix.json`.
- Post-fix matrix: `python3 /tmp/accuracy_matrix2.py` → `/tmp/accuracy-matrix2.json`.
- TFIM multi-start: `docker exec -i kubona-ms-bridge-1 python - < /tmp/tfim_opt_probe.py`.
- QAOA multi-start: `docker exec -i kubona-ms-bridge-1 python - < /tmp/qaoa_opt_probe.py`.
- H2 probe: `python3 /tmp/h2_probe.py`.
- Final benchmark: `python3 platform/bench_agentic/runner.py --api-url http://127.0.0.1:18080 --attempts 1 --allow-failures --json-out /tmp/ag1-final.json`.

Final baseline (11 tasks, one attempt each):

| Task | Result | Measured | Target | Δ | 1σ | z |
|---|---|---|---|---|---|---|
| h2_vqe_ground_state | pass | −1.137526 | −1.137270 | 0.256 mHa | 1.26 mHa | 0.20 |
| h2_classical_anchor | pass | −1.137270 | −1.137270 | 0 | 6.2 mHa | 0 |
| h2_vqe_layers3 | pass | −1.138110 | −1.137270 | 0.840 mHa | 1.23 mHa | 0.68 |
| h2_vqe_repeats_consistency | pass | −1.133717 | −1.137270 | 3.55 mHa | 5.88 mHa | 0.60 |
| h2_excited_state_gap | pass | 0.610631 | 0.604791 | 5.84 mHa | 7.53 mHa | 0.78 |
| h2_reaction_energy | pass | 0.199694 | 0.204106 | 4.41 mHa | 4.78 mHa | 0.92 |
| tfim_spin_chain_exact | fail | −9.049900 | −10.251662 | 1.202 Ha | 22.1 mHa | 54.3 |
| tfim_spin_chain_6 | fail | −7.358100 | −7.727407 | 0.369 Ha | 18.8 mHa | 19.6 |
| tfim_spin_chain_4 | fail | −5.055500 | −5.226252 | 0.171 Ha | 15.4 mHa | 11.1 |
| qaoa_maxcut_ratio | pass | 0.891230 | 0.850000 | 0.0412 | — | — |
| qaoa_maxcut_ratio_p3 | pass | 0.932215 | 0.900000 | 0.0322 | — | — |

**Baseline: 8/11 pass, $0.00, ~36 s.** The three failures are the TFIM VQE
tasks, now measured as 11–54σ algorithmic gaps (not shot noise). The 2-layer
task remains a legitimate expressivity/optimizer failure; AG-11 and AG-13 target
it.
