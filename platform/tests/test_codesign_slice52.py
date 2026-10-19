"""Slice 52: reversible toy BM round circuit (harvested-state proof).

Production bar: the round circuit is proven over harvested valid-walk
states (the divstep discipline) — every unique (sigma, tau, delta, d,
window) state from all single-error walks, all double-error walks, and
a seeded triple sample simulates to exactly the model's post-state,
with inputs preserved and every temp clean (dirty-free raises
otherwise). Survey pins bound the registers structurally (tau <= 9 by
round count, |d| <= 8, sigma deg <= 3 by the walk gate); out-of-scope
inputs are unspecified, same contract as the divstep step. New
machinery: Fermat inverter emitter (66T, matches the paper's trial
Fermat) and 5-bit increment/negate (6T), each proven exhaustively
standalone. Measured round cost pinned (851T); the paper's BM circuit
is the scale-up, queued.
"""

import itertools
import random

import pytest

from kubona.codesign import F24, multi_error_syndromes, single_error_syndromes
from kubona.codesign.bm import _bm_walk, _emit_incr5, _emit_neg5, build_bm_round
from kubona.codesign.f24 import emit_fermat_inv
from kubona.codesign.kickmix import Builder, _sim_trajectory


def _harvest(walks):
    states = []
    refused = 0
    for syn in walks:
        h = []
        _bm_walk(syn, F24, 8, 3, harvest=h)
        for e in h:
            if "post" not in e:
                refused += 1
                continue
            states.append(e)
    return states, refused


def _walks():
    out = []
    for pos in range(15):
        for err in range(1, 16):
            out.append(single_error_syndromes(F24, 2, pos, err, 8))
    pairs = list(itertools.combinations_with_replacement(
        [(p, e) for p in range(15) for e in range(1, 16)], 2))
    for (p1, e1), (p2, e2) in pairs:
        out.append(multi_error_syndromes(F24, 2, [(p1, e1), (p2, e2)], 8))
    rng = random.Random(52)
    for _ in range(200):
        errs = [(rng.randrange(15), rng.randrange(1, 16)) for _ in range(3)]
        out.append(multi_error_syndromes(F24, 2, errs, 8))
    return out


def test_survey_envelope_and_no_refusals():
    states, refused = _harvest(_walks())
    assert refused == 0  # every toy 1/2/3-error walk completes in range
    assert len(states) == (225 + 25425 + 200) * 8
    max_tau = max(max(len(e["pre"][1]), len(e["post"][1])) for e in states)
    max_sig = max(max(len(e["pre"][0]), len(e["post"][0])) for e in states)
    ds = [x for e in states for x in (e["pre"][3], e["post"][3])]
    assert max_tau <= 9 and max_sig <= 4  # registers never overflow
    assert min(ds) >= -8 and max(ds) <= 8  # 5-bit two's complement holds
    cases = {e["case"] for e in states}
    assert cases == {"replace", "zero", "retain"}  # every path exercised


def _init_for(e):
    sig, tau, delta, d = e["pre"]
    syn, r = e["syn"], e["r"]
    W = [syn[r - 1 - i] if r - 1 - i >= 0 else 0 for i in range(4)]
    assert delta != 0  # model invariant sigma0*delta == 1
    init = {}
    for k in range(4):
        c = sig[k] if k < len(sig) else 0
        for i in range(4):
            init[k * 4 + i] = (c >> i) & 1
    for k in range(9):
        c = tau[k] if k < len(tau) else 0
        for i in range(4):
            init[16 + k * 4 + i] = (c >> i) & 1
    for i in range(4):
        init[52 + i] = (delta >> i) & 1
    inv = F24.inv(delta)
    for i in range(4):
        init[56 + i] = (inv >> i) & 1
    dd = d & 31
    for i in range(5):
        init[60 + i] = (dd >> i) & 1
    for k in range(4):
        for i in range(4):
            init[65 + k * 4 + i] = (W[k] >> i) & 1
    return init, W


def _dedup(states):
    uniq = {}
    for e in states:
        sig, tau, delta, d = e["pre"]
        syn, r = e["syn"], e["r"]
        W = tuple(syn[r - 1 - i] if r - 1 - i >= 0 else 0 for i in range(4))
        uniq[(tuple(sig), tuple(tau), delta, d, W)] = e
    return uniq


def test_round_proves_harvested_states():
    (b, regs) = build_bm_round()
    assert regs["kind"] == "bm-round"
    assert regs["ccx"] == 823  # shifted retain-scale (3 mults, not 4)
    assert len(regs["inputs"]) == 81 and len(regs["outputs"]) == 65
    states, refused = _harvest(_walks())
    assert refused == 0
    uniq = _dedup(states)
    cases = {e["case"] for e in uniq.values()}
    assert cases == {"replace", "zero", "retain"}
    ops = b.emit()
    for key, e in uniq.items():
        init, _W = _init_for(e)
        bits, phase, _ = _sim_trajectory(ops, dict(init), random.Random(0))
        assert phase == 0
        sig, tau, _d, _dd = e["post"]
        want_sig = list(sig) + [0] * (4 - len(sig))
        want_tau = list(tau) + [0] * (9 - len(tau))
        got_sig = [sum((bits.get(245 + k * 4 + i, 0) & 1) << i for i in range(4))
                   for k in range(4)]
        got_tau = [sum((bits.get(261 + k * 4 + i, 0) & 1) << i for i in range(4))
                   for k in range(9)]
        assert got_sig == want_sig, (key, e["case"])
        assert got_tau == want_tau, (key, e["case"])
        base = regs["outputs"][0]
        assert [bits.get(base + 52 + i, 0) & 1 for i in range(4)] == \
            [(e["post"][2] >> i) & 1 for i in range(4)], key
        assert [bits.get(base + 56 + i, 0) & 1 for i in range(4)] == \
            [(F24.inv(e["post"][2]) >> i) & 1 for i in range(4)], key
        got_d = sum((bits.get(base + 60 + i, 0) & 1) << i for i in range(5))
        if got_d >= 16:
            got_d -= 32
        assert got_d == e["post"][3], key
        assert all(bits.get(q, 0) == v for q, v in init.items()), key


def test_fermat_emitter_exhaustive():
    from kubona.codesign import f24_inversion_table
    from kubona.codesign.kickmix import Builder
    tab = f24_inversion_table()
    tot = 0
    for x in range(16):
        b = Builder(24)
        for q in list(range(4, 24)):
            b.alloc(q)
        n = [0]
        n[0] += emit_fermat_inv(b, [0, 1, 2, 3], [4, 5, 6, 7],
                                list(range(8, 24)))
        assert n[0] == 66
        tot += n[0]
        init = {i: (x >> i) & 1 for i in range(4)}
        bits, _, _ = _sim_trajectory(b.emit(), dict(init), random.Random(1))
        got = sum((bits.get(4 + i, 0) & 1) << i for i in range(4))
        assert got == tab[x], x
        for q in range(8, 24):
            assert bits.get(q, 0) == 0, (x, q)  # temps clean
        for q in range(8, 24):
            b.free(q)
    assert tot == 16 * 66
    with pytest.raises(ValueError):
        emit_fermat_inv(Builder(24), [0, 1, 2], [4, 5, 6, 7], list(range(8, 24)))
    with pytest.raises(ValueError):
        emit_fermat_inv(Builder(24), [0, 1, 2, 3], [4, 5, 6, 7], list(range(8, 23)))


def test_incr_neg_exhaustive():
    from kubona.codesign.kickmix import Builder
    for name, fn, want in (("incr", _emit_incr5, lambda v: (v + 1) % 32),
                           ("neg", _emit_neg5, lambda v: (-v) % 32)):
        for v in range(32):
            b = Builder(9)
            d, c = [0, 1, 2, 3, 4], [5, 6, 7, 8]
            for q in c:
                b.alloc(q)
            init = {i: (v >> i) & 1 for i in range(5)}
            n = [0]
            fn(b, n, d, c)
            for q in c:
                b.free(q)
            bits, _, _ = _sim_trajectory(b.emit(), dict(init), random.Random(2))
            assert sum((bits.get(i, 0) & 1) << i for i in range(5)) == want(v)
            assert n[0] == 6
