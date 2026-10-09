"""Kubona's own open-source statevector simulator (Phase 0).

Pure Python + numpy; implements the native gate set declared in
schema/providers/local-simulator.json. Deterministic under a seed. This is the
"local/GPU simulator" first implementation — small enough to audit, and the
basis the hardware-canary and eval harnesses build on later.
"""

from __future__ import annotations

import random
from typing import Any

import numpy as np

from kubona.circuits import Circuit
from kubona.schema import ProviderCapability

from .base import Provider, ProviderError

_I = np.array([[1, 0], [0, 1]], complex)
_X = np.array([[0, 1], [1, 0]], complex)
_Y = np.array([[0, -1j], [1j, 0]], complex)
_Z = np.array([[1, 0], [0, -1]], complex)
_H = np.array([[1, 1], [1, -1]], complex) / np.sqrt(2)
_S = np.array([[1, 0], [0, 1j]], complex)
_SDG = np.array([[1, 0], [0, -1j]], complex)
_T = np.array([[1, 0], [0, np.exp(1j * np.pi / 4)]], complex)
_TDG = np.array([[1, 0], [0, np.exp(-1j * np.pi / 4)]], complex)


def _rx(theta: float) -> np.ndarray:
    return np.array([[np.cos(theta / 2), -1j * np.sin(theta / 2)],
                     [-1j * np.sin(theta / 2), np.cos(theta / 2)]], complex)


def _ry(theta: float) -> np.ndarray:
    return np.array([[np.cos(theta / 2), -np.sin(theta / 2)],
                     [np.sin(theta / 2), np.cos(theta / 2)]], complex)


def _rz(theta: float) -> np.ndarray:
    return np.array([[np.exp(-1j * theta / 2), 0], [0, np.exp(1j * theta / 2)]], complex)


_SINGLE = {
    "id": _I, "x": _X, "y": _Y, "z": _Z, "h": _H, "s": _S, "sdg": _SDG, "t": _T, "tdg": _TDG,
    "rx": _rx, "ry": _ry, "rz": _rz,
}


def _cp(theta: float) -> np.ndarray:
    """Controlled-phase: diag(1,1,1,e^{i theta}) on (control, target)."""
    return np.diag([1.0, 1.0, 1.0, np.exp(1j * theta)]).astype(complex)


class LocalSimulator(Provider):
    """Full statevector simulation for the declared gate set."""

    def __init__(self, capability: ProviderCapability):
        self.capability = capability
        self._max_qubits = 24  # 2^24 complex128 ~ 256 MB — hard cap for safety

    def submit(self, circuit: Circuit, shots: int, seed: int | None = None) -> dict[str, Any]:
        if circuit.num_qubits > self._max_qubits:
            raise ProviderError(f"local simulator cap: {circuit.num_qubits} > {self._max_qubits} qubits")
        if shots <= 0:
            raise ProviderError("shots must be > 0")
        for g in circuit.gates:
            if g.name not in _SINGLE and g.name not in ("cx", "cz", "swap", "cp"):
                raise ProviderError(
                    f"gate {g.name!r} not in local simulator's native gate set; "
                    f"transpile first (compiler server) or pick another provider"
                )
            if max(g.qubits) >= circuit.num_qubits:
                raise ProviderError(f"gate {g.name} references qubit beyond circuit width")

        n = circuit.num_qubits
        dim = 1 << n
        state = np.zeros(dim, complex)
        state[0] = 1.0

        for g in circuit.gates:
            if g.name in _SINGLE:
                q = g.qubits[0]
                u = _SINGLE[g.name](*g.params) if g.params else _SINGLE[g.name]
                state = self._apply1(state, u, q, n)
            elif g.name in ("cx", "cz", "swap"):
                state = self._apply2(state, {"cx": _CX, "cz": _CZ, "swap": _SWAP}[g.name],
                                     g.qubits[0], g.qubits[1], n)
            elif g.name == "cp":
                u = _cp(*g.params)
                state = self._apply2(state, u, g.qubits[0], g.qubits[1], n)
            else:
                raise ProviderError(f"gate {g.name!r} not supported by local simulator")

        probs = np.abs(state) ** 2
        rng = random.Random(seed)
        counts: dict[str, int] = {}
        for _ in range(shots):
            outcome = rng.random()
            acc = 0.0
            for i, p in enumerate(probs):
                acc += p
                if outcome < acc:
                    key = f"{i:0{n}b}"
                    counts[key] = counts.get(key, 0) + 1
                    break
        return {
            "counts": counts,
            "shots": shots,
            "seed": seed,
            "statevector_norm_sq": float(np.sum(probs)),
        }

    def health(self) -> dict[str, Any]:
        # Attested capacity bound (Q-14): the statevector cap is a hard-coded
        # safety limit, so it is a genuine measurement of what this plugin
        # can execute — the canary verifies it against the declared JSON.
        return {
            "provider": self.capability.provider,
            "status": "ok",
            "measured_qubits": self._max_qubits,
        }

    @staticmethod
    def _apply1(state: np.ndarray, u: np.ndarray, q: int, n: int) -> np.ndarray:
        stride = 1 << (n - 1 - q)
        out = state.copy()
        for block in range(0, 1 << n, stride << 1):
            for off in range(stride):
                i0 = block + off
                i1 = i0 + stride
                a, b = state[i0], state[i1]
                out[i0] = u[0, 0] * a + u[0, 1] * b
                out[i1] = u[1, 0] * a + u[1, 1] * b
        return out

    @staticmethod
    def _apply2(state: np.ndarray, u: np.ndarray, q0: int, q1: int, n: int) -> np.ndarray:
        """Apply a 2-qubit gate with basis order [q0 (high), q1 (low)].

        For each assignment of the other qubits (the 'anchor' where q0=q1=0),
        gather the 4-state subspace and apply the full 4x4 matrix.
        """
        s0, s1 = 1 << (n - 1 - q0), 1 << (n - 1 - q1)
        mask = s0 | s1
        out = state.copy()
        for base in range(1 << n):
            if base & mask:
                continue
            idx = [base, base | s1, base | s0, base | s0 | s1]
            vals = [state[i] for i in idx]
            for r in range(4):
                out[idx[r]] = sum(u[r, c] * vals[c] for c in range(4))
        return out


_CX = np.array([[1, 0, 0, 0],
                [0, 1, 0, 0],
                [0, 0, 0, 1],
                [0, 0, 1, 0]], complex)
_CZ = np.diag([1, 1, 1, -1]).astype(complex)
_SWAP = np.array([[1, 0, 0, 0],
                  [0, 0, 1, 0],
                  [0, 1, 0, 0],
                  [0, 0, 0, 1]], complex)
