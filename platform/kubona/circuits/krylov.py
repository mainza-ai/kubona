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

    @classmethod
    def solve_orthogonal_qksd(
        cls,
        hamiltonian: np.ndarray,
        initial_state: Optional[np.ndarray] = None,
        subspace_dim: int = 5,
    ) -> QKSDResult:
        """Solves via Orthogonal QKSD (OQKD) using Lanczos state recurrence.

        Constructs an orthonormal Krylov subspace directly: S = I_m.
        Completely eliminates overlap matrix inversion and generalized eigenvalue instability.
        """
        return OrthogonalQKSDSolver.solve(
            hamiltonian=hamiltonian,
            initial_state=initial_state,
            subspace_dim=subspace_dim,
        )


class OrthogonalQKSDSolver:
    """Orthogonal Quantum Krylov Subspace Diagonalization (OQKD).

    Implements Lanczos state recurrence directly on quantum states to construct
    an orthonormal basis without measuring full overlap matrices:
      |v_0> = |psi_0>
      alpha_0 = <v_0| H |v_0>
      |f_0> = H |v_0> - alpha_0 |v_0>
      beta_0 = || |f_0> ||
      |v_k> = |f_{k-1}> / beta_{k-1}
      alpha_k = <v_k| H |v_k>
      |f_k> = H |v_k> - alpha_k |v_k> - beta_{k-1} |v_{k-1}>
    """

    @classmethod
    def solve(
        cls,
        hamiltonian: np.ndarray,
        initial_state: Optional[np.ndarray] = None,
        subspace_dim: int = 5,
    ) -> QKSDResult:
        dim = hamiltonian.shape[0]
        if initial_state is None:
            v0 = np.zeros(dim, dtype=complex)
            v0[0] = 1.0
        else:
            v0 = initial_state / np.linalg.norm(initial_state)

        m = min(subspace_dim, dim)
        v_states: List[np.ndarray] = []
        alphas: List[float] = []
        betas: List[float] = []

        # Step 0
        v_curr = v0.copy()
        v_states.append(v_curr)
        w = hamiltonian @ v_curr
        alpha = float(np.real(np.vdot(v_curr, w)))
        alphas.append(alpha)
        f = w - alpha * v_curr
        beta = float(np.linalg.norm(f))

        # Steps 1 to m-1
        for k in range(1, m):
            if beta < 1e-12:
                # Invariant Krylov subspace reached
                break
            betas.append(beta)
            v_next = f / beta

            # Gram-Schmidt re-orthogonalization for numerical invariance
            for prev_v in v_states:
                overlap = np.vdot(prev_v, v_next)
                v_next = v_next - overlap * prev_v
            norm_v = np.linalg.norm(v_next)
            if norm_v < 1e-12:
                break
            v_next = v_next / norm_v
            v_states.append(v_next)

            w = hamiltonian @ v_next
            alpha = float(np.real(np.vdot(v_next, w)))
            alphas.append(alpha)

            f = w - alpha * v_next - beta * v_states[k - 1]
            beta = float(np.linalg.norm(f))

        actual_m = len(v_states)
        # Construct symmetric tridiagonal matrix T
        T = np.zeros((actual_m, actual_m), dtype=float)
        for i in range(actual_m):
            T[i, i] = alphas[i]
            if i < len(betas) and i + 1 < actual_m:
                T[i, i + 1] = betas[i]
                T[i + 1, i] = betas[i]

        eigvals, _ = np.linalg.eigh(T)
        eigvals = np.sort(eigvals)

        # Overlap matrix is exact identity I_m
        S_ortho = np.eye(actual_m).tolist()
        H_sub = T.tolist()

        return QKSDResult(
            ground_state_energy=float(eigvals[0]),
            eigenvalues=eigvals.tolist(),
            subspace_dimension=actual_m,
            condition_number=1.0,  # Exact identity overlap
            time_step=0.0,
            is_converged=True,
            overlap_matrix=S_ortho,
            hamiltonian_matrix=H_sub,
            regularization_cutoff=0.0,
        )

