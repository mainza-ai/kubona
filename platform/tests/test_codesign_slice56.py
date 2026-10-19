"""Slice 56: one-level Karatsuba F2^12 multiplier (Phase 1 C4) — NEGATIVE RESULT.

Production bar: three raw 6-bit schoolbook leaves (36 ANDs each, derived
not transcribed) + linear recombination + REDUCE_12 reduction (CNOTs);
self-contained (internal temps mirrored clean, answer persists). Measured
432T pinned (3x72T forward + 3x72T mirror). CORRECTED 2026-10-19: the
"1.68x all-in" first claimed here is invalid — kara-forward-only (216T)
leaves internal temps dirty, so the only valid comparison is
self-contained 432T at 46 temps vs schoolbook 363T at zero temps:
schoolbook wins both ways (persist 363 < 432; clear 726 < 864). Kept as
the measured comparison-family member and MBU/recursive substrate, NOT
as the production mult — the paper round and EEA use schoolbook.
Proof pattern stands: 144 basis-pairs + structural multiset + 2000 seeded
samples vs F2_12.mul. No existing pins changed.
"""

import random

import pytest

from kubona.codesign import F2_12
from kubona.codesign.f12 import REDUCE_12
from kubona.codesign.kara12 import N_KARA_TEMPS, emit_f6_mul, emit_kara_mul12
from kubona.codesign.kickmix import Builder, _sim_trajectory

NQ = 12 + 12 + 12 + N_KARA_TEMPS


def _raw6(a, c):
    """Raw 6-bit product (no reduction) — 3-line independent oracle."""
    p = 0
    for i in range(6):
        for j in range(6):
            if (a >> i) & 1 and (c >> j) & 1:
                p ^= 1 << (i + j)
    return p


def test_f6_mul_exhaustive():
    b = Builder(6 + 6 + 11 + 1)
    n = emit_f6_mul(b, list(range(6)), list(range(6, 12)), list(range(12, 23)), [23])
    assert n == 72
    got = sorted((o.qubits[0], o.qubits[1], o.qubits[2]) for o in b.emit() if o.name == "ccx")
    want = []
    for i in range(6):
        for j in range(6):
            want += [(i, 6 + j, 23)] * 2  # compute + uncompute
    assert sorted(want) == got
    for a in range(64):
        for c in range(64):
            b = Builder(6 + 6 + 11 + 1)
            b.alloc(23)
            emit_f6_mul(b, list(range(6)), list(range(6, 12)), list(range(12, 23)), [23])
            init = {i: (a >> i) & 1 for i in range(6)}
            for i in range(6):
                init[6 + i] = (c >> i) & 1
            init[23] = 0
            for q in range(12, 23):
                init[q] = 0
            bits, phase, _ = _sim_trajectory(b.emit(), dict(init), random.Random(1))
            assert phase == 0, (a, c)
            got = sum((bits.get(12 + i, 0) & 1) << i for i in range(11))
            assert got == _raw6(a, c), (a, c)
            assert bits.get(23, 0) == 0, (a, c)
    with pytest.raises(ValueError):
        emit_f6_mul(Builder(24), list(range(5)), list(range(6, 12)), list(range(12, 23)), [23])


def test_kara_basis_sampled_and_pinned():
    assert N_KARA_TEMPS == 47
    rng = random.Random(56)
    cases = [(1 << i, 1 << j) for i in range(12) for j in range(12)]
    cases += [(0, 0), (0, 4095), (4095, 0), (4095, 4095), (1, 1)]
    cases += [(rng.randrange(4096), rng.randrange(4096)) for _ in range(2000)]
    n_tot = 0
    for a, c in cases:
        b = Builder(NQ)
        for q in range(36, NQ):
            b.alloc(q)
        n = emit_kara_mul12(b, list(range(12)), list(range(12, 24)), list(range(24, 36)), list(range(36, NQ)))
        assert n == 432, (a, c)
        n_tot += n
        init = {i: (a >> i) & 1 for i in range(12)}
        for i in range(12):
            init[12 + i] = (c >> i) & 1
        for q in range(36, NQ):
            init[q] = 0
        for q in range(24, 36):
            init[q] = 0
        bits, phase, _ = _sim_trajectory(b.emit(), dict(init), random.Random(2))
        assert phase == 0, (a, c)
        got = sum((bits.get(24 + i, 0) & 1) << i for i in range(12))
        assert got == F2_12.mul(a, c), (a, c)
        for q in range(36, NQ):
            assert bits.get(q, 0) == 0, (a, c, q)
        for q in range(36, NQ):
            b.free(q)
    assert n_tot == len(cases) * 432
    # determinism: identical emission on rebuild
    b1, b2 = Builder(NQ), Builder(NQ)
    for b in (b1, b2):
        for q in range(36, NQ):
            b.alloc(q)
        emit_kara_mul12(b, list(range(12)), list(range(12, 24)), list(range(24, 36)), list(range(36, NQ)))
    assert b1.emit() == b2.emit()


def test_kara_multiset_structural():
    # Every emitted CCX checked against the derived pattern: three sub-mults
    # (p0: a_lo/c_lo -> P[0:11]; p1: t0/t1 -> M; p2: a_hi/c_hi -> P[12:23]),
    # each (i,j) pair exactly 4x (forward compute+uncompute, mirror
    # compute+uncompute) on the shared pool. Fanout/reduction are CNOTs.
    b = Builder(NQ)
    emit_kara_mul12(b, list(range(12)), list(range(12, 24)), list(range(24, 36)), list(range(36, NQ)))
    got = sorted((o.qubits[0], o.qubits[1], o.qubits[2]) for o in b.emit() if o.name == "ccx")
    t0 = list(range(36, 42))
    t1 = list(range(42, 48))
    m = list(range(48, 59))
    p = list(range(59, 82))
    pool = 82
    subs = [
        (list(range(6)), list(range(12, 18)), p[0:11]),
        (t0, t1, m),
        (list(range(6, 12)), list(range(18, 24)), p[12:23]),
    ]
    want = []
    for A, C, _ in subs:
        for i in range(6):
            for j in range(6):
                want += [(A[i], C[j], pool)] * 4
    assert sorted(want) == got
    assert len(got) == 432


def test_kara_refusals():
    b = Builder(NQ)
    with pytest.raises(ValueError):
        emit_kara_mul12(b, list(range(11)), list(range(12, 24)), list(range(24, 36)), list(range(36, NQ)))
    with pytest.raises(ValueError):
        emit_kara_mul12(b, list(range(12)), list(range(12, 24)), list(range(24, 36)), list(range(36, NQ - 1)))
    with pytest.raises(ValueError):
        emit_kara_mul12(b, list(range(12)), list(range(12, 24)), list(range(12, 24)), list(range(36, NQ)))
