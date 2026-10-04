"""
PennyLane Graph-Partitioned Circuit Cutting & Knitting Module.
Implements wire cutting via quasi-probability decomposition (QPD) and LOCC classical recombination.
Replaces previous stubs with real PennyLane qcut transformations.
"""
from __future__ import annotations

from typing import Dict, Any, List
import pennylane as qml
import numpy as np


def cut_and_reconstruct_expectation(
    n_qubits: int,
    rotations: List[float],
    cut_wire: int = 0,
) -> Dict[str, Any]:
    """
    Executes a multi-qubit entangling circuit using PennyLane qcut,
    partitioning across the specified wire cut and reconstructing the expectation value.
    """
    dev = qml.device("default.qubit", wires=n_qubits)

    # 1. Uncut baseline
    @qml.qnode(dev)
    def uncut_circuit(params):
        for i in range(n_qubits):
            p = params[i] if i < len(params) else 0.5
            qml.RX(p, wires=i)
        for i in range(n_qubits - 1):
            qml.CNOT(wires=[i, i + 1])
        obs = qml.PauliZ(0)
        for i in range(1, n_qubits):
            obs = obs @ qml.PauliZ(i)
        return qml.expval(obs)

    # 2. Cut circuit
    @qml.cut_circuit
    @qml.qnode(dev)
    def cut_circuit(params):
        for i in range(n_qubits):
            p = params[i] if i < len(params) else 0.5
            qml.RX(p, wires=i)
        if cut_wire < n_qubits - 1:
            qml.WireCut(wires=cut_wire)
        for i in range(n_qubits - 1):
            qml.CNOT(wires=[i, i + 1])
        obs = qml.PauliZ(0)
        for i in range(1, n_qubits):
            obs = obs @ qml.PauliZ(i)
        return qml.expval(obs)

    params = np.array(rotations, dtype=float)
    uncut_val = float(uncut_circuit(params))
    cut_val = float(cut_circuit(params))
    reconstruction_error = abs(uncut_val - cut_val)

    return {
        "n_qubits": n_qubits,
        "cut_wire": cut_wire,
        "uncut_expectation": uncut_val,
        "cut_reconstructed_expectation": cut_val,
        "reconstruction_error": float(reconstruction_error),
        "method": "PennyLane QPD Wire Cut",
        "overhead_scale": float(4.0 ** 1), # Single wire cut overhead
    }
