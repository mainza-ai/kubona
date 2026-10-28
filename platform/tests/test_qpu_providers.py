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


@pytest.fixture(autouse=True)
def fresh_calibration():
    """Tests exercise admission mechanics; refresh the shipped fixture epochs.

    The contract's real calibrated_at dates are intentionally untouched. The
    stale-contract refusal is pinned separately below.
    """
    import dataclasses
    import time
    from kubona.fidelity import accounting
    for name in ("ibm-heron", "quantinuum-h1", "rigetti-aspen"):
        prof = accounting.DEVICE_CATALOG.get(name)
        if prof is not None:
            accounting.DEVICE_CATALOG[name] = dataclasses.replace(
                prof, last_calibrated_epoch=time.time())


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


class _StubJob:
    def __init__(self, accepted=True):
        self._accepted = accepted

    def cancel(self):
        return self._accepted


class _StubService:
    def __init__(self, seen, accepted=True):
        self._seen = seen
        self._accepted = accepted

    def job(self, job_id):
        self._seen.append(job_id)
        return _StubJob(self._accepted)


def test_ibm_cancel_recalls_via_injected_service(monkeypatch):
    from kubona.providers import qiskit_provider
    monkeypatch.setenv("KUBONA_IBM_QISKIT_TOKEN", "test-token")
    cap = load_provider("ibm-qiskit")
    prov = qiskit_provider.QiskitProvider(cap, backend_name="ibm_brisbane")
    seen: list = []
    out = prov.cancel("hw-123", service_factory=lambda tok: _StubService(seen))
    assert out["recall_requested"] is True
    assert out["cancel_accepted"] is True
    assert seen == ["hw-123"]


def test_ibm_cancel_needs_credential(monkeypatch):
    from kubona.providers import qiskit_provider
    monkeypatch.delenv("KUBONA_IBM_QISKIT_TOKEN", raising=False)
    cap = load_provider("ibm-qiskit")
    prov = qiskit_provider.QiskitProvider(cap, backend_name="ibm_brisbane")
    with pytest.raises(ProviderError, match="KUBONA_IBM_QISKIT_TOKEN"):
        prov.cancel("hw-123", service_factory=lambda tok: _StubService([]))


def test_quantinuum_cancel_recalls_via_injected_client(monkeypatch):
    from kubona.providers import tket_provider
    monkeypatch.setenv("KUBONA_QUANTINUUM_API_KEY", "test-key")
    cap = load_provider("quantinuum")
    prov = tket_provider.TketProvider(cap)
    called: list = []

    class FakeClient:
        def cancel(self, ref):
            called.append(ref)

    out = prov.cancel("nano-9", client_factory=lambda tok: FakeClient())
    assert out["recall_requested"] is True
    assert called == ["nano-9"]


def test_unwired_providers_refuse_cancel():
    from kubona.providers.base import ProviderError as PE
    for name in ("rigetti", "dwave-annealing", "neutral-atom-quera-pasqal",
                 "xanadu-strawberry-fields", "ionq", "aws-braket",
                 "local-simulator"):
        prov = get_provider(name)
        with pytest.raises(PE):
            prov.cancel("hw-xyz")


class _StubRuntimeMeas:
    def __init__(self, counts):
        self._counts = counts

    def get_counts(self):
        return self._counts


class _StubRuntimePub:
    def __init__(self, counts):
        self.data = type("Data", (), {"meas": _StubRuntimeMeas(counts)})()


class _StubRuntimeJob:
    def __init__(self, job_id="ibm-hw-42", status="RUNNING", counts=None, err=None):
        self._job_id = job_id
        self.status_val = status
        self._counts = counts or {"00": 50, "11": 50}
        self._err = err
        self.result_called = 0

    def job_id(self):
        return self._job_id

    def status(self):
        return self.status_val

    def result(self):
        self.result_called += 1
        return [_StubRuntimePub(self._counts)]

    def error_message(self):
        return self._err


class _StubRuntimeService:
    def __init__(self, job: _StubRuntimeJob):
        self._job = job

    def backend(self, name):
        return name

    def sampler(self, backend):
        job = self._job
        return type("Sampler", (), {"run": lambda self, pubs, shots=0: job})()

    def job(self, job_id):
        assert job_id == self._job._job_id
        return self._job


def test_ibm_nonblocking_submit_and_status_poll(monkeypatch):
    pytest.importorskip("qiskit")
    from kubona.circuits import parse_qasm2
    from kubona.providers.qiskit_provider import QiskitProvider

    monkeypatch.setenv("KUBONA_IBM_QISKIT_TOKEN", "test-ibm-token")
    monkeypatch.delenv("BAO_ADDR", raising=False)
    cap = load_provider("ibm-qiskit")
    prov = QiskitProvider(cap, backend_name="ibm_brisbane")
    bell = parse_qasm2("OPENQASM 2.0;\nqreg q[2];\nh q[0];\ncx q[0],q[1];")

    stub_job = _StubRuntimeJob(job_id="ibm-hw-99", status="QUEUED", counts={"00": 64, "11": 64})
    svc = _StubRuntimeService(stub_job)
    pinned: list[str] = []

    out = prov.submit(
        bell,
        shots=128,
        on_hardware_job=pinned.append,
        wait_for_result=False,
        service_factory=lambda tok: svc,
    )
    assert out["deferred"] is True
    assert out["status"] == "running"
    assert out["hardware_job_id"] == "ibm-hw-99"
    assert pinned == ["ibm-hw-99"]
    assert stub_job.result_called == 0, "wait_for_result=False must not block on job.result()"

    # First poll: still queued -> running
    p1 = prov.poll_hardware_job("ibm-hw-99", service_factory=lambda tok: svc)
    assert p1["status"] == "running" and p1["vendor_status"] == "queued"
    assert stub_job.result_called == 0

    # Vendor transitions to DONE -> poll returns completed + counts
    stub_job.status_val = "DONE"
    p2 = prov.poll_hardware_job("ibm-hw-99", service_factory=lambda tok: svc)
    assert p2["status"] == "completed"
    assert p2["result"]["counts"] == {"00": 64, "11": 64}
    assert stub_job.result_called == 1

    # Vendor ERROR -> poll returns failed + message
    stub_job.status_val = "ERROR"
    stub_job._err = "calibration drift exceeded"
    p3 = prov.poll_hardware_job("ibm-hw-99", service_factory=lambda tok: svc)
    assert p3["status"] == "failed"
    assert "calibration drift" in p3["error"]


class _StubTketResult:
    def __init__(self, counts):
        self._counts = counts

    def get_counts(self):
        return self._counts


class _StubTketBackend:
    def __init__(self, handle="h1-job-7", status="QUEUED", counts=None, msg=""):
        self._handle = handle
        self.status_val = status
        self._counts = counts or {(0, 0): 32, (1, 1): 32}
        self._msg = msg
        self.get_result_called = 0

    def compile_circuit(self, c, optimisation_level=0):
        return c

    def process_circuit(self, c, n_shots=0):
        return self._handle

    def circuit_status(self, handle):
        return type("Status", (), {"status": self.status_val, "message": self._msg})()

    def get_result(self, handle):
        self.get_result_called += 1
        return _StubTketResult(self._counts)


def test_quantinuum_nonblocking_submit_and_status_poll(monkeypatch):
    from kubona.circuits import parse_qasm2
    from kubona.providers.tket_provider import TketProvider

    monkeypatch.setenv("KUBONA_QUANTINUUM_API_KEY", "test-q-key")
    monkeypatch.delenv("BAO_ADDR", raising=False)
    cap = load_provider("quantinuum")
    prov = TketProvider(cap)
    prov._ready, prov._err = True, None
    monkeypatch.setattr(prov, "_to_tket", lambda c: c)

    backend = _StubTketBackend(handle="h1-job-77", status="SUBMITTED")
    pinned: list[str] = []
    bell = parse_qasm2("OPENQASM 2.0;\nqreg q[2];\nh q[0];\ncx q[0],q[1];")

    out = prov.submit(
        bell,
        shots=64,
        on_hardware_job=pinned.append,
        wait_for_result=False,
        backend_factory=lambda tok, dev: backend,
    )
    assert out["deferred"] is True
    assert out["status"] == "running"
    assert out["hardware_job_id"] == "h1-job-77"
    assert pinned == ["h1-job-77"]
    assert backend.get_result_called == 0

    # Poll while SUBMITTED -> running
    p1 = prov.poll_hardware_job("h1-job-77", backend_factory=lambda tok, dev: backend)
    assert p1["status"] == "running" and p1["vendor_status"] == "submitted"

    # Poll when COMPLETED -> tuple bitstring keys normalized to strings
    backend.status_val = "COMPLETED"
    p2 = prov.poll_hardware_job("h1-job-77", backend_factory=lambda tok, dev: backend)
    assert p2["status"] == "completed"
    assert p2["result"]["counts"] == {"00": 32, "11": 32}
    assert backend.get_result_called == 1


def test_execution_deferred_hardware_job_releases_worker_and_polls_to_completion(monkeypatch):
    import time
    from kubona.mcp.servers import cost as cost_server
    from kubona.mcp.servers import execution as exec_server

    bell = "OPENQASM 2.0;\nqreg q[2];\nh q[0];\ncx q[0],q[1];"
    sess = cost_server.create_session("deferred-hw", "researcher", budget_ceiling_usd=150.0)

    # Paid dispatch requires a real approval (Phase-0 gate): REVIEW -> HOLD
    # -> APPROVED with an operator-signed mandate bound to this session +
    # program. Never stubbed — this test proves the paid path end to end.
    import os
    from kubona.cost.mandate import canonical_job_binding
    gov = exec_server._gov
    req = gov.request_approval(sess["session_id"], 150.0, "deferred-hw test approval")
    gov.hold_approval(req.approval_id, reviewer="test-operator")
    approved = gov.approve(
        req.approval_id, reviewer="test-operator", note="deferred-hw test",
        tenant_id="lab-a",
        job_hash=canonical_job_binding("circuit_run", sess["session_id"], bell),
        signing_key=os.urandom(32))
    assert approved.state == "APPROVED" and approved.mandate_token

    vendor_state = {"status": "running"}

    class FakeDeferredIBMProvider:
        def submit(self, circuit, shots, seed=None, on_hardware_job=None, wait_for_result=True):
            assert wait_for_result is False, "hardware submit_job must request wait_for_result=False"
            if on_hardware_job:
                on_hardware_job("ibm-deferred-1")
            return {
                "status": "running",
                "deferred": True,
                "hardware_job_id": "ibm-deferred-1",
                "job_id": "ibm-deferred-1",
            }

        def poll_hardware_job(self, hardware_job_id):
            assert hardware_job_id == "ibm-deferred-1"
            if vendor_state["status"] == "completed":
                return {
                    "status": "completed",
                    "hardware_job_id": hardware_job_id,
                    "result": {"counts": {"00": 50, "11": 50}, "hardware": True},
                }
            return {"status": "running", "hardware_job_id": hardware_job_id}

    monkeypatch.setattr(exec_server, "get_provider", lambda name: FakeDeferredIBMProvider())

    sub = exec_server.submit_job(
        bell,
        provider="ibm-qiskit",
        shots=100,
        session_id=sess["session_id"],
        caller_jurisdictions=["US"],
        approval_id=approved.approval_id,
    )
    assert sub["ok"] is True
    job_id = sub["job_id"]

    # Wait briefly for the background worker to finish submitting and release its slot
    for _ in range(40):
        st = exec_server.get_job_status(job_id)
        if st.get("hardware_job_id") == "ibm-deferred-1":
            break
        time.sleep(0.02)
    assert st["status"] == "running"
    assert st["hardware_job_id"] == "ibm-deferred-1"

    # Flip vendor state to completed -> get_job_status polls vendor and transitions store
    vendor_state["status"] = "completed"
    st2 = exec_server.get_job_status(job_id)
    assert st2["status"] == "completed"
    res = exec_server.get_result(job_id)
    assert res["ok"] is True
    assert res["result"]["counts"] == {"00": 50, "11": 50}


def test_submit_qubo_job_and_dwave_annealing_dispatch():
    from kubona.mcp.servers import execution as exec_server

    # 1. Passing OpenQASM to dwave-annealing refuses with actionable QUBO guidance
    bad = exec_server.submit_job(
        "OPENQASM 2.0;\nqreg q[2];\nh q[0];",
        provider="dwave-annealing",
        shots=32,
    )
    assert bad["ok"] is False
    assert "QUBO" in bad["error"]

    # 2. Dry-run estimate via submit_job with JSON 2-D QUBO matrix succeeds
    est = exec_server.submit_job(
        "[[1.0, -1.0], [-1.0, 1.0]]",
        provider="dwave-annealing",
        shots=32,
        estimate_only=True,
    )
    assert est["ok"] is True and est["estimate_only"] is True
    assert est["estimate"]["estimated_usd"] == 0.0
    assert est["estimate"]["n_qubits"] == 2

    # 3. Full QUBO dispatch via submit_job resolves to optimal solution
    dispatched = exec_server.submit_job(
        "[[1.0, -1.0], [-1.0, 1.0]]",
        provider="dwave-annealing",
        shots=32,
    )
    assert dispatched["ok"] is True
    assert dispatched["status"] == "completed"
    res = exec_server.get_result(dispatched["job_id"])
    assert res["ok"] is True
    assert res["result"]["energy"] == pytest.approx(0.0)
    assert res["result"]["solution"][0] == res["result"]["solution"][1]
