"""Provider capability schema tests."""

from kubona.schema import load_all_providers, load_provider


def test_initial_registry():
    caps = load_all_providers()
    assert set(caps) == {"local-simulator", "ibm-qiskit", "qiskit-aer-simulator",
                         "quantinuum", "rigetti",
                         "neutral-atom-quera-pasqal", "xanadu-strawberry-fields",
                         "dwave-annealing"}
    assert caps["local-simulator"].pricing.model == "free"
    assert caps["local-simulator"].execution_plugin == "kubona.providers.local_simulator.LocalSimulator"
    assert caps["ibm-qiskit"].pricing.model == "per_second"
    assert caps["quantinuum"].native_gate_set == ["rzz", "rz", "rx"]
    assert caps["neutral-atom-quera-pasqal"].modality.value == "neutral_atom"


def test_cost_estimate():
    cap = load_provider("local-simulator")
    est = cap.cost_estimate(shots=4096)
    assert est["estimated_usd"] == 0.0
    assert est["pricing_model"] == "free"
