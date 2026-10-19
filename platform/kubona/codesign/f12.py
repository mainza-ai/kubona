"""F2^12 field arithmetic as kickmix programs (schoolbook baseline).

Modulus 0x1053 (x^12 + x^6 + x^4 + x + 1; verified bit-by-bit in-test).
Schoolbook multiply with fused reduction (one CCX per (i,j,t) triple),
CNOT-only squaring derived from the same table, Fermat inversion
x^4094 by repeated squaring + binary mult chain. This mirrors f24.py's
"starting point" role at paper scale: the paper's hand-optimized
variants (Karatsuba/tower/MBU families) are queued as follow-ups and
the measured schoolbook cost is the honest baseline, never presented
as competitive.

REDUCE_12[k] (verified in-test against gf arithmetic on all 144 basis
products): product bit k of a*b lands on these out positions.
"""

from __future__ import annotations

from .kickmix import Builder

REDUCE_12 = {
    0: (0,), 1: (1,), 2: (2,), 3: (3,), 4: (4,), 5: (5,),
    6: (6,), 7: (7,), 8: (8,), 9: (9,), 10: (10,), 11: (11,),
    12: (0, 1, 4, 6), 13: (1, 2, 5, 7), 14: (2, 3, 6, 8),
    15: (3, 4, 7, 9), 16: (4, 5, 8, 10), 17: (5, 6, 9, 11),
    18: (0, 1, 4, 7, 10), 19: (1, 2, 5, 8, 11),
    20: (0, 1, 2, 3, 4, 9), 21: (1, 2, 3, 4, 5, 10),
    22: (2, 3, 4, 5, 6, 11),
}


def _check_word(w: list[int], n: int, name: str, nqubits: int) -> None:
    if not isinstance(w, list) or len(w) != n or any(not isinstance(q, int) or q < 0 or q >= nqubits for q in w):
        raise ValueError(f"{name} must be {n} in-range qubit ids: refusing {w!r}")


def emit_f12_mul(b: Builder, a: list[int], c: list[int], out: list[int]) -> int:
    """Schoolbook F2^12 multiply-accumulate with fused reduction.

    One CCX per (i, j, t) with t in REDUCE_12[i+j] (363 total);
    out must be zeroed (accumulation) and disjoint use is the caller's
    contract. Returns the Toffoli count.
    """
    for name, w in (("a", a), ("c", c), ("out", out)):
        _check_word(w, 12, name, b.num_qubits)
    n = 0
    for i in range(12):
        for j in range(12):
            for t in REDUCE_12[i + j]:
                b.ccx(a[i], c[j], out[t])
                n += 1
    return n


def emit_f12_sqr(b: Builder, a: list[int], out: list[int]) -> None:
    """CNOT-only squaring from the reduction table (Frobenius is linear)."""
    for name, w in (("a", a), ("out", out)):
        _check_word(w, 12, name, b.num_qubits)
    for i in range(12):
        for t in REDUCE_12[2 * i]:
            b.cx(a[i], out[t])


def emit_fermat_inv12(b: Builder, x: list[int], h: list[int], temps: list[int]) -> int:
    """Fermat inverse x^4094 as binary square-and-multiply chain.

    Squares x^2..x^2048 (CNOTs, free), then 10 accumulate-multiplies down
    the chain into h (10 x 363T), then mirror-uncompute of the 9
    intermediate products (9 x 363T) and the squares (free). h must be
    zeroed (accumulation); x preserved; 0 maps to 0 with no special
    casing (zero controls are no-ops). Re-invocation uncomputes (same
    commuting-involution argument as the F2^4 Fermat emitter).
    Needs 20 temp words (11 squares + 9 chain links); all return clean.
    """
    for name, w in (("x", x), ("h", h)):
        _check_word(w, 12, name, b.num_qubits)
    if not isinstance(temps, list) or len(temps) != 240 or any(not isinstance(q, int) or q < 0 or q >= b.num_qubits for q in temps):
        raise ValueError(f"temps must be 240 in-range qubit ids: refusing {temps!r}")
    if len(set(list(x) + list(h) + list(temps))) != 264:
        raise ValueError("x, h, temps must be disjoint: refusing")
    sq = [temps[12 * i:12 * (i + 1)] for i in range(11)]
    us = [temps[132 + 12 * i:132 + 12 * (i + 1)] for i in range(9)]
    n = 0
    # squares x^2 .. x^2048 (free CNOTs)
    prev = x
    for i in range(11):
        emit_f12_sqr(b, prev, sq[i])
        prev = sq[i]
    # chain down: u0 = s11*s10, u1 = u0*s9, ..., u8 = u7*s2, h ^= u8*s1
    # (exponents sum to 2+4+...+2048 = 4094 = -1 mod 4095).
    for k in range(9):
        a = sq[10] if k == 0 else us[k - 1]
        n += emit_f12_mul(b, a, sq[9 - k], us[k])
    n += emit_f12_mul(b, us[8], sq[0], h)
    # mirror uncompute (commuting involutions, reverse order)
    for k in range(8, -1, -1):
        a = sq[10] if k == 0 else us[k - 1]
        n += emit_f12_mul(b, a, sq[9 - k], us[k])
    for i in range(10, -1, -1):
        emit_f12_sqr(b, sq[i - 1] if i > 0 else x, sq[i])
    return n  # 19 schoolbook mults + free squarings
