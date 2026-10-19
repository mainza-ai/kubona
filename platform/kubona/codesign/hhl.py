"""HHL rotation-angle machinery (paper SI III.1) as verified integer math.

Task: X in [2^16, 2^21) (x = X/2^16 in [1,32)), f(X) = round(2^32·asin(2^15/X))
with worst-case error below 1e-7 radians (~429 LSBs of f).

Mechanics implemented (all verified, none narrated):
- dyadic leaves [L, L+2^b), L multiple of 2^b, exact cover, widths 2^8..2^15
- shared coordinate by cyclic shift of the low 15 bits: within a leaf,
  d = 2^{15-b}·delta + cL with cL leaf-constant (ASSERTED per leaf)
- per-leaf integer quadratic q(d) = g0 - d·(g1 - g2·d), fitted then
  EXHAUSTIVELY verified (every X in the leaf)
- adaptive refinement: split failing leaves until pass or width floor 2^8,
  then refuse (honest capacity bound, never a silent coarse leaf)

What this is NOT: the paper's offline search fits, truncated products, or
Toffoli counts. Fits here are least-squares + integer rounding, verified;
truncation is modeled in numerics.py. Error target identical to paper's.
"""

from __future__ import annotations

import math

LO15 = 0x7FFF
X_MIN = 1 << 16
X_MAX = 1 << 21  # exclusive
B_MIN, B_MAX = 8, 15
RAD_TOL = 1e-7
LSB_TOL = 2**32 * RAD_TOL  # ~= 429.5


def true_f(x: int) -> int:
    """Oracle: round(2^32 · asin(2^15 / X)). Floats are the reference here."""
    if not (X_MIN <= x < X_MAX):
        raise ValueError(f"X must be in [{X_MIN},{X_MAX}), got {x!r}: refusing")
    return round((1 << 32) * math.asin((1 << 15) / x))


def rotl15(v: int, s: int) -> int:
    """Rotate-left of a 15-bit value by s (0 <= s < 15)."""
    if not (0 <= v <= LO15):
        raise ValueError(f"15-bit value required, got {v!r}: refusing")
    s %= 15
    return ((v << s) | (v >> (15 - s))) & LO15 if s else v


def shared_coord(x: int, b: int) -> tuple[int, int]:
    """Shared 15-bit coordinate d for X in a width-2^b leaf.

    Returns (d, cL) with d = rotl15(low15(X), 15-b). The caller asserts the
    affine law d == 2^{15-b}·(X-L) + cL per leaf (property test below).
    """
    if not (B_MIN <= b <= B_MAX):
        raise ValueError(f"leaf width 2^{b} outside [{B_MIN},{B_MAX}]: refusing")
    if not (X_MIN <= x < X_MAX):
        raise ValueError(f"X out of domain: refusing {x!r}")
    return rotl15(x & LO15, 15 - b), 0  # cL filled by the leaf check


def leaf_affine_law(leaf: tuple[int, int]) -> int:
    """Verify d == 2^{15-b}·delta + cL for every X in the leaf.

    Returns the leaf-constant cL. Raises on any violation (the shared-kernel
    premise fails) — this is the load-bearing property of the whole scheme.
    """
    L, b = leaf
    w = 1 << b
    if L % w != 0:
        raise ValueError(f"leaf L={L} misaligned to 2^{b}: refusing")
    scale = 1 << (15 - b)
    cL = None
    for x in range(L, min(L + w, X_MAX)):
        d, _ = shared_coord(x, b)
        c = d - scale * (x - L)
        if cL is None:
            cL = c
        elif c != cL:
            raise ValueError(f"affine law broken at X={x} in leaf {(L, b)}: refusing")
    if cL is None:
        raise ValueError(f"empty leaf {(L, b)}: refusing")
    return cL


#: Fixed-point fractional bits (uniform, conservative — wider than needed,
#: costing MORE in a real cost model, never less). Slopes need sub-LSB
#: resolution (flat-end g1 ~= 0.25 LSB/d-unit); S=28 was required to clear
#: the 1e-7-rad bar on the steepest floor leaves (S=24 stalled at 430 LSB).
S1 = S2 = 28


def qeval(d: int, g0: int, g1: int, g2: int) -> int:
    """Integer Horner model q(d) = g0 - ((d*t) >> S1), t = g1 - g2*d*2^(S1-S2).

    g1 carries S1 fractional bits, g2 S2. The inner product g2*d is rescaled
    by EXACTLY S2-S1 (a bare >>S2 is correct only for integer slopes and
    truncates sub-LSB curvature to zero — caught live at 859 LSB, fixed
    here). >> is floor (Python semantics), deterministic and end-to-end
    verified — the error is MEASURED, so shift semantics cannot flatter it.
    """
    if S2 >= S1:
        t = g1 - ((g2 * d) >> (S2 - S1))
    else:
        t = g1 - ((g2 * d) << (S1 - S2))
    return g0 - ((d * t) >> S1)


def fit_leaf(leaf: tuple[int, int]) -> dict:
    """Least-squares fixed-point quadratic in shared coordinate d.

    Returns {g0, g1, g2, S1, S2, max_err_lsb, ...} with max over EVERY X.
    The float fit is a proposal; integer rounding is verified, not assumed.
    """
    import numpy as np
    L, b = leaf
    w = 1 << b
    xs = [x for x in range(L, min(L + w, X_MAX))]
    cL = leaf_affine_law(leaf)
    scale = 1 << (15 - b)
    ds = [scale * (x - L) + cL for x in xs]
    ys = [true_f(x) for x in xs]
    coef = np.polyfit(ds, ys, 2)  # [g2', g1', g0'] with q = g0'+g1'd+g2'd^2
    g2f, g1f, g0f = (float(v) for v in coef)
    # model q(d) = g0 - d*(g1 - g2*d): g1 slope, g2 curvature, fixed-point.
    # Rounded least-squares is the PROPOSAL here; refine_fit (minimax hill-
    # climb) runs only via fit_leaf_refined on floor leaves that fail —
    # refining a wide leaf that splits anyway would burn search for nothing.
    g0, g1, g2 = round(g0f), round(-g1f * (1 << S1)), round(g2f * (1 << S2))
    worst, worst_x = 0, xs[0]
    for x, d, y in zip(xs, ds, ys):
        err = abs(qeval(d, g0, g1, g2) - y)
        if err > worst:
            worst, worst_x = err, x
    return {"leaf": leaf, "cL": cL, "g0": g0, "g1": g1, "g2": g2,
            "S1": S1, "S2": S2, "max_err_lsb": worst, "worst_x": worst_x,
            "max_err_rad": worst / 2**32, "n": len(xs)}


def _max_err(ds: list[int], ys: list[int], g0: int, g1: int, g2: int) -> int:
    worst = 0
    for d, y in zip(ds, ys):
        err = abs(qeval(d, g0, g1, g2) - y)
        if err > worst:
            worst = err
    return worst


def refine_fit(ds: list[int], ys: list[int], g0: int, g1: int, g2: int,
               restarts: int = 3, seed: int = 0) -> tuple[int, int, int]:
    """Coordinate descent on measured max-error (the paper's offline search,
    minimized-verified: start from rounded least-squares, hill-climb integer
    coefficients; every step measured with qeval, never assumed).

    Minimax coordinate descent can stick in local minima, so `restarts`
    deterministic perturbations (seeded PRNG, recorded) re-seed the climb;
    the best over all starts wins. Deterministic: same inputs, same output.
    """
    import random
    # int-composed seed (tuple seeds are rejected; hash() is salted and
    # would break cross-process determinism — same inputs, same output).
    rng = random.Random(seed * 1_000_003 + len(ds) * 1_009 + (g0 & 0xFFFF))
    starts = [(g0, g1, g2)]
    for _ in range(restarts):
        starts.append((g0 + rng.randint(-512, 512),
                       g1 + rng.randint(-(1 << 22), 1 << 22),
                       g2 + rng.randint(-(1 << 22), 1 << 22)))
    best: tuple[int, int, int] | None = None
    best_err = None
    schedules = {
        0: [1 << k for k in range(12, -1, -1)],
        1: [1 << k for k in range(24, -1, -1)],
        2: [1 << k for k in range(24, -1, -1)],
    }
    for start in starts:
        cand_best, cand_err = start, _max_err(ds, ys, *start)
        for _ in range(3):  # rounds over coordinates (cross-term interaction)
            moved = False
            for coord in (0, 1, 2):
                i = 0
                while i < len(schedules[coord]):
                    step = schedules[coord][i]
                    improved = False
                    for direction in (+1, -1):
                        cand = list(cand_best)
                        cand[coord] += direction * step
                        err = _max_err(ds, ys, *cand)
                        if err < cand_err:
                            cand_best, cand_err = tuple(cand), err
                            improved, moved = True, True
                            break
                    if not improved:
                        i += 1  # shrink step only when this size stops helping
            if not moved:
                break
        if best_err is None or cand_err < best_err:
            best, best_err = cand_best, cand_err
    assert best is not None and best_err is not None
    return best


def refine_domain(lo: int = X_MIN, hi: int = X_MAX) -> list[dict]:
    """Adaptive dyadic partition: split failing leaves to width floor 2^8.

    Starts from maximal dyadic blocks; splits any leaf whose fitted
    max_err exceeds LSB_TOL. Refuses if a width-2^8 leaf still fails
    (capacity bound — the quadratic family is insufficient there, and no
    finer leaf is allowed).
    """
    # maximal dyadic tiling of [lo, hi) with widths <= 2^B_MAX
    leaves: list[tuple[int, int]] = []
    L = lo
    while L < hi:
        b = min(B_MAX, (hi - L).bit_length() - 1)
        while b > 0 and (L % (1 << b) != 0 or L + (1 << b) > hi):
            b -= 1
        leaves.append((L, b))
        L += 1 << b
    fitted: list[dict] = []
    stack = list(leaves)
    while stack:
        leaf = stack.pop()
        L, b = leaf
        fit = fit_leaf(leaf)
        if fit["max_err_lsb"] <= LSB_TOL:
            fitted.append(fit)
            continue
        if b <= B_MIN:
            # last resort before refusing: minimax hill-climb from the
            # rounded fit (rounded LS minimizes mean-square, not minimax).
            import numpy as np
            w = 1 << b
            xs = [x for x in range(L, min(L + w, X_MAX))]
            scale = 1 << (15 - b)
            cL = fit["cL"]
            ds = [scale * (x - L) + cL for x in xs]
            ys = [true_f(x) for x in xs]
            g0, g1, g2 = refine_fit(ds, ys, fit["g0"], fit["g1"], fit["g2"])
            worst = 0
            for d, y in zip(ds, ys):
                err = abs(qeval(d, g0, g1, g2) - y)
                worst = max(worst, err)
            if worst <= LSB_TOL:
                fit.update({"g0": g0, "g1": g1, "g2": g2,
                            "max_err_lsb": worst,
                            "max_err_rad": worst / 2**32,
                            "refined": True})
                fitted.append(fit)
                continue
            raise ValueError(
                f"leaf {(L, b)} fails at width floor 2^{B_MIN} "
                f"(err {worst:.0f} > {LSB_TOL:.0f} even refined): refusing")
        w = 1 << (b - 1)
        stack += [(L, b - 1), (L + w, b - 1)]
    fitted.sort(key=lambda f: f["leaf"][0])
    # exact-cover check (no gaps/overlaps — belt, construction guarantees it)
    pos = lo
    for f in fitted:
        if f["leaf"][0] != pos:
            raise ValueError("partition gap/overlap (implementation bug): refusing")
        pos = min(f["leaf"][0] + (1 << f["leaf"][1]), hi)
    if pos != hi:
        raise ValueError("partition does not cover domain (implementation bug): refusing")
    return fitted


def sweep_report(fitted: list[dict]) -> dict:
    """Worst-case error over the fitted partition (all leaves exhaustive)."""
    worst = max(f["max_err_lsb"] for f in fitted)
    arg = next(f for f in fitted if f["max_err_lsb"] == worst)
    return {"leaves": len(fitted), "worst_lsb": worst,
            "worst_rad": worst / 2**32, "worst_at": arg["worst_x"],
            "worst_leaf": arg["leaf"], "within_tol": worst <= LSB_TOL}


#: Toy task leaves over X in [8,16): (members, width exponent). Dyadic:
#: [8,10) and [10,12) are width-2 aligned; [12,16) is width-4 aligned.
TOY_LEAVES: dict[str, tuple[list[int], int]] = {
    "A": ([8, 9], 1), "B": ([10, 11], 1), "C": ([12, 13, 14, 15], 2)}


def toy_true_f(X: int) -> int:
    """Toy oracle: round(48*asin(7/X)) for X in [8,16) (steeper than HHL)."""
    if not 8 <= X < 16:
        raise ValueError(f"toy domain is [8,16), got {X!r}: refusing")
    return round(48 * math.asin(7 / X))


def hhl_toy_fits() -> dict:
    """Brute-force integer line fits per toy leaf (exact, deterministic).

    Shared coordinate d = rotl4(X, 4-b); q(d) = g0 - d*g1 with g0 in
    [0,63], g1 in [0,7]; non-negativity pinned per leaf (else refuse).
    First minimal wins (g0 outer, g1 inner) — same inputs, same output.
    """
    from .passes import rotl_words
    out = {}
    for name, (xs, b) in TOY_LEAVES.items():
        ds = [rotl_words(X, 4 - b, 4) for X in xs]
        ys = [toy_true_f(X) for X in xs]
        best = None
        for g0 in range(64):
            for g1 in range(8):
                if any(g0 - d * g1 < 0 for d in ds):
                    continue
                err = max(abs((g0 - d * g1) - y) for d, y in zip(ds, ys))
                if best is None or err < best[0]:
                    best = (err, g0, g1)
        if best is None:
            raise ValueError(f"no non-negative fit for leaf {name}: refusing")
        out[name] = {"xs": xs, "b": b, "ds": ds, "g0": best[1],
                     "g1": best[2], "max_err": best[0]}
    return out


#: QROM row order addressed by (x2, x1): x2=1 rows share leaf C.
LEAF_ROWS = ["A", "C", "B", "C"]


def build_hhl_toy_task(fits: dict | None = None) -> tuple[object, dict]:
    """Dyadic-select + shift + accumulate task circuit (paper dataflow, toy).

    X[4] input (full 4-bit domain; accuracy promised on [8,16), formula
    defined everywhere by the same fitted tables); K[8] fresh output.
    Per X: leaf by (x2,x1) into rows [A,C,B,C] (x2=1 rows share C); QROM
    coefficient loads (6 intercept + 3 slope bit-planes, shared pool);
    conditional cyclic shift to the shared coordinate (unconditional
    distance-2 swaps + ~x2-controlled 4-cycle, X preserved); Horner
    accumulate (k += g0, k -= d*t via proven emitters); full uncompute
    (reverse shift, re-emitted QROM unloads) and every temp freed.
    """
    from .horner import emit_add, emit_mul_rows, emit_sub, _exec
    from .kickmix import Builder
    from .passes import emit_cswap
    from .qrom import emit_pruned_qrom_into
    if fits is None:
        fits = hhl_toy_fits()
    if set(fits) != {"A", "B", "C"}:
        raise ValueError("fits must cover leaves A/B/C: refusing")
    X = [0, 1, 2, 3]
    K = list(range(4, 12))
    D = list(range(12, 16))
    G0 = list(range(16, 22))
    G1 = list(range(22, 25))
    TNOT = [25]
    QPOOL = list(range(26, 30))
    P = list(range(30, 37))
    ROW = list(range(37, 41))
    CARR = list(range(41, 48))
    ZERO = 48
    G0T = list(range(49, 56))
    NQ = 56
    b = Builder(NQ)
    for q in D + G0 + G1 + TNOT + QPOOL + P + ROW + CARR + [ZERO] + G0T:
        b.alloc(q)
    n_ccx = 0
    # --- coefficient tables: rows [A,C,B,C] addressed by (x2,x1) ---
    order = list(LEAF_ROWS)
    g0s = [fits[L]["g0"] for L in order]
    g1s = [fits[L]["g1"] for L in order]
    addr = [2, 1]
    for j in range(6):
        n_ccx += emit_pruned_qrom_into(b, [(g >> j) & 1 for g in g0s], True,
                                       addr, G0[j], QPOOL)
    for j in range(3):
        n_ccx += emit_pruned_qrom_into(b, [(g >> j) & 1 for g in g1s], True,
                                       addr, G1[j], QPOOL)
    # --- shared coordinate: unconditional rotl-2 swaps + ~x2 4-cycle ---
    for i in range(4):
        b.cx(X[i], D[i])  # copy (X preserved throughout as controls only)
    b.swap(D[0], D[2])
    b.swap(D[1], D[3])  # rotl by 0 or 2 (s1 is always 1: plain swaps, 0T)
    b.x(TNOT[0])
    b.cx(X[2], TNOT[0])  # TNOT = ~x2 (shift iff x2 == 0)
    for a, c in ((2, 3), (1, 2), (0, 1)):  # 4-cycle in reverse-wire order:
        n_ccx += emit_cswap(b, TNOT[0], D[a], D[c])  # LEFT rotl by 0/1
    # --- accumulate: p = d*g1, k += g0, k -= p (proven horner emitters) ---
    t_mul, tun = emit_mul_rows(b, D, G1, P, ROW, CARR[:6], ZERO, 0)
    n_ccx += t_mul
    for i in range(6):  # G0T = g0 (zero-extended to 7 bits)
        b.cx(G0[i], G0T[i])
    n_ccx += emit_add(b, K, G0T, CARR)
    for i in range(6):
        b.cx(G0[i], G0T[i])  # unload
    n_ccx += emit_sub(b, K, P, CARR)
    n_ccx += _exec(b, tun)  # clear P (SUB-based unplan, horner-proven)
    # --- uncompute: reverse shift, unload coefs (re-emitted QROMs clear by
    # re-accumulation), free everything but X (preserved) and K (answer) ---
    for a, c in ((0, 1), (1, 2), (2, 3)):
        n_ccx += emit_cswap(b, TNOT[0], D[a], D[c])
    b.cx(X[2], TNOT[0])
    b.x(TNOT[0])
    b.swap(D[0], D[2])
    b.swap(D[1], D[3])
    for i in range(4):
        b.cx(X[i], D[i])  # uncopy
    for j in reversed(range(3)):
        n_ccx += emit_pruned_qrom_into(b, [(g >> j) & 1 for g in g1s], True,
                                       addr, G1[j], QPOOL)
    for j in reversed(range(6)):
        n_ccx += emit_pruned_qrom_into(b, [(g >> j) & 1 for g in g0s], True,
                                       addr, G0[j], QPOOL)
    for q in D + G0 + G1 + TNOT + QPOOL + P + ROW + CARR + [ZERO] + G0T:
        b.free(q)
    return b, {"x": tuple(X), "k": tuple(K), "ccx": n_ccx,
               "fits": {L: (fits[L]["g0"], fits[L]["g1"]) for L in ("A", "B", "C")}}


def hhl_toy_spec(fits: dict | None = None):
    """TaskSpec: fresh K gets the leaf quadratic at the shifted coordinate.

    Target reads the same fitted tables as the circuit (like horner coefs)
    with the shared coordinate from rotl_words; defined on the full 4-bit
    input by that formula, accuracy promised on [8,16). 16/16 exhaustive.
    """
    from .kickmix import TaskSpec
    from .passes import rotl_words
    if fits is None:
        fits = hhl_toy_fits()
    if set(fits) != {"A", "B", "C"}:
        raise ValueError("fits must cover leaves A/B/C: refusing")

    def _perm(bits: dict[int, int]) -> dict[int, int]:
        out = dict(bits)
        xv = sum((bits.get(i, 0) & 1) << i for i in range(4))
        leaf = LEAF_ROWS[(bits.get(2, 0) & 1) + 2 * (bits.get(1, 0) & 1)]
        b = fits[leaf]["b"]
        d = rotl_words(xv, 4 - b, 4)
        kv = fits[leaf]["g0"] - d * fits[leaf]["g1"]
        for i in range(8):
            out[4 + i] = (kv >> i) & 1
        return out

    anc = tuple(list(range(12, 56)))
    return TaskSpec(num_qubits=56, inputs=(0, 1, 2, 3), outputs=tuple(range(4, 12)),
                    ancillas=anc, target_perm=_perm)
