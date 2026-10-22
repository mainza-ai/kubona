"""Slice 54: F2^12 field arithmetic circuits (schoolbook baseline).

Production bar: the REDUCE table is verified against gf arithmetic on
all 144 basis products; the multiplier's emitted ops are verified
against the table (exact multiset match — no sampling, no trust in the
emission loops); squaring is basis-checked on all 12 units; the Fermat
inverter is proven exhaustive on all 4096 inputs (zero maps cleanly to
zero). Measured: 363T per multiply (structural: one CCX per
(i,j,triple)), 6897T per inversion (19 mults + free squarings). This is
the paper's starting-point family at F2^12 (Karatsuba/tower/MBU
variants queued, same as f24's family comparison); the half-degree
EEA pass builds on exactly these two emitters next.
"""

import random

import pytest

from kubona.codesign import F2_12
from kubona.codesign.f12 import REDUCE_12, emit_f12_mul, emit_f12_sqr, emit_fermat_inv12
from kubona.codesign.kickmix import Builder, _sim_trajectory


def test_reduce_table_matches_gf():
    for i in range(12):
        for j in range(12):
            want = F2_12.mul(1 << i, 1 << j)
            got = 0
            for t in REDUCE_12[i + j]:
                got ^= 1 << t
            assert got == want, (i, j)


def test_emit_mul_matches_table_exactly():
    # every emitted CCX is checked against the table (not the loop):
    # a structural multiset equality over all 144 basis products.
    # (No allocs needed: pure gates, inputs read, out assumed zeroed.)
    b = Builder(36)
    n = emit_f12_mul(b, list(range(12)), list(range(12, 24)), list(range(24, 36)))
    got = sorted((o.qubits[0], o.qubits[1], o.qubits[2])
                 for o in b.emit() if o.name == "ccx")
    want = []
    for i in range(12):
        for j in range(12):
            for t in REDUCE_12[i + j]:
                want.append((i, 12 + j, 24 + t))
    assert sorted(want) == got
    assert n == len(want) == 363
    with pytest.raises(ValueError):
        emit_f12_mul(b, [0, 1, 2], list(range(12, 24)), list(range(24, 36)))


def test_sqr_basis_and_random():
    rng = random.Random(54)
    for x in [1 << i for i in range(12)] + [rng.randrange(4096) for _ in range(8)]:
        b = Builder(24)
        init = {i: (x >> i) & 1 for i in range(12)}
        emit_f12_sqr(b, list(range(12)), list(range(12, 24)))
        bits, _, _ = _sim_trajectory(b.emit(), dict(init), random.Random(1))
        got = sum((bits.get(12 + i, 0) & 1) << i for i in range(12))
        assert got == F2_12.mul(x, x), x


def test_fermat_inverter_exhaustive():
    n_tot = [0]
    for x in range(4096):
        b = Builder(12 + 12 + 240)
        for q in range(24, 264):
            b.alloc(q)
        n = [0]
        n[0] += emit_fermat_inv12(b, list(range(12)), list(range(12, 24)), list(range(24, 264)))
        assert n[0] == 6897
        n_tot[0] += n[0]
        init = {i: (x >> i) & 1 for i in range(12)}
        for q in range(24, 264):
            init[q] = 0
        bits, phase, _ = _sim_trajectory(b.emit(), dict(init), random.Random(2))
        assert phase == 0
        got = sum((bits.get(12 + i, 0) & 1) << i for i in range(12))
        assert got == (F2_12.inv(x) if x else 0), x
        for q in range(24, 264):
            assert bits.get(q, 0) == 0, (x, q)
        for q in range(24, 264):
            b.free(q)
    assert n_tot[0] == 4096 * 6897
    # determinism: identical emission on rebuild
    b1, b2 = Builder(264), Builder(264)
    for b in (b1, b2):
        for q in range(24, 264):
            b.alloc(q)
        emit_fermat_inv12(b, list(range(12)), list(range(12, 24)), list(range(24, 264)))
    assert b1.emit() == b2.emit()


def test_fermat_refusals():
    b = Builder(264)
    with pytest.raises(ValueError):
        emit_fermat_inv12(b, list(range(11)), list(range(12, 24)), list(range(24, 264)))
    with pytest.raises(ValueError):
        emit_fermat_inv12(b, list(range(12)), list(range(12, 24)), list(range(24, 263)))
    with pytest.raises(ValueError):
        emit_fermat_inv12(b, list(range(12)), list(range(12, 24)), list(range(12, 252)))
    with pytest.raises(ValueError):
        emit_fermat_inv12(b, list(range(12)), list(range(12, 24)), [300] * 240)
