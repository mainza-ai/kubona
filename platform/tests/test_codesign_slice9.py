"""Slice 9: binary fields — exhaustive axioms, pinned moduli, F24 table."""

import random

import pytest

from kubona.codesign.gf import F24, F64, BinaryField, f24_inversion_table


def test_pinned_moduli_reverify():
    # Constructors re-prove irreducibility + primitivity; tampered copies refuse.
    assert F24.modulus == 0b10011 and F64.modulus == 0b1000011
    with pytest.raises(ValueError):
        BinaryField(4, 0b10111)  # x^4+x^2+x+1 = (x+1)(x^3+x^2+1): reducible
    # irreducible but imprimitive: x^4+x^3+x^2+x+1 has order 5, not 15
    with pytest.raises(ValueError, match="not primitive"):
        BinaryField(4, 0b11111)


def test_f24_axioms_exhaustive():
    F = F24
    for a in range(16):
        assert F.mul(a, 0) == 0 and F.mul(a, 1) == a
        assert F.add(a, a) == 0
        if a:
            assert F.mul(a, F.inv(a)) == 1
            assert F.inv(F.inv(a)) == a  # double inversion
            p, acc = a, 1  # Fermat: a^15 == 1 by repeated squaring-free multiply
            for _ in range(15):
                acc = F.mul(acc, a)
            assert acc == 1
    for a in range(16):
        for b in range(16):
            assert F.mul(a, b) == F.mul(b, a)
            assert F.mul(a, F.add(b, 1)) == F.add(F.mul(a, b), a)
    for a in range(16):
        for b in range(16):
            for c in range(16):
                assert F.mul(F.mul(a, b), c) == F.mul(a, F.mul(b, c))
                assert F.add(F.add(a, b), c) == F.add(a, F.add(b, c))
    with pytest.raises(ValueError):
        F.inv(0)
    with pytest.raises(ValueError):
        F.mul(16, 1)


def test_f64_inverses_traces_exhaustive():
    F = F64
    for a in range(1, 64):
        assert F.mul(a, F.inv(a)) == 1
        assert F.sqr(a) == F.mul(a, a)
    for a in range(64):
        t = F.trace(a)
        assert t in (0, 1)
    rng = random.Random(3)
    for _ in range(300):
        a, b, c = (rng.randrange(64) for _ in range(3))
        assert F.mul(F.mul(a, b), c) == F.mul(a, F.mul(b, c))
        assert F.trace(a ^ b) == (F.trace(a) ^ F.trace(b))
    assert F.trace(0) == 0 and F.trace(1) == (6 & 1)


def test_f24_inversion_table_group_law():
    t = f24_inversion_table()
    assert len(t) == 16 and t[0] == 0 and t[1] == 1
    assert set(t.values()) == set(range(16))  # bijection (permutation polynomial)
    for x in range(1, 16):
        assert F24.mul(x, t[x]) == 1
