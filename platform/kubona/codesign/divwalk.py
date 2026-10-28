"""Exact divstep walks (Appendix Eq A2) with per-step verification.

Runs the real recurrence for a toy odd prime (default p=257 — small enough
to enumerate, large enough to exercise swaps, excursions, and ambiguous
decisions), checking at every step:
- Lemma A1: determinant f*gv - g*fv == p; f/gv odd, fv even
- Lemma A4: swap visible (v2(f'v)==1 iff s==1); s==0 & z'>=1 -> b==0;
  g'==0 -> b==1
- Lemma A.5: part-1 divisibility (z'>=0 -> 2^{z'+1}|f'v); part-2 premise
  (began z<0 -> fv != 0); window recurrence c <- c + b*2^{j-1} (A10/A12)
  tracked across every excursion from its initiating D>0 swap
- Theorem A.6: generic decode b == bit_{j-1}(g'v*q^-1 mod 2^j) at EVERY
  ambiguous depth (prime-generic — unlike divsteps.py's prime-specific
  Eqs 56a-d), plus the A11 second form via p*(f'q)^-1
- Cor A8: depth-2 special case b == g'v[1]^f'v[3] (kept as the cheap check)
- Prop A2: scaling fvw == 2^t f, gvw == 2^t g (mod p) throughout, and the
  terminal inverse w^-1 == f*fv*2^-t (mod p)

Depths >= 3 boolean rules use the paper-prime tables (divsteps.py) and are
NOT checked here — those rules are prime-specific; this walker proves the
prime-GENERIC machinery (window + both A11 forms) at every depth instead.

Deliberately OUT of toy scope (256-bit circuit construction, no integer
toy): early-stopping freeze machinery (frozen flag + step-index register,
first-498-steps omission at 1.4e-6), power-of-two scale removal by modular
halving per active reverse step (r -> r/2 or (r+p)/2 — needs modular
halving, meaningless over integers), packed 2x256-qubit register reuse,
sparse-prime fold 2^256 = 2^32+977. See carry.py for the portable
truncated-carry/comparison-limit patterns from the same section.
"""

from __future__ import annotations


def _require(condition: bool, message: str) -> None:
    """Load-bearing invariant check; explicit so ``python -O`` cannot strip it."""
    if not condition:
        raise AssertionError(message)


def modinv_odd(a: int, mod: int) -> int:
    """Inverse of odd a modulo a power of two (extended Euclid)."""
    if mod <= 0 or mod & (mod - 1):
        raise ValueError(f"mod must be a power of two, got {mod!r}: refusing")
    if a % 2 == 0:
        raise ValueError(f"a must be odd, got {a!r}: refusing")
    # extended Euclid on (a mod, mod); inputs coprime since a odd, mod 2^k
    old_r, r = a % mod, mod
    old_s, s = 1, 0
    while r:
        q = old_r // r
        old_r, r = r, old_r - q * r
        old_s, s = s, old_s - q * s
    if old_r != 1:
        raise ValueError(f"no inverse (implementation bug): refusing {a!r}")
    return old_s % mod


def v2(u: int) -> int:
    """2-adic valuation (trailing zero bits); v2(0) = +inf (refused as None)."""
    if u == 0:
        raise ValueError("v2(0) undefined: refusing")
    n = 0
    while u % 2 == 0:
        u //= 2
        n += 1
    return n


def bit(u: int, k: int) -> int:
    """Bit k of u mod 2^{k+1} (matches two's-complement digit for u < 0)."""
    return (u >> k) & 1 if u >= 0 else ((u % (1 << (k + 1))) >> k) & 1


def step(state: tuple) -> tuple:
    """One divstep (Eq A2). State (z,f,g,fv,gv) -> (new state, s, b)."""
    z, f, g, fv, gv = state
    if g == 0:
        return state, 0, 0  # frozen
    if g % 2 == 0:
        return (z + 1, f, g // 2, 2 * fv, gv), 0, 0
    if z >= 0:
        return (-z, g, (g - f) // 2, 2 * gv, gv - fv), 1, 1
    return (z + 1, f, (g + f) // 2, 2 * fv, gv + fv), 0, 1


def run_walk(p: int, w: int, cap: int = 10_000) -> dict:
    """Run the walk to g == 0, verifying every invariant per step.

    Returns the walk report (steps, decisions, depth histogram, checks).
    Refuses even p, out-of-range w, and non-termination within cap.
    """
    if not isinstance(p, int) or p < 3 or p % 2 == 0:
        raise ValueError(f"p must be an odd prime >= 3, got {p!r}: refusing")
    if not isinstance(w, int) or not (0 <= w < p):
        raise ValueError(f"w must be in [0,p), got {w!r}: refusing")
    state = (0, p, w, 0, 1)
    steps: list[dict] = []
    hist: dict[int, int] = {}
    checks = {"determinant": 0, "parity": 0, "swap_visible": 0,
              "z_rule": 0, "zero_rule": 0, "depth2": 0, "scaling": 0,
              "divisibility": 0, "window": 0, "decode1": 0, "decode2": 0}
    t = 0
    # excursion window state (Lemma A.5/A13): set by D>0 swaps, cleared when
    # leaving the z<0 regime; every began-z<0 nonswap must extend it
    win_expected: int | None = None
    win_j: int | None = None
    if w == 0:
        _check_invariants(state, p)
        return {"p": p, "w": w, "frozen": True, "steps": steps,
                "hist": hist, "checks": checks, "inverse": None}
    while True:
        z, f, g, fv, gv = state
        if g == 0:
            break
        if t >= cap:
            raise ValueError(f"walk did not terminate within {cap} steps: refusing")
        new_state, s, b = step(state)
        zp, fp, gp, fvp, gvp = new_state
        # Lemma A1 on the new state
        _require(fp * gvp - gp * fvp == p, "determinant broken (implementation bug)")
        _require(fp % 2 == 1 and gvp % 2 == 1 and fvp % 2 == 0, "parity broken")
        checks["determinant"] += 1
        checks["parity"] += 1
        # Lemma A4.1: swap visible from v2(f'v), with v2(0) = +inf:
        # an early even-step doubling of fv=0 leaves f'v=0 (divisible by 4,
        # valuation infinite, hence not 1) — the nonswap side still holds.
        _require((fvp != 0 and v2(fvp) == 1) == (s == 1), "swap visibility broken")
        checks["swap_visible"] += 1
        if s == 1:
            _require(z >= 0 and b == 1, "swap shape broken")
            if z > 0:
                win_expected, win_j = 1, 1  # A9: window opens as {1} at depth 1
            else:
                win_expected, win_j = None, None  # D=0: part-1 regime, no window
        else:
            _require(fvp % 4 == 0, "nonswap f'v not divisible by 4")
            if zp >= 1:
                _require(b == 0, "z-rule broken")
                checks["z_rule"] += 1
            if zp >= 0:
                # Lemma A.5 part 1: the window chain resets in this regime
                _require(fvp % (1 << (zp + 1)) == 0, "A5-part1 divisibility broken")
                checks["divisibility"] += 1
                win_expected, win_j = None, None
            else:
                # began z<0 (lands z'<0): part-2 premise + window recurrence
                _require(fv != 0, "A5-part2 premise (z<0 => fv!=0) broken")
                _require(win_expected is not None and win_j is not None,
                         "window chain broken (began z<0 outside a D>0 excursion)")
                _require(fvp != 0, "doubled nonzero fv vanished (implementation bug)")
                j = v2(fvp)
                _require(j == win_j + 1, "excursion depth must advance by doubling")
                q = fvp // (1 << j)
                _require(q % 2 == 1, "odd part must be odd (implementation bug)")
                M = 1 << j
                win = (gvp * modinv_odd(q % M, M)) % M  # g'v*q^-1 mod 2^j
                _require(win == win_expected + b * (1 << (j - 1)),
                         "window recurrence A10/A12 broken")
                checks["window"] += 1
                _require(b == ((win >> (j - 1)) & 1), "A11 first form broken")
                checks["decode1"] += 1
                _require(fp % 2 == 1, "f' must stay odd (implementation bug)")
                m2 = (p % M) * modinv_odd((fp * q) % M, M) % M
                _require(bit(m2, j - 1) == b, "A11 second form broken")
                checks["decode2"] += 1
                win_expected, win_j = win, j
        if gp == 0:
            _require(b == 1, "zero-rule broken")
            checks["zero_rule"] += 1
        # Ambiguous = negative-counter nonswap, non-terminating
        ambiguous = (s == 0 and z < 0 and gp != 0)
        depth = None
        if ambiguous:
            depth = v2(fvp)
            hist[depth] = hist.get(depth, 0) + 1
            if depth == 2:
                _require(b == (bit(gvp, 1) ^ bit(fvp, 3)), "Cor A8 broken")
                checks["depth2"] += 1
        # Prop A2 scaling on the new state (t+1 active steps)
        _require((fvp * w - pow(2, t + 1, p) * fp) % p == 0, "f-scaling broken")
        _require((gvp * w - pow(2, t + 1, p) * gp) % p == 0, "g-scaling broken")
        checks["scaling"] += 1
        steps.append({"s": s, "b": b, "depth": depth, "z": zp})
        state = new_state
        t += 1
    z, f, g, fv, gv = state
    _require(abs(f) == 1, "non-trivial gcd (p prime, w != 0)")
    inv = (f * fv * pow(pow(2, t, p), -1, p)) % p  # Prop A2 Eq A6
    _require((w * inv) % p == 1, "inverse incorrect (implementation bug)")
    return {"p": p, "w": w, "frozen": False, "active_steps": t,
            "steps": steps, "hist": hist, "checks": checks, "inverse": inv}


def _check_invariants(state: tuple, p: int) -> None:
    z, f, g, fv, gv = state
    _require(f * gv - g * fv == p, "determinant invariant broken")


def batch_report(p: int, ws: list[int]) -> dict:
    """Run walks for each w; aggregate depths, checks, and inverse proofs."""
    agg_hist: dict[int, int] = {}
    agg_checks: dict[str, int] = {}
    n_proven = 0
    worst_steps = 0
    for w in ws:
        rep = run_walk(p, w)
        if rep.get("frozen"):
            continue
        n_proven += 1
        worst_steps = max(worst_steps, rep["active_steps"])
        for d, c in rep["hist"].items():
            agg_hist[d] = agg_hist.get(d, 0) + c
        for k, c in rep["checks"].items():
            agg_checks[k] = agg_checks.get(k, 0) + c
    return {"p": p, "n": len(ws), "proven": n_proven,
            "worst_active_steps": worst_steps,
            "depth_hist": agg_hist, "checks": agg_checks}


def schedule_cut(p: int, ws: list[int], budget: float) -> dict:
    """Fitted schedule from measured stopping times (paper SI II.2 pattern).

    The paper replaces the 1616-step worst-case schedule with a 1432-step
    fitted window (failing inputs outside it count against the budget).
    Toy version: sort measured active-step counts; the cut is the smallest
    T with P(steps > T) <= budget (empirical tail). budget=0 recovers the
    worst case; the tail COUNT is reported, never hidden.
    """
    if not isinstance(budget, (int, float)) or isinstance(budget, bool) \
            or not (0.0 <= budget < 1.0):
        raise ValueError(f"budget must be in [0,1), got {budget!r}: refusing")
    times = []
    for w in ws:
        rep = run_walk(p, w)
        if not rep.get("frozen"):
            times.append(rep["active_steps"])
    if not times:
        raise ValueError("no terminating walks: refusing empty schedule")
    times.sort()
    n = len(times)
    worst = times[-1]
    # smallest T with at most budget*n walks above it
    allowed_over = int(budget * n)  # floor: never exceed the budget
    cut = times[n - 1 - allowed_over] if allowed_over < n else times[0]
    over = sum(1 for t in times if t > cut)
    return {"worst": worst, "cut": cut, "budget": budget,
            "n": n, "over": over, "over_rate": over / n,
            "saved_steps": worst - cut}


def _tc_width(lo: int, hi: int) -> int:
    """Exact two's-complement width for the observed integer range [lo, hi].

    Smallest k with -2^(k-1) <= lo and hi <= 2^(k-1) - 1. No +1 conservatism,
    no sign assumptions — the range is measured, not modeled.
    """
    if not isinstance(lo, int) or not isinstance(hi, int) or lo > hi:
        raise ValueError(f"bad range [{lo!r},{hi!r}]: refusing")
    # -2^(k-1) <= lo needs k-1 >= bitlen(|lo|), minus one when |lo| is an
    # exact power of two (-2^(k-1) itself fits); hi <= 2^(k-1)-1 needs
    # k-1 >= bitlen(hi). Either side vacuous when it cannot bind.
    klo = 1 if lo >= 0 else (abs(lo) - 1).bit_length() + 1
    khi = 1 if hi <= 0 else hi.bit_length() + 1
    return max(klo, khi)


def survey_walks(p: int, ws: list[int], jstore: int = 5, cap: int = 256) -> dict:
    """Paper-scale walk metrology over the REAL prime (all walks verified).

    Every walk runs the fully-checked run_walk (determinant, window,
    decode, scaling gates hold at full width — a 256-bit proof, not a
    toy). Aggregates, with paper-window anchors stated for comparison:
    - stopping times (+ count outside the paper's [499,533] window),
    - history-stack depths at cutoff jstore (count outside the 60 reserve),
    - packed-register peak fills per step index vs cap (paper: 2x256 shared
      g|f_v and f|g_v; fit or exact violator, never an average-as-proof).
    Counts are reported raw; tail probabilities belong to the caller
    (eps_upper_exact over these counts — see tests). Refuses empty input.
    """
    if not isinstance(ws, list) or not ws:
        raise ValueError("ws must be a non-empty list of inputs: refusing")
    if isinstance(jstore, bool) or not isinstance(jstore, int) or jstore < 0:
        raise ValueError(f"jstore must be non-negative int, got {jstore!r}: refusing")
    if not isinstance(cap, int) or cap < 1:
        raise ValueError(f"cap must be positive int, got {cap!r}: refusing")
    times: list[int] = []
    stacks: list[int] = []
    # exact observed ranges per step index (min/max over surveyed walks)
    rng: dict[str, dict[int, list[int]]] = {
        k: {} for k in ("f", "g", "fv", "gv")}
    n_verified = 0
    for w in ws:
        rep = run_walk(p, w)
        if rep.get("frozen"):
            continue
        n_verified += 1
        times.append(rep["active_steps"])
        hist = rep["hist"]
        stacks.append(sum(c for d, c in hist.items() if d >= jstore))
        state = (0, p, w, 0, 1)
        t = 0
        while True:
            z, f, g, fv, gv = state
            if g == 0:
                break
            for k, v in (("f", f), ("g", g), ("fv", fv), ("gv", gv)):
                cell = rng[k].setdefault(t, [v, v])
                if v < cell[0]:
                    cell[0] = v
                if v > cell[1]:
                    cell[1] = v
            state = step(state)[0]
            t += 1
    if not times:
        raise ValueError("no terminating walks: refusing empty survey")
    outside_window = sum(1 for t in times if not (499 <= t <= 533))
    over_stack = sum(1 for s in stacks if s > 60)
    peak_a = peak_b = 0
    over_cap_a: list[int] = []
    over_cap_b: list[int] = []
    n_steps = max(len(rng["g"]), 1)
    for t in range(n_steps):
        try:
            wa = (_tc_width(*rng["g"][t]) + _tc_width(*rng["fv"][t]),
                  _tc_width(*rng["f"][t]) + _tc_width(*rng["gv"][t]))
        except KeyError:
            break  # steps past the shortest walk have no range; stop
        fa, fb = wa
        peak_a, peak_b = max(peak_a, fa), max(peak_b, fb)
        if fa > cap:
            over_cap_a.append(t)
        if fb > cap:
            over_cap_b.append(t)
    return {"n": n_verified, "stopping_times": times,
            "worst_steps": max(times), "min_steps": min(times),
            "outside_window_499_533": outside_window,
            "stack_depths": stacks, "max_stack": max(stacks),
            "over_stack_60": over_stack, "jstore": jstore,
            "peak_fill_a": peak_a, "peak_fill_b": peak_b,
            "over_cap_a_steps": over_cap_a, "over_cap_b_steps": over_cap_b,
            "cap": cap, "fits_cap": not over_cap_a and not over_cap_b}
