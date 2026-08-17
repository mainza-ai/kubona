"""Zero-Noise Extrapolation (ZNE) — the first mitigation technique (Phase 1).

Pipeline: run the circuit at increasing noise scales via global unitary folding
(C -> C C^dag C), estimate the observable at each scale, then extrapolate to
zero-noise (\u03bb -> 0) with Richardson's method.

Mirrors Mitiq's approach but is implemented in-house on the noise-aware backend,
so the eval harness has a controllable, auditable reference implementation.
"""

from __future__ import annotations

from dataclasses import dataclass

import numpy as np

from kubona.circuits import Circuit, Gate
from kubona.mitigation.backend import NoiseAwareSimulator

OBSERVABLES = {"z": lambda q, n: _observable_z(q, n)}


def _observable_z(q: int, n: int) -> np.ndarray:
    """Diagonal Z-on-qubit-q observable in the computational basis."""
    s = 1 << (n - 1 - q)
    diag = np.array([1.0 if (i & s) == 0 else -1.0 for i in range(1 << n)])
    return diag


def fold_gates(circuit: Circuit, factor: int) -> Circuit:
    """Global unitary folding: repeat each gate (2k+1) times for scale 1+2k.

    factor = 1 -> identity (scale 1); factor = 3 -> each gate repeated 3x (scale 3).
    """
    gates: list[Gate] = []
    for g in circuit.gates:
        for _ in range(factor):
            gates.append(g)
    return Circuit(name=circuit.name, num_qubits=circuit.num_qubits, gates=gates)


def expectation_from_counts(counts: dict[str, int], obs: np.ndarray, n: int) -> float:
    total = sum(counts.values()) or 1
    e = 0.0
    for key, c in counts.items():
        i = int(key, 2)
        e += c * obs[i]
    return e / total


def _richardson(scales: list[float], values: list[float]) -> float:
    """Polynomial extrapolation to \u03bb=0 (Richardson): fit degree n-1, eval at 0."""
    coeffs = np.polyfit(scales, values, deg=len(scales) - 1)
    return float(np.polyval(coeffs, 0.0))


@dataclass
class ZNEResult:
    raw_value: float
    mitigated_value: float
    scale_values: dict[float, float]
    technique: str = "ZNE"
    fold: str = "global"
    extrapolator: str = "Richardson"

    def to_dict(self) -> dict:
        return {"raw_value": self.raw_value, "mitigated_value": self.mitigated_value,
                "scale_values": {str(k): v for k, v in self.scale_values.items()},
                "technique": self.technique, "fold": self.fold, "extrapolator": self.extrapolator}


def run_zne(circuit: Circuit, observable: str = "z", qubit: int = 0,
            shots: int = 8000, noise_rate: float = 0.02,
            scales: tuple[int, ...] = (1, 3, 5), seed: int = 7) -> ZNEResult:
    """Run ZNE on a circuit for a single-qubit Z observable."""
    backend = NoiseAwareSimulator(noise_rate=noise_rate)
    n = circuit.num_qubits
    obs = _observable_z(qubit, n)
    scale_values: dict[float, float] = {}
    raw = None
    for s in scales:
        folded = circuit if s == 1 else fold_gates(circuit, s)
        counts = backend.run(folded, shots=shots, seed=seed)["counts"]
        val = expectation_from_counts(counts, obs, n)
        if s == 1:
            raw = val
        scale_values[float(s)] = val
    mitigated = _richardson(list(map(float, scales)), list(scale_values.values()))
    return ZNEResult(raw_value=float(raw), mitigated_value=mitigated, scale_values=scale_values)
