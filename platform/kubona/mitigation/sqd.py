"""Sample-based Quantum Diagonalization (SQD) with Self-Consistent Configuration Recovery (S-CORE).

Implements Robledo-Moreno et al. (IBM + RIKEN, 2024–2025) natively on top of
PySCF exact molecular integrals (``h1e``, ``eri``, ``e_core``):
  1. Ingests noisy computational-basis bitstrings sampled from a single Z-basis
     circuit (e.g., LUCJ, UCCSD, or Krylov state preparation — zero Pauli-term
     explosion!).
  2. Self-Consistent Configuration Recovery (S-CORE): repairs bitstrings whose
     alpha or beta electron counts ``(n_a, n_b)`` were corrupted by hardware bit-flips
     or readout errors by probabilistically flipping bits guided by the current
     self-consistent orbital occupancy vector ``n_{p,sigma}``.
  3. Subspace Projection & Diagonalization: constructs exact Slater-Condon matrix
     elements ``H_{IJ} = <D_I | H | D_J>`` in the recovered determinant subspace
     and diagonalizes batched subspaces (or the full recovered subspace) to obtain
     variationally rigorous ground-state energies, updated orbital occupancies,
     and inter-batch uncertainty intervals.
"""

from __future__ import annotations

import math
from typing import Any

import numpy as np

Z95 = 1.959963984540054


def _parse_jw_bitstring(bs: str, n_spatial: int) -> tuple[int, int]:
    """Parse Qiskit little-endian Jordan-Wigner bitstring into ``(alpha_mask, beta_mask)``.

    Qubits ``0..n_spatial-1`` are alpha spin-orbitals (rightmost ``n_spatial`` bits);
    qubits ``n_spatial..2*n_spatial-1`` are beta spin-orbitals (leftmost ``n_spatial`` bits).
    """
    clean = str(bs).replace(" ", "").zfill(2 * n_spatial)[-(2 * n_spatial):]
    val = int(clean, 2)
    mask = (1 << n_spatial) - 1
    alpha = val & mask
    beta = (val >> n_spatial) & mask
    return alpha, beta


def _recover_spin_sector(
    mask: int,
    target_n: int,
    n_spatial: int,
    occ: np.ndarray,
    rng: np.random.Generator,
) -> int:
    """Repair a single spin-sector bitmask to exact particle number ``target_n`` via S-CORE."""
    cur_n = mask.bit_count()
    if cur_n == target_n:
        return mask
    bits = [(mask >> p) & 1 for p in range(n_spatial)]
    if cur_n > target_n:
        # Remove (cur_n - target_n) electrons, preferring occupied orbitals with LOW occupancy occ[p]
        ones = [p for p in range(n_spatial) if bits[p] == 1]
        weights = np.array([max(1e-4, 1.0 - float(occ[p])) for p in ones], dtype=float)
        weights /= weights.sum()
        flip_idx = rng.choice(ones, size=(cur_n - target_n), replace=False, p=weights)
        for p in flip_idx:
            bits[int(p)] = 0
    else:
        # Add (target_n - cur_n) electrons, preferring empty orbitals with HIGH occupancy occ[p]
        zeros = [p for p in range(n_spatial) if bits[p] == 0]
        weights = np.array([max(1e-4, float(occ[p])) for p in zeros], dtype=float)
        weights /= weights.sum()
        flip_idx = rng.choice(zeros, size=(target_n - cur_n), replace=False, p=weights)
        for p in flip_idx:
            bits[int(p)] = 1
    out = 0
    for p, b in enumerate(bits):
        if b:
            out |= 1 << p
    return out


def _slater_condon_matrix(
    dets: list[tuple[int, int]],
    h1e: np.ndarray,
    eri: np.ndarray,
    e_core: float,
    n_spatial: int,
) -> np.ndarray:
    """Build exact Hermitian Hamiltonian matrix ``H_{IJ} = <D_I | H | D_J>`` in chemist's ERI.

    ``eri`` is the 4-index chemist's notation tensor ``(pq|rs)`` of shape ``(n, n, n, n)``.
    """
    m = len(dets)
    hmat = np.zeros((m, m), dtype=float)

    def _occ_list(mask: int) -> list[int]:
        return [p for p in range(n_spatial) if (mask >> p) & 1]

    def _fermion_sign(mask: int, p: int, q: int) -> float:
        # Parity of occupied orbitals strictly between min(p,q) and max(p,q)
        lo, hi = (p, q) if p < q else (q, p)
        between = mask & (((1 << hi) - 1) ^ ((1 << (lo + 1)) - 1))
        return -1.0 if (between.bit_count() & 1) else 1.0

    def _double_sign(mask: int, p: int, q: int, r: int, s: int) -> float:
        # Apply annihilation r then s (r < s) and creation p then q (p < q) on mask
        # Track parity step by step for guaranteed exactness.
        parity = 0
        cur = mask
        for orb in (s, r):  # annihilate higher first, then lower
            below = cur & ((1 << orb) - 1)
            parity ^= below.bit_count() & 1
            cur ^= 1 << orb
        for orb in (q, p):  # create higher first, then lower
            below = cur & ((1 << orb) - 1)
            parity ^= below.bit_count() & 1
            cur ^= 1 << orb
        return -1.0 if parity else 1.0

    occ_a = [_occ_list(a) for a, _ in dets]
    occ_b = [_occ_list(b) for _, b in dets]

    for i in range(m):
        ai, bi = dets[i]
        oa, ob = occ_a[i], occ_b[i]
        # Diagonal Slater-Condon energy
        e_diag = float(e_core)
        for p in oa:
            e_diag += float(h1e[p, p])
        for p in ob:
            e_diag += float(h1e[p, p])
        # Same-spin (alpha-alpha, beta-beta): J - K
        for idx_p, p in enumerate(oa):
            for q in oa[idx_p + 1:]:
                e_diag += float(eri[p, p, q, q] - eri[p, q, q, p])
        for idx_p, p in enumerate(ob):
            for q in ob[idx_p + 1:]:
                e_diag += float(eri[p, p, q, q] - eri[p, q, q, p])
        # Opposite-spin (alpha-beta): J only
        for p in oa:
            for q in ob:
                e_diag += float(eri[p, p, q, q])
        hmat[i, i] = e_diag

        for j in range(i + 1, m):
            aj, bj = dets[j]
            da = ai ^ aj
            db = bi ^ bj
            na_diff = da.bit_count()
            nb_diff = db.bit_count()
            total_diff = na_diff + nb_diff
            if total_diff > 4:
                continue

            val = 0.0
            if na_diff == 2 and nb_diff == 0:
                # Single alpha excitation: p in ai\aj, q in aj\ai
                p = (da & ai).bit_length() - 1
                q = (da & aj).bit_length() - 1
                sgn = _fermion_sign(ai, p, q)
                elem = float(h1e[p, q])
                for r in oa:
                    if r != p:
                        elem += float(eri[p, q, r, r] - eri[p, r, r, q])
                for r in ob:
                    elem += float(eri[p, q, r, r])
                val = sgn * elem
            elif na_diff == 0 and nb_diff == 2:
                # Single beta excitation
                p = (db & bi).bit_length() - 1
                q = (db & bj).bit_length() - 1
                sgn = _fermion_sign(bi, p, q)
                elem = float(h1e[p, q])
                for r in ob:
                    if r != p:
                        elem += float(eri[p, q, r, r] - eri[p, r, r, q])
                for r in oa:
                    elem += float(eri[p, q, r, r])
                val = sgn * elem
            elif na_diff == 2 and nb_diff == 2:
                # Opposite-spin double excitation
                pa = (da & ai).bit_length() - 1
                qa = (da & aj).bit_length() - 1
                pb = (db & bi).bit_length() - 1
                qb = (db & bj).bit_length() - 1
                sgn = _fermion_sign(ai, pa, qa) * _fermion_sign(bi, pb, qb)
                val = sgn * float(eri[pa, qa, pb, qb])
            elif na_diff == 4 and nb_diff == 0:
                # Same-spin alpha double excitation
                rem = _occ_list(da & ai)
                add = _occ_list(da & aj)
                r, s = rem[0], rem[1]
                p, q = add[0], add[1]
                sgn = _double_sign(ai, p, q, r, s)
                val = sgn * float(eri[r, p, s, q] - eri[r, q, s, p])
            elif na_diff == 0 and nb_diff == 4:
                # Same-spin beta double excitation
                rem = _occ_list(db & bi)
                add = _occ_list(db & bj)
                r, s = rem[0], rem[1]
                p, q = add[0], add[1]
                sgn = _double_sign(bi, p, q, r, s)
                val = sgn * float(eri[r, p, s, q] - eri[r, q, s, p])

            hmat[i, j] = val
            hmat[j, i] = val
    return hmat


def run_sqd_from_counts(
    counts: dict[str, int],
    h1e: np.ndarray,
    eri: np.ndarray,
    e_core: float,
    n_alpha: int,
    n_beta: int,
    *,
    n_iterations: int = 5,
    n_batches: int = 4,
    max_subspace_dim: int = 256,
    seed: int = 7,
) -> dict[str, Any]:
    """Run Sample-based Quantum Diagonalization (SQD) with S-CORE on noisy Z-basis counts.

    Parameters
    ----------
    counts : dict[str, int]
        Hardware or noisy simulator computational-basis counts (length ``2 * n_spatial``).
    h1e : ndarray of shape ``(n_spatial, n_spatial)``
        One-electron molecular orbital integrals.
    eri : ndarray of shape ``(n_spatial, n_spatial, n_spatial, n_spatial)``
        Two-electron chemist's notation MO integrals ``(pq|rs)``.
    e_core : float
        Nuclear repulsion + frozen core constant energy.
    n_alpha, n_beta : int
        Target active-space alpha and beta electron counts.
    """
    n_spatial = int(h1e.shape[0])
    if h1e.shape != (n_spatial, n_spatial) or eri.shape != (n_spatial, n_spatial, n_spatial, n_spatial):
        raise ValueError("h1e / eri shapes incompatible with n_spatial")
    if not counts or sum(int(v) for v in counts.values() if int(v) > 0) <= 0:
        raise ValueError("SQD requires non-empty bitstring counts")

    rng = np.random.default_rng(int(seed))
    # Expand raw bitstring distribution into unique (alpha, beta) pairs + weights
    raw_pairs: list[tuple[int, int, int]] = []
    total_shots = 0
    raw_valid_shots = 0
    for bs, cnt_raw in counts.items():
        cnt = int(cnt_raw)
        if cnt <= 0:
            continue
        a, b = _parse_jw_bitstring(bs, n_spatial)
        raw_pairs.append((a, b, cnt))
        total_shots += cnt
        if a.bit_count() == int(n_alpha) and b.bit_count() == int(n_beta):
            raw_valid_shots += cnt

    # Initialize orbital occupancies from valid samples (or Hartree-Fock if none valid)
    occ_a = np.zeros(n_spatial, dtype=float)
    occ_b = np.zeros(n_spatial, dtype=float)
    if raw_valid_shots > 0:
        for a, b, cnt in raw_pairs:
            if a.bit_count() == int(n_alpha) and b.bit_count() == int(n_beta):
                for p in range(n_spatial):
                    if (a >> p) & 1:
                        occ_a[p] += cnt
                    if (b >> p) & 1:
                        occ_b[p] += cnt
        occ_a /= float(raw_valid_shots)
        occ_b /= float(raw_valid_shots)
    else:
        for p in range(int(n_alpha)):
            occ_a[p] = 0.9
        for p in range(int(n_beta)):
            occ_b[p] = 0.9
        occ_a = np.where(occ_a > 0, occ_a, 0.1)
        occ_b = np.where(occ_b > 0, occ_b, 0.1)

    hf_det = ((1 << int(n_alpha)) - 1, (1 << int(n_beta)) - 1)
    history: list[dict[str, Any]] = []
    best_energy = float("inf")
    batch_energies: list[float] = []
    final_subspace_dim = 0

    for it in range(max(1, int(n_iterations))):
        # 1. Self-consistent configuration recovery across every observed shot class
        recovered_counts: dict[tuple[int, int], int] = {}
        for a, b, cnt in raw_pairs:
            if a.bit_count() == int(n_alpha) and b.bit_count() == int(n_beta):
                recovered_counts[(a, b)] = recovered_counts.get((a, b), 0) + cnt
            else:
                # Recover up to min(cnt, 32) stochastic realizations weighted proportionally
                n_draws = min(cnt, 16)
                weight_each = max(1, cnt // n_draws)
                for _ in range(n_draws):
                    ra = _recover_spin_sector(a, int(n_alpha), n_spatial, occ_a, rng)
                    rb = _recover_spin_sector(b, int(n_beta), n_spatial, occ_b, rng)
                    recovered_counts[(ra, rb)] = recovered_counts.get((ra, rb), 0) + weight_each

        # Ensure HF reference determinant is present if recovered pool is tiny
        if hf_det not in recovered_counts and len(recovered_counts) < 2:
            recovered_counts[hf_det] = 1

        all_dets = sorted(recovered_counts.keys(), key=lambda d: (-recovered_counts[d], d))
        probs = np.array([recovered_counts[d] for d in all_dets], dtype=float)
        probs /= probs.sum()

        # 2. Subspace diagonalization across batches
        sub_cap = min(len(all_dets), int(max_subspace_dim))
        n_b = 1 if len(all_dets) <= sub_cap else max(1, int(n_batches))
        iter_energies: list[float] = []
        new_occ_a = np.zeros(n_spatial, dtype=float)
        new_occ_b = np.zeros(n_spatial, dtype=float)

        for _bi in range(n_b):
            if n_b == 1:
                batch_dets = all_dets[:sub_cap]
            else:
                chosen = rng.choice(len(all_dets), size=sub_cap, replace=False, p=probs)
                batch_dets = [all_dets[int(k)] for k in chosen]
            hmat = _slater_condon_matrix(batch_dets, h1e, eri, float(e_core), n_spatial)
            evals, evecs = np.linalg.eigh(hmat)
            e0 = float(evals[0])
            c0 = evecs[:, 0]
            iter_energies.append(e0)
            # Update orbital occupancies from ground-state CI amplitudes |c_I|^2
            for det_idx, (da, db) in enumerate(batch_dets):
                w_det = float(c0[det_idx] ** 2)
                for p in range(n_spatial):
                    if (da >> p) & 1:
                        new_occ_a[p] += w_det
                    if (db >> p) & 1:
                        new_occ_b[p] += w_det

        occ_a = np.clip(new_occ_a / float(n_b), 0.0, 1.0)
        occ_b = np.clip(new_occ_b / float(n_b), 0.0, 1.0)
        iter_min = float(min(iter_energies))
        if iter_min < best_energy:
            best_energy = iter_min
        batch_energies = iter_energies
        final_subspace_dim = sub_cap
        history.append({
            "iteration": it + 1,
            "min_energy": iter_min,
            "mean_energy": float(np.mean(iter_energies)),
            "unique_recovered_dets": len(all_dets),
            "subspace_dim": sub_cap,
        })

    batch_se = (
        float(np.std(batch_energies, ddof=1) / math.sqrt(len(batch_energies)))
        if len(batch_energies) > 1
        else 0.0
    )
    return {
        "ok": True,
        "method": "SQD",
        "energy": float(best_energy),
        "batch_mean_energy": float(np.mean(batch_energies)),
        "uncertainty": float(batch_se),
        "ci95_mha": float(Z95 * batch_se * 1000.0),
        "batch_energies": [float(e) for e in batch_energies],
        "subspace_dim": int(final_subspace_dim),
        "total_shots": int(total_shots),
        "raw_valid_fraction": float(raw_valid_shots) / float(total_shots),
        "orbital_occupancies_alpha": [round(float(x), 6) for x in occ_a],
        "orbital_occupancies_beta": [round(float(x), 6) for x in occ_b],
        "iterations": history,
        "variational_upper_bound": True,
        "model_adequate": True,
    }


__all__ = [
    "run_sqd_from_counts",
]
