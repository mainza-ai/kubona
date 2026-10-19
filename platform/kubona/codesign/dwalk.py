"""History-free divstep walk (the decoded circuit the naive step beats, proven).

The naive step (dstep.py) persists 2 history qubits per transition; the
decoded walk re-derives every decision bit from the next state instead, so
nothing persists across steps. Three pieces share one logic home here:

- decode decisions from N: s = f'v[1] (visibility, Lemma A4.1), then for
  s == 0: g' == 0 forces b (zero rule), zp >= 1 forces b == 0 (z-rule),
  else the depth-j toy-prime rule decides (depths 2-3 on this domain —
  fit_divstep_state proves the recipe, the circuit implements it exactly).
- decoded forward step: the shared _emit_step core with no history bank
  (flags live only around the mux), S intact, N produced, zero history.
- decode-reverse: from N alone (S bank zero) re-derive flags, run the
  inverse paths into P, route S back out, cancel N via forward recompute,
  clear flags from the restored S. Walk composition ping-pongs two banks.

Widths are the dstep toy widths; every intermediate is model-checked by
fit gates (no silent wrap); unknown depths/zero f'v refuse, never guess.
"""

from __future__ import annotations

from .dstep import (
    REG,
    WZ,
    WF,
    WG,
    WFV,
    WGV,
    P_TOY,
    _add,
    _cp,
    _double,
    _emit_step,
    _ext,
    _flags,
    _halve,
    _mux,
    _neg5,
    _path,
    _scorr,
    _split,
    _sub,
    fit_divstep_state,
    from_bits,
    harvest_states,
    to_bits,
)

#: Shared register file: the dstep core (banks A = S layout, B = N layout)
#: plus the two decoded-flag regs and an eight-bank pool (C/D/E/J/K/L/H/M)
#: for full walks. Decode temps borrow P0 slices while the path banks are
#: clear (documented at each use; the sweep proves clearing). The walk
#: needs a FRESH bank per step: XOR routing into a nonzero bank yields
#: source^next garbage (and the harness refuses input modification), so
#: ping-pong reuse is invalid — straight-line composition into zero banks,
#: intermediates persist. Ten banks cap walks at 9 steps (p = 13 needs at
#: most 8, p = 17 needs at most 9 — measured maxima per prime; longer
#: refuses fail-closed — pool sizing, not physics).
WREG = dict(REG)
WREG["DFS"] = [187]
WREG["DFB"] = [188]
WREG["C"] = list(range(189, 220))
WREG["D"] = list(range(220, 251))
WREG["E"] = list(range(251, 282))
WREG["J"] = list(range(282, 313))
WREG["K"] = list(range(313, 344))
WREG["L"] = list(range(344, 375))
WREG["H"] = list(range(375, 406))
WREG["M"] = list(range(406, 437))
N_QUBITS_W = 437
# Pool letters skip F/G (dstep state regs) — a pool bank named "F" once
# silently replaced the 6-qubit F register and broke every later step.
_POOL = "CDEJKLHM"
BANK_A = {"Z": WREG["Z"], "F": WREG["F"], "G": WREG["G"],
          "FV": WREG["FV"], "GV": WREG["GV"]}
BANK_B = {"Z": WREG["NZ"], "F": WREG["NF"], "G": WREG["NG"],
          "FV": WREG["NFV"], "GV": WREG["NGV"]}
BANK_C = dict(zip(("Z", "F", "G", "FV", "GV"), _split(WREG["C"], [5, 6, 6, 7, 7])))
BANKS = [BANK_A, BANK_B, BANK_C] + [
    dict(zip(("Z", "F", "G", "FV", "GV"), _split(WREG[ch], [5, 6, 6, 7, 7])))
    for ch in _POOL[1:]
]
BANK_IDS = [tuple(b["Z"] + b["F"] + b["G"] + b["FV"] + b["GV"]) for b in BANKS]
BANK_A_IDS = tuple(BANK_A["Z"] + BANK_A["F"] + BANK_A["G"]
                   + BANK_A["FV"] + BANK_A["GV"])
BANK_B_IDS = tuple(BANK_B["Z"] + BANK_B["F"] + BANK_B["G"]
                   + BANK_B["FV"] + BANK_B["GV"])
BANK_C_IDS = tuple(BANK_C["Z"] + BANK_C["F"] + BANK_C["G"]
                   + BANK_C["FV"] + BANK_C["GV"])
# DECODE_POOL sizing lives at each call site (14 + len(depths) regs from
# P0); a shared fixed pool would silently under-serve deeper domains.
DF_IDS = tuple(WREG["DFS"] + WREG["DFB"])
WTEMPS = tuple(WREG["P0"] + WREG["P1"] + WREG["P2"] + WREG["SUMA"]
               + WREG["SUMB"] + WREG["CARR"] + WREG["ONE"] + WREG["ZERO"]
               + WREG["MUT"] + WREG["BF"] + WREG["CF"] + WREG["SF"])


def decode_decisions(nstate: tuple[int, int, int, int, int], p: int = P_TOY,
                     depths: tuple[int, ...] = (2, 3)) -> dict:
    """Re-derive (s, b) from the NEXT state (the history-free recipe).

    s rides f'v bit 1 (f'v always even; v2 == 1 iff swap — prime-generic).
    s == 1 forces b; g' == 0 forces b (zero rule); zp >= 1 forces b == 0
    (z-rule); else the depth-j rule for THIS prime decides over
    f'[1..j-1], q[1..j-1] with q = f'v/2^j. Refuses zero f'v in the
    ambiguous branch and depths outside the proven set — the proof domain
    is exactly the fit-gated swept states, never wider.
    """
    from .divsteps import eval_decoder
    from .divwalk import v2
    if len(nstate) != 5:
        raise ValueError(f"next state must be a 5-tuple: refusing {nstate!r}")
    if isinstance(p, bool) or not isinstance(p, int) or p < 3 or p % 2 == 0:
        raise ValueError(f"p must be an odd prime >= 3, got {p!r}: refusing")
    zp, fp, gp, fvp, gvp = nstate
    if fvp % 2 != 0:
        raise ValueError("f'v odd (Lemma A1 parity broken): refusing")
    s = (fvp >> 1) & 1
    if s == 1:
        return {"s": 1, "b": 1}
    if gp == 0:
        return {"s": 0, "b": 1}
    if zp >= 1:
        return {"s": 0, "b": 0}
    if fvp == 0:
        raise ValueError("ambiguous branch with f'v == 0 (no depth): refusing")
    j = v2(fvp)
    if j not in depths:
        raise ValueError(f"ambiguous depth {j} outside proven {tuple(depths)}: refusing")
    M = 1 << j
    q = (fvp // (1 << j)) % M
    bits = {}
    for i in range(1, j):
        bits[f"f{i}"] = (fp % M >> i) & 1
        bits[f"q{i}"] = (q >> i) & 1
    return {"s": 0, "b": eval_decoder(j, bits, p=p)}


def _decode_ops(NZ, NF, NG, NFV, S_OUT: int, B_OUT: int, T: list[int],
                rules: dict[int, frozenset], depths: tuple[int, ...]):
    """Op list implementing decode_decisions as gates (compute phase).

    Returns (ops, ccx) with ops = [(name, qubits, keep)] where keep marks
    output writes (never mirrored); every other op is self-inverse so the
    mirror is the reversed non-keep subsequence. Temp pool T needs
    14 + len(depths) regs: ZG tree 5, ZZ tree 4, POS 1, one depth flag
    each, DEC/T1/ELO/TAND 1 each. Input regs are flipped and restored
    (X pairs), never consumed. q-bit mapping: q_i at depth j reads
    f'v bit j+i (direct while <= 6); bit 7 is the sign bit (exact for
    fit-gated f'v in FV range — the sweep proves the agreement); bit 8+
    refuses (needs a wider FV register, not silent truncation).
    rules must cover exactly `depths` (fail-closed both ways).
    """
    if (not isinstance(depths, tuple) or not depths or
            any(isinstance(j, bool) or not isinstance(j, int) or j < 2 for j in depths)):
        raise ValueError(f"depths must be a non-empty tuple of ints >= 2, got {depths!r}: refusing")
    if set(rules) != set(depths):
        raise ValueError(f"rules must cover depths {tuple(depths)} exactly: refusing")
    for j, poly in rules.items():
        if not isinstance(poly, frozenset):
            raise ValueError(f"depth-{j} rule must be ANF frozenset: refusing")
        for term in poly:
            for v in term:
                if not (isinstance(v, str) and (v.startswith("f") or v.startswith("q"))):
                    raise ValueError(f"bad rule literal {v!r}: refusing")
                if v.startswith("f"):
                    if not (1 <= int(v[1:]) < j):
                        raise ValueError(f"f-index out of depth-{j} range: refusing {v!r}")
                else:
                    if not (1 <= int(v[1:]) < j):
                        raise ValueError(f"q-index out of depth-{j} range: refusing {v!r}")
    if max(depths) > 4:
        raise ValueError(f"inline decode needs f'v bit {2 * max(depths) - 1} > sign bit: refusing")

    ops: list[tuple[str, tuple[int, ...], bool]] = []
    n_ccx = [0]

    def cx(c, t, keep=False):
        ops.append(("cx", (c, t), keep))

    def x(q):
        ops.append(("x", (q,), False))

    def ccx(a, b, t, keep=False):
        ops.append(("ccx", (a, b, t), keep))
        n_ccx[0] += 1

    def or2(dst, a, b):
        cx(a, dst)
        cx(b, dst)
        ccx(a, b, dst)

    def _qbit(j: int, i: int) -> int:
        # q_i at depth j lives at f'v bit j+i (direct while <= 6);
        # bit 7 is the sign bit (exact for fit-gated f'v in FV range).
        idx = j + i
        if idx <= 6:
            return NFV[idx]
        if idx == 7:
            return NFV[6]
        raise ValueError(f"q{i} at depth {j} needs f'v bit {idx} (no such register): refusing")

    def _lit(name: str, j: int) -> int:
        if name.startswith("f"):
            return NF[int(name[1:])]
        return _qbit(j, int(name[1:]))

    def _and_chain(lits: list[int], target: int, scratch: list[int]) -> None:
        # AND-chain writing target; scratch holds intermediates (caller
        # guarantees scratch clear on entry). Mirror clears scratch only —
        # the target write persists. Single input = plain wire.
        ops_fw: list[tuple[int, int, int]] = []
        if len(lits) == 1:
            cx(lits[0], target)
            return
        if len(lits) - 2 > len(scratch):
            raise ValueError("rule term exceeds scratch (implementation bug): refusing")
        prev = lits[0]
        for k in range(1, len(lits)):
            dst = target if k == len(lits) - 1 else scratch[k - 1]
            ops_fw.append((prev, lits[k], dst))
            prev = dst
        for a, b, t in ops_fw:
            ccx(a, b, t)
        for a, b, t in reversed(ops_fw[:-1]):
            ccx(a, b, t)

    zg = T[0:5]
    zz = T[5:9]
    if len(T) < 14 + len(depths):
        raise ValueError(f"temp pool needs {14 + len(depths)} regs, got {len(T)}: refusing")
    pos = T[9]
    dflag = {}
    nxt = 10
    for j in sorted(depths):
        dflag[j] = T[nxt]
        nxt += 1
    dec, t1, elo, tand = T[nxt], T[nxt + 1], T[nxt + 2], T[nxt + 3]
    chain = [t1, elo, tand]
    # s = f'v[1] (visibility; f'v even always, proven by fit on domain).
    cx(NFV[1], S_OUT, keep=True)
    # ZG = NOR6(g'): 5-node OR tree + X.
    or2(zg[0], NG[0], NG[1])
    or2(zg[1], NG[2], NG[3])
    or2(zg[2], NG[4], NG[5])
    or2(zg[3], zg[0], zg[1])
    or2(zg[4], zg[3], zg[2])
    x(zg[4])
    # ZZ = NOR5(z'): 4-node OR tree + X.
    or2(zz[0], NZ[0], NZ[1])
    or2(zz[1], NZ[2], NZ[3])
    or2(zz[2], zz[0], zz[1])
    or2(zz[3], zz[2], NZ[4])
    x(zz[3])
    # POS = ~sign & ~ZZ (zp >= 1 exactly): flip, AND, restore.
    x(NZ[4])
    x(zz[3])
    ccx(NZ[4], zz[3], pos)
    x(NZ[4])
    x(zz[3])
    # Depth flags Dj = ~f1 & ... & ~(f_{j-1}) & fj (v2 == j needs
    # f'v[0] == 0, always true here). AND-chains borrow t1 (+tand),
    # cleared immediately after (their own later uses see zeros).
    for j in sorted(depths):
        D = dflag[j]
        for i in range(1, j):
            x(NFV[i])
        _and_chain([NFV[i] for i in range(1, j + 1)], D, [t1, tand])
        for i in range(1, j):
            x(NFV[i])
    # DEC = OR over depths of (Dj & rule_j); each term ANDs its literals
    # with Dj (const term = bare Dj) and XORs into DEC. Scratch (t1 and,
    # if needed, elo/tand slots) is free here: ELO/TAND are computed
    # after, and every chain mirrors itself clean.
    for j in sorted(depths):
        Dj = dflag[j]
        for term in sorted(map(sorted, rules[j])):
            if len(term) == 0:
                cx(Dj, dec)
                continue
            lits = [_lit(v, j) for v in term]
            _and_chain([Dj] + lits, dec, [t1, elo, tand])
    # ELO = ~(s | ZG | POS): OR into t1, OR with POS into elo, NOR by X.
    cx(S_OUT, t1)
    cx(zg[4], t1)
    ccx(S_OUT, zg[4], t1)
    cx(t1, elo)
    cx(pos, elo)
    ccx(t1, pos, elo)
    x(elo)
    # TAND = ELO & DEC; B = t1 | TAND (t1 still holds s|ZG here).
    ccx(elo, dec, tand)
    cx(t1, B_OUT, keep=True)
    cx(tand, B_OUT, keep=True)
    ccx(t1, tand, B_OUT, keep=True)
    return ops, n_ccx[0]


def _run_ops(b, ops, n_ccx, mirror: bool = False) -> None:
    """Emit ops (or the clearing mirror: reversed non-keep subsequence)."""
    seq = ops if not mirror else [op for op in reversed(ops) if not op[2]]
    for name, qs, _keep in seq:
        if name == "cx":
            b.cx(*qs)
        elif name == "x":
            b.x(qs[0])
        else:
            b.ccx(*qs)
            n_ccx[0] += 1


def fit_divstep_inverse(state: tuple[int, int, int, int, int],
                        p: int = P_TOY, depths: tuple[int, ...] = (2, 3)) -> dict:
    """Model-check the INVERSE arithmetic for one swept state.

    Runs the forward step, inverts by branch in integers, and asserts every
    inverse intermediate (doublings in SUMA8, differences in SUMA8/SUMB8,
    halvings exact, results in Z5/F6/G6/FV7/GV7) plus exact recovery of the
    input state. The decode-reverse circuit is proven against states this
    gate admits (fail-closed swept domain, same standard as forward fit).
    """
    from .divwalk import step
    info = fit_divstep_state(state, p, depths)
    zp, fp, gp, fvp, gvp = info["next"]
    s, b, branch = info["s"], info["b"], info["branch"]

    def _w(v: int, bits: int, what: str) -> None:
        lo, hi = -(1 << (bits - 1)), (1 << (bits - 1)) - 1
        if not (lo <= v <= hi):
            raise ValueError(f"inverse {what}={v} outside {bits} bits: refusing")

    _w(zp - 1, 5, "z")
    if fvp % 2 != 0:
        raise ValueError("inverse halving inexact (implementation bug): refusing")
    if branch == "even":
        _w(2 * gp, 6, "2gp")
        rec = (zp - 1, fp, 2 * gp, fvp // 2, gvp)
    elif branch == "odd":
        _w(2 * gp, 8, "2gp")
        _w(2 * gp - fp, 8, "2gp-fp")
        _w(2 * gp - fp, 6, "g")
        _w(fvp // 2, 7, "fv-half")
        _w(gvp - fvp // 2, 8, "gv-fv")
        _w(gvp - fvp // 2, 7, "gv")
        rec = (zp - 1, fp, 2 * gp - fp, fvp // 2, gvp - fvp // 2)
    else:
        _w(-zp, 5, "-zp")
        _w(2 * gp, 8, "2gp")
        _w(2 * gp - fp, 8, "2gp-fp")
        _w(fp - 2 * gp, 6, "f")
        _w(fvp // 2 - gvp, 8, "fv scratch")
        _w(fvp // 2 - gvp, 7, "fv")
        rec = (-zp, fp - 2 * gp, fp, fvp // 2 - gvp, fvp // 2)
    if rec != state:
        raise ValueError(f"inverse does not recover {state!r}: refusing {rec!r}")
    return {"branch": branch, "s": s, "b": b, "next": info["next"]}


def _load_double(b, SUMA: list[int], G6: list[int]) -> None:
    """Materialize 2g (6-bit G) into 8-bit SUMA with sign extension.

    SUMA[0] stays 0 (2g even); bits 1-6 shift G up one; 6-7 carry the
    sign. Self-inverse unload (same CXs). |2g| <= 62 always fits.
    """
    for i in range(1, 7):
        b.cx(G6[i - 1], SUMA[i])
    b.cx(G6[5], SUMA[6])
    b.cx(G6[5], SUMA[7])


def _inv_path(b, R, n_ccx, NBNK, which: int, undo: bool) -> None:
    """One INVERSE update path: N-bank values back to the source state.

    Even: z = zp-1, g = 2gp (double), fv = f'v/2 (halve, exact by Lemma
    A1), f/gv copies. Odd: g = 2gp-fp and gv = g'v-f'v via the SUMA/SUMB
    widening pattern (same widths as forward, model-pinned). Swap:
    z = -zp, f = fp-2gp (double, sub, negate in SUMA), g = fp copy,
    gv = f'v/2, fv = f'v/2-g'v (halve into SUMB scratch, sub). undo=True
    mirrors exactly (restore-then-unload discipline, same as _path).
    """
    P = [R["P0"], R["P1"], R["P2"]][which]
    nz, nf, ng, nfv, ngv = _split(P, [5, 6, 6, 7, 7])
    NZ, NF, NG, NFV, NGV = (NBNK["Z"], NBNK["F"], NBNK["G"],
                            NBNK["FV"], NBNK["GV"])
    ONE4 = [R["ONE"][0]] + [R["ZERO"][0]] * 3

    def _sumA_odd_build() -> None:
        _load_double(b, R["SUMA"], NG)
        _sub(b, R, n_ccx, R["SUMA"], _ext(NF, 7))
        _scorr(b, NF[5], R["SUMA"][7])

    def _sumA_odd_clear() -> None:
        _scorr(b, NF[5], R["SUMA"][7])
        _add(b, R, n_ccx, R["SUMA"], _ext(NF, 7))
        _load_double(b, R["SUMA"], NG)

    def _sumB_halved_build() -> None:
        # gv = g'v - f'v/2: halve f'v into the SUMA scratch (SUMA is clear
        # here — the g-block above extracted and cleared it), load g'v
        # sign-extended, subtract the halved value, correct the sign bit.
        _halve(b, R["SUMA"][:7], NFV)
        for i in range(7):
            b.cx(NGV[i], R["SUMB"][i])
        b.cx(NGV[6], R["SUMB"][7])
        _sub(b, R, n_ccx, R["SUMB"], _ext(R["SUMA"][:7], 7))
        _scorr(b, R["SUMA"][6], R["SUMB"][7])

    def _sumB_halved_clear() -> None:
        _scorr(b, R["SUMA"][6], R["SUMB"][7])
        _add(b, R, n_ccx, R["SUMB"], _ext(R["SUMA"][:7], 7))
        for i in range(7):
            b.cx(NGV[i], R["SUMB"][i])
        b.cx(NGV[6], R["SUMB"][7])
        _halve(b, R["SUMA"][:7], NFV)

    if not undo:
        if which == 0:
            _cp(b, nz, NZ)
            _sub(b, R, n_ccx, nz, ONE4)
            _cp(b, nf, NF)
            _double(b, ng, NG)
            _halve(b, nfv, NFV)
            _cp(b, ngv, NGV)
            return
        if which == 1:
            _cp(b, nz, NZ)
            _sub(b, R, n_ccx, nz, ONE4)
            _cp(b, nf, NF)
            _sumA_odd_build()
            _cp(b, ng, R["SUMA"][:6])
            _sumA_odd_clear()
            _halve(b, nfv, NFV)
            _sumB_halved_build()
            _cp(b, ngv, R["SUMB"])
            _sumB_halved_clear()
            return
        # swap inverse: z = -zp; f = fp-2gp; g = fp; gv = f'v/2; fv = f'v/2-g'v
        _cp(b, nz, NZ)
        _neg5(b, R, n_ccx, nz)
        _load_double(b, R["SUMA"], NG)
        _sub(b, R, n_ccx, R["SUMA"], _ext(NF, 7))
        _scorr(b, NF[5], R["SUMA"][7])
        _neg5(b, R, n_ccx, R["SUMA"])
        _cp(b, nf, R["SUMA"][:6])
        _neg5(b, R, n_ccx, R["SUMA"])
        _scorr(b, NF[5], R["SUMA"][7])
        _add(b, R, n_ccx, R["SUMA"], _ext(NF, 7))
        _load_double(b, R["SUMA"], NG)
        _cp(b, ng, NF)
        _halve(b, ngv, NFV)
        _halve(b, R["SUMB"][:7], NFV)
        _sub(b, R, n_ccx, R["SUMB"], _ext(NGV, 7))
        _scorr(b, NGV[6], R["SUMB"][7])
        _cp(b, nfv, R["SUMB"][:7])
        _scorr(b, NGV[6], R["SUMB"][7])
        _add(b, R, n_ccx, R["SUMB"], _ext(NGV, 7))
        _halve(b, R["SUMB"][:7], NFV)
        return
    # mirror: extract-blocks are self-inverse as wholes (re-establish the
    # scratch, XOR the result back out, clear the scratch), so the mirror
    # repeats compute verbatim — reversed order with swapped add/sub
    # double-counts the operand and leaks P2[6] (caught live on all 88).
    if which == 0:
        _cp(b, ngv, NGV)
        _halve(b, nfv, NFV)
        _double(b, ng, NG)
        _cp(b, nf, NF)
        _add(b, R, n_ccx, nz, ONE4)
        _cp(b, nz, NZ)
        return
    if which == 1:
        _sumB_halved_build()
        _cp(b, ngv, R["SUMB"])
        _sumB_halved_clear()
        _halve(b, nfv, NFV)
        _sumA_odd_build()
        _cp(b, ng, R["SUMA"][:6])
        _sumA_odd_clear()
        _cp(b, nf, NF)
        _add(b, R, n_ccx, nz, ONE4)
        _cp(b, nz, NZ)
        return
    _halve(b, R["SUMB"][:7], NFV)
    _sub(b, R, n_ccx, R["SUMB"], _ext(NGV, 7))
    _scorr(b, NGV[6], R["SUMB"][7])
    _cp(b, nfv, R["SUMB"][:7])
    _scorr(b, NGV[6], R["SUMB"][7])
    _add(b, R, n_ccx, R["SUMB"], _ext(NGV, 7))
    _halve(b, R["SUMB"][:7], NFV)
    _halve(b, ngv, NFV)
    _cp(b, ng, NF)
    _load_double(b, R["SUMA"], NG)
    _sub(b, R, n_ccx, R["SUMA"], _ext(NF, 7))
    _scorr(b, NF[5], R["SUMA"][7])
    _neg5(b, R, n_ccx, R["SUMA"])
    _cp(b, nf, R["SUMA"][:6])
    _neg5(b, R, n_ccx, R["SUMA"])
    _scorr(b, NF[5], R["SUMA"][7])
    _add(b, R, n_ccx, R["SUMA"], _ext(NF, 7))
    _load_double(b, R["SUMA"], NG)
    _neg5(b, R, n_ccx, nz)
    _cp(b, nz, NZ)


def _decode_rules(p: int, depths: tuple[int, ...]) -> dict[int, frozenset]:
    """Per-prime ANF branch rules (single derivation home for the decoder).

    decoder_poly_derived is the single source of truth (generic theorem +
    prime); the fit gate, the integer recipe, and the circuit op list all
    consume its output — never a transcribed copy.
    """
    from .divsteps import decoder_poly_derived
    return {j: decoder_poly_derived(j, p=p) for j in depths}


def _decode_temps_needed(depths: tuple[int, ...]) -> int:
    """14 + len(depths) temps: ZG 5 + ZZ 4 + POS 1 + one flag each + DEC/T1/ELO/TAND."""
    return 14 + len(depths)


def build_decode_flags(p: int = P_TOY, depths: tuple[int, ...] = (2, 3)) -> tuple[object, dict]:
    """Standalone flag re-derivation: N-bank in, (s, b) out, temps clear.

    Own file (31 N + s + b + pool temps); the walk reuses the same op
    list on WREG slices. Proves the decode recipe as gates: the mirror
    clears every temp (dirty-free refuses leaks) while s/b persist.
    """
    from .kickmix import Builder
    N = list(range(31))
    NZ, NF, NG, NFV = _split(N, [5, 6, 6, 7])[:4]
    NGV = N[24:31]
    S_OUT, B_OUT = 31, 32
    ntemps = _decode_temps_needed(depths)
    T = list(range(33, 33 + ntemps))
    b = Builder(33 + ntemps)
    for q in T:
        b.alloc(q)
    rules = _decode_rules(p, depths)
    ops, _ccx_compute = _decode_ops(NZ, NF, NG, NFV, S_OUT, B_OUT, T, rules, depths)
    n_ccx = [0]
    _run_ops(b, ops, n_ccx, mirror=False)
    _run_ops(b, ops, n_ccx, mirror=True)
    for q in T:
        b.free(q)
    return b, {"ccx": n_ccx[0], "temps": len(T)}


def decode_flags_spec(p: int = P_TOY, depths: tuple[int, ...] = (2, 3)):
    """TaskSpec: fresh (s, b) get the recipe at the N input bits."""
    from .kickmix import TaskSpec
    N = list(range(31))
    NZ, NF, NG, NFV = _split(N, [5, 6, 6, 7])[:4]
    NGV = N[24:31]
    ntemps = _decode_temps_needed(depths)

    def _perm(bits: dict[int, int]) -> dict[int, int]:
        out = dict(bits)
        ns = tuple(from_bits([bits.get(q, 0) for q in regs], signed=True)
                   for regs in (NZ, NF, NG, NFV, NGV))
        dec = decode_decisions(ns, p, depths)
        out[31] = dec["s"]
        out[32] = dec["b"]
        return out

    return TaskSpec(num_qubits=33 + ntemps, inputs=tuple(N), outputs=(31, 32),
                    ancillas=tuple(range(33, 33 + ntemps)), target_perm=_perm)


def _pack_next(nstate: tuple[int, int, int, int, int]) -> dict[int, int]:
    init: dict[int, int] = {}
    for v, (reg, w) in zip(nstate, (("Z", WZ), ("F", WF), ("G", WG),
                                    ("FV", WFV), ("GV", WGV))):
        for i, bit in enumerate(to_bits(v, w)):
            init[BANK_B[reg][i]] = bit
    return init


def _base_builder():
    """WREG builder with shared temps alloc'd and ONE loaded."""
    from .kickmix import Builder
    R = WREG
    b = Builder(N_QUBITS_W)
    for q in WTEMPS:
        b.alloc(q)
    b.x(R["ONE"][0])
    return b, R


def build_dstep_decoded() -> tuple[object, dict]:
    """Decoded forward step: bank A in, bank B out, zero history.

    The shared _emit_step core with no history bank: flags are computed
    for the mux and mirror-cleared, paths computed/routed/uncomputed.
    S (bank A) intact, N (bank B) produced, every temp freed.
    """
    R = WREG
    b, R = _base_builder()
    n_ccx = [0]
    FLG = {"BF": R["BF"][0], "CF": R["CF"][0], "SF": R["SF"][0]}
    _emit_step(b, R, n_ccx, BANK_A, BANK_B, FLG, None, roundtrip=False)
    b.x(R["ONE"][0])
    for q in WTEMPS:
        b.free(q)
    return b, {"ccx": n_ccx[0], "history_qubits": 0}


def dstep_decoded_spec_fwd():
    """TaskSpec: bank B gets the modeled step (bank A preserved)."""
    from .divwalk import step
    from .kickmix import TaskSpec

    def _perm(bits: dict[int, int]) -> dict[int, int]:
        out = dict(bits)
        state = tuple(from_bits([bits.get(q, 0) for q in BANK_A[k]])
                      for k in ("Z", "F", "G", "FV", "GV"))
        new_state, _s, _b = step(state)
        for v, k in zip(new_state, ("Z", "F", "G", "FV", "GV")):
            for i, bit in enumerate(to_bits(v, {"Z": WZ, "F": WF, "G": WG,
                                                 "FV": WFV, "GV": WGV}[k])):
                out[BANK_B[k][i]] = bit
        return out

    return TaskSpec(num_qubits=N_QUBITS_W, inputs=BANK_A_IDS,
                    outputs=BANK_B_IDS, ancillas=WTEMPS, target_perm=_perm)


def build_dwalk2() -> tuple[object, dict]:
    """Composed decoded walk: two steps, A -> B -> C, zero history.

    Each step writes a FRESH zero bank (B then C); bank A (the input walk
    head) is never modified, intermediates S1/S2 persist in B/C. Longer
    walks repeat the pattern (one fresh bank per step); the 2-step walk
    proves the composition (shared core, fresh-bank discipline, both
    states checked).
    """
    R = WREG
    b, R = _base_builder()
    n_ccx = [0]
    FLG = {"BF": R["BF"][0], "CF": R["CF"][0], "SF": R["SF"][0]}
    _emit_step(b, R, n_ccx, BANK_A, BANK_B, FLG, None, roundtrip=False)
    _emit_step(b, R, n_ccx, BANK_B, BANK_C, FLG, None, roundtrip=False)
    b.x(R["ONE"][0])
    for q in WTEMPS:
        b.free(q)
    return b, {"ccx": n_ccx[0], "nsteps": 2, "history_qubits": 0}


def dwalk2_spec():
    """TaskSpec: S1 lands in bank B, S2 in bank C (bank A preserved)."""
    from .divwalk import step
    from .kickmix import TaskSpec

    def _perm(bits: dict[int, int]) -> dict[int, int]:
        out = dict(bits)
        s0 = tuple(from_bits([bits.get(q, 0) for q in BANK_A[k]])
                   for k in ("Z", "F", "G", "FV", "GV"))
        s1 = step(s0)[0]
        s2 = step(s1)[0]
        for v, k in zip(s1, ("Z", "F", "G", "FV", "GV")):
            for i, bit in enumerate(to_bits(v, {"Z": WZ, "F": WF, "G": WG,
                                                 "FV": WFV, "GV": WGV}[k])):
                out[BANK_B[k][i]] = bit
        for v, k in zip(s2, ("Z", "F", "G", "FV", "GV")):
            for i, bit in enumerate(to_bits(v, {"Z": WZ, "F": WF, "G": WG,
                                                 "FV": WFV, "GV": WGV}[k])):
                out[BANK_C[k][i]] = bit
        return out

    return TaskSpec(num_qubits=N_QUBITS_W, inputs=BANK_A_IDS,
                    outputs=BANK_B_IDS + BANK_C_IDS, ancillas=WTEMPS,
                    target_perm=_perm)


def _walk_states(w: int, p: int = P_TOY, depths: tuple[int, ...] = (2, 3)) -> list:
    """All active states of the w-walk (S0..S_{L-1}), fit-gated.

    Refuses w outside 1..p-1, frozen walks, unfit intermediates, and
    lengths beyond the bank pool — the proof domain is exactly walks the
    circuit can hold, never wider. Depths gate the ambiguous-decision
    domain the decoder implements.
    """
    from .divwalk import run_walk, step
    if isinstance(w, bool) or not isinstance(w, int) or not (1 <= w < p):
        raise ValueError(f"w must be in [1,{p - 1}], got {w!r}: refusing")
    rep = run_walk(p, w)
    if rep.get("frozen"):
        raise ValueError(f"w={w} frozen (no active steps): refusing")
    states = []
    s = (0, p, w, 0, 1)
    while True:
        z, f, g, fv, gv = s
        if g == 0:
            break
        fit_divstep_state(s, p, depths)  # fail-closed domain gate (raises on violators)
        states.append(s)
        s = step(s)[0]
    if len(states) > len(BANKS) - 1:
        raise ValueError(f"walk w={w} needs {len(states)} steps > pool: refusing")
    if not states:
        raise ValueError(f"w={w} has no active steps: refusing")
    return states


def build_dwalk_full(w: int, p: int = P_TOY, depths: tuple[int, ...] = (2, 3)) -> tuple[object, dict]:
    """Full walk to termination: S0 in bank 0, S_k in bank k, zero history.

    One fresh bank per step (straight-line); every input state fit-gated
    by _walk_states. The terminal bank holds g == 0 (termination is an
    OUTPUT value, never fed back — the circuit implements no frozen rule,
    and the builder never asks it to).
    """
    R = WREG
    states = _walk_states(w, p, depths)
    b, R = _base_builder()
    n_ccx = [0]
    FLG = {"BF": R["BF"][0], "CF": R["CF"][0], "SF": R["SF"][0]}
    for k in range(len(states)):
        _emit_step(b, R, n_ccx, BANKS[k], BANKS[k + 1], FLG, None,
                   roundtrip=False)
    b.x(R["ONE"][0])
    for q in WTEMPS:
        b.free(q)
    return b, {"ccx": n_ccx[0], "nsteps": len(states),
               "history_qubits": 0, "w": w}


def dwalk_full_spec(w: int, p: int = P_TOY, depths: tuple[int, ...] = (2, 3)):
    """TaskSpec: bank k holds S_k for k = 1..L (bank 0 preserved)."""
    from .divwalk import step
    from .kickmix import TaskSpec
    states = _walk_states(w, p, depths)
    L = len(states)

    def _perm(bits: dict[int, int]) -> dict[int, int]:
        out = dict(bits)
        s = tuple(from_bits([bits.get(q, 0) for q in BANKS[0][k]])
                  for k in ("Z", "F", "G", "FV", "GV"))
        for k in range(L):
            s = step(s)[0]
            for v, kk in zip(s, ("Z", "F", "G", "FV", "GV")):
                for i, bit in enumerate(to_bits(v, {"Z": WZ, "F": WF, "G": WG,
                                                     "FV": WFV, "GV": WGV}[kk])):
                    out[BANKS[k + 1][kk][i]] = bit
        return out

    outs: tuple = ()
    for k in range(1, L + 1):
        outs = outs + BANK_IDS[k]
    return TaskSpec(num_qubits=N_QUBITS_W, inputs=BANK_IDS[0],
                    outputs=outs, ancillas=WTEMPS, target_perm=_perm)


def build_dwalk_full_roundtrip(w: int, p: int = P_TOY, depths: tuple[int, ...] = (2, 3)) -> tuple[object, dict]:
    """Full-walk roundtrip: forward to termination, then reverse-clear.

    Reverse step k (k = L..1) recomputes flags from bank k-1 (intact —
    reverse steps write only their own bank, read-only elsewhere),
    recomputes paths, re-muxes bank k to zero, uncomputes, mirror-clears
    flags. Final: bank 0 holds S0, every later bank zero, no history.
    Decode is not needed here (every intermediate persists); the
    decode-reverse primitive covers the space-squeezed case instead.
    """
    R = WREG
    states = _walk_states(w, p, depths)
    L = len(states)
    b, R = _base_builder()
    n_ccx = [0]
    FLG = {"BF": R["BF"][0], "CF": R["CF"][0], "SF": R["SF"][0]}
    for k in range(L):
        _emit_step(b, R, n_ccx, BANKS[k], BANKS[k + 1], FLG, None,
                   roundtrip=False)
    for k in range(L, 0, -1):
        P, NBNK = BANKS[k - 1], BANKS[k]
        NLIST = NBNK["Z"] + NBNK["F"] + NBNK["G"] + NBNK["FV"] + NBNK["GV"]
        _flags(b, n_ccx, P, FLG, False)
        for j in (0, 1, 2):
            _path(b, R, n_ccx, P, j, False)
        _mux(b, n_ccx, NLIST, R["P0"], R["P1"], R["P2"],
             FLG["BF"], FLG["SF"], R["MUT"][0])
        for j in (0, 1, 2):
            _path(b, R, n_ccx, P, j, True)
        _flags(b, n_ccx, P, FLG, True)
    b.x(R["ONE"][0])
    for q in WTEMPS:
        b.free(q)
    return b, {"ccx": n_ccx[0], "nsteps": L,
               "history_qubits": 0, "w": w}


def dwalk_full_roundtrip_spec(w: int, p: int = P_TOY, depths: tuple[int, ...] = (2, 3)):
    """TaskSpec: bank 0 preserved, banks 1..L return to zero."""
    from .kickmix import TaskSpec
    states = _walk_states(w, p, depths)
    L = len(states)

    def _perm(bits: dict[int, int]) -> dict[int, int]:
        out = dict(bits)
        for k in range(1, L + 1):
            for q in BANK_IDS[k]:
                out[q] = 0
        return out

    outs: tuple = ()
    for k in range(1, L + 1):
        outs = outs + BANK_IDS[k]
    return TaskSpec(num_qubits=N_QUBITS_W, inputs=BANK_IDS[0],
                    outputs=outs, ancillas=WTEMPS, target_perm=_perm)


def _invert_state(nstate: tuple[int, int, int, int, int], p: int = P_TOY,
                  depths: tuple[int, ...] = (2, 3)) -> tuple:
    """Integer inverse: next state + decoded flags back to source state."""
    dec = decode_decisions(nstate, p, depths)
    zp, fp, gp, fvp, gvp = nstate
    s, bv = dec["s"], dec["b"]
    if s == 1:
        return {"s": s, "b": bv,
                "src": (-zp, fp - 2 * gp, fp, fvp // 2 - gvp, fvp // 2)}
    if bv == 0:
        return {"s": s, "b": bv,
                "src": (zp - 1, fp, 2 * gp, fvp // 2, gvp)}
    return {"s": s, "b": bv,
            "src": (zp - 1, fp, 2 * gp - fp, fvp // 2, gvp - fvp // 2)}


def build_dstep_decode_reverse(p: int = P_TOY, depths: tuple[int, ...] = (2, 3)) -> tuple[object, dict]:
    """Decode-reverse: bank B (next) in, bank A (source) restored, B zeroed.

    From N alone: re-derive flags (decode ops on the N bank), run the three
    inverse paths into P, route the source state back into bank A by
    conditional-XOR mux, clear inverse paths, cancel bank B by forward
    recompute + re-mux, clear forward paths, clear flags from the restored
    source (controls intact — the _flags forward sequence is self-inverse).
    No history anywhere: this is the primitive that makes walk-level
    reversal history-free.
    """
    R = WREG
    b, R = _base_builder()
    n_ccx = [0]
    FLG = {"BF": R["BF"][0], "CF": R["CF"][0], "SF": R["SF"][0]}
    DFS, DFB = R["DFS"][0], R["DFB"][0]
    NBNK = BANK_B
    NZ = [NBNK["Z"][i] for i in range(5)]
    NF = [NBNK["F"][i] for i in range(6)]
    NG = [NBNK["G"][i] for i in range(6)]
    NFV = [NBNK["FV"][i] for i in range(7)]
    # 1. decode flags from N (pool = P0 slices sized for the domain;
    # path banks are still clear).
    rules = _decode_rules(p, depths)
    ops, _c = _decode_ops(NZ, NF, NG, NFV, DFS, DFB,
                          list(R["P0"][:14 + len(depths)]), rules, depths)
    _run_ops(b, ops, n_ccx, mirror=False)
    _run_ops(b, ops, n_ccx, mirror=True)
    # 2-3. inverse paths from N, route source into bank A.
    for w in (0, 1, 2):
        _inv_path(b, R, n_ccx, NBNK, w, False)
    NA = BANK_A["Z"] + BANK_A["F"] + BANK_A["G"] + BANK_A["FV"] + BANK_A["GV"]
    NB = BANK_B["Z"] + BANK_B["F"] + BANK_B["G"] + BANK_B["FV"] + BANK_B["GV"]
    _mux(b, n_ccx, NA, R["P0"], R["P1"], R["P2"], DFB, DFS, R["MUT"][0])
    for w in (0, 1, 2):
        _inv_path(b, R, n_ccx, NBNK, w, True)
    # 4. cancel bank B: forward paths from restored A, re-mux, uncompute.
    for w in (0, 1, 2):
        _path(b, R, n_ccx, BANK_A, w, False)
    _mux(b, n_ccx, NB, R["P0"], R["P1"], R["P2"], DFB, DFS, R["MUT"][0])
    for w in (0, 1, 2):
        _path(b, R, n_ccx, BANK_A, w, True)
    # 5. clear the decoded flags from the restored source: forward flags
    # recomputed from bank A provably equal (s, b) on the swept domain
    # (b == g parity by branch shape, s == the AND — same identities fit
    # proves for the recipe), so XORing them out clears DFS/DFB exactly;
    # the mirror then clears BF/CF/SF. N is already zero here, so the
    # decode mirror cannot run — this S0-derived clear is the only exact
    # option (documented, not incidental).
    _flags(b, n_ccx, BANK_A, FLG, False)
    b.cx(FLG["SF"], DFS)
    b.cx(FLG["BF"], DFB)
    _flags(b, n_ccx, BANK_A, FLG, True)
    b.x(R["ONE"][0])
    for q in WTEMPS:
        b.free(q)
    return b, {"ccx": n_ccx[0], "history_qubits": 0}


def decode_reverse_spec(p: int = P_TOY, depths: tuple[int, ...] = (2, 3)):
    """TaskSpec: bank A gets the inverted source, bank B returns to zero.

    inputs=() is deliberate, not a gap: the circuit CONSUMES the next
    state (bank B ends zeroed), and the harness requires listed inputs to
    be bit-identical after. Every output bit (restored source in A, zeros
    in B) is still checked against the perm on all swept inputs — the
    proof is equally strong, the contract honest about consumption.
    """
    from .kickmix import TaskSpec

    def _perm(bits: dict[int, int]) -> dict[int, int]:
        out = dict(bits)
        ns = tuple(from_bits([bits.get(q, 0) for q in BANK_B[k]])
                   for k in ("Z", "F", "G", "FV", "GV"))
        inv = _invert_state(ns, p, depths)
        for v, k in zip(inv["src"], ("Z", "F", "G", "FV", "GV")):
            for i, bit in enumerate(to_bits(v, {"Z": WZ, "F": WF, "G": WG,
                                                 "FV": WFV, "GV": WGV}[k])):
                out[BANK_A[k][i]] = bit
        for q in BANK_B_IDS:
            out[q] = 0
        return out

    return TaskSpec(num_qubits=N_QUBITS_W, inputs=(),
                    outputs=BANK_A_IDS + BANK_B_IDS, ancillas=WTEMPS + DF_IDS,
                    target_perm=_perm)
