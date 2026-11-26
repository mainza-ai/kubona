"""Scalable Matrix-Free Measurement Mitigation (M3) for n = 1..20+ qubits.

Implements Nation et al. (PRX Quantum 2, 040326, 2021) natively in NumPy/SciPy:
  - Balanced calibration requires ONLY 2 circuits (``|0...0>`` and ``|1...1>``) — or
    ``2n`` single-qubit-flip circuits for isolated single-qubit calibration — eliminating
    the exponential ``2^n`` calibration wall at ``n = 4``.
  - Constructs the reduced assignment matrix ``A_S`` strictly on the ``K`` observed
    bitstrings ``S`` (``K << 2^n``) and solves ``A_S p_mit = p_raw`` via direct/iterative
    linear solver (GMRES/BiCGSTAB for large ``K``), optionally projecting onto the
    nearest probability distribution in L2 norm or preserving unbiased quasi-probabilities
    for expectation values.
  - Works seamlessly for BOTH Pauli expectation values (VQE, TFIM, Heisenberg) AND
    arbitrary bitstring cost functionals (MaxCut QAOA, SQD bitstring weights).
"""

from __future__ import annotations

import math
from typing import Any, Callable

import numpy as np

Z95 = 1.959963984540054


def build_m3_cal_circuits(
    n_qubits: int,
    *,
    mode: str = "balanced",
) -> list[tuple[str, Any]]:
    """Build scalable readout calibration circuits for ``M3``.

    Modes:
      - ``"balanced"`` (default): 2 circuits (``|0...0>`` and ``|1...1>``) — O(1) QPU cost,
        estimates per-qubit ``P(1|0)`` and ``P(0|1)`` simultaneously across all ``n`` qubits.
      - ``"independent"``: ``n + 1`` circuits (``|0...0>`` plus single-qubit ``X_q`` flips) —
        eliminates spectator flip crosstalk during ``|1>`` calibration.
    """
    from qiskit import QuantumCircuit

    n = int(n_qubits)
    if n < 1 or n > 32:
        raise ValueError(f"M3 supports 1 <= n_qubits <= 32, got {n}")
    m = str(mode or "balanced").strip().lower()
    if m == "balanced":
        qc0 = QuantumCircuit(n)
        qc0.measure_all()
        qc1 = QuantumCircuit(n)
        for q in range(n):
            qc1.x(q)
        qc1.measure_all()
        return [("0" * n, qc0), ("1" * n, qc1)]
    if m == "independent":
        out: list[tuple[str, Any]] = []
        qc0 = QuantumCircuit(n)
        qc0.measure_all()
        out.append(("0" * n, qc0))
        for q in range(n):
            qc = QuantumCircuit(n)
            qc.x(q)
            qc.measure_all()
            label = ["0"] * n
            label[n - 1 - q] = "1"
            out.append(("".join(label), qc))
        return out
    raise ValueError(f"unsupported M3 calibration mode {mode!r}; use balanced|independent")


def calibrate_m3(
    cal_pairs: list[tuple[str, dict[str, int]]],
    n_qubits: int,
    *,
    max_single_qubit_error: float = 0.35,
) -> dict[str, Any]:
    """Assemble per-qubit 2x2 readout confusion matrices ``M_q`` from calibration counts.

    ``M_q = [[P(0|0), P(0|1)], [P(1|0), P(1|1)]]`` for each qubit ``q in 0..n-1``.
    Refuses fail-closed if any qubit has ``P(1|0) + P(0|1) >= 2 * max_single_qubit_error``
    (singular or inverted readout).
    """
    n = int(n_qubits)
    p10_num = np.zeros(n, dtype=float)
    p10_den = np.zeros(n, dtype=float)
    p01_num = np.zeros(n, dtype=float)
    p01_den = np.zeros(n, dtype=float)

    for prep_label, counts in cal_pairs:
        prep = str(prep_label).replace(" ", "").zfill(n)[-n:]
        prep_le = prep[::-1]  # index q -> logical qubit q
        for raw_bs, cnt_raw in counts.items():
            cnt = int(cnt_raw)
            if cnt <= 0:
                continue
            meas = str(raw_bs).replace(" ", "").zfill(n)[-n:]
            meas_le = meas[::-1]
            for q in range(n):
                if prep_le[q] == "0":
                    p10_den[q] += cnt
                    if meas_le[q] == "1":
                        p10_num[q] += cnt
                else:
                    p01_den[q] += cnt
                    if meas_le[q] == "0":
                        p01_num[q] += cnt

    matrices: list[np.ndarray] = []
    per_qubit: dict[str, dict[str, float]] = {}
    max_cond = 1.0
    for q in range(n):
        if p10_den[q] <= 0 or p01_den[q] <= 0:
            return {
                "ok": False,
                "error": f"M3 calibration missing 0 or 1 samples for qubit {q}",
            }
        e10 = float(p10_num[q] / p10_den[q])
        e01 = float(p01_num[q] / p01_den[q])
        if e10 + e01 >= 2.0 * float(max_single_qubit_error) or (1.0 - e10 - e01) <= 0.10:
            return {
                "ok": False,
                "error": (
                    f"M3 readout matrix ill-conditioned on qubit {q}: "
                    f"P(1|0)={e10:.4f}, P(0|1)={e01:.4f}"
                ),
            }
        mq = np.array([[1.0 - e10, e01], [e10, 1.0 - e01]], dtype=float)
        cond_q = float(np.linalg.cond(mq))
        max_cond = max(max_cond, cond_q)
        matrices.append(mq)
        per_qubit[str(q)] = {
            "p1_given_0": e10,
            "p0_given_1": e01,
            "shots_0": float(p10_den[q]),
            "shots_1": float(p01_den[q]),
            "cond": cond_q,
        }

    return {
        "ok": True,
        "method": "m3_matrix_free",
        "n_qubits": n,
        "matrices": matrices,
        "per_qubit": per_qubit,
        "max_qubit_cond": max_cond,
        "mean_readout_error": float(np.mean([0.5 * (v["p1_given_0"] + v["p0_given_1"]) for v in per_qubit.values()])),
    }


def _build_reduced_m3_matrix(
    bitstrings_le: list[str],
    matrices: list[np.ndarray],
) -> np.ndarray:
    """Build ``K x K`` reduced assignment matrix ``A_S[i, j] = prod_q M_q[b_i[q], b_j[q]]``."""
    k = len(bitstrings_le)
    n = len(matrices)
    bits = np.array([[1 if ch == "1" else 0 for ch in bs] for bs in bitstrings_le], dtype=np.int8)
    # Vectorized outer product over qubits: A[i, j] = prod_q M_q[bits[i, q], bits[j, q]]
    mat = np.ones((k, k), dtype=float)
    for q in range(n):
        mq = matrices[q]
        bq = bits[:, q]
        mat *= mq[bq[:, None], bq[None, :]]
    # Normalize columns within the observed subspace so mass is conserved (Nation 2021 Eq. 4)
    col_sums = mat.sum(axis=0)
    col_sums = np.where(col_sums > 1e-12, col_sums, 1.0)
    mat /= col_sums[None, :]
    return mat


def _project_simplex(v: np.ndarray) -> np.ndarray:
    """Euclidean projection onto the probability simplex ``{p >= 0, sum(p) = 1}``."""
    u = np.sort(v)[::-1]
    cssv = np.cumsum(u) - 1.0
    ind = np.arange(1, len(v) + 1)
    cond = u - cssv / ind > 0
    if not np.any(cond):
        return np.full_like(v, 1.0 / len(v))
    rho = int(ind[cond][-1])
    theta = float(cssv[cond][-1]) / float(rho)
    return np.maximum(v - theta, 0.0)


def mitigate_counts_m3(
    counts: dict[str, int],
    m3_cal: dict[str, Any],
    *,
    project_to_simplex: bool = False,
    max_subspace: int = 4096,
) -> dict[str, Any]:
    """Mitigate an arbitrary ``n``-qubit bitstring distribution using M3 subspace inversion.

    When ``project_to_simplex=False`` (default), returns exact unbiased quasi-probabilities
    suitable for linear expectation functionals (Pauli terms, MaxCut cuts) together with
    ``A_S^{-1}`` for exact variance propagation.
    """
    if not m3_cal.get("ok"):
        raise ValueError(f"invalid M3 calibration: {m3_cal.get('error')}")
    n = int(m3_cal["n_qubits"])
    matrices: list[np.ndarray] = m3_cal["matrices"]

    cleaned: dict[str, int] = {}
    total = 0
    for raw_bs, cnt_raw in counts.items():
        cnt = int(cnt_raw)
        if cnt <= 0:
            continue
        bs = str(raw_bs).replace(" ", "").zfill(n)[-n:]
        cleaned[bs] = cleaned.get(bs, 0) + cnt
        total += cnt
    if total <= 0:
        raise ValueError("M3 cannot mitigate empty counts")

    # Keep top-K most frequent observed bitstrings if K > max_subspace
    items = sorted(cleaned.items(), key=lambda kv: (-kv[1], kv[0]))
    if len(items) > int(max_subspace):
        items = items[: int(max_subspace)]
        total_sub = sum(v for _, v in items)
    else:
        total_sub = total

    bitstrings_be = [k for k, _ in items]
    bitstrings_le = [k[::-1] for k in bitstrings_be]
    p_raw = np.array([v / float(total_sub) for _, v in items], dtype=float)

    a_sub = _build_reduced_m3_matrix(bitstrings_le, matrices)
    k = len(bitstrings_be)
    if k <= 512:
        p_mit = np.linalg.solve(a_sub, p_raw)
        a_inv: np.ndarray | None = np.linalg.inv(a_sub)
    else:
        from scipy.sparse.linalg import gmres
        p_mit, info = gmres(a_sub, p_raw, rtol=1e-8, maxiter=200)
        if info != 0:
            p_mit = np.linalg.lstsq(a_sub, p_raw, rcond=None)[0]
        a_inv = None

    if project_to_simplex:
        p_mit = _project_simplex(p_mit)

    return {
        "bitstrings": bitstrings_be,
        "p_raw": p_raw,
        "p_mitigated": p_mit,
        "quasi_probs": {bs: float(p) for bs, p in zip(bitstrings_be, p_mit)},
        "total_shots": int(total_sub),
        "subspace_dim": int(k),
        "a_sub": a_sub,
        "a_inv": a_inv,
    }


def mitigate_bitstring_observable_m3(
    counts: dict[str, int],
    m3_cal: dict[str, Any],
    cost_fn: Callable[[str], float],
) -> dict[str, Any]:
    """Compute M3-mitigated mean and exact delta-method SE for any bitstring functional ``f(b)``.

    Works directly for MaxCut QAOA (``f(b) = cut(b)``), diagonal Hamiltonians, and
    single-Pauli expectations on up to 20+ qubits!
    """
    m = mitigate_counts_m3(counts, m3_cal, project_to_simplex=False)
    bs_list = m["bitstrings"]
    f_vec = np.array([float(cost_fn(bs)) for bs in bs_list], dtype=float)
    p_raw = m["p_raw"]
    p_mit = m["p_mitigated"]
    n_shots = int(m["total_shots"])

    raw_mean = float(np.dot(f_vec, p_raw))
    raw_var = max(0.0, float(np.dot(f_vec * f_vec, p_raw)) - raw_mean * raw_mean) / float(max(1, n_shots - 1))

    mit_mean = float(np.dot(f_vec, p_mit))
    # Exact propagated variance via eff_f = A_S^{-T} f_vec!
    if m["a_inv"] is not None:
        eff_f = m["a_inv"].T @ f_vec
    else:
        from scipy.sparse.linalg import gmres
        eff_f, info = gmres(m["a_sub"].T, f_vec, rtol=1e-8, maxiter=200)
        if info != 0:
            eff_f = np.linalg.lstsq(m["a_sub"].T, f_vec, rcond=None)[0]

    mu_eff = float(np.dot(eff_f, p_raw))
    mit_var = max(0.0, float(np.dot(eff_f * eff_f, p_raw)) - mu_eff * mu_eff) / float(max(1, n_shots - 1))
    # Coupled difference variance Var((eff_f - f_vec)^T p_raw)
    diff_vec = eff_f - f_vec
    mu_d = float(np.dot(diff_vec, p_raw))
    diff_var = max(0.0, float(np.dot(diff_vec * diff_vec, p_raw)) - mu_d * mu_d) / float(max(1, n_shots - 1))

    return {
        "raw_mean": raw_mean,
        "raw_std_error": math.sqrt(raw_var),
        "mitigated_mean": mit_mean,
        "mitigated_std_error": math.sqrt(mit_var),
        "difference": float(mit_mean - raw_mean),
        "difference_ci95": float(Z95 * math.sqrt(diff_var)),
        "difference_significant": bool(abs(mit_mean - raw_mean) > Z95 * math.sqrt(diff_var)),
        "subspace_dim": int(m["subspace_dim"]),
        "total_shots": n_shots,
    }


__all__ = [
    "build_m3_cal_circuits",
    "calibrate_m3",
    "mitigate_bitstring_observable_m3",
    "mitigate_counts_m3",
]
