"""R2 + R3 tests: count-derived QAOA ratio CIs, and explicit counts_status."""

from __future__ import annotations

import sys
from pathlib import Path

BRIDGE_DIR = str(Path(__file__).resolve().parents[1] / "bridge")
if BRIDGE_DIR not in sys.path:
    sys.path.insert(0, BRIDGE_DIR)

import manuscript_datasheet  # noqa: E402


def test_r2_ratio_interval_is_rendered_only_when_recorded():
    rows = [
        {
            "job_id": "qaoa_hw_1",
            "kind": "qaoa_hardware",
            "hardware": True,
            "shots": 1024,
            "measured_cut": 8.5,
            "optimal_cut": 10.0,
            "approximation_ratio": 0.85,
            "ratio_ci95": [0.812345, 0.887655],
            "ratio_ci_method": "normal_approx_95",
            "counts_status": "retained",
        },
        {
            "job_id": "qaoa_hw_2",
            "kind": "qaoa_hardware",
            "hardware": True,
            "shots": 1024,
            "measured_cut": 7.5,
            "optimal_cut": 10.0,
            "approximation_ratio": 0.75,
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
    # Recorded interval is printed with its endpoints; an unrecorded ratio is a
    # point estimate, never a fabricated interval.
    assert "[0.812, 0.888]" in tex
    assert "0.7500" in tex
    assert "point estimate" in tex
    assert "point est. (no counts)" in tex
    # The invalid proportion-on-ratio estimator must not reappear.
    assert "Wilson" not in tex
