"""Exact multi-output Boolean minimization (Quine-McCluskey + greedy cover).

Takes truth tables (over <= 10 vars: exhaustive domains only) to shared
AND-pools: prime implicants are derived EXACTLY by QM iteration (every
prime implicant found — machine-checked against brute force on probe
functions); the multi-output cover is GREEDY (stated heuristic, not
optimal — exact set cover is NP-hard), but every emitted cover is
VERIFIED cube-by-cube (covers exactly the onset, never the offset), and
every emitted circuit is proven exhaustive downstream. Invalid encodings
stay offset (never don't-cares): the readout spec pins them to zero and
redefining them now would be weakening, not optimization.

Cube representation: (value, care) int pair over nvars bits; bit i of
care = 1 means variable i is a literal with polarity bit i of value.
"""

from __future__ import annotations


def _check_table(table: dict[int, int], nvars: int) -> None:
    if not isinstance(nvars, int) or isinstance(nvars, bool) or not (1 <= nvars <= 10):
        raise ValueError(f"nvars must be in [1,10], got {nvars!r}: refusing")
    if not isinstance(table, dict):
        raise ValueError("table must be a dict: refusing")
    for k, v in table.items():
        if not isinstance(k, int) or not (0 <= k < (1 << nvars)):
            raise ValueError(f"key {k!r} outside {nvars} bits: refusing")
        if v not in (0, 1):
            raise ValueError(f"value {v!r} must be 0/1: refusing")


def _covers(cube: tuple[int, int], mask: int) -> bool:
    value, care = cube
    return (mask & care) == (value & care)


def prime_implicants(table: dict[int, int], nvars: int) -> set[tuple[int, int]]:
    """All prime implicants of a single-output function (exact QM).

    Starts from onset minterms, repeatedly combines cubes differing in
    exactly one care bit; uncombined cubes are prime. Verified in-test
    against brute-force prime enumeration on probe functions.
    """
    _check_table(table, nvars)
    full = (1 << nvars) - 1
    onset = {m for m, v in table.items() if v == 1}
    cur: set[tuple[int, int]] = {(m, full) for m in onset}
    primes: set[tuple[int, int]] = set()
    while cur:
        nxt: set[tuple[int, int]] = set()
        used: set[tuple[int, int]] = set()
        by_care: dict[int, list[tuple[int, int]]] = {}
        for c in cur:
            by_care.setdefault(c[1], []).append(c)
        for care, group in by_care.items():
            for i, a in enumerate(group):
                for b in group[i + 1:]:
                    diff = (a[0] ^ b[0]) & care
                    if diff and (diff & (diff - 1)) == 0:
                        bit = diff.bit_length() - 1
                        merged = (a[0] & ~(1 << bit), care & ~(1 << bit))
                        nxt.add(merged)
                        used.add(a)
                        used.add(b)
        for c in cur:
            if c not in used:
                primes.add(c)
        cur = nxt
    # every prime must cover only onset minterms (else the table lied)
    for (value, care) in primes:
        for m in range(1 << nvars):
            if _covers((value, care), m) and table.get(m, 0) != 1:
                raise ValueError(f"prime {value:09b}/{care:09b} covers offset {m}: refusing")
    return primes


def _eval_cover(cubes: list[tuple[int, int]], mask: int) -> int:
    out = 0
    for c in cubes:
        if _covers(c, mask):
            out ^= 1
    return out


def _xor_cubes(a: tuple[int, int], b: tuple[int, int], nvars: int) -> tuple[int, int] | None:
    """If a XOR b (as minterm sets) is a single cube, return it."""
    full = (1 << nvars) - 1
    # collect minterms (only feasible for small nvars; caller guards size)
    sa = {m for m in range(1 << nvars) if _covers(a, m)}
    sb = {m for m in range(1 << nvars) if _covers(b, m)}
    sym = sa ^ sb
    if not sym:
        return None
    # single cube containing exactly sym: care = bits constant across sym
    first = next(iter(sym))
    care = full
    for m in sym:
        care &= ~(first ^ m)
    value = first & care
    if {m for m in range(1 << nvars) if _covers((value, care), m)} == sym:
        return (value, care)
    return None


def _reduce_cover(cubes: list[tuple[int, int]], nvars: int) -> list[tuple[int, int]]:
    """Exact pairwise reduction: duplicate pairs cancel, adjacent pairs merge.

    Both operations preserve the XOR-sum exactly. Iterated to fixpoint.
    Only attempted when the minterm space is small enough to enumerate.
    """
    if nvars > 12 or len(cubes) > 400:
        return list(cubes)
    cur = list(cubes)
    while True:
        changed = False
        # duplicate pairs cancel (c XOR c = 0)
        seen: dict[tuple[int, int], int] = {}
        for c in cur:
            seen[c] = seen.get(c, 0) + 1
        nxt = [c for c, k in seen.items() if k % 2 == 1]
        if len(nxt) != len(cur):
            cur, changed = nxt, True
            continue
        merged = False
        for i in range(len(cur)):
            for j in range(i + 1, len(cur)):
                m = _xor_cubes(cur[i], cur[j], nvars)
                if m is not None and m != cur[i] and m != cur[j]:
                    cur = [c for k, c in enumerate(cur) if k not in (i, j)] + [m]
                    merged = True
                    break
            if merged:
                break
        if not merged:
            return cur
        changed = True


def reduce_anf(terms: list[tuple[int, int]], nvars: int) -> list[tuple[int, int]]:
    """Adjacent merging of an ANF cover (exact, structural, no enumeration).

    Two exact XOR identities, iterated to fixpoint:
    (a) same-care cubes differing in exactly one care bit merge to that
        bit dropped (disjoint minterm sets, so the union is the merge);
    (b) cross-care absorption: a cube strictly containing another's care
        by exactly one bit x, agreeing elsewhere, merges to the bigger
        care with bit x flipped (symmetric difference of nested cubes).
    Both strictly reduce total care bits or cube count (termination), and
    only ever shrink-or-tie the ANF (never worse, unlike from-scratch
    SOP search on XOR-dense functions). Validated in-test against direct
    evaluation on full domains.
    """
    if not isinstance(nvars, int) or isinstance(nvars, bool) or nvars < 1:
        raise ValueError(f"nvars must be a positive int, got {nvars!r}: refusing")
    for c in terms:
        if (not isinstance(c, tuple) or len(c) != 2 or
                not all(isinstance(x, int) and x >= 0 for x in c)):
            raise ValueError(f"bad cube {c!r}: refusing")
    cur = list(terms)
    while True:
        # cancel duplicates first (c XOR c = 0)
        seen: dict[tuple[int, int], int] = {}
        for c in cur:
            seen[c] = seen.get(c, 0) + 1
        nxt = [c for c, k in seen.items() if k % 2 == 1]
        if len(nxt) != len(cur):
            cur = nxt
            continue
        found = _find_merge(cur, nvars)
        if found is None:
            return cur
        x, y, z = found
        new_cur = [c for c in cur if c != x and c != y]
        if z in new_cur:
            new_cur.remove(z)
        else:
            new_cur.append(z)
        cur = new_cur


def _find_merge(cur: list[tuple[int, int]], nvars: int):
    """One applicable merge, or None. Same-care first, then absorption."""
    by_care: dict[int, list[tuple[int, int]]] = {}
    for c in cur:
        by_care.setdefault(c[1], []).append(c)
    for care, group in by_care.items():
        if len(group) < 2:
            continue
        for i in range(len(group)):
            for j in range(i + 1, len(group)):
                diff = (group[i][0] ^ group[j][0]) & care
                if diff and (diff & (diff - 1)) == 0:
                    bit = diff.bit_length() - 1
                    return (group[i], group[j],
                            (group[i][0] & ~(1 << bit), care & ~(1 << bit)))
    # cross-care absorption: small care C, big care C | {x}, agree on C
    small_by_care: dict[int, list[tuple[int, int]]] = {}
    for c in cur:
        small_by_care.setdefault(c[1], []).append(c)
    for care, group in small_by_care.items():
        for (vs, _) in group:
            for x in range(nvars):
                if (care >> x) & 1:
                    continue
                big_care = care | (1 << x)
                # big cube must agree with small on the shared care bits
                for big in by_care.get(big_care, []):
                    if (big[0] & care) == (vs & care):
                        flipped = (big[0] ^ (1 << x), big_care)
                        return ((vs, care), big, flipped)
    return None


def verify_cover(cubes: list[tuple[int, int]], table: dict[int, int], nvars: int) -> bool:
    """Cover decides the table on every minterm (exact check)."""
    _check_table(table, nvars)
    return all(_eval_cover(cubes, m) == table.get(m, 0) for m in range(1 << nvars))


def _cube_leaves(cube: tuple[int, int], nvars: int) -> frozenset[tuple[str, int, int]]:
    """Leaf refs of one (value, care) cube."""
    value, care = cube
    return frozenset(("lit", i, (value >> i) & 1)
                     for i in range(nvars) if (care >> i) & 1)


def factor_and_network(covers: list[list[tuple[int, int]]], nvars: int) -> dict:
    """Multi-level AND-sharing network for per-output XOR-covers (greedy CSE).

    SOP emission pays 2(w-1) Toffolis per width-w cube because every cube
    recomputes its AND-chain. This factors the joint cube multiset instead:
    repeatedly extract the literal/node pair with the largest Toffoli
    saving into a shared node computed once (2T: compute + mirror), until
    no pair occurs twice. Absorption (a term holding both X and Y with
    leaves(X) subset of leaves(Y) reduces to Y — AND-idempotence) and
    duplicate-term cancellation (t XOR t = 0) are applied as exact moves.
    Nodes may combine into deeper nodes (genuinely multi-level, not
    two-level SOP); node ids are creation-ordered, which is topological
    (a node only ever references older refs).

    Purely structural: no domain enumeration, so nvars carries no 10-var
    cap (unlike the table-based QM/cover functions); cube bits outside the
    variable space refuse instead of silently dropping. Cost model is
    structural: a k-ref term costs 2(k-1)T, a node 2T, so every move
    strictly decreases T (extraction saves 2(m-1) for m uses;
    absorption/dedup remove term cost with no new node) and the loop
    terminates; a defensive cap fails closed. Returns {"nodes": {nid:
    (left, right)}, "outputs": [[refs]], "nvars", "moves"} with leaves as
    ("lit", var, pol) and nodes as int ids. Every returned network is
    re-verified downstream by eval_network + the exhaustive circuit proof;
    determinism (sorted tie-breaks) is pinned in-test by double-run
    equality (set iteration order must never leak into the network).
    """
    if not isinstance(nvars, int) or isinstance(nvars, bool) or nvars < 1:
        raise ValueError(f"nvars must be a positive int, got {nvars!r}: refusing")
    if not isinstance(covers, list) or not covers:
        raise ValueError("covers must be a non-empty list of outputs: refusing")
    for out in covers:
        if not isinstance(out, list):
            raise ValueError(f"bad output cover {out!r}: refusing")
        for c in out:
            if (not isinstance(c, tuple) or len(c) != 2 or
                    not all(isinstance(x, int) and x >= 0 for x in c)):
                raise ValueError(f"bad cube {c!r}: refusing")
            if c[0] >= (1 << nvars) or c[1] >= (1 << nvars):
                raise ValueError(f"cube {c!r} exceeds {nvars} vars: refusing")

    ONE = ("one",)  # const-1 ref: the empty cube, AND-identity
    # terms: flat list of ref-sets; outputs reference term indices (XOR).
    terms: list[set] = []
    outputs: list[list[int]] = []
    for out in covers:
        idxs = []
        for c in out:
            leaves = set(_cube_leaves(c, nvars))
            if not leaves:
                leaves = {ONE}  # empty cube is const-1, the AND-identity
            terms.append(leaves)
            idxs.append(len(terms) - 1)
        outputs.append(idxs)
    # normalize: duplicate terms cancel within an output (t XOR t = 0).
    # (Empty cubes parse to the ONE ref above, so no term-set is empty.)
    for js in outputs:
        seen: dict[frozenset, int] = {}
        for t in js:
            seen[frozenset(terms[t])] = seen.get(frozenset(terms[t]), 0) + 1
        del js[:]
        for fs, k in seen.items():
            if k % 2 == 1:
                terms.append(set(fs))
                js.append(len(terms) - 1)

    leafsets: dict = {}  # ref -> frozenset of leaf refs beneath it

    def _leaves(ref) -> frozenset:
        if ref == ONE:
            return frozenset()
        if isinstance(ref, tuple):
            return frozenset({ref})
        return leafsets[ref]

    nodes: dict[int, tuple] = {}
    next_id = 0
    moves = 0
    cap = 100000
    while True:
        if moves > cap:
            raise ValueError("factorization exceeded move cap (implementation bug): refusing")
        # live terms: referenced by at least one output
        live = set()
        for js in outputs:
            live.update(js)
        # count co-occurring pairs over live terms (sorted for determinism)
        pair_locs: dict[tuple, list[int]] = {}
        for t in sorted(live):
            refs = sorted(terms[t], key=repr)
            for i in range(len(refs)):
                for j in range(i + 1, len(refs)):
                    pair_locs.setdefault((refs[i], refs[j]), []).append(t)
        best = None  # (saving, kind, payload)
        for pair in sorted(pair_locs, key=repr):
            locs = pair_locs[pair]
            a, b = pair
            la, lb = _leaves(a), _leaves(b)
            if la <= lb or lb <= la:
                # absorption: AND-idempotence drops the subsumed (smaller)
                # ref — X AND Y == Y when leaves(X) ⊆ leaves(Y). (The
                # const-1 ref has empty leaves, so it absorbs away here.)
                small = a if la <= lb else b
                saving = 2 * len(locs)
                cand = (saving, "absorb", (small, locs))
            elif len(locs) >= 2:
                saving = 2 * (len(locs) - 1)
                cand = (saving, "extract", (pair, locs))
            else:
                continue
            if best is None or cand[0] > best[0]:
                best = cand
        if best is None:
            break
        saving, kind, payload = best
        if kind == "extract":
            (a, b), locs = payload
            nid = next_id
            next_id += 1
            nodes[nid] = (a, b)
            leafsets[nid] = _leaves(a) | _leaves(b)
            for t in locs:
                terms[t].discard(a)
                terms[t].discard(b)
                terms[t].add(nid)
        else:
            small, locs = payload
            for t in locs:
                terms[t].discard(small)
        # re-normalize outputs (moves can create duplicate terms)
        for js in outputs:
            seen2: dict[frozenset, int] = {}
            for t in js:
                seen2[frozenset(terms[t])] = seen2.get(frozenset(terms[t]), 0) + 1
            del js[:]
            for fs, k in seen2.items():
                if k % 2 == 1:
                    terms.append(set(fs))
                    js.append(len(terms) - 1)
        moves += 1
    # Binarize: chain leftover multi-ref terms (no repeated pair remains, so
    # every such step is T-neutral: -2 in the term, +2 for the used-once
    # node). After this every term is a single ref and emission needs no
    # scratch temps. Pair choice is arbitrary-but-deterministic (sorted).
    for js in outputs:
        for t in js:
            while len(terms[t]) > 1:
                refs = sorted(terms[t], key=repr)
                a, b = refs[0], refs[1]
                nid = next_id
                next_id += 1
                nodes[nid] = (a, b)
                leafsets[nid] = _leaves(a) | _leaves(b)
                terms[t].discard(a)
                terms[t].discard(b)
                terms[t].add(nid)
                moves += 1
    # prune dead nodes (dedup can orphan a node; emitting it would waste 2T
    # and break the T == 2*live-nodes structural pin)
    used: set[int] = set()
    for js in outputs:
        for t in js:
            stack = [r for r in terms[t] if isinstance(r, int)]
            while stack:
                r = stack.pop()
                if r in used:
                    continue
                used.add(r)
                stack.extend(x for x in nodes[r] if isinstance(x, int))
    nodes = {nid: nodes[nid] for nid in sorted(used)}
    # outputs as single-ref terms (each set has exactly one ref now).
    out_refs = [[set(terms[t]) for t in js] for js in outputs]
    for js in out_refs:
        for s in js:
            if len(s) != 1:
                raise ValueError("term not binarized (implementation bug): refusing")
    return {"nodes": nodes, "outputs": out_refs, "nvars": nvars, "moves": moves}


def eval_network(net: dict, mask: int) -> list[int]:
    """Evaluate a factored network on one input mask (exact, for verification)."""
    nodes, outputs, nvars = net["nodes"], net["outputs"], net["nvars"]
    vals: dict = {}
    for nid in sorted(nodes):
        a, b = nodes[nid]
        vals[nid] = _ref_value(a, mask, vals) & _ref_value(b, mask, vals)
    out = []
    for js in outputs:
        bit = 0
        for term in js:
            prod = 1
            for r in term:
                prod &= _ref_value(r, mask, vals)
            bit ^= prod
        out.append(bit)
    return out


def _ref_value(ref, mask: int, vals: dict) -> int:
    if ref == ("one",):
        return 1
    if isinstance(ref, tuple):
        _, var, pol = ref
        return ((mask >> var) & 1) ^ (1 ^ pol)
    return vals[ref]


def emit_factored(b, n_ccx: list[int], net: dict, temps: list[int], outs: list[int]) -> None:
    """Emit a factored AND-network (shared-node discipline).

    Every internal node is computed once into its own temp in topological
    (creation) order, each output XOR-accumulates its terms, then nodes
    uncompute in reverse order. Negated leaf operands flip around their
    node's compute AND mirror (X is free); node operands read live temps.
    Structural cost: exactly 2T per live node (compute + mirror) — the
    test pins ccx == 2*nnodes. Temps are caller-managed (one per node);
    fewer temps than nodes refuses instead of silently overlapping.
    """
    nodes, outputs, nvars = net["nodes"], net["outputs"], net["nvars"]
    if len(temps) < len(nodes):
        raise ValueError(f"need {len(nodes)} temps, got {len(temps)}: refusing")
    if len(outs) != len(outputs):
        raise ValueError(f"need {len(outputs)} outs, got {len(outs)}: refusing")
    ids = sorted(nodes)
    tmap = {nid: temps[k] for k, nid in enumerate(ids)}

    def _operands(ref):
        if ref == ("one",):
            raise ValueError("const-1 ref inside an AND node (implementation bug): refusing")
        if isinstance(ref, tuple):
            _, var, pol = ref
            return (var, pol)
        return (tmap[ref], 1)

    for nid in ids:
        ra, rb = nodes[nid]
        qa, pa = _operands(ra)
        qb, pb = _operands(rb)
        if not pa:
            b.x(qa)
        if not pb:
            b.x(qb)
        b.ccx(qa, qb, tmap[nid])
        n_ccx[0] += 1
        if not pa:
            b.x(qa)
        if not pb:
            b.x(qb)
    for j, js in enumerate(outputs):
        for term in js:
            if len(term) != 1:
                # binarization guarantees single-ref terms; a multi-ref
                # term here means the network bypassed factor_and_network.
                raise ValueError(f"multi-ref term {term!r} survived: refusing")
            (r,) = tuple(term)
            if r == ("one",):
                b.x(outs[j])
            elif isinstance(r, tuple):
                _, var, pol = r
                if not pol:
                    b.x(var)
                b.cx(var, outs[j])
                if not pol:
                    b.x(var)
            else:
                b.cx(tmap[r], outs[j])
    for nid in reversed(ids):
        ra, rb = nodes[nid]
        qa, pa = _operands(ra)
        qb, pb = _operands(rb)
        if not pa:
            b.x(qa)
        if not pb:
            b.x(qb)
        b.ccx(qa, qb, tmap[nid])
        n_ccx[0] += 1
        if not pa:
            b.x(qa)
        if not pb:
            b.x(qb)


def greedy_multioutput_cover(tables: list[dict[int, int]], nvars: int) -> list[list[tuple[int, int]]]:
    """One shared AND-pool covering all outputs (greedy XOR cover, verified).

    The emitter accumulates by XOR, so coverage is parity state, not a
    monotone set: adding a cube TOGGLES its minterms (an early draft
    assumed OR-accumulation and emitted a cover that cancelled on
    overlaps — caught by verification). Each step picks the (cube,
    output) pair minimizing the remaining symmetric difference to the
    onset; a full-minterm cube always improves by exactly one, so the
    loop cannot stall. Returns per-output cube lists (XOR-sum). Every
    output cover is verified exact before return (a failed verification
    is an implementation bug).
    """
    for t in tables:
        _check_table(t, nvars)
    if not tables:
        raise ValueError("need at least one table: refusing")
    pis = [prime_implicants(t, nvars) for t in tables]
    onsets = [{m for m, v in t.items() if v == 1} for t in tables]
    # usable[cube] = outputs it may serve (covers only their onsets).
    # Singleton literals are always candidates too: parity structure
    # (e.g. a^b^c) has no usable multi-literal PIs, and the parity-state
    # metric + verification keep them exact wherever they help.
    usable: dict[tuple[int, int], set[int]] = {}
    for j, (t, pi) in enumerate(zip(tables, pis)):
        cands = set(pi)
        for i in range(nvars):
            cands.add((0, 1 << i))
            cands.add((1 << i, 1 << i))
        for c in cands:
            if all(m in onsets[j] for m in range(1 << nvars) if _covers(c, m)):
                usable.setdefault(c, set()).add(j)
    cur = [set() for _ in tables]
    chosen: dict[tuple[int, int], set[int]] = {}

    def dist(j: int) -> int:
        return len(cur[j] ^ onsets[j])

    while any(cur[j] != onsets[j] for j in range(len(tables))):
        best = None
        best_gain = 0
        for c, outs in usable.items():
            for j in outs:
                after = cur[j] ^ {m for m in range(1 << nvars) if _covers(c, m)}
                gain = len(cur[j] ^ onsets[j]) - len(after ^ onsets[j])
                if gain > best_gain:
                    best_gain = gain
                    best = (c, j)
        if best is None or best_gain <= 0:
            # stall-breaker: a full minterm cube fixes exactly one bit
            fixed = False
            for j in range(len(tables)):
                missing = onsets[j] - cur[j]
                extra = cur[j] - onsets[j]
                target = next(iter(missing | extra), None)
                if target is None:
                    continue
                full = (1 << nvars) - 1
                c = (target, full)
                cur[j] ^= {target}
                chosen.setdefault(c, set()).add(j)
                fixed = True
                break
            if not fixed:
                raise ValueError("greedy cover stalled (implementation bug): refusing")
            continue
        c, j = best
        cur[j] ^= {m for m in range(1 << nvars) if _covers(c, m)}
        chosen.setdefault(c, set()).add(j)
    out: list[list[tuple[int, int]]] = [[] for _ in tables]
    for c, outs in chosen.items():
        for j in outs:
            out[j].append(c)
    for j, t in enumerate(tables):
        out[j] = _reduce_cover(out[j], nvars)
        if not verify_cover(out[j], t, nvars):
            raise ValueError(f"output {j} cover fails verification (implementation bug): refusing")
    return out
