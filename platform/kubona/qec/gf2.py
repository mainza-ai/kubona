"""Independent GF(2) linear algebra and exact CSS decomposability checks.

Zero-dependency (NumPy only) so it serves both as the primary exact algebraic
filter and as the independent verification path for Gate 1, Gate 2, and Gate 6.
"""

from __future__ import annotations

from typing import Tuple

import numpy as np


def _to_gf2(mat: np.ndarray) -> np.ndarray:
    arr = np.asarray(mat, dtype=np.uint8) & 1
    if arr.ndim != 2:
        raise ValueError(f"expected 2D binary matrix, got shape {arr.shape}")
    return arr.copy()


def gf2_rref(mat: np.ndarray) -> Tuple[np.ndarray, list[int]]:
    """Reduced row echelon form over GF(2). Returns (R, pivot_cols)."""
    R = _to_gf2(mat)
    m, n = R.shape
    pivots: list[int] = []
    row = 0
    for col in range(n):
        if row >= m:
            break
        # Find pivot at or below `row`
        rel = np.flatnonzero(R[row:, col])
        if rel.size == 0:
            continue
        piv = row + int(rel[0])
        if piv != row:
            R[[row, piv]] = R[[piv, row]]
        # Eliminate all other 1s in this column (both above and below)
        others = np.flatnonzero(R[:, col])
        others = others[others != row]
        if others.size > 0:
            R[others] ^= R[row]
        pivots.append(col)
        row += 1
    return R[:row], pivots


def gf2_rank(mat: np.ndarray) -> int:
    """Exact rank over GF(2)."""
    _, pivots = gf2_rref(mat)
    return len(pivots)


def gf2_nullspace(mat: np.ndarray) -> np.ndarray:
    """Exact basis for ker(mat) over GF(2), returned as rows of shape (nullity, n)."""
    R, pivots = gf2_rref(mat)
    n = mat.shape[1]
    piv_set = set(pivots)
    free_cols = [c for c in range(n) if c not in piv_set]
    if not free_cols:
        return np.zeros((0, n), dtype=np.uint8)
    basis = np.zeros((len(free_cols), n), dtype=np.uint8)
    for i, fc in enumerate(free_cols):
        basis[i, fc] = 1
        for r_idx, pc in enumerate(pivots):
            if R[r_idx, fc]:
                basis[i, pc] = 1
    return basis


def css_validate(hx: np.ndarray, hz: np.ndarray) -> Tuple[np.ndarray, np.ndarray]:
    """Validate CSS check matrices (shapes + exact commutativity Hx @ Hz.T == 0 mod 2)."""
    Hx = _to_gf2(hx)
    Hz = _to_gf2(hz)
    if Hx.shape[1] != Hz.shape[1]:
        raise ValueError(f"Hx ({Hx.shape}) and Hz ({Hz.shape}) must have equal qubit columns")
    if Hx.shape[1] == 0:
        raise ValueError("CSS code must have at least 1 physical qubit")
    comm = (Hx.astype(np.int64) @ Hz.T.astype(np.int64)) % 2
    if np.any(comm):
        raise ValueError("invalid CSS code: Hx @ Hz.T != 0 (mod 2)")
    return Hx, Hz


def css_parameters(hx: np.ndarray, hz: np.ndarray) -> dict[str, int]:
    """Compute exact (n, k, rank_x, rank_z) from GF(2) rank."""
    Hx, Hz = css_validate(hx, hz)
    n = int(Hx.shape[1])
    rx = gf2_rank(Hx)
    rz = gf2_rank(Hz)
    k = n - rx - rz
    if k < 0:
        raise ValueError(f"impossible CSS parameters: n={n}, rank_x={rx}, rank_z={rz}")
    return {"n": n, "k": k, "rank_x": rx, "rank_z": rz}


def _quotient_basis(subspace_rows: np.ndarray, ambient_rows: np.ndarray) -> np.ndarray:
    """Return independent rows from ambient_rows not in rowspace(subspace_rows)."""
    n = ambient_rows.shape[1]
    base_R, base_pivs = gf2_rref(subspace_rows) if subspace_rows.shape[0] else (np.zeros((0, n), dtype=np.uint8), [])
    cur_rows = list(base_R)
    cur_pivs = list(base_pivs)
    chosen = []
    for vec in ambient_rows:
        v = vec.copy()
        for r, p in zip(cur_rows, cur_pivs):
            if v[p]:
                v ^= r
        nz = np.flatnonzero(v)
        if nz.size > 0:
            chosen.append(vec.copy())
            cur_rows.append(v)
            cur_pivs.append(int(nz[0]))
    if not chosen:
        return np.zeros((0, n), dtype=np.uint8)
    return np.vstack(chosen).astype(np.uint8)


def css_logicals(hx: np.ndarray, hz: np.ndarray) -> Tuple[np.ndarray, np.ndarray]:
    """Return (Lx, Lz) logical operator bases over GF(2), each shape (k, n).

    Lx rows lie in ker(Hz) \\ rowspace(Hx); Lz rows lie in ker(Hx) \\ rowspace(Hz).
    """
    Hx, Hz = css_validate(hx, hz)
    ker_z = gf2_nullspace(Hz)
    ker_x = gf2_nullspace(Hx)
    lx = _quotient_basis(Hx, ker_z)
    lz = _quotient_basis(Hz, ker_x)
    return lx, lz


def is_decomposable_css(hx: np.ndarray, hz: np.ndarray) -> dict[str, object]:
    """Exact row-mixing-invariant CSS decomposability check (Gate 2 / G10).

    Theorem: A CSS code (Hx, Hz) splits as a direct sum on a qubit bipartition
    Q = Q1 U Q2 under *arbitrary* invertible row-scrambling matrices Mx, Mz
    if and only if the Tanner graph of (RREF(Hx), RREF(Hz)) has >1 connected
    component on its active qubits (or isolated zero-support qubits).
    """
    Hx, Hz = css_validate(hx, hz)
    n = int(Hx.shape[1])
    Rx, _ = gf2_rref(Hx)
    Rz, _ = gf2_rref(Hz)
    combined = np.vstack([Rx, Rz]) if (Rx.size or Rz.size) else np.zeros((0, n), dtype=np.uint8)

    parent = list(range(n))

    def find(a: int) -> int:
        while parent[a] != a:
            parent[a] = parent[parent[a]]
            a = parent[a]
        return a

    def union(a: int, b: int) -> None:
        ra, rb = find(a), find(b)
        if ra != rb:
            parent[rb] = ra

    for row in combined:
        supp = np.flatnonzero(row)
        if supp.size >= 2:
            first = int(supp[0])
            for q in supp[1:]:
                union(first, int(q))

    comp_sizes: dict[int, int] = {}
    for q in range(n):
        r = find(q)
        comp_sizes[r] = comp_sizes.get(r, 0) + 1

    sizes = sorted(comp_sizes.values(), reverse=True)
    return {
        "decomposable": len(sizes) > 1,
        "num_components": len(sizes),
        "component_qubit_sizes": sizes,
        "method": "rref_tanner_components",
    }
