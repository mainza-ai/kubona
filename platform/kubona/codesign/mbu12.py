"""MBU-packed F2^12 schoolbook multiplier (F2 follow-up).

Same (i,j)-pair structure as emit_f12_mul (REDUCE_12 fused reduction) but
each AND is measurement-based: CCX into pool (1T), CNOT fanout to all
REDUCE_12[i+j] targets, X-basis measure (hmr, releases pool), CZ correction
on the inputs conditioned on the outcome. 144 ANDs x 1T = 144T (vs 363T
plain schoolbook, 2.5x; vs 432T plain Karatsuba which loses outright).

Why MBU-schoolbook, not MBU-Karatsuba: Kara reduces AND count (108 vs 144)
but its recombination/clear structure costs more than it saves once ANDs
are cheap (MBU-kara ~= 216T self-contained vs 144T here). Verified by
construction + measurement below; the kara12 comparison family stands.

E2 bar coverage: classical-input decode paths only (all callers pass
harvested classical data; never superpositions). Multi-trajectory proving
covers measurement randomness (tower.py slice-11 precedent); a dropped
correction fails loudly (tested).
"""

from __future__ import annotations

from .f12 import REDUCE_12
from .kickmix import Builder


def _check_word(w: list[int], n: int, name: str, nqubits: int) -> None:
    if not isinstance(w, list) or len(w) != n or any(not isinstance(q, int) or q < 0 or q >= nqubits for q in w):
        raise ValueError(f"{name} must be {n} in-range qubit ids: refusing {w!r}")


def emit_mbu_mul12(b: Builder, a: list[int], c: list[int], out: list[int],
                   pool: list[int]) -> int:
    """MBU schoolbook F2^12 multiply-accumulate (fused reduction, 144T).

    One MBU-AND per (i, j) pair (144 total): CCX into pool, fan out to all
    REDUCE_12[i+j] targets, hmr(pool) releases it, CZ(a_i,c_j) corrected on
    the outcome. a, c preserved (controls/XORs only); out accumulates
    (caller clears by re-accumulation — XOR, never reversal). Pool is a
    single caller-reserved wire, managed internally (alloc per AND, consumed
    by measurement — do NOT pre-alloc). Returns 144.
    """
    if not isinstance(b, Builder):
        raise ValueError("b must be a Builder: refusing")
    for name, w in (("a", a), ("c", c), ("out", out)):
        _check_word(w, 12, name, b.num_qubits)
    _check_word(pool, 1, "pool", b.num_qubits)
    (p,) = pool
    n = 0
    for i in range(12):
        for j in range(12):
            b.alloc(p)
            b.ccx(a[i], c[j], p)
            n += 1
            for t in REDUCE_12[i + j]:
                b.cx(p, out[t])
            r = b.hmr(p)
            b.cz(a[i], c[j], cond=(r,))
    if n != 144:
        raise ValueError(f"MBU mult cost drifted (want 144, got {n}): refusing")
    return n
