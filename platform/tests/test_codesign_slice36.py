"""Slice 36: scheduled Hermitian (F64) walk — freeze, repair, end, route.

Production bar: the scheduled discipline is ported exact (freeze
off-schedule, depth-2/3 block repair on scheduled rounds, ending test
g0=g1=g2==0 with the unrepairability theorem pinned, three-case routing
all verified, 1 history bit per round load-bearing in reverse). The
Hermitian tie-in closes geometry -> syndromes -> walk -> locator at toy
scale (locator vanishes at planted errors). The xi-register incremental
accumulation stays queued (seeding under-specified in the excerpt).
"""

import pytest

from kubona.codesign import (
    F64,
    ScheduledFieldWalker,
    curve_points,
    heval_at,
    hmatvec,
    hreduce,
    hmul,
    locator_recurrence_holds,
    monomials,
    scheduled_find_null_f64,
    syndrome,
)


def test_scheduled_freeze_repair_reverse():
    A = [[39, 49, 45, 2, 29, 62, 0, 63], [49, 21, 57, 58, 63, 47, 14, 29],
         [45, 57, 59, 27, 42, 6, 37, 34], [2, 58, 27, 50, 36, 60, 37, 9],
         [29, 63, 42, 36, 5, 47, 20, 50], [62, 47, 6, 60, 47, 16, 46, 28],
         [0, 14, 37, 37, 20, 46, 48, 21], [63, 29, 34, 9, 50, 28, 21, 56]]
    seed = [36, 9, 3, 2, 14, 55, 39, 61]
    w = ScheduledFieldWalker(A, F64, repair_rounds={2}, max_rounds=12)
    assert w.run(seed) == "dependent"
    notes = [d["note"] for d in w.decisions]
    assert notes[0] == "ordinary" and notes[1] == "frozen"
    assert notes[2] == "repaired" and w.maxdepth == 2
    assert w.frozen_rounds == 1 and w.repairs == 1
    rev = w.reverse()
    assert rev["history_bits"] == len(w.decisions) == 9
    routed = w.route()
    assert routed["case"] == "accumulated"
    assert hmatvec(A, routed["locator"], F64) == [0] * 8
    # the history bit is load-bearing: flipping it refuses
    w.decisions[0] = dict(w.decisions[0], advanced=False)
    with pytest.raises(ValueError):
        w.reverse()


def test_end_check_and_routing_cases():
    zA = [[0, 0], [0, 0]]
    w = ScheduledFieldWalker(zA, F64)
    assert w.end_check([1, 1]) is True  # isotropic + zero action
    assert w.end_check([1, 0]) is False  # non-isotropic: ordinary applies
    assert w.end_check([0, 0]) is True  # zero vector trivially ends
    assert w.run([1, 1]) == "ended" and w.end_round == 0
    assert w.reverse()["rounds"] == 1
    with pytest.raises(ValueError):
        w.route()  # empty basis: nothing to route
    w = ScheduledFieldWalker([[0, 0], [0, 1]], F64, max_rounds=1)
    assert w.run([1, 0]) == "unfinished"
    assert w.route() == {"case": "self", "locator": [1, 0]}
    w = ScheduledFieldWalker([[1, 1], [1, 1]], F64, max_rounds=1)
    assert w.run([1, 0]) == "unfinished"
    assert w.route() == {"case": "advance", "locator": [1, 1]}
    w = ScheduledFieldWalker([[1, 0], [0, 1]], F64, max_rounds=1)
    assert w.run([1, 0]) == "unfinished"
    with pytest.raises(ValueError):
        w.route()  # no case holds: honest miss
    with pytest.raises(ValueError):
        ScheduledFieldWalker([[1, 0], [0, 1]], F64).route()  # walk never ran


def test_end_check_theorem_pins_unrepairability():
    # g0 = g1 = g2 == 0 forces every depth <= 3 Krylov Gram singular
    # (first row (g_0..g_{d-1}) is all zero) — the paper's reason repair
    # is hopeless, machine-checked, not cited.
    from kubona.codesign.hlanczos import dot, matvec
    A = [[0, 0], [0, 0]]
    w = ScheduledFieldWalker(A, F64)
    v = [1, 1]
    assert w.end_check(v) is True
    for d in (1, 2, 3):
        block = [v]
        for _ in range(1, d):
            block.append(matvec(A, block[-1], F64))
        assert w._block_gram_ok(block) is False


def _hermitian_A(errors, k=8):
    basis = monomials(75)[:k]
    syns = [syndrome(errors, {(a, b): 1}, F64) for (a, b) in monomials(150)]
    A = [[syndrome(errors, hreduce(hmul({(a1, b1): 1}, {(a2, b2): 1}, F64), F64), F64)
          for (a2, b2) in basis] for (a1, b1) in basis]
    return A, basis, syns


def test_hermitian_scheduled_locator():
    pts = curve_points(F64)
    for errors in [[(pts[5], 7)],
                   [(pts[5], 7), (pts[100], 9)],
                   [(pts[0], 3), (pts[511], 5)]]:
        A, basis, _syns = _hermitian_A(errors)
        seeds = [[1 if i == s else 0 for i in range(8)] for s in range(8)]
        rep = scheduled_find_null_f64(A, F64, seeds)
        assert rep["proven"] is True, errors
        poly = {m: c for m, c in zip(basis, rep["locator"]) if c}
        assert all(heval_at(poly, P, F64) == 0 for P, _ in errors)
        assert locator_recurrence_holds(poly, errors, monomials(150), F64) is True


def test_scheduled_refusals():
    with pytest.raises(ValueError):
        ScheduledFieldWalker([[0, 1], [1, 0], [0, 0]], F64)  # non-square
    with pytest.raises(ValueError):
        ScheduledFieldWalker([[0, 1], [0, 0]], F64)  # asymmetric
    with pytest.raises(ValueError):
        ScheduledFieldWalker([[1, 0], [0, 1]], F64, repair_rounds={9})
    with pytest.raises(ValueError):
        ScheduledFieldWalker([[1, 0], [0, 1]], F64, repair_rounds=[0])
    with pytest.raises(ValueError):
        ScheduledFieldWalker([[1, 0], [0, 1]], F64, max_rounds=0)
    with pytest.raises(ValueError):
        ScheduledFieldWalker([[1, 0], [0, 1]], F64, max_rounds=65)
    A = [[1, 0], [0, 1]]
    with pytest.raises(ValueError):
        ScheduledFieldWalker(A, F64).run([0, 0])  # zero seed refuses
    with pytest.raises(ValueError):
        scheduled_find_null_f64(A, F64, [])  # no seeds refuses
