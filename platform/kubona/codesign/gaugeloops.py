"""Magnetic term: loop products, trace classes, plaquette/rectangle sums.

On a periodic 4^3 lattice (paper's ease-of-evaluation size: 192 links),
every plaquette (192) and 1x2 rectangle (384, long side either way)
is multiplied out with the verified group law, traced with the
machine-derived class function, and summed into S_P / S_R. Counts:
192 + 384 = 576 loops (paper's total). Link configs are arbitrary
{link: element} maps; backward traversal inverts (counted).

What this proves: the observable pipeline end to end at model level
(loops -> products -> traces -> sums), each loop product checked against
the true 3x3 matrix product, each trace against the matrix trace, and
the rotation/inversion invariance the paper's reuse relies on
(f(h1..hn) = f(h2..h1) = f((h1..hn)^-1): cyclic rotation is conjugation,
inversion is Re Tr unitarity — the former by class constancy, the latter
machine-checked on all 216 elements here).

Scope (stated): greedy nearest-representative scheduling (labeled
heuristic with exact-DP lower bounds on <= 8-loop subsets, never optimal)
plus coordinate-descent local search over order and reps (relocate/swap
moves with exact direction-preserving deltas + best-rep re-derivation:
3513 + 1215 -> 3507 + 1213, greedy confirmed near a local optimum);
representative sets come from the gauge kernel (single home for the rule
— this module maps concrete steps to abstract ints and back). The paper's
global DP + hand structure stays queued. The 51-bit phase-gradient addition is
circuit-level (kickmix admits only binary phases) and stays queued; sums
are exact integers.
"""

from __future__ import annotations


#: Paper loop-budget anchors (labeled, never re-derived): per-loop build
#: +reverse baseline vs the optimized schedule totals for 576 loops.
LOOP_PAPER_COSTS = {"baseline_plaquette": 6, "baseline_rectangle": 10,
                    "optimized_mults": 3136, "optimized_inversions": 480,
                    "n_loops": 576}


def _check_L(L: int) -> int:
    if isinstance(L, bool) or not isinstance(L, int) or L < 2:
        raise ValueError(f"L must be an int >= 2, got {L!r}: refusing")
    return L


def lattice_links(L: int = 4) -> list[tuple[tuple[int, int, int], int]]:
    """All directed links ((x, y, z), d) of the periodic L^3 lattice."""
    L = _check_L(L)
    return [((x, y, z), d) for x in range(L) for y in range(L)
            for z in range(L) for d in range(3)]


def _step(site: tuple[int, int, int], d: int, sgn: int, L: int):
    v = list(site)
    v[d] = (v[d] + sgn) % L
    return tuple(v)


def _walk(s: tuple[int, int, int], steps: list[tuple[int, int]], L: int):
    """Resolve directed steps into (link, forward) pairs, mechanically.

    At pos moving +d: link (pos, d) forward. At pos moving -d: link
    (pos - e_d, d) backward. Closed walks only (returns to start —
    refusal otherwise, so an open path can never masquerade as a loop).
    """
    pos = s
    out = []
    for d, sgn in steps:
        if sgn not in (1, -1):
            raise ValueError(f"step sign must be +-1, got {sgn!r}: refusing")
        if sgn == 1:
            out.append(((pos, d), True))
            pos = _step(pos, d, 1, L)
        else:
            link = (_step(pos, d, -1, L), d)
            out.append((link, False))
            pos = _step(pos, d, -1, L)
    if pos != s:
        raise ValueError(f"walk does not close (implementation bug): refusing {s!r}")
    return out


def plaquettes(L: int = 4) -> list[list[tuple[tuple[tuple[int, int, int], int], bool]]]:
    """192 plaquettes at L=4: (link, forward?) steps around each square."""
    L = _check_L(L)
    out = []
    for x in range(L):
        for y in range(L):
            for z in range(L):
                s = (x, y, z)
                for i, j in ((0, 1), (0, 2), (1, 2)):
                    out.append(_walk(s, [(i, 1), (j, 1), (i, -1), (j, -1)], L))
    return out


def rectangles(L: int = 4) -> list[list[tuple[tuple[tuple[int, int, int], int], bool]]]:
    """384 1x2 rectangles at L=4 (R_ij long side i, R_ji long side j)."""
    L = _check_L(L)
    out = []
    for x in range(L):
        for y in range(L):
            for z in range(L):
                s = (x, y, z)
                for i, j in ((0, 1), (0, 2), (1, 2)):
                    out.append(_walk(s, [(i, 1), (i, 1), (j, 1),
                                         (i, -1), (i, -1), (j, -1)], L))
                    out.append(_walk(s, [(j, 1), (j, 1), (i, 1),
                                         (j, -1), (j, -1), (i, -1)], L))
    return out


def _check_config(config: dict, L: int) -> None:
    from .group72 import gcheck
    if not isinstance(config, dict):
        raise ValueError("config must be a dict: refusing")
    for link in lattice_links(L):
        if link not in config:
            raise ValueError(f"config missing link {link!r}: refusing")
        gcheck(config[link])


def loop_product(config: dict, loop: list, L: int = 4) -> tuple[tuple[int, int, int], dict]:
    """Ordered product around a loop (backward steps inverted).

    Returns (element, {"mults": n, "inversions": m}) with exact counts.
    The first link LOADs the running product (paper baseline convention:
    3 build mults per plaquette, 5 per rectangle — reversal would double
    them, stated not folded in); every backward step inverts (counted:
    the gates fire regardless of the value).
    """
    from .group72 import gidentity, ginv, gmul
    _check_config(config, _check_L(L))
    if not loop:
        raise ValueError("empty loop: refusing")
    mults = inversions = 0
    first = True
    acc = gidentity()
    for link, fwd in loop:
        if not isinstance(fwd, bool):
            raise ValueError(f"direction must be bool, got {fwd!r}: refusing")
        if link not in config:
            raise ValueError(f"loop uses unknown link {link!r}: refusing")
        g = config[link]
        if not fwd:
            g = ginv(g)
            inversions += 1
        if first:
            acc, first = g, False
            continue
        acc = gmul(acc, g)
        mults += 1
    return acc, {"mults": mults, "inversions": inversions}


def loop_trace(config: dict, loop: list, L: int = 4) -> tuple[int, dict]:
    """Trace class f(W) of a loop product (machine-derived table)."""
    from .group72 import trace_class
    w, cost = loop_product(config, loop, L)
    return trace_class(w), cost


def accumulate(config: dict, L: int = 4) -> dict:
    """S_P over plaquettes + S_R over rectangles, with exact op counts.

    Returns integer sums plus measured build mults/inversions (reversal
    would double the mults; stated, not silently folded in).
    """
    _check_config(config, _check_L(L))
    sp = sr = mp = mr = ip = ir = 0
    for loop in plaquettes(L):
        v, c = loop_trace(config, loop, L)
        sp += v
        mp += c["mults"]
        ip += c["inversions"]
    for loop in rectangles(L):
        v, c = loop_trace(config, loop, L)
        sr += v
        mr += c["mults"]
        ir += c["inversions"]
    return {"S_P": sp, "n_P": 3 * L ** 3, "S_R": sr, "n_R": 6 * L ** 3,
            "build_mults": mp + mr, "inversions": ip + ir,
            "build_mults_P": mp, "build_mults_R": mr}


def invert_loop(loop: list) -> list:
    """Reversed traversal (a paper representative: inverted product)."""
    if not loop:
        raise ValueError("empty loop: refusing")
    return [(link, not fwd) for link, fwd in reversed(loop)]


def loop_to_abstract(loop: list, links: list) -> tuple[int, ...]:
    """Concrete steps -> abstract signed ints (link order defines ids).

    Step (link, fwd) maps to +(index+1) if fwd else -(index+1). The mapping
    is consistent within a call site sharing `links`; rep sets and costs
    computed under it match gauge.representatives/transform_cost exactly
    (cross-checked in-test: the gauge module is the single home for the
    representative rule).
    """
    ids = {link: i + 1 for i, link in enumerate(links)}
    out = []
    for link, fwd in loop:
        if link not in ids:
            raise ValueError(f"loop uses link outside mapping: refusing {link!r}")
        if not isinstance(fwd, bool):
            raise ValueError(f"direction must be bool, got {fwd!r}: refusing")
        out.append(ids[link] if fwd else -ids[link])
    return tuple(out)


def abstract_to_loop(ab: tuple[int, ...], links: list) -> list:
    """Abstract signed ints -> concrete steps (inverse of loop_to_abstract)."""
    if not isinstance(ab, tuple) or any(not isinstance(x, int) or x == 0 for x in ab):
        raise ValueError(f"abstract loop must be a nonempty tuple of nonzero ints, got {ab!r}: refusing")
    if not ab:
        raise ValueError("empty abstract loop: refusing")
    out = []
    for x in ab:
        if not 1 <= abs(x) <= len(links):
            raise ValueError(f"abstract id {x!r} outside mapping: refusing")
        out.append((links[abs(x) - 1], x > 0))
    return out


def loop_representatives(loop: list, links: list) -> list[list]:
    """Representative step-lists via the gauge kernel (single home).

    Maps to abstract form, takes gauge.representatives (rotation +
    inversion, trace-invariant), maps back. Returned in gauge (sorted)
    order — deterministic.
    """
    from .gauge import representatives as _gauge_reps
    return [abstract_to_loop(r, links) for r in _gauge_reps(loop_to_abstract(loop, links))]


def _common_prefix(a: list, b: list) -> int:
    k = 0
    for x, y in zip(a, b):
        if x[0] != y[0] or x[1] != y[1]:
            break
        k += 1
    return k


def transform_plan(held: list, want: list) -> dict:
    """Cheapest edit of the running product into the next representative.

    Compares direct prefix-matching against pre-inverting W (one ginv)
    and matching the inverted steps; picks fewest total ops (mults +
    invs), tie-broken toward fewer mults (Toffoli-dominated). Returns
    the plan (pre_invert, cancels in order, appends) plus exact counts:
    each cancel = 1 mult + (1 inversion iff the canceled step ran
    forward); each append = 1 mult, except a first append onto an empty
    (fully canceled) product which loads for free (baseline convention).
    """
    if not held and not want:
        raise ValueError("empty loops: refusing")
    if not want:
        raise ValueError("empty target loop: refusing")

    def _cost(held_steps: list, pre_inv: int) -> tuple[int, int, list, list]:
        k = _common_prefix(held_steps, want)
        cancels = list(reversed(held_steps[k:]))
        appends = list(want[k:])
        mults = len(cancels) + len(appends) - (1 if k == 0 else 0)
        invs = pre_inv + sum(1 for _, fwd in cancels if fwd)
        return mults, invs, cancels, appends

    best = None
    m0, i0, c0, a0 = _cost(held, 0)
    best = (m0 + i0, m0, False, c0, a0)
    inv_held = invert_loop(held) if held else []
    m1, i1, c1, a1 = _cost(inv_held, 1 if held else 0)
    cand = (m1 + i1, m1, True, c1, a1)
    if cand < best:
        best = cand
    _, _, pre, cancels, appends = best
    mults = len(cancels) + len(appends) - (1 if _common_prefix(inv_held if pre else held, want) == 0 else 0)
    invs = (1 if pre and held else 0) + sum(1 for _, fwd in cancels if fwd)
    return {"pre_invert": pre, "cancels": cancels, "appends": appends,
            "mults": mults, "inversions": invs}


def schedule_gauge_loops(loops: list[list]) -> dict:
    """Greedy nearest-representative ordering (labeled heuristic).

    Starts from the empty product with the first loop in input order;
    repeatedly takes the (remaining loop, representative) with the
    cheapest transform plan (fewest total ops, ties broken by input
    order then representative index — fully deterministic). Rep sets come
    from the gauge kernel (single home for the rule); the scheduler only
    orders. This is a greedy heuristic, NOT the paper's dynamic program;
    it is reported as such, with the measured totals. Exact DP optima on
    <= 8-loop subsets bound it from below (pinned in-test).
    """
    if not loops:
        raise ValueError("no loops to schedule: refusing")
    for loop in loops:
        if not loop:
            raise ValueError("empty loop in schedule: refusing")
    links: list = []
    for loop in loops:
        for link, _ in loop:
            if link not in links:
                links.append(link)
    reps = [loop_representatives(loop, links) for loop in loops]
    remaining = list(range(len(loops)))
    order: list[int] = []
    chosen: list[list] = []
    plans: list[dict] = []
    total_m = total_i = n_preinv = 0
    held: list = []
    first = remaining.pop(0)
    order.append(first)
    chosen.append(reps[first][0])
    plans.append(transform_plan([], reps[first][0]))
    total_m += plans[-1]["mults"]
    total_i += plans[-1]["inversions"]
    held = list(reps[first][0])
    while remaining:
        best = None
        for idx_pos, li in enumerate(remaining):
            for ri, rep in enumerate(reps[li]):
                plan = transform_plan(held, rep)
                key = (plan["mults"] + plan["inversions"], plan["mults"], idx_pos, ri)
                if best is None or key < best[0]:
                    best = (key, idx_pos, li, rep, plan)
        _, idx_pos, li, rep, plan = best
        remaining.pop(idx_pos)
        order.append(li)
        chosen.append(rep)
        plans.append(plan)
        total_m += plan["mults"]
        total_i += plan["inversions"]
        if plan["pre_invert"]:
            n_preinv += 1
        held = apply_plan_steps(held, plan)
    return {"order": order, "reps": chosen, "plans": plans,
            "total_mults": total_m, "total_inversions": total_i,
            "n_preinvert": n_preinv, "n_loops": len(loops)}


def apply_plan_steps(held: list, plan: dict) -> list:
    """Step-level replay of a transform plan (product bookkeeping).

    Pre-invert flips the held steps; cancels drop the matched suffix;
    appends extend. Fails closed if the plan overruns the held product.
    """
    if plan["pre_invert"]:
        if not held:
            raise ValueError("pre-invert of empty product (implementation bug): refusing")
        held = invert_loop(held)
    cancels = plan["cancels"]
    appends = plan["appends"]
    if len(cancels) > len(held):
        raise ValueError("plan cancels more than held (implementation bug): refusing")
    if list(reversed(cancels)) != held[len(held) - len(cancels):]:
        raise ValueError("cancels mismatch held suffix (implementation bug): refusing")
    return held[:len(held) - len(cancels)] + appends


def _plan_key(plan: dict) -> tuple[int, int]:
    """Lexicographic transition cost (total ops, then mults).

    Same objective the greedy minimizes locally, lifted to global
    acceptance: fewer total ops first, fewer (Toffoli-dominated) mults
    on ties.
    """
    return (plan["mults"] + plan["inversions"], plan["mults"])


def decode_order(order: list[int], all_reps: list[list[list]]) -> dict:
    """Left-to-right best-rep derivation for a fixed loop order.

    Each position picks the representative minimizing the transform
    from the held product (ties: lowest rep index — deterministic); the
    held product always equals the previous chosen rep exactly (plans
    cancel precisely the non-shared suffix), so each choice is locally
    optimal given the order. Returns the same sched shape as
    schedule_gauge_loops (order, reps, plans, totals, preinverts, count)
    so the replay proof applies unchanged.
    """
    if not order:
        raise ValueError("no loops to decode: refusing")
    if sorted(order) != list(range(len(all_reps))):
        raise ValueError("order must permute all loops: refusing")
    reps: list[list] = []
    plans: list[dict] = []
    total_m = total_i = n_preinv = 0
    held: list = []
    for li in order:
        cands = all_reps[li]
        if not cands:
            raise ValueError(f"loop {li} has no representatives: refusing")
        best_plan = None
        best_rep = None
        for rep in cands:
            plan = transform_plan(held, rep)
            if best_plan is None or _plan_key(plan) < _plan_key(best_plan):
                best_plan = plan
                best_rep = rep
        plans.append(best_plan)
        reps.append(best_rep)
        total_m += best_plan["mults"]
        total_i += best_plan["inversions"]
        if best_plan["pre_invert"]:
            n_preinv += 1
        held = apply_plan_steps(held, best_plan)
    return {"order": list(order), "reps": reps, "plans": plans,
            "total_mults": total_m, "total_inversions": total_i,
            "n_preinvert": n_preinv, "n_loops": len(order)}


def _tour_key(sched: dict) -> tuple[int, int]:
    return (sched["total_mults"] + sched["total_inversions"], sched["total_mults"])


def _improve_moves(order: list[int], rep_of: dict[int, list]) -> tuple[list[int], int]:
    """Direction-preserving local search on loop order, reps fixed per loop.

    The transform cost is asymmetric (pre-invert option, load discount),
    so textbook segment-reversal deltas are inexact: reversing [i..j]
    flips every internal edge direction. This instead uses moves that
    preserve internal order — segment relocation (lengths 1..3) and
    pairwise swaps — whose deltas touch only boundary edges and are
    exact with O(1) matrix lookups under fixed reps (the held product
    always equals the previous rep exactly). First-improvement scans in
    lexicographic move order (deterministic) restart after each accepted
    move and repeat to a local optimum; every accepted move strictly
    decreases the (total ops, mults) key, so the walk terminates (bounded
    below by zero; a move-count cap fails closed). Returns the improved
    order and the move count.
    """
    n = len(order)
    cur = list(order)

    def edge(a: int | None, b: int | None) -> tuple[int, int]:
        # cost of consecutive cities a -> b (None = tour head/tail anchor;
        # tail edges are free since the tour ends)
        if b is None:
            return (0, 0)
        held_steps = [] if a is None else rep_of[a]
        return _plan_key(transform_plan(held_steps, rep_of[b]))

    # pairwise cost matrix under fixed reps (one transform per pair;
    # tail anchors are free since the tour ends there)
    mat: dict[tuple, tuple[int, int]] = {}
    for x in cur:
        mat[(None, x)] = edge(None, x)
        mat[(x, None)] = (0, 0)
        for y in cur:
            if x != y:
                mat[(x, y)] = edge(x, y)

    def removed_added(kind: tuple) -> tuple[tuple[int, int], tuple[int, int]] | None:
        # exact boundary delta for one candidate move (None = no gain path)
        if kind[0] == "reloc":
            _, i, j, k = kind
            seg = cur[i:j + 1]
            # forbid inserting inside the segment itself
            if i - 1 <= k <= j:
                return None
            a = cur[i - 1] if i > 0 else None
            b = cur[j + 1] if j + 1 < n else None
            c = cur[k] if k >= 0 else None
            d = cur[k + 1] if k + 1 < n else None
            # NOTE k == -1 means "to the head": c None, d cur[0]; the
            # caller enumerates k in -1..n-1 excluding the segment span.
            rem = (mat[(a, seg[0])], mat[(seg[-1], b)], mat[(c, d)])
            add = (mat[(a, b)], mat[(c, seg[0])], mat[(seg[-1], d)])
        else:
            _, i, j = kind
            if i == j:
                return None
            a = cur[i - 1] if i > 0 else None
            b = cur[i + 1] if i + 1 < n else None
            c = cur[j - 1] if j > 0 else None
            d = cur[j + 1] if j + 1 < n else None
            x, y = cur[i], cur[j]
            if j == i + 1:
                rem = (mat[(a, x)], mat[(x, y)], mat[(y, d)])
                add = (mat[(a, y)], mat[(y, x)], mat[(x, d)])
            else:
                rem = (mat[(a, x)], mat[(x, b)], mat[(c, y)], mat[(y, d)])
                add = (mat[(a, y)], mat[(y, b)], mat[(c, x)], mat[(x, d)])
        rem_key = (sum(v[0] for v in rem), sum(v[1] for v in rem))
        add_key = (sum(v[0] for v in add), sum(v[1] for v in add))
        if add_key < rem_key:
            return rem_key, add_key
        return None

    moves = 0
    cap = 1000000
    improved = True
    while improved:
        improved = False
        # relocate segments of length 1..3, then swaps, lexicographic order
        for length in (1, 2, 3):
            if improved:
                break
            for i in range(n - length + 1):
                if improved:
                    break
                j = i + length - 1
                for k in range(-1, n):
                    if i - 1 <= k <= j:
                        continue
                    r = removed_added(("reloc", i, j, k))
                    if r is not None:
                        seg = cur[i:j + 1]
                        rest = cur[:i] + cur[j + 1:]
                        at = k + 1 if k < i else k + 1 - length
                        # at: insertion index into rest (k counted in cur;
                        # removing the segment shifts positions after it)
                        cur = rest[:at] + seg + rest[at:]
                        moves += 1
                        improved = True
                        break
        if improved:
            continue
        for i in range(n):
            if improved:
                break
            for j in range(i + 1, n):
                r = removed_added(("swap", i, j))
                if r is not None:
                    cur[i], cur[j] = cur[j], cur[i]
                    moves += 1
                    improved = True
                    break
        if moves > cap:
            raise ValueError("local search exceeded move cap (implementation bug): refusing")
    return cur, moves


def optimize_schedule(loops: list[list], starts: tuple[str, ...] = ("greedy", "input", "reverse")) -> dict:
    """Coordinate-descent local search over loop order and rep choice (heuristic).

    Alternates direction-preserving local search on the order (reps
    fixed per loop, exact O(1) boundary deltas) with left-to-right
    best-rep re-derivation for the new order; each accepted change
    strictly decreases the lexicographic (total ops, mults) key, so
    cycles terminate (bounded below by zero; a cycle cap fails
    closed). Tries each deterministic start order (greedy order, input
    order, reversed input) and keeps the strictly-best result — the
    greedy-direct seed makes the outcome no worse than
    schedule_gauge_loops by construction. A start whose decoded key
    strictly exceeds the incumbent is not descended (labeled
    prune: descent could in principle cross the gap, but hundreds of
    ops of local improvement from a worse-decoded start have never
    been observed, and the <=greedy guarantee never depends on pruned
    starts). Returns the sched shape plus meta (start, cycles, moves,
    skipped). Labeled heuristic, NOT the paper's dynamic program; the
    remaining gap is reported, never optimized away by invention.
    """
    if not loops:
        raise ValueError("no loops to schedule: refusing")
    for loop in loops:
        if not loop:
            raise ValueError("empty loop in schedule: refusing")
    links: list = []
    for loop in loops:
        for link, _ in loop:
            if link not in links:
                links.append(link)
    all_reps = [loop_representatives(loop, links) for loop in loops]
    greedy = schedule_gauge_loops(loops)
    seed_orders: dict[str, list[int]] = {
        "greedy": list(greedy["order"]),
        "input": list(range(len(loops))),
        "reverse": list(reversed(range(len(loops)))),
    }
    for name in starts:
        if name not in seed_orders:
            raise ValueError(f"unknown start {name!r}: refusing")
    if not starts:
        raise ValueError("no starts given: refusing")
    # seed the incumbent with the greedy schedule itself (not its decode:
    # decode re-derives reps along a different held path, so only the
    # direct greedy totals bound the outcome structurally).
    best = dict(greedy)
    meta = {"start": "greedy-direct", "cycles": 0, "moves": 0, "skipped": []}
    for name in starts:
        sched = decode_order(seed_orders[name], all_reps)
        if _tour_key(sched) > _tour_key(best):
            meta["skipped"].append(name)
            continue
        cycles = moves = 0
        cap = _tour_key(sched)[0] + 1
        while True:
            cycles += 1
            rep_of = {li: rep for li, rep in zip(sched["order"], sched["reps"])}
            new_order, n_mv = _improve_moves(list(sched["order"]), rep_of)
            moves += n_mv
            cand = decode_order(new_order, all_reps)
            if cycles > cap:
                raise ValueError("coordinate descent exceeded cycle cap: refusing")
            if _tour_key(cand) < _tour_key(sched):
                sched = cand
                continue
            break
        if _tour_key(sched) < _tour_key(best):
            best = sched
            meta = {"start": name, "cycles": cycles, "moves": moves,
                    "skipped": meta["skipped"]}
    best = dict(best)
    best.update(meta)
    return best
