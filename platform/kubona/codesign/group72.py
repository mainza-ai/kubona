"""Sigma(72x3) group law in normal-form coordinates (SI V, exact integers).

Elements are (P,Q,R,M,U) with P,Q,R in Z3, M in Z4, U in Z2 (paper Eq
nonabelian-normal-form: g = w^p C^q E^r V^{2s+t} X^u, m = 2s+t). Contents:

- gmul(W, g): general normal-form product (paper Alg gaugeprod.a, verbatim).
  Verified as GROUP LAW, not transcription: associativity over sampled
  triples including the X-sector (a mistranscribed X-rewrite line breaks
  associativity almost surely), identity, and brute-force inverses + count
  over all 216 tuples (finite monoid + two-sided inverses = group).
- recode_link(): X -> Xo substitution on links (X = w C^2 E^2 Xo, so a
  set X-bit shifts (q,r) by the V-crossed table values and p by
  1 + dp0 + r*qp0; V-split m = 2s+t is unique). Round-trip gated
  (phase-blind — agreement is the guard, not the round-trip).
- to_internal(): original -> specialized factor order, both Upsilon
  sectors (Upsilon=0 by the INVERSE commute table — exact 9-search,
  uniqueness-gated; the forward table points the wrong way and the
  agreement test caught that; Upsilon=1 by matrix solve over the 27
  (b, Q2, R2) candidates, uniqueness-gated).
- spec_update(): specialized running-product update (paper Alg gaugeprod.b,
  full: tildeP + Q,R perms + M + Upsilon). Agreement with gmul on ALL
  216^2 pairs is checked live:
  to_internal(gmul(W,g)) == spec_update(to_internal(W), recode(g)).

What this proves: the 35T-vs-146T mechanism (opposite factor orders +
folded phase) computes the same product as general multiplication on the
covered sector, and the general law is a sound group law everywhere. The
trace readout is grounded below in the true 3x3 representation (Osorio
et al. 2025, arXiv:2511.17437v1 §II — public CC BY 4.0; generator
matrices Eq 1, character table Tab 2): the coordinate law is verified as
a matrix homomorphism on all 216^2 pairs, conjugacy classes are derived
from the law and checked against the table's size/order rows, and the
trace-class function f(g) = 2Re Tr(M_g) is computed exactly in the 12th
cyclotomic field (never floats) with its value set machine-checked
against the paper's {-3,...,6}.
"""

from __future__ import annotations

from fractions import Fraction


# ---------------------------------------------------------------------------
# True 3x3 representation (Osorio et al. 2025, arXiv:2511.17437v1 Eq 1,
# public CC BY 4.0). Exact arithmetic in Q(zeta_12), zeta = e^{i pi/6},
# modulo x^4 - x^2 + 1 — never floats. Only add/mul are needed (matrix
# products, traces); the one constant requiring division (1/(i sqrt(3)))
# is closed in radicals below and verified by c*(2 zeta^2 - 1) == 1.
# ---------------------------------------------------------------------------


class _Cyc:
    """Exact element of Q(zeta_12): a0 + a1 z + a2 z^2 + a3 z^3 (Fractions)."""

    __slots__ = ("c",)

    def __init__(self, c0=0, c1=0, c2=0, c3=0):
        self.c = (Fraction(c0), Fraction(c1), Fraction(c2), Fraction(c3))

    def __add__(self, o):
        if not isinstance(o, _Cyc):
            return NotImplemented
        return _Cyc(*(x + y for x, y in zip(self.c, o.c)))

    def __neg__(self):
        return _Cyc(*(-x for x in self.c))

    def __sub__(self, o):
        if not isinstance(o, _Cyc):
            return NotImplemented
        return self + (-o)

    def __mul__(self, o):
        if not isinstance(o, _Cyc):
            return NotImplemented
        t = [Fraction(0)] * 7
        for i, x in enumerate(self.c):
            if x:
                for j, y in enumerate(o.c):
                    if y:
                        t[i + j] += x * y
        for k in (6, 5, 4):  # x^k = x^{k-2} - x^{k-4} (from x^4 = x^2 - 1)
            if t[k]:
                t[k - 2] += t[k]
                t[k - 4] -= t[k]
        return _Cyc(*t[:4])

    def __eq__(self, o):
        return isinstance(o, _Cyc) and self.c == o.c

    def __hash__(self):
        return hash(self.c)

    def __repr__(self):
        return f"Cyc{self.c}"


_CYC_ZERO = _Cyc()
_CYC_ONE = _Cyc(1)
# omega = e^{2 pi i/3} = zeta^4 = zeta^2 - 1
_CYC_W = _Cyc(-1, 0, 1, 0)
# omega^2 = -zeta^2
_CYC_W2 = _Cyc(0, 0, -1, 0)
# 1/(i sqrt(3)) = (1 - 2 zeta^2)/3 (verified below by direct multiplication)
_CYC_C0 = _Cyc(Fraction(1, 3), 0, Fraction(-2, 3), 0)


def _mmat(rows: list[list[_Cyc]]) -> tuple:
    return tuple(tuple(r) for r in rows)


def _miden() -> tuple:
    z, o = _CYC_ZERO, _CYC_ONE
    return _mmat([[o, z, z], [z, o, z], [z, z, o]])


def _mmul(A: tuple, B: tuple) -> tuple:
    return _mmat([[A[i][0] * B[0][j] + A[i][1] * B[1][j] + A[i][2] * B[2][j]
                   for j in range(3)] for i in range(3)])


def _mscal(s: _Cyc, A: tuple) -> tuple:
    return _mmat([[s * x for x in row] for row in A])


def _mpow(A: tuple, e: int) -> tuple:
    if e < 0:
        raise ValueError(f"negative matrix power {e}: refusing")
    R, base = _miden(), A
    while e:
        if e & 1:
            R = _mmul(R, base)
        base = _mmul(base, base)
        e >>= 1
    return R


def _mtrace(A: tuple) -> _Cyc:
    return A[0][0] + A[1][1] + A[2][2]


def _gen_matrices() -> dict[str, tuple]:
    """Osorio Eq 1 generator matrices (exact). Verified in-test against
    Eq 2 relations (orders, XC/XE/XV rewrites) and X^2 = C^2 V^2."""
    z, o, w, w2, c0 = _CYC_ZERO, _CYC_ONE, _CYC_W, _CYC_W2, _CYC_C0
    C = _mmat([[o, z, z], [z, w, z], [z, z, w2]])
    E = _mmat([[z, o, z], [z, z, o], [o, z, z]])
    V = _mmat([[c0 * x for x in row] for row in
               [[o, o, o], [o, w, w2], [o, w2, w]]])
    X = _mmat([[c0 * x for x in row] for row in
               [[o, o, w2], [o, w, w], [w, o, w]]])
    return {"C": C, "E": E, "V": V, "X": X}


_GEN: dict[str, tuple] | None = None


def _gens() -> dict[str, tuple]:
    global _GEN
    if _GEN is None:
        _GEN = _gen_matrices()
    return _GEN


def gcheck(el: tuple[int, int, int, int, int]) -> None:
    if not isinstance(el, tuple) or len(el) != 5:
        raise ValueError(f"element must be a 5-tuple, got {el!r}: refusing")
    P, Q, R, M, U = el
    for name, v, mod in (("P", P, 3), ("Q", Q, 3), ("R", R, 3),
                         ("M", M, 4), ("U", U, 2)):
        if isinstance(v, bool) or not isinstance(v, int) or not (0 <= v < mod):
            raise ValueError(f"{name} must be in Z{mod}, got {v!r}: refusing")


def gidentity() -> tuple[int, int, int, int, int]:
    return (0, 0, 0, 0, 0)


def _commute_inv_coords(Q: int, R: int, M: int) -> tuple[int, int, int]:
    """Inverse coordinate map: the unique (Q2, R2, a) with _commute sending
    (Q2, R2) to (Q, R) with phase a (the box's table is a bijection per M;
    non-uniqueness here would be an implementation bug, refused)."""
    found = [(q2, r2, _commute(q2, r2, M)[0])
             for q2 in range(3) for r2 in range(3)
             if _commute(q2, r2, M)[1:] == (Q % 3, R % 3)]
    if len(found) != 1:
        raise ValueError(f"commute not bijective at ({Q},{R},{M}): refusing")
    return found[0]


def _commute(q: int, r: int, M: int) -> tuple[int, int, int]:
    """Move V^M right past C^q E^r: V^M C^q E^r = w^{dp} C^q' E^r' V^M.

    Returns (delta_p, q', r'). The paper's commute table (Alg
    gaugeprod.a), shared by gmul and to_internal — one copy, never two
    that can rot apart. Values and usage match the paper exactly; only
    the prose direction ("commute C^qE^r through V^M") is loose — the
    matrix identity above is the true semantics, machine-verified on all
    36 (q, r, M) against the Osorio representation (which also grounds
    gmul's use: Q += q', R += r', P += p + dp + R q').
    """
    if M == 0:
        return (0, q, r)
    if M == 1:
        return ((2 * q * r) % 3, (-r) % 3, q % 3)
    if M == 2:
        return (0, (-q) % 3, (-r) % 3)
    if M == 3:
        return ((2 * q * r) % 3, r % 3, (-q) % 3)
    raise ValueError(f"M must be in Z4, got {M!r}: refusing")


def gmul(W: tuple[int, int, int, int, int],
         g: tuple[int, int, int, int, int]) -> tuple[int, int, int, int, int]:
    """General normal-form product W*g (paper Alg gaugeprod.a)."""
    gcheck(W)
    gcheck(g)
    P, Q, R, M, ups = W
    p, q, r, m, u = g
    if not (0 <= m < 4):
        raise ValueError(f"link m must be in Z4, got {m!r}: refusing")
    if ups == 1:  # apply the X-relations to the link coordinates
        s, t = m // 2, m % 2
        p0 = (p + q * r + 2 * (1 if q == 2 else 0) + 2 * (1 if r != 0 else 0)) % 3
        q0, r0 = (2 * q + r) % 3, (q + r) % 3
        p = (p0 + 2 * s + s * r0 + 2 * t) % 3
        q, r = (q0 + s) % 3, (r0 + s + t * (1 + s)) % 3
        s, t = s ^ t, t
        m = 2 * s + t
        if u == 1:  # apply X^2 = C^2 V^2
            p = (p + (1 - t) * (2 - s) * r) % 3
            q = (q + (1 - t) * (2 - s)) % 3
            r = (r + t * (2 - s)) % 3
            s = s ^ 1
            m = 2 * s + t
    dp, qp, rp = _commute(q, r, M)
    return ((P + p + dp + R * qp) % 3, (Q + qp) % 3, (R + rp) % 3,
            (M + m) % 4, ups ^ u)


def ginv(g: tuple[int, int, int, int, int]) -> tuple[int, int, int, int, int]:
    """Inverse by exhaustive search over the 216 tuples (fail-closed)."""
    gcheck(g)
    e = gidentity()
    for P in range(3):
        for Q in range(3):
            for R in range(3):
                for M in range(4):
                    for U in range(2):
                        h = (P, Q, R, M, U)
                        if gmul(g, h) == e and gmul(h, g) == e:
                            return h
    raise ValueError(f"no two-sided inverse for {g!r} (implementation bug): refusing")


def all_elements() -> list[tuple[int, int, int, int, int]]:
    """All 216 coordinate tuples (count-gated: the paper's group order)."""
    els = [(P, Q, R, M, U) for P in range(3) for Q in range(3)
           for R in range(3) for M in range(4) for U in range(2)]
    if len(els) != 216:
        raise ValueError("group order is not 216 (implementation bug): refusing")
    return els


def recode_link(g: tuple[int, int, int, int, int]) -> tuple[int, int, int, int, int, int]:
    """Link to specialized form (p,q,r,s,t,u): Xo substitution + V-split.

    X = w C^2 E^2 Xo (matrix-exact: the bare (2,2) shift drops a phase
    that silently corrupts every X-sector product — caught by full-sector
    agreement), and the V^m sitting between E^r and X must be crossed:
    with (dp0, qp0, rp0) = commute(2, 2, m),
    p' = p + 1 + dp0 + r*qp0, q' = q + qp0, r' = r + rp0 (all mod 3).
    At m = 0 this is p+1+2r (the 2r from pushing E^r past C^2). The m = 0
    case alone passes agreement; the V-crossing terms fail without the
    table. Round-trip gated (inverse subtracts the same terms).
    """
    gcheck(g)
    p, q, r, m, u = g
    if u == 1:
        dp0, qp0, rp0 = _commute(2, 2, m)
        p, q, r = (p + 1 + dp0 + r * qp0) % 3, (q + qp0) % 3, (r + rp0) % 3
    return (p, q, r, m // 2, m % 2, u)


def unrecode_link(gl) -> tuple[int, int, int, int, int]:
    """Specialized link back to original coordinates (inverse of recode).

    Undoes the (2,2) shift AND the recode phase (which depends on the
    ORIGINAL r = r' - rp0: p = p' - 1 - dp0 - r*qp0). Dropping the phase
    round-trips cleanly (self-inverse slip) while corrupting products —
    the agreement test, not the round-trip, is the guard.
    """
    if not isinstance(gl, tuple) or len(gl) != 6:
        raise ValueError(f"link must be a 6-tuple, got {gl!r}: refusing")
    p, q, r, s, t, u = gl
    for name, v, mod in (("p", p, 3), ("q", q, 3), ("r", r, 3),
                         ("s", s, 2), ("t", t, 2), ("u", u, 2)):
        if isinstance(v, bool) or not isinstance(v, int) or not (0 <= v < mod):
            raise ValueError(f"{name} out of range: refusing {v!r}")
    if u == 1:
        m = 2 * s + t
        dp0, qp0, rp0 = _commute(2, 2, m)
        r0 = (r - rp0) % 3
        p, q, r = (p - 1 - dp0 - r0 * qp0) % 3, (q - qp0) % 3, r0
    return (p, q, r, 2 * s + t, u)


def to_internal(W: tuple[int, int, int, int, int]) -> tuple[int, int, int, int, int]:
    """Original factor order -> specialized (both Upsilon sectors).

    W = w^P C^Q E^R V^M must read as w^b V^M C^Q2 E^R2 (Upsilon=0) or
    w^b V^M Xo C^Q2 E^R2 (Upsilon=1, Xo = CEX) with tP = b + Q2*R2 folded
    the same way in both sectors. Upsilon=0 uses the inverse commute
    table (exact 9-search; using it forward mistargets — the agreement
    test caught that). Upsilon=1 solves by matrix search over the 27
    (b, Q2, R2) candidates, uniqueness-gated (closed-form X-conjugation
    inversion queued; the solve is exact and fail-closed, ginv
    precedent). Full-sector agreement with spec_update is proven in-test
    over all 216^2 pairs.
    """
    gcheck(W)
    P, Q, R, M, ups = W
    if ups == 0:
        Q2, R2, a = _commute_inv_coords(Q, R, M)
        return ((P - a + Q2 * R2) % 3, Q2, R2, M, 0)
    G = _gens()
    Xo = _mmul(_mmul(G["C"], G["E"]), G["X"])
    Vm = _mpow(G["V"], M)
    target = rep_matrix(W)
    found = []
    for b in range(3):
        wb = _mscal(_pow_cyc(_CYC_W, b), Vm)
        for Q2 in range(3):
            CQ2 = _mpow(G["C"], Q2)
            for R2 in range(3):
                cand = _mmul(wb, _mmul(Xo, _mmul(CQ2, _mpow(G["E"], R2))))
                if cand == target:
                    found.append((b, Q2, R2))
    if len(found) != 1:
        raise ValueError(f"Upsilon=1 conversion not unique for {W!r}: refusing {found!r}")
    b, Q2, R2 = found[0]
    return ((b + Q2 * R2) % 3, Q2, R2, M, 1)


def spec_update(Wt: tuple[int, int, int, int, int],
                gl) -> tuple[int, int, int, int, int]:
    """Specialized running-product update (paper Alg gaugeprod.b, full).

    Wt = (tildeP, Q, R, M, Upsilon); gl = recoded link (p,q,r,s,t,u).
    Returns the updated specialized running product (same representation).
    """
    if not isinstance(Wt, tuple) or len(Wt) != 5:
        raise ValueError(f"running product must be a 5-tuple: refusing {Wt!r}")
    tP, Q, R, M, ups = Wt
    for name, v, mod in (("tildeP", tP, 3), ("Q", Q, 3), ("R", R, 3),
                         ("M", M, 4), ("Upsilon", ups, 2)):
        if isinstance(v, bool) or not isinstance(v, int) or not (0 <= v < mod):
            raise ValueError(f"{name} out of range: refusing {v!r}")
    unrecode_link(gl)  # shape-gate the link (raises on bad ranges)
    p, q, r, s, t, u = gl
    tP = (tP + p + q * r + Q * r + 2 * R * q) % 3
    Q, R = (Q + q) % 3, (R + r) % 3
    if t == 1:
        Q, R = R % 3, (-Q) % 3
    if s == 1:
        Q, R = (-Q) % 3, (-R) % 3
    if u == 1:
        Q, R = (Q - R) % 3, (-Q - R) % 3
    M = (M + (1 if ups == 0 else -1) * (2 * s + t) + 2 * ups * u) % 4
    return (tP, Q, R, M, ups ^ u)


#: Trace-class values (Gu et al. Eq gauge-trace-class, anchored: the SET is
#: the paper's; the per-class ASSIGNMENT below is computed from the true
#: representation, never transcribed). f = 6 marks the identity class.
TRACE_VALUES = frozenset({-3, -2, -1, 0, 1, 2, 6})

#: Flag order for trace_flags (the six non-identity values, Gu Fig
#: plaquette-layer(c): six flags, at most one set).
FLAG_VALUES = (-3, -2, -1, 0, 1, 2)

#: Gu et al. trace-readout cost anchors (paper-measured, never re-derived
#: here): readout+addition+unlookup per plaquette/rectangle vs the 478T
#: QROM baseline, and the six-flag computation.
TRACE_PAPER_COSTS = {"readout_add_unlookup_plaquette": 76,
                     "readout_add_unlookup_rectangle": 75,
                     "baseline_qrom_lookup": 478, "six_flags": 20}


def rep_matrix(g: tuple[int, int, int, int, int]) -> tuple:
    """True 3x3 image: M_g = omega^P C^Q E^R V^M X^U (exact cyclotomic).

    Verified in-test as a homomorphism against gmul on all 216^2 pairs
    (representation fidelity, not transcription).
    """
    gcheck(g)
    G = _gens()
    P, Q, R, M, U = g
    out = _mscal(_pow_cyc(_CYC_W, P), _miden())
    out = _mmul(out, _mpow(G["C"], Q))
    out = _mmul(out, _mpow(G["E"], R))
    out = _mmul(out, _mpow(G["V"], M))
    out = _mmul(out, _mpow(G["X"], U))
    return out


def _pow_cyc(c: _Cyc, e: int) -> _Cyc:
    if e < 0:
        raise ValueError(f"negative cyclotomic power {e}: refusing")
    r, base = _CYC_ONE, c
    while e:
        if e & 1:
            r = r * base
        base = base * base
        e >>= 1
    return r


def trace_class(g: tuple[int, int, int, int, int]) -> int:
    """f(g) = 2 Re Tr(M_g) as an exact int (Gu Eq gauge-trace-class).

    The trace lands in Q(zeta_12); twice its real part is u + v sqrt(3).
    The paper's integer-valued claim is enforced, not assumed: a nonzero
    sqrt(3)-part or a non-integral rational part refuses (either would
    expose a matrix error).
    """
    t = _mtrace(rep_matrix(g))
    a0, a1, a2, a3 = t.c
    if a1 != 0:
        raise ValueError(f"nonzero sqrt(3)-part in 2Re Tr for {g!r}: refusing {t!r}")
    v = 2 * a0 + a2
    if v.denominator != 1:
        raise ValueError(f"non-integral trace value for {g!r}: refusing {v!r}")
    out = int(v)
    if out not in TRACE_VALUES:
        raise ValueError(f"trace value {out} outside paper set for {g!r}: refusing")
    return out


def trace_flags(g: tuple[int, int, int, int, int]) -> tuple[int, ...]:
    """Six one-hot flags over FLAG_VALUES (Gu Fig plaquette-layer(c)).

    Exactly one set for every non-identity element; none set at the
    identity (f = 6 marks the identity class — machine-checked).
    """
    v = trace_class(g)
    return tuple(1 if x == v else 0 for x in FLAG_VALUES)


def trace_flag_table() -> dict:
    """All 216 elements -> (trace value, six flags), computed not cited."""
    return {g: (trace_class(g), trace_flags(g)) for g in all_elements()}


def conjugacy_classes() -> list[list]:
    """Conjugacy classes from the verified law (h g h^-1 over all h)."""
    els = all_elements()
    seen: set = set()
    out = []
    for g in els:
        if g in seen:
            continue
        cls = sorted({gmul(gmul(h, g), ginv(h)) for h in els})
        for c in cls:
            seen.add(c)
        out.append(cls)
    return out


def class_data() -> list[dict]:
    """Per-class size, element order, and trace value (for table checks)."""
    out = []
    for cls in conjugacy_classes():
        g0 = cls[0]
        k, h = 1, g0
        while h != gidentity():
            h = gmul(h, g0)
            k += 1
            if k > 216:
                raise ValueError(f"order overflow at {g0!r}: refusing")
        out.append({"size": len(cls), "order": k,
                    "trace": trace_class(g0), "rep": g0})
    return out


def coord_bits(g: tuple[int, int, int, int, int]) -> tuple[int, ...]:
    """Normal-form coordinates as 9 bits (Osorio qubit encoding).

    Each Z3 variable as two bits (value = index of the set bit, 0 if
    none); M binary; U one bit. Encodings with both Z3-bits set are
    invalid (no element) — flag functions read 0 there by definition.
    """
    gcheck(g)
    P, Q, R, M, U = g
    bits = []
    for v in (P, Q, R):
        bits += [1 if v == 1 else 0, 1 if v == 2 else 0]
    bits += [(M >> 1) & 1, M & 1, U]
    return tuple(bits)


def _anf_from_table(table: dict[tuple[int, ...], int]) -> frozenset:
    """GF(2) ANF by Mobius transform over the 512-entry truth table."""
    names = list(range(9))
    vals = []
    for mask in range(512):
        bits = tuple((mask >> i) & 1 for i in range(9))
        vals.append(table.get(bits, 0))
    f = list(vals)
    for i in range(9):
        st = 1 << i
        for m in range(512):
            if m & st:
                f[m] ^= f[m ^ st]
    return frozenset(frozenset(names[i] for i in range(9) if (m >> i) & 1)
                     for m in range(512) if f[m])


def trace_flag_anf() -> dict[int, frozenset]:
    """Each flag as ANF over the 9 coordinate bits (derived, not cited).

    Invalid encodings (both Z3-bits set) read 0 by definition, so the
    polynomials decide the value on exactly the 216 valid elements.
    """
    tab = trace_flag_table()
    full: dict[tuple[int, ...], list[int]] = {}
    for g, (_v, flags) in tab.items():
        full[coord_bits(g)] = list(flags)
    out = {}
    for j in range(6):
        out[j] = _anf_from_table({bits: fl[j] for bits, fl in full.items()})
    return out


def eval_anf(poly: frozenset, bits: tuple[int, ...]) -> int:
    """Evaluate an ANF polynomial on a bit assignment."""
    out = 0
    for term in poly:
        prod = 1
        for v in term:
            prod &= bits[v]
        out ^= prod
    return out


def _flag_tables() -> list[dict[int, int]]:
    """The six flag truth tables over 9-bit coordinate encodings."""
    tab = trace_flag_table()
    tables = []
    for j in range(6):
        d = {mask: 0 for mask in range(512)}
        for g, (_v, fl) in tab.items():
            bits = coord_bits(g)
            d[sum(b << i for i, b in enumerate(bits))] = fl[j]
        tables.append(d)
    return tables


def _emit_flag_circuit(polys: list[list[list[tuple[int, int]]]], kind: str) -> tuple[object, dict]:
    """Shared emitter for per-flag cube lists (naive or minimized).

    Single home is kickmix.emit_xor_cover; this is the readout-specific
    file layout (9 inputs, 6 outputs, shared temp pool) on top of it.
    """
    from .kickmix import Builder, emit_xor_cover
    maxextra = 0
    for poly in polys:
        for lits in poly:
            maxextra = max(maxextra, len(lits) - 1)
    ntemps = max(0, maxextra)
    outs = list(range(9, 15))
    temps = list(range(15, 15 + ntemps))
    b = Builder(15 + ntemps)
    for q in temps:
        b.alloc(q)
    n_ccx = [0]
    for j, poly in enumerate(polys):
        emit_xor_cover(b, n_ccx, poly, temps, outs[j])
    for q in temps:
        b.free(q)
    nterms = sum(len(p) for p in polys)
    return b, {"ccx": n_ccx[0], "inputs": tuple(range(9)),
               "outputs": tuple(outs), "temps": ntemps,
               "terms": nterms, "kind": kind}


def build_trace_readout_circuit() -> tuple[object, dict]:
    """Six one-hot trace flags from the 9 coordinate bits (naive ANF).

    Each flag's derived ANF becomes AND-chains into shared sequential
    temps, XORed into a fresh target, chains uncomputed (same emission
    discipline as the divstep decoder circuits). Invalid encodings read
    all-zero by construction. Cost is MEASURED and pinned — it will not
    match the paper's hand-optimized 20T (shared substructure on
    specialized coordinates); this circuit proves the table-to-gates path
    end to end, never that naive emission is competitive. SOP-level
    optimization landed in build_trace_readout_minimized; multi-level
    factorization in build_trace_readout_factored / 
    build_spec_readout_factored (same specs, measured below).
    """
    anf = trace_flag_anf()
    polys = []
    for j in range(6):
        cur = []
        for term in anf[j]:
            if len(term) == 0:
                cur.append([])
            else:
                cur.append([(q, 1) for q in sorted(term)])
        polys.append(cur)
    return _emit_flag_circuit(polys, "naive ANF")


def build_trace_readout_minimized() -> tuple[object, dict]:
    """Six one-hot trace flags from minimized shared AND-pool (exact QM).

    Prime implicants derived exactly per flag; greedy XOR-cover with
    pairwise post-reduction; every output cover verified before emission;
    the circuit is proven exhaustive downstream (same spec as naive).
    Invalid encodings read all-zero by definition (never don't-cares —
    weakening the spec would be falsification, not optimization).
    Measured cost is pinned; multi-level factorization (below) cuts it
    further, and the gap to the paper's hand-optimized 20T stays queued
    (its hand structure is not recoverable from the SI text).
    """
    from .minimize import greedy_multioutput_cover
    cover = greedy_multioutput_cover(_flag_tables(), 9)
    polys = []
    for out in cover:
        cur = []
        for (value, care) in out:
            lits = []
            for i in range(9):
                if (care >> i) & 1:
                    lits.append((i, (value >> i) & 1))
            cur.append(lits)
        polys.append(cur)
    return _emit_flag_circuit(polys, "minimized QM+greedy")


def _spec_flag_tables() -> list[dict[int, int]]:
    """The six flag truth tables over 9-bit specialized-coordinate masks.

    Same 512-space convention as _flag_tables (invalid Z3 bit-pairs read
    zero — the spec pins them, never don't-cares); values keyed by the
    specialized representation the paper's optimized readout acts on.
    """
    tab = spec_flag_table()
    by_bits = {}
    for t, (_v, fl) in tab.items():
        bits = spec_coord_bits(t)
        by_bits[sum(b << i for i, b in enumerate(bits))] = fl
    tables = []
    for j in range(6):
        tables.append({mask: (by_bits.get(mask, (0,) * 6))[j] for mask in range(512)})
    return tables


def build_spec_readout_minimized() -> tuple[object, dict]:
    """Specialized-coords readout from minimized shared AND-pool (exact QM).

    The SOP reference on the paper's own coordinates (Fig
    plaquette-layer(c)): same pipeline as build_trace_readout_minimized
    over _spec_flag_tables, proven against spec_readout_spec. Lets the
    factorization below be compared on both coordinate choices instead of
    assuming the specialized basis minimizes better (it barely does at
    SOP level: 904 vs 922 ANF terms).
    """
    from .minimize import greedy_multioutput_cover
    cover = greedy_multioutput_cover(_spec_flag_tables(), 9)
    polys = []
    for out in cover:
        cur = []
        for (value, care) in out:
            lits = []
            for i in range(9):
                if (care >> i) & 1:
                    lits.append((i, (value >> i) & 1))
            cur.append(lits)
        polys.append(cur)
    return _emit_flag_circuit(polys, "spec minimized QM+greedy")


def spec_readout_spec():
    """TaskSpec: specialized bits in, same six flags out (invalid -> zero)."""
    from .kickmix import TaskSpec
    tab = spec_flag_table()
    by_bits = {}
    for t, (_v, fl) in tab.items():
        bits = spec_coord_bits(t)
        by_bits[tuple(bits)] = fl

    def _perm(bits: dict[int, int]) -> dict[int, int]:
        out = dict(bits)
        key = tuple(bits.get(i, 0) & 1 for i in range(9))
        fl = by_bits.get(key, (0, 0, 0, 0, 0, 0))
        for j in range(6):
            out[9 + j] = fl[j]
        return out

    _, info = build_spec_readout_minimized()
    return TaskSpec(num_qubits=15 + info["temps"], inputs=tuple(range(9)),
                    outputs=tuple(range(9, 15)),
                    ancillas=tuple(range(15, 15 + info["temps"])),
                    target_perm=_perm)


def _emit_factored_circuit(cover: list[list[tuple[int, int]]], kind: str) -> tuple[object, dict]:
    """Shared layout for factored-network readout circuits (one home).

    Inputs 0..8, flag outs 9..14, one temp per live network node from 15
    up; structural cost ccx == 2*nnodes (compute + mirror per node).
    Proven against the same TaskSpec as the SOP builders: the spec sizes
    the interface (inputs/outs/first temps), while every temp's
    free_marker is dirty-checked in simulation, so extra temps cannot
    escape verification.
    """
    from .kickmix import Builder
    from .minimize import emit_factored, factor_and_network
    net = factor_and_network(cover, 9)
    nnodes = len(net["nodes"])
    outs = list(range(9, 15))
    temps = list(range(15, 15 + nnodes))
    b = Builder(15 + nnodes)
    for q in temps:
        b.alloc(q)
    n_ccx = [0]
    emit_factored(b, n_ccx, net, temps, outs)
    for q in temps:
        b.free(q)
    nterms = sum(len(o) for o in cover)
    return b, {"ccx": n_ccx[0], "inputs": tuple(range(9)),
               "outputs": tuple(outs), "temps": nnodes,
               "nodes": nnodes, "terms": nterms, "moves": net["moves"],
               "kind": kind}


def build_trace_readout_factored() -> tuple[object, dict]:
    """Normal-coords readout from the factored AND-network (multi-level).

    Same minimized cover as build_trace_readout_minimized, but shared
    AND-subproducts across all six outputs are computed once (greedy
    CSE in minimize.factor_and_network, genuinely multi-level: nodes of
    nodes) instead of recomputing every cube's AND-chain. Same spec,
    proven exhaustive downstream; structural cost 2T per live node.
    """
    from .minimize import greedy_multioutput_cover
    cover = greedy_multioutput_cover(_flag_tables(), 9)
    return _emit_factored_circuit(cover, "factored CSE")


def build_spec_readout_factored() -> tuple[object, dict]:
    """Specialized-coords readout from the factored AND-network.

    Same as build_trace_readout_factored on the paper's coordinates —
    the number to hold against the paper's hand-optimized 20T (same
    representation, same spec). Whatever gap remains after measured
    factorization is the paper's unrecoverable hand structure, stated
    as such, never closed by invention.
    """
    from .minimize import greedy_multioutput_cover
    cover = greedy_multioutput_cover(_spec_flag_tables(), 9)
    return _emit_factored_circuit(cover, "spec factored CSE")


def trace_readout_spec():
    """TaskSpec: fresh flag qubits get the table flags at valid encodings
    (all zero at invalid ones — the ANF definition, not a silent choice)."""
    from .kickmix import TaskSpec
    tab = trace_flag_table()
    by_bits = {coord_bits(g): fl for g, (_v, fl) in tab.items()}

    def _perm(bits: dict[int, int]) -> dict[int, int]:
        out = dict(bits)
        key = tuple(bits.get(i, 0) & 1 for i in range(9))
        fl = by_bits.get(key, (0, 0, 0, 0, 0, 0))
        for j in range(6):
            out[9 + j] = fl[j]
        return out

    _, info = build_trace_readout_circuit()
    return TaskSpec(num_qubits=15 + info["temps"], inputs=tuple(range(9)),
                    outputs=tuple(range(9, 15)),
                    ancillas=tuple(range(15, 15 + info["temps"])),
                    target_perm=_perm)


def spec_coord_bits(t: tuple[int, int, int, int, int]) -> tuple[int, ...]:
    """Specialized coords (tP, Q, R, M, upsilon) as 9 bits (paper Fig
    plaquette-layer(c) layout: rows (M, upsilon), columns (Q, R)).

    Same 2-bit-per-Z3 encoding as coord_bits; invalid Z3 bit-pairs read
    all-zero flags by definition.
    """
    tP, Q, R, M, ups = t
    for name, v, mod in (("tP", tP, 3), ("Q", Q, 3), ("R", R, 3),
                         ("M", M, 4), ("ups", ups, 2)):
        if isinstance(v, bool) or not isinstance(v, int) or not (0 <= v < mod):
            raise ValueError(f"{name} out of range: refusing {(t)!r}")
    bits = []
    for v in (tP, Q, R):
        bits += [1 if v == 1 else 0, 1 if v == 2 else 0]
    bits += [(M >> 1) & 1, M & 1, ups]
    return tuple(bits)


def spec_flag_table() -> dict:
    """All 216 specialized coords -> (trace value, six flags).

    Keyed by specialized representation (injective — machine-checked);
    values computed from the true representation, never transcribed.
    This is the table the paper's optimized readout acts on (Fig
    plaquette-layer(c)); the ANF below is the handoff to optimization.
    """
    out = {}
    for g in all_elements():
        t = to_internal(g)
        v = trace_class(g)
        fl = tuple(1 if x == v else 0 for x in FLAG_VALUES)
        if t in out:
            raise ValueError(f"specialized collision at {t!r}: refusing")
        out[t] = (v, fl)
    return out


def spec_flag_anf() -> dict[int, frozenset]:
    """Each flag as ANF over the 9 specialized-coordinate bits."""
    tab = spec_flag_table()
    full = {spec_coord_bits(t): fl for t, (_v, fl) in tab.items()}
    out = {}
    for j in range(6):
        out[j] = _anf_from_table({bits: fl[j] for bits, fl in full.items()})
    return out
