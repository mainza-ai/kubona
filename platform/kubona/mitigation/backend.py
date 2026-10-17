"""Noise-aware density-matrix backend (Phase 1).

Adds a per-gate depolarizing noise model so mitigation (ZNE) has something real
to fight. Small-qubit density-matrix simulation (n <= 8) — the eval/provenance
server and the autoresearch harnesses build on this later.

Noise model (trace-preserving by construction): after each gate, every qubit
the gate ACTED on passes through a single-qubit depolarizing channel
  rho -> (1-p) rho + (p/3) (X rho X + Y rho Y + Z rho Z),
applied sequentially per acted qubit. Each step preserves Tr(rho) = 1, so the
final state is physical up to fp noise — asserted, not renormalized into
silence. (The old code applied p on EVERY qubit per gate, giving
Tr = 1 + (n-1)*p, then hid it with clip+renormalize.)
"""

from __future__ import annotations

import math
import secrets

import numpy as np

from kubona.circuits import Circuit
from kubona.circuits.model import SUPPORTED_GATES, SUPPORTED_GATES_1Q, SUPPORTED_GATES_2Q
from kubona.providers.local_simulator import (_H, _I, _S, _SDG, _T, _TDG, _X, _Y, _Z,
                                              _cp, _rx, _ry, _rz)

_SINGLE = {"id": _I, "x": _X, "y": _Y, "z": _Z, "h": _H, "s": _S, "sdg": _SDG, "t": _T, "tdg": _TDG,
           "rx": _rx, "ry": _ry, "rz": _rz}
_CX = np.array([[1, 0, 0, 0], [0, 1, 0, 0], [0, 0, 0, 1], [0, 0, 1, 0]], complex)
_CZ = np.diag([1, 1, 1, -1]).astype(complex)
_SWAP = np.array([[1, 0, 0, 0], [0, 0, 1, 0], [0, 1, 0, 0], [0, 0, 0, 1]], complex)

#: Trace-deviation tolerance (fp noise only — the channel is exactly TP).
TRACE_TOL_V1 = 1e-9
#: Negative-probability refusal floor (magnitudes below this are fp noise).
PROB_NEG_TOL_V1 = 1e-9


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
        if not isinstance(noise_rate, (int, float, np.floating)) \
                or not math.isfinite(noise_rate) \
                or not 0.0 <= noise_rate <= 1.0:
            raise ValueError(f"noise_rate must be finite in [0,1], got {noise_rate!r}")
        self.noise_rate = float(noise_rate)

    @staticmethod
    def _depolarize(rho: np.ndarray, p: float, qubits: tuple[int, ...], n: int) -> np.ndarray:
        """Single-qubit depolarizing channel on exactly the ACTED qubits.

        rho -> (1-p) rho + (p/3)(X rho X + Y rho Y + Z rho Z) per acted qubit,
        applied sequentially. Each application is trace-preserving, so the
        composition is: Tr(out) == Tr(rho) up to fp noise. Qubits the gate did
        not touch are NOT noised (the old code noised all n qubits per gate).
        """
        if p == 0.0:
            return rho
        out = rho
        for q in qubits:
            stepped = (1 - p) * out
            for m in (_X, _Y, _Z):
                M = _embed(m, q, n)
                stepped = stepped + (p / 3) * (M @ out @ M.conj().T)
            out = stepped
        return out

    def _validate_gate(self, g, n: int) -> None:
        """Arity / parameter / range validation with the supported set named."""
        if g.name not in SUPPORTED_GATES:
            raise ValueError(
                f"gate {g.name!r} not supported by noise-aware backend; "
                f"supported: {sorted(SUPPORTED_GATES)}"
            )
        want_qubits = 1 if g.name in SUPPORTED_GATES_1Q else 2
        if len(g.qubits) != want_qubits:
            raise ValueError(
                f"gate {g.name!r} needs {want_qubits} qubit(s), got {len(g.qubits)}"
            )
        if any(q < 0 or q >= n for q in g.qubits):
            raise ValueError(
                f"gate {g.name!r} qubit index out of range for n={n}: {g.qubits!r}"
            )
        want_params = 1 if g.name in ("rx", "ry", "rz", "cp") else 0
        if len(g.params) != want_params:
            raise ValueError(
                f"gate {g.name!r} needs {want_params} param(s), got {len(g.params)}"
            )
        if any(not math.isfinite(float(p)) for p in g.params):
            raise ValueError(f"gate {g.name!r} has non-finite params {g.params!r}")

    def run(self, circuit: Circuit, shots: int, seed: int | None = None) -> dict:
        if isinstance(shots, bool) or not isinstance(shots, int) or shots < 1:
            raise ValueError(f"shots must be a positive int, got {shots!r}")
        n = circuit.num_qubits
        if n > 8:
            raise ValueError("noise-aware backend is small-qubit (n <= 8)")
        # Seed: generated + echoed. An unseeded run is reproducible ONLY
        # because the used seed is returned — never a silent default.
        used_seed = int(seed) if seed is not None else secrets.randbits(63)
        dim = 1 << n
        rho = np.zeros((dim, dim), complex)
        rho[0, 0] = 1.0
        for g in circuit.gates:
            self._validate_gate(g, n)
            if g.name in _SINGLE:
                u = _SINGLE[g.name](*g.params) if g.params else _SINGLE[g.name]
                U = _embed(u, g.qubits[0], n)
            elif g.name in ("cx", "cz", "swap"):
                U = _embed2({"cx": _CX, "cz": _CZ, "swap": _SWAP}[g.name], g.qubits[0], g.qubits[1], n)
            elif g.name == "cp":
                U = _embed2(_cp(*g.params), g.qubits[0], g.qubits[1], n)
            else:  # pragma: no cover — _validate_gate already refused
                raise ValueError(
                    f"gate {g.name!r} not supported by noise-aware backend; "
                    f"supported: {sorted(SUPPORTED_GATES)}"
                )
            rho = U @ rho @ U.conj().T
            rho = self._depolarize(rho, self.noise_rate, tuple(g.qubits), n)
        # Trace assertion: the channel is exactly trace-preserving, so a
        # deviation beyond fp noise means the simulation broke — REFUSE
        # instead of clip+renormalize (which would launder the breakage).
        trace = float(np.real(np.trace(rho)))
        if not math.isfinite(trace) or abs(trace - 1.0) > TRACE_TOL_V1:
            raise ValueError(
                f"depolarizing simulation broke trace preservation: Tr(rho)={trace!r} "
                f"(tol {TRACE_TOL_V1}): refusing instead of renormalizing"
            )
        probs = np.real(np.diag(rho))
        worst_neg = float(np.min(probs))
        if not math.isfinite(worst_neg) or worst_neg < -PROB_NEG_TOL_V1:
            raise ValueError(
                f"unphysical measurement probabilities (min {worst_neg:.3e}): refusing"
            )
        # Below-tolerance fp fuzz is zeroed WITHOUT rescaling: the trace
        # assertion above already bounds sum(probs) to 1 +- TRACE_TOL_V1, so
        # this is fp cleanup, not physics laundering.
        probs = np.maximum(probs, 0.0)
        rng = np.random.default_rng(used_seed)
        counts = rng.multinomial(shots, probs / probs.sum())
        return {"counts": {f"{i:0{n}b}": int(c) for i, c in enumerate(counts) if c},
                "shots": shots, "noise_rate": self.noise_rate, "seed": used_seed}
