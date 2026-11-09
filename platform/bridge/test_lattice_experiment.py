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
