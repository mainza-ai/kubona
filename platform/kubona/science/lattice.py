"""Exactly solvable lattice models and a quantum spin-chain experiment core.

The module is deliberately split into two evidence classes:

- ``SpinChainModel`` builds a qubit Hamiltonian (TFIM / Heisenberg / XXZ),
  provides an exact sparse-diagonalization anchor, and exports Pauli terms for
  shot measurement by the bridge.
- ``HubbardModel`` is a classical Fermi-Hubbard solver in the fermionic
  occupation basis. It is a domain-pack anchor, not a qubit result.

No function in this module labels a finite-size numerical result as a proof.
"""

from __future__ import annotations

import math
from dataclasses import dataclass
from typing import Any

import numpy as np
from scipy import sparse
from scipy.sparse import linalg as sparse_linalg


SUPPORTED_SPIN_MODELS = frozenset({"tfim", "heisenberg", "xxz"})


def _validate_sites(n_sites: int) -> int:
    n = int(n_sites)
    if n < 2 or n > 14:
        raise ValueError(f"n_sites must be in [2,14], got {n_sites!r}")
    return n


def _validate_finite(name: str, value: float) -> float:
    v = float(value)
    if not math.isfinite(v):
        raise ValueError(f"{name} must be finite, got {value!r}")
    return v


def _bonds(n_sites: int, periodic: bool) -> list[tuple[int, int]]:
    bonds = [(i, i + 1) for i in range(n_sites - 1)]
    if periodic and n_sites > 2:
        bonds.append((n_sites - 1, 0))
    return bonds


def _z_value(state: int, qubit: int) -> int:
    return 1 - 2 * ((state >> qubit) & 1)


@dataclass(frozen=True)
class LatticeObservables:
    magnetization_z: list[float]
    correlation_zz: list[float]
    entanglement_entropy_half: float
    gap: float
    eigenvalues: list[float]

    def to_dict(self) -> dict[str, Any]:
        return {
            "magnetization_z": self.magnetization_z,
            "correlation_zz": self.correlation_zz,
            "entanglement_entropy_half": self.entanglement_entropy_half,
            "gap": self.gap,
            "eigenvalues": self.eigenvalues,
        }


class SpinChainModel:
    """A 1D spin chain with an exact finite-size anchor and Pauli export."""

    def __init__(
        self,
        model: str = "tfim",
        n_sites: int = 4,
        coupling: float = 1.0,
        field: float = 1.0,
        anisotropy: float = 1.0,
        periodic: bool = True,
    ) -> None:
        key = str(model or "").strip().lower()
        if key not in SUPPORTED_SPIN_MODELS:
            raise ValueError(f"model must be one of {sorted(SUPPORTED_SPIN_MODELS)}, got {model!r}")
        self.model = key
        self.n_sites = _validate_sites(n_sites)
        self.coupling = _validate_finite("coupling", coupling)
        self.field = _validate_finite("field", field)
        self.anisotropy = _validate_finite("anisotropy", anisotropy)
        self.periodic = bool(periodic)
        if self.model in {"heisenberg", "xxz"} and self.coupling == 0:
            raise ValueError("heisenberg/xxz requires a non-zero coupling")
        if self.model == "tfim" and self.coupling == 0 and self.field == 0:
            raise ValueError("tfim requires coupling or field to be non-zero")

    @property
    def dimension(self) -> int:
        return 1 << self.n_sites

    def pauli_terms(self) -> list[tuple[str, float]]:
        """Return (Pauli label, coefficient) with qubit 0 at label position 0."""
        terms: dict[str, float] = {}
        n = self.n_sites

        def add(label: str, coeff: float) -> None:
            if abs(coeff) > 1e-15:
                terms[label] = terms.get(label, 0.0) + float(coeff)

        for i, j in _bonds(n, self.periodic):
            zz = ["I"] * n
            zz[i] = "Z"
            zz[j] = "Z"
            if self.model == "tfim":
                add("".join(zz), -self.coupling)
            else:
                add("".join(zz), self.coupling * self.anisotropy)
                xx = ["I"] * n
                yy = ["I"] * n
                xx[i] = xx[j] = "X"
                yy[i] = yy[j] = "Y"
                add("".join(xx), self.coupling)
                add("".join(yy), self.coupling)
        if self.model == "tfim":
            for i in range(n):
                x = ["I"] * n
                x[i] = "X"
                add("".join(x), -self.field)
        return sorted(terms.items())

    def hamiltonian(self) -> sparse.csr_matrix:
        n = self.n_sites
        dim = self.dimension
        rows: list[int] = []
        cols: list[int] = []
        data: list[complex] = []

        def put(row: int, col: int, value: complex) -> None:
            if abs(value) > 1e-15:
                rows.append(row)
                cols.append(col)
                data.append(complex(value))

        for state in range(dim):
            for i, j in _bonds(n, self.periodic):
                if self.model == "tfim":
                    put(state, state, -self.coupling * _z_value(state, i) * _z_value(state, j))
                else:
                    put(state, state, self.coupling * self.anisotropy
                        * _z_value(state, i) * _z_value(state, j))
                    flipped = state ^ (1 << i) ^ (1 << j)
                    put(flipped, state, self.coupling)
                    phase = -((-1) ** (((state >> i) & 1) + ((state >> j) & 1)))
                    put(flipped, state, self.coupling * phase)
            if self.model == "tfim":
                for i in range(n):
                    put(state ^ (1 << i), state, -self.field)

        h = sparse.coo_matrix((data, (rows, cols)), shape=(dim, dim), dtype=complex).tocsr()
        h.sum_duplicates()
        if (h - h.getH()).nnz and np.max(np.abs((h - h.getH()).data)) > 1e-12:
            raise RuntimeError("constructed spin-chain Hamiltonian is not Hermitian")
        return h

    def ground_state(self, k: int = 4) -> tuple[float, np.ndarray, list[float]]:
        h = self.hamiltonian()
        dim = h.shape[0]
        if dim <= 1024:
            evals, evecs = np.linalg.eigh(h.toarray())
            order = np.argsort(evals)
            evals = evals[order]
            vec = np.asarray(evecs[:, order[0]], dtype=complex)
            return float(np.real(evals[0])), vec, [float(np.real(x)) for x in evals[:k]]
        kk = min(max(2, int(k)), dim - 1)
        evals, evecs = sparse_linalg.eigsh(h, k=kk, which="SA")
        order = np.argsort(evals)
        evals = evals[order]
        vec = np.asarray(evecs[:, order[0]], dtype=complex)
        return float(np.real(evals[0])), vec, [float(np.real(x)) for x in evals]

    def observables(self, vector: np.ndarray | None = None, k: int = 4) -> LatticeObservables:
        """Return observables for ``vector``, or the exact ground state.

        The gap/eigenvalues are always the exact finite-size spectrum of this
        model. Magnetization, correlations, and half-chain entropy are computed
        from the supplied state; passing an optimized VQE vector must not be
        silently replaced by the ground-state anchor.
        """
        _energy, ground_vec, evals = self.ground_state(k=k)
        n = self.n_sites
        if vector is None:
            vec = ground_vec
        else:
            vec = np.asarray(vector, dtype=complex).reshape(-1)
            if vec.size != self.dimension:
                raise ValueError(
                    f"state vector has {vec.size} amplitudes; expected {self.dimension}"
                )
            norm = float(np.linalg.norm(vec))
            if not math.isfinite(norm) or norm <= 0:
                raise ValueError("state vector must be finite and non-zero")
            vec = vec / norm
        probs = np.abs(vec) ** 2
        probs = probs / max(float(probs.sum()), 1e-15)
        mag = []
        for i in range(n):
            mag.append(float(sum(p * _z_value(state, i) for state, p in enumerate(probs))))
        corr = []
        for i, j in _bonds(n, self.periodic):
            corr.append(float(sum(p * _z_value(state, i) * _z_value(state, j)
                                 for state, p in enumerate(probs))))
        entropy = _half_chain_entropy(vec, n)
        gap = float("nan")
        for value in evals[1:]:
            if value > evals[0] + 1e-9:
                gap = value - evals[0]
                break
        return LatticeObservables(
            magnetization_z=mag,
            correlation_zz=corr,
            entanglement_entropy_half=entropy,
            gap=gap,
            eigenvalues=evals,
        )

    def describe(self) -> dict[str, Any]:
        return {
            "model": self.model,
            "n_sites": self.n_sites,
            "coupling": self.coupling,
            "field": self.field,
            "anisotropy": self.anisotropy,
            "periodic": self.periodic,
            "dimension": self.dimension,
            "qubits": self.n_sites,
        }


def _half_chain_entropy(vector: np.ndarray, n_sites: int) -> float:
    if n_sites < 2:
        return 0.0
    left = n_sites // 2
    right = n_sites - left
    matrix = np.asarray(vector, dtype=complex).reshape(1 << left, 1 << right)
    singular = np.linalg.svd(matrix, compute_uv=False)
    probs = np.real(singular) ** 2
    probs = probs[probs > 1e-15]
    if probs.size == 0:
        return 0.0
    probs = probs / probs.sum()
    return float(-np.sum(probs * np.log(probs)))


class HubbardModel:
    """Exact 1D Fermi-Hubbard anchor in the fermionic occupation basis.

    This is a classical solver used by the materials domain pack. It is not a
    qubit experiment and does not emit Pauli terms. N is capped at 6 sites
    (4^6 states) so an accidental call cannot allocate an unbounded matrix.
    """

    def __init__(self, n_sites: int = 4, hopping: float = 1.0,
                 interaction: float = 4.0, chemical_potential: float = 0.0,
                 periodic: bool = True) -> None:
        n = int(n_sites)
        if n < 2 or n > 6:
            raise ValueError(f"n_sites must be in [2,6], got {n_sites!r}")
        self.n_sites = n
        self.hopping = _validate_finite("hopping", hopping)
        self.interaction = _validate_finite("interaction", interaction)
        self.chemical_potential = _validate_finite("chemical_potential", chemical_potential)
        self.periodic = bool(periodic)
        self.orbitals = 2 * n

    @property
    def dimension(self) -> int:
        return 1 << self.orbitals

    def _orbital(self, site: int, spin: int) -> int:
        return 2 * site + spin

    def _parity(self, state: int, orbital: int) -> int:
        return -1 if (state & ((1 << orbital) - 1)).bit_count() % 2 else 1

    def _annihilate(self, state: int, orbital: int) -> tuple[int, int] | None:
        if not ((state >> orbital) & 1):
            return None
        sign = self._parity(state, orbital)
        return state ^ (1 << orbital), sign

    def _create(self, state: int, orbital: int) -> tuple[int, int] | None:
        if (state >> orbital) & 1:
            return None
        sign = self._parity(state, orbital)
        return state | (1 << orbital), sign

    def hamiltonian(self) -> sparse.csr_matrix:
        n = self.n_sites
        dim = self.dimension
        rows: list[int] = []
        cols: list[int] = []
        data: list[float] = []

        def put(row: int, col: int, value: float) -> None:
            if abs(value) > 1e-15:
                rows.append(row)
                cols.append(col)
                data.append(float(value))

        for state in range(dim):
            for site in range(n):
                up = self._orbital(site, 0)
                down = self._orbital(site, 1)
                put(state, state, self.interaction * ((state >> up) & 1) * ((state >> down) & 1))
                put(state, state, -self.chemical_potential * (((state >> up) & 1) + ((state >> down) & 1)))
            for spin in (0, 1):
                for site in range(n - 1):
                    p = self._orbital(site, spin)
                    q = self._orbital(site + 1, spin)
                    for left, right in ((p, q), (q, p)):
                        ann = self._annihilate(state, left)
                        if ann is None:
                            continue
                        mid, sign_a = ann
                        cre = self._create(mid, right)
                        if cre is None:
                            continue
                        out, sign_c = cre
                        put(out, state, -self.hopping * sign_a * sign_c)
                if self.periodic and n > 2:
                    p = self._orbital(n - 1, spin)
                    q = self._orbital(0, spin)
                    for left, right in ((p, q), (q, p)):
                        ann = self._annihilate(state, left)
                        if ann is None:
                            continue
                        mid, sign_a = ann
                        cre = self._create(mid, right)
                        if cre is None:
                            continue
                        out, sign_c = cre
                        put(out, state, -self.hopping * sign_a * sign_c)
        h = sparse.coo_matrix((data, (rows, cols)), shape=(dim, dim), dtype=float).tocsr()
        h.sum_duplicates()
        return h

    def ground_state(self) -> tuple[float, np.ndarray, list[float]]:
        h = self.hamiltonian()
        dim = h.shape[0]
        if dim <= 4096:
            evals, evecs = np.linalg.eigh(h.toarray())
        else:
            k = min(4, dim - 1)
            evals, evecs = sparse_linalg.eigsh(h, k=k, which="SA")
        order = np.argsort(evals)
        evals = evals[order]
        vec = np.asarray(evecs[:, order[0]], dtype=float)
        return float(evals[0]), vec, [float(x) for x in evals[:4]]

    def describe(self) -> dict[str, Any]:
        return {
            "model": "fermi-hubbard-1d",
            "n_sites": self.n_sites,
            "hopping": self.hopping,
            "interaction": self.interaction,
            "chemical_potential": self.chemical_potential,
            "periodic": self.periodic,
            "dimension": self.dimension,
            "solver": "exact-fermionic-occupation-basis",
        }
