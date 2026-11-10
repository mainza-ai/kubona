import math

import numpy as np
import pytest

from kubona.science.lattice import HubbardModel, SpinChainModel


def test_tfim_two_site_known_ground_energy():
    model = SpinChainModel("tfim", n_sites=2, coupling=1.0, field=0.0, periodic=False)
    energy, vec, _evals = model.ground_state()
    assert energy == pytest.approx(-1.0)
    assert model.observables(vec).gap == pytest.approx(2.0)


def test_tfim_field_only_is_negative_sum_of_fields():
    model = SpinChainModel("tfim", n_sites=3, coupling=0.0, field=1.0, periodic=False)
    energy, _vec, _evals = model.ground_state()
    assert energy == pytest.approx(-3.0)


def test_heisenberg_two_site_singlet_energy():
    model = SpinChainModel("heisenberg", n_sites=2, coupling=1.0, periodic=False)
    energy, _vec, evals = model.ground_state()
    assert energy == pytest.approx(-3.0)
    assert evals[1] == pytest.approx(1.0)


def test_tfim_hamiltonian_is_hermitian_and_observables_are_finite():
    model = SpinChainModel("tfim", n_sites=4, coupling=1.0, field=0.73, periodic=True)
    h = model.hamiltonian()
    diff = h - h.getH()
    assert diff.nnz == 0 or float(np.max(np.abs(diff.data))) <= 1e-12
    energy, vec, _evals = model.ground_state()
    obs = model.observables(vec)
    assert math.isfinite(energy)
    assert len(obs.magnetization_z) == 4
    assert len(obs.correlation_zz) == 4
    assert obs.entanglement_entropy_half >= 0.0
    assert math.isfinite(obs.gap)


def test_spin_chain_refuses_bad_domain():
    with pytest.raises(ValueError, match="n_sites"):
        SpinChainModel(n_sites=1)
    with pytest.raises(ValueError, match="model"):
        SpinChainModel(model="unknown")
    with pytest.raises(ValueError, match="non-zero"):
        SpinChainModel("xxz", n_sites=3, coupling=0.0)


def test_hubbard_model_ground_state_is_real_and_finite():
    model = HubbardModel(n_sites=2, hopping=1.0, interaction=4.0, periodic=False)
    energy, vec, evals = model.ground_state()
    assert math.isfinite(energy)
    assert np.isrealobj(vec)
    assert len(evals) >= 1
    assert model.describe()["solver"] == "exact-fermionic-occupation-basis"


def test_pauli_terms_are_consistent_with_hamiltonian():
    model = SpinChainModel("tfim", n_sites=3, coupling=0.7, field=0.3, periodic=False)
    h = model.hamiltonian().toarray()
    rebuilt = np.zeros_like(h)
    pauli = {
        "I": np.eye(2),
        "X": np.array([[0, 1], [1, 0]], dtype=complex),
        "Y": np.array([[0, -1j], [1j, 0]], dtype=complex),
        "Z": np.array([[1, 0], [0, -1]], dtype=complex),
    }
    for label, coeff in model.pauli_terms():
        mat = np.array([[1.0 + 0j]])
        for char in label:
            mat = np.kron(mat, pauli[char])
        rebuilt += coeff * mat
    assert np.max(np.abs(rebuilt - h)) < 1e-12


def test_observables_use_the_supplied_state_not_the_ground_state():
    model = SpinChainModel("tfim", n_sites=2, coupling=0.0, field=1.0, periodic=False)
    excited = np.zeros(4, dtype=complex)
    excited[1] = 1.0
    obs = model.observables(excited)
    assert obs.magnetization_z == pytest.approx([-1.0, 1.0])
    assert obs.gap == pytest.approx(2.0)
    with pytest.raises(ValueError, match="expected 4"):
        model.observables(np.zeros(3, dtype=complex))
