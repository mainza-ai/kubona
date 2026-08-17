"""Loop supervisor: watch + interrupt (the Phase-2 console exit gate backend)."""

import pytest

from kubona.control import LoopSupervisor


def test_loop_lifecycle():
    sup = LoopSupervisor()
    h = sup.register("vqe-loop", session_id="s1", total_steps=10)
    assert h.status == "running"
    assert sup.step(h.loop_id, "step 1") is True
    assert sup.step(h.loop_id, "step 2") is True
    assert sup.get(h.loop_id).step == 2


def test_pause_blocks_steps():
    sup = LoopSupervisor()
    h = sup.register("vqe-loop", "s1", 10)
    sup.step(h.loop_id, "step 1")
    sup.pause(h.loop_id)
    assert sup.step(h.loop_id, "step 2") is False  # paused: loop must stop
    assert sup.get(h.loop_id).step == 1
    sup.resume(h.loop_id)
    assert sup.step(h.loop_id, "step 2 again") is True


def test_kill_blocks_and_marks():
    sup = LoopSupervisor()
    h = sup.register("vqe-loop", "s1", 10)
    sup.kill(h.loop_id)
    assert sup.get(h.loop_id).status == "killed"
    assert sup.step(h.loop_id, "nope") is False


def test_list_shows_loops():
    sup = LoopSupervisor()
    sup.register("a", "s1", 5)
    sup.register("b", "s2", 5)
    assert len(sup.list()) == 2
