"""Slice 61: MBU-packed F2^12 multiplier (F2 follow-up, OUR construction).

Production bar: schoolbook structure (144 AND pairs, REDUCE_12 fused fanout)
with MBU ANDs (1T + measure + CZ-correct, no uncompute): 144T pinned
(vs 363T plain schoolbook, 2.5x; vs 432T plain Kara which loses outright).
Proven functionally exhaustive on basis pairs + seeded sample, multi-seed
(measurement randomness), phase-clean throughout. The CZ corrections are
load-bearing (dropping one produces nonzero phase — tested explicitly,
tower12 slice-11 pattern). E2 bar coverage applies (classical-input paths
only); MBU never touches superposition (no such paths exist in the stack).
"""

import random

import pytest

from kubona.codesign import F2_12
from kubona.codesign.f12 import REDUCE_12
from kubona.codesign.kickmix import Builder, _sim_trajectory
from kubona.codesign.mbu12 import emit_mbu_mul12


def test_mbu_pattern_exact():
    b = Builder(37)
    n = emit_mbu_mul12(b, list(range(12)), list(range(12, 24)), list(range(24, 36)), [36])
    assert n == 144
    ops = b.emit()
    assert sum(1 for o in ops if o.name == "ccx") == 144
    assert sum(1 for o in ops if o.name == "hmr") == 144
    assert sum(1 for o in ops if o.name == "cz") == 144
    # each AND fans out exactly to its REDUCE set (structural multiset)
    got = sorted((o.qubits[0], o.qubits[1], o.qubits[2]) for o in ops if o.name == "ccx")
    want = []
    for i in range(12):
        for j in range(12):
            want.append((i, 12 + j, 36))
    assert sorted(want) == got
    with pytest.raises(ValueError):
        emit_mbu_mul12(Builder(37), [0] * 11, list(range(12, 24)), list(range(24, 36)), [36])


def test_mbu_functional_basis_and_sampled():
    rng = random.Random(61)
    cases = [(1 << i, 1 << j) for i in range(12) for j in range(12)]
    cases += [(0, 0), (0, 4095), (4095, 4095), (1, 1)]
    cases += [(rng.randrange(4096), rng.randrange(4096)) for _ in range(500)]
    for a, c in cases:
        for seed in (1, 2, 3):
            b = Builder(37)
            n = emit_mbu_mul12(b, list(range(12)), list(range(12, 24)), list(range(24, 36)), [36])
            assert n == 144
            init = {i: (a >> i) & 1 for i in range(12)}
            for i in range(12):
                init[12 + i] = (c >> i) & 1
            for i in range(24, 36):
                init[i] = 0
            init[36] = 0
            bits, phase, _ = _sim_trajectory(b.emit(), dict(init), random.Random(seed))
            assert phase == 0, (a, c, seed)
            got = sum((bits.get(24 + i, 0) & 1) << i for i in range(12))
            assert got == F2_12.mul(a, c), (a, c, seed)
    # determinism: identical emission on rebuild
    b1, b2 = Builder(37), Builder(37)
    for b in (b1, b2):
        emit_mbu_mul12(b, list(range(12)), list(range(12, 24)), list(range(24, 36)), [36])
    assert b1.emit() == b2.emit()


def test_mbu_corrections_load_bearing():
    # Drop the first conditioned CZ: the authored correction is structural;
    # the suite must catch its absence (else corrections are decoration).
    b = Builder(37)
    emit_mbu_mul12(b, list(range(12)), list(range(12, 24)), list(range(24, 36)), [36])
    out = []
    dropped = False
    for o in b.emit():
        if not dropped and o.name == "cz" and len(o.cond) > 0:
            dropped = True
            continue
        out.append(o)
    assert dropped
    bad = 0
    total = 0
    for (a, c) in [(5, 7), (100, 200), (1000, 2000), (1, 4095), (2048, 2048)]:
        for seed in range(10):
            init = {i: (a >> i) & 1 for i in range(12)}
            for i in range(12):
                init[12 + i] = (c >> i) & 1
            bits, phase, _ = _sim_trajectory(out, dict(init), random.Random(seed))
            total += 1
            got = sum((bits.get(24 + i, 0) & 1) << i for i in range(12))
            if got != F2_12.mul(a, c) or phase != 0:
                bad += 1
    assert bad > 0, "dropping a correction must be caught (else test is blind)"
