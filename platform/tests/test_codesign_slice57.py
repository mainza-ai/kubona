"""Slice 57: paper-scale BM round circuit (Phase 2 B1a).

Production bar: the single round (F2^12, 36-coeff sigma, 71-word tau bank,
8-bit d counter, tower ainv + schoolbook mults) is proven over harvested
valid-walk states — every round of 8 planted walks (3 single-error, 3
double-error, 2 seeded triple-error), each simulating to exactly the
model's post-state with inputs preserved and every temp clean. One circuit
build, 560 sims (uniform round: only the init differs per round). Survey
over 200 walks (math only) bounds the registers structurally (tau <= 71
by round count — the bank is exact, never truncating; |d| <= 70 fits
8-bit two's complement; sigma deg <= 35 by the model gate) and confirms
all three cases occur; out-of-scope inputs are unspecified, same contract
as the toy. New machinery: 8-bit increment/negate (12T), proven
exhaustive both ways. Measured round cost pinned (82344T); the 70-round
walk with in-place bank discipline is the next slice (B1b).
"""

import itertools
import random

import pytest

from kubona.codesign import F2_12
from kubona.codesign.bm import (
    _bm12_round_layout,
    _bm_walk,
    _emit_bm12_round,
    _emit_incr8,
    _emit_neg8,
    build_bm12_round,
    multi_error_syndromes,
)
from kubona.codesign.kickmix import Builder, _sim_trajectory

GAMMA = 2  # primitive element (modulus is primitive): distinct positions
NROUND = 70


def _walks():
    rng = random.Random(57)
    out = []
    for pos in (0, 1, 100, 1000, 2048, 4094):
        out.append(multi_error_syndromes(F2_12, GAMMA, [(pos, 1 + rng.randrange(4095))], NROUND))
    for _ in range(60):
        errs = [(rng.randrange(4096), 1 + rng.randrange(4095)) for _ in range(2)]
        out.append(multi_error_syndromes(F2_12, GAMMA, errs, NROUND))
    for _ in range(134):
        errs = [(rng.randrange(4096), 1 + rng.randrange(4095)) for _ in range(3)]
        out.append(multi_error_syndromes(F2_12, GAMMA, errs, NROUND))
    return out


def _harvest(syn):
    h = []
    _bm_walk(syn, F2_12, 80, 35, harvest=h)
    return [e for e in h if "post" in e]


def test_incr8_neg8_exhaustive():
    for fn, want_fn, cost in ((_emit_incr8, lambda v: (v + 1) & 0xFF, 12),
                             (_emit_neg8, lambda v: (-v) & 0xFF, 12)):
        # forward correctness + pin, exhaustive over all 256 inputs
        for v in range(256):
            b = Builder(15)
            for q in range(8, 15):
                b.alloc(q)
            n = [0]
            fn(b, n, list(range(8)), list(range(8, 15)))
            assert n[0] == cost, fn.__name__
            init = {i: (v >> i) & 1 for i in range(8)}
            for q in range(8, 15):
                init[q] = 0
            bits, phase, _ = _sim_trajectory(b.emit(), dict(init), random.Random(8))
            assert phase == 0, (fn.__name__, v)
            got = sum((bits.get(i, 0) & 1) << i for i in range(8))
            assert got == want_fn(v), (fn.__name__, v)
            for q in range(8, 15):
                assert bits.get(q, 0) == 0, (fn.__name__, v)
        # dagger inverts forward, exhaustive (forward-then-dagger identity)
        for v in range(256):
            b = Builder(15)
            for q in range(8, 15):
                b.alloc(q)
            n = [0]
            fn(b, n, list(range(8)), list(range(8, 15)))
            fn(b, n, list(range(8)), list(range(8, 15)), dagger=True)
            assert n[0] == 2 * cost, fn.__name__
            init = {i: (v >> i) & 1 for i in range(8)}
            for q in range(8, 15):
                init[q] = 0
            bits, _, _ = _sim_trajectory(b.emit(), dict(init), random.Random(9))
            assert sum((bits.get(i, 0) & 1) << i for i in range(8)) == v, (fn.__name__, v)


def test_survey_envelope_and_no_refusals():
    states = []
    refused = 0
    cases = set()
    for syn in _walks():
        h = []
        try:
            _bm_walk(syn, F2_12, 80, 35, harvest=h)
        except ValueError:
            refused += 1
            continue
        for e in h:
            if "post" not in e:
                refused += 1
                continue
            states.append(e)
            cases.add(e["case"])
    assert refused == 0  # every planted 1/2/3-error walk completes in range
    assert len(states) == 200 * NROUND
    max_tau = max(max(len(e["pre"][1]), len(e["post"][1])) for e in states)
    max_sig = max(max(len(e["pre"][0]), len(e["post"][0])) for e in states)
    ds = [x for e in states for x in (e["pre"][3], e["post"][3])]
    assert max_tau <= 71 and max_sig <= 36  # bank sizes are exact, never truncate
    assert min(ds) >= -70 and max(ds) <= 70  # 8-bit two's complement holds
    assert cases >= {"replace", "zero"}  # retain is rare: hunted separately below


def test_retain_hunt_and_envelope():
    # Retain needs d < 0 with nonzero discrepancy (a replace with d > 0 must
    # come first) — absent from 200 random 1/2/3-error walks, present in
    # high-error walks. Hunted deterministically (seed 99); the two walks
    # join the circuit-proof set so the FT-gated path is exercised.
    rng = random.Random(99)
    found = []
    for _ in range(300):
        ne = 1 + rng.randrange(40)
        errs = [(rng.randrange(4096), 1 + rng.randrange(4095)) for _ in range(ne)]
        syn = multi_error_syndromes(F2_12, GAMMA, errs, NROUND)
        h = []
        try:
            _bm_walk(syn, F2_12, 80, 35, harvest=h)
        except ValueError:
            continue
        ok = [e for e in h if "post" in e]
        if any(e["case"] == "retain" for e in ok):
            found.append((syn, ok))
            if len(found) >= 2:
                break
    assert len(found) == 2
    for _, states in found:
        assert max(max(len(e["pre"][1]), len(e["post"][1])) for e in states) <= 71
        assert max(max(len(e["pre"][0]), len(e["post"][0])) for e in states) <= 36
        ds = [x for e in states for x in (e["pre"][3], e["post"][3])]
        assert min(ds) >= -70 and max(ds) <= 70


def _load(e, L):
    sig, tau, delta, d = e["pre"]
    syn, r = e["syn"], e["r"]
    assert delta != 0  # model invariant sigma0*delta == 1
    W = [syn[r - 1 - i] if r - 1 - i >= 0 else 0 for i in range(36)]
    init = {}
    for k in range(36):
        c = sig[k] if k < len(sig) else 0
        for i in range(12):
            init[L["SIGMA"][12 * k + i]] = (c >> i) & 1
    for k in range(71):
        c = tau[k] if k < len(tau) else 0
        for i in range(12):
            init[L["TAU"][12 * k + i]] = (c >> i) & 1
    for i in range(12):
        init[L["DELTA"][i]] = (delta >> i) & 1
        init[L["IDELTA"][i]] = (F2_12.inv(delta) >> i) & 1
    for i in range(8):
        init[L["D"][i]] = ((d & 0xFF) >> i) & 1
    for k in range(36):
        for i in range(12):
            init[L["W"][12 * k + i]] = (W[k] >> i) & 1
    return init


def _check(e, L, bits):
    psig, ptau, pdelta, pd = e["post"]
    for k in range(36):
        want = psig[k] if k < len(psig) else 0
        got = sum((bits.get(L["OSIG"][12 * k + i], 0) & 1) << i for i in range(12))
        assert got == want, (e["r"], e["case"], k)
    for k in range(71):
        want = ptau[k] if k < len(ptau) else 0
        got = sum((bits.get(L["OTAU"][12 * k + i], 0) & 1) << i for i in range(12))
        assert got == want, (e["r"], e["case"], k)
    assert sum((bits.get(L["ODELTA"][i], 0) & 1) << i for i in range(12)) == pdelta
    assert sum((bits.get(L["OIDELTA"][i], 0) & 1) << i for i in range(12)) == F2_12.inv(pdelta)
    got_d = sum((bits.get(L["OD"][i], 0) & 1) << i for i in range(8))
    assert (got_d - 256 if got_d >= 128 else got_d) == pd


def test_round_harvested_walks():
    rng = random.Random(570)
    walks = []
    for pos in (0, 2048, 4094):
        walks.append(multi_error_syndromes(F2_12, GAMMA, [(pos, 1 + rng.randrange(4095))], NROUND))
    for _ in range(3):
        walks.append(multi_error_syndromes(
            F2_12, GAMMA, [(rng.randrange(4096), 1 + rng.randrange(4095)) for _ in range(2)], NROUND))
    for _ in range(2):
        walks.append(multi_error_syndromes(
            F2_12, GAMMA, [(rng.randrange(4096), 1 + rng.randrange(4095)) for _ in range(3)], NROUND))
    # retain-path coverage: two hunted walks (seed 99, same hunt as above)
    rng99 = random.Random(99)
    n_found = 0
    for _ in range(300):
        ne = 1 + rng99.randrange(40)
        errs = [(rng99.randrange(4096), 1 + rng99.randrange(4095)) for _ in range(ne)]
        syn = multi_error_syndromes(F2_12, GAMMA, errs, NROUND)
        h = []
        try:
            _bm_walk(syn, F2_12, 80, 35, harvest=h)
        except ValueError:
            continue
        ok = [e for e in h if "post" in e]
        if any(e["case"] == "retain" for e in ok):
            walks.append(syn)
            n_found += 1
            if n_found >= 2:
                break
    assert n_found == 2
    states = []
    for syn in walks:
        states.extend(_harvest(syn))
    assert len(states) == 10 * NROUND
    assert {e["case"] for e in states} == {"replace", "zero", "retain"}
    L = _bm12_round_layout(0)
    b = Builder(L["nqubits"])
    for q in L["allocs"]:
        b.alloc(q)
    n_ccx = [0]
    _emit_bm12_round(b, n_ccx, L)
    assert n_ccx[0] == 82344
    ops = b.emit()
    for e in states:
        init = _load(e, L)
        bits, phase, _ = _sim_trajectory(ops, dict(init), random.Random(10))
        assert phase == 0, (e["r"], e["case"])
        _check(e, L, bits)
        for q in L["allocs"]:
            assert bits.get(q, 0) == 0, (e["r"], e["case"], q)
        # inputs preserved (round reads, never writes, the input bank)
        for q in range(L["nin"]):
            assert bits.get(q, 0) == init.get(q, 0), (e["r"], e["case"], q)


def test_round_pins_determinism_layout():
    L = _bm12_round_layout(0)
    assert L["nqubits"] == 5407 and L["nin"] == 1748 and L["ntemps"] == 2343
    assert len(L["SIGMA"]) == 432 and len(L["TAU"]) == 852
    assert len(L["W"]) == 432 and len(L["OSIG"]) == 432 and len(L["OTAU"]) == 852
    # every region pairwise disjoint (bank-collision class), except the
    # alloc list which is the union of temp groups by construction; allocs
    # cover exactly the temp block (inputs/outputs never alloc'd)
    regions = {k: set(v) for k, v in L.items()
               if isinstance(v, list) and v and isinstance(v[0], int) and k != "allocs"}
    keys = list(regions)
    for a in range(len(keys)):
        for bb in range(a + 1, len(keys)):
            assert not (regions[keys[a]] & regions[keys[bb]]), (keys[a], keys[bb])
    assert set(L["allocs"]) == set().union(*[regions[k] for k in (
        "DISC", "FZ", "FR", "FT", "ALPHA", "AINV", "NID", "REPL", "RET",
        "TINCR", "TNEG", "CARRY", "TOWER", "RD", "TD", "DD", "IDD", "DDD")])
    b, meta = build_bm12_round()
    assert meta["ccx"] == 82344
    assert meta["kind"] == "bm12-round"
    b1, _ = build_bm12_round()
    assert b.emit() == b1.emit()
