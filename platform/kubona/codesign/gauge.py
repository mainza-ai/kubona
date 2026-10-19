"""Nonabelian lattice-gauge helpers at toy scale (SI V, exact, no matrices).

Two pieces of the paper's construction as verifiable integer math:

1. Folded phase coordinate tildeP = P + Q*R mod 3 (SI V, specialized
   running-product representation). Reordering C,E factors past each other
   would ordinarily reintroduce a correction to P on every multiplication;
   folding it into the coordinate makes the update closed:
     tildeP <- tildeP + p + q*r + Q*r + 2*R*q  (mod 3)
   with (Q,R) <- (Q+q, R+r) plus the fixed V/Xo permutations. unfold()
   recovers P = tildeP - Q*R; fold(unfold()) round-trips exactly.

2. Loop-schedule DP (Fig plaquette-layer(a) + text): each loop's trace is
   invariant under rotation and inversion (f(h1..hn) = f(rot) = f(inv)), so
   each loop offers several representatives with the same physics. Retaining
   W across loops turns the layer into an ordering + representative-choice
   problem: transform cost from rep A to rep B is (#suffix cancels +
   #appends) by right-multiplication, optionally via one W inversion (which
   mirrors the product and opens the other end). schedule_loops() solves it
   exactly (Held-Karp over reps x order, n <= 8 loops of length <= 6) and
   reports mults/invs separately — the paper's 3,136 mults + 480 invs
   pattern at toy scale, never their constants.

Scope (stated): link labels are abstract integers (negation = inverse
traversal); group relations are NOT modeled here (no Sigma(72x3) matrices).
The DP minimizes retained-product edits, which is exactly the paper's
scheduling subproblem once representatives are listed.
"""

from __future__ import annotations

import itertools


def fold_tildeP(P: int, Q: int, R: int) -> int:
    """Folded coordinate tildeP = P + Q*R mod 3."""
    for name, v in (("P", P), ("Q", Q), ("R", R)):
        if not isinstance(v, int) or not (0 <= v <= 2):
            raise ValueError(f"{name} must be in Z3, got {v!r}: refusing")
    return (P + Q * R) % 3


def unfold_tildeP(tildeP: int, Q: int, R: int) -> int:
    """Recover P = tildeP - Q*R mod 3."""
    for name, v in (("tildeP", tildeP), ("Q", Q), ("R", R)):
        if not isinstance(v, int) or not (0 <= v <= 2):
            raise ValueError(f"{name} must be in Z3, got {v!r}: refusing")
    return (tildeP - Q * R) % 3


def tildeP_update(tildeP: int, Q: int, R: int, p: int, q: int, r: int) -> tuple[int, int, int]:
    """One running-product C,E-fold update (paper Alg gauge-prod-b, first line).

    (Q,R) <- (Q+q, R+r); tildeP <- tildeP + p + q*r + Q*r + 2*R*q (mod 3).
    Returns (tildeP', Q', R'). The V/Xo permutation lines of the algorithm
    act on other coordinates and are out of scope here (stated).
    """
    for name, v in (("tildeP", tildeP), ("Q", Q), ("R", R), ("p", p), ("q", q), ("r", r)):
        if not isinstance(v, int) or not (0 <= v <= 2):
            raise ValueError(f"{name} must be in Z3, got {v!r}: refusing")
    Q2 = (Q + q) % 3
    R2 = (R + r) % 3
    t2 = (tildeP + p + q * r + Q * r + 2 * R * q) % 3
    # conservation gate: unfolding the update must advance P by p + R*q
    # (expand Q2*R2 = QR + Qr + qR + qr and cancel against the folded terms;
    # qR == Rq in Z3). A mismatch is an implementation bug, refused loudly.
    assert unfold_tildeP(t2, Q2, R2) == (unfold_tildeP(tildeP, Q, R) + p + R * q) % 3
    return t2, Q2, R2


def representatives(loop: tuple[int, ...]) -> list[tuple[int, ...]]:
    """Rotation + inversion representatives with identical trace (Eq trace-inv).

    Rotations (cyclicity) and the reversed-negated loop (inverse, real-part
    symmetry) all share f. Returns sorted unique reps. Loops must be
    non-empty tuples of nonzero ints (link labels, sign = direction).
    """
    if not isinstance(loop, tuple) or not loop or any(not isinstance(x, int) or x == 0 for x in loop):
        raise ValueError(f"loop must be non-empty tuple of nonzero ints: refusing {loop!r}")
    n = len(loop)
    if n > 6:
        raise ValueError(f"toy loops cap at length 6, got {n}: refusing")
    reps: set[tuple[int, ...]] = set()
    for k in range(n):
        reps.add(tuple(loop[(k + i) % n] for i in range(n)))
    inv = tuple(-loop[n - 1 - i] for i in range(n))
    for k in range(n):
        reps.add(tuple(inv[(k + i) % n] for i in range(n)))
    return sorted(reps)


def transform_cost(a: tuple[int, ...], b: tuple[int, ...]) -> tuple[int, int]:
    """Minimal (mults, invs) to retarget retained product A -> B.

    Plain: strip the non-shared suffix of A (1 mult per cancel), append the
    remainder of B (1 mult per link). Inverted: one W inversion then the same
    edit on reversed-negated A. Returns the cheaper in mults (ties: fewer
    invs, then plain). Empty A (W=1) costs len(B) appends.
    """
    for name, s in (("a", a), ("b", b)):
        if not isinstance(s, tuple) or any(not isinstance(x, int) or x == 0 for x in s):
            raise ValueError(f"{name} must be a tuple of nonzero ints: refusing")
    if len(a) == 0:
        return len(b), 0
    lcp = 0
    while lcp < min(len(a), len(b)) and a[lcp] == b[lcp]:
        lcp += 1
    plain = (len(a) - lcp) + (len(b) - lcp)
    ainv = tuple(-a[len(a) - 1 - i] for i in range(len(a)))
    lcp2 = 0
    while lcp2 < min(len(ainv), len(b)) and ainv[lcp2] == b[lcp2]:
        lcp2 += 1
    via_inv = (len(ainv) - lcp2) + (len(b) - lcp2)
    # Inversion itself costs one inversion (counted separately), not a mult:
    # compare mults first, then prefer fewer inversions on ties.
    if via_inv < plain:
        return via_inv, 1
    return plain, 0


def schedule_loops(loops: list[tuple[int, ...]]) -> dict:
    """Exact retained-product schedule (Held-Karp over reps x order).

    n <= 8 loops (fail-closed above: exponential DP, no heuristic presented
    as optimal). Returns {order, reps, mults, invs, naive_mults} where naive
    builds each loop from identity and reverses it (2*len per loop — the
    paper's baseline pattern) and the schedule retains W throughout.
    """
    if not isinstance(loops, list) or not loops or len(loops) > 8:
        raise ValueError(f"toy schedule needs 1..8 loops, got {len(loops) if isinstance(loops, list) else loops!r}: refusing")
    reps_list = [representatives(tuple(l)) for l in loops]
    n = len(loops)
    naive = sum(2 * len(tuple(l)) for l in loops)
    # DP over (mask, last, rep_idx): minimal (mults, invs)
    INF = (10 ** 9, 10 ** 9)
    dp: dict[tuple[int, int, int], tuple[int, int]] = {}
    par: dict[tuple[int, int, int], tuple[int, int, int] | None] = {}
    for i in range(n):
        for ri, r in enumerate(reps_list[i]):
            dp[(1 << i, i, ri)] = (len(r), 0)
            par[(1 << i, i, ri)] = None
    for mask in range(1 << n):
        for last in range(n):
            if not (mask & (1 << last)):
                continue
            for rl, rep_last in enumerate(reps_list[last]):
                cur = dp.get((mask, last, rl))
                if cur is None:
                    continue
                for nxt in range(n):
                    if mask & (1 << nxt):
                        continue
                    for rn, rep_nxt in enumerate(reps_list[nxt]):
                        m, v = transform_cost(rep_last, rep_nxt)
                        cand = (cur[0] + m, cur[1] + v)
                        key = (mask | (1 << nxt), nxt, rn)
                        if dp.get(key, INF) > cand:
                            dp[key] = cand
                            par[key] = (mask, last, rl)
    full = (1 << n) - 1
    best_key = min((k for k in dp if k[0] == full), key=lambda k: dp[k])
    mults, invs = dp[best_key]
    # reconstruct
    order: list[int] = []
    chosen: list[tuple[int, ...]] = [()] * n
    key: tuple[int, int, int] | None = best_key
    while key is not None:
        mask, last, rl = key
        order.append(last)
        chosen[last] = reps_list[last][rl]
        key = par[key]
    order.reverse()
    return {"order": order, "reps": [chosen[i] for i in order],
            "mults": mults, "invs": invs, "naive_mults": naive,
            "saved": naive - mults}
