"""Reversible F2^4 arithmetic as kickmix programs (naive Fermat baseline).

Implements the paper's Extended-Data F1 family (Fermat chain: x^14 by free
squarings + field multiplications) honestly: schoolbook multiply with fused
reduction, CNOT-only squaring, and the x^14 chain with full uncomputation.
The paper's other ideation families: F2 full QROM (1-of-16 lookup) is now
build_f24_qrom_inv (4 pruned bit-planes, measured 204T — worst at toy
scale, as expected for a lookup of 16 entries); F3 tower field F(2^2)^2
is tower.py (basis change + norm + MBU packing as circuits, measured
below). The 5T/10Q endpoint came through MBU packing + tower restructuring
plus shared kernels our toy widths do not need; this module reproduces the
paper's STARTING point families (their trial Fermat ran 66T/20Q; ours
measures 88T naive, 48T MBU, 27T Kara, 204T QROM, 60T tower plain, 30T
tower MBU), measured, not narrated — the gaps between them ARE the family
comparison the search loop decides on.

Reduction (X^4 = X + 1): product bit k lands on R(k):
  R(0)={0} R(1)={1} R(2)={2} R(3)={3} R(4)={0,1} R(5)={1,2} R(6)={2,3}.
Squaring (a0+a1 X+a2 X^2+a3 X^3)^2 with X^4=X+1, X^6=X^3+X^2:
  out = (a0^a2) + (a2)X + (a1^a3)X^2 + (a3)X^3 — CNOTs only.
"""

from __future__ import annotations

from .gf import F24
from .kickmix import Builder
from .specs import build_truth_table_spec

# product-bit k of a*b lands on these output positions (fused reduction)
REDUCE = {0: (0,), 1: (1,), 2: (2,), 3: (3,), 4: (0, 1), 5: (1, 2), 6: (2, 3)}


def emit_mul(b: Builder, a: list[int], c: list[int], out: list[int]) -> int:
    """Schoolbook multiply into zeroed out[]; returns Toffoli count (22)."""
    if not (len(a) == len(c) == len(out) == 4):
        raise ValueError("F24 words are 4 qubits: refusing")
    n = 0
    for i in range(4):
        for j in range(4):
            for t in REDUCE[i + j]:
                b.ccx(a[i], c[j], out[t])
                n += 1
    return n


def emit_square(b: Builder, a: list[int], out: list[int]) -> None:
    """CNOT-only squaring into zeroed out[] (paper's DISCOVERY line)."""
    if not (len(a) == len(out) == 4):
        raise ValueError("F24 words are 4 qubits: refusing")
    a0, a1, a2, a3 = a
    o0, o1, o2, o3 = out
    b.cx(a0, o0)
    b.cx(a2, o0)
    b.cx(a2, o1)
    b.cx(a1, o2)
    b.cx(a3, o2)
    b.cx(a3, o3)


def emit_square_inverse(b: Builder, a: list[int], out: list[int]) -> None:
    """Uncompute a square (CNOTs are self-inverse; re-emit in reverse)."""
    if not (len(a) == len(out) == 4):
        raise ValueError("F24 words are 4 qubits: refusing")
    a0, a1, a2, a3 = a
    o0, o1, o2, o3 = out
    b.cx(a3, o3)
    b.cx(a3, o2)
    b.cx(a1, o2)
    b.cx(a2, o1)
    b.cx(a2, o0)
    b.cx(a0, o0)


def emit_mbu_mul_accumulate(b: Builder, a: list[int], c: list[int],
                            out: list[int], pool: list[int]) -> int:
    """Multiply-accumulate with per-AND measured uncompute (Methods Eq 12).

    Each AND goes into a FRESH pool ancilla p (which holds exactly one
    product a_i*c_j of retained controls), fans out into out[] via CNOTs,
    then is released by hmr + a CZ(a_i, c_j) correction conditioned on the
    record. No uncompute pass: T = 22 per multiply, pool ancillas return
    via measurement (consumed, never freed). out[] must be retained
    downstream (final answer or later-uncomputed temp) — never already
    holding a value that must survive, since accumulation XORs into it.
    """
    if not (len(a) == len(c) == len(out) == 4):
        raise ValueError("F24 words are 4 qubits: refusing")
    if len(pool) < 1:
        raise ValueError("pool needs >= 1 ancilla: refusing")
    for q in pool:
        if q < 0 or q >= b.num_qubits:
            raise ValueError(f"pool qubit {q} out of range: refusing")
    k = 0
    n_ccx = 0
    for i in range(4):
        for j in range(4):
            # cycle the pool: hmr releases each ancilla, re-alloc resets it.
            # A 1-ancilla pool is sound (records stay distinct per hmr).
            # One AND per (i,j) with CNOT fan-out (vs 22 fused ANDs in
            # emit_mul): fewer Toffolis, one pool ancilla instead of a word.
            p = pool[k % len(pool)]
            k += 1
            b.alloc(p)
            b.ccx(a[i], c[j], p)
            n_ccx += 1
            for t in REDUCE[i + j]:
                b.cx(p, out[t])
            r = b.hmr(p)
            b.cz(a[i], c[j], cond=(r,))
    return n_ccx


def build_mbu_inverter(pool: list[int] | None = None, num_qubits: int = 29) -> tuple[Builder, dict]:
    """x^14 chain with MBU-packed multiplies.

    mult1 (x8*x4 -> t) is computed AND uncomputed (re-accumulate, 22+22T);
    mult2 (t*x2 -> h) accumulates into the answer (22T, no uncompute).
    Default pool is ONE recycled ancilla (measure releases it); pass more
    to trade qubits for parallelism. No u-word: mult2 goes straight into h.
    """
    if pool is None:
        pool = [28]
    b = Builder(num_qubits)
    x = [0, 1, 2, 3]
    h = [4, 5, 6, 7]
    x2 = [8, 9, 10, 11]
    x4 = [12, 13, 14, 15]
    x8 = [16, 17, 18, 19]
    t = [20, 21, 22, 23]
    for q in x2 + x4 + x8 + t:
        b.alloc(q)
    n_ccx = 0
    emit_square(b, x, x2)
    emit_square(b, x2, x4)
    emit_square(b, x4, x8)
    n_ccx += emit_mbu_mul_accumulate(b, x8, x4, t, pool)   # 22T
    n_ccx += emit_mbu_mul_accumulate(b, t, x2, h, pool)    # 22T into answer
    n_ccx += emit_mbu_mul_accumulate(b, x8, x4, t, pool)   # 22T re-accumulate clears t
    emit_square_inverse(b, x4, x8)
    emit_square_inverse(b, x2, x4)
    emit_square_inverse(b, x, x2)
    for q in x2 + x4 + x8 + t:
        b.free(q)
    regs = {"x": x, "h": h, "ancillas": x2 + x4 + x8 + t,
            "pool": pool, "ccx": n_ccx}
    return b, regs


# Karatsuba F24 multiply (9 ANDs): three 2-bit Karatsuba products
#   p0 = k2(aL,bL), p2 = k2(aH,bH), s1 = k2(aL^aH, bL^bH),
# each k2(u,v) = (u0&v0, (u0^u1)&(v0^v1)^u0&v0^u1&v1, u1&v1).
# Hand-derived AND -> output map (PROPOSED here, PROVED by
# test_kara_map_classical_exhaustive over all 256 pairs below):
#   ANDs: A0=a0&b0 A1=a1&b1 A2=p&q B0=a2&b2 B1=a3&b3 B2=r&s
#         C0=S0&T0 C1=S1&T1 C2=U&V
#   XOR-temps: p=a0^a1 q=b0^b1 r=a2^a3 s=b2^b3 S0=a0^a2 S1=a1^a3
#              T0=b0^b2 T1=b1^b3 U=S0^S1 V=T0^T1
KARA_TEMPS = {
    "p": ("a", (0, 1)), "q": ("b", (0, 1)),
    "r": ("a", (2, 3)), "s": ("b", (2, 3)),
    "S0": ("a", (0, 2)), "S1": ("a", (1, 3)),
    "T0": ("b", (0, 2)), "T1": ("b", (1, 3)),
    "U": ("a", (0, 1, 2, 3)), "V": ("b", (0, 1, 2, 3)),
}
KARA_ANDS = [
    ("A0", ("a", 0), ("b", 0)),
    ("A1", ("a", 1), ("b", 1)),
    ("A2", "p", "q"),
    ("B0", ("a", 2), ("b", 2)),
    ("B1", ("a", 3), ("b", 3)),
    ("B2", "r", "s"),
    ("C0", "S0", "T0"),
    ("C1", "S1", "T1"),
    ("C2", "U", "V"),
]
KARA_OUT = {
    # MACHINE-DERIVED (GF(2) elimination over all 256 pairs; const 0):
    # hand transcription was refuted by the exhaustive check, so the map
    # below is the solver's output, proved by test_kara_map_classical.
    0: ("A0", "A1", "B0", "B1", "C1"),
    1: ("A0", "A2", "B2", "C1"),
    2: ("A0", "A1", "B2", "C0"),
    3: ("A0", "A1", "A2", "B0", "B2", "C0", "C1", "C2"),
}


def kara_product(a: int, b: int) -> int:
    """Pure-classical Karatsuba map (for the exhaustive proof, not circuits)."""
    ab = {i: (a >> i) & 1 for i in range(4)}
    bb = {i: (b >> i) & 1 for i in range(4)}

    def word(src: str, idxs) -> int:
        d = {"a": ab, "b": bb}[src]
        v = 0
        for i in idxs:
            v ^= d[i]
        return v

    vals: dict[str, int] = {}
    for name, spec in KARA_TEMPS.items():
        src, idxs = spec
        vals[name] = word(src, idxs)
    for name, l, r in KARA_ANDS:
        lv = vals[l] if isinstance(l, str) else (ab if l[0] == "a" else bb)[l[1]]
        rv = vals[r] if isinstance(r, str) else (ab if r[0] == "a" else bb)[r[1]]
        vals[name] = lv & rv
    out = 0
    for j, names in KARA_OUT.items():
        bit = 0
        for name in names:
            bit ^= vals[name]
        out |= bit << j
    return out


def emit_kara_mul_accumulate(b: Builder, a: list[int], c: list[int],
                             out: list[int], pool: list[int],
                             temps: list[int]) -> int:
    """9-AND Karatsuba multiply-accumulate with MBU-cleared ANDs.

    XOR-temps materialized via CNOTs (0T, uncomputed after); each AND into
    a cycled pool ancilla, fanned out per KARA_OUT, released by hmr + a
    CZ correction on its two controls. Needs 10 temp bits + >= 1 pool bit.
    """
    if not (len(a) == len(c) == len(out) == 4):
        raise ValueError("F24 words are 4 qubits: refusing")
    if len(temps) < 10:
        raise ValueError(f"need 10 temp bits, got {len(temps)}: refusing")
    if len(pool) < 1:
        raise ValueError("pool needs >= 1 ancilla: refusing")
    for q in list(temps) + list(pool):
        if q < 0 or q >= b.num_qubits:
            raise ValueError(f"qubit {q} out of range: refusing")
    names = list(KARA_TEMPS)
    tq = {name: temps[i] for i, name in enumerate(names)}

    def bit_of(ref, word: list[int]) -> int:
        if isinstance(ref, str):
            return tq[ref]
        src, i = ref
        return (a if src == "a" else c)[i]

    for name in names:  # materialize XOR-temps
        src, idxs = KARA_TEMPS[name]
        w = a if src == "a" else c
        for i in idxs:
            b.cx(w[i], tq[name])
    n_ccx = 0
    k = 0
    for name, l, r in KARA_ANDS:
        p = pool[k % len(pool)]
        k += 1
        b.alloc(p)
        b.ccx(bit_of(l, a), bit_of(r, c), p)
        n_ccx += 1
        for j, members in KARA_OUT.items():
            if name in members:
                b.cx(p, out[j])
        m = b.hmr(p)
        b.cz(bit_of(l, a), bit_of(r, c), cond=(m,))
    for name in reversed(names):  # uncompute XOR-temps
        src, idxs = KARA_TEMPS[name]
        w = a if src == "a" else c
        for i in reversed(idxs):
            b.cx(w[i], tq[name])
    return n_ccx


def build_kara_inverter(tbase: int = 24, num_qubits: int = 35) -> tuple[Builder, dict]:
    """x^14 chain over 9-AND Karatsuba multiplies (mult1 18T + mult2 9T)."""
    b = Builder(num_qubits)
    x = [0, 1, 2, 3]
    h = [4, 5, 6, 7]
    x2 = [8, 9, 10, 11]
    x4 = [12, 13, 14, 15]
    x8 = [16, 17, 18, 19]
    t = [20, 21, 22, 23]
    temps = list(range(tbase, tbase + 10))
    pool = [tbase + 10]
    for q in x2 + x4 + x8 + t + temps:
        b.alloc(q)
    n_ccx = 0
    emit_square(b, x, x2)
    emit_square(b, x2, x4)
    emit_square(b, x4, x8)
    n_ccx += emit_kara_mul_accumulate(b, x8, x4, t, pool, temps)
    n_ccx += emit_kara_mul_accumulate(b, t, x2, h, pool, temps)
    n_ccx += emit_kara_mul_accumulate(b, x8, x4, t, pool, temps)
    emit_square_inverse(b, x4, x8)
    emit_square_inverse(b, x2, x4)
    emit_square_inverse(b, x, x2)
    for q in x2 + x4 + x8 + t + temps:
        b.free(q)
    return b, {"x": x, "h": h, "ancillas": x2 + x4 + x8 + t + temps,
               "pool": pool, "ccx": n_ccx}


def build_inverter(num_qubits: int = 28) -> tuple[Builder, dict]:
    """Full |x>|h> -> |x>|h + x^-1> with 0^-1 = 0. Returns (builder, regs).

    Layout: x[0:4] input, h[4:8] target, x2[8:12], x4[12:16], x8[16:20],
    t[20:24] (x8*x4), u[24:28] (t*x2 = x^14). All temps uncomputed.
    """
    b = Builder(num_qubits)
    x = [0, 1, 2, 3]
    h = [4, 5, 6, 7]
    x2 = [8, 9, 10, 11]
    x4 = [12, 13, 14, 15]
    x8 = [16, 17, 18, 19]
    t = [20, 21, 22, 23]
    u = [24, 25, 26, 27]
    for q in x2 + x4 + x8 + t + u:
        b.alloc(q)
    n_ccx = 0
    emit_square(b, x, x2)
    emit_square(b, x2, x4)
    emit_square(b, x4, x8)
    n_ccx += emit_mul(b, x8, x4, t)   # t = x^12
    n_ccx += emit_mul(b, t, x2, u)    # u = x^14 = x^-1
    for i in range(4):
        b.cx(u[i], h[i])
    n_ccx += emit_mul(b, t, x2, u)    # uncompute u
    n_ccx += emit_mul(b, x8, x4, t)   # uncompute t
    emit_square_inverse(b, x4, x8)
    emit_square_inverse(b, x2, x4)
    emit_square_inverse(b, x, x2)
    for q in x2 + x4 + x8 + t + u:
        b.free(q)
    regs = {"x": x, "h": h, "ancillas": x2 + x4 + x8 + t + u, "ccx": n_ccx}
    return b, regs


def build_f24_inv_spec():
    """Truth-table spec for the Extended-Data task (4 in + 4 out <= 8)."""
    from .gf import f24_inversion_table
    table = f24_inversion_table()
    spec_table = {}
    for x in range(16):
        inv = table[x]
        spec_table[f"{x:04b}"] = {
            "out": {str(4 + i): (inv >> i) & 1 for i in range(4)}
        }
    # inputs are x[0..3] (LSB-first vs table MSB-first: key bit i -> qubit i)
    remapped = {}
    for x in range(16):
        key = "".join(str((x >> i) & 1) for i in range(4))
        remapped[key] = spec_table[f"{x:04b}"]
    return build_truth_table_spec(8, [0, 1, 2, 3], [4, 5, 6, 7], remapped)


def f24_qrom_planes() -> list[list[int]]:
    """F2 family data: 4 bit-planes of the inversion table (16 entries each).

    Plane j lists bit j of x^-1 over x = 0..15 (address = index, LSB-first
    qubits — the pruned_qrom_spec convention). The F2 full-lookup family
    is one pruned QROM per plane (4 independent loaders, proven separately).
    """
    from .gf import f24_inversion_table
    table = f24_inversion_table()
    return [[(table[x] >> j) & 1 for x in range(16)] for j in range(4)]


def build_f24_qrom_inv() -> tuple[list, dict]:
    """F2 full-lookup inverter: 4 pruned-QROM bit-plane loaders.

    Returns ([(builder, regs)] per plane, report with per-plane and total
    Toffolis). Each plane proves separately against its table (16/16);
    composition by planes is exact by construction (bit j of the answer
    comes from plane j — asserted live per address in tests).
    """
    from .qrom import build_pruned_qrom
    planes = f24_qrom_planes()
    builds = [build_pruned_qrom(plane, True) for plane in planes]
    total = sum(regs["ccx"] for _, regs in builds)
    return builds, {"per_plane_ccx": [regs["ccx"] for _, regs in builds],
                    "total_ccx": total, "entries": 16}


def family_comparison() -> dict:
    """Measured Toffoli counts across the F24 ideation families (F1/F2/F3).

    F1 Fermat chain (naive schoolbook start), MBU-packed Fermat, Karatsuba
    Fermat, F2 full QROM (4 bit-planes), F3 tower plain + MBU-packed. The
    paper's 5T/10Q endpoint came through tower restructuring + MBU packing
    + shared kernels; this table reports our measured numbers with no
    winner declared beyond the counts (fitness decides in the search loop,
    never prose).
    """
    b0, _ = build_inverter()
    b1, _ = build_mbu_inverter()
    b2, _ = build_kara_inverter()
    _, qr = build_f24_qrom_inv()
    from .tower import build_tower_inverter
    bt, _ = build_tower_inverter("plain")
    bm, _ = build_tower_inverter("mbu")
    return {"fermat_naive": b0.toffoli_count(),
            "fermat_mbu": b1.toffoli_count(),
            "fermat_kara": b2.toffoli_count(),
            "qrom_full_lookup": qr["total_ccx"],
            "tower_plain": bt.toffoli_count(),
            "tower_mbu": bm.toffoli_count()}
