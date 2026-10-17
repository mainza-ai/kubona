"""Async persistent job farm — production job/session abstraction (G5)."""

import threading
import time

import pytest

from kubona.jobs import (
    InvalidTransitionError,
    JobRunner,
    JobStore,
    RunnerBusyError,
)


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


def test_cancel_queued_or_running_job():
    """Cancel a queued/running job deterministically (slow run + JobRunner)."""
    import threading
    import time

    runner = JobRunner()  # default persistent store
    release = threading.Event()
    start = threading.Event()

    def slow_run():
        start.set()
        release.wait(timeout=10)  # hold the job in 'running' so it is cancelable
        return {"counts": {"00": 1}}

    job = runner.store.create(provider="local-simulator", session_id="sess-cancel")
    runner.enqueue(job.job_id, slow_run)
    assert start.wait(timeout=5)  # the worker started -> job is running
    # now cancel the running job
    assert runner.cancel(job.job_id) is True
    assert runner.store.get(job.job_id).status == "cancelled"

    # cancelling an unknown id fails cleanly via the gateway
    from kubona.mcp.servers._mount import build_gateway
    gw = build_gateway(require_auth=True)
    out = gw.call("execution", "cancel_job", {"job_id": "nope"}, token="dev-token")
    assert out["ok"] is False

    release.set()  # let the worker finish (it will not overwrite 'cancelled' terminal)
    assert release.wait(timeout=5)
    time.sleep(0.2)  # worker observed the fence and discarded its result
    assert runner.store.get(job.job_id).status == "cancelled", \
        "cancelled status must survive the worker finishing (no completed-over-cancelled)"


def test_transition_graph_terminal_immutable(tmp_path):
    """W0.11: any->any is gone — terminal states never resurrect, unknown
    fields never write, ghosts return None."""
    s = JobStore(path=str(tmp_path / "g.jsonl"))
    j = s.create(provider="local-simulator", session_id="s")
    assert s.transition("ghost", "running") is None
    s.transition(j.job_id, "running")
    s.transition(j.job_id, "completed", result={"ok": True})
    for bad in ("queued", "running", "failed", "cancelled"):
        with pytest.raises(InvalidTransitionError):
            s.transition(j.job_id, bad)
    with pytest.raises(InvalidTransitionError):
        s.transition(j.job_id, "completed", meta={"injected": True})
    assert s.get(j.job_id).status == "completed"
    assert s.get(j.job_id).job_id == j.job_id


def test_enqueue_ghost_refuses_before_ledger(tmp_path):
    """W0.11: enqueueing an unknown id refuses with no ledger record."""
    seen = []
    runner = JobRunner(store=JobStore(path=str(tmp_path / "g.jsonl")),
                       ledger=type("L", (), {"append": lambda self, k, p: seen.append(k)})())
    with pytest.raises(KeyError):
        runner.enqueue("ghost", lambda: {"ok": True})
    assert seen == [], f"refused enqueue must leave no ledger trace, got {seen}"


def test_wait_tristate(tmp_path):
    """W0.11: unknown raises KeyError, timeout raises TimeoutError naming the
    status, terminal returns the job."""
    runner = JobRunner(store=JobStore(path=str(tmp_path / "g.jsonl")))
    with pytest.raises(KeyError):
        runner.wait("ghost", timeout_s=0.1)
    gate = threading.Event()
    job = runner.store.create(provider="x", session_id="s")
    runner.enqueue(job.job_id, lambda: (gate.wait(timeout=5), {"ok": True})[1])
    with pytest.raises(TimeoutError, match="running|queued"):
        runner.wait(job.job_id, timeout_s=0.2, poll_s=0.02)
    gate.set()
    done = runner.wait(job.job_id, timeout_s=5.0)
    assert done.status == "completed"


def test_farm_backpressure_explicit(tmp_path):
    """W0.11: a full farm refuses explicitly instead of silently queueing."""
    runner = JobRunner(store=JobStore(path=str(tmp_path / "g.jsonl")), max_workers=1)
    gate = threading.Event()
    j1 = runner.store.create(provider="x", session_id="s")
    runner.enqueue(j1.job_id, lambda: (gate.wait(timeout=5), {"ok": True})[1])
    assert runner.store.get(j1.job_id).status in ("queued", "running")
    j2 = runner.store.create(provider="x", session_id="s")
    with pytest.raises(RunnerBusyError):
        runner.enqueue(j2.job_id, lambda: {"ok": True})
    gate.set()
    assert runner.wait(j1.job_id, timeout_s=5.0).status == "completed"
    # Slot freed: enqueue works again.
    runner.enqueue(j2.job_id, lambda: {"ok": True})
    assert runner.wait(j2.job_id, timeout_s=5.0).status == "completed"
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
