"""Slice 43: divstep at p=17 (prime-agnostic pipeline, second prime).

Production bar: the SAME step circuit (792T fwd / 1584T round-trip, zero
history decoded) proves 128/128 harvested states at a second prime with
zero gate changes — only the prime-specific recipe changes (derived, not
transcribed): depths 2-3 at p=13 become 2-4 at p=17, with the depth-4
q3 bit read from the f'v sign (exact on fit-gated values, swept-proven).
SUMA-width fit fix included (g+-f live in 8-bit SUMA, not 6-bit G —
the old check refused valid p=17 inputs; p=13 behavior unchanged).
"""

import random

import pytest

from kubona.codesign import (
    KickmixEvaluator,
    N_QUBITS,
    N_QUBITS_W,
    REG,
    build_decode_flags,
    build_dstep,
    build_dstep_decode_reverse,
    build_dstep_decoded,
    build_dwalk2,
    build_dwalk_full,
    build_dwalk_full_roundtrip,
    decode_decisions,
    decode_flags_spec,
    decode_reverse_spec,
    dstep_decoded_spec_fwd,
    dstep_spec_fwd,
    dstep_spec_rt,
    dwalk2_spec,
    dwalk_full_roundtrip_spec,
    dwalk_full_spec,
    fit_divstep_inverse,
    fit_divstep_state,
    from_bits,
    harvest_states,
    to_bits,
)
from kubona.codesign.divsteps import decoder_poly_derived
from kubona.codesign.divwalk import run_walk, step
from kubona.codesign.dstep import _pack_state
from kubona.codesign.dwalk import BANKS, _pack_next

P, DD = 17, (2, 3, 4)
LENGTHS = {1: 9, 2: 9, 3: 8, 4: 9, 5: 7, 6: 7, 7: 8, 8: 9,
           9: 8, 10: 8, 11: 8, 12: 8, 13: 7, 14: 8, 15: 6, 16: 9}


def _pack_S(s0):
    init = {q: 0 for q in range(N_QUBITS_W)}
    for v, k in zip(s0, ("Z", "F", "G", "FV", "GV")):
        for i, bit in enumerate(to_bits(v, len(REG[k]))):
            init[REG[k][i]] = bit
    return init


def test_p17_rules_derived_and_fit_covers():
    assert sorted(map(sorted, decoder_poly_derived(2, p=17))) == [["f1"], ["q1"]]
    assert sorted(map(sorted, decoder_poly_derived(3, p=17))) == [["f2"], ["q2"]]
    assert len(decoder_poly_derived(4, p=17)) == 10
    from collections import Counter
    states = harvest_states(P)
    assert len(states) == 128
    branches = Counter(fit_divstep_state(s, P, DD)["branch"] for s in states)
    assert branches == {"swap": 58, "even": 52, "odd": 18}
    assert to_bits(-3, 6) == [1, 0, 1, 1, 1, 1]
    assert from_bits([1, 0, 1, 1, 1, 1]) == -3
    with pytest.raises(ValueError):
        fit_divstep_state((0, 1, 2, 3))
    with pytest.raises(ValueError):
        fit_divstep_state((0, 17, 1, 0, 1), 16)
    with pytest.raises(ValueError):
        fit_divstep_state((-1, -1, -1, 8, -9), 17, (2, 3))  # depth 4 outside
    assert fit_divstep_state((-1, -1, -1, 8, -9), 17, (2, 3, 4))["b"] == 1


def test_p17_recipe_units_and_refusals():
    assert decode_decisions((0, 1, 0, 2, 1), P, DD) == {"s": 1, "b": 1}
    assert decode_decisions((0, -1, 0, -12, -13), P, DD) == {"s": 0, "b": 1}
    assert decode_decisions((1, 1, -3, 4, 1), P, DD) == {"s": 0, "b": 0}
    with pytest.raises(ValueError):
        decode_decisions((0, 1))
    with pytest.raises(ValueError):
        decode_decisions((-1, 1, 1, 32, 1), P, DD)  # ambiguous depth 5
    with pytest.raises(ValueError):
        fit_divstep_inverse((0, 1, 2, 3))


def test_p17_naive_forward_and_roundtrip():
    states = harvest_states(P)
    (b, regs) = build_dstep()
    assert regs["ccx"] == 792 and regs["history_qubits"] == 2
    spec = dstep_spec_fwd()
    ev = KickmixEvaluator()
    for s in states:
        init = {q: 0 for q in range(N_QUBITS)}
        init.update(_pack_state(s))
        res = ev.check_one(b, spec, init, random.Random(0))
        assert res.valid, (s, res.reason)
    (b2, regs2) = build_dstep(roundtrip=True)
    assert regs2["ccx"] == 1584 and regs2["history_qubits"] == 0
    spec2 = dstep_spec_rt()
    for s in states:
        init = {q: 0 for q in range(N_QUBITS)}
        init.update(_pack_state(s))
        res = ev.check_one(b2, spec2, init, random.Random(0))
        assert res.valid, (s, res.reason)


def test_p17_decoded_forward_and_flags():
    states = harvest_states(P)
    (b, regs) = build_dstep_decoded()
    assert regs["ccx"] == 792 and regs["history_qubits"] == 0
    spec = dstep_decoded_spec_fwd()
    ev = KickmixEvaluator()
    for s in states:
        assert ev.check_one(b, spec, _pack_S(s), random.Random(0)).valid, s
    (bf, rf) = build_decode_flags(P, DD)
    assert rf == {"ccx": 89, "temps": 17}
    specf = decode_flags_spec(P, DD)
    nxt = [fit_divstep_state(s, P, DD)["next"] for s in states]
    NZ = list(range(0, 5)); NF = list(range(5, 11)); NG = list(range(11, 17))
    NFV = list(range(17, 24)); NGV = list(range(24, 31))
    for n in nxt:
        init = {q: 0 for q in range(33 + rf["temps"])}
        for v, qq in zip(n, (NZ, NF, NG, NFV, NGV)):
            for i, bit in enumerate(to_bits(v, len(qq))):
                init[qq[i]] = bit
        assert ev.check_one(bf, specf, init, random.Random(0)).valid, n


def test_p17_walk2_prefixes():
    (b, regs) = build_dwalk2()
    assert regs == {"ccx": 1584, "nsteps": 2, "history_qubits": 0}
    spec = dwalk2_spec()
    ev = KickmixEvaluator()
    n = 0
    for w in range(1, P):
        s0 = (0, P, w, 0, 1)
        for st in (s0, step(s0)[0], step(step(s0)[0])[0]):
            fit_divstep_state(st, P, DD)
        assert ev.check_one(b, spec, _pack_S(s0), random.Random(0)).valid, w
        n += 1
    assert n == 16


def test_p17_full_walks_and_roundtrips():
    ev = KickmixEvaluator()
    for w, L in LENGTHS.items():
        (b, regs) = build_dwalk_full(w, P, DD)
        assert regs == {"ccx": L * 792, "nsteps": L,
                        "history_qubits": 0, "w": w}
        s0 = (0, P, w, 0, 1)
        res = ev.check_one(b, dwalk_full_spec(w, P, DD), _pack_S(s0), random.Random(0))
        assert res.valid, (w, res.reason)
        (b2, regs2) = build_dwalk_full_roundtrip(w, P, DD)
        assert regs2 == {"ccx": L * 1584, "nsteps": L,
                         "history_qubits": 0, "w": w}
        res2 = ev.check_one(b2, dwalk_full_roundtrip_spec(w, P, DD), _pack_S(s0), random.Random(0))
        assert res2.valid, (w, res2.reason)
        rep = run_walk(P, w)
        assert rep["active_steps"] == L and (w * rep["inverse"]) % P == 1
        s = s0
        for _ in range(L):
            s = step(s)[0]
        term = tuple(from_bits([res.final_bits.get(q, 0) for q in BANKS[L][k]])
                     for k in ("Z", "F", "G", "FV", "GV"))
        assert term == s and term[2] == 0


def test_p17_decode_reverse():
    states = harvest_states(P)
    for s in states:
        fit_divstep_inverse(s, P, DD)
    (b, regs) = build_dstep_decode_reverse(P, DD)
    assert regs["ccx"] == 1823 and regs["history_qubits"] == 0  # +46 vs p=13: depth-4 rule + flag
    spec = decode_reverse_spec(P, DD)
    assert spec.inputs == ()
    ev = KickmixEvaluator()
    nxt = [fit_divstep_state(s, P, DD)["next"] for s in states]
    for n in nxt:
        init = {q: 0 for q in range(N_QUBITS_W)}
        init.update(_pack_next(n))
        assert ev.check_one(b, spec, init, random.Random(0)).valid, n
    with pytest.raises(ValueError):
        build_dwalk_full(0, P, DD)
    with pytest.raises(ValueError):
        decode_decisions((0, 1, 2, 3), P, DD)
