"""Slice 27: Chien scan + tower-field inversion.

Production bar: the scan proves over its full domain (linear evals are
CNOT-only, zero tests carry the Toffolis); the tower agrees with the
polynomial inverter on all 16 elements with the basis change gated both
ways. Sampler/evaluator consistency (zeroed non-inputs) is pinned by the
scan itself, which failed pre-fix on seed 7's first draw.
"""

import random

import pytest

from kubona.codesign import (
    F24,
    KickmixEvaluator,
    build_chien_scan,
    chien_scan_spec,
    f24_inversion_table,
    from_tower,
    gf4_add,
    gf4_inv,
    gf4_mul,
    to_tower,
    tower_invert,
    tower_mul,
)


def test_chien_scan_full_domain():
    (b, regs) = build_chien_scan()
    assert regs["ccx"] == 75 and regs["positions"] == 15
    res = KickmixEvaluator().evaluate(b, chien_scan_spec(), num_tests=512, seed=7)
    assert res["valid"] is True and res["tests"] == 512
    # decode -> scan composition: located errors are exactly the found set
    from kubona.codesign import berlekamp_massey_toy, check_decode, single_error_syndromes
    from kubona.codesign.kickmix import _sim_trajectory
    for pos, err in ((3, 6), (11, 9)):
        S = single_error_syndromes(F24, 2, pos, err, 6)
        sig = berlekamp_massey_toy(S, F24)["sigma"]
        check_decode(sig, S, F24, 2)
        init = {q: 0 for q in range(33)}
        for k, c in enumerate(sig + [0] * (3 - len(sig))):
            for i in range(4):
                init[k * 4 + i] = (c >> i) & 1
        bits, _, _ = _sim_trajectory(b.emit(), init, random.Random(0))
        found = [j for j in range(15) if bits[12 + j] == 1]
        # root positions j satisfy gamma^{-j} == beta: element (pos 3 -> 15)
        g = 1
        for _ in range(pos):
            g = F24.mul(g, 2)
        assert found == [F24.inv(g) - 1]
    with pytest.raises(ValueError):
        build_chien_scan(ncoeffs=0)  # empty coefficient vector refuses
    with pytest.raises(ValueError):
        build_chien_scan(ncoeffs=True)  # bool is not an int
    # the old ncoeffs=4 cap was proof-scope (2^16 sigma enumeration),
    # dissolved by slice 48's linearity architecture: it builds now
    (_, regs4) = build_chien_scan(ncoeffs=4)
    assert regs4["positions"] == 15 and regs4["ccx"] == 15 * 5


def test_tower_field_agreement_and_gates():
    for a in range(4):
        for c in range(4):
            assert gf4_add(a, c) == (a ^ c)
    assert to_tower(0) == (0, 0)
    table = f24_inversion_table()
    for x in range(16):
        assert tower_invert(x) == table[x]  # all 16, incl. 0 -> 0
    rng = random.Random(0)
    for _ in range(60):  # tower_mul sound vs F16 on random pairs
        A = (rng.randrange(4), rng.randrange(4))
        B = (rng.randrange(4), rng.randrange(4))
        assert from_tower(*tower_mul(*A, *B)) == F24.mul(from_tower(*A), from_tower(*B))
    assert gf4_inv(2) == 3 and gf4_mul(2, 3) == 1  # t * (t+1) = 1
    with pytest.raises(ValueError):
        gf4_inv(0)
    with pytest.raises(ValueError):
        tower_invert(16)
    with pytest.raises(ValueError):
        from_tower(4, 0)
