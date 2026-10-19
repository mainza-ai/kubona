"""Slice 42: minimized divstep decoders (depths 6-8, exact structural merging).

Production bar: SOP-from-scratch minimization LOSES on these XOR-dense
rules (see log: depth-6 greedy gives 223 cubes vs 79 ANF terms, and takes
77s to do it — the wrong normal form, never shipped); instead the ANF
itself is reduced by exact adjacent-combination + cross-care absorption
(never worse than the ANF by construction). The minimized circuits prove
the same spec exhaustively: 46/139/505 terms at 234/960/4568T vs naive
79/268/1092 at 344/1552/8448T (32-46% fewer Toffolis, measured, same
spec, no re-proof gap).
"""

import random

import pytest

from kubona.codesign import (
    KickmixEvaluator,
    build_decoder_circuit_minimized,
    decoder_circuit_spec,
    decoder_poly_derived,
    reduce_anf,
)


def _anf_cubes(depth):
    n = 2 * (depth - 1)
    names = (["f%d" % (i + 1) for i in range(depth - 1)] +
             ["q%d" % (i + 1) for i in range(depth - 1)])
    qid = {v: i for i, v in enumerate(names)}
    cubes, const = [], 0
    for term in decoder_poly_derived(depth):
        if len(term) == 0:
            const += 1
        else:
            m = 0
            for v in term:
                m |= 1 << qid[v]
            cubes.append((m, m))
    return cubes, const, n


def test_reduce_anf_never_worse_and_exact():
    from kubona.codesign.minimize import _eval_cover, verify_cover
    for depth, nterms in ((6, 79), (7, 268), (8, 1092)):
        cubes, const, n = _anf_cubes(depth)
        assert len(cubes) + const == nterms
        red = reduce_anf(sorted(cubes), n)
        assert len(red) <= len(cubes)  # shrink-or-tie, never worse
        # exactness on the full domain (const flips all bits if odd)
        from kubona.codesign import eval_decoder
        names = (["f%d" % (i + 1) for i in range(depth - 1)] +
                 ["q%d" % (i + 1) for i in range(depth - 1)])
        for mask in range(1 << n):
            want = eval_decoder(depth, {name: (mask >> i) & 1
                                        for i, name in enumerate(names)})
            assert (_eval_cover(red, mask) ^ (const & 1)) == want
    with pytest.raises(ValueError):
        reduce_anf([(1, 2)], 0)
    with pytest.raises(ValueError):
        reduce_anf([("x", 1)], 4)


def test_minimized_decoders_prove_spec():
    expect = {6: (46, 234, 3), 7: (139, 960, 4), 8: (505, 4568, 6)}
    ev = KickmixEvaluator()
    for depth, (terms, ccx, ntemps) in expect.items():
        (b, regs) = build_decoder_circuit_minimized(depth)
        assert regs["ccx"] == ccx and regs["temps"] == list(range(2 * (depth - 1) + 1, 2 * (depth - 1) + 1 + ntemps))
        assert regs["terms"] == terms and regs["kind"] == "minimized-anf"
        spec = decoder_circuit_spec(depth)
        n = 2 * (depth - 1)
        for mask in range(1 << n):
            init = {q: (mask >> q) & 1 for q in range(n)}
            res = ev.check_one(b, spec, init, random.Random(1))
            assert res.valid, (depth, mask, res.reason)
    with pytest.raises(ValueError):
        build_decoder_circuit_minimized(9)
    with pytest.raises(ValueError):
        build_decoder_circuit_minimized(1)
