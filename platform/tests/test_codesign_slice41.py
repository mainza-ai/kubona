"""Slice 41: greedy loop-reuse schedule over 576 loops (heuristic, bounded).

Production bar: the representative rule lives in the gauge kernel (single
home — rep sets match it exactly on all 576 loops); the greedy order is
labeled heuristic, never optimal, and the exact Held-Karp DP bounds it
from below on small subsets (dp optimum <= rescored greedy, always);
replay proves every transition (traces match, sums match accumulate,
op counts match the schedule). Totals: 3513 mults + 1215 invs vs the
naive-with-reversal baseline 4992 + 1536 and the paper's 3136 + 480.
"""

import random

import pytest

from kubona.codesign import (
    LOOP_PAPER_COSTS,
    accumulate,
    all_elements,
    apply_plan_steps,
    gidentity,
    ginv,
    gmul,
    lattice_links,
    loop_product,
    loop_representatives,
    loop_to_abstract,
    loop_trace,
    plaquettes,
    rectangles,
    schedule_gauge_loops,
    trace_class,
    transform_plan,
)


def _links():
    return lattice_links()


def test_schedule_valid_and_pinned():
    loops = plaquettes() + rectangles()
    sched = schedule_gauge_loops(loops)
    assert sched["n_loops"] == 576
    assert sorted(sched["order"]) == list(range(576))
    assert sched["total_mults"] == 3513
    assert sched["total_inversions"] == 1215
    assert sched["n_preinvert"] == 325
    rep = accumulate({link: gidentity() for link in lattice_links()})
    assert rep["build_mults"] == 2496  # schedule beats naive-with-reversal
    assert sched["total_mults"] < 2 * rep["build_mults"] == 4992


def test_replay_proves_schedule():
    rng = random.Random(11)
    els = all_elements()
    cfg = {link: els[rng.randrange(216)] for link in lattice_links()}
    loops = plaquettes() + rectangles()
    sched = schedule_gauge_loops(loops)
    W = gidentity()
    held: list = []
    tot_m = tot_i = 0
    for li, rep, plan in zip(sched["order"], sched["reps"], sched["plans"]):
        if plan["pre_invert"]:
            W = ginv(W)
            tot_i += 1
        for link, fwd in plan["cancels"]:
            g = cfg[link]
            W = gmul(W, ginv(g) if fwd else g)
            tot_m += 1
            if fwd:
                tot_i += 1
        load = len(plan["cancels"]) == len(held)  # k==0: fully canceled
        for i, (link, fwd) in enumerate(plan["appends"]):
            g = cfg[link]
            if not fwd:
                g = ginv(g)
            if load and i == 0:
                assert W == gidentity(), "cancels must zero W (replay check)"
                W = g
            else:
                W = gmul(W, g)
                tot_m += 1
        held = apply_plan_steps(held, plan)
        want, _ = loop_trace(cfg, loops[li])
        assert trace_class(W) == want, li
    assert tot_m == sched["total_mults"] and tot_i == sched["total_inversions"]
    assert sum(1 for _ in sched["plans"]) == 576


def test_rep_sets_match_gauge_kernel():
    from kubona.codesign.gauge import representatives as gauge_reps
    links = lattice_links()
    for loop in plaquettes() + rectangles():
        ab = loop_to_abstract(loop, links)
        mine = {loop_to_abstract(r, links) for r in loop_representatives(loop, links)}
        assert mine == set(gauge_reps(ab))


def test_cost_convention_map_hand_checked():
    # my cost model == gauge kernel + load discount (first append free)
    # + counted cancel inversions. Hand-computed pairs pin the map.
    from kubona.codesign.gauge import transform_cost
    links = [("L%d" % i, 0) for i in range(6)]
    a = [(links[0], True), (links[1], True)]
    b = [(links[0], True), (links[2], True)]
    ga, gb = loop_to_abstract(a, links), loop_to_abstract(b, links)
    assert transform_cost(ga, gb) == (2, 0)  # paper example shape
    p = transform_plan(a, b)
    assert (p["mults"], p["inversions"]) == (2, 1)  # k=1: cancel 1 + append 1; fwd cancel inverts
    # identical loops: zero cost both ways
    assert transform_cost(ga, ga) == (0, 0)
    assert transform_plan(a, a)["mults"] == 0
    # empty start: mine loads the first link free, gauge counts it
    p0 = transform_plan([], b)
    assert (p0["mults"], p0["inversions"]) == (1, 0)
    assert transform_cost((), gb) == (2, 0)
    # disjoint: pre-invert wins (total 4 < 5); phases still exact
    c = [(links[3], True), (links[4], True)]
    gc = loop_to_abstract(c, links)
    assert transform_cost(ga, gc) == (4, 0)
    pc = transform_plan(a, c)
    assert (pc["mults"], pc["inversions"]) == (3, 1)
    with pytest.raises(ValueError):
        transform_plan([], [])
    with pytest.raises(ValueError):
        transform_plan(a, [])


def test_dp_bounds_greedy():
    from kubona.codesign.gauge import schedule_loops
    rng = random.Random(3)
    loops = plaquettes() + rectangles()
    links = lattice_links()
    for trial in range(3):
        sub = rng.sample(loops, 6)
        ids = []
        for loop in sub:
            for link, _ in loop:
                if link not in ids:
                    ids.append(link)
        ab = [loop_to_abstract(l, ids) for l in sub]
        dp = schedule_loops([tuple(x) for x in ab])
        greedy = schedule_gauge_loops(sub)
        # rescore greedy under the gauge convention (same numbers DP used)
        from kubona.codesign.gauge import transform_cost
        order = greedy["order"]
        reps = greedy["reps"]
        rab = [loop_to_abstract(r, ids) for r in reps]
        tot = 0
        prev = None
        for r in rab:
            if prev is None:
                tot += len(r)
            else:
                tot += transform_cost(prev, r)[0]
            prev = r
        assert dp["mults"] <= tot
        assert sorted(dp["order"]) == list(range(6))


def test_schedule_refusals():
    with pytest.raises(ValueError):
        schedule_gauge_loops([])
    with pytest.raises(ValueError):
        schedule_gauge_loops([plaquettes()[0], []])
    with pytest.raises(ValueError):
        loop_to_abstract([(((9, 9, 9), 0), True)], lattice_links())
    with pytest.raises(ValueError):
        loop_to_abstract([(((0, 0, 0), 0), "yes")], lattice_links())
    from kubona.codesign.gaugeloops import abstract_to_loop
    with pytest.raises(ValueError):
        abstract_to_loop((), lattice_links())
    with pytest.raises(ValueError):
        abstract_to_loop((0, 1), lattice_links())
    with pytest.raises(ValueError):
        apply_plan_steps([], {"pre_invert": True, "cancels": [], "appends": []})


def test_apply_plan_steps_units():
    from kubona.codesign.gaugeloops import apply_plan_steps
    held = [("a", True), ("b", False)]
    out = apply_plan_steps(held, {"pre_invert": False,
                                  "cancels": [("b", False)],
                                  "appends": [("c", True)]})
    assert out == [("a", True), ("c", True)]
    out2 = apply_plan_steps(held, {"pre_invert": True, "cancels": [], "appends": []})
    assert out2 == [("b", True), ("a", False)]
    with pytest.raises(ValueError):
        apply_plan_steps(held, {"pre_invert": False,
                                "cancels": [("x", True)], "appends": []})
