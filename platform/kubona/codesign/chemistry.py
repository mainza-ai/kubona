"""Alias-table scheduler + frame-Select emitters (SI IV, exact toy scale).

Scheduling: the paper's preprocessing pairs underfilled rows with heavy
terms to make NEARBY rows identical (preferring A,A,B,B over A,B,A,B),
then prunes the QROM address tree over repeated subtrees. alias_schedule()
is that scheduler for toy tables (n <= 8, factorial search); the exact
optimum under (distinct_subtrees cost, -adjacent equal pairs), deterministic
ties, cost <= naive asserted. schedule_alias_words() scales it (exact <= 8,
runs-greedy above with live non-regression gates).

Select: the paper applies a block representative once plus controlled
frames instead of traversing every Pauli (frame-block Select). The emitters
below prove that pattern as kickmix circuits over CLASSICAL frames only
(perms, CNOT-conjugation — both basis-preserving, hence evaluator-checkable):
- build_swap_select(): 2-block {X0, X1} via controlled swaps around one X.
- build_cnot_select(): 2-block {X0, X0X1} via controlled-CNOT conjugation
  (ccx, X, ccx — the identity CNOT.X0.CNOT = X0X1, checked live per input).
Hadamard-like frames leave the kickmix model (superposition-creating) and
stay outside evaluator scope — the paper's own task-boundary exclusion.
"""

from __future__ import annotations

import itertools

from .qrom import distinct_subtrees


def _adjacencies(words: list) -> int:
    return sum(1 for i in range(len(words) - 1) if words[i] == words[i + 1])


def _is_perm(a: list, b: list) -> bool:
    from collections import Counter
    return Counter(a) == Counter(b)


def alias_schedule(words: list) -> dict:
    """Best row order for a toy alias-word multiset (exact, deterministic)."""
    if not isinstance(words, list) or not words:
        raise ValueError(f"words must be a non-empty list: refusing {words!r}")
    if len(words) > 8 or len(words) & (len(words) - 1):
        raise ValueError(f"toy schedule needs 2^k words with k<=3, got {len(words)}: refusing")
    naive = distinct_subtrees(list(words))
    best = None
    for perm in sorted(set(itertools.permutations(range(len(words))))):
        cand = [words[i] for i in perm]
        cost = distinct_subtrees(cand)
        key = (cost, -_adjacencies(cand), cand)
        if best is None or key < best[0]:
            best = (key, perm, cand)
    assert best is not None
    (cost, _neg_adj, ordered) = best[0]
    if cost > naive:
        raise ValueError("schedule worsened the pruned cost (implementation bug): refusing")
    return {"perm": list(best[1]), "ordered": list(ordered), "cost": cost,
            "adjacencies": _adjacencies(ordered), "naive_cost": naive}


def build_swap_select() -> tuple[object, dict]:
    """Frame-Select over the swap 2-block {X0, X1} (2T, proven exhaustive).

    Address a=0, system s0=1, s1=2: cswap(s0,s1) by a, X(s0), cswap back.
    a=0 applies the representative X0; a=1 routes through the frame to X1.
    Swaps are Clifford (0T); the two Fredkins cost 1T each (emit_cswap).
    """
    from .kickmix import Builder
    from .passes import emit_cswap
    from .kickmix import Builder
    from .passes import emit_cswap
    b = Builder(3)
    n = emit_cswap(b, 0, 1, 2)
    b.x(1)
    n += emit_cswap(b, 0, 1, 2)
    assert n == 2
    return b, {"addr": 0, "sys": (1, 2), "ccx": n}


def swap_select_spec():
    """TaskSpec: sys ^= X0 if a==0 else X1 (address preserved)."""
    from .kickmix import TaskSpec

    def _perm(bits: dict[int, int]) -> dict[int, int]:
        out = dict(bits)
        if bits.get(0, 0) == 0:
            out[1] = bits.get(1, 0) ^ 1
        else:
            out[2] = bits.get(2, 0) ^ 1
        return out

    return TaskSpec(num_qubits=3, inputs=(0,), outputs=(1, 2), target_perm=_perm)


def build_cnot_select() -> tuple[object, dict]:
    """Frame-Select over the CNOT 2-block {X0, X0X1} (2T, proven exhaustive).

    Controlled conjugation: ccx(a,s0,s1), X(s0), ccx(a,s0,s1). At a=0 the
    frame is off and the representative X0 acts; at a=1 the pair conjugates
    it to X0X1 (CNOT.X0.CNOT = X0X1 — rechecked by the evaluator per input,
    never trusted from the textbook).
    """
    from .kickmix import Builder
    b = Builder(3)
    b.ccx(0, 1, 2)
    b.x(1)
    b.ccx(0, 1, 2)
    return b, {"addr": 0, "sys": (1, 2), "ccx": 2}


def cnot_select_spec():
    """TaskSpec: sys ^= X0 if a==0 else X0X1 (address preserved)."""
    from .kickmix import TaskSpec

    def _perm(bits: dict[int, int]) -> dict[int, int]:
        out = dict(bits)
        out[1] = bits.get(1, 0) ^ 1
        if bits.get(0, 0) == 1:
            out[2] = bits.get(2, 0) ^ 1
        return out

    return TaskSpec(num_qubits=3, inputs=(0,), outputs=(1, 2), target_perm=_perm)


def schedule_alias_words(words: list) -> dict:
    """Alias-row scheduling at any scale (exact where measurable, else runs).

    The 41k-row Fe2S2 table is far past factorial search (and its data is not
    in-repo — synthetic multisets validate the scheduler, never the paper's
    table). Method: 2^k rows with k<=3 -> exact alias_schedule (optimum);
    otherwise best-of deterministic candidates {identity, repr-sorted,
    repr-sorted rotations} under (pruned cost, -runs) when 2^k, else
    (-runs, lexicographic). Safety is proven, not hoped: the identity is
    always a candidate, so cost never exceeds naive and runs never shrink
    (both asserted live); the output is verified a permutation of the input.
    Unmeasurable costs report None, never a fitted number.
    """
    if not isinstance(words, list) or not words:
        raise ValueError(f"words must be a non-empty list: refusing {words!r}")
    try:
        set(words)
    except TypeError:
        raise ValueError("words must be hashable records: refusing")
    n = len(words)
    if n <= 8 and n & (n - 1) == 0:
        rep = alias_schedule(words)
        return {"ordered": rep["ordered"], "method": "exact", "cost": rep["cost"],
                "naive_cost": rep["naive_cost"], "adjacencies": rep["adjacencies"],
                "naive_adjacencies": _adjacencies(words)}
    if n > 1_000_000:
        raise ValueError(f"table of {n} rows exceeds scale cap 1e6: refusing")
    keyed = sorted(range(n), key=lambda i: repr(words[i]))
    cands = [list(range(n)), keyed]
    step = max(1, n // 4)
    for s in (step, 2 * step, 3 * step):
        cands.append(keyed[s:] + keyed[:s])
    measurable = n & (n - 1) == 0
    naive_adj = _adjacencies(words)
    naive_cost = distinct_subtrees(list(words)) if measurable else None
    best = None
    for perm in cands:
        cand = [words[i] for i in perm]
        cost = distinct_subtrees(cand) if measurable else None
        score = ((cost, -_adjacencies(cand), [repr(w) for w in cand])
                 if measurable else (-_adjacencies(cand), [repr(w) for w in cand]))
        if best is None or score < best[0]:
            best = (score, perm, cand, cost)
    assert best is not None
    _, perm, ordered, cost = best
    if not _is_perm(ordered, words):
        raise ValueError("scheduler output is not a permutation (implementation bug): refusing")
    adj = _adjacencies(ordered)
    if adj < naive_adj:
        raise ValueError("scheduler shrank runs (implementation bug): refusing")
    if measurable and cost is not None and naive_cost is not None and cost > naive_cost:
        raise ValueError("scheduler worsened the pruned cost (implementation bug): refusing")
    return {"ordered": ordered, "method": "greedy", "cost": cost,
            "naive_cost": naive_cost, "adjacencies": adj,
            "naive_adjacencies": naive_adj}


#: Pinned alias row (threshold/alt pair): keep=2, alt=1 (defaults; the
#: general-keep path below covers keep in 0..3 with per-keep borrow-ANF
#: corrections — the pinned default is one instance, not a scope limit).
ALIAS_KEEP, ALIAS_ALT = 2, 1


def _h_erase_plan(keep: int, h: int, r: int, t0: int, t1: int) -> list[tuple]:
    """MBU-erase ops (after the hmr) for h = [tau < keep], 2-bit tau.

    h's ANF per keep (tau = (t1 t0)): keep=0: h = 0 (bare hmr); keep=1:
    h = ~t1 & ~t0 = 1^t1^t0^(t1 t0); keep=2: h = ~t1; keep=3: h = ~(t1 t0)
    = 1^(t1 t0). Each (-1)^{r f} cancels exactly (global flips for the
    constant 1, z for singletons, cz for the pair). Other keeps refuse.
    """
    if keep == 0:
        return []
    if keep == 1:
        return [("flip", (r,)), ("z", (t0, r)), ("z", (t1, r)),
                ("cz", (t0, t1, r))]
    if keep == 2:
        return [("flip", (r,)), ("z", (t1, r))]
    if keep == 3:
        return [("flip", (r,)), ("cz", (t0, t1, r))]
    raise ValueError(f"keep must be in [0,3] (2-bit scope), got {keep!r}: refusing")


def build_alias_step(keep: int = ALIAS_KEEP, alt: int = ALIAS_ALT) -> tuple[object, dict]:
    """Compare-select-erase pipeline (Fe2S2 alias dataflow at toy scale).

    Inputs tau[2], idx[1]; classical keep/alt (default 2/1); output sel[1]:
    h = [tau < keep] via borrow-out of tau-keep (modular-exact, borrows are
    the signal here, not a violation); sel = idx if h else alt via one
    Fredkin-free conditional (sel ^= idx, then sel ^= !h & (idx^alt));
    keep erased by MBU (hmr + conditioned global flips — 0T, reversal
    avoided as in the paper); h erased by MBU with its keep-specific
    borrow-ANF correction (_h_erase_plan, exact). Select-use (Pauli
    application) is the chemistry Select emitters' proven half;
    composition is pack work (stated). 8/8 exhaustive per (keep, alt).
    """
    from .horner import emit_add, emit_sub
    from .kickmix import Builder
    if isinstance(keep, bool) or not isinstance(keep, int) or not (0 <= keep < 4):
        raise ValueError(f"keep must be in [0,3], got {keep!r}: refusing")
    if alt not in (0, 1):
        raise ValueError(f"alt must be 0/1, got {alt!r}: refusing")
    tau, idx, sel = [0, 1], [2], [3]
    SUBT, C2 = [4, 5, 6], [7, 8]
    K2, T, h = [9, 10], 11, 12
    b = Builder(13)
    for q in SUBT + C2 + K2 + [T, h]:
        b.alloc(q)
    n_ccx = 0
    for i, q in enumerate(K2):  # load keep bits
        if (keep >> i) & 1:
            b.x(q)
    for i in range(2):  # SUBT ^= tau (minuend; controls only, preserved)
        b.cx(tau[i], SUBT[i])
    n_ccx += emit_sub(b, SUBT, K2, C2)  # SUBT = tau - keep (borrow = h)
    b.cx(SUBT[2], h)
    n_ccx += emit_add(b, SUBT, K2, C2)  # uncompute (add back -> SUBT = tau)
    for i in range(2):  # unload tau (controls only, preserved)
        b.cx(tau[i], SUBT[i])
    for i, q in enumerate(K2):  # erase keep DIRECTLY (no unload first)
        r = b.hmr(q)
        if (keep >> i) & 1:
            # hmr consumes the loaded qubit; unloading first would zero it
            # and the correction would fire on f=0 (caught live as a phase
            # mismatch on every r1=1 trajectory).
            b.global_flip(cond=(r,))
    b.cx(idx[0], sel[0])  # sel ^= idx
    b.cx(idx[0], T)  # t = idx ^ alt (alt classical)
    if alt:
        b.x(T)
    b.x(h)
    b.ccx(h, T, sel[0])  # sel ^= !h & t
    n_ccx += 1
    b.x(h)
    b.cx(idx[0], T)  # uncompute t
    if alt:
        b.x(T)
    r2 = b.hmr(h)  # MBU-erase h with its keep-specific ANF correction
    for op in _h_erase_plan(keep, h, r2, 0, 1):
        if op[0] == "flip":
            b.global_flip(cond=(op[1][0],))
        elif op[0] == "z":
            b.z(op[1][0], cond=(op[1][1],))
        elif op[0] == "cz":
            b.cz(op[1][0], op[1][1], cond=(op[1][2],))
        else:
            raise ValueError(f"erase plan op {op[0]!r} (implementation bug): refusing")
    for q in SUBT + C2 + [T]:
        b.free(q)
    return b, {"tau": tuple(tau), "idx": tuple(idx), "sel": tuple(sel),
               "ccx": n_ccx, "keep": keep, "alt": alt}


def alias_step_spec(keep: int = ALIAS_KEEP, alt: int = ALIAS_ALT):
    """TaskSpec: sel = idx if tau < keep else alt (inputs kept, phase zero).

    The zero-phase target is load-bearing: every MBU erase leaves a
    record-dependent phase that its correction must cancel exactly. A
    dropped correction fails here (it cannot hide in an unchecked phase).
    """
    from .kickmix import TaskSpec
    if isinstance(keep, bool) or not isinstance(keep, int) or not (0 <= keep < 4):
        raise ValueError(f"keep must be in [0,3], got {keep!r}: refusing")
    if alt not in (0, 1):
        raise ValueError(f"alt must be 0/1, got {alt!r}: refusing")

    def _perm(bits: dict[int, int]) -> dict[int, int]:
        out = dict(bits)
        tau = (bits.get(0, 0) & 1) | ((bits.get(1, 0) & 1) << 1)
        idx = bits.get(2, 0) & 1
        out[3] = (idx if tau < keep else alt) & 1
        return out

    return TaskSpec(num_qubits=13, inputs=(0, 1, 2), outputs=(3,),
                    ancillas=(4, 5, 6, 7, 8, 11), target_perm=_perm,
                    target_phase=lambda bits: 0)
