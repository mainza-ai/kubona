"""Fragile-late scheduling: semantics preserved, fragile pushed later."""

from kubona.circuits import Circuit, Gate
from kubona.circuits.schedule import (
    commutes,
    equivalent_up_to_phase,
    fragile_metrics,
    schedule_fragile_late,
)


def _bell_with_t():
    # T on q0 before the entangler cannot cross CX (shared qubits,
    # non-diagonal pair) — scheduler must leave it; equivalence still holds.
    return Circuit(name="t-bell", num_qubits=2, gates=[
        Gate("t", (0,)), Gate("h", (0,)), Gate("cx", (0, 1))])


def _qft_like():
    # T(0) can drift right past disjoint-qubit Cliffords but not past the
    # same-qubit H; cp is diagonal and slides past commuting neighbors.
    return Circuit(name="qft-like", num_qubits=3, gates=[
        Gate("t", (0,)), Gate("h", (1,)), Gate("h", (0,)),
        Gate("cx", (1, 2)), Gate("tdg", (2,)), Gate("x", (1,))])


def test_blocked_nonclifford_stays_but_equivalent():
    c = _bell_with_t()
    s = schedule_fragile_late(c)
    assert equivalent_up_to_phase(c, s)
    assert [g.name for g in s.gates] == ["t", "h", "cx"]


def test_fragile_moves_later_and_equivalent():
    c = _qft_like()
    before = fragile_metrics(c)
    s = schedule_fragile_late(c)
    after = fragile_metrics(s)
    assert equivalent_up_to_phase(c, s)
    assert after["first_nonclifford"] >= before["first_nonclifford"]
    assert after["n_nonclifford"] == before["n_nonclifford"]
    # The T on q1 genuinely moves past the commuting H-adjacent gates.
    assert after["first_nonclifford"] > before["first_nonclifford"]


def test_all_clifford_untouched():
    c = Circuit(name="bell", num_qubits=2,
                gates=[Gate("h", (0,)), Gate("cx", (0, 1))])
    s = schedule_fragile_late(c)
    assert [g.name for g in s.gates] == ["h", "cx"]
    assert fragile_metrics(s)["first_nonclifford"] == -1


def test_idempotent_and_input_untouched():
    c = _qft_like()
    before = [g.name for g in c.gates]
    once = schedule_fragile_late(c)
    twice = schedule_fragile_late(once)
    assert [g.name for g in c.gates] == before
    assert [g.name for g in twice.gates] == [g.name for g in once.gates]


def test_commutes_soundness_spot_checks():
    assert commutes(Gate("h", (0,)), Gate("x", (1,))) is True  # disjoint
    assert commutes(Gate("t", (0,)), Gate("z", (0,))) is True  # diagonal
    assert commutes(Gate("cz", (0, 1)), Gate("t", (1,))) is True  # diagonal
    assert commutes(Gate("cx", (0, 1)), Gate("z", (1,))) is False  # target!
    assert commutes(Gate("cx", (0, 1)), Gate("h", (0,))) is False
    assert commutes(Gate("rx", (0,), (0.5,)), Gate("rx", (0,), (1.2,))) is True
    assert commutes(Gate("rx", (0,), (0.5,)), Gate("ry", (0,), (1.2,))) is False
