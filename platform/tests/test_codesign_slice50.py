"""Slice 50: mask cancellation + full-scan phase composition (paper scale).

Production bar: the composition is a theorem checked on planted
instances, not an approximation — direct phase == linear part +
scan-sum, exactly, because the residual vanishes on the canceled set
(verified live) and the contracted bit equals the direct trace at every
root (slice 49). The offline solver is square Vandermonde (70 distinct
betas always solve; the solution is verified by residual); 71 refuses
structurally (only 70 syndromes back the linear part — the paper's 71st
needs another degree of freedom, noted not closed). One planted error
sits at canceled beta=1 throughout, so both paths (linear-carried and
scan-carried) are exercised every case.
"""

import random

import pytest

from kubona.codesign import (
    F2_12,
    berlekamp_massey,
    check_decode,
    contracted_phase_bit,
    mask_cancellation,
    multi_error_syndromes,
    trace_dual,
)
from kubona.codesign.bm import _solve_field_system
from kubona.codesign.psi import peval, pderivative, reciprocal_psi

MASKS = None


def _masks():
    global MASKS
    if MASKS is None:
        rng = random.Random(50)
        MASKS = [rng.randrange(4096) for _ in range(4095)]
    return MASKS


def _mus(masks):
    return [trace_dual([(masks[b - 1] >> t) & 1 for t in range(12)], F2_12)
            for b in range(1, 4096)]


def _beta(pos):
    g = 1
    for _ in range(pos):
        g = F2_12.mul(g, 2)
    return F2_12.inv(g)


@pytest.mark.slow  # paper-scale ~25s locally; CI slow lane
def test_cancellation_solves_and_verifies():
    rep = mask_cancellation(_masks(), F2_12)
    assert rep["ncancel"] == 70 and len(rep["canceled"]) == 70
    assert rep["canceled"] == list(range(1, 71)) and len(rep["a"]) == 70
    assert all(rep["residual"][b] == 0 for b in rep["canceled"])
    # the rest is genuinely nonzero work for the scan (not all canceled)
    assert sum(1 for b in range(1, 4096) if rep["residual"][b] != 0) > 4000
    # determinism: no iteration order leaks into the solve
    assert mask_cancellation(_masks(), F2_12) == rep


def test_seventy_first_position_refuses_structurally():
    # Only the 70 supplied syndromes back the linear part; a 71st
    # coefficient would have no syndrome to ride on. The paper's 71 needs
    # another degree of freedom (mask structure or extra input) — noted,
    # not closed, never fudged by solving anyway.
    with pytest.raises(ValueError):
        mask_cancellation(_masks(), F2_12, ncancel=71)
    with pytest.raises(ValueError):
        mask_cancellation(_masks(), F2_12, ncancel=0)
    with pytest.raises(ValueError):
        mask_cancellation(_masks(), F2_12, ncancel=True)
    with pytest.raises(ValueError):
        mask_cancellation([0] * 10, F2_12)
    with pytest.raises(ValueError):
        mask_cancellation([0] * 4095 + [1], F2_12)
    with pytest.raises(ValueError):
        _solve_field_system([[1, 2], [2, 4]], [1, 1], F2_12)  # singular


@pytest.mark.slow  # paper-scale ~25s locally; CI slow lane
def test_full_scan_phase_composition():
    masks = _masks()
    mus = _mus(masks)
    rep = mask_cancellation(masks, F2_12)
    K = set(rep["canceled"])
    cases = []
    for nerr in (1, 3, 10, 35):
        # anchor at canceled beta=1 exercises the linear-carried path
        cases.append([(0, 7)] + [((i * 137) % 4094 + 1, ((i * 7 + 3) % 4095) or 1)
                                for i in range(1, nerr)])
    # no canceled anchor: pure scan-carried path (positions all > 70)
    cases.append([((i * 251) % 4024 + 71, ((i * 13 + 5) % 4095) or 1)
                  for i in range(10)])
    for errs in cases:
        syn = multi_error_syndromes(F2_12, 2, errs, 70)
        sig = berlekamp_massey(syn, F2_12)["sigma"]
        crep = check_decode(sig, syn, F2_12, 2)
        assert crep["deg"] == len(errs) and crep["nroots"] == len(errs)
        # direct phase over the planted support
        direct = 0
        betas = {}
        for p, e in errs:
            b = _beta(p)
            betas[p] = b
            direct ^= F2_12.trace(F2_12.mul(mus[b - 1], e))
        # linear part from the 70 syndromes
        lin = 0
        for i, ai in enumerate(rep["a"], start=1):
            lin ^= F2_12.trace(F2_12.mul(ai, syn[i - 1]))
        # scan sum over uncanceled roots (49: contracted == direct trace)
        psi = reciprocal_psi(sig, F2_12)["psi"]
        dsig = pderivative(sig, F2_12)
        om = crep["omega"]
        scan = 0
        for p, e in errs:
            b = betas[p]
            if b in K:
                continue
            w = peval(om, b, F2_12)
            d = peval(dsig, b, F2_12)
            assert F2_12.mul(w, F2_12.inv(d)) == e  # Forney at scale (49)
            scan ^= contracted_phase_bit(rep["residual"][b], w,
                                         peval(psi, b, F2_12), F2_12)
        assert direct == (lin ^ scan), len(errs)
