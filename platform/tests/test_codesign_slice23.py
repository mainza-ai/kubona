"""Slice 23: F64 block-Lanczos locator + Gaussian fold.

Production bar: extension-field machinery proven over the field (solvers
self-check; factored==direct; nulls verified); the fold is Clifford-exact
with its one-LSB price measured, not narrated.
"""

import random

import pytest

from kubona.codesign import (
    F64,
    FieldWalker,
    KickmixEvaluator,
    build_gauss_fold,
    factored_matvec,
    find_null_f64,
    gauss_fold_model,
    gauss_fold_spec,
    hdot,
    hmatvec,
    self_dot_is_square_sum,
)


def test_field_solvers_self_check():
    rng = random.Random(0)
    for _ in range(200):
        n = 4
        rows = [[rng.randrange(64) for _ in range(n)] for _ in range(n)]
        from kubona.codesign.hlanczos import _solve, _nullspace_list
        rhs = [rng.randrange(64) for _ in range(n)]
        sol = _solve(rows, rhs, F64)
        if sol is not None:
            assert hmatvec(rows, sol, F64) == rhs
        c = _nullspace_list(rows, F64)
        if c is not None:
            assert any(a != 0 for a in c) and hmatvec(rows, c, F64) == [0] * n


def test_cheap_breakdown_test_is_exact():
    rng = random.Random(1)
    for _ in range(50):
        w = [rng.randrange(64) for _ in range(6)]
        assert hdot(w, w, F64) == self_dot_is_square_sum(w, F64)


def test_factored_equals_direct_with_counted_cost():
    rng = random.Random(2)
    n, ns, ny = 6, 8, 5
    phi = [[rng.randrange(64) for _ in range(n)] for _ in range(ns)]
    lin = [[rng.randrange(64) for _ in range(ny)] for _ in range(ns)]
    syn = [rng.randrange(64) for _ in range(ny)]
    u = [rng.randrange(64) for _ in range(n)]
    rep = factored_matvec(phi, lin, syn, u, F64)
    eta = [hdot(r, syn, F64) for r in lin]
    direct = [[0] * n for _ in range(n)]
    for i in range(n):
        for j in range(n):
            s = 0
            for k in range(ns):
                s = F64.add(s, F64.mul(F64.mul(phi[k][i], eta[k]), phi[k][j]))
            direct[i][j] = s
    assert rep["out"] == hmatvec(direct, u, F64)
    assert rep["mults"] == ns < n * n  # pointwise only, maps are linear
    with pytest.raises(ValueError):
        factored_matvec(phi, [[1, 2]], syn, u, F64)  # lin shape refuses


def test_f64_locator_repair_and_honest_miss():
    A = [[49, 29, 35, 34, 20, 21], [29, 1, 42, 39, 43, 8],
         [35, 42, 3, 56, 2, 3], [34, 39, 56, 59, 6, 55],
         [20, 43, 2, 6, 58, 58], [21, 8, 3, 55, 58, 52]]
    seed = [20, 5, 11, 40, 19, 10]
    w = FieldWalker(A, F64)
    assert w.run(seed) == "dependent"
    assert w.repairs == 1 and w.maxdepth == 3
    sig = w.null_vector()
    assert sig is not None and hmatvec(A, sig, F64) == [0] * 6
    rep = find_null_f64(A, F64, [seed])
    assert rep["proven"] is True and rep["maxdepth"] == 3
    assert hmatvec(A, rep["null"], F64) == [0] * 6
    I = [[1 if i == j else 0 for j in range(6)] for i in range(6)]
    assert find_null_f64(I, F64, [[1, 0, 0, 0, 0, 0]])["proven"] is False
    with pytest.raises(ValueError):
        FieldWalker(A, F64).run([0, 0, 0, 0, 0, 0])  # zero seed refuses


def test_gauss_fold_clifford_exact_and_one_lsb():
    assert max(gauss_fold_model(x, 4)["error"] for x in range(16)) == 1
    assert gauss_fold_model(3, 4)["error"] == 0  # nonnegative axis untouched
    (b, regs) = build_gauss_fold(4)
    assert regs["ccx"] == 0  # complement is CNOT-only: no carry chain
    res = KickmixEvaluator().evaluate(b, gauss_fold_spec(4), num_tests=64, seed=7)
    assert res["valid"] is True and res["tests"] == 16
    with pytest.raises(ValueError):
        build_gauss_fold(9)  # outside exhaustive-proof scope
