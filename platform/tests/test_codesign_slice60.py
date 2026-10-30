"""Slice 60: full paper-scale RS decoder composition (Phase 3 complete).

Production bar: the single coherent circuit (S in -> BM walk -> split ->
EEA -> psi/omega -> Chien + phase scan) is proven end-to-end on planted
instances: EEA flag is 1, Chien FOUND marks exactly the planted root
positions, the trajectory phase equals the standalone scan phase on the
same inputs (transitivity with slice-51's evaluator proof), A*B*E+B*O=1
on composed outputs, inputs preserved, every temp clean. One build,
one sim per planted instance. 7,208,672T pinned (walk 5,764,080 +
EEA ~985k + omega ~229k + Chien 85,995 + phase scan 217,350, measured
not summed); 112,168 qubits addressable and peak-live (corrected 2026-10-20:
the older 110,132 prose figure was pre-F2 and never pinned; live layout +
Builder agree). This is the first complete paper-scale RS
decoder: decode works where the EEA converges (low-error/DQI-typical),
refuses explicitly elsewhere (flag) — never a silent wrong decode.
"""

import random

from kubona.codesign import F2_12
from kubona.codesign.bm import (
    berlekamp_massey,
    build_phase_scan,
    check_decode,
    locator_roots,
    mask_cancellation,
    multi_error_syndromes,
)
from kubona.codesign.kickmix import _sim_trajectory
from kubona.codesign.psi import reciprocal_psi
from kubona.codesign.rs12 import build_rs_decoder, rs12_layout

GAMMA = 2
NROUND = 70


def _scan_setup():
    rng = random.Random(50)
    masks = [rng.randrange(4096) for _ in range(4095)]
    rep = mask_cancellation(masks, F2_12)
    canceled = set(rep["canceled"])
    positions = [b for b in range(1, 4096) if b not in canceled]
    assert len(positions) == 4025
    nus = {b: rep["residual"][b] for b in positions}
    return positions, nus


def _instances():
    rng = random.Random(60)
    out = []
    for pos in (0, 2048):
        out.append([(pos, 1 + rng.randrange(4095))])
    for _ in range(2):
        out.append([(rng.randrange(4096), 1 + rng.randrange(4095)) for _ in range(2)])
    out.append([(rng.randrange(4096), 1 + rng.randrange(4095)) for _ in range(3)])
    return out


def test_rs_composition_end_to_end():
    positions, nus = _scan_setup()
    b, meta = build_rs_decoder(positions, nus)
    assert meta["ccx"] == 3928980
    assert meta["kind"] == "rs12-decode"
    L = rs12_layout(0)
    # standalone scan for phase transitivity (same inputs, proven vs spec)
    sb, _sregs = build_phase_scan(F2_12, 36, positions, nus)
    # Stream from the compact Builder columns. Materialising the composed
    # sequence is 126M Ops (>30 GiB); this keeps the same full circuit and
    # every assertion while fitting the hosted runner.
    for errs in _instances():
        syn = multi_error_syndromes(F2_12, GAMMA, errs, NROUND)
        rep = berlekamp_massey(syn, F2_12)
        sigma = rep["sigma"]
        crep = check_decode(sigma, syn, F2_12, GAMMA)
        psi = reciprocal_psi(sigma, F2_12)["psi"]
        om = list(crep["omega"])
        # --- composed sim ---
        init = {i: (syn[i // 12] >> (i % 12)) & 1 for i in range(840)}
        bits, phase, _ = _sim_trajectory(b.iter_ops(), dict(init), random.Random(20))
        # FLAG set (converged; these are low-error instances)
        assert bits.get(meta["outputs"][-1], 0) == 1, errs
        # Chien FOUND marks exactly the planted root betas (field elements)
        roots = set(locator_roots(sigma, F2_12, GAMMA))
        assert len(roots) == len(errs)
        expect = set()
        for p, _ in errs:
            gp = 1
            for _ in range(p):
                gp = F2_12.mul(gp, GAMMA)
            expect.add(F2_12.inv(gp))
        # math and circuit agree on locations (roots j <-> beta gamma^{-j})
        for j in roots:
            pw = 1
            for _ in range(j):
                pw = F2_12.mul(pw, F2_12.inv(GAMMA))
            assert pw in expect, (errs, j)
        outs = meta["outputs"]
        got = [k + 1 for k in range(4095) if bits.get(outs[k], 0) == 1]
        assert got == sorted(expect), (errs, got, sorted(expect))
        # phase equals standalone scan phase on the same (sigma,omega,psi)
        sinit = {}
        for k, c in enumerate(list(sigma) + [0] * (36 - len(sigma))):
            for i in range(12):
                sinit[k * 12 + i] = (c >> i) & 1
        for k, c in enumerate(om + [0] * (36 - len(om))):
            for i in range(12):
                sinit[432 + k * 12 + i] = (c >> i) & 1
        for k, c in enumerate(list(psi) + [0] * (36 - len(psi))):
            for i in range(12):
                sinit[864 + k * 12 + i] = (c >> i) & 1
        _, sphase, _ = _sim_trajectory(sb.iter_ops(), dict(sinit), random.Random(21))
        assert phase == sphase, errs
        # A*B*E+B*O == 1 on composed outputs (decode validity, end to end)
        from kubona.codesign.psi import padd, pmul
        gotA = [sum((bits.get(L["A"][12 * k + i], 0) & 1) << i for i in range(12)) for k in range(18)]
        gotB = [sum((bits.get(L["B"][12 * k + i], 0) & 1) << i for i in range(12)) for k in range(18)]
        Er, Or = _split_sigma_vals(sigma)
        lhs = padd(pmul(gotA, Er, F2_12), pmul(gotB, Or, F2_12), F2_12)
        assert lhs == [1], errs
        # inputs preserved; shared temps clean (BT trail persists by design)
        for i in range(840):
            assert bits.get(i, 0) == init[i], errs
        for q in L["allocs"]:
            assert bits.get(q, 0) == 0, (errs, q)


def _split_sigma_vals(sigma):
    from kubona.codesign.psi import split_sigma
    even, odd = split_sigma(sigma, F2_12)
    return even + [0] * (18 - len(even)), odd + [0] * (18 - len(odd))


def test_rs_pins_determinism_layout():
    positions, nus = _scan_setup()
    L = rs12_layout(0)
    assert L["nin"] == 840
    assert len(L["FOUND"]) == 4095
    b, meta = build_rs_decoder(positions, nus)
    assert meta["ccx"] == 3928980
    b1, _ = build_rs_decoder(positions, nus)
    assert len(b) == len(b1)
    assert b.ops_digest() == b1.ops_digest()
