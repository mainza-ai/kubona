"""In-place quadratic accumulation (the numerics kernel as a circuit).

HHL/Coulomb/Gaussian all evaluate an integer quadratic q(d) = g0 - d*t
with t = g1 - g2*d, consuming coefficients in place and accumulating
directly into the destination (no product registers, no completed value).
This module is that kernel as kickmix programs at toy scale (2-bit d):

- _add_plan / emit_add: in-place ripple add A += B (+ carry_in) with
  len(A) == len(B) + 1 (accumulator one wider: no overflow dropped, callers
  pin bounds by model). emit_sub via X-wrapped add (borrow-free
  precondition: callers pin a >= b over the swept domain — stated,
  model-checked, evaluator-proven per instance).
- emit_mul_rows: schoolbook multiply into zeroed OUT via AND-rows with
  per-row add; returns its op plan (every op self-inverse) so callers
  uncompute by executing it reversed. drop skips partial columns i+j <
  drop (the paper's truncated products; error bounded by trunc_mul_bound).
- build_horner_quad: full program for pinned coefficient sets: load g1,
  t <- g1 - g2*d (in place over g1), k += g0, k -= d*t, then uncompute
  (recompute prod1, add back, unload g1) so only d (preserved) and k
  (answer) survive; every temp freed (dirty-free raises otherwise).
- horner_quad_model: the same integer computation (truncation included);
  the circuit is bit-exact against it, and its error vs the exact
  (drop=0) model stays under the bound.

Widths are fixed (D=2, G1=5, G2=2, K=8); coefficient sets are pinned with
model-checked non-negativity (no silent borrow). Coefficients ride as
classical loads (X patterns, 0T) — the paper's offline-fitted table
values, not quantum data.
"""

from __future__ import annotations

from .kickmix import Builder, Op

D_BITS, G1_BITS, G2_BITS, K_BITS = 2, 5, 2, 8

Plan = list  # list[tuple[name, qubits]] with x/cx/ccx (all self-inverse)


def _chk(b: Builder, qs: list[int]) -> None:
    for q in qs:
        if q < 0 or q >= b.num_qubits:
            raise ValueError(f"qubit {q} out of range: refusing")


def _flip_all(b: Builder, qs: list[int]) -> None:
    """X every DISTINCT qubit once (operand lists share ZERO pads; flipping
    per-entry would flip shared qubits an even number of times = no flip,
    silently computing the wrong complement — caught live as 113/8)."""
    for q in dict.fromkeys(qs):
        b.x(q)


def _add_plan(A: list[int], B: list[int], carry: list[int], carry_in: int) -> Plan:
    """Op plan for A += B (+ carry_in).

    Carries are MAJ into temps, sums consume them in place (destroying the
    A controls), so uncomputing a carry first RESTORES its bit (reverse the
    sums), uncomputes the MAJ with pristine controls, then REDOES the sums:
    per bit high-to-low, reverse(sums_i) + reverse(compute_i) + sums_i nets
    zero on temps and keeps the result. Reversal alone would re-add
    (A += B twice is A + 2B with carry interaction); the first draft did
    exactly that and the evaluator caught dirty carries.
    """
    n = len(B)
    if len(A) != n + 1:
        raise ValueError(f"need len(A) == len(B)+1, got {len(A)},{n}: refusing")
    if len(carry) != n:
        raise ValueError(f"need {n} carry temps, got {len(carry)}: refusing")
    if carry_in not in (0, 1):
        raise ValueError(f"carry_in must be 0/1, got {carry_in!r}: refusing")
    compute: list[Plan] = []
    sums: list[Plan] = []
    c0: Plan = [("ccx", (A[0], B[0], carry[0]))]
    if carry_in:
        c0 += [("cx", (A[0], carry[0])), ("cx", (B[0], carry[0]))]
    compute.append(c0)
    s0: Plan = [("cx", (B[0], A[0]))]
    if carry_in:
        s0 += [("x", (A[0],))]
    sums.append(s0)
    for i in range(1, n):
        compute.append([("ccx", (A[i], B[i], carry[i])),
                        ("ccx", (A[i], carry[i - 1], carry[i])),
                        ("ccx", (B[i], carry[i - 1], carry[i]))])
        sums.append([("cx", (B[i], A[i])), ("cx", (carry[i - 1], A[i]))])
    sums.append([("cx", (carry[n - 1], A[n]))])
    plan: Plan = []
    for ops in compute:
        plan += ops
    for ops in sums:
        plan += ops
    for i in range(n - 1, -1, -1):
        plan += sums[i][::-1] + compute[i][::-1] + sums[i]
    return plan


def _exec(b: Builder, plan: Plan) -> int:
    n_ccx = 0
    for name, qs in plan:
        if name == "x":
            b.x(qs[0])
        elif name == "cx":
            b.cx(qs[0], qs[1])
        elif name == "ccx":
            b.ccx(qs[0], qs[1], qs[2])
            n_ccx += 1
        else:
            raise ValueError(f"plan op {name!r} outside kickmix: refusing")
    return n_ccx


def emit_add(b: Builder, A: list[int], B: list[int], carry: list[int],
             carry_in: int = 0) -> int:
    """In-place ripple add (compute + uncompute); returns Toffoli count."""
    _chk(b, list(A) + list(B) + list(carry))
    return _exec(b, _add_plan(A, B, carry, carry_in))


def emit_sub(b: Builder, A: list[int], B: list[int], carry: list[int]) -> int:
    """In-place subtract A -= B with len(A) == len(B) + 1.

    Two's complement in the full result width: ~B needs n+1 bits while B
    has n, so the top bit arrives as x(A_top) (adds 2^n), then X-wrapped
    add with carry_in 1, then unwrap B (A_top is NOT flipped back — it now
    holds result bit n). Borrow-free precondition (A >= B on every swept
    input) is the caller's — pinned by model check, proven per instance.
    The first draft omitted the top flip (implicitly widening ~B with 0)
    and the evaluator caught the wrong high bit.
    """
    _chk(b, list(A) + list(B) + list(carry))
    b.x(A[-1])
    _flip_all(b, B)
    n = _exec(b, _add_plan(A, B, carry, 1))
    _flip_all(b, B)
    return n


def trunc_mul_bound(na: int, nb: int, drop: int) -> int:
    """Exact dropped-part bound: sum of 2^(i+j) over skipped columns."""
    for name, v in (("na", na), ("nb", nb), ("drop", drop)):
        if isinstance(v, bool) or not isinstance(v, int) or v < 0:
            raise ValueError(f"{name} must be non-negative int: refusing")
    return sum(1 << (i + j) for i in range(na) for j in range(nb) if i + j < drop)


def _trunc_mul_val(a: int, na: int, b: int, nb: int, drop: int) -> int:
    out = 0
    for i in range(na):
        for j in range(nb):
            if i + j >= drop and ((a >> i) & 1) and ((b >> j) & 1):
                out += 1 << (i + j)
    return out


def emit_mul_rows(b: Builder, A: list[int], B: list[int], OUT: list[int],
                  ROW: list[int], CARR: list[int], ZERO: int, drop: int = 0) -> tuple[int, Plan]:
    """Schoolbook multiply into zeroed OUT (len na+nb); returns (T, unplan).

    Forward (executed here): per row j, ANDs, ADD, un-ANDs. unplan clears OUT
    by reversed rows of ANDs + SUBTRACT + un-ANDs — because addition carries,
    re-adding is NOT uncomputing (A += B twice is A + 2B); only subtraction
    reverses an addition (borrow-free here: rows subtract from a superset).
    Callers uncompute via _exec(b, unplan). drop skips ANDs with i+j < drop
    in both directions (truncated columns); skipped positions stay 0.
    """
    na, nb = len(A), len(B)
    if len(OUT) != na + nb:
        raise ValueError(f"need len(OUT) == {na + nb}, got {len(OUT)}: refusing")
    if len(ROW) != na or len(CARR) != na + nb - 1:
        raise ValueError("ROW needs lenA temps, CARR needs lenOUT-1: refusing")
    if not isinstance(drop, int) or drop < 0:
        raise ValueError(f"drop must be non-negative int: refusing {drop!r}")
    _chk(b, list(A) + list(B) + list(OUT) + list(ROW) + list(CARR) + [ZERO])
    rows: list[tuple[Plan, Plan, Plan]] = []
    for j in range(nb):
        and_ops: Plan = [("ccx", (A[i], B[j], ROW[i]))
                         for i in range(na) if i + j >= drop]
        operand = [ZERO] * j + list(ROW) + [ZERO] * (nb - 1 - j)
        add_ops = _add_plan(OUT, operand, CARR, 0)
        # wrap flips each DISTINCT qubit once (dict order = first position):
        # shared ZERO pads flipped per-entry would cancel to no flip.
        wrap = [("x", (q,)) for q in dict.fromkeys(operand)]
        # subtraction needs the top flip (extends ~row to full width):
        # OUT -= row via x(OUT_top) + wrapped add cin=1 (see emit_sub).
        sub_ops = [("x", (OUT[-1],))] + wrap + _add_plan(OUT, operand, CARR, 1) + wrap
        rows.append((and_ops, add_ops, sub_ops))
    n_ccx = 0
    for and_ops, add_ops, _ in rows:
        for op in and_ops:
            b.ccx(*op[1])
            n_ccx += 1
        n_ccx += _exec(b, add_ops)
        for op in reversed(and_ops):
            b.ccx(*op[1])
            n_ccx += 1
    unplan: Plan = []
    for and_ops, _, sub_ops in reversed(rows):
        unplan += and_ops + sub_ops + and_ops[::-1]
    return n_ccx, unplan


def horner_quad_model(d: int, g0: int, g1: int, g2: int, drop: int = 0) -> dict:
    """Exact integer model (truncation included): t = g1 - g2*d, k = g0 - d*t."""
    for name, v in (("d", d), ("g0", g0), ("g1", g1), ("g2", g2)):
        if not isinstance(v, int) or v < 0:
            raise ValueError(f"{name} must be non-negative int: refusing")
    p1 = _trunc_mul_val(g2, G2_BITS, d, D_BITS, drop)
    t = g1 - p1
    if t < 0:
        raise ValueError(f"inner underflow t={t} (coefficients unfit): refusing")
    p2 = _trunc_mul_val(d, D_BITS, t, G1_BITS + 1, drop)
    k = g0 - p2
    if k < 0:
        raise ValueError(f"outer underflow k={k} (coefficients unfit): refusing")
    return {"t": t, "k": k, "p1": p1, "p2": p2}


def _load_const(b: Builder, reg: list[int], val: int) -> None:
    for i, q in enumerate(reg):
        if (val >> i) & 1:
            b.x(q)


def build_horner_quad(g0: int, g1: int, g2: int, drop: int = 0) -> tuple[Builder, dict]:
    """Full in-place quadratic program for a pinned coefficient set.

    Validates the set by model over all d (non-negativity pinned, never
    assumed). Layout: d[2] input; g1r[5] loaded, overwritten by t,
    restored; k[8] accumulates; G2R[2] loaded coefficient; P1[4], ROW[5],
    P2[7], C7[7], G0T[7], ZERO — alloc'd temps, all freed. Coefficients
    ride as classical X-loads (0T); zero-valued ANDs still emit (uniform
    code path — counts reported honestly, no folded savings claimed).
    """
    for name, v, bits in (("g0", g0, 7), ("g1", g1, 5), ("g2", g2, 2)):
        if isinstance(v, bool) or not isinstance(v, int) or not (0 <= v < (1 << bits)):
            raise ValueError(f"{name} must fit in {bits} bits: refusing {v!r}")
    if not isinstance(drop, int) or drop < 0:
        raise ValueError(f"drop must be non-negative int: refusing {drop!r}")
    for d in range(1 << D_BITS):  # pin the borrow-free precondition by model
        horner_quad_model(d, g0, g1, g2, drop)
    d = [0, 1]
    g1r = [2, 3, 4, 5, 6]
    k = list(range(7, 15))
    P1 = list(range(15, 19))
    ROW = list(range(19, 24))
    P2 = list(range(24, 31))
    C7 = list(range(31, 38))
    G0T = list(range(38, 45))
    G2R = [45, 46]
    ZERO = 47
    temps = P1 + ROW + P2 + C7 + G0T + G2R + [ZERO]
    b = Builder(48)
    for q in temps:
        b.alloc(q)
    n_ccx = 0
    _load_const(b, g1r, g1)
    _load_const(b, G2R, g2)
    t1, un1 = emit_mul_rows(b, G2R, d, P1, ROW[:2], C7[:3], ZERO, drop)
    n_ccx += t1
    n_ccx += emit_sub(b, g1r, P1, C7[:4])
    n_ccx += _exec(b, un1)  # clear P1 (subtract rows back)
    t2, un2 = emit_mul_rows(b, d, g1r, P2, ROW[:2], C7[:6], ZERO, drop)
    n_ccx += t2
    _load_const(b, G0T, g0)
    n_ccx += emit_add(b, k, G0T, C7)
    _load_const(b, G0T, g0)  # unload (X is self-inverse)
    n_ccx += emit_sub(b, k, P2, C7)
    n_ccx += _exec(b, un2)  # clear P2
    t1b, un1b = emit_mul_rows(b, G2R, d, P1, ROW[:2], C7[:3], ZERO, drop)
    n_ccx += t1b  # recompute P1 to restore g1r
    n_ccx += emit_add(b, g1r, P1, C7[:4])
    n_ccx += _exec(b, un1b)  # clear P1 again
    _load_const(b, g1r, g1)  # unload
    _load_const(b, G2R, g2)  # unload
    for q in temps:
        b.free(q)
    return b, {"d": tuple(d), "k": tuple(k), "g1r": tuple(g1r),
               "coefs": (g0, g1, g2), "drop": drop, "ccx": n_ccx}


def horner_quad_spec(g0: int, g1: int, g2: int, drop: int = 0):
    """TaskSpec: zeroed k gets the modeled quadratic at input d."""
    from .kickmix import TaskSpec

    def _perm(bits: dict[int, int]) -> dict[int, int]:
        out = dict(bits)
        dv = (bits.get(0, 0) & 1) | ((bits.get(1, 0) & 1) << 1)
        kv = horner_quad_model(dv, g0, g1, g2, drop)["k"]
        for i in range(8):
            out[7 + i] = (kv >> i) & 1
        return out

    anc = (2, 3, 4, 5, 6) + tuple(range(15, 48))
    return TaskSpec(num_qubits=48, inputs=(0, 1), outputs=tuple(range(7, 15)),
                    ancillas=anc, target_perm=_perm)
