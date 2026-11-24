"""Bivariate Bicycle (BB) and CSS quantum LDPC code constructors.

Implements exact cyclic-shift matrix algebra over F2[x, y] / (x^l - 1, y^m - 1)
matching Bravyi et al. (Nature 2024) and qiskit-community/qcode-discovery
(Apache-2.0), plus direct-sum and qubit-relabeling helpers for red-team gates.
"""

from __future__ import annotations

from dataclasses import dataclass
from typing import Sequence, Tuple

import numpy as np

from kubona.qec.gf2 import css_validate


def _shift_matrix(size: int, power: int) -> np.ndarray:
    """Cyclic permutation matrix S^power of shape (size, size) over GF(2)."""
    p = power % size
    mat = np.zeros((size, size), dtype=np.uint8)
    for i in range(size):
        mat[i, (i + p) % size] = 1
    return mat


def _poly_matrix(ell: int, m: int, terms: Sequence[Tuple[int, int]]) -> np.ndarray:
    """Evaluate sum_{(a, b)} (S_ell^a kron S_m^b) mod 2."""
    n_half = ell * m
    acc = np.zeros((n_half, n_half), dtype=np.uint8)
    for a, b in terms:
        sa = _shift_matrix(ell, int(a))
        sb = _shift_matrix(m, int(b))
        acc ^= np.kron(sa, sb).astype(np.uint8)
    return acc


def build_bb_code(
    ell: int,
    m: int,
    a_terms: Sequence[Tuple[int, int]],
    b_terms: Sequence[Tuple[int, int]],
) -> Tuple[np.ndarray, np.ndarray]:
    """Construct CSS check matrices (Hx, Hz) for a Bivariate Bicycle code.

    Hx = [A | B], Hz = [B^T | A^T], with n = 2 * ell * m physical qubits.
    """
    if ell < 1 or m < 1:
        raise ValueError(f"ell and m must be positive integers, got ({ell}, {m})")
    if not a_terms or not b_terms:
        raise ValueError("a_terms and b_terms must be non-empty")
    A = _poly_matrix(ell, m, a_terms)
    B = _poly_matrix(ell, m, b_terms)
    Hx = np.hstack([A, B]).astype(np.uint8)
    Hz = np.hstack([B.T, A.T]).astype(np.uint8)
    return css_validate(Hx, Hz)


def direct_sum_css(
    hx1: np.ndarray,
    hz1: np.ndarray,
    hx2: np.ndarray,
    hz2: np.ndarray,
) -> Tuple[np.ndarray, np.ndarray]:
    """Construct the block-diagonal direct sum of two CSS codes."""
    Hx1, Hz1 = css_validate(hx1, hz1)
    Hx2, Hz2 = css_validate(hx2, hz2)
    rx1, n1 = Hx1.shape
    rx2, n2 = Hx2.shape
    rz1, _ = Hz1.shape
    rz2, _ = Hz2.shape
    Hx = np.zeros((rx1 + rx2, n1 + n2), dtype=np.uint8)
    Hx[:rx1, :n1] = Hx1
    Hx[rx1:, n1:] = Hx2
    Hz = np.zeros((rz1 + rz2, n1 + n2), dtype=np.uint8)
    Hz[:rz1, :n1] = Hz1
    Hz[rz1:, n1:] = Hz2
    return css_validate(Hx, Hz)


def relabel_qubits_css(
    hx: np.ndarray,
    hz: np.ndarray,
    perm: Sequence[int],
) -> Tuple[np.ndarray, np.ndarray]:
    """Return a qubit-relabelled copy of (Hx, Hz)."""
    Hx, Hz = css_validate(hx, hz)
    p = list(perm)
    if len(p) != Hx.shape[1] or sorted(p) != list(range(Hx.shape[1])):
        raise ValueError("perm must be a permutation of range(n)")
    return Hx[:, p].copy(), Hz[:, p].copy()


@dataclass(frozen=True)
class BBCodeSpec:
    name: str
    ell: int
    m: int
    a_terms: Tuple[Tuple[int, int], ...]
    b_terms: Tuple[Tuple[int, int], ...]
    n: int
    k: int
    d: int
    source: str


# Published Bravyi et al. (Nature 2024) Bivariate Bicycle table fixtures.
KNOWN_BB_SPECS: Tuple[BBCodeSpec, ...] = (
    BBCodeSpec(
        name="bravyi2024_bb_72_12_6",
        ell=6,
        m=6,
        a_terms=((3, 0), (0, 1), (0, 2)),
        b_terms=((0, 3), (1, 0), (2, 0)),
        n=72,
        k=12,
        d=6,
        source="Bravyi et al., Nature 627 (2024)",
    ),
    BBCodeSpec(
        name="bravyi2024_bb_90_8_10",
        ell=15,
        m=3,
        a_terms=((9, 0), (0, 1), (0, 2)),
        b_terms=((0, 0), (2, 0), (7, 0)),
        n=90,
        k=8,
        d=10,
        source="Bravyi et al., Nature 627 (2024)",
    ),
    BBCodeSpec(
        name="bravyi2024_bb_108_8_10",
        ell=9,
        m=6,
        a_terms=((3, 0), (0, 1), (0, 2)),
        b_terms=((0, 3), (1, 0), (2, 0)),
        n=108,
        k=8,
        d=10,
        source="Bravyi et al., Nature 627 (2024)",
    ),
    BBCodeSpec(
        name="bravyi2024_gross_144_12_12",
        ell=12,
        m=6,
        a_terms=((3, 0), (0, 1), (0, 2)),
        b_terms=((0, 3), (1, 0), (2, 0)),
        n=144,
        k=12,
        d=12,
        source="Bravyi et al., Nature 627 (2024)",
    ),
    BBCodeSpec(
        name="bravyi2024_bb_288_12_18",
        ell=12,
        m=12,
        a_terms=((3, 0), (0, 2), (0, 7)),
        b_terms=((0, 3), (1, 0), (2, 0)),
        n=288,
        k=12,
        d=18,
        source="Bravyi et al., Nature 627 (2024)",
    ),
)
