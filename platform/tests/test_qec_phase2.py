"""Phase 2 QEC circuit-level memory simulator, Heron-r2 noise, and EBP+OSD tests."""

from __future__ import annotations

import numpy as np
import pytest

pytest.importorskip("stim")
pytest.importorskip("pymatching")
pytest.importorskip("ldpc")

from kubona.qec import (
    benchmark_css_vs_surface,
    build_bb_code,
    build_css_memory_circuit,
    build_surface_code_memory_circuit,
    heron_calibration_noise,
    si1000_noise,
    simulate_memory_circuit,
    tune_ebposd_weights,
    uniform_depolarizing_noise,
    verify_circuit_distance,
)


def _steane_H() -> np.ndarray:
    return np.array(
        [
            [1, 0, 1, 0, 1, 0, 1],
            [0, 1, 1, 0, 0, 1, 1],
            [0, 0, 0, 1, 1, 1, 1],
        ],
        dtype=np.uint8,
    )


def test_surface_code_error_suppression_under_si1000():
    """Rotated surface code d=5 beats d=3 below threshold under SI1000 with PyMatching."""
    nm = si1000_noise(0.002)
    c3, m3 = build_surface_code_memory_circuit(3, rounds=3, noise=nm)
    c5, m5 = build_surface_code_memory_circuit(5, rounds=3, noise=nm)

    r3 = simulate_memory_circuit(c3, shots=4000, rounds=3, num_logicals=1, decoder="pymatching", seed=11, code_name="surface_d3", noise_meta=m3["noise"])
    r5 = simulate_memory_circuit(c5, shots=4000, rounds=3, num_logicals=1, decoder="pymatching", seed=11, code_name="surface_d5", noise_meta=m5["noise"])

    assert r3["result_version"] == "experiment-result-v1"
    assert r3["kind"] == "qec_memory_sim"
    assert r3["d_circ"] == 3
    assert r5["d_circ"] == 5
    assert r5["ler"] < r3["ler"]
    assert r3["decoder_latency_us"] >= 0.0
    assert r3["ci_low"] <= r3["ler"] <= r3["ci_high"]


def test_steane_circuit_under_heron_r2_calibration_noise_and_baseline():
    """G7: HeronCalibrationNoiseModel built from live cal_<sha256> snapshot drives held-out benchmark."""
    cal_snap = {
        "backend": "ibm_kingston",
        "calibration_snapshot_id": "cal_8f65e57d6f49",
        "median_2q_error": 0.0028,
        "median_1q_error": 0.00025,
        "median_readout_error": 0.012,
        "median_t1_us": 240.0,
        "median_t2_us": 150.0,
    }
    nm = heron_calibration_noise(cal_snap, cycle_us=1.0, is_held_out=True)
    assert nm.is_held_out is True
    assert nm.calibration_snapshot_id == "cal_8f65e57d6f49"
    assert 0.0 < nm.p_idle < 0.02

    H = _steane_H()
    cmp = benchmark_css_vs_surface(
        H,
        H,
        noise=nm,
        shots=250,
        rounds=2,
        surface_distance=3,
        seed=7,
        code_name="steane_7_1_3",
        decoder="bposd",
    )
    cand = cmp["candidate"]
    base = cmp["surface_baseline"]
    assert cand["kind"] == "qec_memory_sim"
    assert cand["d_circ"] is not None and cand["d_circ"] >= 1
    assert cand["noise"]["calibration_snapshot_id"] == "cal_8f65e57d6f49"
    assert base["decoder"] == "pymatching"
    assert base["d_circ"] == 3


def test_bb_72_12_6_memory_circuit_synthesis_and_bposd_decode():
    """BB [[72, 12, 6]] synthesizes a valid Stim Z-memory circuit and decodes via BP-OSD."""
    hx, hz = build_bb_code(6, 6, [(3, 0), (0, 1), (0, 2)], [(0, 3), (1, 0), (2, 0)])
    nm = uniform_depolarizing_noise(0.001)
    circ, meta = build_css_memory_circuit(hx, hz, rounds=2, noise=nm)
    assert meta["n_data"] == 72
    assert meta["k"] == 12
    assert meta["total_qubits"] == 144

    # Verify circuit compiles a valid DetectorErrorModel and runs BP-OSD decoding
    res = simulate_memory_circuit(
        circ,
        shots=40,
        rounds=2,
        num_logicals=12,
        decoder="bposd",
        seed=7,
        code_name="bb_72_12_6",
        noise_meta=meta["noise"],
    )
    assert res["success"] is True
    assert res["shots"] == 40
    assert 0.0 <= res["ler"] <= 1.0
    assert res["decoder"] == "bposd"


def test_ebposd_5_iter_weight_tuning_runs_and_reports_latency():
    """S3: Evolutionary BP+OSD (5 iterations) tunes edge-degree weights on Steane DEM."""
    H = _steane_H()
    nm = uniform_depolarizing_noise(0.01)
    circ, _ = build_css_memory_circuit(H, H, rounds=2, noise=nm)
    d_info = verify_circuit_distance(circ)
    assert d_info["verified"] is True

    dem = circ.detector_error_model(decompose_errors=False)
    dets, obs = circ.compile_detector_sampler(seed=9).sample(shots=60, separate_observables=True)
    obs = np.asarray(obs, dtype=np.uint8).reshape(60, -1)

    tuned = tune_ebposd_weights(dem, dets, obs, max_iter=5, osd_order=2, de_maxiter=2, de_popsize=3, seed=7)
    assert tuned["max_iter"] == 5
    assert len(tuned["degree_weights"]) >= 1
    assert 0.0 <= tuned["train_ler"] <= 1.0
    assert tuned["decoder_latency_us"] >= 0.0
