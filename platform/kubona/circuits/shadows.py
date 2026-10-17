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

# ---------------------------------------------------------------------------
# Named v1-heuristic defaults (previously bare literals scattered in code).
# These are pragmatic starting points, NOT derived optima — callers doing
# production estimation pass explicit values.
# ---------------------------------------------------------------------------
#: Default snapshot count heuristic (statistical, not a guarantee).
DEFAULT_NUM_SNAPSHOTS_V1_HEURISTIC = 500
#: Default median-of-means chunk count heuristic.
DEFAULT_NUM_CHUNKS_V1_HEURISTIC = 5
#: Default failure-probability heuristic for the sample-complexity bound.
DEFAULT_FAILURE_PROB_DELTA_V1_HEURISTIC = 0.01
#: Default max Pauli weight heuristic for the sample-complexity bound.
DEFAULT_MAX_PAULI_WEIGHT_V1_HEURISTIC = 2
#: Prefactor of the HKP sample-complexity bound (v1 heuristic constant).
SAMPLE_COMPLEXITY_PREFACTOR_V1_HEURISTIC = 2.0
#: Shadow-norm base: ||O||_shadow^2 = 3^k for k-local Pauli operators.
SHADOW_NORM_BASE_V1 = 3.0
#: Exact-density simulation cap: 2x2^n complex matrices beyond this refuse
#: instead of OOMing the host (n=20 -> 8 TB density matrix; check is cheap).
MAX_SHADOW_QUBITS = 20
#: Probability mass below which a rotated-basis distribution is degenerate
#: (refuse instead of dividing by ~0 and sampling garbage).
PROB_MASS_FLOOR_V1 = 1e-12


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
        num_snapshots: int = DEFAULT_NUM_SNAPSHOTS_V1_HEURISTIC,
        rng: Optional[np.random.Generator] = None,
    ) -> List[ShadowSnapshot]:
        """Simulates randomized Clifford/Pauli shadow acquisition from an exact state vector.

        ``rng`` is REQUIRED: the old code silently substituted
        ``default_rng(42)``, so two callers who forgot the seed shared one
        deterministic stream and believed they had independent statistics.
        Pass an explicit generator (e.g. ``np.random.default_rng(seed)``).
        """
        if rng is None:
            raise ValueError(
                "rng is required: pass an explicit np.random.Generator "
                "(no silent default_rng(42); independent runs need independent streams)"
            )
        if not isinstance(num_qubits, (int, np.integer)) or num_qubits < 1:
            raise ValueError(f"num_qubits must be a positive int, got {num_qubits!r}")
        if num_qubits > MAX_SHADOW_QUBITS:
            raise ValueError(
                f"shadow sampling refused for n={num_qubits} > {MAX_SHADOW_QUBITS}: "
                "exact 2^n density simulation is infeasible above the cap"
            )
        dim = 1 << num_qubits
        state_vector = np.asarray(state_vector, dtype=complex).reshape(-1)
        if state_vector.shape[0] != dim:
            raise ValueError(
                f"state_vector dim {state_vector.shape[0]} != 2^{num_qubits}={dim}"
            )
        norm = float(np.linalg.norm(state_vector))
        if not math.isfinite(norm) or norm == 0.0:
            raise ValueError("state_vector must have finite non-zero norm")
        state_vector = state_vector / norm
        if not isinstance(num_snapshots, (int, np.integer)) or num_snapshots < 1:
            raise ValueError(f"num_snapshots must be a positive int, got {num_snapshots!r}")

        density_matrix = np.outer(state_vector, state_vector.conj())
        snapshots: List[ShadowSnapshot] = []

        bases_choices = ["X", "Y", "Z"]

        for _ in range(int(num_snapshots)):
            # Choose random basis for each qubit (cast numpy.str_ -> str:
            # rng.choice returns numpy scalars that break downstream equality).
            chosen_bases = [str(rng.choice(bases_choices)) for _ in range(num_qubits)]

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
            total = float(np.sum(probabilities))
            if total < PROB_MASS_FLOOR_V1:
                raise ValueError(
                    "degenerate rotated-basis distribution (zero probability mass): "
                    "refusing instead of sampling from a zero-sum distribution"
                )
            probabilities /= total

            # Sample computational basis outcome
            outcome_idx = rng.choice(2**num_qubits, p=probabilities)
            # Binary string representation
            outcome_bits = [int(bit) for bit in format(int(outcome_idx), f"0{num_qubits}b")]

            snapshots.append(ShadowSnapshot(bases=chosen_bases, outcomes=outcome_bits))

        return snapshots

    @staticmethod
    def _median_of_means(estimates: List[float], num_chunks: int) -> float:
        """Balanced median-of-means via array_split (every sample counted once).

        The old code used ``len // num_chunks`` integer chunking, silently
        dropping up to ``num_chunks - 1`` trailing samples from the last chunk
        accounting. array_split distributes the remainder evenly.
        """
        if not estimates:
            raise ValueError(
                "no estimates to aggregate: snapshots list is empty "
                "(refusing instead of returning a fabricated 0.0)"
            )
        if not isinstance(num_chunks, (int, np.integer)) or num_chunks < 1:
            raise ValueError(f"num_chunks must be a positive int, got {num_chunks!r}")
        chunks = np.array_split(np.asarray(estimates, dtype=float), int(num_chunks))
        chunk_means = [float(np.mean(c)) for c in chunks if len(c)]
        if not chunk_means:
            raise ValueError("median-of-means found no non-empty chunk (unreachable)")
        return float(np.median(chunk_means))

    @staticmethod
    def estimate_observable_from_snapshots(
        snapshots: List[ShadowSnapshot],
        pauli_string: str,  # e.g. "ZZ", "XX", "ZIZ"
        num_chunks: int = DEFAULT_NUM_CHUNKS_V1_HEURISTIC,
    ) -> float:
        """Estimates expectation value <O> using median-of-means estimation.

        For single-qubit randomized Pauli shadows, the inverse quantum channel is:
        rho_hat_i = 3 * |b_i><b_i| - I
        Tr(P_i * rho_hat_i) = 3 * <b_i|P_i|b_i> if basis == P_i else 0 (if P_i != I).

        Raises on empty snapshots (never a fabricated 0.0) and on characters
        outside {I, X, Y, Z}.
        """
        if not snapshots:
            raise ValueError(
                "estimate refused: empty snapshot list "
                "(a 0.0 estimate from no data is fabrication)"
            )
        for ch in pauli_string:
            if ch not in PAULI_DICT:
                raise ValueError(
                    f"invalid Pauli character {ch!r} in {pauli_string!r}: want [IXYZ]+"
                )
        num_qubits = len(pauli_string)
        for s in snapshots:
            if len(s.bases) != num_qubits or len(s.outcomes) != num_qubits:
                raise ValueError(
                    f"snapshot width {len(s.bases)} != pauli_string width {num_qubits}"
                )
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
                    val *= SHADOW_NORM_BASE_V1 * eigenval
                else:
                    val = 0.0
                    break
            estimates.append(val)

        # Median of means across balanced chunks
        return ClassicalShadowEngine._median_of_means(estimates, num_chunks)

    @staticmethod
    def reconstruct_snapshot_matrix(snapshot: ShadowSnapshot) -> np.ndarray:
        """Inverted quantum channel snapshot: rho_hat = \\bigotimes_{i=1}^n (3 |b_i><b_i| - I).

        Unknown basis labels raise — the old code fell through to |0><0|,
        silently inventing a Z-0 projector for corrupt input.
        """
        rho_hat = np.array([[1.0]], dtype=complex)
        for b, bit in zip(snapshot.bases, snapshot.outcomes):
            if bit not in (0, 1):
                raise ValueError(f"snapshot outcome must be 0/1, got {bit!r}")
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
                raise ValueError(
                    f"unknown snapshot basis {b!r}: want one of 'X', 'Y', 'Z' "
                    "(refusing instead of substituting |0>)"
                )

            proj = np.outer(state, state.conj())
            single_qubit_rho_hat = SHADOW_NORM_BASE_V1 * proj - SIGMA_I
            rho_hat = np.kron(rho_hat, single_qubit_rho_hat)

        return rho_hat

    @classmethod
    def estimate_matrix_observable(
        cls,
        snapshots: List[ShadowSnapshot],
        observable: np.ndarray,
        num_chunks: int = DEFAULT_NUM_CHUNKS_V1_HEURISTIC,
    ) -> float:
        """Estimates Tr(O * rho) for an arbitrary Hermitian matrix observable via median-of-means."""
        if not snapshots:
            raise ValueError(
                "estimate refused: empty snapshot list "
                "(a 0.0 estimate from no data is fabrication)"
            )
        observable = np.asarray(observable, dtype=complex)
        if observable.ndim != 2 or observable.shape[0] != observable.shape[1]:
            raise ValueError("observable must be a square matrix")
        if not np.allclose(observable, observable.conj().T, atol=1e-9):
            raise ValueError("observable must be Hermitian")
        dim = observable.shape[0]
        n = int(round(math.log2(dim)))
        if 2**n != dim:
            raise ValueError(f"observable dim {dim} is not a power of 2")
        estimates = []
        for s in snapshots:
            if len(s.bases) != n:
                raise ValueError(
                    f"snapshot width {len(s.bases)} != observable qubits {n}"
                )
            rho_hat = cls.reconstruct_snapshot_matrix(s)
            val = float(np.real(np.trace(observable @ rho_hat)))
            estimates.append(val)

        return cls._median_of_means(estimates, num_chunks)

    @staticmethod
    def compute_shadow_norm(pauli_weight: int) -> float:
        """Computes the shadow norm bound: ||O||_shadow^2 = 3^k for k-local Pauli operator."""
        if isinstance(pauli_weight, bool) or not isinstance(pauli_weight, (int, np.integer)) \
                or int(pauli_weight) < 0:
            raise ValueError(f"pauli_weight must be a non-negative int, got {pauli_weight!r}")
        return float(SHADOW_NORM_BASE_V1 ** int(pauli_weight))

    @classmethod
    def sample_complexity_bound(
        cls,
        num_observables_m: int,
        epsilon: float,
        max_pauli_weight: int = DEFAULT_MAX_PAULI_WEIGHT_V1_HEURISTIC,
        failure_prob_delta: float = DEFAULT_FAILURE_PROB_DELTA_V1_HEURISTIC,
    ) -> int:
        """Computes Huang-Kueng-Preskill sample complexity guarantee:

        N ~ O( (log M / epsilon^2) * max_i ||O_i||_shadow^2 )

        Out-of-range inputs RAISE instead of clamping: the old code clamped
        epsilon to >= 1e-6, M to >= 1 and delta into [1e-6, 0.5], silently
        returning a bound for parameters the caller never asked about.
        """
        if isinstance(num_observables_m, bool) or not isinstance(num_observables_m, (int, np.integer)) \
                or int(num_observables_m) < 1:
            raise ValueError(
                f"num_observables_m must be a positive int, got {num_observables_m!r} "
                "(refusing instead of clamping to 1)"
            )
        if not isinstance(epsilon, (int, float, np.floating)) or not math.isfinite(epsilon) \
                or not 0 < epsilon <= 1:
            raise ValueError(
                f"epsilon must be in (0, 1], got {epsilon!r} "
                "(refusing instead of clamping)"
            )
        if isinstance(max_pauli_weight, bool) or not isinstance(max_pauli_weight, (int, np.integer)) \
                or int(max_pauli_weight) < 0:
            raise ValueError(f"max_pauli_weight must be >= 0, got {max_pauli_weight!r}")
        if not isinstance(failure_prob_delta, (int, float, np.floating)) \
                or not math.isfinite(failure_prob_delta) \
                or not 0 < failure_prob_delta < 1:
            raise ValueError(
                f"failure_prob_delta must be in (0, 1), got {failure_prob_delta!r} "
                "(refusing instead of clamping)"
            )

        shadow_norm_sq = cls.compute_shadow_norm(int(max_pauli_weight))
        m = int(num_observables_m)

        # Constant 2 * ln(2M / delta) / eps^2 * ||O||_shadow^2 (v1 heuristic prefactor)
        n_samples = math.ceil(
            (SAMPLE_COMPLEXITY_PREFACTOR_V1_HEURISTIC * math.log(2.0 * m / failure_prob_delta)
             / (epsilon ** 2)) * shadow_norm_sq
        )
        return n_samples
