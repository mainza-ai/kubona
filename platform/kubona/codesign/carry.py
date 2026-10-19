"""Truncated-carry / truncated-compare bounds (paper §II.1 sparse-prime §).

The paper adds a short constant (~2^32+977) to a 256-bit register but
propagates the carry only through the low 58 bits (60 in the fused square),
dropping escapes (~2^-26 per reduction: the carry survives only through a
run of ones); overflow compares inspect only the top 27 bits (wrong only
when all agree, ~2^-27). Both are the same idea: bound the RARE LONG-RUN
event analytically, pay full cost only inside a short window.

Toy-exact version here: n-bit register, m-bit constant C, carry propagated
k bits (m <= k <= n). Escape <=> carry reaches bit k, with EXACT
probability C/2^k over uniform inputs (proved by enumeration, not sampled).
Comparison of top-k bits errs exactly 2^-k on uniform independent pairs.

Plus the sparse-prime fold and fused square as exact integer identities
(paper SI II.1, fused coordinate arithmetic): 2^256 = 2^32+977 mod p lets
reduction stream through products without a general Barrett circuit, and
the 256-bit square splits into three 128-bit-half products accumulated one
at a time (no register ever holds the completed square, no copy-uncompute
second multiplication). Equalities, not estimates.
"""

from __future__ import annotations

from fractions import Fraction

from .divsteps import PAPER_P256

#: Fold constant: 2^256 mod p (derived from the shared prime — never a copy).
FOLD_C256 = (1 << 256) - PAPER_P256  # == 2^32 + 977 for the paper prime


def truncated_add_escape(x: int, c: int, n: int, k: int) -> bool:
    """True iff adding constant c to n-bit x carries into bit k (escape).

    Only the low k bits matter: the escape is decided before bit k, so a
    circuit propagating the carry k bits wide computes every sub-k bit
    exactly and fails exactly on escape.
    """
    _check(x, c, n, k)
    return _carry_into(x, c, k)


def _carry_into(xn: int, c: int, k: int) -> bool:
    lo_mask = (1 << k) - 1
    return ((xn & lo_mask) + c) >> k != 0


def escape_probability(n: int, c: int, k: int) -> Fraction:
    """Measured P(escape) over ALL 2^n inputs (exact rational — proof)."""
    _check(0, c, n, k)
    hits = sum(1 for x in range(1 << n) if _carry_into(x, c, k))
    return Fraction(hits, 1 << n)


def escape_bound(c: int, k: int) -> Fraction:
    """Analytic bound: C/2^k (uniform low-k bits; needs C <= 2^k)."""
    if not isinstance(c, int) or c < 0:
        raise ValueError(f"c must be non-negative int, got {c!r}: refusing")
    if not isinstance(k, int) or k < 0:
        raise ValueError(f"k must be non-negative int, got {k!r}: refusing")
    if c > (1 << k):
        raise ValueError(f"bound needs C <= 2^k (got C={c}, k={k}): refusing")
    return Fraction(c, 1 << k)


def compare_top_wrong(a: int, b: int, n: int, k: int) -> bool:
    """True iff the top k bits of n-bit a,b AGREE (the suspect case).

    The truncated comparator answers from the top k bits alone; full
    agreement is the only case it can misjudge, so agreement is the
    counted failure event (probability exactly 2^-k on uniform pairs)."""
    for v, name in ((a, "a"), (b, "b"), (n, "n"), (k, "k")):
        if isinstance(v, bool) or not isinstance(v, int) or v < 0:
            raise ValueError(f"{name} must be non-negative int: refusing")
    if k > n:
        raise ValueError(f"k={k} exceeds width n={n}: refusing")
    shift = n - k
    return (a >> shift) == (b >> shift)


def _check(x: int, c: int, n: int, k: int) -> None:
    for v, name in ((x, "x"), (c, "c"), (n, "n"), (k, "k")):
        if isinstance(v, bool) or not isinstance(v, int) or v < 0:
            raise ValueError(f"{name} must be non-negative int: refusing")
    if not (0 <= x < (1 << n)):
        raise ValueError(f"x must fit in {n} bits: refusing")
    if not (0 <= c < (1 << n)):
        raise ValueError(f"c must fit in {n} bits: refusing")
    if not (0 <= k <= n):
        raise ValueError(f"k must be in [0,{n}]: refusing")


def fold_reduce(x: int, p: int = PAPER_P256) -> int:
    """Reduce by the sparse-prime fold: write x = lo + hi*2^256, return
    (lo + hi*FOLD_C) mod p. Exact (== x mod p); the correction is short
    (FOLD_C = 2^32+977), so each fold is a small addition, not a division.
    """
    if isinstance(x, bool) or not isinstance(x, int) or x < 0:
        raise ValueError(f"x must be non-negative int, got {x!r}: refusing")
    if not isinstance(p, int) or p != PAPER_P256:
        raise ValueError("fold is prime-specific (shared const only): refusing")
    return ((x & ((1 << 256) - 1)) + (x >> 256) * FOLD_C256) % p


def fused_square_parts(lam: int) -> tuple[int, int, int]:
    """Split lam = lam0 + 2^128 lam1 (128-bit halves, fail-closed bounds)."""
    if isinstance(lam, bool) or not isinstance(lam, int) or not (0 <= lam < (1 << 256)):
        raise ValueError(f"lam must fit in 256 bits, got {lam!r}: refusing")
    return lam & ((1 << 128) - 1), lam >> 128, 128


def fused_square(lam: int, p: int = PAPER_P256) -> dict:
    """lam^2 mod p from three half-products, streamed with the fold.

    lam^2 = lam0^2 + 2^129 lam0 lam1 + 2^256 lam1^2 (mod p): the middle term
    doubles (2*2^128 = 2^129) and the top term folds. No completed square is
    ever formed (three partials, each accumulated under the fold); equality
    with pow(lam, 2, p) is asserted live. Partial widths are reported, never
    minimized-away: the top partial is shift-wide, the saving is structural
    (no square register, no copy-uncompute second multiplication).
    """
    if not isinstance(p, int) or p != PAPER_P256:
        raise ValueError("fused square is prime-specific (shared const only): refusing")
    lam0, lam1, _ = fused_square_parts(lam)
    t0 = lam0 * lam0
    t1 = (lam0 * lam1) << 129
    t2 = (lam1 * lam1) << 256
    acc = (fold_reduce(t0, p) + fold_reduce(t1, p) + fold_reduce(t2, p)) % p
    if acc != pow(lam, 2, p):
        raise ValueError("fused-square identity failed (implementation bug): refusing")
    return {"result": acc, "partials": (t0, t1, t2),
            "max_partial_bits": max(t.bit_length() for t in (t0, t1, t2))}
