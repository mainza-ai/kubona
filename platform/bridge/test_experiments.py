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
    assert res["ansatz_qasm"].startswith("OPENQASM")
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

    res = run_vqe(molecule="LiH", bond_distance=1.6, layers=1, shots=128,
                  seed=7, max_iterations=1, full_space=True)
    assert res["success"] is True
    assert res["active_space"]["full_space"] is True
    assert res["qubit_count"] == 12
    assert res["method"].startswith("ADAPT-VQE")
    assert len(res["selected_operators"]) >= 1
    # Full-space is an explicitly labelled path; it does not borrow the
    # active-space CASCI target.
    assert res["fci_exact_energy"] == res["active_space_fci_energy"]
    assert res["claimable"] is False
    # One optimization pass per selected operator is not allowed to claim a
    # converged answer; resource exhaustion is the honest terminal state.
    assert res["full_space_status"] in ("resource_exhausted", "contested", "measurement_noise")
    assert res["chemical_accuracy"] is False
    assert res["uncertainty"] > 0


def test_qaoa_hardware_loop_measures_every_objective(monkeypatch):
    import experiments

    calls = []

    def fake_batch(qasm_list, shots, seed, backend=None):
        calls.append((list(qasm_list), shots, seed, backend))
        return {"counts": [{"01": 512}], "backend": "ibm-test",
                "hardware_job_id": f"hw-qaoa-loop-{len(calls)}", "usage_seconds": 0.25,
                "actual_usd": 0.0}

    monkeypatch.setattr(experiments, "_hardware_batch", fake_batch)
    res = experiments.run_qaoa_hardware_loop(
        nodes=2, edges=[[0, 1]], optimal_cut=1, layers=1, shots=512,
        seed=7, max_iterations=1, backend="ibm-test",
    )
    assert res["success"] is True
    assert res["kind"] == "qaoa_hardware_loop"
    assert res["simulated_optimizer"] is False
    assert res["measured_cut"] == 1.0
    assert len(calls) >= 1
    assert res["iteration_batches"] == len(calls)
    assert res["hardware_job_ids"]


def test_hardware_vqe_uses_one_batch_and_measures_every_term(monkeypatch):
    import experiments

    calls = []

    def fake_batch(qasm_list, shots, seed, backend=None):
        calls.append((list(qasm_list), shots, seed, backend))
        return {
            "success": True,
            "counts": [{"00": 512} for _ in qasm_list],
            "backend": "ibm-test",
            "hardware_job_id": "hw-1",
            "usage_seconds": 2.0,
            "actual_usd": 0.0,
        }

    monkeypatch.setattr(experiments, "_hardware_batch", fake_batch)
    res = experiments.run_vqe_hardware(
        theta=[0.1] * 8,
        pauli_coefficients={"II": -1.0, "IZ": 0.2, "ZI": 0.1},
        layers=2,
        shots=512,
        seed=7,
        bond_distance=0.7414,
        fci_exact_energy=-0.6,
        backend="ibm-test",
    )
    assert res["success"] is True
    assert res["hardware"] is True
    assert res["simulated_optimizer"] is True
    assert res["hardware_job_id"] == "hw-1"
    assert res["usage_seconds"] == 2.0
    assert res["actual_usd"] == 0.0
    assert len(calls) == 1
    assert len(calls[0][0]) == 3
    assert calls[0][1] == 512
    assert calls[0][3] == "ibm-test"
    # All measured counts are |00>, so every non-II expectation is +1.
    assert res["energy"] == pytest.approx(-0.7)
    assert res["claimable"] is False


def test_hardware_vqe_loop_makes_every_objective_a_batch(monkeypatch):
    import experiments

    calls = []

    def fake_batch(qasm_list, shots, seed, backend=None):
        calls.append((list(qasm_list), shots, seed, backend))
        return {"counts": [{"00": 512} for _ in qasm_list], "backend": "ibm-test",
                "hardware_job_id": f"hw-loop-{len(calls)}", "usage_seconds": 0.5,
                "actual_usd": 0.0}

    monkeypatch.setattr(experiments, "_hardware_batch", fake_batch)
    res = experiments.run_vqe_hardware_loop(
        theta=[0.1] * 8, pauli_coefficients={"II": -1.0, "ZZ": 0.25},
        layers=2, shots=512, seed=7, bond_distance=0.7414,
        fci_exact_energy=-0.75, backend="ibm-test", max_iterations=1,
    )
    assert res["success"] is True
    assert res["kind"] == "vqe_hardware_loop"
    assert res["simulated_optimizer"] is False
    assert res["hardware"] is True
    assert len(calls) >= 1
    assert res["iteration_batches"] == len(calls)
    assert len(res["hardware_job_ids"]) == len(calls)
    assert all(len(call[0]) == 2 for call in calls)


def test_hardware_vqe_loop_uses_qpy_for_active_space(monkeypatch):
    import experiments

    src = experiments.run_vqe(molecule="LiH", bond_distance=1.5, layers=1,
                              shots=128, seed=7, max_iterations=1, full_space=False)
    assert src["success"] and src["ansatz_qpy"]

    calls = []

    def fake_batch(qasm_list, shots, seed, backend=None):
        calls.append((list(qasm_list), shots, seed, backend))
        return {"counts": [{"0000": 512} for _ in qasm_list], "backend": "ibm-test",
                "hardware_job_id": f"hw-active-{len(calls)}", "usage_seconds": 0.25,
                "actual_usd": 0.0}

    monkeypatch.setattr(experiments, "_hardware_batch", fake_batch)
    res = experiments.run_vqe_hardware_loop(
        theta=src["theta"], pauli_coefficients=src["pauli_coefficients"],
        ansatz_qpy=src["ansatz_qpy"], molecule="LiH", layers=1,
        shots=512, seed=7, bond_distance=1.5,
        fci_exact_energy=src["fci_exact_energy"], constant_energy=src["constant_energy"],
        backend="ibm-test", max_iterations=1,
    )
    assert res["success"] is True
    assert res["kind"] == "vqe_hardware_loop"
    assert res["ansatz"].startswith("active-space UCCSD")
    assert res["simulated_optimizer"] is False
    assert len(calls) >= 1


def test_hardware_vqe_zne_extrapolates_energy_from_measured_terms(monkeypatch):
    import experiments

    calls = []

    def fake_batch(qasm_list, shots, seed, backend=None):
        calls.append((list(qasm_list), shots, seed, backend))
        return {"counts": [{"00": 512} for _ in qasm_list], "backend": "ibm-test",
                "hardware_job_id": "hw-zne", "usage_seconds": 3.0, "actual_usd": 0.0}

    monkeypatch.setattr(experiments, "_hardware_batch", fake_batch)
    res = experiments.run_vqe_hardware(
        theta=[0.1] * 8,
        pauli_coefficients={"II": -1.0, "ZZ": 0.25},
        layers=2, shots=512, seed=7, bond_distance=0.7414,
        fci_exact_energy=-0.75, backend="ibm-test",
        mitigation="ZNE", mitigation_scales=[1, 3, 5],
    )
    assert res["success"] is True
    assert res["mitigated"] is True
    assert res["measured"] is False
    assert res["mitigation"]["method"] == "ZNE"
    assert res["mitigation"]["scales"] == [1, 3, 5]
    assert res["mitigation"]["mitigated_measured"] is False
    assert res["claimable"] is False
    assert res["chemical_accuracy"] is False
    assert res["energy"] == pytest.approx(-0.75)
    assert len(calls) == 1
    assert len(calls[0][0]) == 6
    assert res["hardware_job_id"] == "hw-zne"


def test_qaoa_hardware_computes_cut_from_one_batch(monkeypatch):
    import experiments

    calls = []

    def fake_batch(qasm_list, shots, seed, backend=None):
        calls.append((list(qasm_list), shots, seed, backend))
        return {"counts": [{"01": 512}], "backend": "ibm-test",
                "hardware_job_id": "hw-qaoa", "usage_seconds": 1.0, "actual_usd": 0.0}

    monkeypatch.setattr(experiments, "_hardware_batch", fake_batch)
    qasm = "OPENQASM 2.0;\nqreg q[2];\ncreg c[2];\nmeasure q -> c;"
    res = experiments.run_qaoa_hardware(
        qasm=qasm, optimal_cut=2, nodes=2, edges=[[0, 1]], shots=512, seed=7,
        energy_exact=2.0, backend="ibm-test",
    )
    assert res["success"] is True
    assert res["kind"] == "qaoa_hardware"
    assert res["measured_cut"] == 1.0
    assert res["approximation_ratio"] == 0.5
    assert res["hardware"] is True
    assert res["hardware_job_id"] == "hw-qaoa"
    assert calls == [([qasm], 512, 7, "ibm-test")]


def test_pes_supports_active_space_lih_and_reports_each_point():
    from experiments import run_pes

    res = run_pes(molecule="LiH", r_min=1.4, r_max=1.6, steps=2, layers=1,
                  shots=128, seed=7, max_iterations=2)
    assert res["success"] is True
    assert res["molecule"] == "LiH"
    assert res["full_space"] is False
    assert len(res["points"]) == 2
    for point in res["points"]:
        assert point["energy"] == point["energy"]
        assert point["uncertainty"] > 0
        assert point["fci_exact_energy"] != 0
        assert point["method"].startswith("VQE UCCSD")


def test_hardware_vqe_can_measure_a_nature_ansatz_qasm(monkeypatch):
    import experiments

    calls = []

    def fake_batch(qasm_list, shots, seed, backend=None):
        calls.append((list(qasm_list), shots, seed, backend))
        return {"counts": [{"00": 512} for _ in qasm_list], "backend": "ibm-test",
                "hardware_job_id": "hw-nature", "usage_seconds": 2.0, "actual_usd": 0.0}

    monkeypatch.setattr(experiments, "_hardware_batch", fake_batch)
    qasm = "OPENQASM 2.0;\ninclude \"qelib1.inc\";\nqreg q[2];\ncreg c[2];\nh q[0];\ncx q[0], q[1];"
    res = experiments.run_vqe_hardware(
        theta=[], pauli_coefficients={"II": -1.0, "ZZ": 0.25},
        molecule="LiH", ansatz_qasm=qasm, constant_energy=-7.0, layers=0,
        shots=512, seed=7, bond_distance=1.5, fci_exact_energy=-7.0,
        backend="ibm-test",
    )
    assert res["success"] is True
    assert res["molecule"] == "LiH"
    assert res["constant_energy"] == -7.0
    assert res["layers"] == 1
    assert res["energy"] == pytest.approx(-7.0 + (-1.0 + 0.25))
    assert len(calls) == 1


def test_nature_vqe_exposes_hardware_ansatz_and_terms():
    from experiments import run_vqe

    res = run_vqe(molecule="LiH", bond_distance=1.5, layers=1, shots=128,
                  seed=7, max_iterations=1, full_space=False)
    assert res["success"] is True
    assert res["ansatz_qasm"].startswith("OPENQASM")
    assert res["layers"] == 1
    assert res["pauli_coefficients"]
    assert res["constant_energy"] != 0
    assert res["active_space"]["full_space"] is False


def test_h2o_active_space_vqe_reports_casci_and_full_fci():
    from experiments import run_vqe

    res = run_vqe(molecule="H2O", bond_distance=0.96, shots=1024,
                  max_iterations=6, layers=2)
    assert res["success"] is True, res
    assert res["molecule"] == "H2O"
    assert res["qubit_count"] == 8
    assert res["active_space"]["ncas"] == 4
    assert res["active_space"]["nelecas"] == 4
    assert res["fci_exact_energy"] < res["active_space_fci_energy"] < 0
    assert res["measured"] is True
    assert res["claimable"] is False


def test_h2_excited_state_deflation_reports_gap_and_overlap():
    from experiments import run_excited_state

    res = run_excited_state("H2", bond_distance=0.7414, layers=2, shots=8192,
                            max_iterations=60, seed=7)
    assert res["success"] is True, res
    assert res["kind"] == "excited_state"
    assert res["result_version"] == "experiment-result-v1"
    assert res["measured"] is True
    assert len(res["spectrum"]) >= 2
    assert res["excited_energy"] > res["ground_energy"]
    assert res["gap"] > 0
    assert res["overlap_squared"] < 1e-2
    assert res["claimable"] is False


def test_h2_reaction_energy_uses_two_endpoint_anchors():
    from experiments import run_reaction_energy

    res = run_reaction_energy("H2", 0.7414, 10.0, shots=2048, max_iterations=20)
    assert res["success"] is True, res
    assert res["kind"] == "reaction"
    assert res["result_version"] == "experiment-result-v1"
    assert res["reaction_energy_exact"] > 0
    assert res["reactant_energy_exact"] < res["reactant_energy"]
    assert res["claimable"] is False
    assert res["measured"] is True


def test_lih_active_space_excited_state_reports_full_and_active_anchors():
    from experiments import run_excited_state

    res = run_excited_state("LiH", 1.5957, shots=512, max_iterations=8, penalty=10)
    assert res["success"] is True, res
    assert res["kind"] == "excited_state"
    assert res["molecule"] == "LiH"
    assert res["active_space"]["ncas"] == 2
    assert res["active_space"]["nelecas"] == 2
    assert res["fci_exact_energy"] < res["active_space_fci_energy"] < 0
    assert len(res["spectrum"]) >= 2
    assert res["measured"] is True


def test_water_reaction_subtracts_products_and_scopes_the_anchor(monkeypatch):
    import experiments

    monkeypatch.setattr(experiments, "_molecule_atom", lambda spec, r: "fake-water")
    monkeypatch.setattr(experiments, "_full_fci_and_casci", lambda atom, ncas, nelecas: (-76.0, -75.9))
    monkeypatch.setattr(experiments, "_oxygen_atom_fci", lambda: -74.0)
    monkeypatch.setattr(experiments, "h2_hamiltonian", lambda r: (None, -1.1, -1.0))
    calls = []

    def fake_vqe(molecule, bond_distance, layers, shots, seed, max_iterations):
        calls.append((molecule, layers))
        if molecule == "H2O":
            return {"success": True, "energy": -75.8, "uncertainty": 0.001,
                    "active_space_chemical_accuracy": True, "claimable": False}
        return {"success": True, "energy": -1.0, "uncertainty": 0.001, "claimable": True}

    monkeypatch.setattr(experiments, "run_vqe", fake_vqe)
    res = experiments.run_reaction_energy("H2O", 0.96, shots=128, layers=3, max_iterations=1)
    assert res["success"] is True
    assert res["reaction_energy"] == pytest.approx(-0.8)
    assert res["reaction_energy_exact"] == pytest.approx(-0.8)
    assert res["reactant_energy_exact"] == pytest.approx(-75.9)
    assert res["reaction_energy_exact_full"] == pytest.approx(-0.9)
    assert res["claimable"] is True
    assert res["claimability_scope"].startswith("H2O active-space")
    assert calls == [("H2O", 3), ("H2", 3)]
