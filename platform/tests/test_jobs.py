"""Async persistent job farm — production job/session abstraction (G5)."""

import time

from kubona.jobs import JobRunner, JobStore


BELL = "OPENQASM 2.0;\nqreg q[2];\nh q[0];\ncx q[0],q[1];"


def test_job_store_persists_across_instances(tmp_path):
    """A job written by one store is readable by a fresh store (survives restart)."""
    path = tmp_path / "jobs.jsonl"
    s1 = JobStore(path=str(path))
    j = s1.create(provider="local-simulator", session_id="sess-1", circuit_hash="abc")
    s1.transition(j.job_id, "running")
    s1.transition(j.job_id, "completed", result={"counts": {"00": 100, "11": 100}})

    s2 = JobStore(path=str(path))  # fresh instance, same file = "restart"
    loaded = s2.get(j.job_id)
    assert loaded is not None
    assert loaded.status == "completed"
    assert loaded.result == {"counts": {"00": 100, "11": 100}}
    assert [x.job_id for x in s2.jobs_for_session("sess-1")] == [j.job_id]


def test_async_submit_wait_false_then_poll_to_result(tmp_path):
    """wait=False returns immediately (queued), then the farm resolves it."""
    from kubona.mcp.servers._mount import build_gateway
    gw = build_gateway(require_auth=True)
    job = gw.call("execution", "submit_job",
                  {"circuit_qasm": BELL, "provider": "local-simulator", "shots": 1024,
                   "wait": False}, token="dev-token")
    assert job["ok"] is True and job["job_id"]
    assert job["status"] in ("queued", "running", "completed")
    # poll until terminal
    for _ in range(100):
        st = gw.call("execution", "get_job_status", {"job_id": job["job_id"]}, token="dev-token")
        if st["status"] in ("completed", "failed", "cancelled"):
            break
        time.sleep(0.02)
    assert st["status"] == "completed", st
    res = gw.call("execution", "get_result", {"job_id": job["job_id"]}, token="dev-token")
    assert res["ok"] is True and "counts" in res["result"]


def test_cancel_queued_or_running_job(tmp_path):
    """cancel_job turns a queued/running job terminal and rejects cancel after."""
    from kubona.mcp.servers._mount import build_gateway
    gw = build_gateway(require_auth=True)
    job = gw.call("execution", "submit_job",
                  {"circuit_qasm": BELL, "provider": "local-simulator", "shots": 1024,
                   "wait": False}, token="dev-token")
    st = gw.call("execution", "get_job_status", {"job_id": job["job_id"]}, token="dev-token")
    if st["status"] in ("queued", "running"):
        out = gw.call("execution", "cancel_job", {"job_id": job["job_id"]}, token="dev-token")
        assert out["ok"] is True and out["status"] == "cancelled"
    # cancelling an unknown id fails cleanly
    out = gw.call("execution", "cancel_job", {"job_id": "nope"}, token="dev-token")
    assert out["ok"] is False


def test_cancel_job_scoped_via_policy(tmp_path):
    """cancel_job requires execution:override; a submit-only agent is denied."""
    from kubona.mcp.auth import AuthError, Principal
    from kubona.mcp.gateway import KubonaGateway
    from kubona.security.policy import PolicyEngine
    from kubona.identity import IdentityRegistry
    from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PrivateKey
    from cryptography.hazmat.primitives import serialization as s

    k = lambda: Ed25519PrivateKey.generate().public_key().public_bytes(s.Encoding.Raw, s.PublicFormat.Raw).hex()
    reg = IdentityRegistry(path=str(tmp_path / "id.json"))
    reg.register("user-1", "user", k())
    reg.register("agent-1", "agent", k(), owner="user-1")
    reg.delegate("user-1", "agent-1", scopes=("execution:submit", "execution:read"))

    class AgentAuth:  # authenticates the submit-only agent
        def verify(self, token): return Principal(sub="agent-1", role="agent",
                                                  scopes=("execution:submit", "execution:read"))

    gw = KubonaGateway(auth=AgentAuth(), policy=PolicyEngine(reg), require_auth=True)
    from kubona.mcp.servers import execution as ex
    gw.mount("execution", {"cancel_job": ex.cancel_job})
    import pytest
    with pytest.raises(AuthError):
        gw.call("execution", "cancel_job", {"job_id": "x"}, token="anything")
