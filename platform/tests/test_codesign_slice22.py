"""Slice 22: toy DQI acceptance loop + Coulomb accumulator.

Production bar: integration proved, not assembled — sampled errors decode
to exact positions through the verified chain (syndromes -> rescaled BM ->
key-equation verdict), and the accumulator is bit-exact over its full
domain with the uncopy fault caught.
"""

import random

import pytest

from kubona.codesign import (
    F24,
    KickmixEvaluator,
    berlekamp_massey_toy,
    build_coulomb_acc,
    check_decode,
    coulomb_acc_spec,
    coulomb_d2_model,
    dqi_loss_bound,
    eps_from_gate,
    locator_roots,
    multi_error_syndromes,
    single_error_syndromes,
)


def test_toy_dqi_acceptance_loop():
    single = multi = 0
    for t in range(60):
        rng = random.Random(t)
        pos, err = rng.randrange(15), rng.randrange(1, 16)
        S = single_error_syndromes(F24, 2, pos, err, 6)
        rep = check_decode(berlekamp_massey_toy(S, F24)["sigma"], S, F24, 2)
        assert rep["roots"] == [pos]
        single += 1
    for t in range(60, 80):
        rng = random.Random(t)
        p1, p2 = rng.sample(range(15), 2)
        S = multi_error_syndromes(F24, 2, [(p1, 3), (p2, 5)], 6)
        rep = check_decode(berlekamp_massey_toy(S, F24)["sigma"], S, F24, 2)
        assert sorted(rep["roots"]) == sorted([p1, p2])
        multi += 1
    assert (single, multi) == (60, 20)
    # acceptance gate composes: 100/100k detected -> loss bound, shape-pinned
    eps = eps_from_gate(100_000, allowed_failures=100)
    assert 0 < eps < 0.02
    assert dqi_loss_bound(eps, 0.5763) == pytest.approx(4 * eps * 0.5763)
    with pytest.raises(ValueError):
        dqi_loss_bound(eps, -0.1)


def test_coulomb_accumulator_full_domain():
    assert max(coulomb_d2_model(x, y, z) for x in range(4)
               for y in range(4) for z in range(4)) == 27  # 5-bit acc exact
    (b, regs) = build_coulomb_acc()
    assert regs["ccx"] == 276
    res = KickmixEvaluator().evaluate(b, coulomb_acc_spec(), num_tests=128, seed=7)
    assert res["valid"] is True and res["tests"] == 64
    with pytest.raises(ValueError):
        coulomb_d2_model(4, 0, 0)  # coordinate overflows 2 bits: refusing


def test_coulomb_uncopy_fault_caught():
    from kubona.codesign import Builder
    from kubona.codesign.coulomb import C_BITS
    b = Builder(24)
    for q in range(11, 24):
        b.alloc(q)
    b.cx(0, 11)  # copy without uncopy: ancilla fraud
    for q in range(11, 24):
        try:
            b.free(q)
        except ValueError:
            pass
    res = KickmixEvaluator().evaluate(b, coulomb_acc_spec(), num_tests=8, seed=7)
    assert res["valid"] is False
    assert C_BITS == 2
