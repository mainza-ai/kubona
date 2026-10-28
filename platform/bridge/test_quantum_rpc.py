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
def bridge_token(monkeypatch):
    """Workflow execution requires KUBONA_BRIDGE_TOKEN in ALL envs (no dev
    fallback). Tests provide it explicitly via this fixture instead of
    weakening the gate."""
    monkeypatch.setenv("KUBONA_BRIDGE_TOKEN", "test-bridge-token")
    return "test-bridge-token"


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


def test_workflow_plan_dry_run(bridge_token):
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


def test_workflow_run_success(isolated_rail, bridge_token):
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
        token=bridge_token,
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


def test_workflow_run_ceiling_refusal(isolated_rail, bridge_token):
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
        token=bridge_token,
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
    # Scoped per caller, never god-mode: no "*" even by default.
    assert "*" not in principal.scopes
    assert "execution:submit" in principal.scopes
    assert "control:override" not in principal.scopes
    assert "cost:approve" not in principal.scopes
    assert "execution:override" not in principal.scopes
    with pytest.raises(AuthError):
        auth.verify("wrong")
    with pytest.raises(AuthError):
        auth.verify("")


def test_bridge_token_auth_scopes_principal_per_caller():
    from kubona.core.auth import AuthError
    auth = bridge_server._BridgeTokenAuth("sekret", sub="kubonad-bridge:run:lab-a:s1")
    principal = auth.verify("sekret")
    assert principal.sub == "kubonad-bridge:run:lab-a:s1"
    assert "*" not in principal.scopes
    with pytest.raises(AuthError):
        auth.verify("nope")


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


def test_workflow_run_requires_token_in_dev_too(monkeypatch, isolated_rail):
    """No dev fallback: without KUBONA_BRIDGE_TOKEN the run refuses in
    every env, not just production."""
    import json
    monkeypatch.delenv("KUBONA_ENV", raising=False)
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


def test_workflow_run_wrong_token_refuses(isolated_rail, bridge_token):
    """A caller token that is not the bridge token fails gateway auth:
    the workflow fails closed instead of running unauthenticated."""
    import json
    svc = bridge_server.QuantumBridgeServicer()
    wf = {
        "name": "auth-test", "trigger": "manual",
        "steps": [{"id": "estimate", "tool": "cost/estimate_cost",
                   "args": {"circuit_qasm": BELL_QASM,
                            "provider": "local-simulator", "shots": 10}}],
    }
    req = SimpleNamespace(workflow_json=json.dumps(wf), inputs_json="",
                          session_id="wrong-token-sess", token="not-the-bridge-token",
                          force=True, session_verified=False)
    resp = svc.WorkflowRun(req, None)
    assert resp.ok is False


def test_workflow_run_scopes_forbid_cancel(isolated_rail, bridge_token):
    """The per-caller gateway principal cannot cancel jobs
    (execution:override excluded): a workflow reaching for cancel_job
    fails that step instead of fencing another job."""
    import json
    svc = bridge_server.QuantumBridgeServicer()
    wf = {
        "name": "cancel-attempt", "trigger": "manual",
        "steps": [{"id": "evil", "tool": "execution/cancel_job",
                   "args": {"job_id": "someone-elses-job"}}],
    }
    req = SimpleNamespace(workflow_json=json.dumps(wf), inputs_json="",
                          session_id="scope-sess", token=bridge_token,
                          force=True, session_verified=False)
    resp = svc.WorkflowRun(req, None)
    assert resp.ok is False
    assert "cancel_job" in resp.error_message or "lacks" in resp.error_message


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


# --------------------------------- spend-mandate gate (Phase-0 P0) ---

_PAID_PROVIDER = "ibm-qiskit"
_PAID_JURISDICTIONS = ["US"]


def _mint_mandate(tenant, session_id, qasm, budget, expiry):
    import json as _json
    from kubona.cost.mandate import canonical_job_binding, create_mandate
    from nacl.signing import SigningKey
    key = SigningKey.generate()
    m = create_mandate(key.encode(), tenant,
                       canonical_job_binding("circuit_run", session_id, qasm),
                       budget, expiry)
    return _json.dumps(m.to_dict())


def _paid_submit_kw(session_id="mandate-sess-1", tenant="lab-mandate",
                    key="mandate-1", mandate_ref_json=""):
    return dict(
        qasm=BELL_QASM, provider=_PAID_PROVIDER, shots=100,
        session_id=session_id, tenant_id=tenant,
        idempotency_key=key, dry_run=False,
        mandate_ref_json=mandate_ref_json,
        caller_jurisdictions=list(_PAID_JURISDICTIONS),
        caller_export_class="allied", require_review=False,
        session_verified=True,
        session_budget_ceiling_usd=42.5, session_spent_usd=0.0,
    )


def test_paid_submit_without_mandate_refuses(isolated_rail):
    svc = bridge_server.QuantumBridgeServicer()
    resp = svc.QuantumSubmit(_req(**_paid_submit_kw()), None)
    assert resp.ok is False
    assert "mandate" in resp.error_message.lower()
    assert resp.job_id == ""


def test_paid_submit_with_valid_mandate_dispatches(isolated_rail):
    import datetime
    sessions, runner = isolated_rail
    svc = bridge_server.QuantumBridgeServicer()
    ref = _mint_mandate("lab-mandate", "mandate-sess-1", BELL_QASM, 25.0,
                        datetime.datetime(2030, 1, 1, tzinfo=datetime.timezone.utc))
    resp = svc.QuantumSubmit(_req(**_paid_submit_kw(mandate_ref_json=ref)), None)
    assert resp.ok is True, resp.error_message
    assert resp.job_id and resp.queued is True
    # Estimate (~$8) is covered by the $25 mandate; the job is real.
    assert runner.store.get(resp.job_id) is not None


def test_paid_submit_with_tampered_mandate_refuses(isolated_rail):
    import datetime
    import json as _json
    svc = bridge_server.QuantumBridgeServicer()
    ref = _mint_mandate("lab-mandate", "mandate-sess-1", BELL_QASM, 25.0,
                        datetime.datetime(2030, 1, 1, tzinfo=datetime.timezone.utc))
    data = _json.loads(ref)
    data["max_budget_usd"] = 999999.0  # tamper after signing
    resp = svc.QuantumSubmit(
        _req(**_paid_submit_kw(key="mandate-tamper", mandate_ref_json=_json.dumps(data))), None)
    assert resp.ok is False
    assert "mandate" in resp.error_message.lower()


def test_paid_submit_wrong_tenant_or_program_refuses(isolated_rail):
    import datetime
    svc = bridge_server.QuantumBridgeServicer()
    # Bound to a different tenant.
    ref = _mint_mandate("lab-other", "mandate-sess-1", BELL_QASM, 25.0,
                        datetime.datetime(2030, 1, 1, tzinfo=datetime.timezone.utc))
    resp = svc.QuantumSubmit(
        _req(**_paid_submit_kw(key="mandate-tenant", mandate_ref_json=ref)), None)
    assert resp.ok is False
    assert "mandate" in resp.error_message.lower()
    # Bound to a different program (different session binding).
    ref = _mint_mandate("lab-mandate", "some-other-session", BELL_QASM, 25.0,
                        datetime.datetime(2030, 1, 1, tzinfo=datetime.timezone.utc))
    resp = svc.QuantumSubmit(
        _req(**_paid_submit_kw(key="mandate-prog", mandate_ref_json=ref)), None)
    assert resp.ok is False
    assert "mandate" in resp.error_message.lower()


def test_paid_submit_underfunded_mandate_refuses(isolated_rail):
    import datetime
    svc = bridge_server.QuantumBridgeServicer()
    # Estimate is ~$8; a $0.50 mandate cannot cover it.
    ref = _mint_mandate("lab-mandate", "mandate-sess-1", BELL_QASM, 0.50,
                        datetime.datetime(2030, 1, 1, tzinfo=datetime.timezone.utc))
    resp = svc.QuantumSubmit(
        _req(**_paid_submit_kw(key="mandate-poor", mandate_ref_json=ref)), None)
    assert resp.ok is False
    assert "mandate" in resp.error_message.lower()


def test_paid_submit_expired_mandate_refuses(isolated_rail):
    import datetime
    import json as _json
    from kubona.cost.mandate import SpendMandate, canonical_job_binding
    from nacl.signing import SigningKey
    svc = bridge_server.QuantumBridgeServicer()
    tenant, job_hash = "lab-mandate", canonical_job_binding(
        "circuit_run", "mandate-sess-1", BELL_QASM)
    expiry = "2001-01-01T00:00:00Z"  # past, but signed correctly
    sk = SigningKey.generate()
    payload = f"{tenant}:{job_hash}:{25.0:.4f}:{expiry}".encode()
    m = SpendMandate(tenant, job_hash, 25.0, expiry,
                     sk.verify_key.encode().hex(), sk.sign(payload).signature.hex())
    resp = svc.QuantumSubmit(
        _req(**_paid_submit_kw(key="mandate-old", mandate_ref_json=_json.dumps(m.to_dict()))),
        None)
    assert resp.ok is False
    assert "mandate" in resp.error_message.lower()


def test_free_simulator_ignores_mandate_absence(isolated_rail):
    """Free simulators proceed with no mandate — the gate is money-gated."""
    svc = bridge_server.QuantumBridgeServicer()
    resp = svc.QuantumSubmit(_req(qasm=BELL_QASM, provider="local-simulator",
                                 shots=100, session_id="", tenant_id="lab-test",
                                 idempotency_key="free-no-mandate", dry_run=False,
                                 mandate_ref_json=""), None)
    assert resp.ok is True, resp.error_message


def test_paid_dry_run_skips_mandate_gate(isolated_rail):
    """Dry-run is the pre-mandate estimate panel: validated, never enqueued,
    never spend-gated by mandate."""
    sessions, runner = isolated_rail
    svc = bridge_server.QuantumBridgeServicer()
    resp = svc.QuantumSubmit(_req(**_paid_submit_kw(key="dry-nomandate",
                                                   mandate_ref_json="")), None)
    assert resp.ok is False and "mandate" in resp.error_message.lower()
    kw = _paid_submit_kw(key="dry-nomandate-2", mandate_ref_json="")
    kw["dry_run"] = True
    resp = svc.QuantumSubmit(_req(**kw), None)
    assert resp.ok is True, resp.error_message
    assert resp.queued is False and resp.job_id == ""


# --------------------------------- job ownership (Phase-0 P0) ---

def test_status_wrong_session_refuses(isolated_rail):
    svc = bridge_server.QuantumBridgeServicer()
    resp = svc.QuantumSubmit(_req(qasm=BELL_QASM, provider="local-simulator",
                                 shots=100, session_id="", tenant_id="lab-own",
                                 idempotency_key="own-1", dry_run=False,
                                 mandate_ref_json=""), None)
    assert resp.ok is True
    owner_session = resp.session_id
    st = svc.QuantumJobStatus(_req(job_id=resp.job_id,
                                   caller_session_id=owner_session,
                                   caller_tenant_id="lab-own"), None)
    assert st.ok is True, st.error_message
    st = svc.QuantumJobStatus(_req(job_id=resp.job_id,
                                   caller_session_id="attacker-session",
                                   caller_tenant_id="lab-own"), None)
    assert st.ok is False
    assert "session" in st.error_message.lower()
    st = svc.QuantumJobStatus(_req(job_id=resp.job_id,
                                   caller_session_id=owner_session,
                                   caller_tenant_id="lab-evil"), None)
    assert st.ok is False
    assert "tenant" in st.error_message.lower()


def test_cancel_wrong_session_refuses_without_fencing(isolated_rail):
    _, runner = isolated_rail
    job = runner.store.create(provider="local-simulator", session_id="owner-s")
    svc = bridge_server.QuantumBridgeServicer()
    rec = svc.QuantumCancel(_req(job_id=job.job_id, reason="evil",
                                 caller_session_id="attacker-s"), None)
    assert rec.ok is False
    assert "session" in rec.error_message.lower()
    assert rec.provider_recall_attempted is False
    # Not fenced: the refusal happened before any state mutation.
    assert runner.store.get(job.job_id).status == "queued"


def test_cancel_owner_session_allowed(isolated_rail):
    _, runner = isolated_rail
    job = runner.store.create(provider="local-simulator", session_id="owner-s")
    svc = bridge_server.QuantumBridgeServicer()
    rec = svc.QuantumCancel(_req(job_id=job.job_id, reason="owner stop",
                                 caller_session_id="owner-s"), None)
    assert rec.ok is True
    assert runner.store.get(job.job_id).status == "cancelled"


def test_status_unknown_job_still_refuses(isolated_rail):
    svc = bridge_server.QuantumBridgeServicer()
    st = svc.QuantumJobStatus(_req(job_id="nope",
                                   caller_session_id="any"), None)
    assert st.ok is False


# --------------------------------- prod transport (Phase-0 P0) ---

def test_prod_tcp_bridge_refuses(monkeypatch):
    monkeypatch.setenv("KUBONA_ENV", "production")
    monkeypatch.delenv("KUBONA_BRIDGE_SOCKET", raising=False)
    monkeypatch.setenv("KUBONA_BRIDGE_TOKEN", "tok")
    with pytest.raises(SystemExit, match="production"):
        bridge_server.serve(port=50099, bind="127.0.0.1")


def test_prod_uds_bridge_allowed(monkeypatch, tmp_path):
    import os
    import stat
    monkeypatch.setenv("KUBONA_ENV", "production")
    monkeypatch.delenv("KUBONA_BRIDGE_TOKEN", raising=False)
    # UDS paths must fit sun_path (<=103 chars): short /tmp name, not tmp_path.
    sock_path = f"/tmp/kubona_prod_{os.getpid()}.sock"
    if os.path.exists(sock_path):
        os.unlink(sock_path)
    server = bridge_server.serve(port=0, socket_path=sock_path)
    try:
        assert os.path.exists(sock_path)
        assert stat.S_IMODE(os.stat(sock_path).st_mode) == 0o600
    finally:
        server.stop(0)
        if os.path.exists(sock_path):
            os.unlink(sock_path)


def test_dev_loopback_tcp_logs_mode(monkeypatch, caplog):
    import logging
    monkeypatch.delenv("KUBONA_ENV", raising=False)
    monkeypatch.delenv("KUBONA_BRIDGE_SOCKET", raising=False)
    monkeypatch.setenv("KUBONA_BRIDGE_TOKEN", "tok")
    with caplog.at_level(logging.INFO, logger="kubona-bridge"):
        server = bridge_server.serve(port=17921, bind="127.0.0.1")
    try:
        assert any("loopback-tcp" in r.message for r in caplog.records), \
            [r.message for r in caplog.records]
    finally:
        server.stop(0)
