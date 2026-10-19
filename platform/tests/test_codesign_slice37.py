"""Slice 37: Hermitian scheduled walk at paper scale (48-dim locator).

Production bar: the 48-monomial walk runs the paper's program shape
(52-round cap, 17 scheduled repair rounds) on a syndrome-built A and
returns a locator verified three independent ways (vanishes at planted
errors, A.sigma == 0 by direct matvec, nullspace recurrence holds); the
eval-interp identity is proven at 48-dim (eta duality + factored ==
direct with 123 pointwise mults); costs are paper unit costs times the
measured round mix, labeled projection. The xi-register accumulation
stays queued.
"""

import random

import pytest

from kubona.codesign import (
    F64,
    WALK_PAPER_COSTS,
    curve_points,
    heval_at,
    hmatvec,
    hreduce,
    hmul,
    locator_recurrence_holds,
    monomials,
    scheduled_find_null_f64,
    syndrome,
    walk_cost_ledger,
)
from kubona.codesign.hlanczos import _solve, dot, matvec


def _planted_48():
    pts = curve_points(F64)
    loc = monomials(75)
    syn = monomials(150)
    rng = random.Random(0)
    errors = [(pts[rng.randrange(512)], rng.randrange(1, 64)) for _ in range(5)]
    syns = [syndrome(errors, {(a, b): 1}, F64) for (a, b) in syn]
    A = [[syndrome(errors, hreduce(hmul({(a1, b1): 1}, {(a2, b2): 1}, F64), F64), F64)
          for (a2, b2) in loc] for (a1, b1) in loc]
    return pts, loc, syn, errors, syns, A


def test_paper_scale_locator():
    pts, loc, syn, errors, _syns, A = _planted_48()
    assert all(A[i][j] == A[j][i] for i in range(48) for j in range(48))
    rep = sorted(set(range(52)) & set(3 * k + 2 for k in range(17)))
    assert len(rep) == 17
    seeds = [[1 if i == s else 0 for i in range(48)] for s in range(8)]
    from kubona.codesign import ScheduledFieldWalker
    w = ScheduledFieldWalker(A, F64, repair_rounds=set(rep))
    assert w.max_rounds == 52
    out = scheduled_find_null_f64(A, F64, seeds, repair_rounds=set(rep))
    assert out["proven"] is True
    assert out["case"] == "accumulated" and out["outcome"] == "dependent"
    assert out["seeds_tried"] == 1 and out["basis_size"] == 6
    assert out["repairs"] == 0 and out["frozen_rounds"] == 0
    assert out["history_bits"] == 7
    poly = {m: c for m, c in zip(loc, out["locator"]) if c}
    assert all(heval_at(poly, P, F64) == 0 for P, _ in errors)
    assert hmatvec(A, out["locator"], F64) == [0] * 48
    assert locator_recurrence_holds(poly, errors, syn, F64) is True
    from kubona.codesign import ScheduledFieldWalker
    w = ScheduledFieldWalker(A, F64, repair_rounds=set(rep))
    assert w.run([1] + [0] * 47) == "dependent"
    assert w.reverse()["rounds"] == 7
    led = walk_cost_ledger(w.decisions)
    assert led == {"ordinary": 6, "repaired_d2": 0, "repaired_d3": 0,
                   "frozen_terminal": 1, "total_T": 32400, "history_bits": 7}


def test_eval_interp_48():
    _pts, _loc, syn, errors, syns, A = _planted_48()
    pts = curve_points(F64)
    loc = monomials(75)
    rng = random.Random(5)
    Rs = rng.sample(pts, 123)
    Phi = [[heval_at({(a, b): 1}, P, F64) for (a, b) in loc] for P in Rs]
    Emat = [[heval_at({(a, b): 1}, P, F64) for (a, b) in syn] for P in Rs]
    ET = [list(r) for r in zip(*Emat)]
    eta = _solve([r[:] for r in ET], list(syns), F64)
    assert eta is not None
    rng2 = random.Random(9)
    for _ in range(3):
        c = [rng2.randrange(64) for _ in range(123)]
        f = {m: v for m, v in zip(syn, c) if v}
        acc = 0
        for i, P in enumerate(Rs):
            acc = F64.add(acc, F64.mul(eta[i], heval_at(f, P, F64)))
        assert acc == syndrome(errors, f, F64)
    u = [rng2.randrange(64) for _ in range(48)]
    Phi_u = [dot(r, u, F64) for r in Phi]
    out = [0] * 48
    for k in range(48):
        acc = 0
        for i in range(123):
            acc = F64.add(acc, F64.mul(F64.mul(eta[i], Phi[i][k]), Phi_u[i]))
        out[k] = acc
    assert out == matvec(A, u, F64)
    assert len(Rs) == 123  # paper's eval count, reproduced as structure


def test_cost_ledger_arithmetic():
    assert WALK_PAPER_COSTS["ordinary_round"] == 5400
    assert WALK_PAPER_COSTS["repair_round_d2"] == 12000
    assert WALK_PAPER_COSTS["repair_round_d3"] == 17000
    dec = ([{"round": i, "advanced": True, "depth": 1, "note": "ordinary"} for i in range(4)]
           + [{"round": 4, "advanced": True, "depth": 2, "note": "repaired"}]
           + [{"round": 5, "advanced": True, "depth": 3, "note": "repaired"}]
           + [{"round": 6, "advanced": False, "depth": 0, "note": "frozen"}]
           + [{"round": 7, "advanced": True, "depth": 0, "note": "dependent"}])
    led = walk_cost_ledger(dec)
    assert led["total_T"] == 4 * 5400 + 12000 + 17000
    assert led["history_bits"] == 8 and led["frozen_terminal"] == 2
    with pytest.raises(ValueError):
        walk_cost_ledger([{"round": 0, "advanced": True, "depth": 9, "note": "repaired"}])
    with pytest.raises(ValueError):
        walk_cost_ledger([{"round": 0, "note": "bogus"}])


def test_dim_cap_48():
    I48 = [[1 if i == j else 0 for j in range(48)] for i in range(48)]
    from kubona.codesign import ScheduledFieldWalker
    ScheduledFieldWalker(I48, F64)  # paper scale accepted
    I49 = [[1 if i == j else 0 for j in range(49)] for i in range(49)]
    with pytest.raises(ValueError):
        ScheduledFieldWalker(I49, F64)  # runaway sizes still refuse
