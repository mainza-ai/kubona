"""Slice 51: root-gated bilinear phase-scan circuit (paper SI RS scan).

Production bar: the circuit is held to the Layer-1 scan formula by
check_one (phase verified, ancillas zero, inputs preserved), and the
formula is anchored to planted truth per instance (direct == linear +
scan via the 50/49 machinery re-verified here, not remembered). The
bilinear decomposition is basis-proven (144 unit pairs per nu); the
emission is uniform, so planted full-circuit sims prove the wiring.
Measured at paper scale: 217,350T over 4025 positions (54T each:
42T root test+cleanup + 12 products — products match the paper
Toffoli-for-Toffoli; the gap is purely our 42T zero-test+cleanup vs
their hand-built 11T, stated not closed). The 71st-position and
measurement-uncompute variants stay queued.
"""

import random

import pytest

from kubona.codesign import (
    F2_12,
    F24,
    KickmixEvaluator,
    berlekamp_massey,
    berlekamp_massey_toy,
    bilinear_terms,
    build_phase_scan,
    check_decode,
    mask_cancellation,
    multi_error_syndromes,
    pderivative,
    phase_scan_spec,
    single_error_syndromes,
    trace_dual,
)
from kubona.codesign.psi import peval, reciprocal_psi


def _beta(pos, field):
    g = 1
    for _ in range(pos):
        g = field.mul(g, 2)
    return field.inv(g)


def test_bilinear_terms_basis_exhaustive():
    # unit pair (2^wi, 2^pj): term k contributes bit wi of a_k AND bit wj
    # of b_k; agreement on all 144 spans every (w, psi) by linearity.
    assert bilinear_terms(0, F2_12) == []
    rng = random.Random(51)
    for nu in [1, 7] + [rng.randrange(1, 4096) for _ in range(5)]:
        terms = bilinear_terms(nu, F2_12)
        assert 1 <= len(terms) <= 12
        for wi in range(12):
            for pj in range(12):
                got = 0
                for am, bm in terms:
                    got ^= ((am >> wi) & 1) & ((bm >> pj) & 1)
                want = F2_12.trace(F2_12.mul(nu, F2_12.mul(1 << wi, 1 << pj)))
                assert got == want, (nu, wi, pj)


def _paper_setup():
    rng = random.Random(50)
    masks = [rng.randrange(4096) for _ in range(4095)]
    rep = mask_cancellation(masks, F2_12)
    K = set(rep["canceled"])
    positions = [b for b in range(1, 4096) if b not in K]
    nus = {b: rep["residual"][b] for b in positions}
    mus = [trace_dual([(masks[bb - 1] >> t) & 1 for t in range(12)], F2_12)
           for bb in range(1, 4096)]
    return masks, mus, rep, K, positions, nus


@pytest.mark.slow  # paper-scale ~142s locally; CI slow lane
def test_phase_scan_paper_scale():
    _masks, mus, rep, K, positions, nus = _paper_setup()
    assert len(positions) == 4025
    (b, regs) = build_phase_scan(F2_12, 36, positions, nus)
    assert regs["kind"] == "phase-scan" and regs["ncoeffs"] == 36
    assert regs["positions"] == tuple(positions)
    assert regs["ccx"] == sum(42 + r for r in regs["ranks"])
    assert regs["ccx"] == 217350 and set(regs["ranks"]) == {12}
    ev = KickmixEvaluator()
    cases = [
        [(0, 7), (100, 11), (2000, 13)],
        [((i * 137) % 4094 + 1, ((i * 7 + 3) % 4095) or 1) for i in range(35)],
    ]
    for errs in cases:
        syn = multi_error_syndromes(F2_12, 2, errs, 70)
        sig = berlekamp_massey(syn, F2_12)["sigma"]
        crep = check_decode(sig, syn, F2_12, 2)
        assert crep["deg"] == len(errs) and crep["nroots"] == len(errs)
        psi = reciprocal_psi(sig, F2_12)["psi"]
        dsig = pderivative(sig, F2_12)
        om = list(crep["omega"])
        assert len(sig) <= 36 and len(om) <= 36 and len(psi) <= 36
        init = {}
        for k, c in enumerate(list(sig) + [0] * (36 - len(sig))):
            for i in range(12):
                init[k * 12 + i] = (c >> i) & 1
        for k, c in enumerate(om + [0] * (36 - len(om))):
            for i in range(12):
                init[432 + k * 12 + i] = (c >> i) & 1
        ps = list(psi)
        for k, c in enumerate(ps + [0] * (36 - len(ps))):
            for i in range(12):
                init[864 + k * 12 + i] = (c >> i) & 1
        res = ev.check_one(b, phase_scan_spec(F2_12, 36, positions, nus),
                           init, random.Random(3))
        assert res.valid, (errs[0], res.reason)
        # anchor to planted truth (not just to the Layer-1 formula)
        direct = lin = scan = 0
        for p, e in errs:
            bb = _beta(p, F2_12)
            direct ^= F2_12.trace(F2_12.mul(mus[bb - 1], e))
            if bb not in K:
                w = peval(om, bb, F2_12)
                assert F2_12.mul(w, F2_12.inv(peval(dsig, bb, F2_12))) == e
                scan ^= F2_12.trace(F2_12.mul(rep["residual"][bb], e))
        for i, ai in enumerate(rep["a"], start=1):
            lin ^= F2_12.trace(F2_12.mul(ai, syn[i - 1]))
        assert direct == (lin ^ scan)
        assert res.phase == scan


def test_phase_scan_toy_composition_and_determinism():
    # real end-to-end at toy scale (mask -> cancel -> scan -> phase),
    # plus a determinism double-run of the identical emission path.
    rng = random.Random(52)
    masks = [rng.randrange(16) for _ in range(15)]
    rep = mask_cancellation(masks, F24, ncancel=5)
    K = set(rep["canceled"])
    positions = [b for b in range(1, 16) if b not in K]
    assert len(positions) == 10
    nus = {b: rep["residual"][b] for b in positions}
    (b1, r1) = build_phase_scan(F24, 3, positions, nus)
    (b2, r2) = build_phase_scan(F24, 3, positions, nus)
    assert r1 == r2 and b1.emit() == b2.emit()
    pos, err = 3, 9
    S = single_error_syndromes(F24, 2, pos, err, 6)
    sig = berlekamp_massey_toy(S, F24)["sigma"]
    crep = check_decode(sig, S, F24, 2)
    assert crep["deg"] == 1 and crep["nroots"] == 1
    from kubona.codesign.psi import pmul, reciprocal_psi
    om = pmul(list(sig), list(S), F24)[: len(sig) - 1]
    psi = reciprocal_psi(sig, F24)["psi"]
    assert len(sig) <= 3 and len(om) <= 3 and len(psi) <= 3
    init = {}
    for k, c in enumerate(list(sig) + [0] * (3 - len(sig))):
        for i in range(4):
            init[k * 4 + i] = (c >> i) & 1
    for k, c in enumerate(list(om) + [0] * (3 - len(om))):
        for i in range(4):
            init[12 + k * 4 + i] = (c >> i) & 1
    ps = list(psi)
    for k, c in enumerate(ps + [0] * (3 - len(ps))):
        for i in range(4):
            init[24 + k * 4 + i] = (c >> i) & 1
    res = KickmixEvaluator().check_one(
        b1, phase_scan_spec(F24, 3, positions, nus), init, random.Random(4))
    assert res.valid, res.reason
    # same composition anchor as paper scale (canceled or not)
    beta = _beta(pos, F24)
    w = peval(om, beta, F24)
    assert F24.mul(w, F24.inv(peval(pderivative(sig, F24), beta, F24))) == err
    direct = F24.trace(F24.mul(trace_dual([(masks[beta - 1] >> t) & 1
                                           for t in range(4)], F24), err))
    lin = 0
    for i, ai in enumerate(rep["a"], start=1):
        lin ^= F24.trace(F24.mul(ai, S[i - 1]))
    scan = 0 if beta in K else F24.trace(F24.mul(rep["residual"][beta], err))
    assert direct == (lin ^ scan)
    assert res.phase == scan


def test_phase_scan_refusals():
    with pytest.raises(ValueError):
        build_phase_scan(F2_12, 36, [], {})
    with pytest.raises(ValueError):
        build_phase_scan(F2_12, 0, [1], {1: 0})
    with pytest.raises(ValueError):
        build_phase_scan(F2_12, True, [1], {1: 0})
    with pytest.raises(ValueError):
        build_phase_scan(F2_12, 36, [0], {0: 0})
    with pytest.raises(ValueError):
        build_phase_scan(F2_12, 36, [4096], {4096: 0})
    with pytest.raises(ValueError):
        build_phase_scan(F2_12, 36, [1, 1], {1: 0})
    with pytest.raises(ValueError):
        build_phase_scan(F2_12, 36, [1, 2], {1: 0})
    with pytest.raises(ValueError):
        build_phase_scan(F2_12, 36, [1, 2], {1: 0, 2: 0, 3: 0})
    with pytest.raises(ValueError):
        build_phase_scan(F2_12, 36, [1], {1: 4096})
    with pytest.raises(ValueError):
        build_phase_scan(object(), 36, [1], {1: 0})
    with pytest.raises(ValueError):
        phase_scan_spec(F2_12, 36, [1, 1], {1: 0})
