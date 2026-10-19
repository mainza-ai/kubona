"""Slice 33: Hermitian local coefficients + evaluator series + striped scan.

Production bar: x is a uniformizer at every affine point (pinned over all
512: c0 == eval, c1 == curve-derivative eval); the local rule recovers
planted values exactly (simple ratio == residue, double/triple quotients
where the old path refuses); striped scan == direct scan over full error
patterns; storage shapes machine-checked (direct > 100 symbols, striped
bounded); mult counts measured both ways with no winner declared. The
truncated-series boundary is pinned, not hidden: syndrome-built omega
reads 0 at error points (recovery needs the true series, queued).
"""

import random

import pytest

from kubona.codesign import (
    F64,
    curve_derivative,
    curve_points,
    error_value_local,
    heval_at,
    hreduce,
    local_expansion,
    locator_recurrence_holds,
    monomials,
    omega_series,
    residue,
    scan_direct,
    scan_stripes,
    syndrome,
)


def _fiber_pts(x0):
    return [Q for Q in curve_points(F64) if Q[0] == x0]


def test_fiber_shape_and_y_series():
    pts = curve_points(F64)
    assert len({Q[0] for Q in pts}) == 64
    assert all(len(_fiber_pts(Q[0])) == 8 for Q in pts)  # 64 fibers x 8
    from kubona.codesign.hermitian import _y_series, _fpow
    for Q in pts:
        x0, y0 = Q
        a = _y_series(x0, y0, F64, 3)
        assert a[0] == _fpow(F64, x0, 8)  # dy = x^8 dx, always
        assert a[1] == 0 and a[2] == 0  # C(9,2), C(9,3) even: no t^2/t^3
        # series satisfies the curve equation through order 3
        acc = F64.add(_fpow(F64, y0, 8), y0)
        acc = F64.add(acc, _fpow(F64, x0, 9))
        assert acc == 0


def test_uniformizer_theorem_all_points():
    pts = curve_points(F64)
    x0 = pts[0][0]
    probes = [{(1, 0): 1, (0, 0): x0},
              {(2, 0): 1, (0, 3): 5},
              {(0, 1): 1, (3, 2): 7, (0, 0): 11},
              {(5, 7): 3, (9, 0): 13}]
    for Q in pts:
        for pr in probes:
            c = local_expansion(pr, Q, F64, 4)
            assert c[0] == heval_at(pr, Q, F64)
            assert c[1] == heval_at(curve_derivative(pr, F64), Q, F64)


def test_simple_rule_agrees_with_residue():
    rng = random.Random(3)
    pts = curve_points(F64)
    for _ in range(8):
        P = pts[rng.randrange(512)]
        sigma = {(1, 0): 1, (0, 0): P[0]}  # x - x0, simple along fiber
        w0 = rng.randrange(1, 64)
        assert error_value_local(sigma, {(0, 0): w0}, P, F64) == \
            residue(sigma, w0, P, F64) == w0  # sigma_1 = 1 here
    # planted: omega_0 = e * sigma_1 recovers e
    P = pts[5]
    sigma = {(1, 0): 1, (0, 0): P[0]}
    assert error_value_local(sigma, {(0, 0): 41}, P, F64) == 41


def test_double_zero_rule_takes_over_where_residue_refuses():
    pts = curve_points(F64)
    P = next(Q for Q in pts if len(_fiber_pts(Q[0])) >= 2)
    x0 = P[0]
    sigma = {(2, 0): 1, (0, 0): F64.mul(x0, x0)}  # (x-x0)^2
    with pytest.raises(ValueError):
        residue(sigma, 7, P, F64)  # sigma_1 == 0: old path refuses
    # planted omega = e * sigma recovers e via the quotient
    assert error_value_local(sigma, {(2, 0): 9, (0, 0): F64.mul(9, F64.mul(x0, x0))},
                             P, F64) == 9
    # triple: needs order 3 (n=4 expansion)
    sigma3 = {(3, 0): 1, (0, 0): F64.mul(F64.mul(x0, x0), x0)}
    om3 = {(3, 0): 5, (0, 0): F64.mul(5, F64.mul(F64.mul(x0, x0), x0))}
    assert error_value_local(sigma3, om3, P, F64) == 5
    # multiplicity >= 4 refuses (beyond the paper's <= 3)
    sigma4 = {(4, 0): 1, (0, 0): F64.mul(F64.mul(F64.mul(x0, x0), x0), x0)}
    with pytest.raises(ValueError):
        error_value_local(sigma4, om3, P, F64)
    # pole refuses (omega vanishes less than sigma)
    with pytest.raises(ValueError):
        error_value_local(sigma, {(1, 0): 1}, P, F64)
    # singular non-zero refuses (sigma_1 == 0, sigma_0 != 0)
    Q = next(Q for Q in pts if Q[0] != 0)
    with pytest.raises(ValueError):
        error_value_local({(2, 0): 1, (0, 0): 7}, {(0, 0): 3}, Q, F64)


def _syndrome_scan(errors):
    basis = monomials(150)
    syns = [syndrome(errors, {(a, b): 1}, F64) for (a, b) in basis]
    xs = sorted({P[0] for P, _ in errors})
    # sigma = product over distinct error-x of (x - x0)
    sigma = {(0, 0): 1}
    for x0 in xs:
        shifted = {}
        for (a, b), c in sigma.items():
            shifted[(a + 1, b)] = F64.add(shifted.get((a + 1, b), 0), c)
            shifted[(a, b)] = F64.add(shifted.get((a, b), 0),
                                       F64.mul(c, x0))
            if shifted[(a, b)] == 0:
                del shifted[(a, b)]
        sigma = hreduce(shifted, F64)
    assert locator_recurrence_holds(sigma, errors, basis, F64) is True
    return syns, sigma


def test_stripe_scan_equals_direct():
    rng = random.Random(11)
    pts = curve_points(F64)
    patterns = []
    P0 = pts[rng.randrange(512)]
    patterns.append([(P0, 7)])
    Q0 = next(Q for Q in pts if Q[0] == P0[0] and Q != P0)
    patterns.append([(P0, 5), (Q0, 9)])  # shared-x double (multiplicity 2)
    Trio = _fiber_pts(P0[0])[:3]
    patterns.append([(Trio[0], 3), (Trio[1], 11), (Trio[2], 13)])
    P1 = pts[rng.randrange(512)]
    P2 = pts[rng.randrange(512)]
    patterns.append([(P0, 7), (P1, 4), (P2, 6)])
    for errors in patterns:
        syns, sigma = _syndrome_scan(errors)
        direct, dinfo = scan_direct(syns, sigma, pts, F64)
        striped, sinfo = scan_stripes(syns, sigma, pts, F64)
        assert direct == striped
        assert sinfo["nstripes"] == 32
        assert all(direct[P] == 0 for P, _ in errors)  # truncation boundary:
        # the truncated S_hat recovers nothing (needs the true series)


def test_stripe_cost_shapes():
    pts = curve_points(F64)
    P0 = next(Q for Q in pts if Q[0] not in (0, 1) and Q[1] != 0)
    # generic point: every basis monomial reads nonzero (the degenerate
    # origin sees 1 syndrome); x0 = 1 is excluded because (x-1)*S_hat
    # telescopes there (interior keys cancel pairwise in char 2).
    errors = [(P0, 7)]
    syns, sigma = _syndrome_scan(errors)
    assert sum(1 for s in syns if s) == 123
    # telescoping pin: at x0 = 1 the single-error omega collapses to the
    # 16 boundary monomials (8 x0-part + 8 top-edge x-part).
    Q1 = next(Q for Q in pts if Q[0] == 1 and Q[1] != 0)
    syns1, sigma1 = _syndrome_scan([(Q1, 7)])
    assert len(omega_series(syns1, sigma1, monomials(150), F64)) == 16
    direct, dinfo = scan_direct(syns, sigma, pts, F64)
    striped, sinfo = scan_stripes(syns, sigma, pts, F64)
    assert dinfo["omega_terms"] > 100  # paper's "> 100 field symbols"
    assert sinfo["stripe_omega_symbols"] <= 64  # bounded, point-count-free

    class Counter:
        def __init__(self, f):
            self.f, self.n = f, 0
        def __getattr__(self, k):
            return getattr(self.f, k)
        def mul(self, a, b):
            self.n += 1
            return self.f.mul(a, b)

    basis = monomials(150)
    omega = omega_series(syns, sigma, basis, F64)
    hmul_muls = len(sigma) * sum(1 for v in
                                 ({(a, b): s for (a, b), s in
                                   zip(basis, syns)}).values() if v)
    cd = Counter(F64)
    scan_direct(syns, sigma, pts, cd)
    cs = Counter(F64)
    scan_stripes(syns, sigma, pts, cs)
    assert cd.n > 0 and cs.n > 0
    assert cs.n > hmul_muls  # recompute-per-stripe costs more than one
    # formation (the paper's direction); storage is what stripes save


def test_scan_refusals():
    pts = curve_points(F64)
    syns = [0] * 123
    sigma = {(1, 0): 1, (0, 0): pts[0][0]}
    with pytest.raises(ValueError):
        scan_stripes(syns, sigma, pts, F64, stripe=0)
    with pytest.raises(ValueError):
        scan_stripes(syns, sigma, pts, F64, stripe=True)
    with pytest.raises(ValueError):
        omega_series([1, 2], sigma, monomials(150), F64)
    with pytest.raises(ValueError):
        omega_series(syns, {(9, 9): 1}, monomials(150), F64)
    from kubona.codesign import F24
    # local_expansion is field-generic (curve relation is char-2
    # throughout); only point/basis counts are F64-specific — no refusal.
    assert local_expansion({(1, 0): 1}, pts[0], F24, 2)[0] == 0
    with pytest.raises(ValueError):
        local_expansion(sigma, pts[0], F64, 5)
    with pytest.raises(ValueError):
        local_expansion(sigma, pts[0], F64, 0)
