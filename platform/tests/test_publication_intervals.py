"""R2 + R3 tests: QAOA Wilson 95% ratio CIs, overlapping-CI guard, and explicit counts_status."""

from __future__ import annotations

import sys
from pathlib import Path

BRIDGE_DIR = str(Path(__file__).resolve().parents[1] / "bridge")
if BRIDGE_DIR not in sys.path:
    sys.path.insert(0, BRIDGE_DIR)

import manuscript_datasheet  # noqa: E402


def test_r2_wilson_ci_and_overlap_guard():
    lo, hi = manuscript_datasheet.wilson_ci95(0.85, 1024)
    assert 0.80 < lo < 0.85 < hi < 0.90

    # Overlapping intervals MUST be flagged as overlapping
    assert manuscript_datasheet.ci_intervals_overlap(0.80, 0.86, 0.84, 0.90) is True
    assert manuscript_datasheet.ci_intervals_overlap(0.70, 0.78, 0.82, 0.90) is False


def test_r2_r3_uncertainty_table_renders_qaoa_wilson_and_not_retained_status():
    rows = [
        {
            "job_id": "qaoa_hw_1",
            "kind": "qaoa_hardware",
            "hardware": True,
            "shots": 1024,
            "measured_cut": 8.5,
            "optimal_cut": 10.0,
            "approximation_ratio": 0.85,
            "counts_status": "retained",
        },
        {
            "job_id": "spin_hw_1",
            "kind": "spin_chain_hardware",
            "hardware": True,
            "shots": 1024,
            "energy": -2.501347,
            "exact_energy": -2.500000,
            "delta_mha": 1.347,
            "counts_status": "not_retained",
        },
    ]
    tex = manuscript_datasheet.build_zne_and_uncertainty_section(rows)
    assert "(Wilson)" in tex
    assert "0.8500" in tex
    assert "point est. (no counts)" in tex
