"""Slice 55: F2^12 tower-field inverter (OUR construction, first paper-scale win).

Production bar: this tower is ours, not the paper's — the SI's only tower
division is Hermitian F64-with-F8 arithmetic (main.tex:2146); the
Reed-Solomon F2^12 thread gives no tower, so the F2^12/F64/F8 tower here
(Z=937, Y=MU1=70, W=1970, machine-found with round-trip gated on all 4096)
is labeled as our construction throughout.

Proof discipline: the F8 mult/sqr tables are derived from z^3+z+1 at import
and proved against int arithmetic (import gate); the norm chain
(N = A(A+B)+MU1*B^2, M = N*Nbar const, M^-1 = M^6, scale-back) is proved
against F2_12.inv on all 4095 nonzero inputs classically; every emitter is
then proved functionally exhaustive (F8: 64 pairs, F64: 4096 pairs, tower:
4096 inputs with temps clean and phase clean). Measured: 684T per inversion
(38 F8-mult equivalents x 18T) vs 6897T Fermat — a 10.1x win, pinned not
transcribed. Drop-in evidence: tower output == Fermat output follows by
transitivity (both proven exhaustive against the same F2_12.inv oracle;
slice 54 for Fermat), plus a direct boundary cross-check below. Two live
bugs caught during the build are recorded so nobody re-introduces them: a
transposed basis matrix in the out-accumulate (passed B=0 inputs, failed
x=2), and a shared F64-mult scratch reused across sequential mults
(accumulate semantics poisoned it; fixed by emitting a1*b1 into each half
directly — no shared mutable state).
"""

import random

import pytest

from kubona.codesign import F2_12
from kubona.codesign.f12 import emit_fermat_inv12
from kubona.codesign.kickmix import Builder, _sim_trajectory
from kubona.codesign.tower12 import (
    F8_MUL_TERMS,
    N_TOWER_TEMPS,
    Z_ROOT,
    emit_f8_mul,
    emit_f8_sqr,
    emit_f64_mul,
    emit_f64_sqr,
    emit_mu1_scale,
    emit_tower_inv12,
    tower12_change_matrices,
    tower_inv12_classical,
)

Z2 = F2_12.mul(Z_ROOT, Z_ROOT)
Y = 70


def _f8_elts():
    out = {0}
    p = 1
    for _ in range(7):
        p = F2_12.mul(p, Z_ROOT)
        out.add(p)
    return sorted(out)


def _coords8(v):
    for c in range(8):
        b = [(c >> i) & 1 for i in range(3)]
        if (b[0] ^ F2_12.mul(b[1], Z_ROOT) ^ F2_12.mul(b[2], Z2)) == v:
            return b
    raise AssertionError(v)


def _coords64(v):
    f8 = _f8_elts()
    for a in f8:
        for b in f8:
            if F2_12.add(a, F2_12.mul(b, Y)) == v:
                return _coords8(a) + _coords8(b)
    raise AssertionError(v)


def _from64(bits):
    a = bits[0] ^ F2_12.mul(bits[1], Z_ROOT) ^ F2_12.mul(bits[2], Z2)
    c = bits[3] ^ F2_12.mul(bits[4], Z_ROOT) ^ F2_12.mul(bits[5], Z2)
    return F2_12.add(a, F2_12.mul(c, Y))


def test_change_matrices_product_and_roundtrip():
    fwd, back = tower12_change_matrices()
    for a, bb in ((fwd, back), (back, fwd)):
        for i in range(12):
            for j in range(12):
                assert (sum(a[i][k] & bb[k][j] for k in range(12)) & 1) == (1 if i == j else 0)
    from kubona.codesign.tower12 import _tower_basis

    basis = _tower_basis()
    for x in range(4096):
        bits = [(x >> i) & 1 for i in range(12)]
        t = [sum(row[j] & bits[j] for j in range(12)) & 1 for row in fwd]
        recon = 0
        for k in range(12):
            if t[k]:
                recon ^= basis[k]
        assert recon == x, x


def test_tower_math_matches_fermat_exhaustive():
    assert tower_inv12_classical(0) == 0
    for x in range(1, 4096):
        assert tower_inv12_classical(x) == F2_12.inv(x), x
    with pytest.raises(ValueError):
        tower_inv12_classical(4096)


def test_f8_mul_pattern_exact_and_exhaustive():
    b = Builder(10)
    n = emit_f8_mul(b, [0, 1, 2], [3, 4, 5], [6, 7, 8], [9])
    assert n == 18
    got = sorted((o.qubits[0], o.qubits[1], o.qubits[2]) for o in b.emit() if o.name == "ccx")
    want = []
    for i in range(3):
        for j in range(3):
            want += [(i, 3 + j, 9)] * 2  # compute + uncompute, fanout is CNOTs
    assert sorted(want) == got
    for a in _f8_elts():
        for c in _f8_elts():
            b = Builder(10)
            b.alloc(9)
            emit_f8_mul(b, [0, 1, 2], [3, 4, 5], [6, 7, 8], [9])
            init = {q: 0 for q in range(10)}
            for i, bit in enumerate(_coords8(a)):
                init[i] = bit
            for i, bit in enumerate(_coords8(c)):
                init[3 + i] = bit
            bits, phase, _ = _sim_trajectory(b.emit(), dict(init), random.Random(3))
            assert phase == 0
            gv = 0
            for i in range(3):
                if bits.get(6 + i, 0) & 1:
                    gv ^= [1, Z_ROOT, Z2][i]
            assert gv == F2_12.mul(a, c), (a, c)
            assert bits.get(9, 0) == 0
            b.free(9)
    with pytest.raises(ValueError):
        emit_f8_mul(Builder(10), [0, 1], [3, 4, 5], [6, 7, 8], [9])


def test_f64_helpers_exhaustive():
    f8 = _f8_elts()
    f64 = sorted({F2_12.add(a, F2_12.mul(b, Y)) for a in f8 for b in f8})
    assert len(f64) == 64
    # sqr + mu1-scale: 64/64 each
    for v in f64:
        for emit, want in ((emit_f64_sqr, F2_12.sqr(v)), (emit_mu1_scale, F2_12.mul(Y, v))):
            b = Builder(12)
            for q in range(6, 12):
                b.alloc(q)
            emit(b, list(range(6)), list(range(6, 12)))
            init = {q: 0 for q in range(12)}
            for i, bit in enumerate(_coords64(v)):
                init[i] = bit
            bits, phase, _ = _sim_trajectory(b.emit(), dict(init), random.Random(4))
            assert phase == 0
            assert _from64([bits.get(6 + i, 0) & 1 for i in range(6)]) == want, v
    # mult: 4096/4096 pairs + cost pin
    b = Builder(22)
    n = emit_f64_mul(b, list(range(6)), list(range(6, 12)), list(range(12, 18)), [21])
    assert n == 90
    for a in f64:
        for c in f64:
            b = Builder(22)
            for q in range(12, 22):
                b.alloc(q)
            emit_f64_mul(b, list(range(6)), list(range(6, 12)), list(range(12, 18)), [21])
            init = {q: 0 for q in range(22)}
            for i, bit in enumerate(_coords64(a)):
                init[i] = bit
            for i, bit in enumerate(_coords64(c)):
                init[6 + i] = bit
            bits, phase, _ = _sim_trajectory(b.emit(), dict(init), random.Random(5))
            assert phase == 0
            assert _from64([bits.get(12 + i, 0) & 1 for i in range(6)]) == F2_12.mul(a, c), (a, c)
            assert bits.get(21, 0) == 0, (a, c)
    with pytest.raises(ValueError):
        emit_f64_mul(Builder(22), list(range(5)), list(range(6, 12)), list(range(12, 18)), [18, 19, 20], [21])


def test_tower_inverter_exhaustive():
    nq = 12 + 12 + N_TOWER_TEMPS
    assert N_TOWER_TEMPS == 79
    n_tot = 0
    for x in range(4096):
        b = Builder(nq)
        for q in range(24, nq):
            b.alloc(q)
        n = emit_tower_inv12(b, list(range(12)), list(range(12, 24)), list(range(24, nq)))
        assert n == 684, x
        n_tot += n
        init = {i: (x >> i) & 1 for i in range(12)}
        for q in range(24, nq):
            init[q] = 0
        bits, phase, _ = _sim_trajectory(b.emit(), dict(init), random.Random(6))
        assert phase == 0, x
        got = sum((bits.get(12 + i, 0) & 1) << i for i in range(12))
        assert got == (F2_12.inv(x) if x else 0), x
        for q in range(24, nq):
            assert bits.get(q, 0) == 0, (x, q)
        for q in range(24, nq):
            b.free(q)
    assert n_tot == 4096 * 684
    # determinism: identical emission on rebuild
    b1, b2 = Builder(nq), Builder(nq)
    for b in (b1, b2):
        for q in range(24, nq):
            b.alloc(q)
        emit_tower_inv12(b, list(range(12)), list(range(12, 24)), list(range(24, nq)))
    assert b1.emit() == b2.emit()


def test_tower_inverter_refusals():
    nq = 12 + 12 + N_TOWER_TEMPS
    b = Builder(nq)
    with pytest.raises(ValueError):
        emit_tower_inv12(b, list(range(11)), list(range(12, 24)), list(range(24, nq)))
    with pytest.raises(ValueError):
        emit_tower_inv12(b, list(range(12)), list(range(12, 24)), list(range(24, nq - 1)))
    with pytest.raises(ValueError):
        emit_tower_inv12(b, list(range(12)), list(range(12, 24)), list(range(12, 12 + N_TOWER_TEMPS)))


def test_tower_agrees_fermat_on_boundaries():
    # Direct cross-emitter smoke on boundary values; exhaustive agreement
    # follows by transitivity (both emitters proven against F2_12.inv on
    # the full domain: this file for tower, slice 54 for Fermat).
    nq = 12 + 12 + N_TOWER_TEMPS
    for x in (0, 1, 937, 1198, 1799, 2048, 4095):
        bt, bf = Builder(nq), Builder(264)
        for q in range(24, nq):
            bt.alloc(q)
        for q in range(24, 264):
            bf.alloc(q)
        emit_tower_inv12(bt, list(range(12)), list(range(12, 24)), list(range(24, nq)))
        emit_fermat_inv12(bf, list(range(12)), list(range(12, 24)), list(range(24, 264)))
        init = {i: (x >> i) & 1 for i in range(12)}
        bitst, _, _ = _sim_trajectory(bt.emit(), dict(init), random.Random(7))
        inits = dict(init)
        for q in range(24, 264):
            inits[q] = 0
        bitsf, _, _ = _sim_trajectory(bf.emit(), inits, random.Random(7))
        got_t = sum((bitst.get(12 + i, 0) & 1) << i for i in range(12))
        got_f = sum((bitsf.get(12 + i, 0) & 1) << i for i in range(12))
        assert got_t == got_f == (F2_12.inv(x) if x else 0), x
