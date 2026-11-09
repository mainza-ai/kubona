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
        molecule="LiH", ansatz_qasm=qasm, constant_energy=-7.0,
        shots=512, seed=7, bond_distance=1.5, fci_exact_energy=-7.0,
        backend="ibm-test",
    )
    assert res["success"] is True
    assert res["molecule"] == "LiH"
    assert res["constant_energy"] == -7.0
    assert res["energy"] == pytest.approx(-7.0 + (-1.0 + 0.25))
    assert len(calls) == 1


def test_nature_vqe_exposes_hardware_ansatz_and_terms():
    from experiments import run_vqe

    res = run_vqe(molecule="LiH", bond_distance=1.5, layers=1, shots=128,
                  seed=7, max_iterations=1, full_space=False)
    assert res["success"] is True
    assert res["ansatz_qasm"].startswith("OPENQASM")
    assert res["pauli_coefficients"]
    assert res["constant_energy"] != 0
    assert res["active_space"]["full_space"] is False
