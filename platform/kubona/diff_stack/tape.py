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
        from pennylane import numpy as pnp
        w = pnp.array(weights, requires_grad=True)
        grad_fn = qml.grad(cost_fn)
        return np.array(grad_fn(w))

    def create_noisy_vqe_qnode(
        self,
        hamiltonian_coeffs: List[float],
        hamiltonian_observables: List[Any],
        diff_method: str = "parameter-shift",
        noise_channel: str = "phase_damping",
    ) -> Tuple[Callable[[Any, Any], float], qml.Hamiltonian]:
        """Creates a differentiable QNode with parameterizable noise rate phi on default.mixed."""
        dev = qml.device("default.mixed", wires=self.n_qubits)
        H = qml.Hamiltonian(hamiltonian_coeffs, hamiltonian_observables)

        @qml.qnode(dev, diff_method=diff_method)
        def noisy_cost_fn(weights, phi):
            for i in range(self.n_qubits):
                qml.RY(weights[i], wires=i)
            for i in range(self.n_qubits - 1):
                qml.CNOT(wires=[i, i + 1])
                if noise_channel == "phase_damping" and phi > 0.0:
                    qml.PhaseDamping(phi, wires=i)
                    qml.PhaseDamping(phi, wires=i + 1)
                elif noise_channel == "depolarizing" and phi > 0.0:
                    qml.DepolarizingChannel(phi, wires=i)
                    qml.DepolarizingChannel(phi, wires=i + 1)
            for i in range(self.n_qubits):
                qml.RY(weights[self.n_qubits + i], wires=i)
                if noise_channel == "phase_damping" and phi > 0.0:
                    qml.PhaseDamping(phi, wires=i)
                elif noise_channel == "depolarizing" and phi > 0.0:
                    qml.DepolarizingChannel(phi, wires=i)
            return qml.expval(H)

        return noisy_cost_fn, H

    def compute_joint_gradients(
        self,
        cost_fn: Callable[[Any, Any], float],
        weights: np.ndarray,
        phi: float,
    ) -> Tuple[np.ndarray, float]:
        """Computes joint parameter-shift gradients with respect to both theta (weights) and phi (noise)."""
        from pennylane import numpy as pnp
        w = pnp.array(weights, requires_grad=True)
        p = pnp.array(phi, requires_grad=True)
        jac = qml.jacobian(cost_fn)(w, p)
        grad_w = np.array(jac[0])
        grad_p = float(jac[1])
        return grad_w, grad_p
