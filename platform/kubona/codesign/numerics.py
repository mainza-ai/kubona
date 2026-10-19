"""Numerics-pack kernel (HHL/Coulomb/Gaussian pattern, toy-scale real logic).

Implements the paper's shared mechanics as verifiable integer math:
- dyadic leaves [L, L+2^b), L multiple of 2^b, exact cover of [0,2^n)
- shared coordinate d = 2^{c-b} * delta + cL (cyclic-shift model)
- truncated products (dropped low partial-product columns with error bound)
- offline integer-quadratic fit (brute force over declared widths)
- accuracy sweep (worst-case error vs exact fn)

Quantum execution is NOT claimed here: the kernel is the classical
numerical model of the circuit's integer computation (what the offline
fit guarantees). Aer evidence covers the underlying arithmetic primitive
(see test: 2-bit adder sweep). Pack promotion needs both ledger refs.
"""

from __future__ import annotations

import itertools


def dyadic_leaves(n_bits: int, widths: list[int]) -> list[tuple[int, int]]:
    """Tile [0,2^n) left-to-right with dyadic leaves of given widths (exponents).

    widths lists b per consecutive leaf; each leaf [L,L+2^b) requires
    L % 2^b == 0. Exact cover or refuse (no gaps/overlaps/overrun).
    """
    if not isinstance(n_bits, int) or n_bits < 1 or n_bits > 16:
        raise ValueError(f"n_bits must be in [1,16], got {n_bits!r}: refusing")
    if not widths or any(not isinstance(b, int) or b < 0 or b > n_bits for b in widths):
        raise ValueError(f"widths must be exponents in [0,n_bits], got {widths!r}: refusing")
    domain = 1 << n_bits
    leaves: list[tuple[int, int]] = []
    L = 0
    for b in widths:
        w = 1 << b
        if L % w != 0:
            raise ValueError(f"leaf at L={L} not aligned to 2^{b}: refusing")
        if L + w > domain:
            raise ValueError(f"leaves overrun domain at L={L} w={w}: refusing")
        leaves.append((L, b))
        L += w
    if L != domain:
        raise ValueError(f"leaves cover [0,{L}), need exact [0,{domain}): refusing")
    return leaves


def shared_coordinate(x: int, L: int, b: int, coord_bits: int) -> tuple[int, int, int]:
    """Shared local coordinate: d = 2^{c-b} * delta + cL, delta = x - L.

    cL (leaf-constant wrapped bits) = L >> b (leaf index), absorbed into
    coefficients offline. Returns (d, delta, cL). Paper example: width-2^12
    leaf in a 15-bit stage: d = 2^3*delta + cL.
    """
    for name, v in (("x", x), ("L", L), ("b", b), ("coord_bits", coord_bits)):
        if isinstance(v, bool) or not isinstance(v, int) or v < 0:
            raise ValueError(f"{name} must be non-negative int, got {v!r}: refusing")
    w = 1 << b
    if L % w != 0:
        raise ValueError(f"L={L} not aligned to 2^{b}: refusing")
    if not (L <= x < L + w):
        raise ValueError(f"x={x} outside leaf [{L},{L + w}): refusing")
    if b > coord_bits:
        raise ValueError(f"leaf width 2^{b} exceeds {coord_bits}-bit stage: refusing")
    delta = x - L
    cL = L >> b
    # exact paper form: shift varying bits to common scale, wrap fixed bits
    # (cL is absorbed into the offline coefficients within the leaf).
    d = (delta << (coord_bits - b)) + cL
    if d >= (1 << coord_bits):
        # cL can push d past stage width only when the leaf index is wide;
        # fail closed instead of silently masking (masking would corrupt fit).
        raise ValueError(f"d={d} exceeds {coord_bits}-bit stage: refusing")
    return d, delta, cL


def truncated_product(a: int, b: int, drop_cols: int) -> tuple[int, int]:
    """Exact vs truncated integer product. Drops `drop_cols` low columns.

    Returns (truncated, max_error_bound) where bound = 2^drop_cols - 1...
    precisely: the dropped part is (a*b) mod 2^drop_cols < 2^drop_cols.
    """
    for name, v in (("a", a), ("b", b), ("drop_cols", drop_cols)):
        if isinstance(v, bool) or not isinstance(v, int) or v < 0:
            raise ValueError(f"{name} must be non-negative int, got {v!r}: refusing")
    exact = a * b
    if drop_cols == 0:
        return exact, 0
    kept = (exact >> drop_cols) << drop_cols
    return kept, (1 << drop_cols) - 1


def fit_quadratic_integer(xs: list[int], ys: list[int], bits: tuple[int, int, int],
                          max_combos: int = 1 << 18) -> dict:
    """Brute-force integer quadratic q(d) = g0 - d*(g1 - g2*d) minimizing max err.

    Coefficient ranges: g0 in [min(ys),max(ys)] clipped to g0_bits signed?
    Kept simple + honest: unsigned ranges [0,2^bits-1] for g1,g2 and
    [min_y,max_y] for g0. Refuses when the space exceeds max_combos instead
    of silently subsampling.
    """
    if len(xs) != len(ys) or not xs:
        raise ValueError("xs/ys must be non-empty equal-length: refusing")
    b0, b1, b2 = bits
    for name, v in (("b0", b0), ("b1", b1), ("b2", b2)):
        if not isinstance(v, int) or v < 1 or v > 12:
            raise ValueError(f"{name} must be in [1,12], got {v!r}: refusing")
    lo, hi = min(ys), max(ys)
    if (hi - lo + 1) > (1 << b0):
        raise ValueError(
            f"g0 needs {hi - lo + 1} values but b0={b0} holds {1 << b0}: "
            "widen b0 instead of silently clipping")
    g0_vals = list(range(lo, hi + 1))
    g1_vals = list(range(1 << b1))
    g2_vals = list(range(1 << b2))
    space = len(g0_vals) * len(g1_vals) * len(g2_vals)
    if space > max_combos or space == 0:
        raise ValueError(f"fit space {space} exceeds cap {max_combos}: refusing silent subsample")
    best: dict | None = None
    for g0, g1, g2 in itertools.product(g0_vals, g1_vals, g2_vals):
        err = 0
        for x, y in zip(xs, ys):
            pred = g0 - x * (g1 - g2 * x)
            err = max(err, abs(pred - y))
            if best is not None and err >= best["max_err"]:
                break
        if best is None or err < best["max_err"]:
            best = {"g0": g0, "g1": g1, "g2": g2, "max_err": err}
            if err == 0:
                break
    assert best is not None
    return best


def accuracy_sweep(xs: list[int], exact_fn, coefs: dict) -> dict:
    """Worst-case error of q(d)=g0-d*(g1-g2*d) vs exact_fn over xs."""
    if not xs:
        raise ValueError("xs must be non-empty: refusing")
    g0, g1, g2 = coefs["g0"], coefs["g1"], coefs["g2"]
    worst = 0
    worst_x = xs[0]
    for x in xs:
        pred = g0 - x * (g1 - g2 * x)
        err = abs(pred - exact_fn(x))
        if err > worst:
            worst, worst_x = err, x
    return {"max_err": worst, "worst_x": worst_x, "n": len(xs)}


def gauss_fold_model(xs: int, n: int) -> dict:
    """Two's-complement fold onto the nonnegative axis (exact integer math).

    x = -2^{n-1} sign + mag; folded = mag if sign == 0 else ~mag (complement
    WITHOUT the +1: one carry chain removed). |folded - |x|| <= 1 LSB always
    (equality off the nonnegative axis) — the paper's one-unit offset, which
    the end-to-end error budget absorbs. Returns {folded, magnitude, error}.
    """
    if isinstance(xs, bool) or not isinstance(xs, int) or not (0 <= xs < (1 << n)):
        raise ValueError(f"xs must fit in {n} bits: refusing {xs!r}")
    if not isinstance(n, int) or n < 2:
        raise ValueError(f"n must be >= 2, got {n!r}: refusing")
    sign = (xs >> (n - 1)) & 1
    mag = xs & ((1 << (n - 1)) - 1)
    folded = mag if sign == 0 else ((1 << (n - 1)) - 1 - mag)
    true_abs = mag if sign == 0 else (1 << (n - 1)) - mag
    return {"folded": folded, "magnitude": true_abs,
            "error": abs(folded - true_abs)}


def emit_gauss_fold(b, x: list[int], out: list[int]) -> None:
    """Fold datapath (shared by the standalone builder and compositions).

    out_i ^= x_i ^ sign for data bits (complement iff negative, +1 omitted).
    Clifford-only; inputs preserved; out accumulates (caller zeroes).
    """
    n = len(x)
    if len(out) != n - 1:
        raise ValueError(f"need {n - 1} output bits for {n} input bits: refusing")
    for i in range(n - 1):
        b.cx(x[i], out[i])
        b.cx(x[n - 1], out[i])


def build_gauss_fold(n: int) -> tuple[object, dict]:
    """Fold circuit: out_i = x_i XOR sign (2(n-1) CNOTs, 0 Toffolis).

    Complementing via the sign bit costs only Cliffords; the omitted +1 is
    the removed carry chain (error pinned <= 1 LSB by the model above).
    Input preserved, fresh zeroed output (like the QROM loader pattern).
    """
    from .kickmix import Builder
    if not isinstance(n, int) or not (2 <= n <= 8):
        raise ValueError(f"n must be in [2,8] (exhaustive-proof scope), got {n!r}: refusing")
    b = Builder(2 * n - 1)
    x = list(range(n))
    out = list(range(n, 2 * n - 1))
    emit_gauss_fold(b, x, out)
    return b, {"x": tuple(x), "out": tuple(out), "ccx": 0}


def gauss_fold_spec(n: int):
    """TaskSpec: fresh out gets the folded coordinate (x preserved)."""
    from .kickmix import TaskSpec
    if not isinstance(n, int) or not (2 <= n <= 8):
        raise ValueError(f"n must be in [2,8], got {n!r}: refusing")

    def _perm(bits: dict[int, int]) -> dict[int, int]:
        out = dict(bits)
        xs = sum((bits.get(i, 0) & 1) << i for i in range(n))
        f = gauss_fold_model(xs, n)["folded"]
        for i in range(n - 1):
            out[n + i] = (f >> i) & 1
        return out

    return TaskSpec(num_qubits=2 * n - 1, inputs=tuple(range(n)),
                    outputs=tuple(range(n, 2 * n - 1)), target_perm=_perm)
