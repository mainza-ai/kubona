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


def test_fork_leaves_no_orphan_entry(tmp_path):
    """fork_counterfactual registers the child ONLY under its fork id: no
    stale UUID key may linger in _loops, and every key must match its
    handle's loop_id (the old register-then-rekey orphaned the minted UUID)."""
    from kubona.control.steering import SteeringService

    sup = LoopSupervisor(path=tmp_path / "loops.jsonl")
    parent = sup.register("vqe-loop", "s1", 10)
    sup.step(parent.loop_id, "step 1")
    sup.step(parent.loop_id, "step 2")
    child = SteeringService(sup).fork_counterfactual(parent.loop_id, 1)
    assert set(sup._loops) == {parent.loop_id, child.loop_id}
    for key, handle in sup._loops.items():
        assert handle.loop_id == key, f"orphan entry: key {key!r} holds {handle.loop_id!r}"
    assert child.loop_id.startswith("fork-")
    assert child.step == 1
    assert child.history[-1]["event"] == "fork_created"
    assert child.history[-1]["parent_id"] == parent.loop_id


def test_fork_survives_restart_without_orphan(tmp_path):
    """The append-only log must replay to exactly parent + child: a stale
    register row for a minted UUID would resurrect the orphan after restart."""
    from kubona.control.steering import SteeringService

    path = tmp_path / "loops.jsonl"
    sup = LoopSupervisor(path=path)
    parent = sup.register("vqe-loop", "s1", 10)
    sup.step(parent.loop_id, "step 1")
    child = SteeringService(sup).fork_counterfactual(parent.loop_id, 1)

    restarted = LoopSupervisor(path=path)
    assert set(restarted._loops) == {parent.loop_id, child.loop_id}
    for key, handle in restarted._loops.items():
        assert handle.loop_id == key, f"resurrected orphan: {key!r} holds {handle.loop_id!r}"
    assert restarted.get(child.loop_id).history[-1]["event"] == "fork_created"
