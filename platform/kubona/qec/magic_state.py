"""Phase 6 Magic-State Factory binary-matrix verifier (S5: arXiv:2610.06535).

Implements the exact syndrome criterion from Jain, Wills & Singh (2026):
  distance >= 3  <=>  all columns of the parity-check matrix H are nonzero
                      and pairwise distinct over GF(2),
together with the distillation yield exponent gamma = log(n / k) / log(d)
and published S5 reference targets (gamma = 1.589 / 1.057 / 0.998).
"""

from __future__ import annotations

import math
from typing import Any

import numpy as np


# Published S5 (arXiv:2610.06535) pure-T magic-state factory reference targets.
S5_MAGIC_STATE_TARGETS: dict[str, dict[str, Any]] = {
    "jain2026_63_11_3": {"n": 63, "k": 11, "d": 3, "gamma": 1.589},
    "jain2026_850_128_6": {"n": 850, "k": 128, "d": 6, "gamma": 1.057},
    "jain2026_1715_287_6": {"n": 1715, "k": 287, "d": 6, "gamma": 0.998},
}


def compute_gamma(n: int, k: int, d: int) -> float:
    """Distillation overhead exponent gamma = log(n / k) / log(d)."""
    if n <= 0 or k <= 0 or d <= 1:
        raise ValueError(f"require n > 0, k > 0, d > 1; got ({n}, {k}, {d})")
    return round(math.log(n / k) / math.log(d), 6)


def verify_magic_state_syndromes(h_mat: np.ndarray) -> dict[str, Any]:
    """Verify distance >= 3 via the exact S5 distinct-nonzero-syndrome criterion."""
    H = np.asarray(h_mat, dtype=np.uint8) & 1
    if H.ndim != 2 or H.shape[1] == 0:
        raise ValueError("H must be a non-empty 2D binary matrix")
    m, n = H.shape
    seen: dict[bytes, int] = {}
    zero_cols: list[int] = []
    duplicate_pairs: list[tuple[int, int]] = []
    for col in range(n):
        vec = H[:, col]
        if not np.any(vec):
            zero_cols.append(col)
            continue
        key = vec.tobytes()
        if key in seen:
            duplicate_pairs.append((seen[key], col))
        else:
            seen[key] = col
    d_ge_3 = (len(zero_cols) == 0) and (len(duplicate_pairs) == 0)
    return {
        "n": int(n),
        "r": int(m),
        "distance_ge_3": d_ge_3,
        "zero_syndrome_cols": zero_cols,
        "duplicate_syndrome_pairs": duplicate_pairs[:16],
        "method": "exact_distinct_nonzero_syndromes",
    }


def hamming_15_1_3_parity_matrix() -> np.ndarray:
    """Canonical 4x15 binary parity-check matrix for the 15-to-1 ([[15,1,3]]) factory."""
    cols = [[(i >> b) & 1 for b in range(4)] for i in range(1, 16)]
    return np.array(cols, dtype=np.uint8).T
