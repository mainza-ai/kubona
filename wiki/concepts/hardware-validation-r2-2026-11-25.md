---
type: concept
name: Hardware Validation R2 — ZNE Axis Audit
date: 2026-11-25
updated: 2026-11-25
tags: [hardware, zne, validation, error-mitigation, reproducibility]
sources: [source-architecture-paper]
status: tracked
---

# Hardware Validation R2 — ZNE Axis Audit

**Prototype register.** This page records what the 2026-11-25 hardware validation and
offline replay established about R2 (the H₂ ZNE chemical-accuracy claim). It is a
prototype fact, not a design decision. The implementation cited here is at
[`mainza-ai/kubona`](https://github.com/mainza-ai/kubona) commit `a2640825`; the
pre-audit result remains in the ledger as historical, not as a valid ZNE claim.

## What R2 was meant to show

R2 asked whether Zero-Noise Extrapolation on `ibm_fez` could bring the H₂ VQE energy
inside chemical accuracy (`|Δ| ≤ 1.6 mHa` from the PySCF FCI anchor) and separate the
mitigated result from the raw one. The Phase-4B run reported a point estimate of
**2.743 mHa** with a **±11.037 mHa** interval, against a raw **9.999 mHa (±6.803)**.
R2 was therefore **not achieved**.

## The primary cause: the scale axis mixed two folding mechanisms

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

## Why more shots alone cannot close R2

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

Measured all-gate axis on the same H₂ ansatz (strictly increasing):

| requested | 1Q | 2Q | total | actual total ratio | weighted error scale |
|---|---:|---:|---:|---:|---:|
| 1.0 | 8 | 2 | 10 | 1.00 | 1.00 |
| 1.5 | 10 | 4 | 14 | 1.40 | 1.72 |
| 2.0 | 16 | 4 | 20 | 2.00 | 2.00 |
| 3.0 | 24 | 6 | 30 | 3.00 | 3.00 |

## What is not established

- The corrected axis has **not** run on a QPU. The 1.4 RU available at the start of the
  Phase-4B window was consumed by that subset; a new IBM Quantum Open Plan account is now
  configured (10/10 RU, 600 s remaining, verified 2026-11-25), so the corrected-axis run
  is the next hardware step.
- R2 is **not retroactively fixed**. The old 2.743 mHa result used an invalid axis and
  must be replaced by a new, pre-registered run.
- Per-term ZNE is a diagnostic, not yet the primary estimator; full Aer/Pauli-Lindblad
  replay and PEC/CDR remain unbuilt.
- Chemical accuracy is a **difference claim**: a future pass needs a monotonic measured
  axis, `chi2_red ≤ 1.5`, a difference CI excluding zero, `|Δ| ≤ 1.6 mHa`, and a
  reproduced second job.

## Reading

- [[error-mitigation]] — technique family and the standing no-silent-claimability rule.
- [[quantum-advantage-realism]] — record the backend, compute the classical anchor,
  never claim advantage.
- [[remaining-work]] — residual operator evidence and the next-run gate.
