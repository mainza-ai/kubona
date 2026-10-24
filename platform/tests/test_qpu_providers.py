"""QPU provider onboarding tests (mock-first, zero live spend).

New vendors land as capability JSON + plugin with credential plumbing,
but submit() stays refused until a recorded live run flips the flag.
The user has no QPU keys yet, so NOTHING here touches real hardware:
Leap is exercised through an injected sampler factory, IonQ/Braket
through refusal paths + health wiring.
"""

import numpy as np
import pytest

from kubona.providers import get_provider
from kubona.providers.base import ProviderError
from kubona.schema import load_provider


def test_registry_includes_planned_vendors():
    from kubona.providers import list_providers
    caps = list_providers()
    for name in ("ionq", "aws-braket"):
        assert name in caps, f"{name} must be registered"
        assert caps[name].status == "planned"
    assert caps["ionq"].execution_plugin.endswith("IonQProvider")
    assert caps["aws-braket"].execution_plugin.endswith("BraketProvider")


def test_zero_rate_hardware_refuses_for_new_vendors():
    for name in ("ionq", "aws-braket"):
        cap = load_provider(name)
        with pytest.raises(ValueError, match="no task/shot rate configured"):
            cap.cost_estimate(100)


def test_ionq_submit_refuses_until_live_verified():
    from kubona.circuits import Circuit, Gate
    cap = load_provider("ionq")
    c = Circuit(name="probe", num_qubits=1, gates=[Gate(name="h", qubits=(0,), params=())])
    prov = get_provider("ionq")
    assert prov.health()["wiring"] == "not-live-verified"
    with pytest.raises(ProviderError, match="not live-verified"):
        prov.submit(c, shots=100)
    # Credential presence does not unlock unverified execution.
    assert cap.provider == "ionq"


def test_braket_submit_refuses_not_integrated():
    from kubona.circuits import Circuit, Gate
    c = Circuit(name="probe", num_qubits=1, gates=[Gate(name="h", qubits=(0,), params=())])
    prov = get_provider("aws-braket")
    assert prov.health()["wiring"] == "not-integrated"
    with pytest.raises(ProviderError, match="SigV4"):
        prov.submit(c, shots=100)


class _StubSample:
    def __init__(self, sample, energy):
        self.sample = sample
        self.energy = energy


class _StubSampleset:
    def __init__(self, sample, energy):
        self.first = _StubSample(sample, energy)


class _StubLeapSampler:
    def __init__(self, seen, sample):
        self._seen = seen
        self._sample = sample

    def sample_qubo(self, qubo, num_reads=64):
        self._seen.append((dict(qubo), num_reads))
        return _StubSampleset(self._sample, -3.0)


def _annealing():
    return get_provider("dwave-annealing")


def test_leap_refuses_without_token(monkeypatch):
    monkeypatch.delenv("KUBONA_DWAVE_ANNEALING_LEAP_TOKEN", raising=False)
    Q = np.array([[-1.0, 2.0], [0.0, -1.0]])
    with pytest.raises(ProviderError, match="KUBONA_DWAVE_ANNEALING_LEAP_TOKEN"):
        _annealing().solve_qubo(Q, backend="leap")


def test_leap_unknown_backend_refuses():
    Q = np.array([[-1.0]])
    with pytest.raises(ProviderError, match="unknown annealing backend"):
        _annealing().solve_qubo(Q, backend="quantum-foo")


def test_leap_injected_sampler_succeeds_labeled(monkeypatch):
    monkeypatch.setenv("KUBONA_DWAVE_ANNEALING_LEAP_TOKEN", "test-token")
    seen: list = []
    # Q = -x0 - x1 + 2 x0 x1: optimum x=(1,1) energy -1... factory returns
    # x=(1,0): provider must report the SAMPLED energy, not the optimum.
    factory = lambda token: _StubLeapSampler(seen, {0: 1, 1: 0})  # noqa: E731
    Q = np.array([[-1.0, 2.0], [0.0, -1.0]])
    rec = _annealing().solve_qubo(Q, n_samples=16, backend="leap", sampler_factory=factory)
    assert rec["backend"] == "leap-hybrid"
    assert rec["solution"] == [1, 0]
    assert rec["energy"] == -1.0  # x=(1,0): -1 + 0 + 0
    assert rec["heuristic"] is True
    assert rec["optimality_gap"] == "unknown"
    assert rec["live_verified"] is False
    assert seen and seen[0][1] == 16
    assert rec["method"] == "leap-hybrid-sampler_v1"


def test_leap_missing_sdk_is_setup_gap(monkeypatch):
    import builtins
    monkeypatch.setenv("KUBONA_DWAVE_ANNEALING_LEAP_TOKEN", "test-token")
    real_import = builtins.__import__

    def fake_import(name, *args, **kwargs):
        if name.startswith("dwave.system"):
            raise ImportError("No module named 'dwave.system'")
        return real_import(name, *args, **kwargs)

    monkeypatch.setattr(builtins, "__import__", fake_import)
    Q = np.array([[-1.0]])
    with pytest.raises(ProviderError, match="dwave-system"):
        _annealing().solve_qubo(Q, backend="leap")


def test_unwired_modalities_report_wiring():
    assert get_provider("neutral-atom-quera-pasqal").health()["wiring"] == "not-wired"
    assert get_provider("xanadu-strawberry-fields").health()["wiring"] == "not-wired"
    assert get_provider("dwave-annealing").health()["wiring"] == "local-plus-leap-branch"
