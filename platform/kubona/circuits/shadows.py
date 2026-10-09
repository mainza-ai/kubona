"""Classical Shadow Tomography Engine (Huang, Kueng, Preskill 2020).

Predicts many observable expectation values simultaneously with logarithmic sample
complexity N ~ O(log M / epsilon^2), completely bypassing the Pauli grouping and
shot-noise bottleneck of naive VQE.
"""

from __future__ import annotations

import math
from dataclasses import dataclass
from typing import Any, Dict, List, Optional, Tuple
import numpy as np


# Pauli matrices
SIGMA_I = np.array([[1, 0], [0, 1]], dtype=complex)
SIGMA_X = np.array([[0, 1], [1, 0]], dtype=complex)
SIGMA_Y = np.array([[0, -1j], [1j, 0]], dtype=complex)
SIGMA_Z = np.array([[1, 0], [0, -1]], dtype=complex)

PAULI_DICT = {
    "I": SIGMA_I,
    "X": SIGMA_X,
    "Y": SIGMA_Y,
    "Z": SIGMA_Z,
}


@dataclass
class ShadowSnapshot:
    """A single randomized Pauli basis measurement snapshot."""
    bases: List[str]      # Selected measurement basis per qubit ("X", "Y", "Z")
    outcomes: List[int]   # Measured bit per qubit (0 or 1)


class ClassicalShadowEngine:
    """Generates and evaluates classical shadow representations of quantum states."""

    @staticmethod
    def sample_shadows_from_state(
        state_vector: np.ndarray,
        num_qubits: int,
        num_snapshots: int = 500,
        rng: Optional[np.random.Generator] = None,
    ) -> List[ShadowSnapshot]:
        """Simulates randomized Clifford/Pauli shadow acquisition from an exact state vector."""
        if rng is None:
            rng = np.random.default_rng(42)

        density_matrix = np.outer(state_vector, state_vector.conj())
        snapshots: List[ShadowSnapshot] = []

        bases_choices = ["X", "Y", "Z"]

        for _ in range(num_snapshots):
            # Choose random basis for each qubit
            chosen_bases = [rng.choice(bases_choices) for _ in range(num_qubits)]

            # Build rotation matrix U = \bigotimes U_i to rotate computational basis to chosen basis
            # If basis is X: H; if Y: S^dagger H; if Z: I
            U_total = np.array([[1.0]], dtype=complex)
            for b in chosen_bases:
                if b == "X":
                    # Hadamard: rotates X to Z
                    u_i = (1.0 / np.sqrt(2)) * np.array([[1, 1], [1, -1]], dtype=complex)
                elif b == "Y":
                    # S^dagger H: rotates Y to Z
                    u_i = (1.0 / np.sqrt(2)) * np.array([[1, -1j], [1, 1j]], dtype=complex)
                else:
                    u_i = SIGMA_I
                U_total = np.kron(U_total, u_i)

            # State in rotated basis: rho_rot = U * rho * U^dagger
            rho_rot = U_total @ density_matrix @ U_total.conj().T
            probabilities = np.real(np.diag(rho_rot))
            probabilities = np.maximum(probabilities, 0.0)
            probabilities /= np.sum(probabilities)

            # Sample computational basis outcome
            outcome_idx = rng.choice(2**num_qubits, p=probabilities)
            # Binary string representation
            outcome_bits = [int(bit) for bit in format(outcome_idx, f"0{num_qubits}b")]

            snapshots.append(ShadowSnapshot(bases=chosen_bases, outcomes=outcome_bits))

        return snapshots

    @staticmethod
    def estimate_observable_from_snapshots(
        snapshots: List[ShadowSnapshot],
        pauli_string: str,  # e.g. "ZZ", "XX", "ZIZ"
        num_chunks: int = 5,
    ) -> float:
        """Estimates expectation value <O> using median-of-means estimation.

        For single-qubit randomized Pauli shadows, the inverse quantum channel is:
        rho_hat_i = 3 * |b_i><b_i| - I
        Tr(P_i * rho_hat_i) = 3 * <b_i|P_i|b_i> if basis == P_i else 0 (if P_i != I).
        """
        num_qubits = len(pauli_string)
        estimates: List[float] = []

        for s in snapshots:
            val = 1.0
            for i, p in enumerate(pauli_string):
                if p == "I":
                    continue
                # If measurement basis matches Pauli operator
                if s.bases[i] == p:
                    # bit 0 -> +1, bit 1 -> -1
                    eigenval = 1.0 if s.outcomes[i] == 0 else -1.0
                    val *= 3.0 * eigenval
                else:
                    val = 0.0
                    break
            estimates.append(val)

        if not estimates:
            return 0.0

        # Median of means across chunks
        chunk_size = max(1, len(estimates) // num_chunks)
        chunk_means = []
        for c in range(num_chunks):
            start = c * chunk_size
            end = start + chunk_size if c < num_chunks - 1 else len(estimates)
            chunk = estimates[start:end]
            if chunk:
                chunk_means.append(float(np.mean(chunk)))

        return float(np.median(chunk_means)) if chunk_means else 0.0

    @staticmethod
    def reconstruct_snapshot_matrix(snapshot: ShadowSnapshot) -> np.ndarray:
        """Inverted quantum channel snapshot: rho_hat = \\bigotimes_{i=1}^n (3 |b_i><b_i| - I)."""
        rho_hat = np.array([[1.0]], dtype=complex)
        for b, bit in zip(snapshot.bases, snapshot.outcomes):
            # Compute state vector |b_i> for basis and outcome
            if b == "Z":
                state = np.array([1, 0], dtype=complex) if bit == 0 else np.array([0, 1], dtype=complex)
            elif b == "X":
                # |+> or |->
                state = (1.0 / np.sqrt(2)) * (np.array([1, 1], dtype=complex) if bit == 0 else np.array([1, -1], dtype=complex))
            elif b == "Y":
                # |+i> or |-i>
                state = (1.0 / np.sqrt(2)) * (np.array([1, 1j], dtype=complex) if bit == 0 else np.array([1, -1j], dtype=complex))
            else:
                state = np.array([1, 0], dtype=complex)

            proj = np.outer(state, state.conj())
            single_qubit_rho_hat = 3.0 * proj - SIGMA_I
            rho_hat = np.kron(rho_hat, single_qubit_rho_hat)

        return rho_hat

    @classmethod
    def estimate_matrix_observable(
        cls,
        snapshots: List[ShadowSnapshot],
        observable: np.ndarray,
        num_chunks: int = 5,
    ) -> float:
        """Estimates Tr(O * rho) for an arbitrary Hermitian matrix observable via median-of-means."""
        estimates = []
        for s in snapshots:
            rho_hat = cls.reconstruct_snapshot_matrix(s)
            val = float(np.real(np.trace(observable @ rho_hat)))
            estimates.append(val)

        if not estimates:
            return 0.0

        chunk_size = max(1, len(estimates) // num_chunks)
        chunk_means = []
        for c in range(num_chunks):
            start = c * chunk_size
            end = start + chunk_size if c < num_chunks - 1 else len(estimates)
            chunk = estimates[start:end]
            if chunk:
                chunk_means.append(float(np.mean(chunk)))

        return float(np.median(chunk_means)) if chunk_means else 0.0

    @staticmethod
    def compute_shadow_norm(pauli_weight: int) -> float:
        """Computes the shadow norm bound: ||O||_shadow^2 = 3^k for k-local Pauli operator."""
        return float(3.0 ** pauli_weight)

    @classmethod
    def sample_complexity_bound(
        cls,
        num_observables_m: int,
        epsilon: float,
        max_pauli_weight: int = 2,
        failure_prob_delta: float = 0.01,
    ) -> int:
        """Computes Huang-Kueng-Preskill sample complexity guarantee:

        N ~ O( (log M / epsilon^2) * max_i ||O_i||_shadow^2 )
        """
        shadow_norm_sq = cls.compute_shadow_norm(max_pauli_weight)
        eps = max(1e-6, epsilon)
        m = max(1, num_observables_m)
        delta = max(1e-6, min(0.5, failure_prob_delta))

        # Constant 2 * ln(2M / delta) / eps^2 * ||O||_shadow^2
        n_samples = math.ceil((2.0 * math.log(2.0 * m / delta) / (eps ** 2)) * shadow_norm_sq)
        return n_samples

