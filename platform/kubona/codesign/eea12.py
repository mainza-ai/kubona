"""Fixed-schedule half-degree EEA circuit (Phase 3 B2, OUR schedule).

Computes A,B with A*E + B*O = 1 for half-degree inputs (E,O from the
sqrt-split of a separable locator, degree <= 17) via 36 single-term elimination blocks (fixed schedule; 35 = tight
telescoping bound on terms, +1 scheduling block verified empirically --
60/60 converge, flag guards residuals; the SI's "17 elimination blocks"
covers only low-term instances, see F1 in circuit-codesign-followups).
OUR schedule (the SI specifies only "17 elimination blocks" with
quotients parked in cleared words, main.tex:1792-1794 — no block
internals): each block finds both degrees via zero-test ladders,
conditionally swaps to order the pair, eliminates one top term
(quotient via the tower inverter), and trails the quotient/shift/swap
flag in per-block persistent wires.

Deviations from the paper, all stated (see also the module docstring
design notes in the report):
- Dedicated trail wires instead of packing quotients into cleared words
  (provably correct, no aliasing analysis needed).
- Schoolbook mults + tower inverter instead of the paper's leaner
  primitives — our honest T-count lands ~12x above the paper's 44,000T
  per direction (their MBU/Karatsuba mults and leaner blocks are queued
  follow-ups, not hidden gaps).
- Barrel-shifted elimination (runtime shift via 5-stage binary barrel).
- FORWARD-ONLY pass, no reverse pass (SI reverses to reclaim words,
  main.tex:1789). Rationale: reversal saves ~1.3k wires at ~2x T-cost;
  with schoolbook mults dominating T, forward-only wins on TQ AND on
  auditability; reversal is a queued Q-optimization, not a correctness
  gap. Working banks and trail persist as declared outputs.

The failure flag is load-bearing: inputs needing more than 35 terms (none valid;
bound is tight), or non-coprime (E,O) (which the math layer refuses), set FLAG instead of
producing a silent wrong (A,B) — bounded error with explicit refusal,
paper-consistent (main.tex:2049,1653).
"""

from __future__ import annotations

from .kickmix import Builder

#: Schedule: 36 single-term elimination blocks (fixed, every input).
#: The SI specifies "17 elimination blocks" but 17 covers only harvested
#: locators needing <= 17 terms; uniform-random valid inputs need up to 35
#: (measured max over 1500 instances; telescoping bound gives <= 35, and the
#: circuit empirically needs one scheduling block beyond the term count --
#: verified 60/60 converge at 36, flag guards residuals). We run 36 for
#: full-radius completeness (production correctness over paper
#: count-fidelity; see circuit-codesign-followups F1).
N_BLOCKS = 36
#: Working banks hold 36 words (elimination writes s+j <= 34; high words
#: provably stay zero — writes there add zero-valued products — asserted
#: zero in-test; swaps cover the low 18 only).
NW = 36
#: Input/output degree bound (E,O,A,B all degree <= 17 -> 18 words each).
NW_IO = 18

#: Per-block trail (persists; everything the block computes that outlives
#: it). Nothing in BT is ever cleaned (forward-only pass); shared scratch
#: (ladder/tower/zero-test/barrel/PE/QE) is mirror-clean per use.
#: Ladders write DX/DY/LX/LY/XZ/YZ; then LT + pre-subtract share;
#: degree wires swap; post-subtract gives S; tower+q-mult give QT.
_BT_DX = (0, 5)        # deg X (binary)
_BT_DY = (5, 10)       # deg Y (binary)
_BT_LX = (10, 22)      # lc X (12)
_BT_LY = (22, 34)      # lc Y (12)
_BT_XZ = (34, 35)      # X == 0 flag
_BT_YZ = (35, 36)      # Y == 0 flag
_BT_LT = (36, 37)      # swap flag
_BT_S1 = (37, 42)      # pre-swap subtract (ignored)
_BT_B1 = (42, 43)      # pre-swap borrow-out (-> LT)
_BT_T1 = (43, 63)      # pre-subtract temps (20)
_BT_S = (63, 68)       # shift (5, post-swap)
_BT_B2 = (68, 69)      # post-swap borrow-out (always 0, asserted)
_BT_T2 = (69, 89)      # post-subtract temps (20)
_BT_QT = (89, 101)     # quotient term (12)
_BT_CM = (101, 107)    # comparator scratch (6)
N_BT = 107
#: Ladder scratch (shared across ladders, mirror-clean per ladder).
_L_NZ = (0, 18)
_L_HI = (18, 35)
_L_TK = (35, 52)
_L_DX = (52, 57)
_L_LC = (57, 69)
N_LT = 69


def _check_word(w: list[int], n: int, name: str, nqubits: int) -> None:
    if not isinstance(w, list) or len(w) != n or any(not isinstance(q, int) or q < 0 or q >= nqubits for q in w):
        raise ValueError(f"{name} must be {n} in-range qubit ids: refusing {w!r}")


def _emit_ladder(b: Builder, r: list[int], dx: list[int], lc: list[int],
                 xz: list[int], t: list[int], zt: list[int]) -> int:
    """Degree ladder on a low-18-word bank, self-cleaning scratch.

    Zero-tests every word, priority-encodes the top nonzero via a
    higher-chain + one-hot takes, MUXes the binary degree, the leading
    word, and the all-zero flag into caller wires (dx/lc/xz, which
    persist). Scratch (nz/hi/take) cleaned inline by reverse replay
    before return (R untouched throughout). Returns the Toffoli count.
    """
    from .packing import emit_zero_test
    if not isinstance(b, Builder):
        raise ValueError("b must be a Builder: refusing")
    if not isinstance(r, list) or len(r) != 12 * NW_IO or any(not isinstance(q, int) or q < 0 or q >= b.num_qubits for q in r):
        raise ValueError(f"r must be {12 * NW_IO} in-range qubit ids: refusing")
    _check_word(dx, 5, "dx", b.num_qubits)
    _check_word(lc, 12, "lc", b.num_qubits)
    _check_word(xz, 1, "xz", b.num_qubits)
    _check_word(t, N_LT, "t", b.num_qubits)
    _check_word(zt, 10, "zt", b.num_qubits)
    n = 0
    nz = [t[i] for i in range(*_L_NZ)]
    hi = [t[i] for i in range(*_L_HI)]
    tk = [t[i] for i in range(*_L_TK)]
    seg: list = []

    def rec(op: tuple) -> None:
        seg.append(op)

    for k in range(NW_IO):
        n += emit_zero_test(b, r[12 * k:12 * (k + 1)], nz[k], zt)
        rec(("zt", list(r[12 * k:12 * (k + 1)]), nz[k], list(zt)))
        b.x(nz[k])
        rec(("x", nz[k]))
    b.cx(nz[NW_IO - 1], hi[NW_IO - 2])
    rec(("cx", nz[NW_IO - 1], hi[NW_IO - 2]))
    for k in range(NW_IO - 3, -1, -1):
        b.cx(hi[k + 1], hi[k])
        rec(("cx", hi[k + 1], hi[k]))
        b.cx(nz[k + 1], hi[k])
        rec(("cx", nz[k + 1], hi[k]))
        b.ccx(hi[k + 1], nz[k + 1], hi[k])
        rec(("ccx", hi[k + 1], nz[k + 1], hi[k]))
        n += 1
    for k in range(NW_IO - 1):
        b.x(hi[k])
        rec(("x", hi[k]))
        b.ccx(nz[k], hi[k], tk[k])
        rec(("ccx", nz[k], hi[k], tk[k]))
        n += 1
        b.x(hi[k])
        rec(("x", hi[k]))
    for k in range(NW_IO):
        take = tk[k] if k < NW_IO - 1 else nz[NW_IO - 1]
        for i in range(5):
            if (k >> i) & 1:
                b.cx(take, dx[i])
                # NOT recorded: degree/word outputs persist (caller BT wires);
                # only scratch (nz/hi/take) is mirror-cleaned below.
        for tt in range(12):
            b.ccx(take, r[12 * k + tt], lc[tt])
            n += 1
    b.x(xz[0])
    for k in range(NW_IO - 1):
        b.cx(tk[k], xz[0])
    b.cx(nz[NW_IO - 1], xz[0])
    # xz = 1 ^ (OR takes) = 1 iff all-zero. One-hot-or-zero takes make
    # XOR equal OR here (at most one take is 1).
    n += _unemit(seg, b)
    return n


def _unemit(seg: list, b: Builder) -> int:
    """Replay recorded CNOT/X/CCX/zero-test ops in reverse. Returns T count
    for the replayed CCX/zero-test ops."""
    from .packing import emit_zero_test
    n = 0
    for op in reversed(seg):
        if op[0] == "cx":
            _, a, c = op
            b.cx(a, c)
        elif op[0] == "x":
            (_, a) = op
            b.x(a)
        elif op[0] == "ccx":
            _, a, c, o = op
            b.ccx(a, c, o)
            n += 1
        elif op[0] == "zt":
            _, g, flag, zt = op
            n += emit_zero_test(b, list(g), flag, list(zt))
        else:
            raise ValueError(f"unreplayable op {op[0]}: refusing")
    return n


def _emit_fredkins(b: Builder, f: int, xs: list[int], ys: list[int]) -> int:
    """Conditional bank swap (Fredkin per wire: CX + CCX + CX)."""
    if not isinstance(b, Builder):
        raise ValueError("b must be a Builder: refusing")
    if len(xs) != len(ys):
        raise ValueError("swap banks must match: refusing")
    n = 0
    for a, c in zip(xs, ys):
        b.cx(c, a)
        b.ccx(f, a, c)
        n += 1
        b.cx(c, a)
    return n


def eea12_layout(base: int) -> dict:
    """Single half-degree EEA layout (offsets from base)."""
    from .tower12 import N_TOWER_TEMPS

    o = [base]

    def take(n: int) -> list[int]:
        r = list(range(o[0], o[0] + n))
        o[0] += n
        return r

    E = take(12 * NW_IO)
    O = take(12 * NW_IO)
    nin = o[0] - base
    X = take(12 * NW)
    Y = take(12 * NW)
    U0 = take(12 * NW)
    U1 = take(12 * NW)
    V0 = take(12 * NW)
    V1 = take(12 * NW)
    BT = [take(N_BT) for _ in range(N_BLOCKS)]
    LT = take(N_LT)
    ZT = take(10)
    TOWER = take(N_TOWER_TEMPS)
    PE = take(12 * NW_IO)
    QE = take(12 * NW)
    BB = [take(12 * NW) for _ in range(5)]
    INV = take(12)
    BRE = take(1)
    MBP = take(1)
    ZTBIG = take(217)
    ntemps = o[0] - base - nin
    A = take(12 * NW_IO)
    B = take(12 * NW_IO)
    FLAG = take(1)
    if nin != 2 * 12 * NW_IO:
        raise ValueError("input layout arithmetic drifted: refusing")
    expect = (6 * 12 * NW + N_BLOCKS * N_BT + N_LT + 10 + N_TOWER_TEMPS
              + 12 * NW_IO + 12 * NW + 5 * 12 * NW + 12 + 1 + 1 + 217)
    if ntemps != expect:
        raise ValueError("temp layout arithmetic drifted: refusing")
    # NOTE: MBP (MBU pool) is self-managed like ZT (alloc'd per AND,
    # consumed by hmr) — pre-allocating it would double-alloc. Reserved
    # here for disjointness only, never alloc'd by the builder.
    allocs = [q for grp in ([LT, TOWER, PE, QE, INV, BRE] + BB) for q in grp]
    return {"E": E, "O": O, "X": X, "Y": Y, "U0": U0, "U1": U1,
            "V0": V0, "V1": V1, "BT": BT, "LT": LT, "ZT": ZT,
            "TOWER": TOWER, "PE": PE, "QE": QE, "BB": BB, "INV": INV, "BRE": BRE, "MBP": MBP,
            "ZTBIG": ZTBIG, "A": A, "B": B, "FLAG": FLAG, "nin": nin,
            "ntemps": ntemps, "nqubits": o[0] - base, "allocs": allocs}


def _emit_sub5(b: Builder, a: list[int], c: list[int], out: list[int],
               bout: list[int], t: list[int]) -> int:
    """5-bit subtractor out = a - c with borrow-out (ripple, fixed ops).

    All temps persist dirty in t (forward-only pass — no cleanup needed).
    Needs 20 temp wires. Returns the Toffoli count.
    """
    if not isinstance(b, Builder):
        raise ValueError("b must be a Builder: refusing")
    for name, w in (("a", a), ("c", c), ("out", out)):
        _check_word(w, 5, name, b.num_qubits)
    _check_word(bout, 1, "bout", b.num_qubits)
    _check_word(t, 20, "t", b.num_qubits)
    n = 0
    # bin_0 = 0 (const). Per bit: diff + borrow-out into fresh temp.
    # bout_i = (~a_i & c_i) | (~a_i & bin_i) | (c_i & bin_i), 3 ANDs.
    bt: list = [None] * 5  # borrow wires (bin_0 = const 0)
    for i in range(5):
        b.cx(a[i], out[i])
        b.cx(c[i], out[i])
        if i > 0:
            b.cx(bt[i - 1], out[i])
        t0, t1, t2, bo = t[4 * i:4 * i + 4]
        b.x(a[i])
        b.ccx(a[i], c[i], t0)
        n += 1
        b.x(a[i])
        if i > 0:
            b.x(a[i])
            b.ccx(a[i], bt[i - 1], t1)
            n += 1
            b.x(a[i])
            b.ccx(c[i], bt[i - 1], t2)
            n += 1
            b.cx(t0, bo)
            b.cx(t1, bo)
            b.cx(t2, bo)
        else:
            b.cx(t0, bo)
        bt[i] = bo
    b.cx(bt[4], bout[0])
    return n


def _emit_barrel(b: Builder, p: list[list[int]], q: list[int], s: list[int],
                 tmp: list[int], buf: list[list[int]], reverse: bool = False) -> int:
    """Left-shift P (18 words) by s into Q (36 words, zeroed on entry).

    5 double-buffered stages (fresh buffer per stage output). Forward leaves
    Q holding the shifted product (caller accumulates Q->R, then calls with
    reverse=True to clean). Reverse is the exact mirror; all sources intact
    so temps clean. tmp is 1 shared bit, self-cleaning per bit.
    Buffers are 5x36 words. Returns the Toffoli count.
    """
    if not isinstance(b, Builder):
        raise ValueError("b must be a Builder: refusing")
    if len(p) != 18 or any(len(w) != 12 for w in p):
        raise ValueError("p must be 18 words: refusing")
    _check_word(q, 36 * 12, "q", b.num_qubits)
    _check_word(s, 5, "s", b.num_qubits)
    _check_word(tmp, 1, "tmp", b.num_qubits)
    if len(buf) != 5 or any(len(w) != 432 for w in buf):
        raise ValueError("buf must be 5x432 scratch words: refusing")
    (tm,) = tmp
    n = 0

    def copy_sd(src: list[int], nwords: int, dst: list[int]) -> None:
        for d in range(min(nwords, 36)):
            for tt in range(12):
                b.cx(src[12 * d + tt], dst[12 * d + tt])

    def cond(sub_s: list[int], nwords: int, dst: list[int], i: int) -> None:
        nonlocal n
        step = 1 << i
        for d in range(36):
            for tt in range(12):
                if d < nwords:
                    b.cx(sub_s[12 * d + tt], tm)
                if d >= step and d - step < nwords:
                    b.cx(sub_s[12 * (d - step) + tt], tm)
                b.ccx(s[i], tm, dst[12 * d + tt])
                n += 1
                if d < nwords:
                    b.cx(sub_s[12 * d + tt], tm)
                if d >= step and d - step < nwords:
                    b.cx(sub_s[12 * (d - step) + tt], tm)

    flat_p = [w for word in p for w in word]
    if not reverse:
        # forward: copy P->B0, per stage [copy, cond]; Q gets shifted P
        copy_sd(flat_p, 18, buf[0])
        cond(flat_p, 18, buf[0], 0)
        for i in range(1, 5):
            copy_sd(buf[i - 1], 36, buf[i])
            cond(buf[i - 1], 36, buf[i], i)
        for d in range(36):
            for tt in range(12):
                b.cx(buf[4][12 * d + tt], q[12 * d + tt])
    else:
        # reverse (exact mirror; Q holds shifted product, all sources intact)
        for d in range(36):
            for tt in range(12):
                b.cx(buf[4][12 * d + tt], q[12 * d + tt])
        for i in range(4, -1, -1):
            src = flat_p if i == 0 else buf[i - 1]
            nw = 18 if i == 0 else 36
            cond(src, nw, buf[i], i)
            copy_sd(src, nw, buf[i])
    return n


def _emit_eea_block(b: Builder, n_ccx: list[int], L: dict, blk: int) -> None:
    """One fixed-schedule elimination block (forward-only).

    Ladder both remainders (degrees/leading words into BT) -> LT flag ->
    swap banks+degrees -> shift -> tower inverse -> quotient -> eliminate
    X,U0,V0 via barrel-shifted schoolbook mults. BT trail persists;
    all shared scratch returns clean. No mirror (forward-only pass).
    """
    from .mbu12 import emit_mbu_mul12
    from .tower12 import emit_tower_inv12
    X, Y = L["X"], L["Y"]
    U0, U1, V0, V1 = L["U0"], L["U1"], L["V0"], L["V1"]
    BT = L["BT"][blk]
    LT, ZT = L["LT"], L["ZT"]
    TOWER, PE, QE = L["TOWER"], L["PE"], L["QE"]
    BB = L["BB"]
    MBP = L["MBP"]
    BRE = L["BRE"][0]
    INV = L["INV"]

    def words(bank: list[int], i: int) -> list[int]:
        return bank[12 * i:12 * (i + 1)]

    def btw(rng: tuple[int, int]) -> list[int]:
        return [BT[i] for i in range(*rng)]

    DX, DY = btw(_BT_DX), btw(_BT_DY)
    LX, LY = btw(_BT_LX), btw(_BT_LY)
    XZ, YZ = btw(_BT_XZ), btw(_BT_YZ)
    LTf, S = btw(_BT_LT), btw(_BT_S)
    S1, B1, T1 = btw(_BT_S1), btw(_BT_B1), btw(_BT_T1)
    B2, T2 = btw(_BT_B2), btw(_BT_T2)
    QT = btw(_BT_QT)
    CM = btw(_BT_CM)
    # 1. ladders (scratch self-cleans; degrees/words persist in BT)
    n_ccx[0] += _emit_ladder(b, X[0:216], DX, LX, XZ, LT, ZT)
    n_ccx[0] += _emit_ladder(b, Y[0:216], DY, LY, YZ, LT, ZT)
    # 2. LT = (dx<dy) via pre-subtract borrow, or (Xz and !Yz)
    n_ccx[0] += _emit_sub5(b, DX, DY, S1, B1, T1)
    b.x(YZ[0])
    b.ccx(XZ[0], YZ[0], CM[0])
    n_ccx[0] += 1
    b.x(YZ[0])
    b.cx(B1[0], LTf[0])
    b.cx(CM[0], LTf[0])
    b.ccx(B1[0], CM[0], LTf[0])
    n_ccx[0] += 1
    # 3. swap banks (low 18 words) + degree wires on LT
    n_ccx[0] += _emit_fredkins(b, LTf[0], X[0:216], Y[0:216])
    n_ccx[0] += _emit_fredkins(b, LTf[0], U0[0:216], U1[0:216])
    n_ccx[0] += _emit_fredkins(b, LTf[0], V0[0:216], V1[0:216])
    n_ccx[0] += _emit_fredkins(b, LTf[0], DX + LX + XZ, DY + LY + YZ)
    # 4. shift s = dx-dy (post-swap; dx>=dy or Y all-zero by construction)
    n_ccx[0] += _emit_sub5(b, DX, DY, S, B2, T2)
    # 5. quotient: tower(Ly) then q = Lx * invLy
    n_ccx[0] += emit_tower_inv12(b, LY, INV, TOWER)
    n_ccx[0] += emit_mbu_mul12(b, LX, INV, QT, MBP)
    # 6. eliminate X, U0, V0 (barrel-shifted mults into low+high words)
    for R, Dd in ((X, Y), (U0, U1), (V0, V1)):
        for j in range(18):
            n_ccx[0] += emit_mbu_mul12(b, QT, words(Dd, j), PE[12 * j:12 * (j + 1)], MBP)
        Pw = [[PE[12 * j + tt] for tt in range(12)] for j in range(18)]
        n_ccx[0] += _emit_barrel(b, Pw, QE, S, [BRE], BB)
        for k in range(36):
            for tt in range(12):
                b.cx(QE[12 * k + tt], R[12 * k + tt])
        n_ccx[0] += _emit_barrel(b, Pw, QE, S, [BRE], BB, reverse=True)
        for j in range(17, -1, -1):
            n_ccx[0] += emit_mbu_mul12(b, QT, words(Dd, j), PE[12 * j:12 * (j + 1)], MBP)
    # 7. clear quotient path (tower re-emit needs LY intact: it is)
    n_ccx[0] += emit_mbu_mul12(b, LX, INV, QT, MBP)
    n_ccx[0] += emit_tower_inv12(b, LY, INV, TOWER)


def _emit_eea(b: Builder, n_ccx: list[int], L: dict) -> None:
    """Full 36-block forward EEA pass + extract + failure flag."""
    from .mbu12 import emit_mbu_mul12
    from .packing import emit_zero_test
    from .tower12 import emit_tower_inv12
    X, Y, U0, V0 = L["X"], L["Y"], L["U0"], L["V0"]
    MBP = L["MBP"]
    E, O, A, B = L["E"], L["O"], L["A"], L["B"]
    FLAG = L["FLAG"][0]
    ZT, ZTBIG = L["ZT"], L["ZTBIG"]
    TOWER, PE = L["TOWER"], L["PE"]

    def words(bank: list[int], i: int) -> list[int]:
        return bank[12 * i:12 * (i + 1)]

    # init: X=E, Y=O, U0=1, V1=1 (rest zero)
    for i in range(12 * NW_IO):
        b.cx(E[i], X[i])
        b.cx(O[i], Y[i])
    b.x(U0[0])
    b.x(L["V1"][0])
    for blk in range(N_BLOCKS):
        _emit_eea_block(b, n_ccx, L, blk)
    # extract A,B: A=U0/c, B=V0/c with c=X[0] via tower inverse
    n_ccx[0] += emit_tower_inv12(b, words(X, 0), PE[0:12], TOWER)
    for i in range(NW_IO):
        n_ccx[0] += emit_mbu_mul12(b, words(U0, i), PE[0:12], words(A, i), MBP)
        n_ccx[0] += emit_mbu_mul12(b, words(V0, i), PE[0:12], words(B, i), MBP)
    n_ccx[0] += emit_tower_inv12(b, words(X, 0), PE[0:12], TOWER)
    # failure flag: Y==0 AND X[1:]==0 AND X[0]!=0  (else refuse downstream)
    f1, f2, f3 = ZTBIG[0], ZTBIG[1], ZTBIG[2]
    n_ccx[0] += emit_zero_test(b, X[0:216], f1, ZTBIG[3:217])
    n_ccx[0] += emit_zero_test(b, X[12:216], f2, ZTBIG[3:205])
    n_ccx[0] += emit_zero_test(b, Y[0:216], f3, ZTBIG[3:217])
    # FLAG = f3 & f2 & !f1 via PE[0] temp (cleaned by re-emit)
    b.x(f1)
    b.ccx(f3, f2, PE[0])
    n_ccx[0] += 1
    b.ccx(PE[0], f1, FLAG)
    n_ccx[0] += 1
    b.ccx(f3, f2, PE[0])
    n_ccx[0] += 1
    b.x(f1)


def build_eea12() -> tuple[object, dict]:
    """Full paper-scale half-degree EEA pass (fixed 36-block schedule).

    Inputs E,O (18 words each); outputs A,B (18 words each) plus FLAG
    (1 = converged with A*E+B*O=1 verifiable; 0 = refuse downstream).
    """
    from .kickmix import Builder
    L = eea12_layout(0)
    b = Builder(L["nqubits"])
    for q in L["allocs"]:
        b.alloc(q)
    n_ccx = [0]
    _emit_eea(b, n_ccx, L)
    for q in L["allocs"]:
        b.free(q)
    return b, {"inputs": tuple(range(L["nin"])), "outputs": tuple(list(L["A"]) + list(L["B"]) + list(L["FLAG"])),
               "temps": L["ntemps"], "ccx": n_ccx[0], "kind": "eea12"}
