"""Slice 49: paper-scale RS decode composition (F2^12, 70 syndromes, degree <= 35).

Production bar: every link is exact on planted instances with known
answers — BM recovers the locator (slice 16), the Chien circuit finds
exactly the planted positions (slice 48's circuit, composed here),
Forney recovers the planted values, and the contracted phase bit equals
the direct trace. No sampling anywhere: root sets are exact by the
degree bound (a degree-d locator has at most d roots), misdecodes
refuse loudly. Unblocks: the split_sigma and EEA degree locks were
toy proof-scope (round-trip gate + Bezout gate verify every degree
live) — lifted, with the old slice-15 refusal comment corrected to
its honest reason (E,O share a factor for that input, still refusing).
"""

import random

import pytest

from kubona.codesign import (
    F2_12,
    berlekamp_massey,
    build_chien_scan,
    check_decode,
    contracted_phase_bit,
    error_value_from_quotient,
    locator_roots,
    multi_error_syndromes,
)
from kubona.codesign.psi import peval, pderivative, reciprocal_psi


def _beta(pos: int) -> int:
    g = 1
    for _ in range(pos):
        g = F2_12.mul(g, 2)
    return F2_12.inv(g)


def _planted(nerr: int):
    return [((i * 137) % 4095 or 1, ((i * 7 + 3) % 4095) or 1) for i in range(nerr)]


def test_paper_scale_decode_composition():
    from kubona.codesign.kickmix import _sim_trajectory
    (b, regs) = build_chien_scan(ncoeffs=36, field=F2_12)
    assert regs["ccx"] == 4095 * 21  # slice-48 pin, reused not re-proven
    ops = b.emit()
    rng = random.Random(49)
    for nerr in (1, 3, 10, 35):
        errs = _planted(nerr)
        syn = multi_error_syndromes(F2_12, 2, errs, 70)
        sig = berlekamp_massey(syn, F2_12)["sigma"]
        rep = check_decode(sig, syn, F2_12, 2)
        assert rep["deg"] == nerr and rep["nroots"] == nerr
        assert rep["tail_zero"] is True
        # math roots == planted positions (slice-16 shape, more errors)
        assert sorted(locator_roots(sig, F2_12, 2)) == sorted(p for p, _ in errs)
        # circuit roots == planted betas (full 85,995T sim per case)
        padded = list(sig) + [0] * (36 - len(sig))
        init = {}
        for k, c in enumerate(padded):
            for i in range(12):
                init[k * 12 + i] = (c >> i) & 1
        bits, _, _ = _sim_trajectory(ops, init, random.Random(0))
        found = {j + 1 for j in range(4095) if bits.get(432 + j, 0) == 1}
        assert found == {_beta(p) for p, _ in errs}
        # Forney values == planted values; contracted phase == direct trace
        psi = reciprocal_psi(sig, F2_12)["psi"]
        dsig = pderivative(sig, F2_12)
        for p, err in errs:
            beta = _beta(p)
            w = peval(rep["omega"], beta, F2_12)
            d = peval(dsig, beta, F2_12)
            assert error_value_from_quotient(w, d, F2_12) == err
            mu = rng.randrange(1, 4096)
            got = contracted_phase_bit(mu, w, peval(psi, beta, F2_12), F2_12)
            assert got == F2_12.trace(F2_12.mul(mu, err))


def test_misdecode_past_capacity_refuses():
    # 36 errors against 70 syndromes (capacity 35): the locator cannot
    # split into 36 roots — loud refusal, counted not silent.
    errs = [(i, (i * 11 + 5) % 4095 or 1) for i in range(36)]
    syn = multi_error_syndromes(F2_12, 2, errs, 70)
    sig = berlekamp_massey(syn, F2_12)["sigma"]
    with pytest.raises(ValueError):
        check_decode(sig, syn, F2_12, 2)
