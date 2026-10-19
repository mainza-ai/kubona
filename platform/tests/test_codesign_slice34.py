"""Slice 34: Sigma(72x3) trace matrices, classes, and flag readout.

Production bar: the true 3x3 generators (Osorio et al. 2025,
arXiv:2511.17437v1 Eq 1, public CC BY 4.0) are exact cyclotomics, never
floats; the coordinate law is verified as a matrix homomorphism on all
216^2 pairs; conjugacy classes derived from the law match the published
character table's size/order rows exactly; f(g) = 2Re Tr is computed with
the integer-valued claim enforced (nonzero sqrt(3)-parts refuse); the
six-flag table is machine-derived, one-hot, with ANF and a proven naive
readout circuit. The naive circuit costs 6580T vs the paper's
hand-optimized 20T — measured and pinned, optimization queued, never
presented as competitive.
"""

import random
from collections import Counter

import pytest

from kubona.codesign import (
    FLAG_VALUES,
    TRACE_PAPER_COSTS,
    TRACE_VALUES,
    all_elements,
    build_trace_readout_circuit,
    class_data,
    conjugacy_classes,
    coord_bits,
    eval_anf,
    gidentity,
    ginv,
    gmul,
    rep_matrix,
    trace_class,
    trace_flag_anf,
    trace_flag_table,
    trace_flags,
    trace_readout_spec,
)
from kubona.codesign import KickmixEvaluator


def test_generator_relations_exact():
    from kubona.codesign.group72 import _gens, _miden, _mmul, _mpow, _mscal, _CYC_W
    G = _gens()
    C, E, V, X = G["C"], G["E"], G["V"], G["X"]
    I = _miden()
    assert _mpow(C, 3) == I and _mpow(E, 3) == I
    assert _mpow(V, 4) == I and _mpow(X, 4) == I
    w = _CYC_W
    assert _mmul(X, C) == _mmul(_mmul(_mpow(C, 2), E), X)
    assert _mmul(X, E) == _mmul(_mscal(w * w, C), _mmul(E, X))
    assert _mmul(X, V) == _mmul(_mscal(w * w, E), _mmul(_mpow(V, 3), X))
    assert _mpow(X, 2) == _mmul(_mpow(C, 2), _mpow(V, 2))  # gmul's X^2 rule
    assert _mmul(E, C) == _mscal(w, _mmul(C, E))  # Heisenberg premise


def test_representation_is_homomorphism():
    els = all_elements()
    M = {g: rep_matrix(g) for g in els}
    assert len(set(M.values())) == 216  # faithful
    from kubona.codesign.group72 import _miden, _mmul
    assert M[gidentity()] == _miden()
    for a in els:
        for b in els:
            assert _mmul(M[a], M[b]) == M[gmul(a, b)], (a, b)


def test_commute_table_matches_matrices():
    # True semantics (matrix-verified): V^M C^q E^r = w^dp C^q' E^r' V^M
    # for all 36 (q, r, M) — the table values and gmul's usage match the
    # paper's Alg gaugeprod.a exactly; only its prose direction is loose.
    from kubona.codesign.group72 import _commute, _gens, _mmul, _mpow, _mscal, _pow_cyc, _CYC_W
    G = _gens()
    for q in range(3):
        for r in range(3):
            lhs_base = _mmul(_mpow(G["C"], q), _mpow(G["E"], r))
            for M in range(4):
                dp, qp, rp = _commute(q, r, M)
                lhs = _mmul(_mpow(G["V"], M), lhs_base)
                rhs = _mscal(_pow_cyc(_CYC_W, dp),
                             _mmul(_mmul(_mpow(G["C"], qp), _mpow(G["E"], rp)),
                                   _mpow(G["V"], M)))
                assert lhs == rhs, (q, r, M)


def test_classes_match_character_table():
    # Osorio et al. Tab 2 Size/Ord rows (anchored public-table data).
    assert sorted(d["size"] for d in class_data()) == \
        [1, 1, 1, 9, 9, 9, 18, 18, 18, 18, 18, 18, 18, 18, 18, 24]
    assert sorted((d["size"], d["order"]) for d in class_data()) == \
        [(1, 1), (1, 3), (1, 3), (9, 2), (9, 6), (9, 6),
         (18, 4), (18, 4), (18, 4),
         (18, 12), (18, 12), (18, 12), (18, 12), (18, 12), (18, 12),
         (24, 3)]
    assert len(conjugacy_classes()) == 16
    # class equation closes on the group order
    assert sum(len(c) for c in conjugacy_classes()) == 216


def test_trace_values():
    assert TRACE_VALUES == frozenset({-3, -2, -1, 0, 1, 2, 6})
    assert FLAG_VALUES == (-3, -2, -1, 0, 1, 2)
    assert trace_class(gidentity()) == 6
    assert {g for g in all_elements() if trace_class(g) == 6} == {gidentity()}
    for cls in conjugacy_classes():
        assert len({trace_class(g) for g in cls}) == 1  # class function
    assert {trace_class(g) for g in all_elements()} == set(TRACE_VALUES)


def test_flag_table_one_hot():
    tab = trace_flag_table()
    assert len(tab) == 216
    for g, (v, fl) in tab.items():
        assert v == trace_class(g)
        assert fl == tuple(1 if x == v else 0 for x in FLAG_VALUES)
        assert (sum(fl) == 1) == (g != gidentity())
    assert TRACE_PAPER_COSTS["six_flags"] == 20  # anchor, not re-derived


def test_flag_anf_decides_table():
    anf = trace_flag_anf()
    assert {j: len(p) for j, p in anf.items()} == \
        {0: 256, 1: 128, 2: 118, 3: 192, 4: 96, 5: 132}
    tab = trace_flag_table()
    by_bits = {coord_bits(g): fl for g, (_v, fl) in tab.items()}
    for mask in range(512):
        bits = tuple((mask >> i) & 1 for i in range(9))
        for j in range(6):
            assert eval_anf(anf[j], bits) == by_bits.get(bits, (0,) * 6)[j]


def test_readout_circuit_proves_table():
    (b, regs) = build_trace_readout_circuit()
    assert regs["ccx"] == 6580 and regs["temps"] == 8 and regs["terms"] == 922
    spec = trace_readout_spec()
    ev = KickmixEvaluator()
    for mask in range(512):
        init = {q: 0 for q in range(15 + regs["temps"])}
        for i in range(9):
            init[i] = (mask >> i) & 1
        res = ev.check_one(b, spec, init, random.Random(0))
        assert res.valid, (mask, res.reason)


def test_rep_refusals():
    with pytest.raises(ValueError):
        rep_matrix((0, 0, 0, 0, 2))
    with pytest.raises(ValueError):
        trace_class((3, 0, 0, 0, 0))
    with pytest.raises(ValueError):
        trace_flags("not-a-tuple")
