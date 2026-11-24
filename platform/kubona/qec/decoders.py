"""QEC decoders with latency instrumentation and EBP+OSD weight tuning (Phase 2 / S3).

Supports:
- PyMatching (MWPM) for graphlike codes (rotated surface codes)
- ldpc BP-OSD for general qLDPC / Bivariate Bicycle codes
- Evolutionary BP+OSD (EBP+OSD): differential-evolution tuning of degree-indexed
  BP channel weights for low-iteration (5 vs 32) decoding (Kwak et al., S3).
"""

from __future__ import annotations

import time
from typing import Any, Tuple

import numpy as np
import pymatching
import stim
from ldpc import BpOsdDecoder
from scipy.optimize import differential_evolution


def dem_to_matrices(dem: stim.DetectorErrorModel) -> Tuple[np.ndarray, np.ndarray, np.ndarray]:
    """Convert a decomposed/flat Stim DetectorErrorModel into (H_dem, obs_matrix, priors)."""
    flat = dem.flattened()
    num_det = flat.num_detectors
    num_obs = flat.num_observables

    # Aggregate identical error mechanisms
    mech_probs: dict[Tuple[Tuple[int, ...], Tuple[int, ...]], float] = {}
    for inst in flat:
        if inst.type != "error":
            continue
        p = float(inst.args_copy()[0])
        if p <= 0.0:
            continue
        dets = sorted({t.val for t in inst.targets_copy() if t.is_relative_detector_id()})
        obs = sorted({t.val for t in inst.targets_copy() if t.is_logical_observable_id()})
        if not dets:
            continue
        key = (tuple(dets), tuple(obs))
        if key in mech_probs:
            p0 = mech_probs[key]
            mech_probs[key] = p0 * (1.0 - p) + (1.0 - p0) * p
        else:
            mech_probs[key] = p

    n_err = len(mech_probs)
    H = np.zeros((num_det, n_err), dtype=np.uint8)
    obs_mat = np.zeros((num_obs, n_err), dtype=np.uint8)
    priors = np.zeros(n_err, dtype=np.float64)
    for col, ((dets, obs), p) in enumerate(mech_probs.items()):
        for d in dets:
            H[d, col] = 1
        for o in obs:
            obs_mat[o, col] = 1
        priors[col] = min(0.499, max(1e-9, p))
    return H, obs_mat, priors


def decode_pymatching(
    dem: stim.DetectorErrorModel,
    dets: np.ndarray,
) -> Tuple[np.ndarray, float]:
    """Decode detection events using PyMatching. Returns (pred_obs, latency_us_per_shot)."""
    matcher = pymatching.Matching.from_detector_error_model(dem)
    t0 = time.perf_counter()
    preds = matcher.decode_batch(dets)
    dt_us = ((time.perf_counter() - t0) * 1e6) / max(1, dets.shape[0])
    return np.asarray(preds, dtype=np.uint8), round(dt_us, 3)


def decode_bposd(
    dem: stim.DetectorErrorModel,
    dets: np.ndarray,
    *,
    max_iter: int = 32,
    osd_order: int = 4,
    degree_weights: dict[int, float] | None = None,
) -> Tuple[np.ndarray, float]:
    """Decode detection events via ldpc BP-OSD (optionally with EBP+OSD degree weights)."""
    H, obs_mat, priors = dem_to_matrices(dem)
    if H.shape[1] == 0:
        return np.zeros((dets.shape[0], obs_mat.shape[0]), dtype=np.uint8), 0.0

    eff_priors = priors.copy()
    if degree_weights:
        col_degs = H.sum(axis=0)
        for col in range(H.shape[1]):
            deg = int(col_degs[col])
            w = float(degree_weights.get(deg, 1.0))
            eff_priors[col] = min(0.499, max(1e-9, priors[col] * w))

    dec = BpOsdDecoder(
        H,
        channel_probs=eff_priors,
        max_iter=int(max_iter),
        bp_method="minimum_sum",
        osd_method="osd_cs",
        osd_order=int(osd_order),
    )
    preds = np.zeros((dets.shape[0], obs_mat.shape[0]), dtype=np.uint8)
    t0 = time.perf_counter()
    for i in range(dets.shape[0]):
        syn = np.asarray(dets[i], dtype=np.uint8)
        if not np.any(syn):
            continue
        corr = np.asarray(dec.decode(syn), dtype=np.uint8) & 1
        preds[i] = (obs_mat @ corr) % 2
    dt_us = ((time.perf_counter() - t0) * 1e6) / max(1, dets.shape[0])
    return preds, round(dt_us, 3)


def tune_ebposd_weights(
    dem: stim.DetectorErrorModel,
    train_dets: np.ndarray,
    train_obs: np.ndarray,
    *,
    max_iter: int = 5,
    osd_order: int = 2,
    de_maxiter: int = 4,
    de_popsize: int = 4,
    seed: int = 7,
) -> dict[str, Any]:
    """Tune edge-degree-indexed BP weights via differential evolution (EBP+OSD, S3).

    Uses ``max_iter=5`` (vs standard 32) and returns learned degree weights +
    measured logical error rate and latency.
    """
    H, _, _ = dem_to_matrices(dem)
    unique_degs = sorted({int(d) for d in H.sum(axis=0) if d > 0})
    if not unique_degs or train_dets.shape[0] == 0:
        return {"degree_weights": {}, "train_ler": 0.0, "max_iter": max_iter}

    bounds = [(0.25, 4.0) for _ in unique_degs]

    def _objective(vec: np.ndarray) -> float:
        weights = {deg: float(w) for deg, w in zip(unique_degs, vec)}
        preds, _ = decode_bposd(
            dem,
            train_dets,
            max_iter=max_iter,
            osd_order=osd_order,
            degree_weights=weights,
        )
        mismatches = np.any(preds != train_obs, axis=1)
        return float(np.mean(mismatches))

    res = differential_evolution(
        _objective,
        bounds=bounds,
        maxiter=de_maxiter,
        popsize=de_popsize,
        seed=seed,
        polish=False,
    )
    best_weights = {deg: round(float(w), 4) for deg, w in zip(unique_degs, res.x)}
    preds, lat_us = decode_bposd(
        dem,
        train_dets,
        max_iter=max_iter,
        osd_order=osd_order,
        degree_weights=best_weights,
    )
    ler = float(np.mean(np.any(preds != train_obs, axis=1)))
    return {
        "degree_weights": best_weights,
        "train_ler": ler,
        "decoder_latency_us": lat_us,
        "max_iter": max_iter,
        "osd_order": osd_order,
    }
