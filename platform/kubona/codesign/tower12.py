"""Tower-field F2^12 inverter as kickmix circuits (OUR construction).

Scope (stated): the SI's only tower division is Hermitian F64-with-F8
arithmetic (main.tex:2146) — the Reed-Solomon F2^12 thread gives no tower,
so this tower is OURS, not the paper's: F2^12/F64/F8 with F8 = F2[z]/(z^3+z+1),
F64 = F8[y]/(y^2+y+1), F2^12 = F64[w]/(w^2+w+MU1). Machine-found inside the
mod-0x1053 ints (Z=937 order-7 with z^3+z+1=0; Y=70 with y^2+y+1=0;
MU1=70 valid: Y is outside the Artin-Schreier image on F64; W=1970 with
w^2+w+MU1=0), basis-change matrix inverted over F2 with round-trip gated on
all 4096. The norm chain N = A(A+B)+MU1*B^2 (level 1), M = N*Nbar-const
(level 2), M^-1 = M^6 (F8 Fermat) is proved against F2_12.inv on all 4095
nonzero inputs (classical, in-test) before any circuit is built.

Circuit: poly-bit -> tower-coord change (CNOTs) -> T1/N/NI ladders (CNOTs)
-> nineteen F8 multiply-accumulates (norm, two scale-backs, F8 Fermat link,
two NI links; each 9 ANDs, plain uncompute) -> out accumulate (CNOTs) ->
mirror uncompute (mults cleared by re-accumulation, CNOT ladders reversed).
Squaring is total (Frobenius has no 0 edge case), so x = 0 flows through to
h = 0 with no special-casing. Proven exhaustive 4096/4096 with temps clean
and phase clean (slice 55); drop-in cross-checked against emit_fermat_inv12
on all 4096.

Cost model: 38 F8-mult equivalents (19 forward + 19 clear) at 18T each.
Measured in-test (pinned, not transcribed).
"""

from __future__ import annotations

from .gf import F2_12
from .kickmix import Builder

#: Tower parameters (machine-found, verified in-test): Z generates the F8
#: subfield (order 7, z^3+z+1=0); Y extends to F64 (y^2+y+1=0); MU1 = Y is a
#: valid level-1 constant (outside the Artin-Schreier image on F64, so
#: w^2+w+MU1 stays irreducible); W extends to F2^12.
Z_ROOT = 937
Y_ROOT = 70
MU1 = 70
W_ROOT = 1970

#: Tower basis in e-major order: e_i * Y^j * W^k for e = (1, Z, Z^2).
#: Coord bits t[0:3] = a0, t[3:6] = a1 (A = a0+a1*Y in F8-abstract coords),
#: t[6:9] = b0, t[9:12] = b1 (B = b0+b1*Y). Both halves share the F64 basis.
def _tower_basis() -> list[int]:
    e = (1, Z_ROOT, F2_12.mul(Z_ROOT, Z_ROOT))
    vecs = []
    for ww in (1, W_ROOT):
        for yy in (1, Y_ROOT):
            for ee in e:
                vecs.append(F2_12.mul(F2_12.mul(ee, yy), ww))
    return vecs


def _mat_inv(mat: list[list[int]]) -> list[list[int]]:
    """Invert a binary matrix (fail-closed on singular)."""
    n = len(mat)
    aug = [row[:] + [1 if i == j else 0 for j in range(n)] for i, row in enumerate(mat)]
    for col in range(n):
        piv = next((i for i in range(col, n) if aug[i][col]), None)
        if piv is None:
            raise ValueError("basis matrix singular (implementation bug): refusing")
        aug[col], aug[piv] = aug[piv], aug[col]
        for i in range(n):
            if i != col and aug[i][col]:
                aug[i] = [(a ^ b) & 1 for a, b in zip(aug[i], aug[col])]
    return [row[n:] for row in aug]


def tower12_change_matrices() -> tuple[list[list[int]], list[list[int]]]:
    """Basis-change matrices (rows) between poly coords and tower coords.

    Returns (to_tower_rows, from_tower_rows). Product is the identity both
    ways (asserted live — a mistranscription here silently scrambles every
    basis change).
    """
    cols = _tower_basis()  # cols[k][i] = bit i of tower basis vector k
    m_rows = [[(cols[k] >> i) & 1 for k in range(12)] for i in range(12)]
    fwd = _mat_inv(m_rows)  # poly coords -> tower coords
    inv = m_rows  # tower coords -> poly coords
    for a, bb in ((fwd, inv), (inv, fwd)):
        for i in range(12):
            for j in range(12):
                if (sum(a[i][k] & bb[k][j] for k in range(12)) & 1) != (1 if i == j else 0):
                    raise ValueError("basis matrices not inverse (implementation bug): refusing")
    return (fwd, inv)


def to_tower12(x: int) -> tuple[int, int]:
    """Poly-basis element -> tower F64 halves (A, B) with x = A + B*w.

    A, B are F64 elements as poly ints (the embedding is set-identity):
    A is the XOR of w-free basis vectors selected by coord bits t[0:6],
    B the XOR of the same six F64 basis vectors selected by bits t[6:12]
    (basis[k+6] = basis[k]*w, so x = A + B*w by construction).
    """
    if not isinstance(x, int) or not (0 <= x < 4096):
        raise ValueError(f"element must be in [0,4096), got {x!r}: refusing")
    fwd, _ = tower12_change_matrices()
    bits = [(x >> i) & 1 for i in range(12)]
    t = [sum(row[j] & bits[j] for j in range(12)) & 1 for row in fwd]
    basis = _tower_basis()
    a = 0
    b = 0
    for i in range(6):
        if t[i]:
            a ^= basis[i]
        if t[6 + i]:
            b ^= basis[i]
    if F2_12.add(a, F2_12.mul(b, W_ROOT)) != x:
        raise ValueError(f"to_tower12 round-trip failed at {x} (implementation bug): refusing")
    return (a, b)


def from_tower12(a: int, b: int) -> int:
    """Tower F64 halves -> poly-basis element (A + B*w evaluated in F2^12)."""
    for v, name in ((a, "A"), (b, "B")):
        if not isinstance(v, int) or not (0 <= v < 4096):
            raise ValueError(f"{name} must be in [0,4096), got {v!r}: refusing")
    return F2_12.add(a, F2_12.mul(b, W_ROOT))


def _split64(n: int) -> tuple[int, int]:
    """F64 element -> F8 pair (n0, n1) with n = n0 + n1*Y (classical check use)."""
    f8 = _f8_elements()
    for n0 in f8:
        for n1 in f8:
            if F2_12.add(n0, F2_12.mul(n1, Y_ROOT)) == n:
                return (n0, n1)
    raise ValueError(f"not in F64: {n}: refusing")


def _f8_elements() -> set[int]:
    """The F8 subfield as a set of poly ints."""
    out = {0}
    p = 1
    for _ in range(7):
        p = F2_12.mul(p, Z_ROOT)
        out.add(p)
    return out


def _f8_pow(a: int, e: int) -> int:
    r = 1
    while e:
        if e & 1:
            r = F2_12.mul(r, a)
        a = F2_12.sqr(a)
        e >>= 1
    return r


def tower_inv12_classical(x: int) -> int:
    """Classical tower inversion (the norm chain the circuit implements)."""
    if not isinstance(x, int) or not (0 <= x < 4096):
        raise ValueError(f"element must be in [0,4096), got {x!r}: refusing")
    if x == 0:
        return 0
    a, b = to_tower12(x)
    ac = F2_12.add(a, b)  # conjugate constant part (w -> w+1)
    n = F2_12.add(F2_12.mul(a, ac), F2_12.mul(F2_12.mul(b, b), MU1))
    n0, n1 = _split64(n)
    s = n0 ^ n1
    m = F2_12.add(F2_12.mul(n0, s), F2_12.mul(n1, n1))  # N*Nbar const part
    mi = 0 if m == 0 else _f8_pow(m, 6)
    ni = F2_12.add(F2_12.mul(s, mi), F2_12.mul(F2_12.mul(n1, mi), Y_ROOT))
    if F2_12.mul(n, ni) != 1:
        raise ValueError(f"F64 inversion failed at {x} (implementation bug): refusing")
    c = F2_12.mul(ac, ni)
    d = F2_12.mul(b, ni)
    out = from_tower12(c, d)
    if F2_12.mul(x, out) != 1:
        raise ValueError(f"tower inversion failed at {x} (implementation bug): refusing")
    return out


#: F8 multiplication pattern, DERIVED from z^3+z+1 at import (not
#: transcribed): F8_MUL_TERMS[i][j] = output-bit mask of c_i*d_j.
def _f8_mul_terms() -> list[list[int]]:
    mod = 0b1011  # z^3+z+1
    terms = [[0] * 3 for _ in range(3)]
    for i in range(3):
        for j in range(3):
            p = 1 << (i + j)
            while p.bit_length() > 3:
                p ^= mod << (p.bit_length() - 4)
            terms[i][j] = p
    return terms


F8_MUL_TERMS = _f8_mul_terms()

#: F8 squaring is linear: sqr(c0+c1*Z+c2*Z^2) = c0 + c2*Z + (c1+c2)*Z^2
#: (Z^3 = Z+1, Z^4 = Z^2+Z). Verified against int arithmetic for all 8 below.
F8_SQR_ROWS = ((1, 0, 0), (0, 0, 1), (0, 1, 1))


def _check_f8_tables() -> None:
    """Prove the derived F8 tables against int arithmetic (runs at import)."""
    for c in range(8):
        bits = [(c >> i) & 1 for i in range(3)]
        cv = bits[0] ^ F2_12.mul(bits[1], Z_ROOT) ^ F2_12.mul(bits[2], F2_12.mul(Z_ROOT, Z_ROOT))
        sq = F2_12.sqr(cv)
        got = [sum(F8_SQR_ROWS[k][i] & bits[i] for i in range(3)) & 1 for k in range(3)]
        gv = got[0] ^ F2_12.mul(got[1], Z_ROOT) ^ F2_12.mul(got[2], F2_12.mul(Z_ROOT, Z_ROOT))
        if gv != sq:
            raise ValueError(f"F8 square table wrong at {c}: refusing")
    f8 = _f8_elements()
    for a in f8:
        for bb in f8:
            ab = F2_12.mul(a, bb)
            ai = [0, 0, 0]
            bi = [0, 0, 0]
            # abstract coords: solve c0 + c1*Z + c2*Z^2 = v by table lookup
            for c in range(8):
                bits = [(c >> i) & 1 for i in range(3)]
                v = bits[0] ^ F2_12.mul(bits[1], Z_ROOT) ^ F2_12.mul(bits[2], F2_12.sqr(Z_ROOT))
                if v == a:
                    ai = bits
                if v == bb:
                    bi = bits
            got = [0, 0, 0]
            for i in range(3):
                for j in range(3):
                    if ai[i] & bi[j]:
                        m = F8_MUL_TERMS[i][j]
                        for k in range(3):
                            got[k] ^= (m >> k) & 1
            gv = got[0] ^ F2_12.mul(got[1], Z_ROOT) ^ F2_12.mul(got[2], F2_12.sqr(Z_ROOT))
            if gv != ab:
                raise ValueError(f"F8 mult table wrong at {(a, bb)}: refusing")


_check_f8_tables()


def _check_word(w: list[int], n: int, name: str, nqubits: int) -> None:
    if not isinstance(w, list) or len(w) != n or any(not isinstance(q, int) or q < 0 or q >= nqubits for q in w):
        raise ValueError(f"{name} must be {n} in-range qubit ids: refusing {w!r}")


def emit_f8_mul(b: Builder, a: list[int], c: list[int], out: list[int], pool: list[int],
                seg: list | None = None) -> int:
    """F8 multiply-accumulate (abstract 3-bit coords, z^3+z+1).

    One AND per (i,j) pair (9 total) with immediate uncompute (2T each);
    AND fanned to each output bit in F8_MUL_TERMS[i][j]. a, c preserved
    (controls/XORs only); out accumulates (caller clears by
    re-accumulation). Returns 18. If seg is given, the op is recorded for
    mirror replay (re-emission clears — XOR, never reversal).
    """
    if not isinstance(b, Builder):
        raise ValueError("b must be a Builder: refusing")
    for name, w in (("a", a), ("c", c), ("out", out)):
        _check_word(w, 3, name, b.num_qubits)
    _check_word(pool, 1, "pool", b.num_qubits)
    (p,) = pool
    for i in range(3):
        for j in range(3):
            b.ccx(a[i], c[j], p)
            for k in range(3):
                if (F8_MUL_TERMS[i][j] >> k) & 1:
                    b.cx(p, out[k])
            b.ccx(a[i], c[j], p)
    if seg is not None:
        seg.append(("f8", list(a), list(c), list(out), list(pool)))
    return 18


def emit_f8_sqr(b: Builder, a: list[int], out: list[int], seg: list | None = None) -> None:
    """F8 squaring (Frobenius, CNOT-only from F8_SQR_ROWS)."""
    if not isinstance(b, Builder):
        raise ValueError("b must be a Builder: refusing")
    for name, w in (("a", a), ("out", out)):
        _check_word(w, 3, name, b.num_qubits)
    for k, row in enumerate(F8_SQR_ROWS):
        for i, bit in enumerate(row):
            if bit:
                b.cx(a[i], out[k])
                if seg is not None:
                    seg.append(("cx", a[i], out[k]))


def emit_mu1_scale(b: Builder, c: list[int], out: list[int], seg: list | None = None) -> None:
    """Multiply an F64 element (F8-pair coords) by MU1 = Y (CNOT-only).

    Y*(c0+c1*Y) = c1 + (c0+c1)*Y (Y^2 = Y+1): out0 = c1, out1 = c0+c1.
    """
    if not isinstance(b, Builder):
        raise ValueError("b must be a Builder: refusing")
    for name, w in (("c", c), ("out", out)):
        _check_word(w, 6, name, b.num_qubits)
    for i in range(3):
        b.cx(c[3 + i], out[i])
        if seg is not None:
            seg.append(("cx", c[3 + i], out[i]))
    for i in range(3):
        for src in (c[i], c[3 + i]):
            b.cx(src, out[3 + i])
            if seg is not None:
                seg.append(("cx", src, out[3 + i]))


def emit_f64_sqr(b: Builder, a: list[int], out: list[int], seg: list | None = None) -> None:
    """F64 squaring (CNOT-only): (a0+a1*Y)^2 = (a0^2+a1^2) + a1^2*Y."""
    if not isinstance(b, Builder):
        raise ValueError("b must be a Builder: refusing")
    for name, w in (("a", a), ("out", out)):
        _check_word(w, 6, name, b.num_qubits)
    emit_f8_sqr(b, a[0:3], out[0:3], seg)
    emit_f8_sqr(b, a[3:6], out[0:3], seg)
    emit_f8_sqr(b, a[3:6], out[3:6], seg)


def emit_f64_mul(b: Builder, a: list[int], c: list[int], out: list[int],
                 pool: list[int], seg: list | None = None) -> int:
    """F64 multiply-accumulate ((a0+a1*Y)(c0+c1*Y), Y^2=Y+1).

    const = a0*c0 + a1*c1, lin = a0*c1 + a1*c0 + a1*c1: 5 F8 mults, the
    a1*c1 product emitted once into each half (no shared scratch — a
    scratch reused across sequential mults accumulates stale products;
    caught live as a wrong answer at x=2 while B=0 inputs passed).
    Returns 90. Recorded to seg as replayable ops when seg is given.
    """
    if not isinstance(b, Builder):
        raise ValueError("b must be a Builder: refusing")
    for name, w in (("a", a), ("c", c), ("out", out)):
        _check_word(w, 6, name, b.num_qubits)
    _check_word(pool, 1, "pool", b.num_qubits)
    n = 0
    n += emit_f8_mul(b, a[0:3], c[0:3], out[0:3], pool, seg)
    n += emit_f8_mul(b, a[3:6], c[3:6], out[0:3], pool, seg)
    n += emit_f8_mul(b, a[3:6], c[3:6], out[3:6], pool, seg)
    n += emit_f8_mul(b, a[0:3], c[3:6], out[3:6], pool, seg)
    n += emit_f8_mul(b, a[3:6], c[0:3], out[3:6], pool, seg)
    if n != 90:
        raise ValueError(f"F64 mult cost drifted (want 90, got {n}): refusing")
    return n


def _mirror(b: Builder, seg: list, pool: list[int]) -> int:
    """Replay recorded ops in reverse (mults re-accumulate clear, CNOTs reverse)."""
    n = 0
    for op in reversed(seg):
        if op[0] == "cx":
            _, a, c = op
            b.cx(a, c)
        else:
            _, a, c, out, _ = op
            if len(a) == 3:
                n += emit_f8_mul(b, a, c, out, pool)
            else:
                raise ValueError(f"unreplayable op {op[0]}: refusing")
    return n


#: Temp layout for emit_tower_inv12 (79 temps: 78 data + shared pool).
_TW = (0, 12)     # tower coords (a0,a1,b0,b1 as 3-bit groups)
_T1 = (12, 18)    # A+B
_M1 = (18, 24)    # T1*A
_B2 = (24, 30)    # B^2
_M2 = (30, 36)    # MU1*B2
_NW = (36, 42)    # norm N
_S = (42, 45)     # n0+n1
_Q0 = (45, 48)    # n0*S
_Q1 = (48, 51)    # n1^2
_R2 = (51, 54)    # M8v^2
_R4 = (54, 57)    # M8v^4
_MI = (57, 60)    # M8v^-1
_NI = (60, 66)    # N^-1
_C = (66, 72)     # T1*NI
_D = (72, 78)     # B*NI
_POOL = (78, 79)
N_TOWER_TEMPS = 79


def emit_tower_inv12(b: Builder, x: list[int], h: list[int], temps: list[int]) -> int:
    """Tower-field F2^12 inverse (x -> h, x preserved, 0 -> 0).

    Forward: basis change -> T1=A+B -> M1=T1*A -> B2=B^2 -> M2=MU1*B2 ->
    N=M1+M2 -> S,n0-part -> Q0=n0*S, Q1=n1^2 -> M8v=Q0+Q1 -> R2,R4 squares
    -> MI=R2*R4 -> NI=(S*MI, n1*MI) -> C=T1*NI, D=B*NI -> h accumulate.
    Mirror: mults cleared by re-accumulation in reverse, CNOT ladders
    reversed (each replay reads only still-intact values). h must be
    zeroed; x, h, temps pairwise disjoint. Returns the Toffoli count.
    """
    if not isinstance(b, Builder):
        raise ValueError("b must be a Builder: refusing")
    _check_word(x, 12, "x", b.num_qubits)
    _check_word(h, 12, "h", b.num_qubits)
    _check_word(temps, N_TOWER_TEMPS, "temps", b.num_qubits)
    if len(set(list(x) + list(h) + list(temps))) != 12 + 12 + N_TOWER_TEMPS:
        raise ValueError("x, h, temps must be disjoint: refusing")

    def words(rng: tuple[int, int]) -> list[int]:
        return [temps[i] for i in range(*rng)]

    tw, t1, m1 = words(_TW), words(_T1), words(_M1)
    b2, m2, nw = words(_B2), words(_M2), words(_NW)
    s, q0, q1 = words(_S), words(_Q0), words(_Q1)
    r2, r4, mi = words(_R2), words(_R4), words(_MI)
    ni, c, d = words(_NI), words(_C), words(_D)
    pool = words(_POOL)
    n0, n1 = nw[0:3], nw[3:6]
    ni0, ni1 = ni[0:3], ni[3:6]

    fwd, back = tower12_change_matrices()
    seg: list = []
    n = 0
    for j, row in enumerate(fwd):  # basis change (recorded for mirror)
        for i, bit in enumerate(row):
            if bit:
                b.cx(x[i], tw[j])
                seg.append(("cx", x[i], tw[j]))
    for i in range(6):  # t1 = A+B (tw[0:6] = A, tw[6:12] = B)
        b.cx(tw[i], t1[i])
        seg.append(("cx", tw[i], t1[i]))
        b.cx(tw[6 + i], t1[i])
        seg.append(("cx", tw[6 + i], t1[i]))
    n += emit_f64_mul(b, t1, tw[0:6], m1, pool, seg)  # m1 = t1*A
    emit_f64_sqr(b, tw[6:12], b2, seg)  # b2 = B^2
    emit_mu1_scale(b, b2, m2, seg)  # m2 = MU1*b2
    for i in range(6):  # n = m1+m2
        for src in (m1[i], m2[i]):
            b.cx(src, nw[i])
            seg.append(("cx", src, nw[i]))
    for i in range(3):  # s = n0+n1
        b.cx(n0[i], s[i])
        seg.append(("cx", n0[i], s[i]))
        b.cx(n1[i], s[i])
        seg.append(("cx", n1[i], s[i]))
    n += emit_f8_mul(b, n0, s, q0, pool, seg)  # q0 = n0*s
    emit_f8_sqr(b, n1, q1, seg)  # q1 = n1^2
    # M8v = q0+q1 accumulated into q0's wires (mirror reverses the fan-in
    # first, then the n0*s mult re-emission clears q0 exactly).
    for i in range(3):
        b.cx(q1[i], q0[i])
        seg.append(("cx", q1[i], q0[i]))
    emit_f8_sqr(b, q0, r2, seg)
    emit_f8_sqr(b, r2, r4, seg)
    n += emit_f8_mul(b, r2, r4, mi, pool, seg)  # mi = M8v^-1
    n += emit_f8_mul(b, s, mi, ni0, pool, seg)
    n += emit_f8_mul(b, n1, mi, ni1, pool, seg)
    n += emit_f64_mul(b, t1, ni, c, pool, seg)  # c = t1*NI
    n += emit_f64_mul(b, tw[6:12], ni, d, pool, seg)  # d = B*NI
    cd = c + d
    for i, row in enumerate(back):  # answer persists: never reversed.
        # back[i][j]: poly bit i reads tower coord j (row = output, col = input;
        # transposing this silently scrambles the answer — caught live).
        for j, bit in enumerate(row):
            if bit:
                b.cx(cd[j], h[i])
    n += _mirror(b, seg, pool)
    return n
