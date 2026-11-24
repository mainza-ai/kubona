"""Exact (MILP) and heuristic (BP-OSD) CSS distance engine (Gate 1).

Non-negotiable rule: every distance result explicitly carries
``kind: "proved" | "upper_bound"``. Heuristic bounds are ALWAYS labelled
``upper_bound`` and may never be promoted into a DiscoveryClaim.
"""

from __future__ import annotations

from typing import Any

import numpy as np

from kubona.qec.gf2 import css_logicals, css_validate


def _min_weight_logical_milp(
    parity_checks: np.ndarray,
    logical_op: np.ndarray,
    time_limit_s: float = 20.0,
) -> tuple[int | None, bool, str]:
    """Solve min sum(e_j) s.t. H e = 0 (mod 2), l.e = 1 (mod 2) via HiGHS MILP.

    Returns (weight, proved_optimal, status_label).
    """
    import highspy

    H = parity_checks
    m, n = H.shape
    l_supp = np.flatnonzero(logical_op)
    if l_supp.size == 0:
        return None, False, "empty_logical"

    h = highspy.Highs()
    h.setOptionValue("output_flag", False)
    h.setOptionValue("time_limit", float(max(0.5, time_limit_s)))
    h.setOptionValue("threads", 1)

    # Variables: e_0..e_{n-1} in {0,1}, s_0..s_{m-1} in Z>=0 (parity slacks), t in Z>=0 (logical slack)
    n_vars = n + m + 1
    costs = np.zeros(n_vars, dtype=np.float64)
    costs[:n] = 1.0
    lower = np.zeros(n_vars, dtype=np.float64)
    upper = np.zeros(n_vars, dtype=np.float64)
    upper[:n] = 1.0
    for i in range(m):
        upper[n + i] = float(int(H[i].sum()) // 2)
    upper[n + m] = float(int(l_supp.size) // 2)

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

    # Odd overlap constraint: sum_{j in supp(l)} e_j - 2 t == 1
    idx_l = np.append(l_supp.astype(np.int32), np.int32(n + m))
    val_l = np.append(np.ones(l_supp.size, dtype=np.float64), -2.0)
    h.addRow(1.0, 1.0, len(idx_l), idx_l, val_l)

    h.run()
    status = h.getModelStatus()
    sol = h.getSolution()
    if status == highspy.HighsModelStatus.kOptimal:
        e_vec = np.rint(np.array(sol.col_value[:n])).astype(np.uint8)
        w = int(e_vec.sum())
        # Independent check of the returned witness before trusting solver status
        if np.any((H @ e_vec) % 2) or int((logical_op @ e_vec) % 2) != 1:
            return None, False, "witness_verification_failed"
        return w, True, "optimal"
    # If time-limited with a valid primal incumbent, return it strictly as upper_bound
    if sol.value_valid and len(sol.col_value) >= n:
        e_vec = np.rint(np.array(sol.col_value[:n])).astype(np.uint8)
        if not np.any((H @ e_vec) % 2) and int((logical_op @ e_vec) % 2) == 1:
            return int(e_vec.sum()), False, str(status)
    return None, False, str(status)


def prove_distance_milp(
    hx: np.ndarray,
    hz: np.ndarray,
    *,
    time_limit_s: float = 20.0,
) -> dict[str, Any]:
    """Compute exact CSS distance d = min(d_X, d_Z) via HiGHS MILP.

    Returns ``kind == "proved"`` ONLY when every logical operator subproblem
    reaches verified optimality within ``time_limit_s``; otherwise returns
    ``kind == "upper_bound"``.
    """
    Hx, Hz = css_validate(hx, hz)
    lx, lz = css_logicals(Hx, Hz)
    k = int(lx.shape[0])
    if k == 0:
        return {
            "value": None,
            "kind": "upper_bound",
            "method": "highspy_milp",
            "reason": "k=0 (no logical qubits)",
            "certificate": {"k": 0},
        }

    # For d_X: min weight e in ker(Hz) with odd overlap against some l_z in Lz
    # For d_Z: min weight e in ker(Hx) with odd overlap against some l_x in Lx
    per_sub_limit = max(0.5, float(time_limit_s) / max(1, 2 * k))
    dx_best: int | None = None
    dz_best: int | None = None
    all_optimal = True
    statuses: list[str] = []

    for row in lz:
        w, opt, st = _min_weight_logical_milp(Hz, row, time_limit_s=per_sub_limit)
        statuses.append(f"X:{st}")
        if not opt:
            all_optimal = False
        if w is not None and (dx_best is None or w < dx_best):
            dx_best = w

    for row in lx:
        w, opt, st = _min_weight_logical_milp(Hx, row, time_limit_s=per_sub_limit)
        statuses.append(f"Z:{st}")
        if not opt:
            all_optimal = False
        if w is not None and (dz_best is None or w < dz_best):
            dz_best = w

    vals = [v for v in (dx_best, dz_best) if v is not None]
    d_val = min(vals) if vals else None
    kind = "proved" if (all_optimal and d_val is not None) else "upper_bound"
    return {
        "value": d_val,
        "dx": dx_best,
        "dz": dz_best,
        "kind": kind,
        "method": "highspy_milp",
        "certificate": {
            "solver": "highspy",
            "subproblems": 2 * k,
            "all_optimal": all_optimal,
            "statuses": statuses[:16],
        },
    }


def estimate_distance_bposd(
    hx: np.ndarray,
    hz: np.ndarray,
    *,
    num_trials: int = 32,
    seed: int = 7,
) -> dict[str, Any]:
    """Fast heuristic distance upper bound via ldpc BP-OSD on random logical cosets.

    ALWAYS returns ``kind == "upper_bound"`` — never certified.
    """
    from ldpc import BpOsdDecoder

    Hx, Hz = css_validate(hx, hz)
    lx, lz = css_logicals(Hx, Hz)
    k = int(lx.shape[0])
    if k == 0:
        return {"value": None, "kind": "upper_bound", "method": "bposd_heuristic", "trials": 0}

    rng = np.random.default_rng(seed)

    def _side_bound(H: np.ndarray, L_opp: np.ndarray) -> int | None:
        best: int | None = None
        for idx in range(L_opp.shape[0]):
            row = L_opp[idx]
            # Augmented matrix [H; row] with syndrome [0..0, 1] seeks min-weight e in ker(H) with row.e = 1
            H_aug = np.vstack([H, row.reshape(1, -1)]).astype(np.uint8)
            syn = np.zeros(H_aug.shape[0], dtype=np.uint8)
            syn[-1] = 1
            for _ in range(max(1, num_trials // max(1, L_opp.shape[0]))):
                # Randomized error-rate weights break symmetry across trials
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
                if not np.any((H @ cand) % 2) and int((row @ cand) % 2) == 1:
                    w = int(cand.sum())
                    if w > 0 and (best is None or w < best):
                        best = w
        return best

    dx = _side_bound(Hz, lz)
    dz = _side_bound(Hx, lx)
    vals = [v for v in (dx, dz) if v is not None]
    return {
        "value": min(vals) if vals else None,
        "dx": dx,
        "dz": dz,
        "kind": "upper_bound",
        "method": "bposd_heuristic",
        "trials": num_trials,
    }
