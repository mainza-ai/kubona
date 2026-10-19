"""Slice 31: history-free decoded divstep walk (forward + decode + reversal).

Production bar: the naive step persists 2 history qubits per transition;
the decoded walk re-derives every decision bit from the next state, so
nothing persists. Four proofs over the swept domain: the decode recipe as
gates (43T), the decoded forward step (792T, same core as naive, no
history), the 2-step walk into fresh banks (1584T = 2x792, composition
adds no overhead), and decode-reverse from N alone (1777T, N canceled,
source restored). Every earlier mistarget — early flag clearing, add/sub
mirror swaps, halve-vs-copy, fvp-vs-fvp/2, ping-pong bank reuse — stays
pinned by these proofs.
"""

import random

import pytest

from kubona.codesign import (
    KickmixEvaluator,
    N_QUBITS_W,
    REG,
    build_decode_flags,
    build_dstep_decode_reverse,
    build_dstep_decoded,
    build_dwalk2,
    decode_decisions,
    decode_flags_spec,
    decode_reverse_spec,
    dstep_decoded_spec_fwd,
    dwalk2_spec,
    fit_divstep_inverse,
    fit_divstep_state,
    harvest_states,
    to_bits,
)
from kubona.codesign.divsteps import decoder_poly_derived
from kubona.codesign.divwalk import step
from kubona.codesign.dwalk import _pack_next


def _pack_bank(state, bank):
    init = {}
    for v, k in zip(state, ("Z", "F", "G", "FV", "GV")):
        for i, bit in enumerate(to_bits(v, len(bank[k]))):
            init[bank[k][i]] = bit
    return init


def test_recipe_units_and_refusals():
    # toy-prime rules are prime-specific (2/3 terms, not the paper's 3/3).
    assert len(decoder_poly_derived(2, p=13)) == 2
    assert len(decoder_poly_derived(3, p=13)) == 3
    # swap next: s visible, b forced; zero-rule and z-rule next-states.
    assert decode_decisions((0, 1, 0, 2, 1)) == {"s": 1, "b": 1}
    assert decode_decisions((0, -1, 0, -12, -13)) == {"s": 0, "b": 1}
    assert decode_decisions((1, 1, -3, 4, 1)) == {"s": 0, "b": 0}
    with pytest.raises(ValueError):
        decode_decisions((0, 1))  # not a 5-tuple
    with pytest.raises(ValueError):
        # ambiguous shape (s=0, gp!=0, zp<=0) at depth 4: outside proven 2-3.
        decode_decisions((-1, 1, 1, 16, 1))
    with pytest.raises(ValueError):
        fit_divstep_inverse((0, 1, 2, 3))  # not a state (forward fit refuses)


def test_decode_flags_proves_recipe():
    states = harvest_states()
    (b, regs) = build_decode_flags()
    assert regs == {"ccx": 43, "temps": 16}
    spec = decode_flags_spec()
    ev = KickmixEvaluator()
    nxt = [fit_divstep_state(s)["next"] for s in states]
    for n in nxt:
        init = {q: 0 for q in range(49)}
        init.update(_pack_bank(n, {"Z": list(range(0, 5)), "F": list(range(5, 11)),
                                    "G": list(range(11, 17)), "FV": list(range(17, 24)),
                                    "GV": list(range(24, 31))}))
        res = ev.check_one(b, spec, init, random.Random(0))
        assert res.valid, (n, res.reason)


def test_decoded_forward_proves_step():
    states = harvest_states()
    (b, regs) = build_dstep_decoded()
    assert regs["ccx"] == 792 and regs["history_qubits"] == 0
    spec = dstep_decoded_spec_fwd()
    ev = KickmixEvaluator()
    for s in states:
        init = {q: 0 for q in range(N_QUBITS_W)}
        for v, k in zip(s, ("Z", "F", "G", "FV", "GV")):
            for i, bit in enumerate(to_bits(v, len(REG[k]))):
                init[REG[k][i]] = bit
        res = ev.check_one(b, spec, init, random.Random(0))
        assert res.valid, (s, res.reason)


def test_walk2_proves_composition():
    (b, regs) = build_dwalk2()
    assert regs == {"ccx": 1584, "nsteps": 2, "history_qubits": 0}
    spec = dwalk2_spec()
    ev = KickmixEvaluator()
    for w in range(1, 13):
        s0 = (0, 13, w, 0, 1)
        for st in (s0, step(s0)[0], step(step(s0)[0])[0]):
            fit_divstep_state(st)  # all three walk states fit (12/12 pins it)
        init = {q: 0 for q in range(N_QUBITS_W)}
        for v, k in zip(s0, ("Z", "F", "G", "FV", "GV")):
            for i, bit in enumerate(to_bits(v, len(REG[k]))):
                init[REG[k][i]] = bit
        res = ev.check_one(b, spec, init, random.Random(0))
        assert res.valid, (w, res.reason)


def test_decode_reverse_proves_history_free_reversal():
    states = harvest_states()
    for s in states:
        fit_divstep_inverse(s)  # inverse arithmetic fits + recovers (88/88)
    (b, regs) = build_dstep_decode_reverse()
    assert regs["ccx"] == 1777 and regs["history_qubits"] == 0
    spec = decode_reverse_spec()
    assert spec.inputs == ()  # consuming circuit: B ends zeroed, honestly declared
    ev = KickmixEvaluator()
    nxt = [fit_divstep_state(s)["next"] for s in states]
    for n in nxt:
        init = {q: 0 for q in range(N_QUBITS_W)}
        init.update(_pack_next(n))
        res = ev.check_one(b, spec, init, random.Random(0))
        assert res.valid, (n, res.reason)
