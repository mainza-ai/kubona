"""Unit tests for the peer-reviewed hardware harness ZNE/CDR validation & G6 gates."""

from __future__ import annotations

import argparse
import sys
from pathlib import Path

import pytest

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT / "scripts"))

import run_peer_reviewed_experiment as harness  # noqa: E402


def test_zne_scales_accepts_fractional_and_integer():
    assert harness._parse_zne_scales("1,1.5,2,3,4") == [1.0, 1.5, 2.0, 3.0, 4.0]


@pytest.mark.parametrize("raw", ["1,1.5,1.5,2", "1,2", "0.5,1,2", "1,2,3,4,5,6,7"])
def test_zne_scales_rejects_invalid_axis(raw):
    with pytest.raises(argparse.ArgumentTypeError):
        harness._parse_zne_scales(raw)


def test_cdr_cli_parsers_validate_bounds():
    assert harness._parse_cdr_training_circuits("12") == 12
    assert harness._parse_target_shots("65536") == 65536
    for bad in ("5", "33", "abc"):
        with pytest.raises(argparse.ArgumentTypeError):
            harness._parse_cdr_training_circuits(bad)
    for bad in ("512", "200000", "xyz"):
        with pytest.raises(argparse.ArgumentTypeError):
            harness._parse_target_shots(bad)


def test_cdr_g6_gates_fail_closed_and_pass_on_adequate_separated_result():
    fci = -1.137306
    good = {
        "success": True,
        "energy": -1.136500,
        "layout_quality": {"suspicious": False, "max_readout_error": 0.012},
        "mitigation": {
            "method": "CDR",
            "model_adequate": True,
            "chi2_reduced": 0.85,
            "r_squared": 0.998,
            "loo_cv_mha": 1.8,
            "difference_ha": -0.018,
            "difference_ci95_mha": 3.2,
            "difference_significant": True,
            "readout_calibration": {"applied": True, "method": "live_assignment_matrix", "condition_number": 1.15},
        },
    }
    gates = harness.evaluate_cdr_g6_gates(good, fci, live_readout_cal=True)
    assert all(g["passed"] for g in gates.values()), gates

    # Fail closed if chi2 > 1.5, difference not significant, or layout suspicious
    bad_chi2 = {**good, "mitigation": {**good["mitigation"], "chi2_reduced": 2.4}}
    assert harness.evaluate_cdr_g6_gates(bad_chi2, fci)["model_adequate"]["passed"] is False
    assert harness.evaluate_cdr_g6_gates(bad_chi2, fci)["all_gates"]["passed"] is False

    bad_ro = {
        **good,
        "mitigation": {
            **good["mitigation"],
            "readout_calibration": {"applied": True, "method": "live_assignment_matrix", "condition_number": 60.0},
        },
    }
    assert harness.evaluate_cdr_g6_gates(bad_ro, fci)["readout_cal"]["passed"] is False
    assert harness.evaluate_cdr_g6_gates(bad_ro, fci)["all_gates"]["passed"] is False

    bad_layout = {**good, "layout_quality": {"suspicious": True, "max_readout_error": 0.08}}
    assert harness.evaluate_cdr_g6_gates(bad_layout, fci)["layout_clean"]["passed"] is False
    assert harness.evaluate_cdr_g6_gates(bad_layout, fci)["all_gates"]["passed"] is False
