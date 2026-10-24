"""Provider capability schema tests."""

from kubona.schema import load_all_providers, load_provider


def test_initial_registry():
    caps = load_all_providers()
    assert set(caps) == {"local-simulator", "ibm-qiskit", "qiskit-aer-simulator",
                         "quantinuum", "rigetti",
                         "neutral-atom-quera-pasqal", "xanadu-strawberry-fields",
                         "dwave-annealing", "ionq", "aws-braket"}
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


def test_per_second_estimate_has_time_model_and_basis():
    from kubona.circuits import parse_qasm2
    cap = load_provider("ibm-qiskit")
    assert cap.pricing.per_second_usd > 0, "rate must be configured, never 0.0"
    c = parse_qasm2("OPENQASM 2.0;\nqreg q[2];\nh q[0];\ncx q[0],q[1];")
    est = cap.cost_estimate(4000, c)
    assert est["estimated_usd"] > 0
    assert "timing" in est and est["timing"]["billed_seconds"] >= 1.0
    assert "cost_basis" in est and "ibm.com/quantum/pricing" in est["cost_basis"]


def test_per_second_without_circuit_refuses():
    import pytest
    cap = load_provider("ibm-qiskit")
    with pytest.raises(ValueError):
        cap.cost_estimate(4000)


def test_zero_rate_hardware_refuses_not_free():
    # A $0 estimate reads as "free tier" downstream and approves hardware
    # spend without a budget. Zero-rate hardware models must refuse, never
    # estimate. (quantinuum/neutral-atom/dwave/ionq/braket ship 0.0
    # rates today.)
    import pytest
    for name in ("quantinuum", "neutral-atom-quera-pasqal", "dwave-annealing",
                 "ionq", "aws-braket"):
        cap = load_provider(name)
        with pytest.raises(ValueError, match="no task/shot rate configured"):
            cap.cost_estimate(1000)


def test_per_shot_zero_rate_refuses():
    import pytest
    from kubona.schema.provider import PricingModel
    cap = load_provider("quantinuum")
    zeroed = cap.model_copy(update={"pricing": PricingModel(
        model="per_shot", currency="USD", per_shot_usd=0.0,
        notes="zero-rate probe")})
    with pytest.raises(ValueError, match="no per-shot rate configured"):
        zeroed.cost_estimate(1000)


def test_per_second_preflight_requires_ceiling():
    from kubona.circuits import parse_qasm2
    from kubona.cost.estimator import preflight
    cap = load_provider("ibm-qiskit")
    c = parse_qasm2("OPENQASM 2.0;\nqreg q[2];\nh q[0];\ncx q[0],q[1];")
    assert preflight(cap, c, 4000)["preflight_ok"] is False
    assert preflight(cap, c, 4000, 150.0)["preflight_ok"] is True
