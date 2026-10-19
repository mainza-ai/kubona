"""Slice 21: derived decoders, sparse fold, Horner kernel.

Production bar: paper numbers derived, not cited (all seven decoder counts
from the generic theorem); identities asserted live per instance; the two
uncompute bugs caught this run stay pinned by the exhaustive proofs that
caught them (reversal-is-not-addition, shared-wrap parity).
"""

import random

import pytest

from kubona.codesign import (
    Builder,
    KickmixEvaluator,
    TaskSpec,
    build_horner_quad,
    decoder_poly_derived,
    decoder_terms,
    emit_add,
    emit_sub,
    eval_decoder,
    fold_reduce,
    fused_square,
    fused_square_parts,
    horner_quad_model,
    horner_quad_spec,
    term_count,
    trunc_mul_bound,
)
from kubona.codesign.divsteps import PAPER_P256
from kubona.codesign.divwalk import bit, modinv_odd


def test_decoder_counts_all_derived():
    assert {d: term_count(d) for d in range(2, 9)} == \
        {2: 3, 3: 3, 4: 11, 5: 26, 6: 79, 7: 268, 8: 1092}
    assert decoder_terms(9) == 3 + 3 + 11 + 26 + 79 + 268 + 1092
    with pytest.raises(ValueError):
        decoder_poly_derived(9)
    with pytest.raises(ValueError):
        decoder_poly_derived(4, p=8)  # derivation needs odd modulus arithmetic


def test_derived_conforms_to_generic_theorem():
    rng = random.Random(0)
    for depth in range(2, 9):
        j, M = depth, 1 << depth
        for _ in range(12):
            fp = rng.randrange(1, M, 2)
            qp = rng.randrange(1, M, 2)
            bits = {f"f{i + 1}": (fp >> (i + 1)) & 1 for i in range(j - 1)}
            bits.update({f"q{i + 1}": (qp >> (i + 1)) & 1 for i in range(j - 1)})
            want = bit((PAPER_P256 % M) * modinv_odd((fp * qp) % M, M) % M, j - 1)
            assert eval_decoder(depth, bits) == want


def test_sparse_fold_and_fused_square():
    from kubona.codesign.carry import FOLD_C256
    assert FOLD_C256 == 2**32 + 977
    assert PAPER_P256 + FOLD_C256 == 1 << 256  # single-home consistency
    rng = random.Random(1)
    for _ in range(20):
        x = rng.getrandbits(rng.choice((100, 256, 300, 512, 700)))
        assert fold_reduce(x) == x % PAPER_P256
    for _ in range(10):
        lam = rng.getrandbits(256)
        rep = fused_square(lam)
        assert rep["result"] == pow(lam, 2, PAPER_P256)
        assert len(rep["partials"]) == 3  # three half-products, never a square
    with pytest.raises(ValueError):
        fold_reduce(-1)
    with pytest.raises(ValueError):
        fused_square_parts(1 << 256)


def _check_add_sub_exhaustive():
    rng = random.Random(2)
    ev = KickmixEvaluator()
    for cin in (0, 1):
        for a in range(16):
            for bv in range(8):
                b = Builder(32)
                A, B, C = list(range(4)), list(range(10, 13)), list(range(20, 23))
                for q in C:
                    b.alloc(q)
                init = {q: 0 for q in range(32)}
                for i, q in enumerate(A):
                    init[q] = (a >> i) & 1
                for i, q in enumerate(B):
                    init[q] = (bv >> i) & 1
                emit_add(b, A, B, C, cin)
                for q in C:
                    b.free(q)
                want = (a + bv + cin) % 16
                spec = TaskSpec(num_qubits=32, inputs=tuple(B), outputs=tuple(A),
                                ancillas=tuple(C),
                                target_perm=lambda bits, w=want: {
                                    **bits, **{q: ((w >> i) & 1) for i, q in enumerate(A)}})
                res = ev.check_one(b, spec, init, rng)
                assert res.valid, (a, bv, cin, res.reason)
    for a in range(16):
        for bv in range(8):
            if bv > a:
                continue
            b = Builder(32)
            A, B, C = list(range(4)), list(range(10, 13)), list(range(20, 23))
            for q in C:
                b.alloc(q)
            init = {q: 0 for q in range(32)}
            for i, q in enumerate(A):
                init[q] = (a >> i) & 1
            for i, q in enumerate(B):
                init[q] = (bv >> i) & 1
            emit_sub(b, A, B, C)
            for q in C:
                b.free(q)
            want = a - bv
            spec = TaskSpec(num_qubits=32, inputs=tuple(B), outputs=tuple(A),
                            ancillas=tuple(C),
                            target_perm=lambda bits, w=want: {
                                **bits, **{q: ((w >> i) & 1) for i, q in enumerate(A)}})
            res = ev.check_one(b, spec, init, rng)
            assert res.valid, (a, bv, res.reason)


def test_adder_subtractor_exhaustive():
    _check_add_sub_exhaustive()


def test_horner_configs_proven_and_truncated_bounded():
    for coefs in ((127, 23, 1), (100, 17, 2), (69, 31, 3)):
        (b, regs) = build_horner_quad(*coefs)
        res = KickmixEvaluator().evaluate(b, horner_quad_spec(*coefs),
                                           num_tests=16, seed=7)
        assert res["valid"] is True and res["tests"] == 4
    assert trunc_mul_bound(2, 2, 0) == 0
    assert trunc_mul_bound(2, 2, 1) == 1
    assert trunc_mul_bound(2, 2, 2) == 5
    g0, g1, g2 = (127, 23, 1)
    bound = trunc_mul_bound(2, 2, 2) + trunc_mul_bound(2, 6, 2)
    worst = 0
    for drop in (1, 2):
        (b, regs) = build_horner_quad(g0, g1, g2, drop)
        res = KickmixEvaluator().evaluate(b, horner_quad_spec(g0, g1, g2, drop),
                                           num_tests=16, seed=7)
        assert res["valid"] is True  # bit-exact vs the truncated model
        for d in range(4):
            err = abs(horner_quad_model(d, g0, g1, g2, drop)["k"]
                      - horner_quad_model(d, g0, g1, g2)["k"])
            worst = max(worst, err)
    assert worst <= bound
    with pytest.raises(ValueError):
        build_horner_quad(127, 1, 3)  # t = 1-3d underflows: unfit set refuses
    with pytest.raises(ValueError):
        horner_quad_model(3, 10, 31, 3)  # k = 10-66 underflows: refusing
