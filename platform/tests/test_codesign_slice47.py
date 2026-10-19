"""Slice 47: coordinate-descent ordering for the gauge-loop schedule.

Production bar: the optimizer is a labeled heuristic (coordinate
descent over order <-> reps with exact, direction-preserving deltas),
and every number is verified — the improved schedule replays exactly
(trace values match, op totals match) and is pinned, never presented
as optimal. Measured: greedy 3513 + 1215 -> 3507 + 1213 (21 accepted
moves, 2 cycles, input/reverse starts pruned). The gain is deliberately
reported as thin: greedy + local search plateaus 12% above the paper's
3136 + 480, which falsifies "we just didn't try" and locates the gap
where it belongs — the paper's global dynamic program over orderings
and representatives, unrecoverable from the SI text (which names the DP
but gives no recurrence). Substantive design point, machine-checked:
the transform cost is asymmetric on 46% of sampled rep pairs
(pre-invert option + load discount), so textbook segment-reversal
2-opt deltas would be inexact; the slice uses order-preserving
relocation/swap moves with exact O(1) boundary deltas instead.
"""

import random

import pytest

from kubona.codesign import (
    all_elements,
    apply_plan_steps,
    decode_order,
    gidentity,
    ginv,
    gmul,
    lattice_links,
    loop_representatives,
    loop_trace,
    optimize_schedule,
    schedule_gauge_loops,
    trace_class,
)


def _loops():
    from kubona.codesign import plaquettes, rectangles
    return plaquettes() + rectangles()


def test_optimized_schedule_pinned_and_beats_greedy():
    loops = _loops()
    sched = optimize_schedule(loops)
    assert sched["n_loops"] == 576
    assert sorted(sched["order"]) == list(range(576))
    assert sched["total_mults"] == 3507
    assert sched["total_inversions"] == 1213
    assert sched["n_preinvert"] == 325
    assert sched["start"] == "greedy" and sched["cycles"] == 2
    assert sched["moves"] == 21 and sched["skipped"] == ["input", "reverse"]
    greedy = schedule_gauge_loops(loops)
    assert (sched["total_mults"] + sched["total_inversions"],
            sched["total_mults"]) <= (
                greedy["total_mults"] + greedy["total_inversions"],
                greedy["total_mults"])
    # the paper's DP + hand structure stays queued (12% mults, 2.5x invs)
    assert sched["total_mults"] > 3136 and sched["total_inversions"] > 480


def test_replay_proves_optimized_schedule():
    rng = random.Random(11)
    els = all_elements()
    cfg = {link: els[rng.randrange(216)] for link in lattice_links()}
    loops = _loops()
    sched = optimize_schedule(loops)
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


def test_transform_cost_asymmetric():
    # the design decision, machine-checked: reversal deltas would be
    # inexact on nearly half the sampled pairs, so the slice uses
    # order-preserving moves. Fixed seed: deterministic probe.
    from kubona.codesign import gaugeloops as G
    loops = _loops()
    links = []
    for loop in loops:
        for link, _ in loop:
            if link not in links:
                links.append(link)
    reps = [loop_representatives(loop, links) for loop in loops]
    rng = random.Random(0)
    n_asym = n_tot = 0
    for _ in range(2000):
        a = rng.randrange(576)
        b = rng.randrange(576)
        if a == b:
            continue
        ra = reps[a][rng.randrange(len(reps[a]))]
        rb = reps[b][rng.randrange(len(reps[b]))]
        n_tot += 1
        if G._plan_key(G.transform_plan(ra, rb)) != G._plan_key(G.transform_plan(rb, ra)):
            n_asym += 1
    assert (n_asym, n_tot) == (912, 1997)


def test_subset_exercises_multistart_and_refusals():
    # small-scale logic proof (fast): multi-start runs, structural bound
    # holds, determinism holds, refusals refuse.
    from kubona.codesign import gaugeloops as G
    loops = _loops()[:12]
    greedy = schedule_gauge_loops(loops)
    sched = optimize_schedule(loops)
    assert sorted(sched["order"]) == list(range(12))
    assert (sched["total_mults"] + sched["total_inversions"],
            sched["total_mults"]) <= (
                greedy["total_mults"] + greedy["total_inversions"],
                greedy["total_mults"])
    assert optimize_schedule(loops) == sched  # deterministic
    assert set(sched) >= {"order", "reps", "plans", "total_mults",
                          "total_inversions", "n_preinvert", "n_loops",
                          "start", "cycles", "moves", "skipped"}
    with pytest.raises(ValueError):
        optimize_schedule([])
    with pytest.raises(ValueError):
        optimize_schedule([[]])
    with pytest.raises(ValueError):
        optimize_schedule(loops, starts=("sideways",))
    with pytest.raises(ValueError):
        optimize_schedule(loops, starts=())
    with pytest.raises(ValueError):
        decode_order([], [[[(0, True)]]])
    with pytest.raises(ValueError):
        decode_order([1, 0], [[[(0, True)]]])
