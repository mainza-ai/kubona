"""Slice 48: Chien root-scan at paper scale (F2^12, degree <= 35, 4095 positions).

Production bar: the old ncoeffs<=3 lock was proof-scope (exhaustive
sigma-space needs 2^(4*ncoeffs) trajectories), not a circuit limit —
dissolved by the linearity argument, not by sampling. VAL is F2-linear
in the coefficient bits, so the basis-exhaustive mask check below
proves VAL == sigma(beta) for every coefficient vector and every
position with zero sigma-space enumeration; the 12-bit zero-test is
proven exhaustive on all 4096 inputs; planted locators (degree 35 and
2, exact root sets by the degree bound) prove the wired circuit end to
end. Measured: 85,995T over 4095 positions (21T each, CNOT-only VAL),
sigma 432 bits. The toy default is byte-identical (75T/15 positions —
slice 27 pins hold); its ncoeffs=4 refusal probe is updated to the new
legal behavior, with the strength moved into the basis architecture
here. This completes the RS decode chain at paper params alongside
slice 16's paper-scale BM (70 syndromes, degree <= 35).
"""

import random

import pytest

from kubona.codesign import (
    F2_12,
    KickmixEvaluator,
    build_chien_scan,
    chien_scan_spec,
)
from kubona.codesign.bm import _chien_masks
from kubona.codesign.packing import emit_zero_test


@pytest.mark.slow  # paper-scale ~9s locally; CI slow lane
def test_masks_basis_exhaustive_paper_params():
    # For every position beta and every sigma basis bit: the mask claims
    # the contribution iff direct field evaluation has it. By F2-linearity
    # this decides VAL == sigma(beta) on the full 2^432 sigma space.
    n, ncoeffs = 12, 36
    for beta in range(1, 4096):
        masks = _chien_masks(F2_12, beta, ncoeffs)
        sets = [set(m) for m in masks]
        pw = 1
        for k in range(ncoeffs):
            for i in range(n):
                c = F2_12.mul(1 << i, pw)
                bit_id = k * n + i
                for t in range(n):
                    assert ((bit_id in sets[t]) == (((c >> t) & 1) == 1)), (beta, k, i, t)
            pw = F2_12.mul(pw, beta)


def test_zero_test_12bit_exhaustive():
    # The scan's only Toffoli component, proven on all 4096 inputs.
    from kubona.codesign.kickmix import Builder, TaskSpec
    n = 12
    b = Builder(1 + n + (n - 2))
    g = list(range(1, 1 + n))
    temps = list(range(1 + n, 1 + n + n - 2))
    assert emit_zero_test(b, g, 0, temps) == 2 * n - 3
    ev = KickmixEvaluator()

    def _perm(bits):
        out = dict(bits)
        out[0] = 1 if all((bits.get(q, 0) & 1) == 0 for q in g) else 0
        return out

    spec = TaskSpec(num_qubits=1 + n + n - 2, inputs=tuple(g), outputs=(0,),
                    ancillas=tuple(temps), target_perm=_perm)
    for mask in range(1 << n):
        init = {q: (mask >> (q - 1)) & 1 for q in g}
        res = ev.check_one(b, spec, init, random.Random(0))
        assert res.valid, (mask, res.reason)


def _planted_coeffs(roots):
    # sigma(x) = prod_r (x + beta_r) over F2_12 (char 2: minus is plus).
    poly = [1]
    for r in roots:
        nxt = [0] * (len(poly) + 1)
        for i, c in enumerate(poly):
            nxt[i] ^= F2_12.mul(c, r)
            nxt[i + 1] ^= c
        poly = nxt
    return poly


def _direct_roots(coeffs):
    # Horner evaluation (field law: XOR accumulation, never integer sum).
    out = []
    for beta in range(1, 4096):
        acc = 0
        for c in reversed(coeffs):
            acc = F2_12.mul(acc, beta) ^ c
        if acc == 0:
            out.append(beta)
    return out


@pytest.mark.slow  # paper-scale ~31s locally; CI slow lane
def test_planted_roots_end_to_end():
    # Exact root sets by the degree bound (no sampling, no flake):
    # degree-35 -> exactly the 35 planted roots; degree-2 -> its two.
    from kubona.codesign.kickmix import _sim_trajectory
    (b, regs) = build_chien_scan(ncoeffs=36, field=F2_12)
    assert regs["ccx"] == 4095 * 21 and regs["positions"] == 4095
    assert regs["ncoeffs"] == 36 and regs["nbits"] == 12
    assert len(regs["sigma"]) == 432 and len(regs["found"]) == 4095
    ops = b.emit()
    for roots in (list(range(1, 36)), [7, 1234]):
        coeffs = _planted_coeffs(roots) + [0] * (36 - len(_planted_coeffs(roots)))
        assert _direct_roots(coeffs) == sorted(roots)
        init = {}
        for k, c in enumerate(coeffs):
            for i in range(12):
                init[k * 12 + i] = (c >> i) & 1
        bits, _, _ = _sim_trajectory(ops, init, random.Random(0))
        # untouched qubits stay |0> without entering the dict: .get, not []
        found = [j + 1 for j in range(4095) if bits.get(432 + j, 0) == 1]
        assert found == sorted(roots), roots[:3]
    # spec agreement on the degree-2 planted case (inputs + ancillas checked)
    coeffs = _planted_coeffs([7, 1234]) + [0] * 34
    init = {}
    for k, c in enumerate(coeffs):
        for i in range(12):
            init[k * 12 + i] = (c >> i) & 1
    res = KickmixEvaluator().check_one(b, chien_scan_spec(ncoeffs=36, field=F2_12),
                                       init, random.Random(1))
    assert res.valid, res.reason


def test_scan_refusals_and_regressions():
    with pytest.raises(ValueError):
        build_chien_scan(ncoeffs=0)
    with pytest.raises(ValueError):
        build_chien_scan(ncoeffs=True)
    with pytest.raises(ValueError):
        build_chien_scan(ncoeffs=3, field=object())
    with pytest.raises(ValueError):
        chien_scan_spec(ncoeffs=0)
    # toy default byte-identical (slice 27 pins hold, strengthened here)
    (b, regs) = build_chien_scan()
    assert regs["ccx"] == 75 and regs["positions"] == 15
    assert regs["ncoeffs"] == 3 and regs["nbits"] == 4
    (b4, regs4) = build_chien_scan(ncoeffs=4)  # legal now: linearity covers it
    assert regs4["positions"] == 15
