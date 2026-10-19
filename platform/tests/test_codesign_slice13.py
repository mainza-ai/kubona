"""Slice 13: HHL dyadic machinery — partition, coordinate, fits, sweep.

Paper anchors: 345 dyadic leaves widths 2^8..2^15, shared coordinate
d = 2^{15-b}δ + cL, quadratic qL(d) = g0 - d(g1 - g2d), worst error
9.95e-8 rad. Ours is OUR partition/fits (counts differ), same 1e-7 spec.
"""

import math

import pytest

from kubona.codesign import hhl


def test_true_f_endpoints():
    assert hhl.true_f(1 << 16) == round(2**32 * math.pi / 6)  # asin(1/2)
    assert abs(hhl.true_f((1 << 21) - 1) - 2**32 * math.asin(32768 / 2097151)) <= 1
    with pytest.raises(ValueError):
        hhl.true_f(0)


def test_rotl15():
    assert hhl.rotl15(0b1, 1) == 0b10
    assert hhl.rotl15(1 << 14, 1) == 1  # wraps within 15 bits
    assert hhl.rotl15(0b101, 0) == 0b101
    with pytest.raises(ValueError):
        hhl.rotl15(1 << 15, 1)


def test_affine_law_sample_leaves():
    # The load-bearing property: d == 2^{15-b}·δ + cL, cL leaf-constant.
    for leaf in ((65536, 15), (221952, 8), (2093056, 12)):
        cL = hhl.leaf_affine_law(leaf)
        L, b = leaf
        scale = 1 << (15 - b)
        for x in (L, L + 1, L + (1 << (b - 1)), min(L + (1 << b) - 1, 2097151)):
            d, _ = hhl.shared_coord(x, b)
            assert d == scale * (x - L) + cL, (leaf, x)
    with pytest.raises(ValueError):
        hhl.leaf_affine_law((3, 2))  # misaligned


def test_qeval_fixed_point_scales():
    # Regression for the >>S2 scale bug (859 LSB on the hard leaf): the
    # inner product keeps S1-scale precision (shift S2-S1, not S2).
    L, b = 221952, 8
    fit = hhl.fit_leaf((L, b))
    assert fit["max_err_lsb"] <= hhl.LSB_TOL
    # exactness of the model on a hand-checkable point
    assert hhl.qeval(0, 100, 0, 0) == 100
    assert hhl.qeval(4, 100, 1 << 28, 0) == 100 - 4


def test_refine_fit_deterministic_and_improving():
    L, b = 221952, 8
    xs = list(range(L, L + 16))
    ds = [(x - L) for x in xs]
    ys = [hhl.true_f(x) for x in xs]
    r1 = hhl.refine_fit(ds, ys, 0, 0, 0, seed=5)
    r2 = hhl.refine_fit(ds, ys, 0, 0, 0, seed=5)
    assert r1 == r2  # seeded determinism
    e0 = max(abs(hhl.qeval(d, 0, 0, 0) - y) for d, y in zip(ds, ys))
    e1 = max(abs(hhl.qeval(d, *r1) - y) for d, y in zip(ds, ys))
    assert e1 <= e0


def test_full_domain_within_tol():
    fitted = hhl.refine_domain()
    rep = hhl.sweep_report(fitted)
    assert rep["within_tol"] is True
    assert rep["worst_rad"] < 1e-7
    # exact cover: sorted leaves tile [2^16, 2^21) with dyadic alignment
    pos = 1 << 16
    for f in fitted:
        L, b = f["leaf"]
        assert L == pos and L % (1 << b) == 0 and 8 <= b <= 15
        pos = min(L + (1 << b), 1 << 21)
    assert pos == 1 << 21
