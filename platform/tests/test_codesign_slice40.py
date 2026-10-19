"""Slice 40: gauge loop products and trace accumulation (naive, model-level).

Production bar: all 576 loops (192 plaquettes + 384 rectangles on the
periodic 4^3 lattice) multiply out with the verified group law and match
the true 3x3 matrix products; every trace reads from the machine-derived
class table; the all-identity config pins S_P/S_R exactly; rotation and
inversion representatives provably preserve every loop value (the reuse
handoff); counts are measured, with the paper's baseline reproduced
exactly (4992 = 192x6 + 384x10 with reversal) and its optimized schedule
(3136 + 480) recorded as the queued target, never claimed.
"""

import random

import pytest

from kubona.codesign import (
    LOOP_PAPER_COSTS,
    accumulate,
    all_elements,
    gidentity,
    ginv,
    invert_loop,
    lattice_links,
    loop_product,
    loop_representatives,
    loop_to_abstract,
    loop_trace,
    plaquettes,
    rectangles,
    rep_matrix,
    trace_class,
    trace_flags,
)


def test_lattice_counts():
    assert len(lattice_links()) == 192
    P, R = plaquettes(), rectangles()
    assert len(P) == 192 and all(len(l) == 4 for l in P)
    assert len(R) == 384 and all(len(l) == 6 for l in R)
    links = set(lattice_links())
    for loop in P + R:
        for link, fwd in loop:
            assert link in links and isinstance(fwd, bool)
    assert LOOP_PAPER_COSTS == {"baseline_plaquette": 6, "baseline_rectangle": 10,
                                "optimized_mults": 3136, "optimized_inversions": 480,
                                "n_loops": 576}


def test_all_identity_sums_pinned():
    cfg = {link: gidentity() for link in lattice_links()}
    rep = accumulate(cfg)
    assert rep["S_P"] == 1152 and rep["n_P"] == 192
    assert rep["S_R"] == 2304 and rep["n_R"] == 384
    assert rep["build_mults_P"] == 576 and rep["build_mults_R"] == 1920
    assert rep["build_mults"] == 2496 and rep["inversions"] == 1536
    # reversal doubles the mults: naive total == paper baseline exactly
    assert 2 * rep["build_mults"] == 192 * 6 + 384 * 10 == 4992


def test_loop_products_match_matrices():
    from kubona.codesign.group72 import _miden, _mmul
    rng = random.Random(4)
    els = all_elements()
    cfg = {link: els[rng.randrange(216)] for link in lattice_links()}
    for loop in plaquettes() + rectangles():
        w, _cost = loop_product(cfg, loop)
        M = _miden()
        for link, fwd in loop:
            g = cfg[link]
            M = _mmul(M, rep_matrix(g if fwd else ginv(g)))
        assert M == rep_matrix(w)


def test_traces_and_single_link_probes():
    rng = random.Random(5)
    els = all_elements()
    cfg = {link: els[rng.randrange(216)] for link in lattice_links()}
    for loop in plaquettes() + rectangles():
        v, cost = loop_trace(cfg, loop)
        w, _ = loop_product(cfg, loop)
        assert v == trace_class(w) and cost["mults"] == len(loop) - 1
    rep = accumulate(cfg)
    assert abs(rep["S_P"]) <= 6 * rep["n_P"] and abs(rep["S_R"]) <= 6 * rep["n_R"]
    # single non-identity link: every loop through it reads its trace
    g = els[100]
    host = next(iter(lattice_links()))
    cfg1 = {link: gidentity() for link in lattice_links()}
    cfg1[host] = g
    n = 0
    for loop in plaquettes() + rectangles():
        uses = [(link, fwd) for link, fwd in loop if link == host]
        if len(uses) == 1:
            (_link, fwd) = uses[0]
            v, _ = loop_trace(cfg1, loop)
            assert v == trace_class(g if fwd else ginv(g))
            n += 1
    assert n > 0


def test_representative_invariance():
    rng = random.Random(6)
    els = all_elements()
    cfg = {link: els[rng.randrange(216)] for link in lattice_links()}
    links = lattice_links()
    for loop in plaquettes() + rectangles():
        v, _ = loop_trace(cfg, loop)
        for rep in loop_representatives(loop, links):
            assert loop_trace(cfg, rep)[0] == v, (loop, rep)
    assert all(trace_class(ginv(g)) == trace_class(g) for g in els)
    # one-hot flags hold on loop values too
    for loop in plaquettes()[:8]:
        v, _ = loop_trace(cfg, loop)
        fl = trace_flags(loop_product(cfg, loop)[0])
        assert sum(fl) == (0 if v == 6 else 1)
    # concrete reps match the gauge kernel (single home for the rule)
    from kubona.codesign.gauge import representatives as _gauge_reps
    for loop in plaquettes()[:8] + rectangles()[:8]:
        ab = loop_to_abstract(loop, links)
        assert {loop_to_abstract(r, links) for r in loop_representatives(loop, links)} == set(_gauge_reps(ab))


def test_loop_refusals():
    cfg = {link: gidentity() for link in lattice_links()}
    with pytest.raises(ValueError):
        loop_product(cfg, [])
    with pytest.raises(ValueError):
        invert_loop([])
    with pytest.raises(ValueError):
        loop_product(cfg, [((0, 0, 0), 0), "bad"])
    with pytest.raises(ValueError):
        loop_product(cfg, [(((9, 9, 9), 0), True)])
    bad = dict(cfg)
    bad[((0, 0, 0), 0)] = (9, 9, 9, 9, 9)
    with pytest.raises(ValueError):
        loop_product(bad, plaquettes()[0])
    with pytest.raises(ValueError):
        lattice_links(1)
    with pytest.raises(ValueError):
        plaquettes(0)
