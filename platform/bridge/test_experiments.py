"""Real VQE/PES runner tests (bridge image dependencies)."""
from __future__ import annotations

import pytest


def test_h2_vqe_is_measured_and_near_fci():
    from experiments import run_vqe

    res = run_vqe(molecule="H2", bond_distance=0.7414, layers=2,
                  shots=8192, seed=7, max_iterations=80)
    assert res["success"] is True
    assert res["result_version"] == "experiment-result-v1"
    assert res["measured"] is True
    assert res["method"].startswith("VQE")
    assert res["energy_measured"] == res["energy"]
    assert res["fci_exact_energy"] == pytest.approx(-1.137270174660903, abs=1e-12)
    # 8k shots is noisy; the exact optimizer point is the convergence check.
    assert abs(res["energy_exact"] - res["fci_exact_energy"]) < 2e-3
    assert res["uncertainty"] > 0
    assert res["trajectory"]


def test_unsupported_molecule_refuses():
    from experiments import run_vqe

    res = run_vqe(molecule="CH4")
    assert res["success"] is False
    assert "refusing" in res["error"]


def test_pes_points_are_measured():
    from experiments import run_pes

    res = run_pes(r_min=0.7, r_max=0.78, steps=2, shots=2048,
                  max_iterations=48)
    assert res["success"] is True
    assert res["result_version"] == "experiment-result-v1"
    assert [p["bond_distance"] for p in res["points"]] == [0.7, 0.78]
    assert all(p["energy"] != 0 for p in res["points"])
    assert all(p["fci_exact_energy"] != 0 for p in res["points"])


def test_full_space_vqe_reports_the_full_orbital_space():
    from experiments import run_vqe

    res = run_vqe(molecule="LiH", bond_distance=1.6, layers=2, shots=128,
                  seed=7, max_iterations=1, full_space=True)
    assert res["success"] is True
    assert res["active_space"]["full_space"] is True
    assert res["qubit_count"] == 12
    # Full-space is an explicitly labelled path; it does not borrow the
    # active-space CASCI target.
    assert res["fci_exact_energy"] == res["active_space_fci_energy"]
