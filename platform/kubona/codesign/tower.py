"""Tower-field F(2^2)^2 inverter (paper F3 family, exact integer math).

The paper's endpoint came through tower restructuring: F16 as a quadratic
extension of F4, where inversion drops to subfield operations (norm +
Fermat inverse a^2, both tiny) plus basis changes (CNOT-only). This module
is that mathematics, exactly:

- GF(4) as 2-bit ints with t^2 = t + 1 (add = XOR), isomorphism-verified
  against the concrete subfield {0,1,6,7} of F16 over all 16 pairs.
- Tower basis {1, t=6, z=2, tz} with z^2+z+6 = 0 (machine-found roots,
  verified); to_tower/from_tower are inverse 4x4 binary changes of basis
  (round-trip gated on all 16 elements).
- tower_invert(): N = A(A+B) + 6 B^2 in F4, A^-1 = ((A+B) + B z)/N with
  F4 inversion as squaring. Agrees with F24.inv on all 15 nonzero inputs
  (exhaustive — the F3 leg of the family comparison, as math).

Scope (stated): classical exact math (no Toffolis counted) — the CIRCUIT
version (F4-mult emitters + MBU packing + basis-change CNOTs) is queued
behind this verified arithmetic. F1/F2/MBU/Kara circuits carry the
gate counts meanwhile.
"""

from __future__ import annotations


def gf4_add(a: int, b: int) -> int:
    """GF(4) add (XOR) with t^2 = t + 1 representation (t = 2)."""
    for v in (a, b):
        if not isinstance(v, int) or not (0 <= v < 4):
            raise ValueError(f"GF(4) element must be in [0,4), got {v!r}: refusing")
    return a ^ b


def gf4_mul(a: int, b: int) -> int:
    """GF(4) multiply: (a0+a1 t)(b0+b1 t), t^2 = t+1."""
    for v in (a, b):
        if not isinstance(v, int) or not (0 <= v < 4):
            raise ValueError(f"GF(4) element must be in [0,4), got {v!r}: refusing")
    a0, a1 = a & 1, (a >> 1) & 1
    b0, b1 = b & 1, (b >> 1) & 1
    return ((a0 & b0) ^ (a1 & b1)) | (((a0 & b1) ^ (a1 & b0) ^ (a1 & b1)) << 1)


def gf4_inv(a: int) -> int:
    """GF(4) Fermat inverse: a^3 = 1, so a^-1 = a^2 (a = 0 refuses)."""
    if not isinstance(a, int) or not (0 <= a < 4):
        raise ValueError(f"GF(4) element must be in [0,4), got {a!r}: refusing")
    if a == 0:
        raise ValueError("gf4_inv(0) undefined: refusing")
    return gf4_mul(a, a)


#: Tower parameters (machine-found, verified below): abstract GF(4) uses
#: t = 2 (10 binary) with t^2 = t + 1; T_SUB embeds it as {0,1,6,7} in F16;
#: Z_ROOT = 2 satisfies z^2 + z + T_SUB[2] = z^2 + z + 6 = 0 inside F16.
#: Z_MINIMAL_MU is the ABSTRACT mu (= t = 2) used inside GF(4) arithmetic —
#: the concrete 6 lives only across the T_SUB boundary (mixing them raised
#: a range refusal live, which is why the two spellings both appear here).
T_SUB = {0: 0, 1: 1, 2: 6, 3: 7}
Z_ROOT = 2
Z_MINIMAL_MU = 2


def tower_mul(Aa: int, Ab: int, Ba: int, Bb: int) -> tuple[int, int]:
    """Multiply (Aa + Ab z)(Ba + Bb z) with z^2 = z + t over GF(4)."""
    for v in (Aa, Ab, Ba, Bb):
        if not isinstance(v, int) or not (0 <= v < 4):
            raise ValueError(f"tower coefficient must be in GF(4): refusing {v!r}")
    # z^2 = z + 6: const = Aa Ba + 6 Ab Bb; z = Aa Bb + Ab Ba + Ab Bb.
    const = gf4_add(gf4_mul(Aa, Ba), gf4_mul(gf4_mul(Ab, Bb), Z_MINIMAL_MU))
    lin = gf4_add(gf4_add(gf4_mul(Aa, Bb), gf4_mul(Ab, Ba)), gf4_mul(Ab, Bb))
    return (const, lin)


def tower_inv(A: int, B: int) -> tuple[int, int]:
    """Invert A + B z (not both zero): N = A(A+B) + 6B^2, out = ((A+B) + Bz)/N."""
    for v in (A, B):
        if not isinstance(v, int) or not (0 <= v < 4):
            raise ValueError(f"tower coefficient must be in GF(4): refusing {v!r}")
    if A == 0 and B == 0:
        raise ValueError("tower_inv(0) undefined (encode 0^-1 = 0 at task layer): refusing")
    N = gf4_add(gf4_mul(A, gf4_add(A, B)), gf4_mul(gf4_mul(B, B), Z_MINIMAL_MU))
    if N == 0:
        raise ValueError("zero norm on nonzero input (implementation bug): refusing")
    Ni = gf4_inv(N)
    return (gf4_mul(gf4_add(A, B), Ni), gf4_mul(B, Ni))


def _basis_matrix() -> list[list[int]]:
    """Columns: coordinates of tower basis {1, t, z, tz} in poly basis."""
    from .gf import F24
    t, z = 6, Z_ROOT
    tz = F24.mul(t, z)
    cols = []
    for v in (1, t, z, tz):
        cols.append([(v >> i) & 1 for i in range(4)])
    return cols


def _mat_apply(mat: list[list[int]], vec: list[int]) -> list[int]:
    """4x4 binary matrix times 4-bit vector (rows of mat dot vec)."""
    out = []
    for r in mat:
        out.append(sum(a & b for a, b in zip(r, vec)) & 1)
    return out


def _mat_inv(mat: list[list[int]]) -> list[list[int]]:
    """Invert a 4x4 binary matrix (fail-closed on singular)."""
    m = [row[:] + [1 if i == j else 0 for j in range(4)] for i, row in enumerate(mat)]
    for col in range(4):
        piv = next((i for i in range(col, 4) if m[i][col]), None)
        if piv is None:
            raise ValueError("basis matrix singular (implementation bug): refusing")
        m[col], m[piv] = m[piv], m[col]
        for i in range(4):
            if i != col and m[i][col]:
                m[i] = [(a ^ b) & 1 for a, b in zip(m[i], m[col])]
    return [row[4:] for row in m]


def to_tower(x: int) -> tuple[int, int]:
    """Poly-basis element -> tower coords (A, B) with x = A + B z."""
    if not isinstance(x, int) or not (0 <= x < 16):
        raise ValueError(f"element must be in [0,16), got {x!r}: refusing")
    inv = _mat_inv([[c[i] for c in _basis_matrix()] for i in range(4)])
    bits = [(x >> i) & 1 for i in range(4)]
    t = _mat_apply(inv, bits)
    A = t[0] | (t[1] << 1)
    B = t[2] | (t[3] << 1)
    # round-trip gate (conversion must reproduce the input exactly)
    if from_tower(A, B) != x:
        raise ValueError(f"to_tower round-trip failed at {x} (implementation bug): refusing")
    return (A, B)


def from_tower(A: int, B: int) -> int:
    """Tower coords -> poly-basis element (A + B z evaluated in F16)."""
    from .gf import F24
    for v in (A, B):
        if not isinstance(v, int) or not (0 <= v < 4):
            raise ValueError(f"tower coefficient must be in GF(4): refusing {v!r}")
    # A + B z with A, B read through the subfield embedding T_SUB
    a = T_SUB[A]
    b = T_SUB[B]
    return F24.add(a, F24.mul(b, Z_ROOT))


def tower_invert(x: int) -> int:
    """F3 inversion: change basis, invert in the tower, change back."""
    from .gf import F24
    if not isinstance(x, int) or not (0 <= x < 16):
        raise ValueError(f"element must be in [0,16), got {x!r}: refusing")
    if x == 0:
        return 0
    A, B = to_tower(x)
    Ai, Bi = tower_inv(A, B)
    out = from_tower(Ai, Bi)
    if F24.mul(x, out) != 1:
        raise ValueError(f"tower inversion failed at {x} (implementation bug): refusing")
    return out


def tower_change_matrices() -> tuple[list[list[int]], list[list[int]]]:
    """Basis-change matrices (rows) between poly coords and tower coords.

    Returns (to_tower_rows, from_tower_rows): to_tower_rows[j] is the mask
    of poly bits feeding tower bit j (A0, A1, B0, B1); from_tower_rows the
    reverse. Product is the identity (asserted live — a mistranscription
    here silently scrambles every basis change).
    """
    cols = _basis_matrix()  # cols[k][i] = bit i of tower basis vector k
    m_rows = [[cols[k][i] for k in range(4)] for i in range(4)]
    fwd = _mat_inv(m_rows)  # poly coords -> tower coords (same map to_tower uses)
    inv = m_rows  # tower coords -> poly coords
    # product gate (both directions identity, else refuse)
    for a, bb in ((fwd, inv), (inv, fwd)):
        for i in range(4):
            for j in range(4):
                if (sum(a[i][k] & bb[k][j] for k in range(4)) & 1) != (1 if i == j else 0):
                    raise ValueError("basis matrices not inverse (implementation bug): refusing")
    return (fwd, inv)


def _emit_f4_mul(b, A: list[int], B: list[int], OUT: list[int],
                 pool: list[int], mode: str) -> int:
    """F4 multiply-accumulate (4 ANDs) in plain or MBU-packed form.

    OUT ^= A*B with OUT0 <- A0B0,A1B1 and OUT1 <- A0B1,A1B0,A1B1 (from
    (a0+a1 t)(b0+b1 t), t^2 = t+1). Plain: each AND into the pool temp,
    fanned out, uncomputed (2T). MBU: measured away with CZ corrections
    on the two controls (1T). A, B preserved (controls/XORs only); OUT
    accumulates (caller clears by re-accumulating — XOR, never reversal).
    """
    from .kickmix import Builder
    if not isinstance(b, Builder):
        raise ValueError("b must be a Builder: refusing")
    if mode not in ("plain", "mbu"):
        raise ValueError(f"mode must be plain|mbu, got {mode!r}: refusing")
    for name, w in (("A", A), ("B", B), ("OUT", OUT)):
        if len(w) != 2:
            raise ValueError(f"{name} must be 2 F4 bits: refusing")
    if len(pool) != 1:
        raise ValueError("pool must be exactly 1 temp: refusing")
    (p,) = pool
    terms = [(0, 0, 0), (1, 1, 0), (0, 1, 1), (1, 0, 1), (1, 1, 1)]
    n_ccx = 0
    for i, j, k in terms:
        if mode == "mbu":
            b.alloc(p)
        b.ccx(A[i], B[j], p)
        n_ccx += 1
        b.cx(p, OUT[k])
        if mode == "mbu":
            r = b.hmr(p)
            b.cz(A[i], B[j], cond=(r,))
        else:
            b.ccx(A[i], B[j], p)
            n_ccx += 1
    return n_ccx


def _out_pattern() -> list[tuple[int, int]]:
    """CNOT pattern accumulating tower (C,D) into poly target h.

    For each abstract bit (C0,C1,D0,D1), set it alone and read the poly
    element C_c + D_c*z through the embedding (emission-time classical
    computation, exact). Returns [(src_index, h_bit)] with src 0..3.
    """
    from .gf import F24
    pat = []
    for p in range(4):  # one-hot over abstract bits (C0, C1, D0, D1)
        Cb = (1 if p == 0 else 0, 1 if p == 1 else 0)
        Db = (1 if p == 2 else 0, 1 if p == 3 else 0)
        Cc = T_SUB[Cb[0] | (Cb[1] << 1)]
        Dc = T_SUB[Db[0] | (Db[1] << 1)]
        out = Cc ^ F24.mul(Dc, Z_ROOT)
        for t in range(4):
            if (out >> t) & 1:
                pat.append((p, t))
    return pat


def build_tower_inverter(mode: str = "plain") -> tuple[object, dict]:
    """F3 tower inverter as a kickmix circuit (plain or MBU-packed).

    x[4] in (preserved), h[4] target (init 0, gets x^-1 with 0^-1 = 0):
    basis change (CNOTs) -> t1/N/NI ladders (CNOTs) -> three F4 mults
    (norm, two scale-backs) -> out accumulate (CNOTs) -> mirror uncompute
    (CNOT ladders reversed; mults cleared by re-accumulation — XOR, never
    reversal). Squaring is total (Frobenius has no 0 edge case), so x = 0
    flows through to h = 0 with no special-casing. Proven exhaustive 16/16.
    """
    from .kickmix import Builder
    if mode not in ("plain", "mbu"):
        raise ValueError(f"mode must be plain|mbu, got {mode!r}: refusing")
    x = [0, 1, 2, 3]
    h = [4, 5, 6, 7]
    A, B = [8, 9], [10, 11]
    T1, M1 = [12, 13], [14, 15]
    B2, M2 = [16, 17], [18, 19]
    N, NI = [20, 21], [22, 23]
    C, D = [24, 25], [26, 27]
    POOL = [28]
    data = A + B + T1 + M1 + B2 + M2 + N + NI + C + D
    b = Builder(29)
    for q in data + (POOL if mode == "plain" else []):
        # MBU pool is alloc'd per AND inside _emit_f4_mul (recycled after
        # each hmr release); pre-allocating it would double-alloc.
        b.alloc(q)
    def _un(stage: list[tuple[int, int]]) -> None:
        for a, c in reversed(stage):
            b.cx(a, c)

    n_ccx = 0
    fwd, _ = tower_change_matrices()
    TB = A + B
    seg_basis: list[tuple[int, int]] = []
    for j, row in enumerate(fwd):
        for i, bit in enumerate(row):
            if bit:
                b.cx(x[i], TB[j])
                seg_basis.append((x[i], TB[j]))
    seg_t1: list[tuple[int, int]] = []
    for i in range(2):
        b.cx(A[i], T1[i])  # t1 = A + B (both halves; B-only was missing
        seg_t1.append((A[i], T1[i]))  # in the first draft — caught live: every
        b.cx(B[i], T1[i])  # B != 0 input failed while B == 0 passed)
        seg_t1.append((B[i], T1[i]))
    n_ccx += _emit_f4_mul(b, T1, A, M1, POOL, mode)  # m1 = t1 * A
    seg_b2 = [(B[0], B2[0]), (B[1], B2[0]), (B[1], B2[1])]
    for a, c in seg_b2:
        b.cx(a, c)  # b2 = B^2 (Frobenius, linear)
    seg_m2 = [(B2[1], M2[0]), (B2[0], M2[1]), (B2[1], M2[1])]
    for a, c in seg_m2:
        b.cx(a, c)  # m2 = b2 * t (mu = t is linear)
    seg_N: list[tuple[int, int]] = []
    for i in range(2):  # N = m1 + m2
        for a, c in ((M1[i], N[i]), (M2[i], N[i])):
            b.cx(a, c)
            seg_N.append((a, c))
    seg_Ni = [(N[0], NI[0]), (N[1], NI[0]), (N[1], NI[1])]
    for a, c in seg_Ni:
        b.cx(a, c)  # Ni = N^2 (Frobenius)
    n_ccx += _emit_f4_mul(b, T1, NI, C, POOL, mode)  # C = t1 * Ni
    n_ccx += _emit_f4_mul(b, B, NI, D, POOL, mode)  # D = B * Ni
    CD = C + D
    for p, t in _out_pattern():
        b.cx(CD[p], h[t])  # answer persists: never reversed
    # ---- mirror uncompute. Mults clear by re-accumulation (XOR cancels;
    # reversal would re-add). Each clear/mirror step reads only still-intact
    # values: D/C clears need B/NI/t1; Ni..b2 mirrors need N/M1/M2/B/N;
    # m1-clear needs T1/A; t1/basis mirrors need A/B/x. Any other order
    # consumes a control before its reader runs (caught live as dirty N).
    n_ccx += _emit_f4_mul(b, B, NI, D, POOL, mode)  # clear D
    n_ccx += _emit_f4_mul(b, T1, NI, C, POOL, mode)  # clear C
    _un(seg_Ni)
    _un(seg_N)
    _un(seg_m2)
    _un(seg_b2)
    n_ccx += _emit_f4_mul(b, T1, A, M1, POOL, mode)  # clear m1
    _un(seg_t1)
    _un(seg_basis)
    for q in data:
        b.free(q)
    if mode == "plain":
        b.free(POOL[0])  # MBU pool was consumed by measurement instead
    return b, {"x": tuple(x), "h": tuple(h), "ccx": n_ccx, "mode": mode}


def tower_inv_spec():
    """TaskSpec for the tower inverter (same 4+4 interface as the families)."""
    from .gf import f24_inversion_table
    from .kickmix import TaskSpec
    table = f24_inversion_table()

    def _perm(bits: dict[int, int]) -> dict[int, int]:
        out = dict(bits)
        xv = sum((bits.get(i, 0) & 1) << i for i in range(4))
        inv = table[xv]
        for i in range(4):
            out[4 + i] = (inv >> i) & 1
        return out

    anc = tuple(list(range(8, 28)))
    return TaskSpec(num_qubits=29, inputs=(0, 1, 2, 3), outputs=(4, 5, 6, 7),
                    ancillas=anc, target_perm=_perm)
