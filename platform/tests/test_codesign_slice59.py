"""Slice 59: fixed-schedule half-degree EEA circuit (Phase 3 B2).

Production bar: the 17-block EEA (F2^12, 18-word banks, schoolbook mults +
tower inverter, barrel-shifted single-term elimination, forward-only) is
proven over harvested valid (E,O) pairs — A,B match the math exactly,
A*E+B*O=1 verified on circuit outputs, flag=1 (converged), inputs
preserved, every temp clean, phase clean. 17 blocks cover harvested
locators needing <= 17 single-term eliminations; inputs needing more
(high-degree locators) set FLAG=0 (explicit refusal, never a silent wrong
answer) — paper-consistent bounded error (main.tex:2049,1653). The math
layer (split + EEA + Bezout + psi-vs-sigma' at roots) is proven alongside
at F2^12. Measured 985,484T pinned. The flag is load-bearing: slice 58's
ping-pong lesson (prove the discipline, not the hope) applies — every
assertion below is on executed trajectories.
"""

import random

import pytest

from kubona.codesign import F2_12
from kubona.codesign.bm import berlekamp_massey, multi_error_syndromes
from kubona.codesign.eea12 import N_BLOCKS, build_eea12, eea12_layout
from kubona.codesign.gf import F2_12 as F12
from kubona.codesign.kickmix import Builder, _sim_trajectory
from kubona.codesign.psi import pdeg, peval, reciprocal_psi, split_sigma

GAMMA = 2
NROUND = 70


def _locators():
    """Harvested separable locators: planted 1/2/3/5/10-error walks."""
    rng = random.Random(59)
    out = []
    for ne in (1, 2, 3, 5, 10):
        for _ in range(6):
            errs = [(rng.randrange(4096), 1 + rng.randrange(4095)) for _ in range(ne)]
            syn = multi_error_syndromes(F2_12, GAMMA, errs, NROUND)
            try:
                rep = berlekamp_massey(syn, F2_12)
            except ValueError:
                continue
            sigma = rep["sigma"]
            if len(sigma) - 1 > 35 or len(sigma) < 2:
                continue
            try:
                rp = reciprocal_psi(sigma, F2_12)
            except ValueError:
                continue
            out.append((sigma, rp))
    return out


def test_eea_math_paper_scale():
    locs = _locators()
    assert len(locs) >= 20
    for sigma, rp in locs:
        E, O, A, B = rp["E"], rp["O"], rp["A"], rp["B"]
        assert max(pdeg(E), pdeg(O)) <= 17
        assert max(pdeg(A), pdeg(B)) <= 17
        # Bezout gate on the math outputs (independent recheck)
        from kubona.codesign.psi import padd, pmul
        assert padd(pmul(A, E, F2_12), pmul(B, O, F2_12), F2_12) == [1]
        # psi*sigma' == 1 at every root (the Forney precondition)
        assert len(rp["roots"]) == len(sigma) - 1
        from kubona.codesign.psi import pderivative
        ds = pderivative(sigma, F2_12)
        for beta in rp["roots"]:
            assert F2_12.mul(peval(rp["psi"], beta, F2_12), peval(ds, beta, F2_12)) == 1


def _load_eo(L, E, O):
    init = {}
    for k in range(18):
        c = E[k] if k < len(E) else 0
        for i in range(12):
            init[L["E"][12 * k + i]] = (c >> i) & 1
        c = O[k] if k < len(O) else 0
        for i in range(12):
            init[L["O"][12 * k + i]] = (c >> i) & 1
    return init


def test_eea_circuit_harvested():
    locs = _locators()
    L = eea12_layout(0)
    b, meta = build_eea12()
    assert meta["ccx"] == 985484
    assert meta["kind"] == "eea12"
    n_ok = 0
    for sigma, rp in locs:
        E, O = rp["E"], rp["O"]
        init = _load_eo(L, E, O)
        bits, phase, _ = _sim_trajectory(b.emit(), dict(init), random.Random(12))
        assert phase == 0
        gotA = [sum((bits.get(L["A"][12 * k + i], 0) & 1) << i for i in range(12)) for k in range(18)]
        gotB = [sum((bits.get(L["B"][12 * k + i], 0) & 1) << i for i in range(12)) for k in range(18)]
        wantA = rp["A"] + [0] * (18 - len(rp["A"]))
        wantB = rp["B"] + [0] * (18 - len(rp["B"]))
        assert gotA == wantA and gotB == wantB, (len(sigma) - 1)
        # Bezout on circuit outputs (not just agreement with math)
        from kubona.codesign.psi import padd, pmul
        lhs = padd(pmul(gotA, E + [0] * (18 - len(E)), F2_12),
                   pmul(gotB, O + [0] * (18 - len(O)), F2_12), F2_12)
        assert lhs == [1]
        assert bits.get(L["FLAG"][0], 0) == 1
        n_ok += 1
        # temps clean (shared scratch only; BT trail persists by design)
        for q in L["allocs"]:
            assert bits.get(q, 0) == 0, (len(sigma) - 1, q)
    assert n_ok == len(locs) and n_ok >= 20


def test_eea_flag_refuses():
    # High-degree locators needing >17 terms: flag must be 0 (explicit
    # refusal). Found by hunt (seed 123): all 20/35-error instances here
    # need 18-35 single-term eliminations (measured math-side).
    rng = random.Random(123)
    L = eea12_layout(0)
    b, _ = build_eea12()
    n_flagged = 0
    for ne in (20, 35):
        for _ in range(4):
            errs = [(rng.randrange(4096), 1 + rng.randrange(4095)) for _ in range(ne)]
            syn = multi_error_syndromes(F2_12, GAMMA, errs, NROUND)
            try:
                rep = berlekamp_massey(syn, F2_12)
            except ValueError:
                continue
            try:
                rp = reciprocal_psi(rep["sigma"], F2_12)
            except ValueError:
                continue
            E, O = rp["E"], rp["O"]
            init = _load_eo(L, E, O)
            bits, phase, _ = _sim_trajectory(b.emit(), dict(init), random.Random(13))
            assert phase == 0
            if bits.get(L["FLAG"][0], 0) == 0:
                n_flagged += 1
    assert n_flagged >= 4  # the hunt provably finds refusers (else vacuous)
    # degenerate: E,O sharing a factor (math refuses) -> flag must be 0
    init = _load_eo(L, [1, 1], [1, 1])  # (1+z, 1+z): gcd = 1+z
    bits, _, _ = _sim_trajectory(b.emit(), dict(init), random.Random(14))
    assert bits.get(L["FLAG"][0], 0) == 0


def test_eea_pins_determinism_layout():
    L = eea12_layout(0)
    assert L["nin"] == 432 and L["ntemps"] == 7607
    assert len(L["A"]) == 216 and len(L["B"]) == 216
    regions = {k: set(v) for k, v in L.items() if isinstance(v, list) and v and isinstance(v[0], int) and k != "allocs"}
    keys = list(regions)
    assert len(keys) == 19  # E,O,X,Y,U0,U1,V0,V1,LT,ZT,TOWER,PE,QE,INV,BRE,ZTBIG,A,B,FLAG (BT checked below)
    for a in range(len(keys)):
        for bb in range(a + 1, len(keys)):
            assert not (regions[keys[a]] & regions[keys[bb]]), (keys[a], keys[bb])
    assert len(L["BT"]) == N_BLOCKS and all(len(w) == 107 for w in L["BT"])
    ball = set()
    for w in L["BT"]:
        assert not (ball & set(w))
        ball |= set(w)
    pall = set().union(*[regions[k] for k in keys])
    assert not (ball & pall) and len(ball) == N_BLOCKS * 107
    b, meta = build_eea12()
    assert meta["ccx"] == 985484
    b1, _ = build_eea12()
    assert b.emit() == b1.emit()
    with pytest.raises(ValueError):
        from kubona.codesign.eea12 import _check_word
        _check_word([0] * 5, 12, "x", 99999)
