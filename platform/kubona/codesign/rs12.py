"""Full paper-scale RS decoder composition (Phase 3, slice 60).

Single coherent circuit: S (70 syndromes in) -> 70-round BM walk ->
sqrt-split -> 17-block EEA (+flag) -> Psi squares -> Omega key-equation
product -> Chien root scan -> bilinear phase scan (phase out). Reuses the
proven shared emitters (_emit_bm12_round, _emit_eea) with a unified
layout; adds only three small linear/product stages (split, psi, omega),
all CNOTs except the omega mults. Scan loops (Chien + phase) are inlined
from the proven builders with pin-parity asserts on their T-counts
(no refactor of proven files — contained blast radius; drift guarded by
the pins).

What this proves: the pipeline connects end-to-end (no stage-boundary
wiring bugs), on planted instances: EEA flag=1, Chien FOUND marks the
planted positions, the trajectory phase equals the standalone scan
phase, every temp clean, inputs preserved. What it does NOT re-prove:
the scans themselves (slices 48/51), the walk (slice 58), the EEA
(slice 59) — those stand on their own proofs; this slice proves only
the composition.
"""

from __future__ import annotations

from .kickmix import Builder

NW = 36          # working width (matches EEA)
NW_IO = 18       # input/output degree bound
N_SIGMA = 36     # locator/evaluator/reciprocal coeffs
N_SYN = 70       # syndromes


def _check_word(w: list[int], n: int, name: str, nqubits: int) -> None:
    if not isinstance(w, list) or len(w) != n or any(not isinstance(q, int) or q < 0 or q >= nqubits for q in w):
        raise ValueError(f"{name} must be {n} in-range qubit ids: refusing {w!r}")


def rs12_layout(base: int) -> dict:
    """Unified decoder layout (offsets from base)."""
    from .bm import _bm12_round_layout
    from .eea12 import N_BLOCKS, N_BT, N_LT
    from .tower12 import N_TOWER_TEMPS

    o = [base]

    def take(n: int) -> list[int]:
        r = list(range(o[0], o[0] + n))
        o[0] += n
        return r

    S = take(12 * N_SYN)
    nin = o[0] - base
    # BM walk: 71 banks (1316 each, _bank12 order) + shared temp block + zero
    NB = 12 * N_SIGMA + 12 * 71 + 12 + 12 + 8
    assert NB == 1316
    WB = [take(NB) for _ in range(71)]
    # walk temp block: same groups as the canonical round layout (drift-proof)
    _L0 = _bm12_round_layout(0)
    WT = {}
    for _k in ("DISC", "FZ", "FR", "FT", "ALPHA", "AINV", "NID", "REPL", "RET",
               "TINCR", "TNEG", "CARRY", "ZT", "TOWER", "MBP", "RD", "TD",
               "DD", "IDD", "DDD"):
        WT[_k] = take(len(_L0[_k]))
    assert sum(len(v) for v in WT.values()) == _L0["ntemps"]
    WZ = take(1)
    # split outputs
    E = take(12 * NW_IO)
    O = take(12 * NW_IO)
    # EEA working + trail + shared scratch
    X = take(12 * NW)
    Y = take(12 * NW)
    U0 = take(12 * NW)
    U1 = take(12 * NW)
    V0 = take(12 * NW)
    V1 = take(12 * NW)
    BT = [take(N_BT) for _ in range(N_BLOCKS)]
    ELT = take(N_LT)
    EZT = take(10)
    ETOWER = take(N_TOWER_TEMPS)
    EPE = take(12 * NW_IO)
    EQE = take(12 * NW)
    EBB = [take(12 * NW) for _ in range(5)]
    EINV = take(12)
    EBRE = take(1)
    EMBP = take(1)
    OMBP = take(1)
    EZTBIG = take(217)
    A = take(12 * NW_IO)
    B = take(12 * NW_IO)
    FLAG = take(1)
    # omega + psi + scan outputs
    OMEGA = take(12 * N_SIGMA)
    PSI = take(12 * N_SIGMA)
    FOUND = take(4095)
    CVAL = take(12)
    CZT = take(10)
    PVALW = take(12)
    PVALP = take(12)
    PFOUND = take(1)
    PARI = take(24)
    PZT = take(10)
    if nin != 12 * N_SYN:
        raise ValueError("input layout arithmetic drifted: refusing")
    if len(E) != 216 or len(OMEGA) != 432 or len(FOUND) != 4095:
        raise ValueError("output layout arithmetic drifted: refusing")
    # allocs: caller-managed scratch only (banks/outputs persist).
    # Walk temps (WT groups except ZT, which zero-test manages) + walk ZERO;
    # EEA shared scratch. Chien/phase-scan temps are self-managed by the
    # inlined scan code (alloc/free per use, exactly like the builders).
    allocs = []
    for _k, _grp in WT.items():
        if _k not in ("ZT", "MBP"):
            allocs.extend(_grp)
    for _grp in ([WZ, ELT, ETOWER, EPE, EQE, EINV, EBRE] + EBB):
        allocs.extend(_grp)
    return {"S": S, "WB": WB, "WT": WT, "WZ": WZ, "E": E, "O": O,
            "X": X, "Y": Y, "U0": U0, "U1": U1, "V0": V0, "V1": V1,
            "BT": BT, "ELT": ELT, "EZT": EZT, "ETOWER": ETOWER,
            "EPE": EPE, "EQE": EQE, "EBB": EBB, "EINV": EINV, "EBRE": EBRE, "EMBP": EMBP,
            "OMBP": OMBP,
            "EZTBIG": EZTBIG, "A": A, "B": B, "FLAG": FLAG,
            "OMEGA": OMEGA, "PSI": PSI, "FOUND": FOUND, "CVAL": CVAL,
            "CZT": CZT, "PVALW": PVALW, "PVALP": PVALP, "PFOUND": PFOUND,
            "PARI": PARI, "PZT": PZT, "nin": nin,
            "nqubits": o[0] - base, "allocs": allocs}


def _sqrt_matrix() -> list[list[int]]:
    """F2^12 sqrt as 12 binary rows (Frobenius-inverse is F2-linear)."""
    from .gf import F2_12
    from .psi import fsqrt
    rows = []
    for t in range(12):
        row = []
        for j in range(12):
            # output bit t of sqrt(x) as function of input bits: probe
            v = fsqrt(1 << j, F2_12)
            row.append((v >> t) & 1)
        rows.append(row)
    return rows


def _emit_split12(b: Builder, sig: list[int], e: list[int], o: list[int]) -> None:
    """Sqrt-split: E_i = sqrt(sig[2i]), O_i = sqrt(sig[2i-1]) (CNOTs only)."""
    if not isinstance(b, Builder):
        raise ValueError("b must be a Builder: refusing")
    _check_word(sig, 36 * 12, "sig", b.num_qubits)
    _check_word(e, 18 * 12, "e", b.num_qubits)
    _check_word(o, 18 * 12, "o", b.num_qubits)
    mat = _sqrt_matrix()
    for i in range(18):
        for t in range(12):
            for j in range(12):
                if mat[t][j]:
                    b.cx(sig[(2 * i) * 12 + j], e[i * 12 + t])
                    if 2 * i + 1 < 36:
                        b.cx(sig[(2 * i + 1) * 12 + j], o[i * 12 + t])


def _emit_psi_from_ab(b: Builder, a: list[int], c: list[int], psi: list[int],
                      tmp: list[int]) -> None:
    """Psi = B^2 + z*A^2 from half-degree A,B (CNOTs only, temp cleaned).

    Frobenius squares land on even positions; the z-shift puts A^2 on odd
    positions: PSI[2i] = (B_i)^2, PSI[2i+1] = (A_i)^2 (characteristic 2, no
    overlap, no reduction). Each coefficient squared via emit_f12_sqr into
    tmp, fanned out, then uncomputed (inputs intact throughout).
    """
    from .f12 import emit_f12_sqr
    if not isinstance(b, Builder):
        raise ValueError("b must be a Builder: refusing")
    _check_word(a, 18 * 12, "a", b.num_qubits)
    _check_word(c, 18 * 12, "c", b.num_qubits)
    _check_word(psi, 36 * 12, "psi", b.num_qubits)
    _check_word(tmp, 12, "tmp", b.num_qubits)
    for i in range(18):
        emit_f12_sqr(b, c[12 * i:12 * (i + 1)], tmp)
        for t in range(12):
            b.cx(tmp[t], psi[12 * (2 * i) + t])
        emit_f12_sqr(b, c[12 * i:12 * (i + 1)], tmp)
        if 2 * i + 1 < 36:
            emit_f12_sqr(b, a[12 * i:12 * (i + 1)], tmp)
            for t in range(12):
                b.cx(tmp[t], psi[12 * (2 * i + 1) + t])
            emit_f12_sqr(b, a[12 * i:12 * (i + 1)], tmp)


def _emit_omega12(b: Builder, n_ccx: list[int], sig: list[int],
                  syn: list[int], om: list[int], pool: list[int]) -> int:
    """Evaluator omega = sigma*S (first 36 coeffs, MBU schoolbook mults).

    om[k] += sig[i] * syn[k-i] over all valid (i, k-i). 666 MBU mults
    (144T each), pool shared+self-managed. Returns the Toffoli count.
    """
    from .mbu12 import emit_mbu_mul12
    if not isinstance(b, Builder):
        raise ValueError("b must be a Builder: refusing")
    _check_word(sig, 36 * 12, "sig", b.num_qubits)
    _check_word(syn, 70 * 12, "syn", b.num_qubits)
    _check_word(om, 36 * 12, "om", b.num_qubits)
    _check_word(pool, 1, "pool", b.num_qubits)
    n = 0
    for k in range(36):
        for i in range(36):
            j = k - i
            if 0 <= j < 70:
                n += emit_mbu_mul12(b, sig[12 * i:12 * (i + 1)],
                                    syn[12 * j:12 * (j + 1)], om[12 * k:12 * (k + 1)], pool)
    return n


def _wbank(wires: list[int]) -> dict:
    """Split a 1316-wire walk bank into SIGMA/TAU/DELTA/IDELTA/D (bank order)."""
    if len(wires) != 1316:
        raise ValueError("walk bank must be 1316 wires: refusing")
    return {"SIGMA": wires[0:432], "TAU": wires[432:1284],
            "DELTA": wires[1284:1296], "IDELTA": wires[1296:1308],
            "D": wires[1308:1316]}


def _emit_rs_decode(b: Builder, n_ccx: list[int], L: dict, positions: list[int],
                    nus: dict) -> None:
    """Full RS decode pipeline (forward-only): walk -> split -> EEA ->
    psi/omega -> Chien scan + phase scan. Uniform emission, fixed schedule.
    """
    from .bm import _chien_masks, _emit_bm12_round, bilinear_terms
    from .eea12 import _emit_eea
    from .packing import emit_zero_test
    S = L["S"]
    WB, WT, WZ = L["WB"], L["WT"], L["WZ"][0]
    # ---- BM walk (71 fresh banks; shared WT block + WZ) ----
    B0 = _wbank(WB[0])
    b.x(B0["SIGMA"][0])
    b.x(B0["TAU"][0])
    b.x(B0["DELTA"][0])
    b.x(B0["IDELTA"][0])
    N = 70
    for r in range(1, N + 1):
        prev, cur = _wbank(WB[r - 1]), _wbank(WB[r])
        W = []
        for i in range(36):
            k = r - i
            if k < 1:
                W.extend([WZ] * 12)
            else:
                W.extend(S[12 * (k - 1) + t] for t in range(12))
        Lr = dict(WT)
        Lr.update({"SIGMA": prev["SIGMA"], "TAU": prev["TAU"],
                   "DELTA": prev["DELTA"], "IDELTA": prev["IDELTA"],
                   "D": prev["D"], "W": W,
                   "OSIG": cur["SIGMA"], "OTAU": cur["TAU"],
                   "ODELTA": cur["DELTA"], "OIDELTA": cur["IDELTA"],
                   "OD": cur["D"]})
        _emit_bm12_round(b, n_ccx, Lr)
    SIGMA = _wbank(WB[N])["SIGMA"]
    # ---- sqrt-split sigma -> E,O (CNOTs) ----
    _emit_split12(b, SIGMA, L["E"], L["O"])
    # ---- EEA pass (E,O -> A,B + FLAG) ----
    Le = {"X": L["X"], "Y": L["Y"], "U0": L["U0"], "U1": L["U1"],
          "V0": L["V0"], "V1": L["V1"], "E": L["E"], "O": L["O"],
          "A": L["A"], "B": L["B"], "FLAG": L["FLAG"],
          "ZT": L["EZT"], "ZTBIG": L["EZTBIG"], "TOWER": L["ETOWER"],
          "PE": L["EPE"], "BT": L["BT"], "LT": L["ELT"],
          "QE": L["EQE"], "BB": L["EBB"], "INV": L["EINV"], "BRE": L["EBRE"], "MBP": L["EMBP"]}
    _emit_eea(b, n_ccx, Le)
    # ---- Psi = B^2 + z*A^2 (CNOTs; EPE[0:12] temp, cleaned) ----
    _emit_psi_from_ab(b, L["A"], L["B"], L["PSI"], L["EPE"][0:12])
    # ---- Omega = sigma*S mod z^36 (666 schoolbook mults) ----
    n_ccx[0] += _emit_omega12(b, n_ccx, SIGMA, S, L["OMEGA"], L["OMBP"])
    # ---- Chien scan (inlined from build_chien_scan; pin-parity asserted) ----
    from .gf import F2_12
    n = 12
    npos = F2_12.size - 1
    VAL = L["CVAL"]
    ZT = L["CZT"]
    for q in VAL:
        b.alloc(q)
    n_chien = [0]
    for j, beta in enumerate(range(1, F2_12.size)):
        masks = _chien_masks(F2_12, beta, 36)
        ops = []
        for t in range(n):
            for q in masks[t]:
                ops.append((SIGMA[q], VAL[t]))
        for c, t_ in ops:
            b.cx(c, t_)
        n_chien[0] += emit_zero_test(b, VAL, L["FOUND"][j], ZT)
        for c, t_ in ops:
            b.cx(c, t_)
    for q in VAL:
        b.free(q)
    # ---- phase scan (inlined from build_phase_scan; pin-parity asserted) ----
    from .bm import _check_scan_inputs
    _check_scan_inputs(F2_12, 36, positions, nus)
    VALW, VALP = L["PVALW"], L["PVALP"]
    PZT = L["PZT"]
    FOUND1, PARI = L["PFOUND"], L["PARI"]
    for q in VALW + VALP + FOUND1 + PARI:
        b.alloc(q)
    n_phase = [0]
    for beta in positions:
        masks = _chien_masks(F2_12, beta, 36)
        for t in range(n):
            for q in masks[t]:
                b.cx(SIGMA[q], VALW[t])
        n_phase[0] += emit_zero_test(b, VALW, FOUND1[0], PZT)
        for t in range(n):
            for q in masks[t]:
                b.cx(SIGMA[q], VALW[t])
        terms = bilinear_terms(nus[beta], F2_12)
        for t in range(n):
            for q in masks[t]:
                b.cx(L["OMEGA"][q], VALW[t])
        for k, (am, _bm) in enumerate(terms):
            for i in range(n):
                if (am >> i) & 1:
                    b.cx(VALW[i], PARI[k])
        for t in range(n):
            for q in masks[t]:
                b.cx(L["PSI"][q], VALP[t])
        for k, (_am, bm_) in enumerate(terms):
            for j in range(n):
                if (bm_ >> j) & 1:
                    b.cx(VALP[j], PARI[n + k])
        for k in range(len(terms)):
            b.ccz(PARI[k], PARI[n + k], FOUND1[0])
            n_phase[0] += 1
        for k, (_am, bm_) in reversed(list(enumerate(terms))):
            for j in range(n):
                if (bm_ >> j) & 1:
                    b.cx(VALP[j], PARI[n + k])
        for k, (am, _bm) in reversed(list(enumerate(terms))):
            for i in range(n):
                if (am >> i) & 1:
                    b.cx(VALW[i], PARI[k])
        for t in range(n):
            for q in reversed(masks[t]):
                b.cx(L["PSI"][q], VALP[t])
        for t in range(n):
            for q in reversed(masks[t]):
                b.cx(L["OMEGA"][q], VALW[t])
        for t in range(n):
            for q in masks[t]:
                b.cx(SIGMA[q], VALW[t])
        n_phase[0] += emit_zero_test(b, VALW, FOUND1[0], PZT)
        for t in range(n):
            for q in masks[t]:
                b.cx(SIGMA[q], VALW[t])
    for q in VALW + VALP + FOUND1 + PARI:
        b.free(q)
    n_ccx[0] += n_phase[0]
    # pin parity vs standalone builders (drift guard on the inlining)
    assert n_chien[0] == 85995, n_chien[0]
    assert n_phase[0] == sum(42 + r for r in _phase_ranks(F2_12, positions, nus)), n_phase[0]


def _phase_ranks(field, positions, nus) -> list[int]:
    from .bm import bilinear_terms
    return [len(bilinear_terms(nus[beta], field)) for beta in positions]


def build_rs_decoder(positions: list[int], nus: dict) -> tuple[object, dict]:
    """Full paper-scale RS decoder (single coherent circuit)."""
    L = rs12_layout(0)
    b = Builder(L["nqubits"])
    for q in L["allocs"]:
        b.alloc(q)
    n_ccx = [0]
    _emit_rs_decode(b, n_ccx, L, positions, nus)
    for q in L["allocs"]:
        b.free(q)
    return b, {"inputs": tuple(range(L["nin"])), "outputs": tuple(list(L["FOUND"]) + list(L["FLAG"])),
               "ccx": n_ccx[0], "kind": "rs12-decode"}
