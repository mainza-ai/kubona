"""Circuit-level QEC memory simulator with Wilson 95% CIs and baseline comparison (Phase 2)."""

from __future__ import annotations

import math
from typing import Any

import numpy as np
import stim

from kubona.qec.circuits import (
    CircuitNoiseModel,
    build_css_memory_circuit,
    build_surface_code_memory_circuit,
    verify_circuit_distance,
)
from kubona.qec.decoders import decode_bposd, decode_pymatching


def wilson_ci_95(errors: int, shots: int) -> tuple[float, float]:
    """Exact 95% Wilson score interval for a binomial proportion."""
    if shots <= 0:
        return 0.0, 1.0
    z = 1.959963984540054
    p = errors / shots
    denom = 1.0 + (z * z) / shots
    center = (p + (z * z) / (2.0 * shots)) / denom
    half = (z * math.sqrt((p * (1.0 - p) + (z * z) / (4.0 * shots)) / shots)) / denom
    return max(0.0, center - half), min(1.0, center + half)


def simulate_memory_circuit(
    circuit: stim.Circuit,
    *,
    shots: int = 1000,
    rounds: int = 3,
    num_logicals: int = 1,
    decoder: str = "auto",
    seed: int = 7,
    code_name: str = "qec_circuit",
    noise_meta: dict[str, Any] | None = None,
    degree_weights: dict[int, float] | None = None,
    max_iter: int = 32,
) -> dict[str, Any]:
    """Sample a Stim memory circuit, decode with PyMatching or BP-OSD, and emit experiment-result-v1."""
    if shots < 1:
        raise ValueError(f"shots must be >= 1, got {shots}")
    sampler = circuit.compile_detector_sampler(seed=seed)
    dets, obs = sampler.sample(shots=shots, separate_observables=True)
    obs = np.asarray(obs, dtype=np.uint8)
    if obs.ndim == 1:
        obs = obs.reshape(-1, 1)

    chosen = decoder
    if chosen == "auto":
        try:
            dem = circuit.detector_error_model(decompose_errors=True)
            chosen = "pymatching"
        except Exception:
            dem = circuit.detector_error_model(decompose_errors=False)
            chosen = "bposd"
    elif chosen == "pymatching":
        dem = circuit.detector_error_model(decompose_errors=True)
    else:
        dem = circuit.detector_error_model(decompose_errors=False)

    if chosen == "pymatching":
        preds, lat_us = decode_pymatching(dem, dets)
    elif chosen in ("bposd", "ebposd"):
        eff_iter = 5 if (chosen == "ebposd" and max_iter == 32) else max_iter
        preds, lat_us = decode_bposd(dem, dets, max_iter=eff_iter, degree_weights=degree_weights)
    else:
        raise ValueError(f"unknown decoder {decoder!r}: use auto, pymatching, bposd, or ebposd")

    if preds.ndim == 1:
        preds = preds.reshape(-1, 1)
    logical_failures = np.any(preds != obs, axis=1)
    n_errors = int(np.sum(logical_failures))
    ler = n_errors / shots
    ci_lo, ci_hi = wilson_ci_95(n_errors, shots)
    ler_per_round = 1.0 - ((1.0 - min(0.999999, ler)) ** (1.0 / max(1, rounds)))
    ler_per_logical_per_round = ler_per_round / max(1, num_logicals)
    d_circ_info = verify_circuit_distance(circuit)

    return {
        "result_version": "experiment-result-v1",
        "success": True,
        "kind": "qec_memory_sim",
        "code_name": code_name,
        "total_qubits": int(circuit.num_qubits),
        "k": int(num_logicals),
        "rounds": int(rounds),
        "shots": int(shots),
        "errors": n_errors,
        "ler": round(ler, 6),
        "ci_low": round(ci_lo, 6),
        "ci_high": round(ci_hi, 6),
        "ler_per_round": round(ler_per_round, 6),
        "ler_per_logical_per_round": round(ler_per_logical_per_round, 6),
        "decoder": chosen,
        "decoder_latency_us": lat_us,
        "d_circ": d_circ_info.get("d_circ"),
        "d_circ_method": d_circ_info.get("method"),
        "noise": noise_meta or {},
        "measured": True,
        "hardware": False,
    }


def benchmark_css_vs_surface(
    hx: np.ndarray,
    hz: np.ndarray,
    *,
    noise: CircuitNoiseModel,
    shots: int = 1000,
    rounds: int = 3,
    surface_distance: int = 3,
    seed: int = 7,
    code_name: str = "css_candidate",
    decoder: str = "bposd",
) -> dict[str, Any]:
    """Compare a CSS/BB code against a rotated surface-code baseline under identical noise."""
    c_css, meta_css = build_css_memory_circuit(hx, hz, rounds=rounds, noise=noise)
    res_css = simulate_memory_circuit(
        c_css,
        shots=shots,
        rounds=rounds,
        num_logicals=meta_css["k"],
        decoder=decoder,
        seed=seed,
        code_name=code_name,
        noise_meta=meta_css["noise"],
    )

    c_surf, meta_surf = build_surface_code_memory_circuit(surface_distance, rounds=rounds, noise=noise)
    res_surf = simulate_memory_circuit(
        c_surf,
        shots=shots,
        rounds=rounds,
        num_logicals=1,
        decoder="pymatching",
        seed=seed,
        code_name=f"surface_d{surface_distance}",
        noise_meta=meta_surf["noise"],
    )

    non_overlapping_win = bool(res_css["ci_high"] < res_surf["ci_low"])
    return {
        "candidate": res_css,
        "surface_baseline": res_surf,
        "noise": noise.to_dict(),
        "non_overlapping_win": non_overlapping_win,
    }
