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


def test_p1_5_surface_code_suppression_d5_below_d3_non_overlapping_ci():
    """P1-5: at 10,000 shots and p=0.003, rotated surface d=5 strictly suppresses LER below d=3 with separated 95% CIs."""
    rep = run_campaign_a_catalog_benchmark(
        max_total_qubits=100,  # surface_d3 + surface_d5 only (excludes bb_72 at 144 qubits)
        shots=10000,
        rounds=2,
        si1000_p=0.003,
        seed=7,
    )
    si_rows = [r for r in rep["rows"] if not (r.get("noise") or {}).get("is_held_out")]
    d3 = next(r for r in si_rows if r["code_name"] == "surface_d3")
    d5 = next(r for r in si_rows if r["code_name"] == "surface_d5")
    assert d5["ler"] < d3["ler"]
    assert d5["ci_high"] < d3["ci_low"]
    assert d5["ci_separated_below_d3"] is True


def test_p0_2_ledger_calibration_lookup_and_fail_closed(tmp_path):
    """P0-2: load_heron_calibration_from_ledger extracts exact values and refuses unknown snapshots."""
    import json
    import sqlite3

    from kubona.qec import heron_calibration_noise, load_heron_calibration_from_ledger

    db = tmp_path / "ledger.db"
    conn = sqlite3.connect(db)
    conn.execute("CREATE TABLE jobs (id TEXT, created_at TEXT, result_json TEXT)")
    conn.execute(
        "INSERT INTO jobs VALUES (?, ?, ?)",
        (
            "job1",
            "2026-11-24T00:00:00Z",
            json.dumps({
                "calibration": {
                    "calibration_snapshot_id": "cal_8f65e57d6f49",
                    "backend": "ibm_kingston",
                    "t1_us": 267.172889,
                    "t2_us": 142.351605,
                    "two_qubit_error": 0.0018203,
                    "single_qubit_error": 0.00024423,
                    "readout_error": 0.011475,
                }
            }),
        ),
    )
    conn.commit()
    conn.close()

    cal = load_heron_calibration_from_ledger("cal_8f65e57d6f49", db)
    nm = heron_calibration_noise(cal)
    assert nm.calibration_snapshot_id == "cal_8f65e57d6f49"
    assert abs(nm.p_2q - 0.0018203) < 1e-9
    assert abs(nm.p_meas - 0.011475) < 1e-9

    with pytest.raises(ValueError, match="unknown calibration snapshot"):
        load_heron_calibration_from_ledger("cal_does_not_exist", db)

    with pytest.raises(ValueError, match="missing required field"):
        heron_calibration_noise({"calibration_snapshot_id": "cal_incomplete"})


def test_p1_6_qec_manuscript_table_renders_metrics_not_dashes():
    """P1-6: _results_table renders [[n,k,d]] / LER CI for QEC rows instead of '--'."""
    import sys
    from pathlib import Path

    bridge_dir = str(Path(__file__).resolve().parents[1] / "bridge")
    if bridge_dir not in sys.path:
        sys.path.insert(0, bridge_dir)
    from manuscript_ops import _results_table

    tex = _results_table([
        {
            "job_id": "aaaaaaaa-1111",
            "kind": "qec_code_eval",
            "n": 72,
            "k": 12,
            "d": 6,
            "d_kind": "proved",
            "fom": 6.0,
            "backend": "highspy",
        },
        {
            "job_id": "bbbbbbbb-2222",
            "kind": "qec_memory_sim",
            "ler": 0.0046,
            "ci_low": 0.0037,
            "ci_high": 0.0056,
            "d_circ": 3,
            "decoder": "pymatching",
            "decoder_latency_us": 12.4,
            "backend": "stim",
        },
    ])
    assert "[[72,12,6]]" in tex
    assert "FOM 6.00" in tex
    assert "PROVED" in tex
    assert "LER 0.0046 [0.0037,0.0056]" in tex
    assert "d\\_circ=3" in tex
    assert "pymatching (12.4 us)" in tex
