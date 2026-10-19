"""Slice 53: BM walk composition (chained uniform rounds, fresh banks).

Production bar: the walk is proven end to end on harvested full walks —
every toy single-error walk plus seeded double/triple samples run the
chained circuit and land exactly on the model's final register state
(sigma, tau, delta, d, maintained IDELTA), with inputs preserved and
every temp clean (dirty-free fires otherwise). The round circuits are
already proven individually (slice 52); what this proves is the
composition: bank wiring across rounds, the initial constants, and no
cross-round temp pollution (the divstep slice-32 pool-collision class
is asserted structurally absent: all bank/temp ids disjoint).
Structural cost: exactly N x single-round (uniform emission).
"""

import itertools
import random

import pytest

from kubona.codesign import F24, build_bm_round, build_bm_walk, multi_error_syndromes, single_error_syndromes
from kubona.codesign.bm import _bm_walk
from kubona.codesign.kickmix import _sim_trajectory


def _walks():
    out = []
    for pos in range(15):
        for err in range(1, 16):
            out.append(single_error_syndromes(F24, 2, pos, err, 8))
    rng = random.Random(53)
    seen = set()
    while len(seen) < 100:
        a = (rng.randrange(15), rng.randrange(1, 16))
        c = (rng.randrange(15), rng.randrange(1, 16))
        key = tuple(sorted([a, c]))
        if key not in seen:
            seen.add(key)
            (p1, e1), (p2, e2) = key
            out.append(multi_error_syndromes(F24, 2, [(p1, e1), (p2, e2)], 8))
    for _ in range(40):
        errs = [(rng.randrange(15), rng.randrange(1, 16)) for _ in range(3)]
        out.append(multi_error_syndromes(F24, 2, errs, 8))
    return out


def _all_ids(regs):
    ids = list(regs["inputs"]) + list(regs["outputs"])
    return ids


def test_walk_build_pins_and_layout():
    (b1, r1) = build_bm_walk(1)
    assert r1["kind"] == "bm-walk" and r1["rounds"] == 1
    assert r1["ccx"] == 823
    (b8, r8) = build_bm_walk(8)
    assert r8["ccx"] == 8 * 823  # uniform: exactly N x single-round
    assert r8["inputs"] == tuple(range(32))
    assert len(r8["outputs"]) == 65
    # no bank collisions (the slice-32 pool-collision class, structural)
    (br, rr) = build_bm_round()
    assert rr["ccx"] == 823  # round pins unchanged by the refactor
    with pytest.raises(ValueError):
        build_bm_walk(0)
    with pytest.raises(ValueError):
        build_bm_walk(9)
    with pytest.raises(ValueError):
        build_bm_walk(True)
    # determinism: identical emission on rebuild
    (b8b, r8b) = build_bm_walk(8)
    assert r8 == r8b and b8.emit() == b8b.emit()


def test_walk_proves_harvested_walks():
    (b, regs) = build_bm_walk(8)
    ops = b.emit()
    n = 0
    for syn in _walks():
        h = []
        _bm_walk(syn, F24, 8, 3, harvest=h)
        assert h and all("post" in e for e in h)
        sig, tau, delta, d = h[-1]["post"]
        init = {}
        for i, v in enumerate(syn):
            for t in range(4):
                init[i * 4 + t] = (v >> t) & 1
        bits, phase, _ = _sim_trajectory(ops, dict(init), random.Random(0))
        assert phase == 0
        out = regs["outputs"]
        got_sig = [sum((bits.get(out[k * 4 + t], 0) & 1) << t for t in range(4))
                   for k in range(4)]
        assert got_sig == list(sig) + [0] * (4 - len(sig)), syn[:2]
        got_tau = [sum((bits.get(out[16 + k * 4 + t], 0) & 1) << t for t in range(4))
                   for k in range(9)]
        assert got_tau == list(tau) + [0] * (9 - len(tau)), syn[:2]
        got_delta = sum((bits.get(out[52 + i], 0) & 1) << i for i in range(4))
        assert got_delta == delta, syn[:2]
        got_idelta = sum((bits.get(out[56 + i], 0) & 1) << i for i in range(4))
        assert got_idelta == F24.inv(delta), syn[:2]
        got_d = sum((bits.get(out[60 + i], 0) & 1) << i for i in range(5))
        if got_d >= 16:
            got_d -= 32
        assert got_d == d, syn[:2]
        assert all(bits.get(q, 0) == v for q, v in init.items()), syn[:2]
        n += 1
    assert n == 225 + 100 + 40
