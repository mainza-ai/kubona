"""Tests for Differentiable Quantum Stack & Unified Parameter-Shift Tape."""

import numpy as np
import pennylane as qml
import pytest

from kubona.diff_stack.tape import DifferentiableQuantumTape


def test_noiseless_vqe_gradient():
    tape = DifferentiableQuantumTape(n_qubits=2)
    coeffs = [1.0, 0.5]
    obs = [qml.PauliZ(0) @ qml.PauliZ(1), qml.PauliX(0) @ qml.PauliX(1)]
    cost_fn, H = tape.create_vqe_qnode(coeffs, obs)

    weights = np.array([0.1, 0.2, 0.3, 0.4])
    grad = tape.compute_gradient(cost_fn, weights)

    assert grad.shape == (4,)
    assert not np.isnan(grad).any()
    assert not np.allclose(grad, 0.0)


def test_noisy_joint_parameter_shift_gradient():
    tape = DifferentiableQuantumTape(n_qubits=2)
    coeffs = [1.0, 0.5]
    obs = [qml.PauliZ(0) @ qml.PauliZ(1), qml.PauliX(0) @ qml.PauliX(1)]
    noisy_cost_fn, H = tape.create_noisy_vqe_qnode(coeffs, obs, noise_channel="phase_damping")

    weights = np.array([0.1, 0.2, 0.3, 0.4])
    phi = 0.05
    grad_w, grad_p = tape.compute_joint_gradients(noisy_cost_fn, weights, phi)

    assert grad_w.shape == (4,)
    assert isinstance(grad_p, float)
    assert not np.isnan(grad_w).any()
    assert not np.isnan(grad_p)
    # Physical invariant: increasing phase damping drives coherence toward 0, affecting expectation value
    assert grad_p != 0.0
