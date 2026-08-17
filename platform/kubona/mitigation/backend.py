"""Noise-aware density-matrix backend (Phase 1).

Adds a per-gate depolarizing noise model so mitigation (ZNE) has something real
to fight. Small-qubit density-matrix simulation (n <= 8) — the eval/provenance
server and the autoresearch harnesses build on this later.
"""

from __future__ import annotations

import numpy as np

from kubona.circuits import Circuit
from kubona.providers.local_simulator import (_H, _I, _S, _SDG, _T, _TDG, _X, _Y, _Z,
                                              _rx, _ry, _rz)

_SINGLE = {"id": _I, "x": _X, "y": _Y, "z": _Z, "h": _H, "s": _S, "sdg": _SDG, "t": _T, "tdg": _TDG,
           "rx": _rx, "ry": _ry, "rz": _rz}
_CX = np.array([[1, 0, 0, 0], [0, 1, 0, 0], [0, 0, 0, 1], [0, 0, 1, 0]], complex)
_CZ = np.diag([1, 1, 1, -1]).astype(complex)
_SWAP = np.array([[1, 0, 0, 0], [0, 0, 1, 0], [0, 1, 0, 0], [0, 0, 0, 1]], complex)


def _embed(u2: np.ndarray, q: int, n: int) -> np.ndarray:
    """Place a 2x2 gate on qubit q of an n-qubit system (big-endian indexing)."""
    dim = 1 << n
    s = 1 << (n - 1 - q)
    U = np.zeros((dim, dim), complex)
    for i in range(dim):
        b = (i >> (n - 1 - q)) & 1
        U[i, i] = u2[b, b]
        U[i, i ^ s] = u2[b, 1 - b]
    return U


def _embed2(u4: np.ndarray, q0: int, q1: int, n: int) -> np.ndarray:
    """Place a 4x4 gate on (q0, q1) of an n-qubit system (q0 is the high bit)."""
    dim = 1 << n
    s0, s1 = 1 << (n - 1 - q0), 1 << (n - 1 - q1)
    U = np.zeros((dim, dim), complex)
    for base in range(dim):
        if base & (s0 | s1):
            continue
        idx = [base, base | s1, base | s0, base | s0 | s1]
        for r, i in enumerate(idx):
            for c, j in enumerate(idx):
                U[i, j] = u4[r, c]
    return U


class NoiseAwareSimulator:
    """Density-matrix simulator with a per-gate depolarizing channel."""

    def __init__(self, noise_rate: float = 0.0):
        if not (0.0 <= noise_rate <= 1.0):
            raise ValueError("noise_rate must be in [0,1]")
        self.noise_rate = noise_rate

    @staticmethod
    def _depolarize(rho: np.ndarray, p: float, n: int) -> np.ndarray:
        """rho -> (1-p) rho + (p/3)(X rho X + Y rho Y + Z rho Z) on every qubit."""
        if p == 0.0:
            return rho
        out = (1 - p) * rho
        for q in range(n):
            for m in (_X, _Y, _Z):
                M = _embed(m, q, n)
                out += (p / 3) * (M @ rho @ M.conj().T)
        return out

    def run(self, circuit: Circuit, shots: int, seed: int | None = None) -> dict:
        n = circuit.num_qubits
        if n > 8:
            raise ValueError("noise-aware backend is small-qubit (n <= 8)")
        dim = 1 << n
        rho = np.zeros((dim, dim), complex)
        rho[0, 0] = 1.0
        for g in circuit.gates:
            if g.name in _SINGLE:
                u = _SINGLE[g.name](*g.params) if g.params else _SINGLE[g.name]
                U = _embed(u, g.qubits[0], n)
            elif g.name in ("cx", "cz", "swap"):
                U = _embed2({"cx": _CX, "cz": _CZ, "swap": _SWAP}[g.name], g.qubits[0], g.qubits[1], n)
            else:
                raise ValueError(f"gate {g.name} not supported by noise-aware backend")
            rho = U @ rho @ U.conj().T
            rho = self._depolarize(rho, self.noise_rate, n)
        probs = np.clip(np.real(np.diag(rho)), 0, 1)
        probs = probs / probs.sum()
        rng = np.random.default_rng(seed)
        counts = rng.multinomial(shots, probs)
        return {"counts": {f"{i:0{n}b}": int(c) for i, c in enumerate(counts) if c},
                "shots": shots, "noise_rate": self.noise_rate}
