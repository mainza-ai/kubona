"""Clifford Data Regression (CDR) with live asymmetric readout calibration.

Implements:
1. Live ``2^n x 2^n`` asymmetric readout assignment matrix calibration for
   ``n <= 4`` with fail-closed conditioning checks (``cond(M) < 50``).
2. Structure-preserving, energy-stratified near-Clifford training circuit
   generation (identical 2Q gate count/topology to the target circuit, exact
   symmetry-sector alignment when Z2 symmetry verification is active, never
   including the unperturbed target circuit itself).
3. Errors-in-variables / Deming linear regression (exact inverse-WLS MLE when
   training exact energies have zero variance, ``scipy.odr`` fallback when
   approximate training uncertainties are supplied) with Birge-scaled parameter
   covariance, LOO cross-validation, and exact coupled paired-difference CIs.
4. Same-batch 4-layer ablation:
   ``raw_unmitigated -> live_readout_mitigated -> symmetry_verified -> cdr_mitigated``.
"""

from __future__ import annotations

import math
from typing import Any

import numpy as np

from .qwc import (
    _index_to_bitstring,
    counts_to_prob_vector,
    estimate_energy_from_qwc_counts,
    group_observable_vector,
    linear_functional_mean_and_var,
)
from .symmetry import (
    _statevector_z_expectation,
    symmetry_verified_energy,
)

Z95 = 1.959963984540054
_CLIFFORD_ANGLES = (0.0, 0.5 * math.pi, math.pi, 1.5 * math.pi)
_ROTATION_GATES = {"rx", "ry", "rz", "p", "u1", "u2", "u3", "u"}


def _count_two_qubit_gates(qc: Any) -> int:
    return sum(
        1 for inst in qc.data
        if len(inst.qubits) >= 2
        and str(getattr(inst.operation, "name", "")) not in ("barrier", "measure", "reset")
    )


def _exact_energy_from_state(state: np.ndarray, coeffs: dict[str, float]) -> float:
    from qiskit.quantum_info import SparsePauliOp

    paulis = [(str(k).strip().upper(), float(v)) for k, v in coeffs.items() if abs(float(v)) > 0.0]
    if not paulis:
        return 0.0
    op = SparsePauliOp.from_list(paulis)
    val = np.vdot(state, op.to_matrix() @ state)
    return float(np.real(val))


def _exact_energy_and_state(qc: Any, coeffs: dict[str, float]) -> tuple[float, np.ndarray]:
    from qiskit.quantum_info import Statevector

    state = np.asarray(Statevector.from_instruction(qc).data, dtype=complex)
    return _exact_energy_from_state(state, coeffs), state


def build_readout_cal_circuits(n_qubits: int) -> list[tuple[str, Any]]:
    """Build all ``2^n`` computational-basis calibration circuits for ``n <= 4``."""
    from qiskit import QuantumCircuit

    n = int(n_qubits)
    if n < 1 or n > 4:
        raise ValueError(f"live full readout matrix calibration requires 1 <= n_qubits <= 4, got {n}")
    circuits: list[tuple[str, Any]] = []
    for idx in range(1 << n):
        bs = _index_to_bitstring(idx, n)
        qc = QuantumCircuit(n)
        # Qiskit little-endian: bs[-1] is qubit 0, bs[0] is qubit n-1.
        for q, bit in enumerate(bs[::-1]):
            if bit == "1":
                qc.x(q)
        qc.measure_all()
        circuits.append((bs, qc))
    return circuits


def build_assignment_matrix(
    cal_counts: list[dict[str, int]],
    n_qubits: int,
    *,
    max_condition_number: float = 50.0,
) -> dict[str, Any]:
    """Assemble and invert the ``2^n x 2^n`` readout assignment matrix ``M``.

    Column ``j`` of ``M`` is ``P(measured = i | prepared = j)``. Refuses
    inversion fail-closed when ``cond(M) >= max_condition_number``.
    """
    n = int(n_qubits)
    dim = 1 << n
    if len(cal_counts) != dim:
        raise ValueError(f"expected {dim} calibration count dicts for {n} qubits, got {len(cal_counts)}")

    mat = np.zeros((dim, dim), dtype=float)
    shots_per_state: list[int] = []
    for j, counts in enumerate(cal_counts):
        p_col, total = counts_to_prob_vector(counts, n)
        mat[:, j] = p_col
        shots_per_state.append(int(total))

    cond = float(np.linalg.cond(mat))
    diag = [float(mat[j, j]) for j in range(dim)]
    mean_err = float(1.0 - np.mean(diag))

    # Per-qubit asymmetric readout diagnostics P(1|0) and P(0|1) averaged over spectator states.
    per_qubit_asym: dict[str, dict[str, float]] = {}
    for q in range(n):
        p1_given_0: list[float] = []
        p0_given_1: list[float] = []
        for j in range(dim):
            prep_bit = (j >> q) & 1
            prob_1 = sum(float(mat[i, j]) for i in range(dim) if ((i >> q) & 1) == 1)
            if prep_bit == 0:
                p1_given_0.append(prob_1)
            else:
                p0_given_1.append(1.0 - prob_1)
        per_qubit_asym[str(q)] = {
            "p1_given_0": float(np.mean(p1_given_0)) if p1_given_0 else 0.0,
            "p0_given_1": float(np.mean(p0_given_1)) if p0_given_1 else 0.0,
        }

    if not math.isfinite(cond) or cond >= float(max_condition_number):
        return {
            "ok": False,
            "error": (
                f"readout assignment matrix is ill-conditioned: cond(M)={cond:.3f} "
                f">= {max_condition_number}"
            ),
            "n_qubits": n,
            "condition_number": cond,
            "assignment_matrix": mat.tolist(),
            "shots_per_state": shots_per_state,
            "diagonal_fidelities": diag,
            "mean_state_error": mean_err,
            "per_qubit_asymmetry": per_qubit_asym,
        }

    m_inv = np.linalg.inv(mat)
    return {
        "ok": True,
        "n_qubits": n,
        "condition_number": cond,
        "assignment_matrix": mat,
        "inverse_matrix": m_inv,
        "assignment_matrix_list": mat.tolist(),
        "shots_per_state": shots_per_state,
        "diagonal_fidelities": diag,
        "mean_state_error": mean_err,
        "per_qubit_asymmetry": per_qubit_asym,
    }


def _nearest_clifford(angle: float) -> float:
    return round(float(angle) / (0.5 * math.pi)) * (0.5 * math.pi)


def _rebuild_with_angles(qc: Any, rot_indices: list[int], new_params: dict[int, list[float]]) -> Any:
    rebuilt = qc.copy_empty_like()
    for i, inst in enumerate(qc.data):
        op, qargs, cargs = inst.operation, inst.qubits, inst.clbits
        if i in new_params:
            cloned = op.copy()
            cloned.params = [float(p) for p in new_params[i]]
            rebuilt.append(cloned, qargs, cargs)
        else:
            rebuilt.append(op, qargs, cargs)
    return rebuilt


def _param_distance(orig: dict[int, list[float]], cand: dict[int, list[float]]) -> float:
    max_d = 0.0
    for idx, vals in orig.items():
        cvals = cand.get(idx, vals)
        for a, b in zip(vals, cvals):
            diff = abs((float(a) - float(b) + math.pi) % (2.0 * math.pi) - math.pi)
            if diff > max_d:
                max_d = diff
    return max_d


def generate_cdr_training_circuits(
    qc: Any,
    coeffs: dict[str, float],
    *,
    n_circuits: int = 12,
    seed: int = 7,
    symmetries: list[dict[str, Any]] | None = None,
    min_distinct_energies: int = 8,
) -> dict[str, Any]:
    """Generate structure-preserving, energy-stratified near-Clifford CDR circuits.

    Every training circuit preserves the exact 2Q gate count and topology of ``qc``
    and differs from ``qc`` by at least one rotation parameter (no target-circuit
    data leakage). When ``symmetries`` are supplied, prioritizes circuits in the
    exact same symmetry sector so the training pipeline matches the target
    ``readout -> symmetry`` pipeline identically.
    """
    m = int(n_circuits)
    if m < 4 or m > 64:
        raise ValueError(f"CDR n_circuits must be in [4, 64], got {m}")

    target_2q = _count_two_qubit_gates(qc)
    target_energy, target_state = _exact_energy_and_state(qc, coeffs)
    rng = np.random.default_rng(int(seed))

    rot_indices: list[int] = []
    orig_params: dict[int, list[float]] = {}
    for i, inst in enumerate(qc.data):
        name = str(getattr(inst.operation, "name", "")).lower()
        params = list(getattr(inst.operation, "params", []) or [])
        if name in _ROTATION_GATES and params and len(inst.qubits) == 1:
            rot_indices.append(i)
            orig_params[i] = [float(p) for p in params]

    if not rot_indices:
        raise ValueError("CDR training requires at least one single-qubit rotation gate in the ansatz")

    def _in_target_sector(state: np.ndarray) -> bool:
        if not symmetries:
            return True
        for sym in symmetries:
            exp_val = _statevector_z_expectation(state, str(sym["pauli"]))
            if abs(exp_val - float(sym["eigenvalue"])) > 1e-4:
                return False
        return True

    # Build a rich, deterministic candidate pool of pure-Clifford and near-Clifford
    # (1 or 2 non-Clifford rotations) circuits with identical 2Q structure.
    candidates: list[dict[str, Any]] = []
    seen_energy_keys: set[tuple[bool, int]] = set()

    def _consider(param_map: dict[int, list[float]], kind: str, n_non_clifford: int) -> None:
        if _param_distance(orig_params, param_map) < 1e-5:
            return  # Never leak the exact target circuit into training!
        cand_qc = _rebuild_with_angles(qc, rot_indices, param_map)
        if _count_two_qubit_gates(cand_qc) != target_2q:
            return
        e_exact, st = _exact_energy_and_state(cand_qc, coeffs)
        in_sec = _in_target_sector(st)
        key = (in_sec, int(round(e_exact * 2000.0)))  # 0.5 mHa dedupe resolution
        if key in seen_energy_keys:
            return
        seen_energy_keys.add(key)
        candidates.append({
            "circuit": cand_qc,
            "exact_energy": float(e_exact),
            "in_target_sector": bool(in_sec),
            "kind": kind,
            "non_clifford_gates": int(n_non_clifford),
            "two_qubit_gates": target_2q,
        })

    # 1. Nearest-Clifford baseline + random pure-Clifford snaps.
    nearest_map = {idx: [_nearest_clifford(p) for p in vals] for idx, vals in orig_params.items()}
    _consider(nearest_map, "clifford_nearest", 0)
    for _ in range(96):
        cmap = {
            idx: [float(_CLIFFORD_ANGLES[int(rng.integers(0, 4))]) for _ in vals]
            for idx, vals in orig_params.items()
        }
        _consider(cmap, "clifford_random", 0)

    # 2. Near-Clifford (1 non-Clifford rotation retained or swept on a coarse grid,
    #    all other rotations Clifford): reaches ground-state neighborhood cleanly.
    sweep_angles = [0.15, 0.35, 0.65, 1.0, 1.35, 2.0, 2.5, 2.9, -0.25, -0.7, -1.2]
    for keep_idx in rot_indices:
        # Keep original target angle at keep_idx, nearest Clifford elsewhere
        pmap = {
            idx: (list(vals) if idx == keep_idx else [_nearest_clifford(p) for p in vals])
            for idx, vals in orig_params.items()
        }
        _consider(pmap, "near_clifford_1_target", 1)
        # Keep original target angle at keep_idx with random Clifford elsewhere
        for _ in range(18):
            pmap2 = {
                idx: (
                    list(vals)
                    if idx == keep_idx
                    else [float(_CLIFFORD_ANGLES[int(rng.integers(0, 4))]) for _ in vals]
                )
                for idx, vals in orig_params.items()
            }
            _consider(pmap2, "near_clifford_1_rand", 1)
        # Sweep keep_idx with nearest/zero Clifford elsewhere to fill energy gaps
        for ang in sweep_angles:
            pmap3 = {
                idx: ([ang for _ in vals] if idx == keep_idx else [0.0 for _ in vals])
                for idx, vals in orig_params.items()
            }
            _consider(pmap3, "near_clifford_1_sweep_zero", 1)
            pmap4 = {
                idx: ([ang for _ in vals] if idx == keep_idx else [_nearest_clifford(p) for p in vals])
                for idx, vals in orig_params.items()
            }
            _consider(pmap4, "near_clifford_1_sweep_near", 1)

    # 3. Near-Clifford (2 non-Clifford rotations) if pool is still sparse.
    if len(rot_indices) >= 2:
        for _ in range(96):
            chosen = set(int(rot_indices[j]) for j in rng.choice(len(rot_indices), size=2, replace=False))
            pmap = {}
            for idx, vals in orig_params.items():
                if idx in chosen:
                    # Half target angle, half perturbed angle
                    if rng.random() < 0.5:
                        pmap[idx] = list(vals)
                    else:
                        pmap[idx] = [float(p + rng.uniform(-0.8, 0.8)) for p in vals]
                else:
                    pmap[idx] = [float(_CLIFFORD_ANGLES[int(rng.integers(0, 4))]) for _ in vals]
            _consider(pmap, "near_clifford_2", 2)

    sector_pool = [c for c in candidates if c["in_target_sector"]]
    used_sector_filter = bool(symmetries and len(sector_pool) >= m)
    pool = sector_pool if used_sector_filter else candidates
    if len(pool) < min(4, m):
        raise ValueError(
            f"CDR generator found only {len(pool)} distinct training circuits (need at least {min(4, m)})"
        )

    pool.sort(key=lambda c: c["exact_energy"])

    # Prefer the physical energy band anchored near target_energy so non-unital
    # T1 bias is calibrated locally without sacrificing linear lever arm:
    # keep candidates within [min_e, target_energy + 1.2 Ha] if enough exist,
    # otherwise use the full pool.
    local_band = [c for c in pool if c["exact_energy"] <= target_energy + 1.25]
    if len(local_band) >= m:
        pool = local_band

    # Energy-stratified selection across [E_min, E_max]: choose one circuit nearest
    # each quantile target (with extra density near E_target).
    e_min = pool[0]["exact_energy"]
    e_max = pool[-1]["exact_energy"]
    if len(pool) <= m:
        selected = list(pool)
    else:
        # Half of targets densely cover the lower 40% near E_target, half span the upper 60%.
        n_low = max(2, m // 2)
        n_high = m - n_low
        e_mid = e_min + 0.40 * (e_max - e_min)
        targets = list(np.linspace(e_min, e_mid, n_low)) + list(
            np.linspace(e_mid, e_max, n_high + 1)[1:]
        )
        used_idx: set[int] = set()
        selected = []
        for t in targets:
            best_i = min(
                (i for i in range(len(pool)) if i not in used_idx),
                key=lambda i: (abs(pool[i]["exact_energy"] - t), pool[i]["non_clifford_gates"]),
            )
            used_idx.add(best_i)
            selected.append(pool[best_i])
        selected.sort(key=lambda c: c["exact_energy"])

    distinct = len({round(c["exact_energy"], 4) for c in selected})
    need_distinct = min(int(min_distinct_energies), len(selected))
    if distinct < need_distinct:
        raise ValueError(
            f"CDR training set has only {distinct} distinct exact energies (need >= {need_distinct})"
        )

    sel_min = float(selected[0]["exact_energy"])
    sel_max = float(selected[-1]["exact_energy"])
    span = max(sel_max - sel_min, 1e-9)
    tol = max(0.025, 0.05 * span)
    return {
        "circuits": [c["circuit"] for c in selected],
        "exact_energies": [float(c["exact_energy"]) for c in selected],
        "metadata": [
            {
                "index": i,
                "exact_energy": float(c["exact_energy"]),
                "in_target_sector": bool(c["in_target_sector"]),
                "kind": c["kind"],
                "non_clifford_gates": int(c["non_clifford_gates"]),
                "two_qubit_gates": int(c["two_qubit_gates"]),
            }
            for i, c in enumerate(selected)
        ],
        "target_exact_energy": float(target_energy),
        "target_two_qubit_gates": int(target_2q),
        "distinct_energies": int(distinct),
        "energy_min": sel_min,
        "energy_max": sel_max,
        "energy_span": float(sel_max - sel_min),
        "sector_aligned": bool(used_sector_filter),
        "target_strictly_bracketed": bool(sel_min - 1e-9 <= target_energy <= sel_max + 1e-9),
        "target_in_training_domain": bool(sel_min - tol <= target_energy <= sel_max + tol),
        "extrapolation_distance_ha": float(max(0.0, sel_min - target_energy, target_energy - sel_max)),
    }


def _fit_inverse_wls(
    x_noisy: np.ndarray,
    y_exact: np.ndarray,
    sx: np.ndarray,
) -> tuple[float, float, np.ndarray, float, float]:
    """Exact MLE for ``x_i = a * y_i + b + eps_i, eps_i ~ N(0, sx_i^2)``.

    Returns ``(a, b, cov_ab_unscaled, chi2, chi2_red)``.
    """
    w = 1.0 / np.maximum(sx, 1e-9) ** 2
    s_w = float(np.sum(w))
    s_y = float(np.sum(w * y_exact))
    s_yy = float(np.sum(w * y_exact * y_exact))
    s_x = float(np.sum(w * x_noisy))
    s_xy = float(np.sum(w * x_noisy * y_exact))
    det = s_w * s_yy - s_y * s_y
    if not math.isfinite(det) or abs(det) <= 1e-18:
        raise ValueError("CDR training energies are degenerate (singular regression matrix)")
    a = (s_w * s_xy - s_y * s_x) / det
    b = (s_yy * s_x - s_y * s_xy) / det
    cov_ab = np.array([[s_w, -s_y], [-s_y, s_yy]], dtype=float) / det
    resid = x_noisy - (a * y_exact + b)
    chi2 = float(np.sum(w * resid * resid))
    dof = max(1, len(x_noisy) - 2)
    chi2_red = chi2 / float(dof)
    return float(a), float(b), cov_ab, chi2, chi2_red


def fit_cdr_odr(
    train_noisy: list[float],
    train_exact: list[float],
    train_uncertainties: list[float],
    target_noisy: float,
    target_uncertainty: float,
    *,
    train_exact_uncertainties: list[float] | None = None,
    raw_baseline_energy: float | None = None,
    raw_baseline_uncertainty: float | None = None,
    input_vs_raw_diff_var: float | None = None,
    chi2_limit: float = 1.5,
    r2_min: float = 0.90,
) -> dict[str, Any]:
    """Fit linear CDR model with Deming/errors-in-variables uncertainty propagation.

    When training exact energies have zero uncertainty (standard CDR), solves the
    exact errors-in-variables MLE ``x_i = a y_i + b + eps_i`` (zero attenuation
    bias, closed-form Gauss-Markov covariance) and inverts to
    ``E_CDR = (E_target - b) / a = alpha * E_target + beta``. When non-zero
    ``train_exact_uncertainties`` are provided, uses ``scipy.odr``.

    Reports exact coupled paired-difference CIs against both the immediate CDR
    input baseline (``target_noisy``) and an optional unmitigated raw baseline.
    """
    x = np.asarray(train_noisy, dtype=float)
    y = np.asarray(train_exact, dtype=float)
    sx = np.asarray(train_uncertainties, dtype=float)
    m = len(x)
    if m < 4 or len(y) != m or len(sx) != m:
        raise ValueError(f"CDR requires >= 4 aligned training points, got {m}")
    if np.any(~np.isfinite(x)) or np.any(~np.isfinite(y)) or np.any(~np.isfinite(sx)):
        raise ValueError("CDR training arrays contain non-finite values")
    if np.any(sx <= 0.0):
        # Floor zero shot-SE (e.g. deterministic noise-free simulator test) so WLS stays defined.
        sx = np.maximum(sx, 1e-6)

    use_scipy_odr = (
        train_exact_uncertainties is not None
        and any(float(v) > 0.0 for v in train_exact_uncertainties)
    )
    dof = m - 2

    if use_scipy_odr:
        from scipy import odr as scipy_odr

        sy = np.maximum(np.asarray(train_exact_uncertainties, dtype=float), 1e-9)
        a0, b0, _, _, _ = _fit_inverse_wls(x, y, sx)
        model = scipy_odr.Model(lambda beta, x_in: beta[0] * x_in + beta[1])
        data = scipy_odr.RealData(y, x, sx=sy, sy=sx)
        out = scipy_odr.ODR(data, model, beta0=[a0, b0]).run()
        a, b = float(out.beta[0]), float(out.beta[1])
        chi2_red = float(out.res_var)
        chi2 = chi2_red * float(dof)
        cov_ab_unscaled = np.asarray(out.cov_beta, dtype=float)
        fit_method = "cdr_scipy_odr"
    else:
        a, b, cov_ab_unscaled, chi2, chi2_red = _fit_inverse_wls(x, y, sx)
        fit_method = "cdr_eiv_deming_exact"

    if not math.isfinite(a) or abs(a) < 1e-6:
        raise ValueError(f"CDR regression slope collapsed (a={a!r}); noise channel unidentifiable")

    # Birge-scale parameter covariance when chi2_red > 1 so lack-of-fit inflates CI honestly.
    birge = max(1.0, chi2_red)
    cov_ab = cov_ab_unscaled * birge

    alpha = 1.0 / a
    beta = -b / a
    x0 = float(target_noisy)
    s0 = max(0.0, float(target_uncertainty))
    e_cdr = float((x0 - b) / a)

    # Jacobian of E_CDR = (x0 - b) / a w.r.t. (a, b): [-E_CDR / a, -1 / a]
    g_ab = np.array([-e_cdr / a, -1.0 / a], dtype=float)
    var_fit = float(max(0.0, g_ab @ cov_ab @ g_ab))
    var_target = (alpha * s0) ** 2
    var_cdr = var_target + var_fit
    se_cdr = math.sqrt(max(0.0, var_cdr))

    # Propagated covariance of (alpha, beta) for diagnostics
    j_ab = np.array([[-1.0 / (a * a), 0.0], [b / (a * a), -1.0 / a]], dtype=float)
    cov_alpha_beta = j_ab @ cov_ab @ j_ab.T

    # R^2 on exact vs noisy linear relation
    x_pred = a * y + b
    ss_res = float(np.sum((x - x_pred) ** 2))
    ss_tot = float(np.sum((x - np.mean(x)) ** 2))
    r_squared = float(1.0 - ss_res / ss_tot) if ss_tot > 1e-18 else 0.0

    # Lack-of-fit p-value from chi2 distribution
    lack_of_fit_p: float | None = None
    try:
        from scipy.stats import chi2 as chi2_dist
        lack_of_fit_p = float(chi2_dist.sf(chi2, dof))
    except Exception:
        lack_of_fit_p = None

    # Leave-One-Out Cross-Validation RMSE (in mHa) on exact energy predictions
    loo_errors: list[float] = []
    for i in range(m):
        mask = np.arange(m) != i
        try:
            ai, bi, _, _, _ = _fit_inverse_wls(x[mask], y[mask], sx[mask])
            if abs(ai) > 1e-6:
                y_hat_i = (x[i] - bi) / ai
                loo_errors.append(float(y_hat_i - y[i]))
        except Exception:
            continue
    loo_cv_mha = (
        float(math.sqrt(float(np.mean(np.square(loo_errors)))) * 1000.0)
        if len(loo_errors) == m
        else float("inf")
    )

    # Exact coupled paired-difference CI against CDR input baseline x0:
    # D = E_CDR - x0 = (alpha - 1) * x0 + beta  =>  Var(D) = (alpha - 1)^2 * s0^2 + var_fit
    diff_vs_input = e_cdr - x0
    var_diff_input = ((alpha - 1.0) * s0) ** 2 + var_fit
    diff_vs_input_se = math.sqrt(max(0.0, var_diff_input))
    diff_vs_input_ci95 = Z95 * diff_vs_input_se

    # Difference vs raw unmitigated baseline (if supplied separately from x0)
    if raw_baseline_energy is not None:
        raw_e = float(raw_baseline_energy)
        raw_se = max(0.0, float(raw_baseline_uncertainty or 0.0))
        diff_vs_raw = e_cdr - raw_e
        if input_vs_raw_diff_var is not None and input_vs_raw_diff_var >= 0.0:
            # Exact same-batch target covariance of (alpha * x0 - raw_e) + var_fit
            var_diff_raw = float(input_vs_raw_diff_var) + var_fit
        else:
            # Conservative independent fallback
            var_diff_raw = var_cdr + raw_se * raw_se
        diff_vs_raw_se = math.sqrt(max(0.0, var_diff_raw))
        diff_vs_raw_ci95 = Z95 * diff_vs_raw_se
    else:
        diff_vs_raw = diff_vs_input
        diff_vs_raw_se = diff_vs_input_se
        diff_vs_raw_ci95 = diff_vs_input_ci95

    y_min, y_max = float(np.min(y)), float(np.max(y))
    span = max(y_max - y_min, 1e-9)
    tol = max(0.025, 0.05 * span)
    in_domain = bool(y_min - tol <= e_cdr <= y_max + tol)
    mean_train_se_mha = float(np.mean(sx) * 1000.0)
    loo_limit_mha = max(5.0, 2.5 * mean_train_se_mha)
    model_adequate = bool(
        a > 0.1
        and math.isfinite(r_squared)
        and r_squared >= float(r2_min)
        and chi2_red <= float(chi2_limit)
        and math.isfinite(loo_cv_mha)
        and loo_cv_mha <= loo_limit_mha
        and in_domain
    )

    return {
        "method": "CDR",
        "fit_method": fit_method,
        "mitigated": float(e_cdr),
        "uncertainty": float(se_cdr),
        "ci95_mha": float(Z95 * se_cdr * 1000.0),
        "slope": float(alpha),
        "intercept": float(beta),
        "forward_slope": float(a),
        "forward_intercept": float(b),
        "slope_std_error": float(math.sqrt(max(0.0, cov_alpha_beta[0, 0]))),
        "intercept_std_error": float(math.sqrt(max(0.0, cov_alpha_beta[1, 1]))),
        "cov_alpha_beta": cov_alpha_beta.tolist(),
        "var_target_component": float(var_target),
        "var_fit_component": float(var_fit),
        "birge_scale": float(birge),
        "r_squared": float(r_squared),
        "chi2": float(chi2),
        "chi2_reduced": float(chi2_red),
        "dof": int(dof),
        "lack_of_fit_p": lack_of_fit_p,
        "loo_cv_mha": float(loo_cv_mha),
        "loo_cv_limit_mha": float(loo_limit_mha),
        "training_count": int(m),
        "training_exact_min": y_min,
        "training_exact_max": y_max,
        "target_in_training_domain": in_domain,
        "model_adequate": model_adequate,
        "baseline_energy": x0,
        "baseline_uncertainty": s0,
        "difference_vs_input_ha": float(diff_vs_input),
        "difference_vs_input_ci95_ha": float(diff_vs_input_ci95),
        "difference_vs_input_ci95_mha": float(diff_vs_input_ci95 * 1000.0),
        "difference_vs_input_significant": bool(abs(diff_vs_input) > diff_vs_input_ci95),
        "difference_vs_raw_ha": float(diff_vs_raw),
        "difference_vs_raw_ci95_ha": float(diff_vs_raw_ci95),
        "difference_vs_raw_ci95_mha": float(diff_vs_raw_ci95 * 1000.0),
        "difference_vs_raw_significant": bool(abs(diff_vs_raw) > diff_vs_raw_ci95),
        # Canonical G6 difference keys compare full mitigated stack vs unmitigated raw
        "difference_ha": float(diff_vs_raw),
        "difference_ci95_ha": float(diff_vs_raw_ci95),
        "difference_ci95_mha": float(diff_vs_raw_ci95 * 1000.0),
        "difference_significant": bool(abs(diff_vs_raw) > diff_vs_raw_ci95),
    }


def _same_batch_coupled_diff_var(
    grouped: dict[str, Any],
    target_counts: list[dict[str, int]],
    coeffs: dict[str, float],
    alpha: float,
    *,
    readout_inverse: np.ndarray | None = None,
    assignment_matrix: np.ndarray | None = None,
    cal_shots_per_state: list[int] | None = None,
    symmetries: list[dict[str, Any]] | None = None,
    sv_result: dict[str, Any] | None = None,
) -> float:
    """Exact target-shot variance of ``alpha * E_input - E_raw_unmitigated`` on shared counts."""
    n_qubits = int(grouped["n_qubits"])
    groups = grouped["groups"]
    if n_qubits > 10:
        return -1.0

    # If symmetry verification wasn't used, both E_input and E_raw are linear functionals
    # of the same per-group empirical probability vectors p_g!
    if not symmetries or sv_result is None or not sv_result.get("projector_exact"):
        total_v = 0.0
        for grp, counts in zip(groups, target_counts):
            x_raw = group_observable_vector(grp["terms"], n_qubits)
            p_meas, total = counts_to_prob_vector(counts, n_qubits)
            if readout_inverse is not None:
                m_inv = np.asarray(readout_inverse, dtype=float)
                eff_input = m_inv.T @ x_raw
            else:
                eff_input = x_raw
            eff_diff = float(alpha) * eff_input - x_raw
            mu = float(np.dot(eff_diff, p_meas))
            v_single = max(0.0, float(np.dot(eff_diff * eff_diff, p_meas)) - mu * mu)
            denom = float(total - 1) if total > 1 else float(total)
            v_shot = v_single / denom
            v_cal = 0.0
            if (
                readout_inverse is not None
                and assignment_matrix is not None
                and cal_shots_per_state is not None
            ):
                # Calibration noise only enters alpha * E_input (not E_raw)
                _, _, meta = linear_functional_mean_and_var(
                    float(alpha) * x_raw, counts, n_qubits,
                    readout_inverse=readout_inverse,
                    assignment_matrix=assignment_matrix,
                    cal_shots_per_state=cal_shots_per_state,
                )
                v_cal = meta["var_cal"]
            total_v += v_shot + v_cal
        return float(total_v)

    # Exact delta-method linearization for alpha * (N/D) - E_raw on shared counts:
    # delta(alpha * N/D - E_raw) = (alpha/D) * delta(N - E_sv * D) - delta(E_raw)
    from .symmetry import _expand_symmetry_group, _term_compatible_group_index, multiply_paulis

    z_idx = next(i for i, g in enumerate(groups) if g.get("is_z_basis"))
    sym_group = _expand_symmetry_group(symmetries, n_qubits)
    g_order = float(len(sym_group))
    d_const = 0.0
    d_terms: dict[str, float] = {}
    for sign, gp in sym_group:
        w = float(sign) / g_order
        if set(gp) <= {"I"}:
            d_const += w
        else:
            d_terms[gp] = d_terms.get(gp, 0.0) + w
    d_vec = d_const * np.ones(1 << n_qubits, dtype=float)
    if d_terms:
        d_vec = d_vec + group_observable_vector(d_terms, n_qubits)

    num_by_group: list[dict[str, float]] = [{} for _ in groups]
    for term, coeff in coeffs.items():
        c = float(coeff)
        if abs(c) == 0.0:
            continue
        t = str(term).strip().upper()
        for sign, gp in sym_group:
            phase, prod = multiply_paulis(t, gp)
            w = c * float(sign) * float(phase.real) / g_order
            gi = _term_compatible_group_index(prod, groups)
            if gi is not None and gi >= 0:
                num_by_group[gi][prod] = num_by_group[gi].get(prod, 0.0) + w

    e_sv = float(sv_result["energy"])
    d_val = float(sv_result["denominator"])
    scale = float(alpha) / d_val
    m_inv = np.asarray(readout_inverse, dtype=float) if readout_inverse is not None else None

    total_v = 0.0
    for idx, (grp, counts) in enumerate(zip(groups, target_counts)):
        x_raw = group_observable_vector(grp["terms"], n_qubits)
        x_num = (
            group_observable_vector(num_by_group[idx], n_qubits)
            if num_by_group[idx]
            else np.zeros(1 << n_qubits, dtype=float)
        )
        y_sv = scale * (x_num - (e_sv * d_vec if idx == z_idx else 0.0))
        eff_sv = (m_inv.T @ y_sv) if m_inv is not None else y_sv
        eff_diff = eff_sv - x_raw
        p_meas, total = counts_to_prob_vector(counts, n_qubits)
        mu = float(np.dot(eff_diff, p_meas))
        v_single = max(0.0, float(np.dot(eff_diff * eff_diff, p_meas)) - mu * mu)
        denom = float(total - 1) if total > 1 else float(total)
        v_shot = v_single / denom
        v_cal = 0.0
        if (
            m_inv is not None
            and assignment_matrix is not None
            and cal_shots_per_state is not None
        ):
            _, _, meta = linear_functional_mean_and_var(
                y_sv, counts, n_qubits,
                readout_inverse=m_inv,
                assignment_matrix=assignment_matrix,
                cal_shots_per_state=cal_shots_per_state,
            )
            v_cal = meta["var_cal"]
        total_v += v_shot + v_cal
    return float(total_v)


def evaluate_cdr_batch(
    grouped: dict[str, Any],
    coeffs: dict[str, float],
    target_group_counts: list[dict[str, int]],
    train_group_counts: list[list[dict[str, int]]],
    train_exact_energies: list[float],
    *,
    constant_energy: float = 0.0,
    fci_exact_energy: float | None = None,
    readout_cal_counts: list[dict[str, int]] | None = None,
    diagonal_readout_factors: dict[str, float] | None = None,
    symmetries: list[dict[str, Any]] | None = None,
    train_sector_aligned: bool = True,
    max_readout_cond: float = 50.0,
) -> dict[str, Any]:
    """Evaluate the complete 4-layer same-batch ablation and ODR CDR estimator.

    Layers:
      1. ``raw_unmitigated`` — QWC counts with exact joint covariance.
      2. ``live_readout_mitigated`` — full ``2^n x 2^n`` live readout matrix
         inverse (or diagonal TREX when ``readout_cal_counts`` is omitted).
      3. ``symmetry_verified`` — exact Bonet-Monroig ``Z2`` symmetry projection
         on top of layer 2 (when ``symmetries`` are present).
      4. ``cdr_mitigated`` — Deming/EIV linear CDR fit trained on the exact same
         preprocessing pipeline as the target circuit.
    """
    n_qubits = int(grouped["n_qubits"])

    # Layer 1: Raw unmitigated (exact joint QWC covariance)
    raw_res = estimate_energy_from_qwc_counts(grouped, target_group_counts)

    # Layer 2: Live 2^n x 2^n readout calibration matrix (or diagonal fallback)
    readout_cal_meta: dict[str, Any] | None = None
    m_mat: np.ndarray | None = None
    m_inv: np.ndarray | None = None
    cal_shots: list[int] | None = None
    if readout_cal_counts is not None:
        cal_res = build_assignment_matrix(
            readout_cal_counts, n_qubits, max_condition_number=max_readout_cond,
        )
        if not cal_res["ok"]:
            return {"ok": False, "error": cal_res["error"], "readout_calibration": cal_res}
        m_mat = cal_res["assignment_matrix"]
        m_inv = cal_res["inverse_matrix"]
        cal_shots = cal_res["shots_per_state"]
        readout_cal_meta = {
            "method": "live_assignment_matrix",
            "applied": True,
            "n_qubits": n_qubits,
            "condition_number": cal_res["condition_number"],
            "shots_per_state": cal_shots,
            "diagonal_fidelities": cal_res["diagonal_fidelities"],
            "mean_state_error": cal_res["mean_state_error"],
            "per_qubit_asymmetry": cal_res["per_qubit_asymmetry"],
            "assignment_matrix": cal_res["assignment_matrix_list"],
        }
        ro_res = estimate_energy_from_qwc_counts(
            grouped, target_group_counts,
            readout_inverse=m_inv,
            assignment_matrix=m_mat,
            cal_shots_per_state=cal_shots,
        )
    elif diagonal_readout_factors:
        readout_cal_meta = {
            "method": "diagonal_trex_fallback",
            "applied": True,
            "factors": diagonal_readout_factors,
        }
        ro_res = estimate_energy_from_qwc_counts(
            grouped, target_group_counts,
            diagonal_readout_factors=diagonal_readout_factors,
        )
    else:
        readout_cal_meta = {"method": "none", "applied": False}
        ro_res = raw_res

    # Layer 3: Z2 symmetry verification (exact Bonet-Monroig projector)
    sv_res: dict[str, Any] | None = None
    use_sym_for_cdr = False
    if symmetries:
        sv_res = symmetry_verified_energy(
            grouped, target_group_counts, coeffs, symmetries,
            readout_inverse=m_inv,
            assignment_matrix=m_mat,
            cal_shots_per_state=cal_shots,
        )
        use_sym_for_cdr = bool(train_sector_aligned)

    cdr_input_res = sv_res if (sv_res is not None and use_sym_for_cdr) else ro_res
    cdr_input_layer = "symmetry_verified" if (sv_res is not None and use_sym_for_cdr) else "live_readout_mitigated"

    # Evaluate every training circuit through the IDENTICAL preprocessing pipeline!
    train_noisy: list[float] = []
    train_ses: list[float] = []
    for t_counts in train_group_counts:
        if use_sym_for_cdr and symmetries:
            tm = symmetry_verified_energy(
                grouped, t_counts, coeffs, symmetries,
                readout_inverse=m_inv,
                assignment_matrix=m_mat,
                cal_shots_per_state=cal_shots,
            )
        elif m_inv is not None:
            tm = estimate_energy_from_qwc_counts(
                grouped, t_counts,
                readout_inverse=m_inv,
                assignment_matrix=m_mat,
                cal_shots_per_state=cal_shots,
            )
        elif diagonal_readout_factors:
            tm = estimate_energy_from_qwc_counts(
                grouped, t_counts,
                diagonal_readout_factors=diagonal_readout_factors,
            )
        else:
            tm = estimate_energy_from_qwc_counts(grouped, t_counts)
        train_noisy.append(float(tm["energy"]))
        # Within one batch, readout-calibration shot noise is common-mode across
        # all training circuits and the target (absorbed by CDR's affine fit);
        # use per-circuit independent shot SE for WLS weights and chi2_reduced.
        indep_var = tm.get("variance_shot")
        train_ses.append(
            float(math.sqrt(max(0.0, float(indep_var))))
            if indep_var is not None
            else float(tm["uncertainty"])
        )

    target_indep_var = cdr_input_res.get("variance_shot")
    target_cdr_se = (
        float(math.sqrt(max(0.0, float(target_indep_var))))
        if target_indep_var is not None
        else float(cdr_input_res["uncertainty"])
    )

    # Preliminary fit to get alpha for exact coupled raw-difference covariance
    prelim = fit_cdr_odr(
        train_noisy, train_exact_energies, train_ses,
        target_noisy=float(cdr_input_res["energy"]),
        target_uncertainty=target_cdr_se,
        raw_baseline_energy=float(raw_res["energy"]),
        raw_baseline_uncertainty=float(raw_res["uncertainty"]),
    )
    coupled_var = _same_batch_coupled_diff_var(
        grouped, target_group_counts, coeffs, float(prelim["slope"]),
        readout_inverse=m_inv,
        assignment_matrix=None,  # shared M^{-1} error cancels in same-batch CDR
        cal_shots_per_state=None,
        symmetries=(symmetries if use_sym_for_cdr else None),
        sv_result=(sv_res if use_sym_for_cdr else None),
    )
    cdr_fit = fit_cdr_odr(
        train_noisy, train_exact_energies, train_ses,
        target_noisy=float(cdr_input_res["energy"]),
        target_uncertainty=target_cdr_se,
        raw_baseline_energy=float(raw_res["energy"]),
        raw_baseline_uncertainty=float(raw_res["uncertainty"]),
        input_vs_raw_diff_var=(coupled_var if coupled_var >= 0.0 else None),
    )

    c_e = float(constant_energy)
    def _layer_entry(name: str, elec_e: float, se: float, extra: dict[str, Any] | None = None) -> dict[str, Any]:
        tot_e = float(elec_e + c_e)
        entry: dict[str, Any] = {
            "layer": name,
            "electronic_energy": float(elec_e),
            "energy": tot_e,
            "uncertainty": float(se),
            "ci95_mha": float(Z95 * se * 1000.0),
        }
        if fci_exact_energy is not None:
            entry["delta_mha"] = float(abs(tot_e - float(fci_exact_energy)) * 1000.0)
        if extra:
            entry.update(extra)
        return entry

    ablation_layers: dict[str, Any] = {
        "raw_unmitigated": _layer_entry("raw_unmitigated", raw_res["energy"], raw_res["uncertainty"]),
        "live_readout_mitigated": _layer_entry(
            "live_readout_mitigated", ro_res["energy"], ro_res["uncertainty"],
            {"method": readout_cal_meta["method"]},
        ),
        "symmetry_verified": (
            _layer_entry(
                "symmetry_verified", sv_res["energy"], sv_res["uncertainty"],
                {
                    "method": sv_res["method"],
                    "projector_exact": sv_res["projector_exact"],
                    "survival_probability": sv_res["survival_probability"],
                    "raw_z_acceptance_rate": sv_res["raw_z_acceptance_rate"],
                },
            )
            if sv_res is not None
            else None
        ),
        "cdr_mitigated": _layer_entry(
            "cdr_mitigated", cdr_fit["mitigated"], cdr_fit["uncertainty"],
            {"input_layer": cdr_input_layer, "model_adequate": cdr_fit["model_adequate"]},
        ),
    }

    return {
        "ok": True,
        "cdr": cdr_fit,
        "cdr_input_layer": cdr_input_layer,
        "train_noisy_energies": train_noisy,
        "train_noisy_uncertainties": train_ses,
        "train_exact_energies": [float(e) for e in train_exact_energies],
        "raw": raw_res,
        "readout_mitigated": ro_res,
        "symmetry_verified": sv_res,
        "readout_calibration": readout_cal_meta,
        "ablation_layers": ablation_layers,
    }


__all__ = [
    "build_assignment_matrix",
    "build_readout_cal_circuits",
    "evaluate_cdr_batch",
    "fit_cdr_odr",
    "generate_cdr_training_circuits",
]
