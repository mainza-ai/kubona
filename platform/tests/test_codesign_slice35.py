"""Slice 35: Upsilon=1 conversion via matrices + specialized readout table.

Production bar: the recode phase fix is matrix-exact (every u=1 link's
recoded form equals the original element — the old phaseless recode
fails all 108); the Upsilon=1 conversion solves uniquely with matrix
identity + M-preservation on all 108; the specialized flag table agrees
with the normal-form table on all 216 with ANF evaluated on all 512
encodings. The specialized ANF (904 terms) barely beats normal-form
(922) — the paper's 20T needs real factorization, queued, never claimed.
"""

import pytest

from kubona.codesign import (
    FLAG_VALUES,
    all_elements,
    eval_anf,
    rep_matrix,
    spec_coord_bits,
    spec_flag_anf,
    spec_flag_table,
    to_internal,
    trace_class,
    trace_flag_table,
)


def _spec_link_matrix(p, q, r, m, u):
    from kubona.codesign.group72 import _gens, _mmul, _mpow, _mscal, _pow_cyc, _CYC_W
    G = _gens()
    Xo = _mmul(_mmul(G["C"], G["E"]), G["X"])
    out = _mscal(_pow_cyc(_CYC_W, p),
                 _mmul(_mpow(G["C"], q), _mpow(G["E"], r)))
    out = _mmul(out, _mpow(G["V"], m))
    if u:
        out = _mmul(out, Xo)
    return out


def test_recode_preserves_element_matrix_exact():
    from kubona.codesign import recode_link
    for g in all_elements():
        gl = recode_link(g)
        p, q, r, s, t, u = gl
        assert _spec_link_matrix(p, q, r, 2 * s + t, u) == rep_matrix(g), g


def test_upsilon1_conversion_unique_and_exact():
    from kubona.codesign.group72 import _gens, _mmul, _mpow, _mscal, _pow_cyc, _CYC_W
    G = _gens()
    Xo = _mmul(_mmul(G["C"], G["E"]), G["X"])
    n = 0
    for g in all_elements():
        if g[4] != 1:
            continue
        n += 1
        tP, Q2, R2, M, ups = to_internal(g)
        assert ups == 1 and M == g[3]
        b = (tP - Q2 * R2) % 3
        rebuilt = _mscal(_pow_cyc(_CYC_W, b), _mpow(G["V"], M))
        rebuilt = _mmul(rebuilt, Xo)
        rebuilt = _mmul(rebuilt, _mmul(_mpow(G["C"], Q2),
                                       _mpow(G["E"], R2)))
        assert rebuilt == rep_matrix(g)
    assert n == 108


def test_specialized_table_agrees():
    tab = spec_flag_table()
    ntab = trace_flag_table()
    assert len(tab) == 216
    for g in all_elements():
        t = to_internal(g)
        assert tab[t][0] == ntab[g][0] == trace_class(g)
        assert tab[t][1] == ntab[g][1]
        assert sum(tab[t][1]) == (1 if g != (0, 0, 0, 0, 0) else 0)


def test_specialized_anf_decides_table():
    anf = spec_flag_anf()
    assert {j: len(p) for j, p in anf.items()} == \
        {0: 256, 1: 64, 2: 168, 3: 192, 4: 32, 5: 192}
    tab = spec_flag_table()
    by_bits = {spec_coord_bits(t): fl for t, (_v, fl) in tab.items()}
    for mask in range(512):
        bits = tuple((mask >> i) & 1 for i in range(9))
        for j in range(6):
            assert eval_anf(anf[j], bits) == by_bits.get(bits, (0,) * 6)[j]
    # coordinate change alone barely simplifies (904 vs 922 normal-form):
    # the paper's 20T needs real factorization, queued.
    from kubona.codesign import trace_flag_anf
    assert sum(len(p) for p in anf.values()) == 904
    assert sum(len(p) for p in trace_flag_anf().values()) == 922


def test_spec_coord_bits_refuses():
    with pytest.raises(ValueError):
        spec_coord_bits((3, 0, 0, 0, 0))
    with pytest.raises(ValueError):
        spec_coord_bits((0, 0, 0, 4, 0))
    with pytest.raises(ValueError):
        spec_coord_bits("not-a-tuple")
    assert FLAG_VALUES == (-3, -2, -1, 0, 1, 2)
