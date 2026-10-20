"""Slice 58: paper-scale BM walk (Phase 2 B1b) + bank-reuse impossibility.

Production bar: the full 70-round walk (F2^12, fresh 1316-bit banks per
round, one shared 2343-temp block, single ZERO qubit) is proven over full
planted walks end to end — final bank equals the model's final state
exactly (sigma/tau/delta/idelta/d), inputs preserved, every temp clean.
One build, one sim per walk. Measured 5,764,080T = 70 x 82,344T
(structural); 96,620 qubits, of which 93,436 are append-only banks.

WHY FRESH BANKS (recorded so nobody "optimizes" this into a bug):
ping-pong overwriting was built first and produced a 1-bit sigma error in
round 2 — the round emitter preloads its outputs as the retain/zero case
and flag-gates deltas, correct only on |0> output wires; a stale bank
(fresh-bank output of round r-2) corrupts the preload. Overwriting a stale
bank unitarily is impossible in general (erasure is non-unitary;
uncomputing the stale content needs its inputs, cascading to the walk
start — finite pools fail by pigeonhole). Measurement-clear
(MBU-for-banks, paper-precedented for temps) is a queued model change, not
this slice. Banks are append-only; everything recyclable (temps) IS
recycled via the per-round mirror.
"""

import random

import pytest

from kubona.codesign import F2_12
from kubona.codesign.bm import (
    _bank12,
    _bm_walk,
    berlekamp_massey,
    build_bm12_walk,
    multi_error_syndromes,
)
from kubona.codesign.kickmix import _sim_trajectory

GAMMA = 2
NROUND = 70
NB = 1316
BANK0 = 840


def _walks():
    rng = random.Random(58)
    out = []
    for pos in (0, 2048, 4094):
        out.append(multi_error_syndromes(F2_12, GAMMA, [(pos, 1 + rng.randrange(4095))], NROUND))
    for _ in range(3):
        out.append(multi_error_syndromes(
            F2_12, GAMMA, [(rng.randrange(4096), 1 + rng.randrange(4095)) for _ in range(2)], NROUND))
    for _ in range(2):
        out.append(multi_error_syndromes(
            F2_12, GAMMA, [(rng.randrange(4096), 1 + rng.randrange(4095)) for _ in range(3)], NROUND))
    rng99 = random.Random(99)
    n_found = 0
    for _ in range(300):
        ne = 1 + rng99.randrange(40)
        errs = [(rng99.randrange(4096), 1 + rng99.randrange(4095)) for _ in range(ne)]
        syn = multi_error_syndromes(F2_12, GAMMA, errs, NROUND)
        h = []
        try:
            _bm_walk(syn, F2_12, 80, 35, harvest=h)
        except ValueError:
            continue
        if any(e.get("case") == "retain" for e in h if "post" in e):
            out.append(syn)
            n_found += 1
            if n_found >= 2:
                break
    assert n_found == 2
    return out


def test_walk_build_pins():
    b, meta = build_bm12_walk()
    assert meta["rounds"] == 70 and meta["kind"] == "bm12-walk"
    assert meta["ccx"] == 70 * 34602 == 2422140
    nqubits = 840 + 71 * NB + 2343 + 1
    assert nqubits == 96620
    assert len(meta["inputs"]) == 840 and len(meta["outputs"]) == 1316
    assert meta["temps"] == 2343 - 10 + 1  # ZT managed by the zero-test
    b1, _ = build_bm12_walk()
    assert b.emit() == b1.emit()


def test_walk_harvested_end_to_end():
    walks = _walks()
    assert len(walks) == 10
    b, meta = build_bm12_walk()
    outs = meta["outputs"]
    temps0 = BANK0 + 71 * NB
    for syn in walks:
        rep = berlekamp_massey(syn, F2_12)
        init = {i: (syn[i // 12] >> (i % 12)) & 1 for i in range(840)}
        bits, phase, _ = _sim_trajectory(b.emit(), dict(init), random.Random(11))
        assert phase == 0
        final = {k: sum((bits.get(outs[k * 12 + i], 0) & 1) << i for i in range(12))
                 for k in range(1316 // 12)}
        assert list(final[k] for k in range(36)) == rep["sigma"] + [0] * (36 - len(rep["sigma"]))
        # tau/delta/idelta/d live after sigma in the final bank
        got_tau = [final[k] for k in range(36, 36 + 71)]
        # model tau: last round's post tau (harvest the walk to read it)
        h = []
        _bm_walk(syn, F2_12, 80, 35, harvest=h)
        last = h[-1]["post"]
        assert got_tau == list(last[1]) + [0] * (71 - len(last[1]))
        got_delta = sum((bits.get(outs[(36 + 71) * 12 + i], 0) & 1) << i for i in range(12))
        assert got_delta == last[2]
        got_idelta = sum((bits.get(outs[(36 + 72) * 12 + i], 0) & 1) << i for i in range(12))
        assert got_idelta == F2_12.inv(last[2])
        got_d = sum((bits.get(outs[(36 + 73) * 12 + i], 0) & 1) << i for i in range(8))
        assert (got_d - 256 if got_d >= 128 else got_d) == last[3]
        # inputs preserved, temps clean
        for i in range(840):
            assert bits.get(i, 0) == init[i]
        for q in range(temps0, temps0 + 2343):
            assert bits.get(q, 0) == 0, q
