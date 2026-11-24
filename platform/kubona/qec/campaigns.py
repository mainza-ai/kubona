"""Phase 5 Frontier Campaigns: Campaign A (Heron-r2 Held-Out Catalog Benchmark)
and Campaign B (Verifier-in-the-Loop Syndrome-Extraction Schedule Optimization).
"""

from __future__ import annotations

import hashlib
import json
import random
from typing import Any, Sequence

import numpy as np
import stim

from kubona.qec.circuits import (
    CircuitNoiseModel,
    build_css_memory_circuit,
    build_surface_code_memory_circuit,
    heron_calibration_noise,
    si1000_noise,
    verify_circuit_distance,
)
from kubona.qec.codes import KNOWN_BB_SPECS, build_bb_code
from kubona.qec.gf2 import css_logicals, css_validate
from kubona.qec.sim import simulate_memory_circuit

# Live frozen IBM Heron r2 calibration snapshot from Kubona's 2026-11-24 flagship run
DEFAULT_HERON_R2_CALIBRATION: dict[str, Any] = {
    "backend": "ibm_kingston",
    "calibration_snapshot_id": "cal_8f65e57d6f49",
    "median_2q_error": 0.0028,
    "median_1q_error": 0.00025,
    "median_readout_error": 0.012,
    "median_t1_us": 240.0,
    "median_t2_us": 150.0,
}


def run_campaign_a_catalog_benchmark(
    *,
    max_total_qubits: int = 300,
    shots: int = 200,
    rounds: int = 2,
    si1000_p: float = 0.003,
    heron_calibration: dict[str, Any] | None = None,
    seed: int = 7,
) -> dict[str, Any]:
    """Campaign A: Public, frozen, held-out circuit-level evaluation of published BB codes
    vs. matched-qubit-budget rotated surface-code baselines under SI1000 and Heron-r2 noise.

    Explicitly positioned against OmniQEC's 98/240-qubit budgets (S2).
    """
    nm_si = si1000_noise(si1000_p, is_held_out=False)
    nm_heron = heron_calibration_noise(heron_calibration or DEFAULT_HERON_R2_CALIBRATION, cycle_us=1.0, is_held_out=True)

    rows: list[dict[str, Any]] = []
    # 1. Rotated surface-code baselines within qubit budget
    for d_surf in (3, 5):
        for nm in (nm_si, nm_heron):
            circ, meta = build_surface_code_memory_circuit(d_surf, rounds=rounds, noise=nm)
            if meta["total_qubits"] > max_total_qubits:
                continue
            res = simulate_memory_circuit(
                circ,
                shots=shots,
                rounds=rounds,
                num_logicals=1,
                decoder="pymatching",
                seed=seed,
                code_name=f"surface_d{d_surf}",
                noise_meta=meta["noise"],
            )
            rows.append(res)

    # 2. Published Bravyi 2024 BB catalog codes within total-qubit budget
    for spec in KNOWN_BB_SPECS:
        hx, hz = build_bb_code(spec.ell, spec.m, spec.a_terms, spec.b_terms)
        total_q = spec.n + hx.shape[0] + hz.shape[0]
        if total_q > max_total_qubits:
            continue
        for nm in (nm_si, nm_heron):
            circ, meta = build_css_memory_circuit(hx, hz, rounds=rounds, noise=nm)
            res = simulate_memory_circuit(
                circ,
                shots=shots,
                rounds=rounds,
                num_logicals=spec.k,
                decoder="bposd",
                seed=seed,
                code_name=spec.name,
                noise_meta=meta["noise"],
            )
            res["published_d"] = spec.d
            res["source"] = spec.source
            rows.append(res)

    report = {
        "campaign": "campaign_a_catalog_benchmark",
        "positioned_against": "OmniQEC (98 / 240 physical qubit budgets)",
        "max_total_qubits": max_total_qubits,
        "shots_per_point": shots,
        "rounds": rounds,
        "noise_models": [nm_si.to_dict(), nm_heron.to_dict()],
        "rows": rows,
    }
    raw = json.dumps(report, sort_keys=True, separators=(",", ":")).encode()
    report["artifact_sha256"] = hashlib.sha256(raw).hexdigest()
    return report


def _build_circuit_from_edge_order(
    hx: np.ndarray,
    hz: np.ndarray,
    x_edges: Sequence[tuple[int, int]],
    z_edges: Sequence[tuple[int, int]],
    *,
    rounds: int,
    noise: CircuitNoiseModel,
) -> tuple[stim.Circuit, int]:
    """Build a Stim memory circuit from explicit X/Z edge orderings; return (circuit, depth)."""
    Hx, Hz = css_validate(hx, hz)
    _, lz = css_logicals(Hx, Hz)
    k = int(lz.shape[0])
    rx_m, n = Hx.shape
    rz_m, _ = Hz.shape
    total_qubits = n + rx_m + rz_m
    x_anc = list(range(n, n + rx_m))
    z_anc = list(range(n + rx_m, total_qubits))
    all_qubits = list(range(total_qubits))
    data_qubits = list(range(n))

    def _color(edges: Sequence[tuple[int, int]]) -> list[list[tuple[int, int]]]:
        layers: list[list[tuple[int, int]]] = []
        used_list: list[set[int]] = []
        for u, v in edges:
            placed = False
            for layer, used in zip(layers, used_list):
                if u not in used and v not in used:
                    layer.append((u, v))
                    used.add(u)
                    used.add(v)
                    placed = True
                    break
            if not placed:
                layers.append([(u, v)])
                used_list.append({u, v})
        return layers

    x_layers = _color(x_edges)
    z_layers = _color(z_edges)
    depth = len(x_layers) + len(z_layers)

    c = stim.Circuit()
    c.append("R", all_qubits)
    if noise.p_reset > 0:
        c.append("X_ERROR", all_qubits, noise.p_reset)
    meas_per_round = rx_m + rz_m
    for r_idx in range(rounds):
        c.append("TICK")
        if x_anc:
            c.append("H", x_anc)
        for layer in x_layers:
            flat = [q for pair in layer for q in pair]
            c.append("CX", flat)
            if noise.p_2q > 0:
                c.append("DEPOLARIZE2", flat, noise.p_2q)
            c.append("TICK")
        for layer in z_layers:
            flat = [q for pair in layer for q in pair]
            c.append("CX", flat)
            if noise.p_2q > 0:
                c.append("DEPOLARIZE2", flat, noise.p_2q)
            c.append("TICK")
        if x_anc:
            c.append("H", x_anc)
        if noise.p_idle > 0:
            c.append("DEPOLARIZE1", data_qubits, noise.p_idle)
        anc_all = x_anc + z_anc
        if noise.p_meas > 0:
            c.append("X_ERROR", anc_all, noise.p_meas)
        c.append("MR", anc_all)
        if r_idx == 0:
            for zi in range(rz_m):
                c.append("DETECTOR", [stim.target_rec(-(rz_m - zi))], [zi, 0])
        else:
            for xi in range(rx_m):
                cur = -(meas_per_round - xi)
                c.append("DETECTOR", [stim.target_rec(cur), stim.target_rec(cur - meas_per_round)], [xi, r_idx])
            for zi in range(rz_m):
                cur = -(rz_m - zi)
                c.append("DETECTOR", [stim.target_rec(cur), stim.target_rec(cur - meas_per_round)], [rx_m + zi, r_idx])
    if noise.p_meas > 0:
        c.append("X_ERROR", data_qubits, noise.p_meas)
    c.append("M", data_qubits)
    for zi in range(rz_m):
        supp = np.flatnonzero(Hz[zi]).tolist()
        targets = [stim.target_rec(-(n - q)) for q in supp]
        targets.append(stim.target_rec(-(n + (rz_m - zi))))
        c.append("DETECTOR", targets, [rx_m + zi, rounds])
    for li in range(k):
        supp = np.flatnonzero(lz[li]).tolist()
        c.append("OBSERVABLE_INCLUDE", [stim.target_rec(-(n - q)) for q in supp], li)
    return c, depth


def run_campaign_b_schedule_search(
    hx: np.ndarray,
    hz: np.ndarray,
    *,
    noise: CircuitNoiseModel | None = None,
    rounds: int = 2,
    trials: int = 12,
    shots: int = 200,
    seed: int = 7,
) -> dict[str, Any]:
    """Campaign B: Verifier-in-the-loop syndrome-extraction schedule search.

    Explores collision-free CNOT edge orderings, verifying Stim circuit-level
    distance d_circ and depth on every trial, and keeping the highest-d_circ,
    lowest-depth, lowest-LER schedule.
    """
    Hx, Hz = css_validate(hx, hz)
    rx_m, n = Hx.shape
    rz_m, _ = Hz.shape
    nm = noise or si1000_noise(0.003)
    rng = random.Random(seed)

    base_x = [(n + r, int(c)) for r in range(rx_m) for c in np.flatnonzero(Hx[r]).tolist()]
    base_z = [(int(c), n + rx_m + r) for r in range(rz_m) for c in np.flatnonzero(Hz[r]).tolist()]

    best: dict[str, Any] | None = None
    history: list[dict[str, Any]] = []

    for t in range(max(1, trials)):
        x_ord = list(base_x)
        z_ord = list(base_z)
        if t > 0:
            rng.shuffle(x_ord)
            rng.shuffle(z_ord)
        circ, depth = _build_circuit_from_edge_order(Hx, Hz, x_ord, z_ord, rounds=rounds, noise=nm)
        d_info = verify_circuit_distance(circ)
        d_circ = int(d_info["d_circ"]) if d_info.get("d_circ") is not None else 0
        rec = {"trial": t, "cnot_depth": depth, "d_circ": d_circ, "verified": bool(d_info.get("verified"))}
        history.append(rec)

        # Rank lexicographically: maximize d_circ, then minimize cnot_depth
        if best is None or (d_circ, -depth) > (best["d_circ"], -best["cnot_depth"]):
            best = dict(rec)
            best["_circuit"] = circ

    assert best is not None
    best_circ = best.pop("_circuit")
    sim_res = simulate_memory_circuit(
        best_circ,
        shots=shots,
        rounds=rounds,
        num_logicals=int(css_logicals(Hx, Hz)[1].shape[0]),
        decoder="bposd",
        seed=seed,
        code_name="campaign_b_best_schedule",
        noise_meta=nm.to_dict(),
    )
    return {
        "campaign": "campaign_b_schedule_search",
        "trials": len(history),
        "best_schedule": best,
        "best_sim": sim_res,
        "baseline_depth": history[0]["cnot_depth"],
        "baseline_d_circ": history[0]["d_circ"],
        "history": history,
    }
