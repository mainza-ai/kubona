# Zero-noise extrapolation (ZNE): axis and claim gates

This page describes the **current** ZNE behaviour in the bridge and the peer-review
harness. It is an operator/developer reference, not a results log. For the 2026-11-25
hardware finding and its evidence, see
[`wiki/concepts/hardware-validation-r2-2026-11-25.md`](../wiki/concepts/hardware-validation-r2-2026-11-25.md).

## What a ZNE run does

1. The base ansatz is transpiled and measured once per Pauli term at scale 1.
2. For each requested scale `> 1`, the bridge folds gates with barrier-protected
   **local all-gate** folding (`_fold_local_all`). Both single-qubit and two-qubit
   gates participate. Global all-gate folding (`_fold_unitary`) remains available only
   as an explicit legacy helper and must never be mixed with local folding in one fit.
3. Each folded circuit records its measured axis metadata:
   `requested`, `actual_total_ratio`, `logical_1q`, `logical_2q`, `total_gates`, and
   `weighted_error_scale` (calibration-weighted, including readout).
4. The fit runs on the measured axis. The result reports the point, interval, model
   spread, `chi2_reduced`, dof, lack-of-fit p-value, per-term diagnostics, and the
   raw-minus-ZNE difference CI.

## Hard invariants

- **One mechanism per fit.** A local fit may not contain a globally folded circuit.
- **Strictly increasing total gates.** Scales that round to the same total-gate ratio,
  or that produce a non-increasing ratio, are refused **before dispatch**. Choose a
  scale set that is representable on the circuit (small circuits cannot realize every
  fractional scale).
- **Measured x, not nominal x.** The fit uses `actual_total_ratio`; the nominal
  requested scale is provenance only.
- **Archive enough to replay.** `counts_by_scale` retains every per-scale, per-term
  histogram. The supplement stages it as `<job>_by_scale.json` (or `.json.gz` above the
  gzip threshold) with a SHA-256 manifest entry.
- **No silent claimability.** A ZNE value is an extrapolation: `measured:false`,
  `claimable:false`, and `chemical_accuracy:false` unless every gate below passes.

## Result fields

| Field | Meaning |
|---|---|
| `fold_axis` | Per-scale `requested`, `actual_total_ratio`, 1Q/2Q/total gates, `weighted_error_scale` |
| `axis_valid` | Strictly increasing measured axis and one folding mechanism |
| `model_points` / `model_spread_ha` | Alternative estimator points and their spread |
| `chi2_reduced` / `lack_of_fit_p` | Fit adequacy; `chi2_red > 1.5` fails the adequacy gate |
| `model_adequate` | `axis_valid`, `chi2_red ≤ 1.5`, and model spread within the interval |
| `baseline_energy` / `baseline_uncertainty` | Scale-1 baseline from the same batch |
| `difference_ha` / `difference_ci95_ha` | Raw/ZNE difference and its 95% interval |
| `difference_significant` | `abs(difference) > difference_ci95_ha` |
| `per_term` / `per_term_energy` | Per-term ZNE diagnostic (identity term constant) |
| `counts_by_scale` | Per-scale, per-term histograms (supplement artifact) |

## Peer-review gates (G6)

The harness records these checks and fails closed rather than reporting a chemical-accuracy
result:

- `pillar1_h2_vqe_hw_zne_axis_valid` — measured axis is monotonic and single-mechanism.
- `pillar1_h2_vqe_hw_zne_model_adequate` — `chi2_red ≤ 1.5` and model spread within the interval.
- `pillar1_h2_vqe_hw_zne_difference_ci` — the raw/ZNE difference CI excludes zero.
- `pillar1_h2_vqe_hw_zne_chemical_accuracy` — `|Δ_ZNE| ≤ 1.6 mHa`.

The legacy `pillar1_h2_vqe_hw_zne_kandala2019` check (ZNE better than raw) remains, but it is
not sufficient on its own.

## Offline verification

The simulator rail exercises the same result shape without QPU spend:

```bash
python3 scripts/run_peer_reviewed_experiment.py \
  --mode simulator --tfim-zne \
  --report-out /tmp/simulator_zne.json
```

Inspect `fold_axis`, `axis_valid`, `model_adequate`, `difference_ci95_mha`, and
`difference_significant` before scheduling hardware. A hardware run additionally requires
a fresh calibration, a valid layout (`layout_quality.suspicious == false`), and a spend
mandate that covers the full folded batch.

## Hardware status

The 2026-11-25 Phase-4B run consumed the prior 1.4 RU. A new IBM Quantum Open Plan
account is configured (10/10 RU, 600 s remaining, verified 2026-11-25). The corrected
all-gate axis has **not** been exercised on a QPU. R2 remains open; the prior 2.743 mHa
result used a mixed axis and is historical evidence, not a chemical-accuracy claim.
