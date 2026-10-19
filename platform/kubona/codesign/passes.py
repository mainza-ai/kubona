"""Reusable co-design passes (production logic, not heuristics-as-truth).

- mbu_saving: exact Toffoli saving of measurement-based uncomputation for
  one temporary AND (2T compute+uncompute -> 1T + CZ correction).
  Paper anchors: MBU of a LOOKUP erases with 17T vs 126T reversing (SI
  III.2, construction-dependent — the AND formula above is the exact
  special case; lookup-scale MBU is queued compiler work).
- detect_routing_opportunity: structural detector for the paper's
  conditional-routing shape (2x controlled-K sharing one kernel).
  Returns a machine-checkable report, never a rewritten circuit — the
  rewrite stays in the search/evaluator loop where correctness is proven.
"""

from __future__ import annotations

from .kickmix import Builder, Op


def madd_costs(n: int) -> dict[str, int]:
    """Paper Fig-overview(b) schoolbook MADD costs (exact integer math).

    T_naive(n) = 3n^2 + 17n - 2 (two controlled multiply-adds);
    T_routed(n) = T_naive(n)/2 (swap y/z by b, one shared MADD, swap back).
    The numerator is always even (n(n+1) even), so halving is exact;
    a non-integer there would be an implementation bug, refused loudly.
    """
    if isinstance(n, bool) or not isinstance(n, int) or n < 1:
        raise ValueError(f"n must be positive int, got {n!r}: refusing")
    naive = 3 * n * n + 17 * n - 2
    if naive % 2 != 0:
        raise ValueError(f"T_naive({n})={naive} not even (implementation bug): refusing")
    return {"n": n, "naive_toffoli": naive, "routed_toffoli": naive // 2,
            "saved": naive - naive // 2}


def routed_madd_costs(n: int) -> dict[str, int]:
    """Back-compat alias for madd_costs (same dict, same refusal)."""
    return madd_costs(n)


def emit_cswap(b: Builder, ctrl: int, a: int, c: int) -> int:
    """Controlled-swap (Fredkin) in 1 Toffoli + 2 CNOTs (machine-verified).

    Sequence cx(a,c); ccx(ctrl,c,a); cx(a,c) provably swaps a<->c iff
    ctrl==1 (exhaustively checked over all 8 basis inputs; see test).
    Returns the Toffoli count (1). Qubit ids validated by Builder.
    """
    b.cx(a, c)
    b.ccx(ctrl, c, a)
    b.cx(a, c)
    return 1


def build_routed_1bit_madd() -> tuple[Builder, dict]:
    """1-bit conditional MADD via routing (Fig-overview(b) at toy scale).

    Registers: b=0, c=1, y=2, z=3, x=4. Map: x ^= c & (y if b==0 else z),
    y/z/c/b preserved. Circuit: cswap(y,z) by b; ccx(c,y,x); cswap back.
    Cost: 2*1T (swaps) + 1T (shared kernel) = 3T, 5Q, zero ancillas.
    Correctness is proven by the evaluator (16/16 inputs exhaustive).
    """
    bld = Builder(5)
    b, c, y, z, x = 0, 1, 2, 3, 4
    n = 0
    n += emit_cswap(bld, b, y, z)
    bld.ccx(c, y, x)
    n += 1
    n += emit_cswap(bld, b, y, z)
    assert n == 3
    regs = {"b": b, "c": c, "y": y, "z": z, "x": x, "ccx": n}
    return bld, regs


def routed_1bit_spec():
    """TaskSpec for the 1-bit routed MADD (inputs b,c,y,z; output x)."""
    from .kickmix import TaskSpec

    def _perm(bits: dict[int, int]) -> dict[int, int]:
        out = dict(bits)
        b = bits.get(0, 0) & 1
        c = bits.get(1, 0) & 1
        y = bits.get(2, 0) & 1
        z = bits.get(3, 0) & 1
        out[4] = bits.get(4, 0) ^ (c & (z if b else y))
        return out

    return TaskSpec(num_qubits=5, inputs=(0, 1, 2, 3), outputs=(4,),
                    target_perm=_perm)


def rotl_words(v: int, s: int, n: int) -> int:
    """Rotate-left of an n-bit value by s (exact integer model)."""
    if not isinstance(n, int) or n < 1:
        raise ValueError(f"n must be positive int, got {n!r}: refusing")
    if not isinstance(v, int) or not (0 <= v < (1 << n)):
        raise ValueError(f"v must fit in {n} bits, got {v!r}: refusing")
    s %= n
    return ((v << s) | (v >> (n - s))) & ((1 << n) - 1) if s else v


def build_barrel_rotl4() -> tuple[Builder, dict]:
    """Conditional cyclic shift (HHL shared-coordinate pattern at toy scale).

    X[4] input preserved; D[4] fresh output gets rotl(X, 2*b1 + b0) with
    b0 = qubit 4 (distance-1 stage), b1 = qubit 5 (distance-2 stage).
    Copy (4 CX) + rotl-0/2 stage (2 Fredkins) + rotl-0/1 stage (3 Fredkins
    forming the 4-cycle) = 5T. No subtraction, no rescaling, no coordinate
    register — the paper's replacement of coordinate arithmetic.
    """
    bld = Builder(10)
    x = [0, 1, 2, 3]
    b0, b1 = 4, 5
    d = [6, 7, 8, 9]
    n = 0
    for i in range(4):
        bld.cx(x[i], d[i])
    for a, c in ((0, 2), (1, 3)):  # rotl by 0 or 2
        n += emit_cswap(bld, b1, d[a], d[c])
    # 4-cycle in reverse wire order: (2,3),(1,2),(0,1) composes to a LEFT
    # rotation by 0/1 (forward order rotates right — caught live against
    # the rotl model, which follows the paper's left-shift convention).
    for a, c in ((2, 3), (1, 2), (0, 1)):  # then rotl by 0 or 1
        n += emit_cswap(bld, b0, d[a], d[c])
    assert n == 5
    return bld, {"x": tuple(x), "b": (b0, b1), "d": tuple(d), "ccx": n}


def barrel_rotl4_spec():
    """TaskSpec: fresh D gets rotl(X, 2*b1 + b0), everything else kept."""
    from .kickmix import TaskSpec

    def _perm(bits: dict[int, int]) -> dict[int, int]:
        out = dict(bits)
        xv = sum((bits.get(i, 0) & 1) << i for i in range(4))
        s = (bits.get(4, 0) & 1) + 2 * (bits.get(5, 0) & 1)
        rv = rotl_words(xv, s, 4)
        for i in range(4):
            out[6 + i] = (rv >> i) & 1
        return out

    return TaskSpec(num_qubits=10, inputs=(0, 1, 2, 3, 4, 5), outputs=(6, 7, 8, 9),
                    target_perm=_perm)


def mbu_saving(num_temp_ands: int) -> dict[str, int]:
    """Exact saving for N temporary ANDs cleared by MBU vs naive uncompute."""
    if not isinstance(num_temp_ands, int) or num_temp_ands < 0:
        raise ValueError(f"num_temp_ands must be non-negative int, got {num_temp_ands!r}")
    naive = 2 * num_temp_ands  # compute + uncompute Toffoli each
    with_mbu = 1 * num_temp_ands  # one Toffoli + Clifford CZ each
    return {"naive_toffoli": naive, "mbu_toffoli": with_mbu, "saved": naive - with_mbu}


def detect_routing_opportunity(ops: list[Op]) -> dict:
    """Detect adjacent controlled-arithmetic pairs sharing operand shape.

    Conservative (sound, not complete): reports a candidate when two ccx
    ops share the same target and their control sets differ in exactly one
    qubit — the minimal shape where swap-routing can share one kernel.
    No rewrite is performed here.
    """
    cands: list[dict] = []
    ccxs = [(i, o) for i, o in enumerate(ops) if o.name == "ccx"]
    for k in range(len(ccxs) - 1):
        (i, a), (j, b) = ccxs[k], ccxs[k + 1]
        if a.qubits[2] != b.qubits[2]:
            continue
        ca, cb = set(a.qubits[:2]), set(b.qubits[:2])
        if len(ca.symmetric_difference(cb)) == 2 and len(ca.intersection(cb)) == 1:
            cands.append({
                "op_indices": [i, j],
                "shared_target": a.qubits[2],
                "shared_control": sorted(ca.intersection(cb)),
                "differing": sorted(ca.symmetric_difference(cb)),
            })
    return {"candidates": cands, "count": len(cands)}
