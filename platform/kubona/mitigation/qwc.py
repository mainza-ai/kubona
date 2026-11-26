"""Qubit-wise commuting (QWC) Pauli grouping, joint covariance, and Neyman shots.

Follows Qiskit's little-endian bitstring convention used across Kubona:
``term[-1]`` acts on qubit 0 (LSB), ``term[0]`` acts on qubit ``n - 1`` (MSB).
Constant identity terms (``II...I``) are folded into ``identity_offset`` so zero
quantum circuits are wasted measuring the identity operator.
"""

from __future__ import annotations

import math
from typing import Any

import numpy as np


def _is_identity(term: str) -> bool:
    return bool(term) and set(term) <= {"I"}


def _qwc_compatible(basis: str, term: str) -> bool:
    if len(basis) != len(term):
        return False
    for b, t in zip(basis, term):
        if b != "I" and t != "I" and b != t:
            return False
    return True


def _merge_basis(basis: str, term: str) -> str:
    return "".join(t if b == "I" else b for b, t in zip(basis, term))


def group_pauli_terms(coeffs: dict[str, float]) -> dict[str, Any]:
    """Partition Pauli Hamiltonian terms into deterministic QWC measurement groups.

    Returns ``{"identity_offset": float, "identity_terms": dict, "groups": list}``.
    Each group has ``{"basis": str, "terms": dict[str, float], "l1_weight": float,
    "l2_weight": float}``.
    """
    identity_offset = 0.0
    identity_terms: dict[str, float] = {}
    non_id: list[tuple[str, float]] = []
    n_qubits: int | None = None
    for term, coeff in coeffs.items():
        t = str(term).strip().upper()
        c = float(coeff)
        if not t or any(ch not in ("I", "X", "Y", "Z") for ch in t):
            raise ValueError(f"invalid Pauli string {term!r}")
        if n_qubits is None:
            n_qubits = len(t)
        elif len(t) != n_qubits:
            raise ValueError(f"Pauli string length mismatch: {t!r} vs {n_qubits}")
        if _is_identity(t):
            identity_offset += c
            identity_terms[t] = c
        elif abs(c) > 0.0:
            non_id.append((t, c))

    # Deterministic largest-first greedy grouping (ties broken lexicographically).
    non_id.sort(key=lambda item: (-abs(item[1]), item[0]))
    groups: list[dict[str, Any]] = []
    for term, coeff in non_id:
        placed = False
        for grp in groups:
            if _qwc_compatible(grp["basis"], term):
                grp["basis"] = _merge_basis(grp["basis"], term)
                grp["terms"][term] = coeff
                placed = True
                break
        if not placed:
            groups.append({"basis": term, "terms": {term: coeff}})

    # Order groups deterministically: Z-only basis first (used for Z2 symmetry),
    # then by descending L1 weight, then basis name.
    for grp in groups:
        grp["l1_weight"] = float(sum(abs(c) for c in grp["terms"].values()))
        grp["l2_weight"] = float(math.sqrt(sum(c * c for c in grp["terms"].values())))
        grp["is_z_basis"] = set(grp["basis"]) <= {"I", "Z"}
    groups.sort(key=lambda g: (0 if g["is_z_basis"] else 1, -g["l1_weight"], g["basis"]))
    return {
        "n_qubits": int(n_qubits or 0),
        "identity_offset": float(identity_offset),
        "identity_terms": identity_terms,
        "groups": groups,
    }


def measurement_circuit_for_basis(qc: Any, basis: str) -> Any:
    """Return a copy of ``qc`` rotated into the QWC measurement basis with ``measure_all``."""
    measured = qc.copy()
    rev_basis = str(basis)[::-1]
    for q, ch in enumerate(rev_basis):
        if ch == "X":
            measured.h(q)
        elif ch == "Y":
            measured.sdg(q)
            measured.h(q)
        elif ch not in ("I", "Z"):
            raise ValueError(f"unsupported Pauli basis character {ch!r} in {basis!r}")
    measured.measure_all()
    return measured


def pauli_parity_eigenvalue(term: str, bitstring: str) -> int:
    """Return ±1 eigenvalue of Pauli ``term`` on Qiskit big-endian ``bitstring``."""
    bits = bitstring.replace(" ", "")[::-1]
    rev_term = term[::-1]
    parity = 0
    for q, ch in enumerate(rev_term):
        if ch != "I" and q < len(bits) and bits[q] == "1":
            parity += 1
    return 1 if (parity % 2 == 0) else -1


def _bitstring_to_index(bitstring: str, n_qubits: int) -> int:
    clean = bitstring.replace(" ", "")
    if len(clean) < n_qubits:
        clean = clean.zfill(n_qubits)
    return int(clean[-n_qubits:], 2)


def _index_to_bitstring(idx: int, n_qubits: int) -> str:
    return format(idx, f"0{n_qubits}b")


def group_observable_vector(
    terms: dict[str, float],
    n_qubits: int,
    diagonal_readout_factors: dict[str, float] | None = None,
) -> np.ndarray:
    """Build length-2^n vector ``x[b] = sum_{P in terms} (c_P / f_P) * s_P(b)``."""
    dim = 1 << n_qubits
    vec = np.zeros(dim, dtype=float)
    for idx in range(dim):
        bs = _index_to_bitstring(idx, n_qubits)
        val = 0.0
        for term, coeff in terms.items():
            f = 1.0
            if diagonal_readout_factors and term in diagonal_readout_factors:
                f = float(diagonal_readout_factors[term])
                if abs(f) < 1e-9:
                    f = 1.0
            val += (float(coeff) / f) * pauli_parity_eigenvalue(term, bs)
        vec[idx] = val
    return vec


def counts_to_prob_vector(counts: dict[str, int], n_qubits: int) -> tuple[np.ndarray, int]:
    """Convert Qiskit counts dict to probability vector over 0..2^n-1 and total shots."""
    dim = 1 << n_qubits
    vec = np.zeros(dim, dtype=float)
    total = 0
    for bs, n in counts.items():
        cnt = int(n)
        if cnt <= 0:
            continue
        idx = _bitstring_to_index(str(bs), n_qubits)
        vec[idx] += cnt
        total += cnt
    if total <= 0:
        raise ValueError("QWC group measurement returned zero counts")
    return vec / float(total), total


def linear_functional_mean_and_var(
    x_vec: np.ndarray,
    counts: dict[str, int],
    n_qubits: int,
    readout_inverse: np.ndarray | None = None,
    assignment_matrix: np.ndarray | None = None,
    cal_shots_per_state: list[int] | None = None,
) -> tuple[float, float, dict[str, float]]:
    """Exact mean and variance of ``x^T p`` (raw) or ``x^T M^{-1} p`` (readout-mitigated).

    Propagates full intra-group covariance across all co-measured Pauli terms
    and, when ``assignment_matrix`` and ``cal_shots_per_state`` are provided,
    adds first-order calibration-matrix shot variance.
    """
    p_meas, total = counts_to_prob_vector(counts, n_qubits)
    if readout_inverse is not None:
        m_inv = np.asarray(readout_inverse, dtype=float)
        eff_x = m_inv.T @ x_vec
    else:
        eff_x = np.asarray(x_vec, dtype=float)

    mean = float(np.dot(eff_x, p_meas))
    second_moment = float(np.dot(eff_x * eff_x, p_meas))
    var_single = max(0.0, second_moment - mean * mean)
    denom = float(total - 1) if total > 1 else float(total)
    var_shot = var_single / denom

    var_cal = 0.0
    if (
        readout_inverse is not None
        and assignment_matrix is not None
        and cal_shots_per_state is not None
    ):
        m_mat = np.asarray(assignment_matrix, dtype=float)
        p_mit = np.asarray(readout_inverse, dtype=float) @ p_meas
        for j, n_cal in enumerate(cal_shots_per_state):
            if n_cal is None or int(n_cal) <= 1 or j >= m_mat.shape[1]:
                continue
            col = np.clip(m_mat[:, j], 0.0, 1.0)
            col_sum = float(np.sum(col))
            if col_sum <= 0:
                continue
            col = col / col_sum
            mu_j = float(np.dot(eff_x, col))
            v_j = max(0.0, float(np.dot(eff_x * eff_x, col)) - mu_j * mu_j)
            var_cal += (float(p_mit[j]) ** 2) * v_j / float(int(n_cal) - 1)

    return mean, float(var_shot + var_cal), {
        "var_shot": float(var_shot),
        "var_cal": float(var_cal),
        "shots": float(total),
    }


def estimate_energy_from_qwc_counts(
    grouped: dict[str, Any],
    group_counts: list[dict[str, int]],
    *,
    readout_inverse: np.ndarray | None = None,
    assignment_matrix: np.ndarray | None = None,
    cal_shots_per_state: list[int] | None = None,
    diagonal_readout_factors: dict[str, float] | None = None,
) -> dict[str, Any]:
    """Compute exact QWC energy, joint-covariance SE, and per-term expectations."""
    groups = grouped["groups"]
    n_qubits = int(grouped["n_qubits"])
    if len(group_counts) != len(groups):
        raise ValueError(f"expected {len(groups)} QWC count dicts, got {len(group_counts)}")
    if readout_inverse is not None and diagonal_readout_factors is not None:
        raise ValueError("cannot combine full readout_inverse with diagonal_readout_factors")

    energy = float(grouped["identity_offset"])
    total_var = 0.0
    total_var_shot = 0.0
    total_var_cal = 0.0
    per_term: dict[str, dict[str, Any]] = {}
    for id_term, c in grouped.get("identity_terms", {}).items():
        per_term[id_term] = {
            "coefficient": float(c),
            "expectation": 1.0,
            "raw_expectation": 1.0,
            "std_error": 0.0,
            "basis": "I" * n_qubits,
        }

    group_summaries: list[dict[str, Any]] = []
    for grp, counts in zip(groups, group_counts):
        terms = grp["terms"]
        if n_qubits <= 10:
            x_vec = group_observable_vector(
                terms, n_qubits, diagonal_readout_factors=diagonal_readout_factors,
            )
            g_mean, g_var, g_meta = linear_functional_mean_and_var(
                x_vec, counts, n_qubits,
                readout_inverse=readout_inverse,
                assignment_matrix=assignment_matrix,
                cal_shots_per_state=cal_shots_per_state,
            )
            p_meas, total = counts_to_prob_vector(counts, n_qubits)
            p_eff = (np.asarray(readout_inverse, dtype=float) @ p_meas) if readout_inverse is not None else p_meas
            for term, coeff in terms.items():
                t_vec = group_observable_vector({term: 1.0}, n_qubits)
                raw_exp = float(np.dot(t_vec, p_meas))
                if readout_inverse is not None:
                    exp_val = float(np.dot(t_vec, p_eff))
                    _, t_var, _ = linear_functional_mean_and_var(
                        t_vec, counts, n_qubits,
                        readout_inverse=readout_inverse,
                        assignment_matrix=assignment_matrix,
                        cal_shots_per_state=cal_shots_per_state,
                    )
                    se_val = math.sqrt(max(0.0, t_var))
                else:
                    f = 1.0
                    if diagonal_readout_factors and term in diagonal_readout_factors:
                        f = float(diagonal_readout_factors[term]) or 1.0
                    exp_val = raw_exp / f
                    se_val = math.sqrt(max(0.0, 1.0 - raw_exp * raw_exp) / float(total)) / abs(f)
                per_term[term] = {
                    "coefficient": float(coeff),
                    "expectation": float(exp_val),
                    "raw_expectation": float(raw_exp),
                    "std_error": float(se_val),
                    "basis": grp["basis"],
                }
        else:
            # Large-n fallback (no dense 2^n vector, exact sample variance over observed bitstrings).
            if readout_inverse is not None:
                raise ValueError(f"full 2^n readout matrix inversion unsupported for n_qubits={n_qubits} > 10")
            total = sum(int(v) for v in counts.values() if int(v) > 0)
            if total <= 0:
                raise ValueError("QWC group measurement returned zero counts")
            vals: list[tuple[float, int]] = []
            for bs, n in counts.items():
                cnt = int(n)
                if cnt <= 0:
                    continue
                x_b = 0.0
                for term, coeff in terms.items():
                    f = 1.0
                    if diagonal_readout_factors and term in diagonal_readout_factors:
                        f = float(diagonal_readout_factors[term]) or 1.0
                    x_b += (float(coeff) / f) * pauli_parity_eigenvalue(term, str(bs))
                vals.append((x_b, cnt))
            g_mean = sum(x * n for x, n in vals) / float(total)
            second = sum(x * x * n for x, n in vals) / float(total)
            denom = float(total - 1) if total > 1 else float(total)
            g_var = max(0.0, second - g_mean * g_mean) / denom
            g_meta = {"var_shot": float(g_var), "var_cal": 0.0, "shots": float(total)}
            for term, coeff in terms.items():
                acc = sum(int(n) * pauli_parity_eigenvalue(term, str(bs)) for bs, n in counts.items() if int(n) > 0)
                raw_exp = acc / float(total)
                f = float(diagonal_readout_factors.get(term, 1.0)) if diagonal_readout_factors else 1.0
                exp_val = raw_exp / (f or 1.0)
                se_val = math.sqrt(max(0.0, 1.0 - raw_exp * raw_exp) / float(total)) / abs(f or 1.0)
                per_term[term] = {
                    "coefficient": float(coeff),
                    "expectation": float(exp_val),
                    "raw_expectation": float(raw_exp),
                    "std_error": float(se_val),
                    "basis": grp["basis"],
                }

        energy += g_mean
        total_var += g_var
        total_var_shot += g_meta["var_shot"]
        total_var_cal += g_meta["var_cal"]
        group_summaries.append({
            "basis": grp["basis"],
            "terms": sorted(terms.keys()),
            "shots": int(g_meta["shots"]),
            "energy_contribution": float(g_mean),
            "std_error": float(math.sqrt(max(0.0, g_var))),
        })

    return {
        "energy": float(energy),
        "uncertainty": float(math.sqrt(max(0.0, total_var))),
        "variance_shot": float(total_var_shot),
        "variance_cal": float(total_var_cal),
        "terms": per_term,
        "groups": group_summaries,
    }


def allocate_neyman_shots(
    groups: list[dict[str, Any]],
    total_shots: int,
    *,
    group_variances: list[float] | None = None,
    min_shots: int = 1024,
    quantum: int = 1,
) -> list[int]:
    """Allocate ``total_shots`` across QWC groups by Neyman optimal weighting.

    Uses ``sqrt(group_variances[i])`` when supplied, otherwise falls back to the
    group L2 coefficient norm ``l2_weight``. Every group receives at least
    ``min_shots`` (capped so the sum stays feasible). When ``quantum > 1`` and
    ``total_shots`` is a multiple of ``quantum``, all returned counts are exact
    multiples of ``quantum`` so uniform-shot Qiskit Runtime batches keep a large
    GCD and small PUB count.
    """
    m = len(groups)
    if m == 0:
        return []
    total = int(total_shots)
    q = max(1, int(quantum))
    if q > 1 and total % q == 0 and total >= q * m:
        min_units = max(1, min(int(math.ceil(int(min_shots) / q)), (total // q) // m))
        units = allocate_neyman_shots(
            groups,
            total // q,
            group_variances=group_variances,
            min_shots=min_units,
            quantum=1,
        )
        return [u * q for u in units]
    if total < m:
        raise ValueError(f"total_shots {total} must be >= number of groups {m}")
    floor = min(int(min_shots), total // m)
    remaining = total - floor * m

    weights: list[float] = []
    for i, grp in enumerate(groups):
        if group_variances is not None and i < len(group_variances) and group_variances[i] > 0:
            weights.append(math.sqrt(float(group_variances[i])))
        else:
            w = float(grp.get("l2_weight") or grp.get("l1_weight") or 1.0)
            weights.append(max(w, 1e-6))
    w_sum = sum(weights)
    if w_sum <= 0:
        weights = [1.0] * m
        w_sum = float(m)

    alloc = [floor + int(math.floor(remaining * w / w_sum)) for w in weights]
    leftover = total - sum(alloc)
    # Distribute rounding remainder to highest fractional parts deterministically.
    fracs = sorted(
        range(m),
        key=lambda i: (-(remaining * weights[i] / w_sum - math.floor(remaining * weights[i] / w_sum)), i),
    )
    for idx in fracs[:leftover]:
        alloc[idx] += 1
    return alloc


def estimate_circuit_qwc_variances(
    qc: Any,
    grouped: dict[str, Any],
    *,
    symmetry_protected_z: bool = False,
    z_noise_floor: float = 0.005,
    xy_noise_floor: float = 0.02,
) -> list[float]:
    """Estimate per-QWC-group single-shot observable variance from the exact statevector.

    Adds a small depolarizing floor (smaller on Z when ``Z2`` symmetry
    verification projects out single-bit-flip leakage) so low-variance
    computational-basis anchors still receive realistic shot budgets on noisy
    hardware. Falls back to group L2 coefficient variance for ``n_qubits > 10``.
    """
    groups = grouped["groups"]
    n_qubits = int(grouped["n_qubits"])
    if n_qubits > 10:
        return [max(1e-6, float(g.get("l2_weight") or 1.0) ** 2) for g in groups]

    from qiskit.quantum_info import Statevector

    variances: list[float] = []
    for grp in groups:
        basis = str(grp["basis"])
        mc = qc.copy()
        for q, ch in enumerate(basis[::-1]):
            if ch == "X":
                mc.h(q)
            elif ch == "Y":
                mc.sdg(q)
                mc.h(q)
        probs = np.abs(Statevector.from_instruction(mc).data) ** 2
        nf = float(z_noise_floor) if (symmetry_protected_z and grp.get("is_z_basis")) else float(xy_noise_floor)
        p_eff = (1.0 - nf) * probs + nf * (np.ones_like(probs) / float(len(probs)))
        x_vec = group_observable_vector(grp["terms"], n_qubits)
        mu = float(np.dot(x_vec, p_eff))
        var = max(1e-6, float(np.dot(x_vec * x_vec, p_eff) - mu * mu))
        variances.append(var)
    return variances

