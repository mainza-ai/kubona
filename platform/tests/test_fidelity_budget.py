"""Fidelity budgets: leave-one-out mechanism shares (first-order, honest)."""

from kubona.fidelity.accounting import DEVICE_CATALOG, FidelityAccountingEngine


def test_ideal_device_has_no_budget():
    import dataclasses
    engine = FidelityAccountingEngine()
    perfect = dataclasses.replace(
        DEVICE_CATALOG["local-simulator"], t1_us=1e18, t2_us=1e18)
    out = engine.fidelity_budget(perfect, 2, 1, 1)
    assert out["predicted_fidelity"] == 1.0
    assert all(e["absolute_gain"] == 0.0 for e in out["mechanisms"])
    assert out["dominant"] is None


def test_heron_bell_budget_names_readout():
    engine = FidelityAccountingEngine()
    out = engine.fidelity_budget(DEVICE_CATALOG["ibm-heron"], 2, 1, 1, 1.0)
    assert 0 < out["predicted_fidelity"] < 1.0
    assert out["dominant"] == "readout"
    shares = {e["name"]: e["share"] for e in out["mechanisms"]}
    assert abs(sum(shares.values()) - 1.0) < 1e-3
    # Absolute gains are the honest numbers; shares only rank.
    gains = {e["name"]: e["absolute_gain"] for e in out["mechanisms"]}
    assert gains["readout"] > gains["decoherence"] > gains["two_qubit_gates"] > gains["single_qubit_gates"]
    for e in out["mechanisms"]:
        assert e["fidelity_without"] >= out["predicted_fidelity"]


def test_deeper_circuit_shifts_budget_not_verdict_shape():
    engine = FidelityAccountingEngine()
    dev = DEVICE_CATALOG["ibm-heron"]
    shallow = engine.fidelity_budget(dev, 2, 1, 1, 1.0)
    deep = engine.fidelity_budget(dev, 8, 40, 60, 5.0)
    assert deep["predicted_fidelity"] < shallow["predicted_fidelity"]
    assert set(e["name"] for e in deep["mechanisms"]) == {
        "decoherence", "single_qubit_gates", "two_qubit_gates", "readout"}


def test_fidelity_contract_parity_between_python_and_json():
    import json
    from pathlib import Path
    from kubona.fidelity.accounting import DEVICE_CATALOG, PROVIDER_DEVICE_MAP
    from kubona.mcp.servers import execution as exec_server

    contract_path = Path(__file__).resolve().parents[1] / "contracts" / "fidelity-devices.json"
    raw = json.loads(contract_path.read_text(encoding="utf-8"))
    assert PROVIDER_DEVICE_MAP == raw["provider_device_map"]
    assert exec_server._FIDELITY_DEVICE_FOR_PROVIDER == raw["provider_device_map"]
    for dev in raw["devices"]:
        name = dev["name"]
        prof = DEVICE_CATALOG[name]
        assert prof.t1_us == dev["t1_us"]
        assert prof.t2_us == dev["t2_us"]
        assert prof.single_qubit_error == dev["single_qubit_error"]
        assert prof.two_qubit_error == dev["two_qubit_error"]
        assert prof.readout_error == dev["readout_error"]
        assert prof.qubits == dev["qubits"]
        assert prof.source == dev["source"]
        for alias in dev.get("aliases", []):
            assert alias in DEVICE_CATALOG

