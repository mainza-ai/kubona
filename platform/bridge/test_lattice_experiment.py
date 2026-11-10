import math

import pytest

from lattice_experiment import run_spin_chain


def test_tfim_experiment_is_shot_measured_and_anchored():
    res = run_spin_chain(
        model="tfim", n_sites=3, coupling=1.0, field=0.7,
        periodic=False, layers=2, shots=12000, seed=3,
    )
    assert res["success"] is True, res
    assert res["kind"] == "spin_chain"
    assert res["result_version"] == "experiment-result-v1"
    assert res["measured"] is True
    assert res["hardware"] is False
    assert res["simulated_optimizer"] is True
    assert math.isfinite(res["energy"])
    assert math.isfinite(res["energy_exact"])
    assert res["uncertainty"] >= 0.0
    assert len(res["measured_magnetization_z"]) == 3
    assert len(res["exact_correlation_zz"]) == 2
    assert res["gap"] > 0.0
    assert "ground_state_reached" in res
    assert "observable_state" in res


def test_exact_state_mode_measures_the_known_ground_state():
    res = run_spin_chain(
        model="tfim", n_sites=4, coupling=1.0, field=1.0,
        periodic=True, layers=2, shots=12000, seed=9, state_mode="exact",
    )
    assert res["success"] is True, res
    assert res["state_mode"] == "exact"
    assert res["simulated_optimizer"] is False
    assert res["ground_state_reached"] is True
    assert res["claimable"] is False
    assert abs(res["energy"] - res["energy_exact"]) < 5 * res["uncertainty"]


def test_conjecture_requires_valid_domain():
    res = run_spin_chain(model="not-a-model", n_sites=4)
    assert res["success"] is False
    assert "model" in res["error"]

    res = run_spin_chain(model="tfim", n_sites=20)
    assert res["success"] is False
    assert "n_sites" in res["error"]


def test_heisenberg_experiment_reports_singlet_anchor():
    res = run_spin_chain(
        model="heisenberg", n_sites=2, coupling=1.0,
        periodic=False, layers=2, shots=8000, seed=7,
    )
    assert res["success"] is True, res
    assert res["energy_exact"] == pytest.approx(-3.0, abs=1e-9)


def test_bridge_run_experiment_dispatches_spin_chain():
    import json
    from types import SimpleNamespace
    from server import QuantumBridgeServicer

    svc = QuantumBridgeServicer()
    resp = svc.RunExperiment(
        SimpleNamespace(
            spec_json=json.dumps({
                "kind": "spin_chain", "model": "tfim", "n_sites": 3,
                "coupling": 1.0, "field": 0.7, "periodic": False,
                "layers": 2, "shots": 1000, "seed": 4,
            }),
            session_id="test-session",
        ),
        None,
    )
    assert resp.ok is True, resp.error_message
    result = json.loads(resp.result_json)
    assert result["kind"] == "spin_chain"
    assert result["measured"] is True


def test_bridge_run_experiment_dispatches_conjecture():
    import json
    from types import SimpleNamespace
    from server import QuantumBridgeServicer

    svc = QuantumBridgeServicer()
    resp = svc.RunExperiment(
        SimpleNamespace(
            spec_json=json.dumps({
                "kind": "conjecture", "values": [1, 1, 2, 3, 5, 8, 13, 21, 34],
                "holdout": 2,
            }),
            session_id="test-session",
        ),
        None,
    )
    assert resp.ok is True, resp.error_message
    result = json.loads(resp.result_json)
    assert result["kind"] == "conjecture"
    assert result["candidate_count"] >= 1
    assert result["measured"] is False


def test_spin_chain_hardware_uses_one_governed_batch(monkeypatch):
    import experiments
    from lattice_experiment import run_spin_chain_hardware

    calls = []

    def fake_batch(qasms, shots, seed, backend=None):
        calls.append({"n": len(qasms), "shots": shots, "backend": backend})
        return {
            "counts": [{"000": shots} for _ in qasms],
            "backend": backend or "fake-qpu",
            "hardware_job_id": "vendor-spin-1",
            "usage_seconds": 2.0,
            "actual_usd": 0.0,
        }

    monkeypatch.setattr(experiments, "_hardware_batch", fake_batch)
    res = run_spin_chain_hardware(
        qasm="OPENQASM 2.0; include \"qelib1.inc\"; qreg q[3];",
        pauli_coefficients={"ZZI": 1.0, "IZZ": 1.0, "XXX": 0.5},
        n_sites=3,
        energy_exact=2.5,
        model="tfim",
        shots=1000,
        backend="ibm_fez",
        state_mode="vqe",
    )
    assert res["success"] is True, res
    assert res["kind"] == "spin_chain_hardware"
    assert res["hardware"] is True
    assert res["measured"] is True
    assert res["hardware_job_id"] == "vendor-spin-1"
    assert calls and calls[0]["n"] == 4
    assert res["energy"] == pytest.approx(2.5)
    assert res["claimable"] is True


def test_spin_chain_hardware_refuses_exact_state_qasm():
    from lattice_experiment import run_spin_chain_hardware

    res = run_spin_chain_hardware(
        qasm="OPENQASM 2.0; qreg q[2];",
        pauli_coefficients={"ZZ": 1.0},
        n_sites=2,
        energy_exact=1.0,
        state_mode="exact",
    )
    assert res["success"] is False
    assert "state_mode" in res["error"]


def test_spin_chain_hardware_maps_source_labels_to_qiskit_order(monkeypatch):
    import experiments
    from qiskit import qasm2
    from lattice_experiment import run_spin_chain_hardware

    captured = []

    def fake_batch(qasms, shots, seed, backend=None):
        captured.extend(qasms)
        return {
            "counts": [{"00": shots} for _ in qasms],
            "backend": backend or "fake-qpu",
            "hardware_job_id": "vendor-spin-labels",
            "usage_seconds": 0.1,
            "actual_usd": 0.0,
        }

    monkeypatch.setattr(experiments, "_hardware_batch", fake_batch)
    res = run_spin_chain_hardware(
        qasm="OPENQASM 2.0; include \"qelib1.inc\"; qreg q[2];",
        pauli_coefficients={"XI": 1.0},
        n_sites=2,
        energy_exact=1.0,
        model="tfim",
        shots=128,
        backend="ibm_fez",
        state_mode="vqe",
    )
    assert res["success"] is True, res
    assert "XI" in res["measurements"]
    # The source label XI means X on qubit 0. Qiskit order reverses the label,
    # so the prepared measurement must put H on qubit 0, not qubit 1.
    first = qasm2.loads(captured[0])
    h_qubits = [
        first.qubits.index(inst.qubits[0])
        for inst in first.data
        if inst.operation.name == "h"
    ]
    assert h_qubits == [0]


def test_square_lattice_quench_dynamics_is_exact_and_unclaimable():
    from lattice_experiment import run_lattice_dynamics

    res = run_lattice_dynamics(rows=2, cols=2, coupling=1.0,
                               initial_field=1.0, quench_field=0.2,
                               times=[0.0, 0.5, 1.0], periodic=True)
    assert res["success"] is True, res
    assert res["kind"] == "lattice_dynamics"
    assert res["n_sites"] == 4
    assert len(res["times"]) == 3
    assert res["times"][0]["total_magnetization"] == pytest.approx(0.0, abs=1e-12)
    assert res["measured"] is False
    assert res["claimable"] is False
