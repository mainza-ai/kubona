"""Slice 8: Lanczos null finder — verified nulls, repair path, honest misses."""

import random

import pytest

from kubona.codesign import (
    LanczosWalker,
    check_symmetric,
    find_null,
    gfdot,
    matvec,
    nullspace_vector,
)


def _singular(n, k, rng):
    M = [[rng.getrandbits(1) for _ in range(k)] for _ in range(n)]
    rows = []
    for i in range(n):
        mask = 0
        for j in range(n):
            if sum(M[i][t] * M[j][t] for t in range(k)) & 1:
                mask |= 1 << j
        rows.append(mask)
    return rows


def test_gf2_helpers():
    assert gfdot(0b101, 0b110) == 1
    assert gfdot(0b101, 0b010) == 0
    assert matvec([0b10, 0b01], 0b11, 2) == 0b11
    with pytest.raises(ValueError):
        check_symmetric([0b10, 0b10], 2)  # asymmetric refuses
    with pytest.raises(ValueError):
        check_symmetric([0b1], 2)  # wrong row count refuses


def test_nullspace_solver():
    assert nullspace_vector([0b01], 2) == 0b10
    assert nullspace_vector([0b11, 0b11], 2) == 0b11  # x^y==0
    assert nullspace_vector([0b01, 0b10], 2) is None  # full rank: honest miss


def test_batch_all_nulls_verify():
    rng = random.Random(11)
    n = 8
    wins = 0
    vias: dict[str, int] = {}
    for _ in range(30):
        A = _singular(n, 5, rng)
        seeds = [1 << i for i in range(n)] + [rng.randrange(1, 1 << n) for _ in range(4)]
        rep = find_null(A, n, seeds)
        assert rep["theorem_holds"]
        if rep["proven"]:
            wins += 1
            vias[rep["via"]] = vias.get(rep["via"], 0) + 1
            s = rep["null"]
            assert s != 0 and matvec(A, s, n) == 0
    assert wins == 30
    assert set(vias) <= {"projection", "direct"}


def test_repair_path_pinned_and_proven():
    # Deterministic case that forces a depth-2 block repair yet still proves.
    rng = random.Random(11)
    n = 8
    found = None
    for _ in range(30):
        A = _singular(n, 5, rng)
        seeds = [1 << i for i in range(n)] + [rng.randrange(1, 1 << n) for _ in range(4)]
        rep = find_null(A, n, seeds)
        if rep["proven"] and rep["repairs"] >= 1:
            found = (A, rep)
            break
    assert found is not None, "no repair case in 30 draws (seed drift?)"
    A, rep = found
    assert rep["null"] != 0
    assert matvec(A, rep["null"], n) == 0


def test_invertible_reports_unproven_not_fake():
    n = 4
    ident = [1 << i for i in range(n)]
    rep = find_null(ident, n, [1 << i for i in range(n)])
    assert rep["proven"] is False and rep["null"] is None


def test_bad_inputs_refuse():
    with pytest.raises(ValueError):
        find_null([0b1], 2, [])  # no seeds
    with pytest.raises(ValueError):
        find_null([0b1], 2, [0])  # zero seed
    with pytest.raises(ValueError):
        LanczosWalker([0b1], 1)  # n too small
    with pytest.raises(ValueError):
        LanczosWalker([0b1] * 4, 17)  # n too large (rows asymmetric too)
