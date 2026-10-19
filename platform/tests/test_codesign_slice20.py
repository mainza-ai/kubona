"""Slice 20: Hermitian geometry + Sigma(72x3) group law.

Production bar: paper anchors reproduced as machine-checked counts (512
points, 123/48 monomials); the group law verified as a group (216
inverses, sampled associativity incl. the X-sector); the specialized
update proven against general multiplication on the convertible sector.
"""

import random

import pytest

from kubona.codesign import (
    F64,
    all_elements,
    curve_derivative,
    curve_points,
    gidentity,
    ginv,
    gmul,
    heval_at,
    hreduce,
    locator_recurrence_holds,
    monomials,
    recode_link,
    residue,
    spec_update,
    syndrome,
    to_internal,
    unrecode_link,
)


def test_hermitian_anchors_reproduced():
    assert len(curve_points(F64)) == 512
    assert len(monomials(150)) == 123
    assert len(monomials(75)) == 48
    with pytest.raises(ValueError):
        monomials(151)
    from kubona.codesign import F24
    with pytest.raises(ValueError):
        curve_points(F24)  # not F64: refusing


def test_hermitian_residue_and_recurrence():
    pts = curve_points(F64)
    P, xj = pts[0], pts[0][0]
    sigma = {(1, 0): 1, (0, 0): xj}  # x + xj vanishes on xj's fiber
    fiber = [Q for Q in pts if Q[0] == xj]
    assert all(heval_at(sigma, Q, F64) == 0 for Q in fiber)
    d = curve_derivative(sigma, F64)
    assert d == {(0, 0): 1}  # transverse: simple zeros admit residues
    assert all(heval_at(d, Q, F64) != 0 for Q in fiber)
    assert residue(sigma, F64.mul(7, heval_at(d, P, F64)), P, F64) == 7
    assert locator_recurrence_holds(sigma, [(P, 7)], monomials(150), F64) is True
    # wrong fiber: recurrence detects the non-locator
    other = next(Q for Q in pts if Q[0] != xj)
    bad = {(1, 0): 1, (0, 0): other[0]}
    assert locator_recurrence_holds(bad, [(P, 7)], monomials(150), F64) is False
    # repeated zero: derivative vanishes, residue refuses division
    sq = {(2, 0): 1, (0, 0): F64.mul(xj, xj)}
    assert heval_at(curve_derivative(sq, F64), P, F64) == 0
    with pytest.raises(ValueError):
        residue(sq, 7, P, F64)


def test_hermitian_reduction_sound():
    rng = random.Random(0)
    pts = curve_points(F64)
    for _ in range(20):
        p = {(rng.randrange(0, 6), rng.randrange(0, 12)): rng.randrange(1, 64)
             for _ in range(4)}
        Q = pts[rng.randrange(512)]
        assert heval_at(hreduce(p, F64), Q, F64) == heval_at(p, Q, F64)
    assert syndrome([(pts[0], 5)], {(0, 0): 1}, F64) == 5  # S(1) = error sum


def test_sigma72_group_axioms():
    els = all_elements()
    assert len(els) == 216
    e = gidentity()
    assert all(gmul(e, g) == g and gmul(g, e) == g for g in els)
    assert all(gmul(g, ginv(g)) == e and gmul(ginv(g), g) == e for g in els)
    rng = random.Random(0)
    for _ in range(200):  # associativity incl. the X-sector rewrite lines
        a, b, c = (els[rng.randrange(216)] for _ in range(3))
        assert gmul(gmul(a, b), c) == gmul(a, gmul(b, c))
    # commute table self-consistency: M=2/M=3 follow from M=1 moves
    from kubona.codesign.group72 import _commute
    for q in range(3):
        for r in range(3):
            once = _commute(q, r, 1)
            twice = _commute(once[1], once[2], 1)
            thrice = _commute(twice[1], twice[2], 1)
            assert twice[1:] == _commute(q, r, 2)[1:]
            assert thrice[1:] == _commute(q, r, 3)[1:]
    with pytest.raises(ValueError):
        gmul(e, (0, 0, 0, 0, 2))  # U outside Z2 refuses


def test_recode_and_sector_agreement():
    els = all_elements()
    assert all(unrecode_link(recode_link(g)) == g for g in els)
    # Upsilon=1 conversion lands via matrix solve (was a queued refusal)
    assert to_internal((0, 0, 0, 0, 1))[4] == 1
    # round-trip over ALL 216 (both sectors; was: Upsilon=0 only)
    for W in els:
        P, Q, R, M, ups = W
        tP, Q2, R2, MM, ups2 = to_internal(W)
        assert ups2 == ups and MM == M
        b = (tP - Q2 * R2) % 3
        if ups == 0:
            assert gmul((b, 0, 0, M, 0), (0, Q2, R2, 0, 0)) == W
        else:
            Xo = (0, 1, 1, 0, 1)
            assert gmul(gmul(gmul((b, 0, 0, 0, 0), (0, 0, 0, M, 0)),
                            Xo), (0, Q2, R2, 0, 0)) == W
    # full-sector agreement, exhaustive over all 216^2 pairs with
    # precomputed conversions (was: 500-sampled Upsilon=0/u=0 only)
    conv = {g: to_internal(g) for g in els}
    for W in els:
        tW = conv[W]
        for g in els:
            assert conv[gmul(W, g)] == spec_update(tW, recode_link(g))
    with pytest.raises(ValueError):
        spec_update((0, 0, 0, 0, 0), (0, 0, 0, 0, 0, 3))  # bad link refuses
