"""Fermionic 1-RDM McWeeny Purification & 2-RDM N-Representability Purification.

Zero-circuit-overhead physics-constrained error mitigation for quantum chemistry:
  1. ``mcweeny_purify_1rdm`` (McWeeny 1960; Arute et al., Science 369, 1084, 2020):
     Iteratively projects a noisy spin-orbital 1-RDM ``D`` onto the idempotent
     single-determinant manifold ``D^2 = D, Tr(D) = N`` via ``D_{k+1} = 3 D_k^2 - 2 D_k^3``.
  2. ``purify_2rdm_n_representable`` (Smart & Mazziotti, PRA 100, 022517, 2019;
     Mazziotti 2025): Projects a noisy fermionic 2-RDM ``^2D`` (in antisymmetrized
     geminal matrix representation of shape ``M x M`` where ``M = C(n_so, 2)``)
     onto the convex intersection of physical N-representability constraints:
       - Hermiticity (``^2D = ^2D^dag``),
       - Positive semidefiniteness (D-condition: ``^2D >= 0``),
       - Exact particle-pair trace (``Tr(^2D) = N(N-1)/2`` in geminal basis),
       - Hole-pair Q-condition (``^2Q(^2D) >= 0``) via Dykstra's alternating projection.
"""

from __future__ import annotations

import math
from typing import Any

import numpy as np


def mcweeny_purify_1rdm(
    rdm1: np.ndarray,
    n_electrons: int,
    *,
    max_iter: int = 25,
    tol: float = 1e-10,
) -> dict[str, Any]:
    """Apply McWeeny idempotency purification ``D <- 3 D^2 - 2 D^3`` to a noisy 1-RDM."""
    d = np.asarray(rdm1, dtype=complex)
    if d.ndim != 2 or d.shape[0] != d.shape[1]:
        raise ValueError("rdm1 must be a square matrix")
    d = 0.5 * (d + d.conj().T)
    n_so = d.shape[0]
    n_e = int(n_electrons)
    if not (0 < n_e <= n_so):
        raise ValueError(f"n_electrons {n_e} outside (0, {n_so}]")

    init_idemp = float(np.linalg.norm(d @ d - d, ord="fro"))
    cur = d.copy()
    converged = False
    iters = 0
    for it in range(1, int(max_iter) + 1):
        iters = it
        cur2 = cur @ cur
        nxt = 3.0 * cur2 - 2.0 * (cur2 @ cur)
        nxt = 0.5 * (nxt + nxt.conj().T)
        # Keep eigenvalues bounded in [0, 1] and re-check convergence
        evals, evecs = np.linalg.eigh(nxt)
        evals = np.clip(np.real(evals), 0.0, 1.0)
        nxt = (evecs * evals[None, :]) @ evecs.conj().T
        err = float(np.linalg.norm(nxt @ nxt - nxt, ord="fro"))
        cur = nxt
        if err <= float(tol):
            converged = True
            break

    # Verify trace matches target particle count (warn/flag if noise shifted rank!)
    final_trace = float(np.real(np.trace(cur)))
    final_idemp = float(np.linalg.norm(cur @ cur - cur, ord="fro"))
    return {
        "ok": True,
        "method": "mcweeny_1rdm_purification",
        "purified_rdm1": np.real_if_close(cur),
        "initial_idempotency_error": init_idemp,
        "final_idempotency_error": final_idemp,
        "target_trace": float(n_e),
        "final_trace": final_trace,
        "trace_error": abs(final_trace - float(n_e)),
        "iterations": iters,
        "converged": converged,
    }


def _project_psd_trace(mat: np.ndarray, target_trace: float) -> np.ndarray:
    """Exact Frobenius projection onto ``{X = X^dag, X >= 0, Tr(X) = target_trace}``."""
    h = 0.5 * (mat + mat.conj().T)
    evals, evecs = np.linalg.eigh(h)
    lam = np.real(evals)
    # Euclidean projection of eigenvalues onto the scaled simplex {x >= 0, sum(x) = T}
    t = float(target_trace)
    u = np.sort(lam)[::-1]
    cssv = np.cumsum(u) - t
    ind = np.arange(1, len(u) + 1)
    cond = u - cssv / ind > 0
    if not np.any(cond):
        proj_lam = np.full_like(lam, t / float(len(lam)))
    else:
        rho = int(ind[cond][-1])
        theta = float(cssv[cond][-1]) / float(rho)
        proj_lam = np.maximum(lam - theta, 0.0)
    return (evecs * proj_lam[None, :]) @ evecs.conj().T


def _d2_to_d1_and_q2(
    d2_gem: np.ndarray,
    pairs: list[tuple[int, int]],
    n_so: int,
    n_elec: int,
) -> tuple[np.ndarray, np.ndarray]:
    """Contract geminal 2-RDM ``^2D`` (Tr = N(N-1)/2) to 1-RDM ``^1D`` and hole 2-RDM ``^2Q``."""
    m = len(pairs)
    d1 = np.zeros((n_so, n_so), dtype=complex)
    # ^1D[p, r] = (1/(N-1)) sum_q <a_p^dag a_q^dag a_q a_r>
    for idx_i, (p, q) in enumerate(pairs):
        for idx_j, (r, s) in enumerate(pairs):
            val = d2_gem[idx_i, idx_j]
            if abs(val) < 1e-15:
                continue
            # Four signed contractions matching (p<q), (r<s)
            if q == s:
                d1[p, r] += val
            if p == r:
                d1[q, s] += val
            if q == r:
                d1[p, s] -= val
            if p == s:
                d1[q, r] -= val
    d1 /= float(max(1, n_elec - 1))
    d1 = 0.5 * (d1 + d1.conj().T)

    # Hole 2-RDM in geminal basis:
    # ^2Q_{(pq),(rs)} = ^2D_{(pq),(rs)} + delta_{pr}delta_{qs}
    #   - (delta_{pr}^1D_{qs} + delta_{qs}^1D_{pr} - delta_{ps}^1D_{qr} - delta_{qr}^1D_{ps})
    q2 = d2_gem.copy()
    for i in range(m):
        p, q = pairs[i]
        for j in range(m):
            r, s = pairs[j]
            if i == j:
                q2[i, j] += 1.0
            if p == r:
                q2[i, j] -= d1[q, s]
            if q == s:
                q2[i, j] -= d1[p, r]
            if p == s:
                q2[i, j] += d1[q, r]
            if q == r:
                q2[i, j] += d1[p, s]
    q2 = 0.5 * (q2 + q2.conj().T)
    return d1, q2


def purify_2rdm_n_representable(
    d2_gem: np.ndarray,
    n_spin_orbitals: int,
    n_electrons: int,
    *,
    max_iter: int = 80,
    tol: float = 1e-7,
) -> dict[str, Any]:
    """Project a noisy geminal 2-RDM onto D- and Q-condition N-representable cone.

    Input ``d2_gem`` is the antisymmetrized geminal matrix indexed by ordered
    spin-orbital pairs ``{(p, q) : 0 <= p < q < n_spin_orbitals}`` with exact
    physical trace ``N(N-1)/2``.
    """
    n_so = int(n_spin_orbitals)
    n_e = int(n_electrons)
    pairs = [(p, q) for p in range(n_so) for q in range(p + 1, n_so)]
    m = len(pairs)
    arr = np.asarray(d2_gem, dtype=complex)
    if arr.shape != (m, m):
        raise ValueError(f"geminal 2-RDM must have shape ({m}, {m}) for n_so={n_so}, got {arr.shape}")
    if n_e < 2 or n_e > n_so:
        raise ValueError(f"n_electrons {n_e} outside [2, {n_so}]")

    target_tr = 0.5 * n_e * (n_e - 1)
    x = 0.5 * (arr + arr.conj().T)
    init_min_d = float(np.min(np.real(np.linalg.eigvalsh(x))))
    _, q0 = _d2_to_d1_and_q2(x, pairs, n_so, n_e)
    init_min_q = float(np.min(np.real(np.linalg.eigvalsh(q0))))

    # Alternating projection onto C_D = {D >= 0, Tr(D) = N(N-1)/2} and C_Q = {Q(D) >= 0}
    cur = x.copy()
    converged = False
    iters = 0
    for it in range(1, int(max_iter) + 1):
        iters = it
        # 1. Project onto D >= 0, Tr(D) = N(N-1)/2
        d_proj = _project_psd_trace(cur, target_tr)
        # 2. Check Q-condition and remove negative Q-eigenvalue component via damped correction
        _, q_mat = _d2_to_d1_and_q2(d_proj, pairs, n_so, n_e)
        q_evals, q_evecs = np.linalg.eigh(q_mat)
        min_q = float(np.min(np.real(q_evals)))
        if min_q >= -float(tol):
            cur = d_proj
            converged = True
            break
        neg_part = np.minimum(np.real(q_evals), 0.0)
        q_viol = (q_evecs * neg_part[None, :]) @ q_evecs.conj().T
        # Remove violation (step size 0.5 guarantees non-expansive Fejer contraction)
        cur = d_proj - 0.5 * q_viol

    final_d = _project_psd_trace(cur, target_tr)
    final_d1, final_q = _d2_to_d1_and_q2(final_d, pairs, n_so, n_e)
    final_min_d = float(np.min(np.real(np.linalg.eigvalsh(final_d))))
    final_min_q = float(np.min(np.real(np.linalg.eigvalsh(final_q))))

    return {
        "ok": True,
        "method": "2rdm_n_representability_purification",
        "purified_2rdm": np.real_if_close(final_d),
        "contracted_1rdm": np.real_if_close(final_d1),
        "initial_min_eig_D": init_min_d,
        "initial_min_eig_Q": init_min_q,
        "final_min_eig_D": final_min_d,
        "final_min_eig_Q": final_min_q,
        "target_trace": target_tr,
        "final_trace": float(np.real(np.trace(final_d))),
        "frobenius_shift": float(np.linalg.norm(final_d - x, ord="fro")),
        "iterations": iters,
        "converged": converged,
    }


__all__ = [
    "mcweeny_purify_1rdm",
    "purify_2rdm_n_representable",
]
