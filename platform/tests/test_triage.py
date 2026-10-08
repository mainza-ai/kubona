"""Tests for the Family 278 Quantum Triage Engine."""

import pytest
from kubona.science.triage import QuantumTriageEngine, MolecularSystem, TriageTarget


def test_single_reference_routes_to_classical_dft():
    # H2O or Benzene (single reference)
    water = MolecularSystem(
        molecule_id="H2O",
        formula="H2O",
        num_electrons=10,
        nuclear_charges=[8, 1, 1],
        is_multi_reference=False,
        coulomb_continuum=True,
        estimated_active_space_orbitals=6,
    )
    result = QuantumTriageEngine.triage(water)
    assert result["target"] == TriageTarget.CLASSICAL_DFT_HPC.value
    assert not result["requires_qpu"]
    assert "PySCF_DFT_B3LYP" in result["algorithm_recommendation"]


def test_small_multireference_routes_to_classical_dmrg():
    # Fe(II) heme active site (small active space: 14 electrons in 14 orbitals)
    heme_core = MolecularSystem(
        molecule_id="Fe_heme_core",
        formula="FeC4N4",
        num_electrons=34,
        nuclear_charges=[26, 6, 6, 6, 6, 7, 7, 7, 7],
        is_multi_reference=True,
        coulomb_continuum=True,
        estimated_active_space_orbitals=14,
    )
    result = QuantumTriageEngine.triage(heme_core)
    assert result["target"] == TriageTarget.CLASSICAL_DMRG_TENSOR.value
    assert not result["requires_qpu"]
    assert result["family_id"] == 265
    assert "Classical_DMRG_Tensor_Network" in result["algorithm_recommendation"]


def test_large_multireference_routes_to_qpu():
    # FeMoco nitrogenase active cluster (Fe7MoS9C) - strongly correlated multi-reference active space > 50
    femoco = MolecularSystem(
        molecule_id="FeMoco",
        formula="Fe7MoS9C",
        num_electrons=112,
        nuclear_charges=[26] * 7 + [42] + [16] * 9 + [6],
        is_multi_reference=True,
        coulomb_continuum=True,
        estimated_active_space_orbitals=74,
    )
    result = QuantumTriageEngine.triage(femoco)
    assert result["target"] == TriageTarget.FEDERATED_QUANTUM_QPU.value
    assert result["requires_qpu"]
    assert result["family_id"] == 278
    assert "Quantum_Krylov_Subspace_Diagonalization" in result["algorithm_recommendation"]
    assert result["complexity_regime"] == "QMA_Hard_Family_275"
