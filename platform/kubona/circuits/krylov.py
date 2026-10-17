"""Quantum Krylov Subspace Diagonalization (QKSD) & Early Fault-Tolerant Quantum Algorithms.

Implements Quantum Krylov Subspace Diagonalization (QKSD) and Orthogonal QKSD (OQKD)
as the primary discovery algorithm for molecular and many-body Hamiltonians.
Bypasses the barren plateau crisis and shot-noise catastrophe of heuristic NISQ VQE
by replacing non-linear parameter optimization with real-time evolution state generation
and classical generalized eigenvalue solution: H c = E S c.

PROVENANCE: every solver here runs classical emulation (numpy/scipy eigh on the
full Hamiltonian) — no quantum execution happens in this module. Results carry
``provenance="classical-emulation"`` so no downstream reader mistakes them for
QPU measurements.
"""

from __future__ import annotations

import math
from dataclasses import dataclass, field
from typing import Any, Dict, List, Optional, Tuple
import numpy as np


# ---------------------------------------------------------------------------
# Named v1 defaults (previously bare literals). Thresholds are v1 heuristics
# and labeled as such — they are starting points, not derived optima.
# ---------------------------------------------------------------------------
#: Default Krylov subspace dimension heuristic.
DEFAULT_SUBSPACE_DIM_V1_HEURISTIC = 5
#: Default real-time evolution step heuristic.
DEFAULT_TIME_STEP_V1_HEURISTIC = 0.2
#: Default overlap-matrix singular-value cutoff heuristic.
DEFAULT_REGULARIZATION_CUTOFF_V1_HEURISTIC = 1e-6
#: Lanczos breakdown tolerance heuristic (below this, the Krylov direction is exhausted).
LANCZOS_BREAKDOWN_TOL_V1_HEURISTIC = 1e-12
#: Ritz-residual convergence tolerance heuristic (Hartree): ||Hv - Ev|| below
#: this marks the ground-state solve converged.
CONVERGENCE_RESIDUAL_TOL_V1_HEURISTIC = 1e-6
#: Hermiticity acceptance tolerance heuristic.
HERMITICITY_ATOL_V1_HEURISTIC = 1e-9

#: Provenance label for every result from this module (classical eigh path).
PROVENANCE_CLASSICAL_EMULATION = "classical-emulation"


def _require_square_hermitian(hamiltonian: np.ndarray) -> np.ndarray:
    """Fail-closed input validation: square + Hermitian, else refuse.

    The old code fed any matrix into eigh (which silently discards the lower
    triangle of a non-Hermitian input), inventing eigenvalues for invalid input.
    """
    H = np.asarray(hamiltonian, dtype=complex)
    if H.ndim != 2 or H.shape[0] != H.shape[1]:
        raise ValueError(f"hamiltonian must be a square matrix, got shape {H.shape}")
    if H.shape[0] == 0:
        raise ValueError("hamiltonian must be non-empty")
    violation = float(np.max(np.abs(H - H.conj().T)))
    if not math.isfinite(violation) or violation > HERMITICITY_ATOL_V1_HEURISTIC:
        raise ValueError(
            f"hamiltonian is not Hermitian (max |H-H†| = {violation:.3e} > "
            f"{HERMITICITY_ATOL_V1_HEURISTIC:.0e}): refusing instead of "
            "diagonalizing a non-physical operator"
        )
    # Symmetrize fp noise so S/H_sub construction is exactly Hermitian.
    return 0.5 * (H + H.conj().T)


def _require_reference_state(initial_state: Optional[np.ndarray], dim: int,
                              solver: str) -> np.ndarray:
    """The reference state is caller-supplied, never silently |0...0>.

    The old code defaulted a missing reference to |0> without recording it,
    so results carried an undisclosed assumption about overlap with the true
    ground state.
    """
    if initial_state is None:
        raise ValueError(
            f"{solver}: initial_state is required — pass an explicit reference "
            "state vector (no silent |0...0> default; the reference conditions "
            "the whole Krylov subspace)"
        )
    psi = np.asarray(initial_state, dtype=complex).reshape(-1)
    if psi.shape[0] != dim:
        raise ValueError(
            f"initial_state dim {psi.shape[0]} != hamiltonian dim {dim}"
        )
    norm = float(np.linalg.norm(psi))
    if not math.isfinite(norm) or norm <= 0.0:
        raise ValueError("initial_state must have finite non-zero norm")
    return psi / norm


def _ritz_residual_norm(H: np.ndarray, basis: List[np.ndarray],
                        coeffs: np.ndarray, energy: float) -> float:
    """||H v - E v|| for the Ritz vector v = sum_k coeffs[k] * basis[k]."""
    v = np.zeros(H.shape[0], dtype=complex)
    for c, b in zip(coeffs, basis):
        v = v + complex(c) * b
    n = float(np.linalg.norm(v))
    if n == 0.0:
        return math.inf
    v = v / n
    return float(np.linalg.norm(H @ v - energy * v))


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
    # --- production-bar provenance (defaults keep old constructions valid) ---
    provenance: str = PROVENANCE_CLASSICAL_EMULATION
    discarded_modes: int = 0
    ritz_residual: float = math.inf
    reference_assumption: str = "explicit caller-supplied reference state"

    def to_dict(self) -> Dict[str, Any]:
        # Full precision: display rounding happens at presentation, never in
        # the record (the old code rounded eigenvalues to 6dp and cond to 4dp,
        # destroying the provenance trail for tight comparisons).
        return {
            "ground_state_energy": float(self.ground_state_energy),
            "eigenvalues": [float(ev) for ev in self.eigenvalues],
            "subspace_dimension": self.subspace_dimension,
            "condition_number": float(self.condition_number),
            "time_step": self.time_step,
            "is_converged": self.is_converged,
            "regularization_cutoff": self.regularization_cutoff,
            "provenance": self.provenance,
            "discarded_modes": self.discarded_modes,
            "ritz_residual": float(self.ritz_residual),
            "reference_assumption": self.reference_assumption,
        }


class QKSDSolver:
    """Quantum Krylov Subspace Diagonalization solver."""

    @classmethod
    def solve(
        cls,
        hamiltonian: np.ndarray,
        initial_state: Optional[np.ndarray] = None,
        subspace_dim: int = DEFAULT_SUBSPACE_DIM_V1_HEURISTIC,
        time_step: float = DEFAULT_TIME_STEP_V1_HEURISTIC,
        regularization_cutoff: float = DEFAULT_REGULARIZATION_CUTOFF_V1_HEURISTIC,
        reference_assumption: str = "explicit caller-supplied reference state",
    ) -> QKSDResult:
        """Solves for ground and low-lying states via real-time Quantum Krylov subspace.

        Args:
            hamiltonian: Hermitian matrix H (size 2^n x 2^n). Validated
                square + Hermitian fail-closed.
            initial_state: REQUIRED reference |psi_0>. None refuses (no
                silent |0...0>). The assumption is recorded on the result via
                ``reference_assumption``.
            subspace_dim: Dimension m of Krylov subspace.
            time_step: Real-time evolution parameter tau.
            regularization_cutoff: Threshold for discarding singular values of overlap matrix S.
            reference_assumption: free-text record of which reference was used
                (stored on the result for provenance).

        ``is_converged`` is COMPUTED from the ground-state Ritz residual, not
        asserted: all-singular overlap (no surviving modes) forces False.
        """
        H = _require_square_hermitian(hamiltonian)
        dim = H.shape[0]
        if not isinstance(subspace_dim, (int, np.integer)) or subspace_dim < 1:
            raise ValueError(f"subspace_dim must be a positive int, got {subspace_dim!r}")
        if not isinstance(time_step, (int, float, np.floating)) or not math.isfinite(time_step):
            raise ValueError(f"time_step must be finite, got {time_step!r}")
        if not isinstance(regularization_cutoff, (int, float, np.floating)) \
                or not math.isfinite(regularization_cutoff) or regularization_cutoff < 0:
            raise ValueError(f"regularization_cutoff must be finite >= 0, got {regularization_cutoff!r}")
        psi0 = _require_reference_state(initial_state, dim, "QKSDSolver.solve")

        # Generate Krylov vectors |psi_k> = exp(-i * k * H * tau) |psi_0>
        # Diagonalize H for exact real-time evolution simulation
        eigvals, eigvecs = np.linalg.eigh(H)

        krylov_states = []
        for k in range(int(subspace_dim)):
            phase = np.exp(-1j * k * time_step * eigvals)
            # |psi_k> = V * diag(phase) * V^dagger * |psi_0>
            psi_k = eigvecs @ (phase * (eigvecs.conj().T @ psi0))
            krylov_states.append(psi_k)

        # Build subspace Overlap matrix S and Hamiltonian matrix H_sub
        S = np.zeros((subspace_dim, subspace_dim), dtype=complex)
        H_sub = np.zeros((subspace_dim, subspace_dim), dtype=complex)

        for j in range(subspace_dim):
            for k in range(subspace_dim):
                S[j, k] = np.vdot(krylov_states[j], krylov_states[k])
                H_sub[j, k] = np.vdot(krylov_states[j], H @ krylov_states[k])

        # S is Hermitian positive semi-definite; SVD regularization for generalized eigenvalue problem:
        # H_sub c = E S c
        s_vals, s_vecs = np.linalg.eigh(S)
        s_max = float(np.max(s_vals))
        s_min = float(np.min(s_vals))
        # Honest conditioning: a non-positive smallest singular value means
        # the overlap is numerically rank-deficient — cond is +inf, not
        # 1e12-behind-a-floor (the old max(min,1e-12) hid the singularity).
        cond = float(s_max / s_min) if s_min > 0 else math.inf

        # Filter singular values >= cutoff to prevent ill-conditioned inversion
        valid_idx = s_vals >= regularization_cutoff
        discarded = int(subspace_dim - np.count_nonzero(valid_idx))
        all_singular = not bool(np.any(valid_idx))
        if all_singular:
            # Fallback to largest eigenvalue if all are below cutoff
            valid_idx = np.zeros_like(s_vals, dtype=bool)
            valid_idx[np.argmax(s_vals)] = True
            discarded = int(subspace_dim - 1)

        s_vals_filtered = s_vals[valid_idx]
        s_vecs_filtered = s_vecs[:, valid_idx]

        # Transformation matrix: X = s_vecs * diag(1 / sqrt(s_vals))
        X = s_vecs_filtered @ np.diag(1.0 / np.sqrt(s_vals_filtered))

        # Project Hamiltonian to regularized subspace: H_ortho = X^dagger * H_sub * X
        H_ortho = X.conj().T @ H_sub @ X

        # Diagonalize orthogonalized subspace Hamiltonian
        krylov_eigvals, krylov_eigvecs = np.linalg.eigh(H_ortho)
        order = np.argsort(np.real(krylov_eigvals))
        krylov_eigvals = np.real(krylov_eigvals[order])
        krylov_eigvecs = krylov_eigvecs[:, order]

        ground_energy = float(krylov_eigvals[0])

        # Computed convergence: Ritz residual of the ground-state Ritz vector.
        # All-singular overlap forces unconverged (the kept "mode" is a
        # numerical fallback, not a solved subspace).
        coeffs = X @ krylov_eigvecs[:, 0]
        residual = _ritz_residual_norm(H, krylov_states, np.asarray(coeffs).ravel(),
                                       ground_energy)
        is_converged = bool(
            (not all_singular)
            and math.isfinite(residual)
            and residual <= CONVERGENCE_RESIDUAL_TOL_V1_HEURISTIC
        )

        return QKSDResult(
            ground_state_energy=ground_energy,
            eigenvalues=krylov_eigvals.tolist(),
            subspace_dimension=subspace_dim,
            condition_number=cond,
            time_step=time_step,
            is_converged=is_converged,
            overlap_matrix=np.real(S).tolist(),
            hamiltonian_matrix=np.real(H_sub).tolist(),
            regularization_cutoff=regularization_cutoff,
            provenance=PROVENANCE_CLASSICAL_EMULATION,
            discarded_modes=discarded,
            ritz_residual=float(residual),
            reference_assumption=reference_assumption,
        )

    @classmethod
    def solve_orthogonal_qksd(
        cls,
        hamiltonian: np.ndarray,
        initial_state: Optional[np.ndarray] = None,
        subspace_dim: int = DEFAULT_SUBSPACE_DIM_V1_HEURISTIC,
        reference_assumption: str = "explicit caller-supplied reference state",
    ) -> QKSDResult:
        """Solves via Orthogonal QKSD (OQKD) using Lanczos state recurrence.

        Constructs an orthonormal Krylov subspace directly: S = I_m.
        Completely eliminates overlap matrix inversion and generalized eigenvalue instability.
        """
        return OrthogonalQKSDSolver.solve(
            hamiltonian=hamiltonian,
            initial_state=initial_state,
            subspace_dim=subspace_dim,
            reference_assumption=reference_assumption,
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
        subspace_dim: int = DEFAULT_SUBSPACE_DIM_V1_HEURISTIC,
        reference_assumption: str = "explicit caller-supplied reference state",
    ) -> QKSDResult:
        H = _require_square_hermitian(hamiltonian)
        dim = H.shape[0]
        if not isinstance(subspace_dim, (int, np.integer)) or subspace_dim < 1:
            raise ValueError(f"subspace_dim must be a positive int, got {subspace_dim!r}")
        v0 = _require_reference_state(initial_state, dim, "OrthogonalQKSDSolver.solve")

        m = min(int(subspace_dim), dim)
        v_states: List[np.ndarray] = []
        alphas: List[float] = []
        betas: List[float] = []

        # Step 0
        v_curr = v0.copy()
        v_states.append(v_curr)
        w = H @ v_curr
        alpha = float(np.real(np.vdot(v_curr, w)))
        alphas.append(alpha)
        f = w - alpha * v_curr
        beta = float(np.linalg.norm(f))

        # Steps 1 to m-1
        for k in range(1, m):
            if beta < LANCZOS_BREAKDOWN_TOL_V1_HEURISTIC:
                # Invariant Krylov subspace reached
                break
            betas.append(beta)
            v_next = f / beta

            # Gram-Schmidt re-orthogonalization for numerical invariance
            for prev_v in v_states:
                overlap = np.vdot(prev_v, v_next)
                v_next = v_next - overlap * prev_v
            norm_v = np.linalg.norm(v_next)
            if norm_v < LANCZOS_BREAKDOWN_TOL_V1_HEURISTIC:
                break
            v_next = v_next / norm_v
            v_states.append(v_next)

            w = H @ v_next
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

        eigvals, eigvecs = np.linalg.eigh(T)
        order = np.argsort(eigvals)
        eigvals = eigvals[order]
        eigvecs = eigvecs[:, order]

        # Computed convergence from the ground-state Ritz residual over the
        # Lanczos basis (early breakdown with a large residual = unconverged,
        # not a silent True).
        coeffs = np.asarray(eigvecs[:, 0]).ravel()
        residual = _ritz_residual_norm(H, v_states, coeffs, float(eigvals[0]))
        is_converged = bool(
            math.isfinite(residual)
            and residual <= CONVERGENCE_RESIDUAL_TOL_V1_HEURISTIC
        )

        # Overlap matrix is exact identity I_m
        S_ortho = np.eye(actual_m).tolist()
        H_sub = T.tolist()

        return QKSDResult(
            ground_state_energy=float(eigvals[0]),
            eigenvalues=eigvals.tolist(),
            subspace_dimension=actual_m,
            condition_number=1.0,  # Exact identity overlap
            time_step=0.0,
            is_converged=is_converged,
            overlap_matrix=S_ortho,
            hamiltonian_matrix=H_sub,
            regularization_cutoff=0.0,
            provenance=PROVENANCE_CLASSICAL_EMULATION,
            discarded_modes=int(subspace_dim) - actual_m,
            ritz_residual=float(residual),
            reference_assumption=reference_assumption,
        )
