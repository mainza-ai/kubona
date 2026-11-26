"""Quantum Subspace Expansion (QSE) for error mitigation and excited states.

Implements McClean et al. (PRA 95, 042308, 2017) & Colless et al. (PRX 8,
011021, 2018):
  - Given an operator expansion basis ``{E_0 = I, E_1, ..., E_{K-1}}`` (default:
    identity + single-qubit Pauli excitations), expands every matrix element
    ``H_{ij} = <psi| E_i^dag H E_j |psi>`` and ``S_{ij} = <psi| E_i^dag E_j |psi>``
    into Pauli strings, deduplicates across the entire ``K x K`` matrices, and
    groups them into minimal QWC measurement bases!
  - Solves the SVD-regularized generalized eigenvalue problem ``H c = E S c``
    (truncating overlap eigenvalues below ``s_min`` fail-closed to prevent
    noise-amplified spurious eigenvalues) to mitigate incoherent errors on the
    ground state AND recover excited-state energies.
"""

from __future__ import annotations

import math
from typing import Any

import numpy as np

from .qwc import group_pauli_terms
from .symmetry import multiply_paulis

Z95 = 1.959963984540054


def default_qse_operators(n_qubits: int, *, include_y: bool = False) -> list[str]:
    """Generate compact Pauli QSE operator basis ``{I...I, X_q, Z_q (, Y_q)}``."""
    n = int(n_qubits)
    ops = ["I" * n]
    for q in range(n):
        for ch in (("X", "Y", "Z") if include_y else ("X", "Z")):
            chars = ["I"] * n
            chars[n - 1 - q] = ch
            ops.append("".join(chars))
    return ops


def build_qse_plan(
    coeffs: dict[str, float],
    *,
    operators: list[str] | None = None,
) -> dict[str, Any]:
    """Compile the deduplicated QWC measurement plan for QSE ``H_{ij}`` and ``S_{ij}``."""
    if not coeffs:
        raise ValueError("QSE requires non-empty Hamiltonian pauli_coefficients")
    sample_term = next(iter(coeffs.keys()))
    n_qubits = len(str(sample_term).strip())
    ops = [str(o).strip().upper() for o in (operators or default_qse_operators(n_qubits))]
    k = len(ops)

    # Track complex linear combinations of Pauli expectations for each (i, j)
    h_terms: list[list[dict[str, complex]]] = [[{} for _ in range(k)] for _ in range(k)]
    s_terms: list[list[dict[str, complex]]] = [[{} for _ in range(k)] for _ in range(k)]
    all_needed_paulis: dict[str, float] = {}

    for i in range(k):
        ei = ops[i]
        for j in range(k):
            ej = ops[j]
            # Overlap S_{ij} = <E_i E_j> (Pauli operators are Hermitian)
            ph_s, prod_s = multiply_paulis(ei, ej)
            s_terms[i][j][prod_s] = s_terms[i][j].get(prod_s, 0.0j) + ph_s
            if not (set(prod_s) <= {"I"}):
                all_needed_paulis[prod_s] = max(all_needed_paulis.get(prod_s, 0.0), 1.0)
            # Hamiltonian H_{ij} = sum_P c_P <E_i P E_j>
            for p_str, c_val in coeffs.items():
                c = float(c_val)
                if abs(c) == 0.0:
                    continue
                p_clean = str(p_str).strip().upper()
                ph1, mid = multiply_paulis(ei, p_clean)
                ph2, full_p = multiply_paulis(mid, ej)
                w = c * ph1 * ph2
                h_terms[i][j][full_p] = h_terms[i][j].get(full_p, 0.0j) + w
                if not (set(full_p) <= {"I"}):
                    all_needed_paulis[full_p] = max(all_needed_paulis.get(full_p, 0.0), abs(c))

    grouped = group_pauli_terms(all_needed_paulis) if all_needed_paulis else {
        "n_qubits": n_qubits, "identity_offset": 0.0, "identity_terms": {}, "groups": []
    }
    return {
        "n_qubits": n_qubits,
        "operators": ops,
        "h_terms": h_terms,
        "s_terms": s_terms,
        "grouped": grouped,
    }


def solve_qse_from_expectations(
    plan: dict[str, Any],
    pauli_expectations: dict[str, float],
    *,
    pauli_std_errors: dict[str, float] | None = None,
    constant_energy: float = 0.0,
    s_min: float = 1e-2,
    n_bootstrap: int = 200,
    seed: int = 7,
) -> dict[str, Any]:
    """Assemble ``H`` & ``S`` from measured Pauli expectations and solve regularized ``H c = E S c``."""
    ops = plan["operators"]
    k = len(ops)
    h_terms = plan["h_terms"]
    s_terms = plan["s_terms"]

    def _assemble(exp_map: dict[str, float]) -> tuple[np.ndarray, np.ndarray]:
        h_mat = np.zeros((k, k), dtype=complex)
        s_mat = np.zeros((k, k), dtype=complex)
        for i in range(k):
            for j in range(k):
                hs = 0.0j
                for p, w in h_terms[i][j].items():
                    val = 1.0 if set(p) <= {"I"} else float(exp_map.get(p, 0.0))
                    hs += w * val
                ss = 0.0j
                for p, w in s_terms[i][j].items():
                    val = 1.0 if set(p) <= {"I"} else float(exp_map.get(p, 0.0))
                    ss += w * val
                h_mat[i, j] = hs
                s_mat[i, j] = ss
        # Enforce exact Hermiticity
        h_mat = 0.5 * (h_mat + h_mat.conj().T)
        s_mat = 0.5 * (s_mat + s_mat.conj().T)
        return h_mat, s_mat

    def _solve_gep(h_mat: np.ndarray, s_mat: np.ndarray) -> tuple[np.ndarray, int]:
        s_evals, s_evecs = np.linalg.eigh(s_mat)
        keep = s_evals >= float(s_min)
        if not np.any(keep):
            raise ValueError(f"QSE overlap matrix has no eigenvalues >= s_min={s_min}")
        u = s_evecs[:, keep]
        inv_sqrt = np.diag(1.0 / np.sqrt(s_evals[keep]))
        x = u @ inv_sqrt
        h_eff = x.conj().T @ h_mat @ x
        h_eff = 0.5 * (h_eff + h_eff.conj().T)
        evals = np.linalg.eigvalsh(h_eff)
        return np.real(evals), int(np.sum(keep))

    h_mat, s_mat = _assemble(pauli_expectations)
    try:
        evals, eff_rank = _solve_gep(h_mat, s_mat)
    except ValueError as exc:
        return {"ok": False, "error": str(exc)}

    ground_elec = float(evals[0])
    spectrum = [float(e + float(constant_energy)) for e in evals]

    # Parametric bootstrap over Pauli measurement SEs for honest QSE uncertainty
    se_ground = 0.0
    if pauli_std_errors and any(float(v) > 0 for v in pauli_std_errors.values()) and int(n_bootstrap) > 10:
        rng = np.random.default_rng(int(seed))
        samples: list[float] = []
        keys = list(pauli_expectations.keys())
        for _ in range(int(n_bootstrap)):
            pert = {
                p: float(pauli_expectations[p] + rng.normal(0.0, float(pauli_std_errors.get(p, 0.0))))
                for p in keys
            }
            hb, sb = _assemble(pert)
            try:
                eb, _ = _solve_gep(hb, sb)
                samples.append(float(eb[0]))
            except Exception:
                continue
        if len(samples) >= 10:
            se_ground = float(np.std(samples, ddof=1))

    return {
        "ok": True,
        "method": "QSE",
        "mitigated": ground_elec,
        "energy": float(ground_elec + float(constant_energy)),
        "uncertainty": float(se_ground),
        "ci95_mha": float(Z95 * se_ground * 1000.0),
        "spectrum": spectrum,
        "subspace_dim": int(k),
        "effective_rank": int(eff_rank),
        "s_min": float(s_min),
        "model_adequate": bool(eff_rank >= 1),
    }


__all__ = [
    "build_qse_plan",
    "default_qse_operators",
    "solve_qse_from_expectations",
]
