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
    from kubona.mcp.servers import cost
    monkeypatch.setattr(cost, "_sessions", sessions)
    monkeypatch.setattr(cost, "_gov", gov)
    return sessions, runner


@pytest.fixture(autouse=True)
def fresh_calibration():
    import dataclasses
    import time
    from kubona.fidelity import accounting
    for name in ("ibm-heron", "quantinuum-h1", "rigetti-aspen"):
        prof = accounting.DEVICE_CATALOG.get(name)
        if prof is not None:
            accounting.DEVICE_CATALOG[name] = dataclasses.replace(
                prof, last_calibrated_epoch=time.time())



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


def test_submit_syncs_external_session_and_sets_replay_flag(isolated_rail):
    sessions, _ = isolated_rail
    svc = bridge_server.QuantumBridgeServicer()
    kw = dict(
        qasm=BELL_QASM,
        provider="local-simulator",
        shots=100,
        session_id="go-sqlite-sess-1",
        tenant_id="lab-alpha",
        dry_run=False,
        mandate_ref_json="",
        caller_jurisdictions=["US", "EU"],
        caller_export_class="allied",
        require_review=False,
        session_verified=True,
        session_budget_ceiling_usd=42.5,
        session_spent_usd=12.0,
    )
    first = svc.QuantumSubmit(_req(idempotency_key="sync-idem-1", **kw), None)
    assert first.ok is True, first.error_message
    assert first.idempotent_replay is False
    mirrored = sessions.get("go-sqlite-sess-1")
    assert mirrored is not None
    assert mirrored.budget_ceiling_usd == 42.5
    assert mirrored.meta.get("tenant_id") == "lab-alpha"
    # Go already reserved this job; Python must not charge the mirror.
    assert mirrored.meta.get("spend_prepaid") is True

    ex._runner.wait(first.job_id, timeout_s=30.0)
    second = svc.QuantumSubmit(_req(idempotency_key="sync-idem-1", **kw), None)
    assert second.ok is True
    assert second.job_id == first.job_id
    assert second.idempotent_replay is True

    st = svc.QuantumJobStatus(_req(job_id=first.job_id), None)
    assert st.ok is True
    assert st.session_id == "go-sqlite-sess-1"


def test_quantinuum_provider_maps_to_catalog_fidelity_device():
    from kubona.fidelity.accounting import DEVICE_CATALOG, FidelityAccountingEngine
    gate = FidelityAccountingEngine()
    for prov in ("quantinuum", "quantinuum-tket"):
        dev = ex._FIDELITY_DEVICE_FOR_PROVIDER.get(prov)
        assert dev == "quantinuum-h1"
        assert dev in DEVICE_CATALOG
        verdict = gate.evaluate_admission(
            device_name=dev,
            n_qubits=2,
            n_single_qubit_gates=1,
            n_two_qubit_gates=1,
            fidelity_floor=0.5,
        )
        assert verdict.admitted is True
        assert verdict.predicted_fidelity < 1.0  # real trapped-ion error model, not ideal 1.0 fallback


def test_workflow_plan_dry_run():
    import json
    svc = bridge_server.QuantumBridgeServicer()
    wf_payload = {
        "name": "dry-plan-test",
        "trigger": "manual",
        "steps": [
            {
                "id": "step1",
                "tool": "compiler/transpile_circuit",
                "args": {"qasm": BELL_QASM, "target_basis": "{{input.basis}}"}
            }
        ],
        "on_failure": [
            {"id": "comp1", "tool": "provenance/write_record", "args": {"msg": "failed"}}
        ]
    }
    req = _req(
        workflow_json=json.dumps(wf_payload),
        session_id="test-plan-sess",
        inputs_json=json.dumps({"basis": "ibm"}),
    )
    resp = svc.WorkflowPlan(req, None)
    assert resp.ok is True, resp.error_message
    assert resp.workflow_id
    steps = json.loads(resp.planned_steps_json)
    assert len(steps) == 1
    assert steps[0]["args"]["target_basis"] == "ibm"
    comps = json.loads(resp.compensations_planned_json)
    assert len(comps) == 1


def test_workflow_run_success(isolated_rail):
    import json
    sessions, _ = isolated_rail
    svc = bridge_server.QuantumBridgeServicer()
    wf_payload = {
        "name": "run-test",
        "trigger": "manual",
        "budget_ceiling_usd": 10.0,
        "steps": [
            {
                "id": "estimate",
                "tool": "cost/estimate_cost",
                "args": {"circuit_qasm": BELL_QASM, "provider": "local-simulator", "shots": 100}
            },
            {
                "id": "write_audit",
                "tool": "provenance/write_record",
                "args": {"kind": "workflow_result", "payload": {"status": "ok"}}
            }
        ]
    }
    req = _req(
        workflow_json=json.dumps(wf_payload),
        session_id="wf-exec-sess-1",
        token="test-token",
        inputs_json="{}",
        force=True,
        session_verified=True,
        session_budget_ceiling_usd=25.0,
        session_spent_usd=2.0,
        tenant_id="tenant-alpha",
    )
    resp = svc.WorkflowRun(req, None)
    assert resp.ok is True, resp.error_message
    assert resp.steps_completed == 2
    assert len(resp.step_records) >= 1
    # Check session was mirrored in Python SessionStore
    mirrored = sessions.get("wf-exec-sess-1")
    assert mirrored is not None
    assert mirrored.budget_ceiling_usd == 25.0


def test_workflow_run_ceiling_refusal(isolated_rail):
    import json
    svc = bridge_server.QuantumBridgeServicer()
    wf_payload = {
        "name": "over-ceiling-test",
        "trigger": "manual",
        "budget_ceiling_usd": 0.50,
        "steps": [
            {
                "id": "expensive_step",
                "tool": "cost/estimate_cost",
                "cost_usd": 1.00,  # declared cost exceeds 0.50 ceiling
                "args": {"circuit_qasm": BELL_QASM, "provider": "local-simulator", "shots": 100}
            }
        ],
        "on_failure": [
            {
                "id": "comp_step",
                "tool": "provenance/write_record",
                "args": {"kind": "workflow_result", "payload": {"reason": "breach"}}
            }
        ]
    }
    req = _req(
        workflow_json=json.dumps(wf_payload),
        session_id="wf-fail-sess",
        token="test-token",
        inputs_json="{}",
        force=True,
        session_verified=False,
        session_budget_ceiling_usd=0.0,
        session_spent_usd=0.0,
        tenant_id="tenant-alpha",
    )
    resp = svc.WorkflowRun(req, None)
    assert resp.ok is False
    assert "breach ceiling" in resp.error_message
    comps = json.loads(resp.compensations_json)
    assert len(comps) == 1
    assert comps[0]["id"] == "comp_step"
    assert comps[0]["ok"] is True


def test_bridge_unix_domain_socket_serving():
    import os
    import stat
    sock_path = f"/tmp/kubona_test_{os.getpid()}.sock"
    if os.path.exists(sock_path):
        os.unlink(sock_path)
    server = bridge_server.serve(port=0, socket_path=sock_path)
    try:
        assert os.path.exists(sock_path)
        mode = stat.S_IMODE(os.stat(sock_path).st_mode)
        assert mode == 0o600
    finally:
        server.stop(0)
        if os.path.exists(sock_path):
            os.unlink(sock_path)






# ------------------------------------------------- bridge auth (2026-10-26) ---

def test_non_loopback_bridge_without_token_refuses(monkeypatch):
    monkeypatch.delenv("KUBONA_BRIDGE_TOKEN", raising=False)
    monkeypatch.delenv("KUBONA_BRIDGE_SOCKET", raising=False)
    with pytest.raises(SystemExit):
        bridge_server.serve(port=0, bind="0.0.0.0")


def test_bridge_token_auth_is_constant_time_and_narrow():
    from kubona.core.auth import AuthError
    auth = bridge_server._BridgeTokenAuth("sekret")
    principal = auth.verify("sekret")
    assert principal.sub == "kubonad-bridge"
    assert principal.role == "operator"
    with pytest.raises(AuthError):
        auth.verify("wrong")
    with pytest.raises(AuthError):
        auth.verify("")


def test_execute_circuit_refuses_hardware_route():
    svc = bridge_server.QuantumBridgeServicer()
    req = SimpleNamespace(
        job_id="hw-attempt", qasm=BELL_QASM, shots=10,
        requested_backend="ibm_brisbane", allow_classical_fallback=True,
        caller_jurisdictions=["US"], caller_export_class="any",
        export_reviewed=True,
    )
    resp = svc.ExecuteCircuit(req, None)
    assert resp.success is False
    assert "QuantumSubmit" in resp.error_message
    assert not resp.hardware_job_id


def test_workflow_run_requires_token_in_production(monkeypatch):
    import json
    monkeypatch.setenv("KUBONA_ENV", "production")
    monkeypatch.delenv("KUBONA_BRIDGE_TOKEN", raising=False)
    svc = bridge_server.QuantumBridgeServicer()
    wf = {
        "name": "auth-test", "trigger": "manual",
        "steps": [{"id": "estimate", "tool": "cost/estimate_cost",
                   "args": {"circuit_qasm": BELL_QASM,
                            "provider": "local-simulator", "shots": 10}}],
    }
    req = SimpleNamespace(workflow_json=json.dumps(wf), inputs_json="",
                          session_id="", token="", force=False,
                          session_verified=False)
    resp = svc.WorkflowRun(req, None)
    assert resp.ok is False
    assert "token" in resp.error_message.lower()


# ------------------------------------- session assertion (2026-10-26) ---

def test_session_assertion_vector_matches_go():
    req = SimpleNamespace(session_id="s1", session_budget_ceiling_usd=25.0,
                          session_spent_usd=0.25, tenant_id="lab",
                          session_verified=True)
    got = bridge_server._session_assertion_mac("k", req)
    want = "4e0b142452c9b1d33d7d916a409ef2434b24a97d1c6a9d4acdc842178530c7ea"
    assert got == want


def test_session_assertion_rejects_self_declared_money():
    class Ctx:
        def __init__(self, md):
            self._md = md
        def invocation_metadata(self):
            return self._md
        def abort(self, code, msg):
            raise RuntimeError(msg)

    req = SimpleNamespace(session_id="s1", session_budget_ceiling_usd=999999.0,
                          session_spent_usd=-5.0, tenant_id="evil",
                          session_verified=True)
    # Signed for the honest values, replayed against a tampered request.
    honest = SimpleNamespace(session_id="s1", session_budget_ceiling_usd=25.0,
                             session_spent_usd=0.25, tenant_id="lab",
                             session_verified=True)
    sig = bridge_server._session_assertion_mac("k", honest)
    with pytest.raises(RuntimeError):
        bridge_server._verify_session_assertion(req, Ctx((("x-kubona-session-assertion", sig),)), "k")
    # Correct signature passes.
    bridge_server._verify_session_assertion(honest, Ctx((("x-kubona-session-assertion", sig),)), "k")
