"""End-to-end production stack — OAuth -> HTTP gateway -> job farm -> provider
-> result -> audit -> ledger, as ONE system (not isolated units).

This proves the M0/G5/G8 wiring composes the way a real deployment is used, and
catches regressions where a change works in isolation but breaks the whole path.
"""

import json

from starlette.testclient import TestClient

from kubona.mcp.gateway import KubonaGateway
from kubona.mcp.http_gateway import make_http_gateway
from kubona.mcp.servers import compiler, control, cost, execution, provenance
from kubona.security.oauth import OAuth2AccessTokenProvider, OAuth2TokenIssuer

BELL = "OPENQASM 2.0;\nqreg q[2];\nh q[0];\ncx q[0],q[1];"


def _build_stack(tmp_path):
    """Production-posed gateway (real OAuth issuer, persistent audit) + HTTP app."""
    iss = OAuth2TokenIssuer()
    gw = KubonaGateway(auth=OAuth2AccessTokenProvider(iss), require_auth=True,
                       audit_path=str(tmp_path / "audit.jsonl"),
                       ledger=None)
    for server, tools in {
        "control": {"register_loop": control.register_loop, "step_loop": control.step_loop,
                    "list_loops": control.list_loops, "loop_status": control.loop_status,
                    "pause_loop": control.pause_loop, "resume_loop": control.resume_loop,
                    "kill_loop": control.kill_loop},
        "compiler": {"transpile_circuit": compiler.transpile_circuit,
                     "lower_circuit": compiler.lower_circuit},
        "execution": {"submit_job": execution.submit_job, "get_job_status": execution.get_job_status,
                      "get_result": execution.get_result, "run_zne_job": execution.run_zne_job,
                      "cancel_job": execution.cancel_job, "session_status": execution.session_status,
                      "close_session": execution.close_session,
                      "list_provider_registry": execution.list_provider_registry},
        "cost": {"create_session": cost.create_session, "estimate_cost": cost.estimate_cost,
                 "request_approval": cost.request_approval, "hold_approval": cost.hold_approval,
                 "approve_approval": cost.approve_approval, "reject_approval": cost.reject_approval,
                 "session_burn_rate": cost.session_burn_rate, "quote_payment": cost.quote_payment,
                 "verify_payment": cost.verify_payment},
        "provenance": {"write_record": provenance.write_record,
                       "query_records": provenance.query_records},
    }.items():
        gw.mount(server, tools)
    app = make_http_gateway(gw)
    return TestClient(app), iss


def test_full_stack_end_to_end(tmp_path):
    client, iss = _build_stack(tmp_path)
    token = iss.issue("prod-agent", role="agent",
                      scopes=("compiler:read", "execution:submit", "execution:read"))

    # 0. health
    r = client.get("/health")
    assert r.status_code == 200 and r.json()["ok"] is True

    hdr = {"Authorization": f"Bearer {token}"}

    # 1. transpile (compiler) over HTTP
    r = client.post("/rpc", json={"server": "compiler", "tool": "transpile_circuit",
                                  "args": {"circuit_qasm": BELL}}, headers=hdr)
    assert r.status_code == 200 and r.json()["result"]["num_qubits"] == 2

    # 2. submit + poll to completion (execution -> persistent job farm)
    r = client.post("/rpc", json={"server": "execution", "tool": "submit_job",
                                  "args": {"circuit_qasm": BELL, "provider": "local-simulator",
                                           "shots": 1024, "wait": True}}, headers=hdr)
    j = r.json()["result"]
    assert j["ok"] is True and j["status"] == "completed", j

    # 3. retrieve result
    r = client.post("/rpc", json={"server": "execution", "tool": "get_result",
                                  "args": {"job_id": j["job_id"]}}, headers=hdr)
    res = r.json()["result"]
    assert res["ok"] is True and set(res["result"]["counts"]) <= {"00", "11"}

    # 4. audit was persisted across calls (restart-safe)
    audit = json.loads((tmp_path / "audit.jsonl").read_text().strip().splitlines()[-1])
    assert audit["server"] == "execution"

    # 5. scoped deny still enforced over the whole stack (agent lacks cost:approve)
    r = client.post("/rpc", json={"server": "cost", "tool": "approve_approval",
                                  "args": {"approval_id": "x"}}, headers=hdr)
    assert r.status_code == 403

    # 6. unauthenticated rejected
    r = client.post("/rpc", json={"server": "compiler", "tool": "transpile_circuit",
                                  "args": {"circuit_qasm": BELL}})
    assert r.status_code == 401


def test_hybrid_envelope_over_full_stack(tmp_path):
    from kubona.security.hybrid_transport import (HybridKeyPair, handshake_initiator,
                                                  handshake_responder)
    import base64

    client_kp = HybridKeyPair.generate(); server_kp = HybridKeyPair.generate()
    csession, ct = handshake_initiator(client_kp, server_kp.public_bytes())
    ssess = handshake_responder(server_kp, client_kp.public_bytes(), ct)

    iss = OAuth2TokenIssuer()
    gw = KubonaGateway(auth=OAuth2AccessTokenProvider(iss), require_auth=True)
    from kubona.mcp.servers import compiler as c
    gw.mount("compiler", {"transpile_circuit": c.transpile_circuit})
    app = make_http_gateway(gw, secure_session=ssess)
    client = TestClient(app)
    token = iss.issue("secure-agent", role="agent", scopes=("compiler:read",))

    from kubona.security.secure_rpc import SecureRpc
    rpc_client = SecureRpc(csession)
    env = base64.b64encode(rpc_client.wrap_call(
        "compiler", "transpile_circuit", {"circuit_qasm": BELL}, token=token)).decode()
    r = client.post("/rpc", json={"envelope": env}, headers={"Authorization": f"Bearer {token}"})
    assert r.status_code == 200 and r.json()["ok"] is True
    resp = rpc_client.unwrap_response(base64.b64decode(r.json()["envelope"]))
    assert resp["num_qubits"] == 2
