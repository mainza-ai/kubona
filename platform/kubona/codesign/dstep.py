"""Naive history-stack divstep step (the baseline decoding beats, proven).

One reversible divstep transition at toy scale (p = 13) as a kickmix
program: branch flags from cheap tests (swap visible in v2, counter rule,
g parity), all three update paths computed unconditionally, Fredkin-free
conditional-XOR routing into the next state, (s, b) pushed on a 2-qubit
history stack. Reverse = recompute paths + re-route (XOR cancels) with the
STORED bits, then re-derived flags clear the stack copies. Widths are fixed
two's complement (Z5/F6/G6/FV7/GV7); swept states are model-checked to fit
every intermediate and result (no silent wrap). This is deliberately the
NAIVE variant: history persists (2 qubits/step, measured) where the decoded
circuit re-derives (divwalk + decoder circuits, costed in HISTORY_QUBITS).
"""

from __future__ import annotations

from .kickmix import Builder

P_TOY = 13
WZ, WF, WG, WFV, WGV = 5, 6, 6, 7, 7


def _signed(v: int, bits: int) -> int:
    v %= 1 << bits
    return v - (1 << bits) if v >= (1 << (bits - 1)) else v


def fit_divstep_state(state: tuple[int, int, int, int, int], p: int = P_TOY,
                      depths: tuple[int, ...] = (2, 3)) -> dict:
    """Model-check one transition for width/borrow/parity fit.

    Runs divwalk.step, then asserts every intermediate (sums, differences,
    halvings, doublings, negations) and every result fits the fixed signed
    widths, halvings are even, and the branch is determined. Refuses
    violators (fail-closed swept domain, reported coverage in tests).

    The decode recipe is prime-specific: s is visible in f'v bit 1
    (prime-generic); the s == 1 branch, zero rule, and z-rule are
    prime-generic; otherwise the depth-j rule for THIS prime decides.
    `depths` is the proven ambiguous-depth domain (p = 13 proves 2-3;
    anything outside refuses — the paper-prime rules do NOT apply here,
    and neither do other primes' rules).
    """
    from .divwalk import step
    if len(state) != 5:
        raise ValueError(f"state must be a 5-tuple: refusing {state!r}")
    if isinstance(p, bool) or not isinstance(p, int) or p < 3 or p % 2 == 0:
        raise ValueError(f"p must be an odd prime >= 3, got {p!r}: refusing")
    if (not isinstance(depths, tuple) or not depths or
            any(isinstance(j, bool) or not isinstance(j, int) or j < 2 for j in depths)):
        raise ValueError(f"depths must be a non-empty tuple of ints >= 2, got {depths!r}: refusing")
    z, f, g, fv, gv = state
    new_state, s, b = step(state)
    zp, fp, gp, fvp, gvp = new_state
    R = {"Z": WZ, "F": WF, "G": WG, "FV": WFV, "GV": WGV, "SUMA": 8}
    vals = {"z": z, "f": f, "g": g, "fv": fv, "gv": gv,
            "zp": zp, "fp": fp, "gp": gp, "fvp": fvp, "gvp": gvp,
            "g+f": g + f, "g-f": g - f, "-z": -z, "2fv": 2 * fv, "2gv": 2 * gv,
            "gv+fv": gv + fv, "gv-fv": gv - fv, "z+1": z + 1}
    # g+-f widen into 8-bit SUMA (checked there: the 6-bit G check the first
    # draft used refuses valid states like p=17's g+f=32 — caught live);
    # gv+-fv copy out through a 7-bit lane, so they stay GV-checked;
    # halved/doubled/negated results are pinned by their destination widths.
    kinds = {"z": "Z", "zp": "Z", "-z": "Z", "z+1": "Z", "f": "F", "fp": "F",
             "g": "G", "gp": "G", "g+f": "SUMA", "g-f": "SUMA", "fv": "FV",
             "fvp": "FV", "2fv": "FV", "2gv": "FV", "gv": "GV", "gvp": "GV",
             "gv+fv": "GV", "gv-fv": "GV"}
    for name, v in vals.items():
        lo, hi = -(1 << (R[kinds[name]] - 1)), (1 << (R[kinds[name]] - 1)) - 1
        if not (lo <= v <= hi):
            raise ValueError(f"{name}={v} outside {kinds[name]} range: refusing")
    if b == 1 and g % 2 == 0:
        raise ValueError("odd-branch bit with even g (implementation bug): refusing")
    if s == 1 and (b != 1 or z < 0):
        raise ValueError("swap shape broken (implementation bug): refusing")
    if g != 0 and b == 0 and (g // 2) * 2 != g:
        raise ValueError("even-branch halving inexact (implementation bug): refusing")
    # Decode identities (the history-free recipe, proven here on the domain):
    # s is visible in f'v bit 1 (f'v always even; v2 == 1 iff swap); the
    # s == 1 branch forces b; g' == 0 forces b (zero rule); a nonswap landing
    # zp >= 1 forces b == 0 (z-rule); otherwise (zp <= 0, gp != 0) the
    # depth-j rule for THIS prime decides (depths are the proven domain).
    from .divsteps import eval_decoder
    if fvp % 2 != 0:
        raise ValueError("f'v odd (Lemma A1 parity broken): refusing")
    if s != ((fvp >> 1) & 1):
        raise ValueError("swap not visible in f'v bit 1 (Lemma A4.1): refusing")
    if s == 0:
        if gp == 0:
            if b != 1:
                raise ValueError("zero rule broken (g'==0 forces b): refusing")
        elif zp >= 1:
            if b != 0:
                raise ValueError("z-rule broken (zp>=1 forces b==0): refusing")
        else:
            from .divwalk import v2
            j = v2(fvp)
            if j not in depths:
                raise ValueError(f"ambiguous depth {j} outside proven {tuple(depths)}: refusing")
            M = 1 << j
            q = (fvp // (1 << j)) % M
            bits = {}
            for i in range(1, j):
                bits[f"f{i}"] = (fp % M >> i) & 1
                bits[f"q{i}"] = (q >> i) & 1
            if b != eval_decoder(j, bits, p=p):
                raise ValueError(f"p={p} decoder rule mismatch: refusing")
    return {"branch": "swap" if s == 1 else ("odd" if b == 1 else "even"),
            "s": s, "b": b, "next": new_state}


#: Fixed register file (two's complement widths Z5/F6/G6/FV7/GV7).
REG = {
    "Z": [0, 1, 2, 3, 4], "F": [5, 6, 7, 8, 9, 10],
    "G": [11, 12, 13, 14, 15, 16], "FV": list(range(17, 24)),
    "GV": list(range(24, 31)),
    "NZ": [31, 32, 33, 34, 35], "NF": [36, 37, 38, 39, 40, 41],
    "NG": [42, 43, 44, 45, 46, 47], "NFV": list(range(48, 55)),
    "NGV": list(range(55, 62)),
    "BF": [62], "CF": [63], "SF": [64], "HIST": [65, 66],
    "P0": list(range(67, 98)), "P1": list(range(98, 129)),
    "P2": list(range(129, 160)),
    "SUMA": list(range(160, 168)), "SUMB": list(range(168, 176)),
    "CARR": list(range(176, 184)), "ONE": [184], "ZERO": [185],
    "MUT": [186],
}
N_QUBITS = 187
STATE_IDS = tuple(REG["Z"] + REG["F"] + REG["G"] + REG["FV"] + REG["GV"])
OUT_IDS = tuple(REG["NZ"] + REG["NF"] + REG["NG"] + REG["NFV"] + REG["NGV"])
TEMPS = tuple(REG["P0"] + REG["P1"] + REG["P2"] + REG["SUMA"] + REG["SUMB"]
              + REG["CARR"] + REG["ONE"] + REG["ZERO"] + REG["MUT"]
              + REG["BF"] + REG["CF"] + REG["SF"])


def _split(regs: list[int], widths: list[int]) -> list[list[int]]:
    out, pos = [], 0
    for w in widths:
        out.append(regs[pos:pos + w])
        pos += w
    return out


def to_bits(v: int, bits: int) -> list[int]:
    """Signed integer to two's complement bit list (LSB first)."""
    if not isinstance(bits, int) or bits < 1:
        raise ValueError(f"bits must be positive int, got {bits!r}: refusing")
    return [(v % (1 << bits) >> i) & 1 for i in range(bits)]


def from_bits(bs: list[int], signed: bool = True) -> int:
    """Bit list (LSB first) back to integer (two's complement if signed)."""
    v = sum((b & 1) << i for i, b in enumerate(bs))
    if signed and bs and bs[-1]:
        v -= 1 << len(bs)
    return v


def _emit_step(b, R, n_ccx, SRC, DST, FLG, HST, roundtrip: bool = False) -> None:
    """One divstep transition, bank-parameterized (naive/decoded/walk share it).

    SRC/DST map "Z","F","G","FV","GV" to register lists (source state bank /
    next-state bank); FLG maps "BF","CF","SF" to flag regs; HST is the
    2-qubit history bank or None (decoded: flags live only around the mux,
    nothing persists). Emits flags, optional history push, the three
    unconditional paths, the conditional-XOR mux, and path cleanup; with
    roundtrip=True appends the reverse (recompute, re-mux cancel, uncompute,
    plus history/flag clearing).     ONE must already read 1; temps alloc'd.
    """
    # ---- assembly: flags, [push], paths, mux, path cleanup ----
    N = DST["Z"] + DST["F"] + DST["G"] + DST["FV"] + DST["GV"]
    # Selectors ride the history copies when they persist (naive) and the
    # live flags when nothing persists (decoded) — never the evolving N
    # (caught live: N-reading temps cannot uncompute). Live flags must stay
    # SET through the mux: the naive mirror clears early only because the
    # HIST copies already carry the selectors (decoded caught live here —
    # early mirror zeroes the mux condition on every state).
    HB, HS = ((HST[1], HST[0]) if HST is not None
              else (FLG["BF"], FLG["SF"]))
    _flags(b, n_ccx, SRC, FLG, False)
    if HST is not None:
        _push(b, FLG, HST)
        _flags(b, n_ccx, SRC, FLG, True)  # mirror: AND re-fires while intact
    for w in (0, 1, 2):
        _path(b, R, n_ccx, SRC, w, False)
    _mux(b, n_ccx, N, R["P0"], R["P1"], R["P2"], HB, HS, R["MUT"][0])
    for w in (0, 1, 2):
        _path(b, R, n_ccx, SRC, w, True)
    if HST is None:
        _flags(b, n_ccx, SRC, FLG, True)  # decoded: clear live flags after
    if roundtrip:
        # reverse: recompute paths, re-mux (DST cancels), uncompute paths,
        # then clear the persisted selectors — history copies by re-push,
        # decoded by recompute-then-mirror around the reverse mux (live
        # flags were cleared after the forward mux, so they must be
        # re-derived; reusing stale-cleared flags would route nothing).
        if HST is None:
            _flags(b, n_ccx, SRC, FLG, False)
        for w in (0, 1, 2):
            _path(b, R, n_ccx, SRC, w, False)
        _mux(b, n_ccx, N, R["P0"], R["P1"], R["P2"], HB, HS, R["MUT"][0])
        for w in (0, 1, 2):
            _path(b, R, n_ccx, SRC, w, True)
        if HST is not None:
            _flags(b, n_ccx, SRC, FLG, False)
            _push(b, FLG, HST)  # XORs the recomputed flags back out of HIST
            _flags(b, n_ccx, SRC, FLG, True)
        else:
            _flags(b, n_ccx, SRC, FLG, True)
    # DST/HIST ride plain (never alloc'd/freed, like N): DST persists
    # forward, HIST persists iff it exists; roundtrip returns both to zero.

def _add(b, R, n_ccx, A: list[int], B: list[int]) -> None:
    from .horner import emit_add
    n_ccx[0] += emit_add(b, A, B, R["CARR"][:len(B)])


def _sub(b, R, n_ccx, A: list[int], B: list[int]) -> None:
    from .horner import emit_sub
    n_ccx[0] += emit_sub(b, A, B, R["CARR"][:len(B)])



def _cp(b, dst: list[int], src: list[int]) -> None:
    for bq, aq in zip(dst, src):
        b.cx(aq, bq)



def _ext(reg: list[int], n: int) -> list[int]:
    # SIGN-extending pad: B reads f mod 2^n exactly (needs |f| < 2^{n-1},
    # model-checked by fit), so the modular sum is off from the intended
    # value by exactly 2^n iff the sign bit is set — repaired by one CX
    # (_scorr). Invariant: pad width n and scorr bit n always agree.
    # Zero pads read f mod 2^{n-1} (quarter-modulus error, unfixable by one
    # CX — caught live twice); shrinking is refused, never clipped.
    if len(reg) > n:
        raise ValueError("extension shrinks (implementation bug): refusing")
    return list(reg) + [reg[-1]] * (n - len(reg))



def _scorr(b, stop: int, accn: int) -> None:
    # Signed-operand correction: flip accumulator bit n iff the operand
    # sign bit is set. Must pair with a sign-extended _ext of matching
    # width (bit n); a mismatched bit silently double-corrects or misses
    # (both caught live on the way here).
    b.cx(stop, accn)



def _neg5(b, R, n_ccx, reg: list[int]) -> None:
    # Two's complement negate in place width: ~reg + 1, the +1 as a
    # one-bit-extended constant (B must be one shorter than A).
    for q in reg:
        b.x(q)
    _add(b, R, n_ccx, reg, [R["ONE"][0]] + [R["ZERO"][0]] * (len(reg) - 2))


def _inc(b, R, n_ccx, reg: list[int], n: int) -> None:
    _add(b, R, n_ccx, reg, [R["ONE"][0]] + [R["ZERO"][0]] * (n - 1))



def _halve(b, dst: list[int], src: list[int]) -> None:
    for i in range(len(dst) - 1):
        b.cx(src[i + 1], dst[i])
    b.cx(src[-1], dst[-1])


def _double(b, dst: list[int], src: list[int]) -> None:
    # Arithmetic left shift in place width (model pins |2v| in range):
    # dst[0] untouched (fresh zero on compute, zero on uncompute),
    # dst[i] ^= src[i-1], sign bit dst[-1] ^= src[-1]. Self-mirroring.
    if len(dst) != len(src):
        raise ValueError("double needs equal widths (implementation bug): refusing")
    for i in range(1, len(dst) - 1):
        b.cx(src[i - 1], dst[i])
    b.cx(src[-1], dst[-1])



def _flags(b, n_ccx, SRC, FLG, undo: bool = False) -> None:
    # Forward computes (CX, X, CX, CCX); mirror runs REVERSED: the AND
    # must re-fire while its controls are still intact — re-firing in
    # forward order clears the controls first and the AND re-fire then
    # toggles by 0&0 (caught live: SF leaked on every swap input).
    # Whole sequence is self-inverse (every op is), so re-running forward
    # also clears — but only with intact controls; the mirror is required
    # wherever controls were consumed in between (none here today).
    ops = [("cx", (SRC["G"][0], FLG["BF"])), ("x", (FLG["CF"],)),
            ("cx", (SRC["Z"][-1], FLG["CF"])),
            ("ccx", (FLG["BF"], FLG["CF"], FLG["SF"]))]
    seq = ops if not undo else ops[::-1]
    for name, qs in seq:
        if name == "cx":
            b.cx(*qs)
        elif name == "x":
            b.x(qs[0])
        else:
            b.ccx(*qs)
            n_ccx[0] += 1


def _push(b, FLG, HST) -> None:
    b.cx(FLG["SF"], HST[0])
    b.cx(FLG["BF"], HST[1])



def _path(b, R, n_ccx, SRC, which: int, undo: bool) -> None:
    """One update path; undo=True mirrors it (all path temps to zero).

    Sum temps are built, used, and unbuilt WITHIN each phase: forward
    leaves only result regs dirty (mux reads those); mirror rebuilds
    sums around each result clear. Sharing sum temps across paths
    without this discipline silently cross-contaminates (caught live:
    every state failed the same way). Copies/shifts/negations re-emit
    (self-inverse). Operands (S values) stay intact throughout.
    """
    P = [R["P0"], R["P1"], R["P2"]][which]
    nz, nf, ng, nfv, ngv = _split(P, [5, 6, 6, 7, 7])
    Z, F, G, FV, GV = SRC["Z"], SRC["F"], SRC["G"], SRC["FV"], SRC["GV"]

    def _sumA_build() -> None:
        # Widening load WITH sign extension (a zero-filled negative
        # silently changes value — caught live on every negative input).
        for i in range(6):
            b.cx(G[i], R["SUMA"][i])
        b.cx(G[5], R["SUMA"][6])
        b.cx(G[5], R["SUMA"][7])
        if which == 1:
            _add(b, R, n_ccx, R["SUMA"], _ext(F, 7))
            _scorr(b, F[5], R["SUMA"][7])
        else:
            _sub(b, R, n_ccx, R["SUMA"], _ext(F, 7))
            _scorr(b, F[5], R["SUMA"][7])

    def _sumA_clear() -> None:
        if which == 1:
            _sub(b, R, n_ccx, R["SUMA"], _ext(F, 7))
            _scorr(b, F[5], R["SUMA"][7])
        else:
            _add(b, R, n_ccx, R["SUMA"], _ext(F, 7))
            _scorr(b, F[5], R["SUMA"][7])
        for i in range(6):
            b.cx(G[i], R["SUMA"][i])
        b.cx(G[5], R["SUMA"][6])
        b.cx(G[5], R["SUMA"][7])

    def _sumB_build() -> None:
        for i in range(7):
            b.cx(GV[i], R["SUMB"][i])
        b.cx(GV[6], R["SUMB"][7])

        if which == 1:
            _add(b, R, n_ccx, R["SUMB"], _ext(FV, 7))
            _scorr(b, FV[6], R["SUMB"][7])
        else:
            _sub(b, R, n_ccx, R["SUMB"], _ext(FV, 7))
            _scorr(b, FV[6], R["SUMB"][7])

    def _sumB_clear() -> None:
        if which == 1:
            _sub(b, R, n_ccx, R["SUMB"], _ext(FV, 7))
            _scorr(b, FV[6], R["SUMB"][7])
        else:
            _add(b, R, n_ccx, R["SUMB"], _ext(FV, 7))
            _scorr(b, FV[6], R["SUMB"][7])
        for i in range(7):
            b.cx(GV[i], R["SUMB"][i])
        b.cx(GV[6], R["SUMB"][7])


    if not undo:
        _cp(b, nz, Z)
        if which == 2:
            _neg5(b, R, n_ccx, nz)
        else:
            _inc(b, R, n_ccx, nz, 4)
        _cp(b, nf, F if which != 2 else G)
        if which == 0:
            _halve(b, ng, G)
            _double(b, nfv, FV)
            _cp(b, ngv, GV)
            return
        _sumA_build()
        _halve(b, ng, R["SUMA"])
        _sumA_clear()
        _double(b, nfv, FV if which == 1 else GV)
        _sumB_build()
        _cp(b, ngv, R["SUMB"])
        _sumB_clear()
        return
    # mirror: results first (rebuild sums around each result clear).
    _cp(b, nf, F if which != 2 else G)
    if which == 0:
        _sub(b, R, n_ccx, nz, [R["ONE"][0]] + [R["ZERO"][0]] * 3)  # un-inc
        _halve(b, ng, G)
        _double(b, nfv, FV)
        _cp(b, ngv, GV)
        _cp(b, nz, Z)
        return
    _sumA_build()
    _halve(b, ng, R["SUMA"])
    _sumA_clear()
    _double(b, nfv, FV if which == 1 else GV)
    _sumB_build()
    _cp(b, ngv, R["SUMB"])
    _sumB_clear()
    # mirror tail is exact reverse of forward (restore value first,
    # THEN unload the copy — unloading a not-yet-restored register
    # XORs garbage in and subtracts from it; caught live twice now).
    if which == 2:
        _neg5(b, R, n_ccx, nz)  # -(-z) = z (before unload, not after)
    else:
        _sub(b, R, n_ccx, nz, [R["ONE"][0]] + [R["ZERO"][0]] * 3)  # (z+1)-1 = z
    _cp(b, nz, Z)  # unload to zero (nz holds z again here)

def _mux(b, n_ccx, N: list[int], P0: list[int], P1: list[int], P2: list[int],
         HB: int, HS: int, MT: int) -> None:
    """Two-stage conditional-XOR routing into N (self-inverting).

    N ^= P0; if b: N ^= (P0^P1); if s: N ^= (P1^P2) — every selector
    reads PATH regs only, never the evolving N. Reading N for the
    second stage (as a first draft did) makes the temp uncomputable:
    once N updates, its old value is erased and the temp cannot clear
    (caught live as a corrupted next state with clean temps). Valid
    combos (s implies b): (0,0)->E, (0,1)->O, (1,1)->S.
    """
    for i in range(31):  # N ^= P0 (even base)
        b.cx(P0[i], N[i])
    # Per-bit compute/use/uncompute sharing one temp: fusing the three
    # loops (compute-all, select-all, uncompute-all) breaks scoping —
    # the temp would accumulate all 31 parities instead of one bit
    # (caught live as cross-bit corruption with clean temps).
    for i in range(31):  # if b: N ^= (P0 ^ P1)
        b.cx(P0[i], MT)
        b.cx(P1[i], MT)
        b.ccx(HB, MT, N[i])
        n_ccx[0] += 1
        b.cx(P0[i], MT)
        b.cx(P1[i], MT)
    for i in range(31):  # if s: N ^= (P1 ^ P2)
        b.cx(P1[i], MT)
        b.cx(P2[i], MT)
        b.ccx(HS, MT, N[i])
        n_ccx[0] += 1
        b.cx(P1[i], MT)
        b.cx(P2[i], MT)



def build_dstep(roundtrip: bool = False) -> tuple[object, dict]:
    """Naive history-stack divstep step (forward; +reverse if roundtrip).

    Forward: flags (b = g parity, c = [z >= 0], s = b&c) pushed to HIST,
    flags uncomputed; three update paths computed into P0/P1/P2; two-stage
    conditional-XOR routing (E base, b-select O, s-select S) into N; paths
    uncomputed (mirror SUBs/ADDs, re-negations, unloads); N + HIST survive.
    Roundtrip appends: recompute paths, re-mux (N cancels to zero),
    uncompute paths, recompute flags, clear HIST, uncompute flags — all
    zero but S (reversibility proven, not just validity).
    All arithmetic is horner-emitter adds/subs (modular-exact) plus
    rewiring shifts and X-wrap negations; swept states pin exactness.
    Shares the bank-parameterized _emit_step core with the decoded step
    (dwalk.py) — one logic home, both proofs guard it.
    """
    from .kickmix import Builder
    R = REG
    b = Builder(N_QUBITS)
    for q in TEMPS:
        b.alloc(q)
    # ONE is the constant 1 bit for inc/negate/sub (read-only throughout;
    # loaded once here, unloaded before freeing — an all-zero ONE would
    # silently turn every +1 into +0, caught live as all-zero next states).
    b.x(R["ONE"][0])
    n_ccx = [0]
    SRC = {"Z": R["Z"], "F": R["F"], "G": R["G"], "FV": R["FV"], "GV": R["GV"]}
    DST = {"Z": R["NZ"], "F": R["NF"], "G": R["NG"], "FV": R["NFV"], "GV": R["NGV"]}
    FLG = {"BF": R["BF"][0], "CF": R["CF"][0], "SF": R["SF"][0]}
    _emit_step(b, R, n_ccx, SRC, DST, FLG, R["HIST"], roundtrip=roundtrip)
    b.x(R["ONE"][0])  # unload the constant (dirty-free would refuse 1)
    for q in TEMPS:
        b.free(q)
    return b, {"ccx": n_ccx[0], "roundtrip": roundtrip,
               "history_qubits": 0 if roundtrip else 2}


def _pack_state(state: tuple[int, int, int, int, int]) -> dict[int, int]:
    init: dict[int, int] = {}
    for v, (reg, w) in zip(state, (("Z", WZ), ("F", WF), ("G", WG),
                                     ("FV", WFV), ("GV", WGV))):
        for i, bit in enumerate(to_bits(v, w)):
            init[REG[reg][i]] = bit
    return init


def dstep_spec_fwd():
    """TaskSpec: N + HIST get the modeled step (S preserved)."""
    from .divwalk import step
    from .kickmix import TaskSpec

    def _perm(bits: dict[int, int]) -> dict[int, int]:
        out = dict(bits)
        state = tuple(from_bits([bits.get(q, 0) for q in REG[k]])
                       for k in ("Z", "F", "G", "FV", "GV"))
        new_state, s, b = step(state)
        for v, (reg, w) in zip(new_state, (("NZ", WZ), ("NF", WF),
                                             ("NG", WG), ("NFV", WFV),
                                             ("NGV", WGV))):
            for i, bit in enumerate(to_bits(v, w)):
                out[REG[reg][i]] = bit
        out[REG["HIST"][0]] = s
        out[REG["HIST"][1]] = b
        return out

    return TaskSpec(num_qubits=N_QUBITS, inputs=STATE_IDS,
                    outputs=OUT_IDS + tuple(REG["HIST"]), ancillas=TEMPS,
                    target_perm=_perm)


def dstep_spec_rt():
    """TaskSpec: roundtrip returns everything but S to zero (S preserved)."""
    from .kickmix import TaskSpec
    return TaskSpec(num_qubits=N_QUBITS, inputs=STATE_IDS, outputs=(),
                    ancillas=OUT_IDS + tuple(REG["HIST"]) + TEMPS,
                    target_perm=lambda bits: dict(bits))


def harvest_states(p: int = P_TOY) -> list[tuple[int, int, int, int, int]]:
    """All intermediate active states of full walks (valid by construction)."""
    from .divwalk import step
    out = []
    w = 1
    while True:
        state = (0, p, w, 0, 1)
        while True:
            z, f, g, fv, gv = state
            if g == 0:
                break
            out.append(state)
            state = step(state)[0]
        w += 1
        if w >= p:
            break
    return out
