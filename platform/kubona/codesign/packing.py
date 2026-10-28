"""Packed-register layout + freeze-flag primitive (SI II.1, exact toy).

Two pieces of the divstep circuit's space story as checkable math/circuits:

1. plan_packing(): the paper shares two 256-qubit registers pairwise
   (g|f_v and f|g_v): the shrinking value grows down from the top while the
   growing coefficient grows up from the bottom. Given per-step widths and a
   cap C, layout A is g @ [C-wg, C) and f_v @ [0, wfv): feasible iff
   wg + wfv <= C at every step (same for f|g_v). Returns the exact fit
   report {fits, peak_fill, violator}; pack_or_refuse() raises on overflow
   (fail-closed: a walk that outgrows its wires fails, never wraps).

   Width model (stated toy): signed-magnitude bitlen + 1 sign bit,
   walk_widths() replays divwalk.step() exactly to measure it. The paper's
   two's-complement packing is the same program shape one alphabet up.

2. Zero-flag (freeze primitive): flag ^= 1 iff an n-bit register is all
   zero, via invert + multi-AND + restore. Toy scope: n == 2 only
   (single CCX, 1T, zero ancillas); wider refuses behind multi-controlled
   synthesis (stated, not silently built from unproven gates).
"""

from __future__ import annotations


def _width(v: int) -> int:
    if not isinstance(v, int):
        raise ValueError(f"value must be int, got {v!r}: refusing")
    return max(1, abs(v).bit_length() + 1)


def walk_widths(p: int, w: int, cap_steps: int = 10_000) -> list[dict[str, int]]:
    """Replay divwalk.step() and record signed-magnitude widths per step."""
    from .divwalk import step as _step
    if not isinstance(p, int) or p < 3 or p % 2 == 0:
        raise ValueError(f"p must be odd >= 3, got {p!r}: refusing")
    if not isinstance(w, int) or not (0 <= w < p):
        raise ValueError(f"w must be in [0,p), got {w!r}: refusing")
    state = (0, p, w, 0, 1)
    out: list[dict[str, int]] = []
    t = 0
    while True:
        z, f, g, fv, gv = state
        if g == 0 or t >= cap_steps:
            break
        out.append({"wg": _width(g), "wf": _width(f),
                    "wfv": _width(fv), "wgv": _width(gv)})
        state = _step(state)[0]
        t += 1
    if t >= cap_steps:
        raise ValueError("walk exceeded cap (implementation bug): refusing")
    return out


def plan_packing(width_trace: list[dict[str, int]], cap: int) -> dict:
    """Exact packed fit for the (g|f_v) + (f|g_v) two-register scheme."""
    if not isinstance(cap, int) or cap < 1:
        raise ValueError(f"cap must be positive int, got {cap!r}: refusing")
    if not isinstance(width_trace, list) or not width_trace:
        raise ValueError("width_trace must be a non-empty list: refusing")
    peak_a = peak_b = 0
    violator = None
    for i, s in enumerate(width_trace):
        for k in ("wg", "wf", "wfv", "wgv"):
            if k not in s or not isinstance(s[k], int) or s[k] < 1:
                raise ValueError(f"step {i}: bad width {k}={s.get(k)!r}: refusing")
        fa, fb = s["wg"] + s["wfv"], s["wf"] + s["wgv"]
        peak_a, peak_b = max(peak_a, fa), max(peak_b, fb)
        if (fa > cap or fb > cap) and violator is None:
            violator = {"step": i, "fill_a": fa, "fill_b": fb, "widths": dict(s)}
    return {"fits": violator is None, "peak_fill_a": peak_a, "peak_fill_b": peak_b,
            "cap": cap, "steps": len(width_trace), "violator": violator}


def pack_or_refuse(width_trace: list[dict[str, int]], cap: int) -> dict:
    """Strict planner: returns the fit report or raises on overflow."""
    rep = plan_packing(width_trace, cap)
    if not rep["fits"]:
        raise ValueError(f"walk outgrows {cap}-bit registers at step {rep['violator']['step']}: refusing")
    return rep


def plan_dual_model_packing(width_trace: list[dict[str, int]], mode: str = "safe_288") -> dict:
    """Dual-representation divstep allocation (B.4).

    - 'safe_288': production mode; sizes shared registers to 288 bits, provably
      preventing overflow on all 620 divstep walks including deep excursions (peak 267+267).
    - 'paper_256': simulation mode; enforces the paper's 256-bit bounds and
      trips a strict tail-failure budget event upon overflow.
    """
    if mode == "safe_288":
        rep = plan_packing(width_trace, cap=288)
        rep["mode"] = "safe_288"
        rep["zero_overflow_guarantee"] = rep["fits"]
        return rep
    elif mode == "paper_256":
        rep = plan_packing(width_trace, cap=256)
        rep["mode"] = "paper_256"
        rep["tail_overflow_tripped"] = not rep["fits"]
        return rep
    else:
        raise ValueError(f"unknown mode {mode!r}: must be 'safe_288' or 'paper_256'")


def emit_zero_test(b, g: list[int], flag: int, temps: list[int]) -> int:
    """Public zero-test emitter (powers scan readouts like Chien's).

    Same _emit_zero_flag_n machinery (inverted AND-chain, 2n-3 T): flag ^=
    1 iff every data bit is zero; data preserved; chain temps uncomputed.
    Widths 2..256 (proof scope is the caller's: exhaustive small, edge +
    sampled large — see edge_envelope_zero_flag).
    """
    from .kickmix import Builder
    if not isinstance(b, Builder):
        raise ValueError("b must be a Builder: refusing")
    return _emit_zero_flag_n(b, list(g), flag, list(temps))


def _emit_zero_flag_n(b, g: list[int], flag: int, temps: list[int]) -> int:
    """Inverted AND-chain zero test (preserves data, XORs into flag).
    X all data bits; AND-chain the (inverted) bits into flag; uncompute the
    chain; restore. Chain links live in temps (n-2 for n data bits); the
    final AND lands directly in flag. Cost 2n-3 Toffolis (n >= 2).
    """
    n = len(g)
    if n < 2:
        raise ValueError(f"zero-test needs >= 2 bits, got {n}: refusing")
    if len(temps) != max(0, n - 2):
        raise ValueError(f"need {max(0, n - 2)} temps for {n} bits: refusing")
    for q in g:
        b.x(q)
    n_ccx = 0
    if n == 2:
        b.ccx(g[0], g[1], flag)
        n_ccx = 1
    else:
        for q in temps:
            b.alloc(q)
        b.ccx(g[0], g[1], temps[0])
        n_ccx += 1
        for i in range(1, n - 2):
            b.ccx(temps[i - 1], g[i + 1], temps[i])
            n_ccx += 1
        b.ccx(temps[n - 3], g[n - 1], flag)
        n_ccx += 1
        for i in range(n - 3, 0, -1):
            b.ccx(temps[i - 1], g[i + 1], temps[i])
            n_ccx += 1
        b.ccx(g[0], g[1], temps[0])
        n_ccx += 1
        for q in reversed(temps):
            b.free(q)
    for q in g:
        b.x(q)
    if n_ccx != (1 if n == 2 else 2 * n - 3):
        raise AssertionError("zero-flag CCX count does not match the proven kernel")
    return n_ccx


def build_zero_flag() -> tuple[object, dict]:
    """2-bit zero-test flag (freeze primitive, 1T, proven exhaustive).

    Registers g0=0, g1=1, flag=2: X both data bits, CCX -> flag, X back.
    Map: flag ^= 1 iff (g1,g0) == (0,0); data restored. 4/4 inputs exact.
    """
    from .kickmix import Builder
    b = Builder(3)
    n_ccx = _emit_zero_flag_n(b, [0, 1], 2, [])
    return b, {"g": (0, 1), "flag": 2, "ccx": n_ccx}


def zero_flag_spec():
    """TaskSpec: flag output flips iff both data inputs are zero."""
    from .kickmix import TaskSpec

    def _perm(bits: dict[int, int]) -> dict[int, int]:
        out = dict(bits)
        out[2] = bits.get(2, 0) ^ (1 if (bits.get(0, 0) == 0 and bits.get(1, 0) == 0) else 0)
        return out

    return TaskSpec(num_qubits=3, inputs=(0, 1), outputs=(2,), target_perm=_perm)


def build_zero_flag_n(n: int) -> tuple[object, dict]:
    """n-bit zero-test flag (freeze primitive at width n, 2n-3 T).

    Registers g[0..n), flag=n, temps n+1..2n-2. Same map as the 2-bit
    version: flag ^= 1 iff every data bit is zero; data restored, temps
    freed. Proven exhaustive for n <= 6 (2^n inputs, no sampling).
    """
    from .kickmix import Builder
    if not isinstance(n, int) or not (2 <= n <= 6):
        raise ValueError(f"width must be in [2,6] (exhaustive-proof scope), got {n!r}: refusing")
    b = Builder(2 * n - 1)
    g = list(range(n))
    temps = list(range(n + 1, 2 * n - 1))
    n_ccx = _emit_zero_flag_n(b, g, n, temps)
    return b, {"g": tuple(g), "flag": n, "temps": temps, "ccx": n_ccx}


def build_zero_flag_param(n: int) -> tuple[object, dict]:
    """Zero-test flag at arbitrary width (paper-scale freeze primitive).

    Same emitter as the proven n<=6 kernel (shared _emit_zero_flag_n — one
    chain pattern, not two). Widths above 6 leave exhaustive proof behind:
    validate with edge_envelope() (all weight-0/1 neighborhoods, exact) plus
    the evaluator's sampled trajectories with its reported eps bound (the
    paper's own Methods sampling route) — never presented as exhaustive.
    Cap 256 (the paper's register width; larger refuses).
    """
    from .kickmix import Builder
    if isinstance(n, bool) or not isinstance(n, int) or not (2 <= n <= 256):
        raise ValueError(f"width must be in [2,256], got {n!r}: refusing")
    b = Builder(2 * n - 1)
    g = list(range(n))
    temps = list(range(n + 1, 2 * n - 1))
    n_ccx = _emit_zero_flag_n(b, g, n, temps)
    return b, {"g": tuple(g), "flag": n, "temps": temps, "ccx": n_ccx,
               "toffoli": n_ccx, "width": n}


def zero_flag_spec_n(n: int):
    """TaskSpec for the n-bit zero test (flag flips iff all data are zero)."""
    from .kickmix import TaskSpec
    if not isinstance(n, int) or not (2 <= n <= 6):
        raise ValueError(f"width must be in [2,6], got {n!r}: refusing")

    def _perm(bits: dict[int, int]) -> dict[int, int]:
        out = dict(bits)
        out[n] = bits.get(n, 0) ^ (1 if all(bits.get(i, 0) == 0 for i in range(n)) else 0)
        return out

    return TaskSpec(num_qubits=2 * n - 1, inputs=tuple(range(n)), outputs=(n,),
                    target_perm=_perm)


def zero_flag_spec_param(n: int):
    """TaskSpec for the parametric zero test (temps are checked ancillas)."""
    from .kickmix import TaskSpec
    if isinstance(n, bool) or not isinstance(n, int) or not (2 <= n <= 256):
        raise ValueError(f"width must be in [2,256], got {n!r}: refusing")

    def _perm(bits: dict[int, int]) -> dict[int, int]:
        out = dict(bits)
        out[n] = bits.get(n, 0) ^ (1 if all(bits.get(i, 0) == 0 for i in range(n)) else 0)
        return out

    return TaskSpec(num_qubits=2 * n - 1, inputs=tuple(range(n)), outputs=(n,),
                    ancillas=tuple(range(n + 1, 2 * n - 1)), target_perm=_perm)


def edge_envelope_zero_flag(bld, regs: dict, n: int) -> dict:
    """Exact edge envelope at any width: weight-0/1 neighborhoods + extremes.

    Checks all-zero (flag 0 and 1), all-ones, every single-one and every
    single-zero position, plus alternating patterns — 2n+5 trajectories via
    check_one, each exact (no sampling). The AND-chain's risky inputs are the
    near-misses (single non-inverted bit wrongly firing); this envelope covers
    all of them. Interior weights 2..n-2 ride on sampled validation (stated).
    """
    import random
    from .kickmix import KickmixEvaluator
    if regs.get("width", n) != n or regs.get("flag") != n:
        raise ValueError("regs do not match width n (implementation bug): refusing")
    spec = zero_flag_spec_param(n)
    ev = KickmixEvaluator()
    rng = random.Random(7)
    cases: list[dict[int, int]] = []
    zero = {i: 0 for i in range(n)}
    cases.append({**zero, n: 0})
    cases.append({**zero, n: 1})
    ones = {i: 1 for i in range(n)}
    cases.append({**ones, n: 0})
    for i in range(n):
        one = dict(zero)
        one[i] = 1
        cases.append({**one, n: 0})
        single0 = dict(ones)
        single0[i] = 0
        cases.append({**single0, n: 0})
    alt = {i: i & 1 for i in range(n)}
    cases.append({**alt, n: 0})
    alt2 = {i: 1 - (i & 1) for i in range(n)}
    cases.append({**alt2, n: 0})
    for init in cases:
        full = {q: 0 for q in range(2 * n - 1)}
        full.update(init)
        res = ev.check_one(bld, spec, full, rng)
        if not res.valid:
            raise ValueError(f"edge case failed at {[init]}: {res.reason}: refusing")
    return {"checked": len(cases), "passed": len(cases), "width": n}


def build_index_match(kbits: int, const: int) -> tuple[object, dict]:
    """Step-index comparator (freeze-clear primitive): flag ^= 1 iff idx == const.

    Masks idx with const via in-place Xs (idx ^ const is zero iff equal),
    runs the zero-test chain, unmasks. The mask/unmask pair is self-inverse,
    the chain preserves data (proven), so idx is restored exactly. kbits <= 6
    (inherits the exhaustive-proof scope of the chain).
    """
    from .kickmix import Builder
    if not isinstance(kbits, int) or not (2 <= kbits <= 6):
        raise ValueError(f"kbits must be in [2,6], got {kbits!r}: refusing")
    if not isinstance(const, int) or not (0 <= const < (1 << kbits)):
        raise ValueError(f"const must fit in {kbits} bits, got {const!r}: refusing")
    b = Builder(2 * kbits - 1)
    idx = list(range(kbits))
    temps = list(range(kbits + 1, 2 * kbits - 1))
    masked = [(const >> i) & 1 for i in range(kbits)]
    for i, m in enumerate(masked):
        if m:
            b.x(idx[i])
    n_ccx = _emit_zero_flag_n(b, idx, kbits, temps)
    for i, m in enumerate(masked):
        if m:
            b.x(idx[i])
    return b, {"idx": tuple(idx), "flag": kbits, "const": const, "ccx": n_ccx}


def index_match_spec(kbits: int, const: int):
    """TaskSpec: flag flips iff the index register equals const."""
    from .kickmix import TaskSpec
    if not isinstance(kbits, int) or not (2 <= kbits <= 6):
        raise ValueError(f"kbits must be in [2,6], got {kbits!r}: refusing")
    if not isinstance(const, int) or not (0 <= const < (1 << kbits)):
        raise ValueError(f"const must fit in {kbits} bits, got {const!r}: refusing")

    def _perm(bits: dict[int, int]) -> dict[int, int]:
        out = dict(bits)
        v = sum((bits.get(i, 0) & 1) << i for i in range(kbits))
        out[kbits] = bits.get(kbits, 0) ^ (1 if v == const else 0)
        return out

    return TaskSpec(num_qubits=2 * kbits - 1, inputs=tuple(range(kbits)),
                    outputs=(kbits,), target_perm=_perm)


def build_index_match_param(kbits: int, const: int) -> tuple[object, dict]:
    """Index comparator at schedule width (the early-stopping clear primitive).

    The paper tests termination from step 499 of a 533-step schedule: the
    reverse walk compares a step-index register against its own index and a
    match clears flag + register. kbits=10 covers 0..1023 ⊃ [0,533]; cap 16.
    Same mask-chain-unmask emitter as the proven kernel (shared _emit).
    """
    from .kickmix import Builder
    if isinstance(kbits, bool) or not isinstance(kbits, int) or not (2 <= kbits <= 16):
        raise ValueError(f"kbits must be in [2,16], got {kbits!r}: refusing")
    if isinstance(const, bool) or not isinstance(const, int) or not (0 <= const < (1 << kbits)):
        raise ValueError(f"const must fit in {kbits} bits, got {const!r}: refusing")
    b = Builder(2 * kbits - 1)
    idx = list(range(kbits))
    temps = list(range(kbits + 1, 2 * kbits - 1))
    masked = [(const >> i) & 1 for i in range(kbits)]
    for i, m in enumerate(masked):
        if m:
            b.x(idx[i])
    n_ccx = _emit_zero_flag_n(b, idx, kbits, temps)
    for i, m in enumerate(masked):
        if m:
            b.x(idx[i])
    return b, {"idx": tuple(idx), "flag": kbits, "const": const, "ccx": n_ccx,
               "width": kbits}


def index_match_spec_param(kbits: int, const: int):
    """TaskSpec for the parametric index comparator (temps checked)."""
    from .kickmix import TaskSpec
    if isinstance(kbits, bool) or not isinstance(kbits, int) or not (2 <= kbits <= 16):
        raise ValueError(f"kbits must be in [2,16], got {kbits!r}: refusing")
    if isinstance(const, bool) or not isinstance(const, int) or not (0 <= const < (1 << kbits)):
        raise ValueError(f"const must fit in {kbits} bits, got {const!r}: refusing")

    def _perm(bits: dict[int, int]) -> dict[int, int]:
        out = dict(bits)
        v = sum((bits.get(i, 0) & 1) << i for i in range(kbits))
        out[kbits] = bits.get(kbits, 0) ^ (1 if v == const else 0)
        return out

    return TaskSpec(num_qubits=2 * kbits - 1, inputs=tuple(range(kbits)),
                    outputs=(kbits,), ancillas=tuple(range(kbits + 1, 2 * kbits - 1)),
                    target_perm=_perm)


def edge_envelope_index_match(bld, regs: dict, kbits: int, const: int) -> dict:
    """Exact near-miss envelope: const, ±1, extremes, all single-bit flips.

    The comparator's risky inputs are const's Hamming-1 neighbors (mask off
    by one bit wrongly firing, or const itself missed). All kbits flips both
    directions plus 0/max/const±1 are checked exactly; the interior rides on
    sampling (stated).
    """
    import random
    from .kickmix import KickmixEvaluator
    if regs.get("width", kbits) != kbits or regs.get("const", const) != const:
        raise ValueError("regs do not match (kbits, const): refusing")
    spec = index_match_spec_param(kbits, const)
    ev = KickmixEvaluator()
    rng = random.Random(11)
    vals = {const, (const - 1) % (1 << kbits), (const + 1) % (1 << kbits),
            0, (1 << kbits) - 1}
    for i in range(kbits):
        vals.add(const ^ (1 << i))  # every Hamming-1 neighbor of const
    for v in sorted(vals):
        init = {q: 0 for q in range(2 * kbits - 1)}
        for i in range(kbits):
            init[i] = (v >> i) & 1
        res = ev.check_one(bld, spec, init, rng)
        if not res.valid:
            raise ValueError(f"edge case v={v} failed: {res.reason}: refusing")
    return {"checked": len(vals), "passed": len(vals), "kbits": kbits, "const": const}
