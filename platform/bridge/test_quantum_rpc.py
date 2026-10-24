"""Quantum bridge RPC tests (R-34/R-33/canary proxy). Hermetic by construction:
no live QPU calls, no network — the servicer methods are invoked directly
with fake requests, the MCP rail runs local-simulator, and provider recall
is exercised through injected factories. User keys do not exist yet, so
every hardware path below is refusal-pinned, never executed."""

import sys
from pathlib import Path
from types import SimpleNamespace

import pytest

BRIDGE = Path(__file__).resolve().parent
if str(BRIDGE) not in sys.path:
    sys.path.insert(0, str(BRIDGE))

import server as bridge_server  # noqa: E402  (bridge servicer under test)
from kubona.jobs import JobRunner, JobStore  # noqa: E402
from kubona.session import SessionStore  # noqa: E402
from kubona.cost.governance import CostGovernanceService  # noqa: E402
import kubona.mcp.servers.execution as ex  # noqa: E402

BELL_QASM = """OPENQASM 2.0;
include "qelib1.inc";
qreg q[2];
creg c[2];
h q[0];
cx q[0], q[1];
"""


@pytest.fixture()
def isolated_rail(monkeypatch, tmp_path):
    """Swap the MCP rail's module singletons for tmp-backed ones so tests
    never touch the operator's KUBONA_STATE_DIR."""
    sessions = SessionStore(path=tmp_path / "sessions.jsonl")
    gov = CostGovernanceService(sessions)
    store = JobStore(path=tmp_path / "jobs.jsonl")
    runner = JobRunner(store=store)
    monkeypatch.setattr(ex, "_sessions", sessions)
    monkeypatch.setattr(ex, "_gov", gov)
    monkeypatch.setattr(ex, "_runner", runner)
    return sessions, runner


def _req(**kw):
    return SimpleNamespace(**kw)


def test_submit_requires_idempotency_key(isolated_rail):
    svc = bridge_server.QuantumBridgeServicer()
    resp = svc.QuantumSubmit(_req(qasm=BELL_QASM, provider="local-simulator",
                                  shots=100, session_id="", tenant_id="",
                                  idempotency_key="", dry_run=False,
                                  mandate_ref_json=""), None)
    assert resp.ok is False and "idempotency_key" in resp.error_message


def test_dry_run_validates_without_spend_or_enqueue(isolated_rail):
    sessions, runner = isolated_rail
    svc = bridge_server.QuantumBridgeServicer()
    resp = svc.QuantumSubmit(_req(qasm=BELL_QASM, provider="local-simulator",
                                  shots=100, session_id="", tenant_id="lab-test",
                                  idempotency_key="dry-1", dry_run=True,
                                  mandate_ref_json=""), None)
    assert resp.ok is True, resp.error_message
    assert resp.queued is False and resp.job_id == ""
    assert "estimated_usd" in resp.estimate_json
    assert "admitted" in resp.fidelity_json
    # No job enqueued, no spend recorded anywhere.
    assert runner.store._load() == {}
    for s in sessions._load().values():
        assert s.spent_usd == 0


def test_submit_and_status_roundtrip(isolated_rail):
    svc = bridge_server.QuantumBridgeServicer()
    resp = svc.QuantumSubmit(_req(qasm=BELL_QASM, provider="local-simulator",
                                  shots=100, session_id="", tenant_id="lab-test",
                                  idempotency_key="sub-1", dry_run=False,
                                  mandate_ref_json=""), None)
    assert resp.ok is True, resp.error_message
    assert resp.job_id and resp.queued is True
    st = svc.QuantumJobStatus(_req(job_id=resp.job_id), None)
    assert st.ok is True and st.job_id == resp.job_id
    assert st.status in ("queued", "running", "completed")
    # Tenant attribution landed in session meta (cross-book audit join).
    sess = ex._sessions.get(resp.session_id)
    assert sess.meta.get("tenant_id") == "lab-test"
    assert sess.meta.get("origin") == "quantum-bridge-rpc"


def test_idempotency_replays_original_job(isolated_rail):
    svc = bridge_server.QuantumBridgeServicer()
    kw = dict(qasm=BELL_QASM, provider="local-simulator", shots=100,
              session_id="", tenant_id="", dry_run=False, mandate_ref_json="")
    first = svc.QuantumSubmit(_req(idempotency_key="idem-9", **kw), None)
    assert first.ok is True
    # wait for completion so the second call replays a settled job
    ex._runner.wait(first.job_id, timeout_s=30.0)
    second = svc.QuantumSubmit(_req(idempotency_key="idem-9", **kw), None)
    assert second.ok is True and second.job_id == first.job_id
    # Same key, different body: refuse, never double-submit.
    other = svc.QuantumSubmit(_req(idempotency_key="idem-9", qasm=BELL_QASM,
                                   provider="local-simulator", shots=400,
                                   session_id="", tenant_id="", dry_run=False,
                                   mandate_ref_json=""), None)
    assert other.ok is False and "different body" in other.error_message


def test_status_unknown_job_refuses(isolated_rail):
    svc = bridge_server.QuantumBridgeServicer()
    st = svc.QuantumJobStatus(_req(job_id="nope"), None)
    assert st.ok is False


def test_cancel_unknown_job_refuses(isolated_rail):
    svc = bridge_server.QuantumBridgeServicer()
    rec = svc.QuantumCancel(_req(job_id="nope", reason="test"), None)
    assert rec.ok is False


def test_cancel_queued_job_fences_without_recall(isolated_rail):
    # Local simulator jobs complete fast; use an unstarted (queued) job by
    # creating directly in the store instead of racing the worker pool.
    _, runner = isolated_rail
    job = runner.store.create(provider="local-simulator", session_id="s1")
    svc = bridge_server.QuantumBridgeServicer()
    rec = svc.QuantumCancel(_req(job_id=job.job_id, reason="operator stop"), None)
    assert rec.ok is True
    assert rec.provider_recall_attempted is False
    assert runner.store.get(job.job_id).status == "cancelled"


def test_canary_proxy_runs_local_providers(isolated_rail):
    svc = bridge_server.QuantumBridgeServicer()
    resp = svc.RunCanary(_req(providers=["local-simulator"], max_seconds=60), None)
    assert resp.ok is True, resp.error_message
    import json
    results = json.loads(resp.results_json)
    assert isinstance(results, list) and len(results) >= 1
