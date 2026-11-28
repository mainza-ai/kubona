"""Exact (full-group MILP) and heuristic (BP-OSD) CSS distance engine (Gate 1).

Non-negotiable rules:
1. Every distance result explicitly carries ``kind: "proved" | "upper_bound"``.
2. ``kind == "proved"`` is emitted ONLY when the full ``2^k - 1`` non-trivial
   logical group is certified (either ``kOptimal`` or ``kInfeasible`` strictly
   below an independently verified logical witness), never generator-only.
3. Heuristic BP-OSD bounds are ALWAYS labelled ``upper_bound``.
"""

from __future__ import annotations

import hashlib
import time
from typing import Any

import numpy as np

from kubona.qec.gf2 import css_logicals, css_validate


def _verify_logical_witness(
    parity_checks: np.ndarray,
    logical_matrix: np.ndarray,
    vec: np.ndarray | None,
) -> tuple[bool, int | None]:
    """Independently verify that ``vec`` is a non-trivial logical operator:
    H @ vec == 0 (mod 2) and L_opp @ vec != 0 (mod 2).
    """
    if vec is None:
        return False, None
    e = np.asarray(vec, dtype=np.uint8) & 1
    if e.ndim != 1 or e.shape[0] != parity_checks.shape[1]:
        return False, None
    w = int(e.sum())
    if w <= 0:
        return False, None
    if np.any((parity_checks @ e) % 2):
        return False, None
    if not np.any((logical_matrix @ e) % 2):
        return False, None
    return True, w


def _min_weight_full_group_milp(
    parity_checks: np.ndarray,
    logical_matrix: np.ndarray,
    time_limit_s: float = 20.0,
    upper_bound: int | None = None,
    seed_witness: np.ndarray | None = None,
) -> tuple[int | None, np.ndarray | None, bool, str]:
    """Solve the exact full-group minimum-weight logical MILP over all 2^k - 1
    non-trivial logical classes simultaneously:

        min  sum_{j=0}^{n-1} e_j
        s.t. H e - 2 s == 0            (e in ker(H) mod 2)
             L e - 2 t - y == 0        (y_r = (L[r] . e) mod 2 in {0, 1})
             sum_{r=0}^{k-1} y_r >= 1  (non-trivial logical in full 2^k - 1 group)

    When ``upper_bound`` is backed by an independently verified ``seed_witness``,
    adds ``sum(e_j) <= upper_bound - 1``: ``kInfeasible`` then proves NO
    non-trivial logical in the entire 2^k - 1 group has weight < upper_bound.
    """
    import highspy

    H = parity_checks
    L = logical_matrix
    m, n = H.shape
    k = L.shape[0]
    if k == 0:
        return None, None, False, "empty_logical_group"

    # Never trust an unverified cutoff seed.
    verified_ub: int | None = None
    verified_witness: np.ndarray | None = None
    if upper_bound is not None and seed_witness is not None:
        ok, w_seed = _verify_logical_witness(H, L, seed_witness)
        if ok and w_seed is not None and w_seed == int(upper_bound):
            verified_ub = w_seed
            verified_witness = np.asarray(seed_witness, dtype=np.uint8) & 1

    h = highspy.Highs()
    h.setOptionValue("output_flag", False)
    h.setOptionValue("time_limit", float(max(0.5, time_limit_s)))
    h.setOptionValue("threads", 1)

    # Variable layout:
    #   e_0..e_{n-1}     in {0, 1}       (indices 0 .. n-1)
    #   s_0..s_{m-1}     in Z>=0         (indices n .. n+m-1)
    #   y_0..y_{k-1}     in {0, 1}       (indices n+m .. n+m+k-1)
    #   t_0..t_{k-1}     in Z>=0         (indices n+m+k .. n+m+2k-1)
    n_vars = n + m + 2 * k
    costs = np.zeros(n_vars, dtype=np.float64)
    costs[:n] = 1.0
    lower = np.zeros(n_vars, dtype=np.float64)
    upper = np.zeros(n_vars, dtype=np.float64)
    upper[:n] = 1.0
    for i in range(m):
        upper[n + i] = float(int(H[i].sum()) // 2)
    upper[n + m : n + m + k] = 1.0
    for r in range(k):
        upper[n + m + k + r] = float(int(L[r].sum()) // 2)

    integrality = [highspy.HighsVarType.kInteger] * n_vars
    h.addVars(n_vars, lower, upper)
    h.changeColsCost(n_vars, np.arange(n_vars, dtype=np.int32), costs)
    h.changeColsIntegrality(n_vars, np.arange(n_vars, dtype=np.int32), integrality)

    # Parity constraints: sum_{j in supp(H[i])} e_j - 2 s_i == 0
    for i in range(m):
        supp = np.flatnonzero(H[i])
        if supp.size == 0:
            continue
        idx = np.append(supp.astype(np.int32), np.int32(n + i))
        val = np.append(np.ones(supp.size, dtype=np.float64), -2.0)
        h.addRow(0.0, 0.0, len(idx), idx, val)

    # Logical parity indicators: sum_{j in supp(L[r])} e_j - 2 t_r - y_r == 0
    for r in range(k):
        supp = np.flatnonzero(L[r])
        idx = np.concatenate([
            supp.astype(np.int32),
            np.array([n + m + r, n + m + k + r], dtype=np.int32),
        ])
        val = np.concatenate([
            np.ones(supp.size, dtype=np.float64),
            np.array([-1.0, -2.0], dtype=np.float64),
        ])
        h.addRow(0.0, 0.0, len(idx), idx, val)

    # Full-group non-triviality: sum_{r=0}^{k-1} y_r >= 1
    y_idx = np.arange(n + m, n + m + k, dtype=np.int32)
    y_val = np.ones(k, dtype=np.float64)
    h.addRow(1.0, float(k), k, y_idx, y_val)

    # Cutoff constraint using independently verified witness
    if verified_ub is not None and verified_ub >= 1:
        idx_c = np.arange(n, dtype=np.int32)
        val_c = np.ones(n, dtype=np.float64)
        h.addRow(1.0, float(verified_ub - 1), n, idx_c, val_c)

    h.run()
    status = h.getModelStatus()
    sol = h.getSolution()

    if status == highspy.HighsModelStatus.kInfeasible and verified_ub is not None and verified_witness is not None:
        return verified_ub, verified_witness, True, "proved_no_lighter_full_group"

    if status == highspy.HighsModelStatus.kOptimal:
        e_vec = np.rint(np.array(sol.col_value[:n])).astype(np.uint8) & 1
        ok, w = _verify_logical_witness(H, L, e_vec)
        if not ok or w is None:
            return verified_ub, verified_witness, False, "witness_verification_failed"
        return w, e_vec, True, "optimal_full_group"

    # Time-limited: if solver found a strictly better verified incumbent, keep it as upper_bound only
    if sol.value_valid and len(sol.col_value) >= n:
        e_vec = np.rint(np.array(sol.col_value[:n])).astype(np.uint8) & 1
        ok, w = _verify_logical_witness(H, L, e_vec)
        if ok and w is not None and (verified_ub is None or w < verified_ub):
            return w, e_vec, False, str(status)

    return verified_ub, verified_witness, False, str(status)


def prove_distance_milp(
    hx: np.ndarray,
    hz: np.ndarray,
    *,
    time_limit_s: float = 20.0,
) -> dict[str, Any]:
    """Compute exact CSS distance d = min(d_X, d_Z) over the FULL 2^k - 1 logical group
    via HiGHS MILP with verified-witness cutoff pruning.

    Returns ``kind == "proved"`` ONLY when both X-side and Z-side full-group
    MILP problems are certified optimal or infeasible strictly below an
    independently verified logical witness.
    """
    Hx, Hz = css_validate(hx, hz)
    lx, lz = css_logicals(Hx, Hz)
    k = int(lx.shape[0])
    if k == 0:
        return {
            "value": None,
            "kind": "upper_bound",
            "method": "highspy_full_group_milp",
            "reason": "k=0 (no logical qubits)",
            "certificate": {"k": 0, "proof_scope": "none"},
        }

    # Fast verified witness seed from optional BP-OSD (warm-start cutoff only;
    # HiGHS MILP proves exact distance independently even when ldpc is absent).
    try:
        seed_ub = estimate_distance_bposd(Hx, Hz, num_trials=max(16, 2 * k), seed=7, return_witnesses=True)
    except ImportError:
        seed_ub = {}
    dx_seed = seed_ub.get("dx")
    dz_seed = seed_ub.get("dz")
    wx_seed = seed_ub.get("witness_x")
    wz_seed = seed_ub.get("witness_z")

    half_budget = max(1.0, float(time_limit_s) / 2.0)
    t0 = time.monotonic()
    dx, wx, opt_x, st_x = _min_weight_full_group_milp(
        Hz, lz, time_limit_s=half_budget, upper_bound=dx_seed, seed_witness=wx_seed
    )
    rem = max(1.0, float(time_limit_s) - (time.monotonic() - t0))
    dz, wz, opt_z, st_z = _min_weight_full_group_milp(
        Hx, lx, time_limit_s=rem, upper_bound=dz_seed, seed_witness=wz_seed
    )

    vals = [v for v in (dx, dz) if v is not None]
    d_val = min(vals) if vals else None
    all_optimal = bool(opt_x and opt_z and d_val is not None)
    kind = "proved" if all_optimal else "upper_bound"

    # Hash the verified witnesses into the certificate so Gate 1 can audit them
    h_obj = hashlib.sha256()
    if wx is not None:
        h_obj.update(b"X:" + np.asarray(wx, dtype=np.uint8).tobytes())
    if wz is not None:
        h_obj.update(b"Z:" + np.asarray(wz, dtype=np.uint8).tobytes())

    return {
        "value": d_val,
        "dx": dx,
        "dz": dz,
        "kind": kind,
        "method": "highspy_full_group_milp",
        "certificate": {
            "solver": "highspy",
            "proof_scope": "full_logical_group" if all_optimal else "upper_bound_only",
            "logical_group_size": (1 << k) - 1,
            "subproblems": 2,
            "all_optimal": all_optimal,
            "statuses": [f"X:{st_x}", f"Z:{st_z}"],
            "witness_x_support": np.flatnonzero(wx).tolist() if wx is not None else None,
            "witness_z_support": np.flatnonzero(wz).tolist() if wz is not None else None,
            "witness_sha256": h_obj.hexdigest(),
        },
    }


def estimate_distance_bposd(
    hx: np.ndarray,
    hz: np.ndarray,
    *,
    num_trials: int = 32,
    seed: int = 7,
    return_witnesses: bool = False,
) -> dict[str, Any]:
    """Fast heuristic distance upper bound via ldpc BP-OSD on logical cosets.

    ALWAYS returns ``kind == "upper_bound"`` — never certified.
    """
    from ldpc import BpOsdDecoder

    Hx, Hz = css_validate(hx, hz)
    lx, lz = css_logicals(Hx, Hz)
    k = int(lx.shape[0])
    if k == 0:
        return {"value": None, "kind": "upper_bound", "method": "bposd_heuristic", "trials": 0}

    rng = np.random.default_rng(seed)

    def _side_bound(H: np.ndarray, L_opp: np.ndarray) -> tuple[int | None, np.ndarray | None]:
        best: int | None = None
        best_vec: np.ndarray | None = None
        trials_per_row = max(1, num_trials // max(1, L_opp.shape[0]))
        for idx in range(L_opp.shape[0]):
            row = L_opp[idx]
            H_aug = np.vstack([H, row.reshape(1, -1)]).astype(np.uint8)
            syn = np.zeros(H_aug.shape[0], dtype=np.uint8)
            syn[-1] = 1
            for _ in range(trials_per_row):
                p_vec = rng.uniform(0.02, 0.15, size=H.shape[1])
                dec = BpOsdDecoder(
                    H_aug,
                    channel_probs=p_vec,
                    max_iter=20,
                    bp_method="minimum_sum",
                    osd_method="osd_cs",
                    osd_order=4,
                )
                cand = np.asarray(dec.decode(syn), dtype=np.uint8) & 1
                ok, w = _verify_logical_witness(H, L_opp, cand)
                if ok and w is not None and (best is None or w < best):
                    best = w
                    best_vec = cand.copy()
        return best, best_vec

    dx, wx = _side_bound(Hz, lz)
    dz, wz = _side_bound(Hx, lx)
    vals = [v for v in (dx, dz) if v is not None]
    out: dict[str, Any] = {
        "value": min(vals) if vals else None,
        "dx": dx,
        "dz": dz,
        "kind": "upper_bound",
        "method": "bposd_heuristic",
        "trials": num_trials,
    }
    if return_witnesses:
        out["witness_x"] = wx
        out["witness_z"] = wz
    return out
