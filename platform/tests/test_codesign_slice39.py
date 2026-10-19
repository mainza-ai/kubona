"""Slice 39: exact multi-output Boolean minimization of the trace readout.

Production bar: prime implicants derived exactly (QM, machine-checked
against brute force on probes); the greedy XOR-cover is parity-aware
(an OR-accumulation draft cancelled on overlaps — caught by
verification); every emitted cover verified before emission and the
circuit proven exhaustive (512/512, same spec as naive). Measured:
6580T naive -> 1434T minimized (4.6x); the gap to the paper's
hand-optimized 20T stays queued (needs multi-level factorization, not
SOP). One-hot outputs provably admit no cross-flag cube sharing
(disjoint onsets), so the win is pure minimization.
"""

import random

import pytest

from kubona.codesign import (
    KickmixEvaluator,
    build_trace_readout_minimized,
    greedy_multioutput_cover,
    prime_implicants,
    trace_readout_spec,
    verify_cover,
)


def _bruteforce_primes(table, nvars):
    """Independent prime check: maximal subcubes of the onset."""
    from kubona.codesign.minimize import _covers
    onset = {m for m, v in table.items() if v == 1}
    primes = set()
    for mask in range(1 << nvars):
        if mask not in onset:
            continue
        # largest subcube containing mask and inside onset: none needed;
        # instead collect maximal (value, care) with mask covered, onset-closed
        best = []
        for care in range(1 << nvars):
            value = mask & care
            if all(mm in onset for mm in range(1 << nvars) if _covers((value, care), mm)):
                best.append((value, care))
        # maximal = not strictly contained in another onset-closed cube
        for c in best:
            if not any(c != d and _cube_contains(d, c, nvars) for d in best):
                primes.add(c)
    return primes


def _cube_contains(big, small, nvars):
    from kubona.codesign.minimize import _covers
    (vb, cb), (vs, cs) = big, small
    return all(not _covers(small, m) or _covers(big, m) for m in range(1 << nvars))


def test_qm_matches_bruteforce():
    rng = random.Random(0)
    for trial in range(30):
        nvars = 4
        table = {m: rng.randrange(2) for m in range(16)}
        assert prime_implicants(table, nvars) == _bruteforce_primes(table, nvars)
    # known answers: majority-3 is 3 cubes; xor-3 keeps 4 minterms
    maj = {m: 1 if bin(m).count("1") >= 2 else 0 for m in range(8)}
    assert len(prime_implicants(maj, 3)) == 3
    xor3 = {m: bin(m).count("1") % 2 for m in range(8)}
    assert len(prime_implicants(xor3, 3)) == 4


def test_greedy_known_answers_and_refusals():
    # shared subcube across outputs is reused, not duplicated
    t1 = {m: 1 if (m & 0b11) == 0b11 else 0 for m in range(16)}
    t2 = {m: 1 if (m & 0b11) == 0b11 or (m & 0b1100) == 0b1100 else 0 for m in range(16)}
    cov = greedy_multioutput_cover([t1, t2], 4)
    assert len(cov[0]) == 1 and len(cov[1]) == 3
    assert all(verify_cover(c, t, 4) for c, t in zip(cov, [t1, t2]))
    with pytest.raises(ValueError):
        greedy_multioutput_cover([], 4)
    with pytest.raises(ValueError):
        greedy_multioutput_cover([{0: 2}], 4)
    with pytest.raises(ValueError):
        prime_implicants({0: 1}, 0)
    with pytest.raises(ValueError):
        prime_implicants({0: 1}, 11)


def test_minimized_readout_proves_table():
    (b, regs) = build_trace_readout_minimized()
    assert regs["kind"] == "minimized QM+greedy"
    assert regs["ccx"] == 1434 and regs["temps"] == 8 and regs["terms"] == 100
    assert regs["ccx"] < 6580  # measured improvement over naive, pinned
    spec = trace_readout_spec()  # same spec as the naive circuit
    ev = KickmixEvaluator()
    for mask in range(512):
        init = {q: (mask >> q) & 1 for q in range(9)}
        res = ev.check_one(b, spec, init, random.Random(1))
        assert res.valid, (mask, res.reason)
