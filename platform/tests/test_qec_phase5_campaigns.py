"""Phase 5 Campaign A & Campaign B execution and publication-figure tests."""

from __future__ import annotations

import numpy as np
import pytest

pytest.importorskip("stim")
pytest.importorskip("pymatching")
pytest.importorskip("ldpc")

from kubona.qec import (
    run_campaign_a_catalog_benchmark,
    run_campaign_b_schedule_search,
    si1000_noise,
)


def test_campaign_a_catalog_benchmark_under_si1000_and_heron_r2():
    """Campaign A runs Bravyi BB catalog + surface baselines under SI1000 & Heron-r2 noise."""
    rep = run_campaign_a_catalog_benchmark(
        max_total_qubits=150,
        shots=100,
        rounds=2,
        si1000_p=0.003,
        seed=7,
    )
    assert rep["campaign"] == "campaign_a_catalog_benchmark"
    assert len(rep["artifact_sha256"]) == 64
    assert len(rep["noise_models"]) == 2
    assert rep["noise_models"][1]["is_held_out"] is True
    assert rep["noise_models"][1]["calibration_snapshot_id"] == "cal_8f65e57d6f49"
    # Includes surface_d3, surface_d5, and bravyi2024_bb_72_12_6 (144 total qubits <= 150) x 2 noise models
    names = {r["code_name"] for r in rep["rows"]}
    assert "surface_d3" in names
    assert "bravyi2024_bb_72_12_6" in names
    for row in rep["rows"]:
        assert row["result_version"] == "experiment-result-v1"
        assert row["kind"] == "qec_memory_sim"
        assert row["ci_low"] <= row["ler"] <= row["ci_high"]


def test_campaign_b_schedule_search_preserves_or_improves_d_circ_and_depth():
    """Campaign B searches collision-free CNOT schedules and verifies d_circ + LER."""
    H = np.array(
        [
            [1, 0, 1, 0, 1, 0, 1],
            [0, 1, 1, 0, 0, 1, 1],
            [0, 0, 0, 1, 1, 1, 1],
        ],
        dtype=np.uint8,
    )
    out = run_campaign_b_schedule_search(
        H,
        H,
        noise=si1000_noise(0.003),
        rounds=2,
        trials=8,
        shots=100,
        seed=7,
    )
    assert out["campaign"] == "campaign_b_schedule_search"
    assert out["trials"] == 8
    assert out["best_schedule"]["d_circ"] >= out["baseline_d_circ"]
    assert out["best_schedule"]["cnot_depth"] <= out["baseline_depth"] or out["best_schedule"]["d_circ"] > out["baseline_d_circ"]
    assert out["best_sim"]["kind"] == "qec_memory_sim"
    assert out["best_sim"]["success"] is True
