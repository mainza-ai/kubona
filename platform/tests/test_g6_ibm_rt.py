"""IBM/Qiskit provider real-hardware path (G6): Aer fallback + Runtime route.

The IBM Runtime path is validated against a recorded fixture (a stubbed
qiskit_ibm_runtime module); no live hardware call happens in CI. Live IBM runs
require KUBONA_IBM_QISKIT_TOKEN + network, and are exercised on opted-in backends.
"""

import sys
import types

import pytest

from kubona.circuits import Circuit, Gate
from kubona.providers.qiskit_provider import QiskitProvider
from kubona.schema import load_provider


BELL = Circuit(name="bell", num_qubits=2,
               gates=[Gate("h", (0,)), Gate("cx", (0, 1))])


@pytest.fixture
def fake_runtime(monkeypatch):
    """Stub qiskit_ibm_runtime with a recorded-fixture service/sampler."""
    captured = {}

    def _service(**kw):
        captured["token"] = kw.get("token")
        class B: pass
        return types.SimpleNamespace(backend=lambda name: B())

    def _sampler(backend):
        def run(circuits, shots=None):
            captured["shots"] = shots
            class Pub:
                def get_counts(self): return {"00": 256, "11": 256}
            # real PrimitiveResult is subscriptable: result[0].data.meas
            def result():
                pub = types.SimpleNamespace(data=types.SimpleNamespace(meas=Pub()))
                return (pub,)
            class Job:
                def job_id(self): return "ibm-job-1"
                def result(self): return result()
            return Job()
        return types.SimpleNamespace(run=run)

    mod = types.ModuleType("qiskit_ibm_runtime")
    mod.QiskitRuntimeService = _service
    mod.SamplerV2 = _sampler
    monkeypatch.setitem(sys.modules, "qiskit_ibm_runtime", mod)
    return captured


def test_aer_fallback_without_credentials(monkeypatch):
    """No token -> REFUSAL (W2.18, no silent Aer downgrade for hardware
    modalities); explicit allow_sim_fallback=True opts into a LABELED Aer
    run (open-source first, never silent)."""
    from kubona.providers.base import ProviderError

    monkeypatch.delenv("KUBONA_IBM_QISKIT_TOKEN", raising=False)
    monkeypatch.delenv("BAO_ADDR", raising=False)
    p = QiskitProvider(load_provider("ibm-qiskit"), backend_name="ibm_brisbane")
    with pytest.raises(ProviderError, match="allow_sim_fallback"):
        p.submit(BELL, shots=512, seed=7)
    out = p.submit(BELL, shots=512, seed=7, allow_sim_fallback=True)
    assert out["hardware"] is False and out["backend"] == "qiskit-aer"
    assert out["sim_fallback"] is True
    assert set(out["counts"]) <= {"00", "11"}


def test_ibm_runtime_path_when_credentials_present(fake_runtime, monkeypatch):
    """With a token configured, the provider routes to real IBM Runtime."""
    monkeypatch.setenv("KUBONA_IBM_QISKIT_TOKEN", "test-token")
    p = QiskitProvider(load_provider("ibm-qiskit"), backend_name="ibm_brisbane")
    out = p.submit(BELL, shots=512, seed=7)
    assert out["hardware"] is True and out["job_id"] == "ibm-job-1"
    assert out["counts"] == {"00": 256, "11": 256}
    assert fake_runtime["token"] == "test-token"
    assert fake_runtime["shots"] == 512
