"""Slice 32: full-walk composition to termination + whole-walk roundtrip.

Production bar: every w-walk at p=13 (all 12) composes to termination in
fresh banks (L+1 banks for L steps — straight-line, intermediates persist,
zero history), and reverses whole (newest-first uncompute, all banks but
the head back to zero). Terminal banks decode to the model terminal with
g == 0, and the divwalk inverse anchors w against the prime. Composition
adds no overhead: Lx792 forward / Lx1584 roundtrip exactly. The pool-letter
collision (bank "F" silently replacing the F register) stays pinned by the
bank-width assertion below.
"""

import random

import pytest

from kubona.codesign import (
    BANKS,
    N_QUBITS_W,
    REG,
    KickmixEvaluator,
    build_dwalk_full,
    build_dwalk_full_roundtrip,
    dwalk_full_roundtrip_spec,
    dwalk_full_spec,
    from_bits,
    to_bits,
)
from kubona.codesign.divwalk import run_walk, step

LENGTHS = {1: 7, 2: 7, 3: 7, 4: 7, 5: 7, 6: 6,
           7: 8, 8: 7, 9: 8, 10: 8, 11: 8, 12: 8}


def _init_s0(w):
    init = {q: 0 for q in range(N_QUBITS_W)}
    for v, k in zip((0, 13, w, 0, 1), ("Z", "F", "G", "FV", "GV")):
        for i, bit in enumerate(to_bits(v, len(REG[k]))):
            init[REG[k][i]] = bit
    return init


def test_bank_pool_layout():
    assert len(BANKS) == 10
    for b in BANKS:
        assert ({k: len(v) for k, v in b.items()} ==
                {"Z": 5, "F": 6, "G": 6, "FV": 7, "GV": 7})
    assert N_QUBITS_W == 437


def test_full_walks_prove_termination():
    ev = KickmixEvaluator()
    for w, L in LENGTHS.items():
        (b, regs) = build_dwalk_full(w)
        assert regs == {"ccx": L * 792, "nsteps": L,
                        "history_qubits": 0, "w": w}
        res = ev.check_one(b, dwalk_full_spec(w), _init_s0(w),
                           random.Random(0))
        assert res.valid, (w, res.reason)
        s = (0, 13, w, 0, 1)
        for _ in range(L):
            s = step(s)[0]
        term = tuple(from_bits([res.final_bits.get(q, 0) for q in BANKS[L][k]])
                     for k in ("Z", "F", "G", "FV", "GV"))
        assert term == s and term[2] == 0
        assert (w * run_walk(13, w)["inverse"]) % 13 == 1


def test_full_walk_roundtrips_clear():
    ev = KickmixEvaluator()
    for w, L in LENGTHS.items():
        (b, regs) = build_dwalk_full_roundtrip(w)
        assert regs == {"ccx": L * 1584, "nsteps": L,
                        "history_qubits": 0, "w": w}
        res = ev.check_one(b, dwalk_full_roundtrip_spec(w), _init_s0(w),
                           random.Random(0))
        assert res.valid, (w, res.reason)


def test_walk_domain_refuses():
    for bad in (0, 13, -1, True, "1"):
        with pytest.raises(ValueError):
            build_dwalk_full(bad)
        with pytest.raises(ValueError):
            build_dwalk_full_roundtrip(bad)
