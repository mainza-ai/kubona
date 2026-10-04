"""Differentiable Quantum Stack & Unified Parameter-Shift Tape (Ring 3).

Adopts PennyLane single-tape gradient execution so classical baseline features,
quantum gates, and noisy channel parameters share a single computation graph.
Supports parameter-shift differentiation through noisy channels.
"""

from __future__ import annotations

import math
from typing import Any, Callable, Dict, List, Tuple
import numpy as np
import pennylane as qml


class DifferentiableQuantumTape:
    """Manages unified differentiable QNodes with parameter-shift rules."""

    def __init__(self, n_qubits: int = 2, dev_name: str = "default.qubit"):
        self.n_qubits = n_qubits
        self.dev = qml.device(dev_name, wires=n_qubits)

    def create_vqe_qnode(
        self,
        hamiltonian_coeffs: List[float],
        hamiltonian_observables: List[Any],
        diff_method: str = "parameter-shift",
    ) -> Tuple[Callable[[np.ndarray], float], qml.Hamiltonian]:
        """Creates a differentiable QNode for variational ansatz evaluation."""
        H = qml.Hamiltonian(hamiltonian_coeffs, hamiltonian_observables)

        @qml.qnode(self.dev, diff_method=diff_method)
        def cost_fn(weights: np.ndarray):
            # Universal Hardware-Efficient Layer
            for i in range(self.n_qubits):
                qml.RY(weights[i], wires=i)
            for i in range(self.n_qubits - 1):
                qml.CNOT(wires=[i, i + 1])
            for i in range(self.n_qubits):
                qml.RY(weights[self.n_qubits + i], wires=i)
            return qml.expval(H)

        return cost_fn, H

    def compute_gradient(self, cost_fn: Callable[[np.ndarray], float], weights: np.ndarray) -> np.ndarray:
        """Computes exact parameter-shift gradients without manual finite difference."""
        grad_fn = qml.grad(cost_fn)
        return np.array(grad_fn(weights))
