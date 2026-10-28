"""Loop supervisor: watch + interrupt (the Phase-2 console exit gate backend)."""

import pytest

from kubona.control import LoopSupervisor


def test_loop_lifecycle(tmp_path):
    sup = LoopSupervisor(path=tmp_path / "loops.jsonl")
    h = sup.register("vqe-loop", session_id="s1", total_steps=10)
    assert h.status == "running"
    assert sup.step(h.loop_id, "step 1") is True
    assert sup.step(h.loop_id, "step 2") is True
    assert sup.get(h.loop_id).step == 2


def test_pause_blocks_steps(tmp_path):
    sup = LoopSupervisor(path=tmp_path / "loops.jsonl")
    h = sup.register("vqe-loop", "s1", 10)
    sup.step(h.loop_id, "step 1")
    sup.pause(h.loop_id)
    assert sup.step(h.loop_id, "step 2") is False  # paused: loop must stop
    assert sup.get(h.loop_id).step == 1
    sup.resume(h.loop_id)
    assert sup.step(h.loop_id, "step 2 again") is True


def test_kill_blocks_and_marks(tmp_path):
    sup = LoopSupervisor(path=tmp_path / "loops.jsonl")
    h = sup.register("vqe-loop", "s1", 10)
    sup.kill(h.loop_id)
    assert sup.get(h.loop_id).status == "killed"
    assert sup.step(h.loop_id, "nope") is False


def test_list_shows_loops(tmp_path):
    sup = LoopSupervisor(path=tmp_path / "loops.jsonl")
    sup.register("a", "s1", 5)
    sup.register("b", "s2", 5)
    assert len(sup.list()) == 2


def test_loops_survive_restart_and_run_becomes_paused(tmp_path):
    path = tmp_path / "loops.jsonl"
    sup = LoopSupervisor(path=path)
    h = sup.register("vqe-loop", "s1", 10, tenant="lab-a")
    sup.step(h.loop_id, "step 1", metric=0.5)

    restarted = LoopSupervisor(path=path)
    recovered = restarted.get(h.loop_id)
    assert recovered.name == "vqe-loop"
    assert recovered.tenant == "lab-a"
    assert recovered.step == 1
    assert recovered.history[0]["desc"] == "step 1"
    assert recovered.status == "paused"
    assert recovered.meta["recovered_after_restart"] is True

    restarted.resume(h.loop_id)
    # Even an explicit resume is not trusted as "alive" by a later process:
    # only the owning process can attest a heartbeat, so recovery is paused.
    assert LoopSupervisor(path=path).get(h.loop_id).status == "paused"


def test_fork_inherits_parent_tenant(tmp_path):
    from kubona.control.steering import SteeringService

    sup = LoopSupervisor(path=tmp_path / "loops.jsonl")
    parent = sup.register("vqe-loop", "s1", 10, tenant="lab-a")
    sup.step(parent.loop_id, "step 1")
    child = SteeringService(sup).fork_counterfactual(parent.loop_id, 1)
    assert child.tenant == "lab-a"
    assert sup.get_for(child.loop_id, "lab-a").name == child.name
    try:
        sup.get_for(child.loop_id, "lab-b")
    except PermissionError:
        pass
    else:
        raise AssertionError("fork must not cross tenant boundaries")
