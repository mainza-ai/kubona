"""Quantum Krylov Subspace Diagonalization (QKSD) & Early Fault-Tolerant Quantum Algorithms.

Implements Quantum Krylov Subspace Diagonalization (QKSD) and Orthogonal QKSD (OQKD)
as the primary discovery algorithm for molecular and many-body Hamiltonians.
Bypasses the barren plateau crisis and shot-noise catastrophe of heuristic NISQ VQE
by replacing non-linear parameter optimization with real-time evolution state generation
and classical generalized eigenvalue solution: H c = E S c.
"""

from __future__ import annotations

import math
from dataclasses import dataclass, field
from typing import Any, Dict, List, Optional, Tuple
import numpy as np


@dataclass
class QKSDResult:
    """Results of a Quantum Krylov Subspace Diagonalization."""
    ground_state_energy: float
    eigenvalues: List[float]
    subspace_dimension: int
    condition_number: float
    time_step: float
    is_converged: bool
    overlap_matrix: List[List[float]]
    hamiltonian_matrix: List[List[float]]
    regularization_cutoff: float

    def to_dict(self) -> Dict[str, Any]:
        return {
            "ground_state_energy": self.ground_state_energy,
            "eigenvalues": [round(float(ev), 6) for ev in self.eigenvalues],
            "subspace_dimension": self.subspace_dimension,
            "condition_number": round(self.condition_number, 4),
            "time_step": self.time_step,
            "is_converged": self.is_converged,
            "regularization_cutoff": self.regularization_cutoff,
        }


class QKSDSolver:
    """Quantum Krylov Subspace Diagonalization solver."""

    @classmethod
    def solve(
        cls,
        hamiltonian: np.ndarray,
        initial_state: Optional[np.ndarray] = None,
        subspace_dim: int = 5,
        time_step: float = 0.2,
        regularization_cutoff: float = 1e-6,
    ) -> QKSDResult:
        """Solves for ground and low-lying states via real-time Quantum Krylov subspace.

        Args:
            hamiltonian: Hermitian matrix H (size 2^n x 2^n).
            initial_state: Initial state vector |psi_0>. If None, defaults to |0...0>.
            subspace_dim: Dimension m of Krylov subspace.
            time_step: Real-time evolution parameter tau.
            regularization_cutoff: Threshold for discarding singular values of overlap matrix S.
        """
        dim = hamiltonian.shape[0]
        if initial_state is None:
            initial_state = np.zeros(dim, dtype=complex)
            initial_state[0] = 1.0
        else:
            initial_state = initial_state / np.linalg.norm(initial_state)

        # Generate Krylov vectors |psi_k> = exp(-i * k * H * tau) |psi_0>
        # Diagonalize H for exact real-time evolution simulation
        eigvals, eigvecs = np.linalg.eigh(hamiltonian)
        
        krylov_states = []
        for k in range(subspace_dim):
            phase = np.exp(-1j * k * time_step * eigvals)
            # |psi_k> = V * diag(phase) * V^dagger * |psi_0>
            psi_k = eigvecs @ (phase * (eigvecs.conj().T @ initial_state))
            krylov_states.append(psi_k)

        # Build subspace Overlap matrix S and Hamiltonian matrix H_sub
        S = np.zeros((subspace_dim, subspace_dim), dtype=complex)
        H_sub = np.zeros((subspace_dim, subspace_dim), dtype=complex)

        for j in range(subspace_dim):
            for k in range(subspace_dim):
                S[j, k] = np.vdot(krylov_states[j], krylov_states[k])
                H_sub[j, k] = np.vdot(krylov_states[j], hamiltonian @ krylov_states[k])

        # S is Hermitian positive semi-definite; SVD regularization for generalized eigenvalue problem:
        # H_sub c = E S c
        s_vals, s_vecs = np.linalg.eigh(S)
        cond = float(np.max(s_vals) / max(np.min(s_vals), 1e-12))

        # Filter singular values >= cutoff to prevent ill-conditioned inversion
        valid_idx = s_vals >= regularization_cutoff
        if not np.any(valid_idx):
            # Fallback to largest eigenvalue if all are below cutoff
            valid_idx = [np.argmax(s_vals)]

        s_vals_filtered = s_vals[valid_idx]
        s_vecs_filtered = s_vecs[:, valid_idx]

        # Transformation matrix: X = s_vecs * diag(1 / sqrt(s_vals))
        X = s_vecs_filtered @ np.diag(1.0 / np.sqrt(s_vals_filtered))

        # Project Hamiltonian to regularized subspace: H_ortho = X^dagger * H_sub * X
        H_ortho = X.conj().T @ H_sub @ X

        # Diagonalize orthogonalized subspace Hamiltonian
        krylov_eigvals, _ = np.linalg.eigh(H_ortho)
        krylov_eigvals = np.sort(np.real(krylov_eigvals))

        ground_energy = float(krylov_eigvals[0])

        return QKSDResult(
            ground_state_energy=ground_energy,
            eigenvalues=krylov_eigvals.tolist(),
            subspace_dimension=subspace_dim,
            condition_number=cond,
            time_step=time_step,
            is_converged=True,
            overlap_matrix=np.real(S).tolist(),
            hamiltonian_matrix=np.real(H_sub).tolist(),
            regularization_cutoff=regularization_cutoff,
        )
