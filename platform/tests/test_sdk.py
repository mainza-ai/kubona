"""Provider-registration SDK: the documented plugin API (Phase-4 gate)."""

import pytest

from kubona.providers.base import ProviderError
from kubona.sdk import register_provider, validate_capability


def test_validate_capability_rejects_bad_schema():
    with pytest.raises(Exception):
        validate_capability({"provider": 42})  # not a string -> pydantic fails


def test_register_local_simulator_plugin(tmp_path, monkeypatch):
    """Register a new provider through the SDK (data + plugin, sandbox-tested)."""
    cap = {
        "provider": "sdk-test-sim",
        "modality": "statevector_simulator",
        "physical_qubits": 4,
        "native_gate_set": ["h", "x"],
        "pricing": {"model": "free"},
        "status": "beta",
    }
    # write to the real schema dir (isolated via monkeypatch of SCHEMA_DIR)
    import kubona.sdk.register as reg

    monkeypatch.setattr(reg, "SCHEMA_DIR", tmp_path / "providers")
    (tmp_path / "providers").mkdir(exist_ok=True)

    out = register_provider(cap, "kubona.providers.local_simulator.LocalSimulator",
                            sandbox_test=True, write=True)
    assert out["provider"] == "sdk-test-sim"
    assert out["smoke"]["ok"] is True
    assert out["written"] is True
    assert (tmp_path / "providers" / "sdk-test-sim.json").exists()


def test_register_rejects_missing_plugin():
    cap = {"provider": "ghost", "modality": "gate_model", "pricing": {"model": "free"}}
    with pytest.raises(ProviderError):
        register_provider(cap, "kubona.providers.does_not_exist.NoSuch", sandbox_test=False, write=False)
