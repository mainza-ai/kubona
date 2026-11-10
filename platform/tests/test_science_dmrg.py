"""DMRG finite-size anchor checks for the classical spin-chain baseline."""

import pytest

from kubona.science.dmrg import dmrg_ground_energy
from kubona.science.lattice import SpinChainModel


def test_dmrg_matches_exact_finite_size_tfim():
    result = dmrg_ground_energy(
        model="tfim", n_sites=4, coupling=1.0, field=1.0,
        bond_dim=8, sweeps=6, tolerance=1e-10,
    )
    assert result["success"] is True, result
    assert result["boundary"] == "open"
    assert result["claimable"] is False
    exact = SpinChainModel(
        model="tfim", n_sites=4, coupling=1.0, field=1.0, periodic=False,
    ).ground_state()[0]
    assert result["energy"] == pytest.approx(exact, abs=1e-6)


def test_dmrg_refuses_invalid_bond_dimension_and_site_count():
    assert dmrg_ground_energy(model="tfim", n_sites=3)["success"] is False
    assert dmrg_ground_energy(model="tfim", n_sites=8, bond_dim=2)["success"] is False
    assert dmrg_ground_energy(model="tfim", n_sites=8, cutoff=0)["success"] is False
    assert dmrg_ground_energy(model="tfim", n_sites=8, coupling=0, field=0)["success"] is False
