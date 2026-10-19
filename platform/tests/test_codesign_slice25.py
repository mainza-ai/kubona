"""Slice 25: contracted Forney closure + F24 QROM family.

Production bar: the scan math closes without circularity (kappa constancy
is a measured pin, never an input); the lookup family measures all four
costs with no winner declared beyond the counts.
"""

import pytest

from kubona.codesign import (
    F24,
    KickmixEvaluator,
    berlekamp_massey_toy,
    build_f24_qrom_inv,
    check_decode,
    contracted_phase_bit,
    error_value_from_quotient,
    f24_qrom_planes,
    family_comparison,
    multi_error_syndromes,
    pruned_qrom_spec,
    single_error_syndromes,
    trace_dual,
)
from kubona.codesign.psi import peval, pderivative, reciprocal_psi


def _beta(pos: int) -> int:
    g = 1
    for _ in range(pos):
        g = F24.mul(g, 2)
    return F24.inv(g)


def test_kappa_unity_is_geometric_not_assumed():
    for pos in (0, 3, 5, 9, 14):
        seen = set()
        for err in (1, 5, 9, 13):
            S = single_error_syndromes(F24, 2, pos, err, 6)
            sig = berlekamp_massey_toy(S, F24)["sigma"]
            beta = _beta(pos)
            from kubona.codesign.psi import pmul
            w = peval(pmul(sig, S, F24)[: len(sig) - 1], beta, F24)
            d = peval(pderivative(sig, F24), beta, F24)
            seen.add(F24.mul(F24.mul(err, d), F24.inv(w)))
        assert seen == {1}  # position factor is 1 in this convention, always
    with pytest.raises(ValueError):
        error_value_from_quotient(3, 0, F24)  # repeated root refuses


def test_contracted_equals_direct_forney():
    for pos, err, mask in ((5, 9, 0b1011), (0, 7, 0b1111), (14, 3, 0b0000),
                           (9, 12, 0b0101), (2, 1, 0b1000)):
        S = single_error_syndromes(F24, 2, pos, err, 6)
        sig = berlekamp_massey_toy(S, F24)["sigma"]
        rep = check_decode(sig, S, F24, 2)
        beta = _beta(pos)
        psi = reciprocal_psi(sig, F24)["psi"]
        w = peval(rep["omega"], beta, F24)
        d = peval(pderivative(sig, F24), beta, F24)
        assert error_value_from_quotient(w, d, F24) == err
        mu = trace_dual([(mask >> j) & 1 for j in range(4)], F24)
        got = contracted_phase_bit(mu, w, peval(psi, beta, F24), F24)
        assert got == (bin(mask & err).count("1") & 1)
    with pytest.raises(ValueError):
        contracted_phase_bit(16, 0, 0, F24)  # outside field refuses


def test_f24_qrom_family_proves_and_reports():
    builds, rep = build_f24_qrom_inv()
    assert rep["per_plane_ccx"] == [69, 41, 53, 41]
    assert rep["total_ccx"] == 204
    from kubona.codesign.gf import f24_inversion_table
    table = f24_inversion_table()
    planes = f24_qrom_planes()
    assert len(planes) == 4 and all(len(p) == 16 for p in planes)
    for j, (b, _) in enumerate(builds):
        res = KickmixEvaluator().evaluate(b, pruned_qrom_spec(planes[j]),
                                           num_tests=32, seed=7)
        assert res["valid"] is True and res["tests"] == 16
        from kubona.codesign.kickmix import _sim_trajectory
        import random
        for x in range(16):  # composition: plane j carries bit j, exactly
            init = {q: 0 for q in range(13)}
            for i in range(4):
                init[i] = (x >> i) & 1
            bits, _, _ = _sim_trajectory(b.emit(), init, random.Random(0))
            assert bits[4] == ((table[x] >> j) & 1)
    fam = family_comparison()
    assert fam == {"fermat_naive": 88, "fermat_mbu": 48,
                   "fermat_kara": 27, "qrom_full_lookup": 204,
                   "tower_plain": 60, "tower_mbu": 30}
