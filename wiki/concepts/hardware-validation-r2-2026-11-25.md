---
type: concept
name: Hardware Validation R2 — ZNE Axis Audit
date: 2026-11-25
updated: 2026-11-26
tags: [hardware, zne, validation, error-mitigation, reproducibility]
sources: [source-architecture-paper]
status: tracked
---

# Hardware Validation R2 — ZNE Axis Audit

**Prototype register.** This page records what the 2026-11-25 audit and the
2026-11-26 corrected-axis hardware runs established about R2 (the H₂ ZNE
chemical-accuracy claim). It is a prototype fact, not a design decision. The
implementation cited here is at
[`mainza-ai/kubona`](https://github.com/mainza-ai/kubona) commit `a2640825`; the
old 2.743 mHa result remains in the ledger as historical, not as a valid ZNE
claim.

## What R2 was meant to show

R2 asked whether Zero-Noise Extrapolation on `ibm_fez` could bring the H₂ VQE energy
inside chemical accuracy (`|Δ| ≤ 1.6 mHa` from the PySCF FCI anchor) and separate the
mitigated result from the raw one. The Phase-4B run reported a point estimate of
**2.743 mHa** with a **±11.037 mHa** interval, against a raw **9.999 mHa (±6.803)**.
R2 was therefore **not achieved**.

## 2026-11-26 corrected-axis hardware runs

The corrected all-gate axis was run on three governed jobs under the new 10 RU
Open Plan account: H₂ STO-3G at 0.735 Å, hardware-efficient `layers=2`, five
all-gate-local scales `[1, 1.5, 2, 3, 4]`, 8192 shots per ZNE circuit, exponential
fixed-asymptote fit, readout mitigation + XY4 dynamical decoupling + gate/measurement
twirling, layout `[149, 150]`.

| Run | Backend | ZNE Δ (mHa) | 95% CI | χ²_red | lack-of-fit p | model spread (mHa) | paired diff (mHa) | separated? |
|---|---|---|---:|---:|---:|---:|---:|---|
| K1 | `ibm_kingston` | **7.365** | ±6.924 | 1.527 | 0.205 | 4.93 | −6.354 ±8.515 | no |
| F1 | `ibm_fez` | **0.483** | ±9.111 | 3.675 | 0.0116 | 2.12 | −3.839 ±10.226 | no |
| F2 | `ibm_fez` | **5.595** | ±10.028 | 4.805 | 0.0024 | 9.43 | +0.609 ±11.053 | no |

Raw baselines were 14.893, 8.999, and 13.361 mHa respectively. The two `ibm_fez`
raw jobs differ by **4.36 mHa** on the same calibration snapshot; the two
identical ZNE runs differ by **5.112 mHa**.

> ⚑ Correction (2026-11-26 post-run raw-count audit): five statements in the
> initial 2026-11-26 summary above were refined after inspecting the raw per-scale
> counts and fold receipts directly (the verdict — R2 not achieved, do not spend
> more QPU on ZNE — stands unchanged):
> 1. **Layouts (A5):** `K1` (`ibm_kingston`) ran on physical qubits `[149, 150]`;
>    `F1` and `F2` (`ibm_fez`) ran on `[142, 143]`, not `[149, 150]`.
> 2. **2Q-folding kink at `s = 1.5` (A1):** `_fold_local_all` previously sliced
>    fractional folds from the tail of the circuit, where `cx(0,1)` sits. At
>    `s = 1.5` (`1.40×` total gates), `logical_2q` jumped `2 → 4` (`2.0×` 2Q gates)
>    and stayed at `4` at `s = 2.0` (`2.0×` total gates), producing a deterministic
>    `+5.8–7.0 mHa` kink at `s = 1.4` on both `F1` and `F2` that inflated `χ²_red`.
>    `_fold_local_all` now folds 1Q and 2Q gates proportionally and the ZNE guard
>    refuses duplicate `logical_2q` counts or `>20%` 2Q-vs-total scale divergence.
> 3. **Weighted term energy vs. Pauli expectation (A2):** the `≈ −0.38` figure on
>    `K1` was the Hamiltonian-weighted term energy `c_IZ ⟨IZ⟩ = (−0.39794)(+0.95)`,
>    not a raw expectation. Raw `⟨IZ⟩`/`⟨ZI⟩` were `+0.93..+0.96` (decaying
>    monotonically `0.961 → 0.919` on `ibm_fez`, and held flat on `K1` by coherent
>    `cx` interference where `01` counts dropped `136 → 67` under folding).
> 4. **Shot noise vs. device drift (A3):** the `4.362 ± 9.598 mHa` (`0.89σ`, `p = 0.37`)
>    raw spread and `5.112 ± 13.549 mHa` (`0.74σ`) ZNE spread on `ibm_fez` are
>    consistent with shot noise, not proof of device instability; the 8192-shot
>    scale-1 baselines in `F1` (`−1.132984 Ha`) and `F2` (`−1.132319 Ha`) agreed to
>    **`0.665 mHa` (`0.20σ`)**.
> 5. **TREX baseline conflation (A4):** `baseline_energy` (`+4.32 / +4.99 mHa` on
>    `F1`/`F2`) already included diagonal TREX readout mitigation, which had removed
>    `~11 mHa` from true unmitigated raw (`+15.66 / +11.46 mHa`). ZNE receipts now
>    report `raw_unmitigated_energy` and `readout_mitigated_baseline_energy` separately.

**The axis is now valid; R2 is still not achieved.** Every run fails at least two
of the pre-registered gates. F1 passes chemical accuracy on the point alone
(0.483 mHa) but fails model adequacy (χ² 3.68, p = 0.012) and difference
significance. No run satisfies
`axis_valid ∧ model_adequate ∧ difference_significant ∧ |Δ| ≤ 1.6 mHa`.

## The remaining blocker: the estimator, not the axis

**The fixed-asymptote model is statistically inadequate.** The per-scale
residuals are 2.4–2.6 mHa at 8192 shots; χ²_red is 3.68/4.81 on `ibm_fez` and
1.527 on `ibm_kingston` (just above the 1.5 gate). More shots would increase χ²,
not reduce the model bias. Reaching a ±1.6 mHa interval from the observed
±9–10 mHa would need roughly 30× more shots (~35 RU), and the model would be
rejected even more strongly.

**The point is model- and axis-dependent.** The same data give:

| Run | total-gate fixed exp | total-gate linear | weighted fixed exp | weighted linear |
|---|---:|---:|---:|---:|
| K1 | 7.365 | 7.390 | 1.758 | 1.851 |
| F1 | 0.483 | 0.614 | 11.968 | 11.457 |
| F2 | 5.595 | 5.692 | 4.609 | 4.235 |

The axis choice moves the point by up to **11 mHa**; the weighted-error axis helps
K1 but hurts F1. There is no consistent coordinate that makes the three runs agree.

**The fixed asymptote is physically wrong for this noise.** Non-identity Pauli
expectations do not decay to zero: on K1 the `IZ` and `ZI` expectations sit at
≈ −0.38 at every scale. The exact asymptote `E∞ = c_II` assumes a unital Pauli
channel; the hardware noise is non-unital (amplitude damping, leakage, coherent
errors). A free three-parameter exponential is unidentifiable over this narrow
noise range (it diverges to unphysical asymptotes with ~1e8 mHa standard error).

**Per-term ZNE is a diagnostic, not a rescue.** The per-term sums are 6.227,
0.315, and 3.345 mHa, but the `IZ`/`ZI` fits have χ²_red 3.4–8.9 and the apparent
cancellation between terms is a fitting artifact.

## The Phase-4B cause: the scale axis mixed two folding mechanisms

`_fold_circuit` previously dispatched odd integer scales to global all-gate folding
(`_fold_unitary`) and all other scales to local two-qubit-only folding
(`_fold_local_2q`). For the Phase-4B H₂ ansatz the measured gate counts were:

| nominal `x` | 1Q | 2Q | total | total-gate ratio | calibration-weighted error ratio |
|---|---:|---:|---:|---:|---:|
| 1 | 8 | 2 | 10 | 1.00 | 1.00 |
| 2 | 8 | 4 | 12 | **1.20** | **1.68** |
| 3 | 24 | 6 | 30 | **3.00** | **3.00** |
| 4 | 8 | 8 | 16 | **1.60** | **3.04** |

Scales 3 and 4 carry nearly the same physical noise but were fitted as `x=3` and `x=4`;
scale 2 is only 1.20× the gates (1.68× the weighted error) but was fitted as `x=2`.
The curve was non-monotonic (`−1.12940, −1.13317, −1.13283, −1.12265 Ha`) and the
model was fitting a distorted coordinate, not a noise axis.

This explains the `chi2_red ≈ 2.0`, the **±11 mHa** interval, and the estimator
instability: linear `2.737 mHa`, fixed-asymptote exponential `2.743 mHa`, free
three-parameter exponential `2.737 mHa`, quadratic `20.090 mHa`.

## The shot-floor analysis from the Phase-4B audit

The recorded per-scale residuals imply a lack-of-fit floor even at infinite shots:

| Shots | 95% CI half-width |
|---:|---:|
| 4,096 | ±11.04 mHa |
| 8,192 | ±9.23 |
| 16,384 | ±8.17 |
| 32,768 | ±7.59 |
| 65,536 | ±7.28 |
| ∞ (model floor) | **±6.96** |

The **difference CI** between raw and ZNE is **±12.965 mHa**, while the observed
improvement is **7.256 mHa**; the improvement is **not separated**. Run-to-run spread is
also larger than the claimed gain: the same estimator on the earlier three-scale Phase-4
data gives **11.075 mHa**, not 2.743.

The fixed asymptote `E∞ = c_II` is exact only for unital/global depolarizing noise. IBM
hardware includes amplitude damping, leakage, and coherent errors, so the asymptote is a
model assumption, not a measurement; it removed the `dof=0` covariance blow-up but can
bias the extrapolation.

## What the prototype now enforces

Implementation: [`platform/bridge/experiments.py`](https://github.com/mainza-ai/kubona/blob/a2640825/platform/bridge/experiments.py#L426)
(`_fold_local_all`, `_fold_circuit`), gate metadata in the mitigation receipt
([`experiments.py`](https://github.com/mainza-ai/kubona/blob/a2640825/platform/bridge/experiments.py#L920)),
and G6 gates in
[`scripts/run_peer_reviewed_experiment.py`](https://github.com/mainza-ai/kubona/blob/a2640825/scripts/run_peer_reviewed_experiment.py#L467).

- **One mechanism per fit.** Every scale > 1 uses barrier-protected local folding of
  **all** gates (1Q and 2Q). Global folding remains an explicit legacy helper and is
  never mixed into a local fit.
- **Measured axis.** Each folded circuit records `requested`, `actual_total_ratio`,
  `logical_1q`, `logical_2q`, `total_gates`, and `weighted_error_scale`; a
  non-increasing or duplicate total-gate ratio is **refused before dispatch**.
- **Replayable data.** `counts_by_scale` retains every per-scale, per-term histogram
  (previously only scale 1 survived), and the supplement stages it as a separate
  `.json`/`.json.gz` artifact with a SHA-256 manifest entry.
- **Honest statistics.** The receipt carries `chi2_reduced`, `model_points`,
  `model_spread_ha`, `model_adequate`, `lack_of_fit_p`, `baseline_energy`,
  `difference_ha`, `difference_ci95_ha`, and `difference_significant`; per-term ZNE is
  computed as a diagnostic from the retained counts.
- **Fail-closed gates.** G6 now requires `axis_valid`, `model_adequate`,
  `difference_significant`, and `|Δ_ZNE| ≤ 1.6 mHa` to pass; a `chi2_red > 1.5`, a
  model spread beyond the interval, or an overlapping difference CI fails the check.

Measured all-gate axis on the 2026-11-26 hardware runs (strictly increasing):

| requested | total gates | actual total ratio | weighted error scale (kingston) | weighted error scale (fez) |
|---:|---:|---:|---:|---:|
| 1.0 | 10 | 1.00 | 1.000 | 1.000 |
| 1.5 | 14 | 1.40 | 1.235 | 1.281 |
| 2.0 | 20 | 2.00 | 1.351 | 1.384 |
| 3.0 | 30 | 3.00 | 1.702 | 1.768 |
| 4.0 | 40 | 4.00 | 2.053 | 2.152 |

## What is not established

- R2 is **not achieved**. The corrected axis has now run on hardware; the open item is
  the estimator, not the folding scheme. The old 2.743 mHa result used an invalid axis
  and remains historical.
- The ZNE point is not reproducible at the target scale: two identical `ibm_fez` runs
  differ by 5.112 mHa, and the paired difference CIs overlap in every run.
- Per-term ZNE is a diagnostic, not the primary estimator; full Aer/Pauli-Lindblad
  replay and PEC/CDR remain unbuilt.
- Chemical accuracy is a **difference claim**: a future pass needs a monotonic measured
  axis, `chi2_red ≤ 1.5`, a difference CI excluding zero, `|Δ| ≤ 1.6 mHa`, and a
  reproduced second job.

## Next experiment

**Do not spend more QPU on ZNE shots.** The pre-registered decision rule is triggered:
the model residual, not shot noise, is the floor. **6.200205 RU / 369 s remain** on the
current account for a different mitigation. The next hardware experiment should be a
calibrated non-unital noise-model/PEC or CDR pilot on the same 2-qubit ansatz, compared
against the paired scale-1 baseline, with the same `|Δ| ≤ 1.6 mHa`, difference-CI, and
reproduction gates.

## 2026-11-26 Phase-6 CDR pilot and reproduction (`ibm_fez`)

Following the post-run audit, the same-batch **QWC + live 4×4 asymmetric readout matrix +
Bonet-Monroig `Z2` symmetry verification + errors-in-variables ODR CDR** pipeline
(commit `44806e0f`, 43 unique circuits expanded via GCD uniform-shot replication to 272
1024-shot PUBs = `278,528` total shots per governed job: target `[65536, 16384, 16384]`,
12 energy-stratified near-Clifford training circuits at Neyman `[10240, 1024, 1024]`, and
4 computational-basis readout calibration circuits at `8192` shots each) ran twice on
real **`ibm_fez`** hardware (layout `[142, 143]`, `83 s / 1.375 RU` per job, `$0.00`
Open Plan; **`3.450861 RU / 203 s` remain**):

| Run | Hardware job | Raw unmit. (mHa) | Live 4×4 RO (mHa) | `Z2` Sym (mHa) | ODR CDR signed Δ (mHa) | CDR 95% CI | `χ²_red` (`p`) | `R²` | Diff vs raw (mHa) | G6 gates |
|---|---|---:|---:|---:|---:|---:|---:|---:|---:|---|
| C1 | `dc42qqanj2vs73dj9vv0` | +17.789 ±2.472 | +7.422 ±3.023 | +3.960 ±2.504 | **−0.195** | ±5.981 | **1.147** (0.322) | 0.9996 | **−17.983 ±5.487** (6.4σ) | **6/6 PASS** (40/40) |
| C2 | `dc42sleq4uuc73a7vo1g` | +15.073 ±2.443 | +4.224 ±3.032 | **+1.315** ±2.494 | **−5.554** | ±5.660 | **0.426** (0.935) | 0.9999 | **−20.627 ±5.144** (7.9σ) | **5/6 PASS** (38/40; chem fail) |

**Honest verdict:**
- **Estimator adequacy and statistical separation are solved and reproduced.** Unlike ZNE
  (`χ²_red = 3.68–4.81`, overlapping difference CIs), both CDR runs passed readout
  conditioning (`cond(M) = 1.032–1.035`), model adequacy (`χ²_red = 0.426–1.147`,
  `p = 0.322–0.935`, `R² ≥ 0.9996`), clean layout (`suspicious = false`), and strong
  paired separation from unmitigated raw (**6.4σ** and **7.9σ**). The two runs are
  statistically consistent across all four layers (`C2 − C1 = −5.359 ± 8.235 mHa`, `1.28σ`).
- **R2 chemical-accuracy reproduction is NOT yet achieved.** Pilot 1 (`C1`) cleared all
  40/40 checks (`|Δ_CDR| = 0.195 mHa`), but independent reproduction (`C2`) landed at
  `−5.554 mHa` (`1.92σ` below FCI, inside its own `±5.660 mHa` 95% CI). Pooled CDR is
  `−1.140328 Ha` (`−3.022 ± 4.111 mHa`, consistent with FCI at `1.44σ`).
- **Exact root cause localized:** CDR SE decomposition shows `target_se = 1.26 mHa`
  (`±2.46 mHa` 95% CI) while **`fit_se = 2.60–2.78 mHa`** (`±5.1–5.4 mHa` 95% CI)
  dominates the total interval (`±5.66–5.98 mHa`) because 9 of the 12 training circuits
  sat at high-energy superposition states (`se = 6–9.4 mHa` at `12,288` shots). Meanwhile,
  parameter-free **live readout + Bonet-Monroig `Z2` symmetry** alone reduced the raw
  error from `+16.42 ± 1.74 mHa` (pooled) to **`+2.63 ± 1.77 mHa`** (`+3.960` / `+1.315 mHa`).
- **No further QPU spend** without operator authorization; shrinking `fit_se` below `0.8 mHa`
  requires concentrating CDR training shots and states in the low-variance ground-state
  neighborhood (or pooling a lower-variance anchor) rather than repeating uniform-shot high-energy training circuits.

## Reading

- [[error-mitigation]] — technique family and the standing no-silent-claimability rule.
- [[quantum-advantage-realism]] — record the backend, compute the classical anchor,
  never claim advantage.
- [[remaining-work]] — residual operator evidence and the next-run gate.
