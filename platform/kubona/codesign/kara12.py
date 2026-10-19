"""One-level Karatsuba F2^12 multiplier (12 -> 6-bit schoolbook leaves).

Split a = a_lo + a_hi*x^6, c = c_lo + c_hi*x^6; three raw 6-bit products
(p0 = a_lo*c_lo, p1 = (a_lo+a_hi)(c_lo+c_hi), p2 = a_hi*c_hi, 36 ANDs each,
no internal reduction); middle m = p1+p0+p2; full 23-bit product
P = p0 + m*x^6 + p2*x^12; reduce bits 12..22 via REDUCE_12 (CNOTs only).
Self-contained: internal temps (sums, middle, product) are mirrored clean
(mults cleared by re-accumulation, CNOT ladders reversed); the answer in
`out` persists.

Cost: 3 x 72T forward + 3 x 72T mirror = 432T pinned (vs 363T schoolbook
accumulate-only, 726T schoolbook accumulate+clear: 1.68x all-in at the cost
of 46 temps). Recursive Karatsuba (6 -> 3-bit leaves, ~324T) is a queued
follow-up, not this slice. The Fermat inverter keeps schoolbook mults
(deliberate: it is the pinned baseline family member; the tower is the
production inversion path; this emitter serves the EEA/BM builds of
Phase 2/3, which need raw mults, not another inverter).
"""

from __future__ import annotations

from .f12 import REDUCE_12
from .kickmix import Builder

#: Temp layout for emit_kara_mul12 (47 temps: 46 data + shared pool).
_T0 = (0, 6)     # a_lo + a_hi
_T1 = (6, 12)    # c_lo + c_hi
_M = (12, 23)    # middle product p1 (+p0+p2 folded in)
_P = (23, 46)    # full 23-bit product
_POOL = (46, 47)
N_KARA_TEMPS = 47


def _check_word(w: list[int], n: int, name: str, nqubits: int) -> None:
    if not isinstance(w, list) or len(w) != n or any(not isinstance(q, int) or q < 0 or q >= nqubits for q in w):
        raise ValueError(f"{name} must be {n} in-range qubit ids: refusing {w!r}")


def emit_f6_mul(b: Builder, a: list[int], c: list[int], out: list[int],
                pool: list[int], seg: list | None = None) -> int:
    """Raw 6-bit schoolbook multiply-accumulate (no reduction, 11-bit product).

    One AND per (i, j) pair (36 total) with immediate uncompute (2T each);
    each AND fanned to the single product bit i+j (CNOT). a, c preserved;
    out accumulates (caller clears by re-accumulation). Returns 72.
    Recorded to seg for mirror replay when seg is given.
    """
    if not isinstance(b, Builder):
        raise ValueError("b must be a Builder: refusing")
    for name, w in (("a", a), ("c", c)):
        _check_word(w, 6, name, b.num_qubits)
    _check_word(out, 11, "out", b.num_qubits)
    _check_word(pool, 1, "pool", b.num_qubits)
    (p,) = pool
    for i in range(6):
        for j in range(6):
            b.ccx(a[i], c[j], p)
            b.cx(p, out[i + j])
            b.ccx(a[i], c[j], p)
    if seg is not None:
        seg.append(("f6", list(a), list(c), list(out), list(pool)))
    return 72


def _mirror(b: Builder, seg: list, pool: list[int]) -> int:
    """Replay recorded ops in reverse (mults re-accumulate clear, CNOTs reverse)."""
    n = 0
    for op in reversed(seg):
        if op[0] == "cx":
            _, a, c = op
            b.cx(a, c)
        elif op[0] == "f6":
            _, a, c, out, _ = op
            n += emit_f6_mul(b, a, c, out, pool)
        else:
            raise ValueError(f"unreplayable op {op[0]}: refusing")
    return n


def emit_kara_mul12(b: Builder, a: list[int], c: list[int], out: list[int],
                    temps: list[int]) -> int:
    """Karatsuba F2^12 multiply (a * c added into out, a/c preserved, 0-safe).

    Forward: sums -> three 6-bit products -> middle fold -> product place ->
    reduce into out. Mirror: mults cleared by re-accumulation in reverse,
    CNOT ladders reversed (each replay reads only still-intact values: the
    middle un-folds before the products clear; the products clear before
    the sums reverse). out must be zeroed; a, c, out, temps pairwise
    disjoint. Returns the Toffoli count (432 pinned).
    """
    if not isinstance(b, Builder):
        raise ValueError("b must be a Builder: refusing")
    for name, w in (("a", a), ("c", c), ("out", out)):
        _check_word(w, 12, name, b.num_qubits)
    _check_word(temps, N_KARA_TEMPS, "temps", b.num_qubits)
    if len(set(list(a) + list(c) + list(out) + list(temps))) != 12 + 12 + 12 + N_KARA_TEMPS:
        raise ValueError("a, c, out, temps must be disjoint: refusing")

    def words(rng: tuple[int, int]) -> list[int]:
        return [temps[i] for i in range(*rng)]

    t0, t1 = words(_T0), words(_T1)
    m, p = words(_M), words(_P)
    pool = words(_POOL)

    seg: list = []
    n = 0
    for i in range(6):  # sums (recorded for mirror)
        b.cx(a[i], t0[i])
        seg.append(("cx", a[i], t0[i]))
        b.cx(a[6 + i], t0[i])
        seg.append(("cx", a[6 + i], t0[i]))
        b.cx(c[i], t1[i])
        seg.append(("cx", c[i], t1[i]))
        b.cx(c[6 + i], t1[i])
        seg.append(("cx", c[6 + i], t1[i]))
    n += emit_f6_mul(b, a[0:6], c[0:6], p[0:11], pool, seg)  # p0
    n += emit_f6_mul(b, t0, t1, m, pool, seg)  # p1
    n += emit_f6_mul(b, a[6:12], c[6:12], p[12:23], pool, seg)  # p2
    for i in range(11):  # m ^= p0
        b.cx(p[i], m[i])
        seg.append(("cx", p[i], m[i]))
    for i in range(11):  # m ^= p2
        b.cx(p[12 + i], m[i])
        seg.append(("cx", p[12 + i], m[i]))
    for i in range(11):  # P[6:17] ^= m
        b.cx(m[i], p[6 + i])
        seg.append(("cx", m[i], p[6 + i]))
    for i in range(12):  # answer persists: never reversed
        b.cx(p[i], out[i])
    for k in range(12, 23):
        for t in REDUCE_12[k]:
            b.cx(p[k], out[t])
    n += _mirror(b, seg, pool)
    return n
