"""Toy Berlekamp-Massey walk with the paper's rescaled update (SI I.1).

Implements the circuit's exact recurrence at toy scale as verifiable field
math (no circuit Toffolis claimed here — the arithmetic + invariant logic):

- Rounds r=1..N over syndrome sequence S_1..S_N (SI Eq rs-bm-round-update).
- Signed counter d (replace iff d>=0 on nonzero discrepancy; ties replace,
  which stores less history — paper's choice, stated).
- Rescaled replace (main-text Eq rescaled-overview): standard replace gives
  SIGMA = sigma - alpha*tau_tilde; the circuit keeps TAU_TILDE - alpha^{-1}*sigma
  = alpha^{-1}*(standard) (char 2: -1 = +1), same roots up to nonzero scale,
  so one shared arithmetic path suffices (no displaced-template history).
- Scale invariant sigma0*delta == 1 maintained (SI Eq rs-scale-invariant);
  asserted live every round — a violation is an implementation bug, refused.
- Same-roots check asserted live on every replace (rescaled == alpha^{-1} *
  standard coefficient-wise).

Toy scalings (stated, not hidden): field F2^12 -> F2^4 (BinaryField F24,
same characteristic, same program shape); 70 rounds / degree<=35 ->
N<=8 / degree<=3 (fail-closed above); single-error syndromes generated from
the field's primitive element (exact key-equation data, not sampled noise).

What this proves: the history-saving trick is sound (invariant holds,
rescaled locator has the same roots, toy decodes recover the error
position). What it does NOT claim: the paper's 140kT/440kT counts or the
DQI distribution behavior — those stay paper-measured anchors in dqi.py.
"""

from __future__ import annotations

from .psi import pmul


def _check_field(field) -> None:
    if not hasattr(field, "add") or not hasattr(field, "mul") or not hasattr(field, "inv"):
        raise ValueError("field must expose add/mul/inv (BinaryField): refusing")


def poly_add(a: list[int], b: list[int], field) -> list[int]:
    """Coefficient-wise add (char 2: subtraction is addition)."""
    _check_field(field)
    n = max(len(a), len(b))
    out = []
    for i in range(n):
        ai = a[i] if i < len(a) else 0
        bi = b[i] if i < len(b) else 0
        out.append(field.add(ai, bi))
    while len(out) > 1 and out[-1] == 0:
        out.pop()
    return out


def poly_scale(p: list[int], s: int, field) -> list[int]:
    """Scalar multiple s*p (s==0 gives [0])."""
    _check_field(field)
    if s == 0:
        return [0]
    out = [field.mul(c, s) for c in p]
    while len(out) > 1 and out[-1] == 0:
        out.pop()
    return out


def poly_eval(p: list[int], x: int, field) -> int:
    """Horner evaluation (exact field arithmetic)."""
    _check_field(field)
    acc = 0
    for c in reversed(p):
        acc = field.add(field.mul(acc, x), c)
    return acc


def _pow(field, a: int, e: int) -> int:
    out = 1
    for _ in range(e):
        out = field.mul(out, a)
    return out


def single_error_syndromes(field, gamma: int, pos: int, err: int, n: int) -> list[int]:
    """Exact syndromes S_i = err * gamma^{i*pos} for i=1..n (key-equation data).

    pos selects the field point gamma^pos; err != 0. Fail-closed on bad geometry.
    """
    _check_field(field)
    for name, v in (("gamma", gamma), ("err", err)):
        if not isinstance(v, int) or not (0 < v < field.size):
            raise ValueError(f"{name} must be nonzero field element: refusing {v!r}")
    if not isinstance(pos, int) or pos < 0:
        raise ValueError(f"pos must be non-negative int: refusing {pos!r}")
    if not isinstance(n, int) or not (1 <= n <= 8):
        raise ValueError(f"toy n must be in [1,8], got {n!r}: refusing")
    gp = _pow(field, gamma, pos)
    out = []
    gpi = gp
    for _ in range(1, n + 1):
        out.append(field.mul(err, gpi))
        gpi = field.mul(gpi, gp)
    return out


def berlekamp_massey_toy(syndromes: list[int], field) -> dict:
    """Rescaled BM walk over the syndrome sequence (circuit's recurrence).

    Starts (sigma,tau,delta,d) = ([1],[1],1,0); each round shifts tau,
    forms the discrepancy, and applies retain / rescaled-replace. Returns
    {sigma, rounds, invariant_holds, n_replace}. Refuses empty
    syndromes, zero delta escape, or invariant break (implementation bug).
    """
    rep = _bm_walk(syndromes, field, max_rounds=8, max_degree=3)
    return {"sigma": rep["sigma"], "rounds": rep["rounds"],
            "invariant_holds": True,
            "n_replace": sum(1 for x in rep["rounds"] if x["case"] == "replace")}


def multi_error_syndromes(field, gamma: int, errs: list[tuple[int, int]], n: int) -> list[int]:
    """Exact syndromes S_i = sum_j err_j * (gamma^pos_j)^i, i=1..n.

    Paper Eq rs-syndromes (S_1-based: the first list element is the i=1
    syndrome). errs lists (pos, err) with err != 0; positions need not be distinct
    (coincident terms add as field elements — callers use distinct). Caps
    at n <= 80 / 64 errors (fail-closed above: exactness, not scale; 64
    covers over-capacity probes past the 35-error decoding radius).
    """
    _check_field(field)
    if not isinstance(gamma, int) or not (0 < gamma < field.size):
        raise ValueError(f"gamma must be nonzero field element: refusing {gamma!r}")
    if not isinstance(errs, list) or not errs or len(errs) > 64:
        raise ValueError(f"errs must be 1..64 (pos, err) pairs: refusing {len(errs) if isinstance(errs, list) else errs!r}")
    if not isinstance(n, int) or not (1 <= n <= 80):
        raise ValueError(f"n must be in [1,80], got {n!r}: refusing")
    pts = []
    for pos, err in errs:
        if not isinstance(pos, int) or pos < 0:
            raise ValueError(f"pos must be non-negative int: refusing {pos!r}")
        if not isinstance(err, int) or not (0 < err < field.size):
            raise ValueError(f"err must be nonzero field element: refusing {err!r}")
        pts.append((_pow(field, gamma, pos), err))
    out = []
    cur = [p for p, _ in pts]
    for _ in range(1, n + 1):
        s = 0
        for (base, e), j in zip(pts, range(len(pts))):
            # accumulate BEFORE advancing: out[0] is the i=1 syndrome
            # (an advance-first draft yielded i=2..n+1 and broke Forney's
            # kappa — caught by the paper-scale composition, fixed here).
            s = field.add(s, field.mul(e, cur[j]))
            cur[j] = field.mul(cur[j], base)
        out.append(s)
    return out


def berlekamp_massey(syndromes: list[int], field, max_degree: int = 35) -> dict:
    """Full-scale rescaled BM (same verified recurrence, paper-sized caps).

    Covers the paper's 70-syndrome RS instance (max_degree 35 default).
    Besides {sigma, rounds} it reports the adaptive-range profile the
    circuit exploits: per-round measured active widths (coefficients the
    round actually touched) against the worst-case bound min(r, max_degree+1)
    (paper Fig dqi-rs(b) dashed line), plus stack_needed = retain rounds with
    nonzero discrepancy (history entries for exact decode) and the
    retain/replace counts. Caps: 80 rounds, degree 64 (fail-closed above).
    """
    rep = _bm_walk(syndromes, field, max_rounds=80, max_degree=max_degree)
    return rep


def _bm_walk(syndromes: list[int], field, max_rounds: int, max_degree: int) -> dict:
    _check_field(field)
    if not isinstance(max_rounds, int) or max_rounds < 1:
        raise ValueError(f"max_rounds must be positive int: refusing {max_rounds!r}")
    if not isinstance(max_degree, int) or max_degree < 1 or max_degree > 64:
        raise ValueError(f"max_degree must be in [1,64]: refusing {max_degree!r}")
    if not syndromes or len(syndromes) > max_rounds:
        raise ValueError(
            f"syndromes need 1..{max_rounds} entries, got {len(syndromes)}: refusing")
    for s in syndromes:
        if not isinstance(s, int) or not (0 <= s < field.size):
            raise ValueError(f"syndrome {s!r} outside field: refusing")
    sigma = [1]
    tau = [1]
    delta = 1
    d = 0
    rounds: list[dict] = []
    active: list[int] = []
    for r in range(1, len(syndromes) + 1):
        tau_tilde = [0] + list(tau)
        # discrepancy over available coefficients (S is 1-indexed here)
        deg = len(sigma) - 1
        disc = 0
        for i in range(0, min(deg, r - 1) + 1):
            disc = field.add(disc, field.mul(sigma[i], syndromes[r - 1 - i]))
        # measured active width: locator coefficients the round's update
        # actually touched (deg sigma + 1). The template shift is a virtual
        # index offset, not stored zeros — counting tau_tilde's length would
        # inflate this mechanically by one per round (caught in review).
        active.append(0)  # placeholder, filled with post-round deg below
        if disc == 0:
            sigma, tau, delta, d = list(sigma), tau_tilde, delta, d + 1
            rounds.append({"r": r, "case": "zero", "disc": 0, "d": d})
        else:
            if delta == 0:
                raise ValueError(f"round {r}: stored delta is 0 (implementation bug): refusing")
            alpha = field.mul(disc, field.inv(delta))
            if d >= 0:
                # replace (rescaled): keep tau_tilde - alpha^{-1} sigma
                ainv = field.inv(alpha)
                standard = poly_add(list(sigma), poly_scale(tau_tilde, alpha, field), field)
                rescaled = poly_add(list(tau_tilde), poly_scale(list(sigma), ainv, field), field)
                # same-roots gate: rescaled == alpha^{-1} * standard (char 2)
                expect = poly_scale(standard, ainv, field)
                if rescaled != expect:
                    raise ValueError(f"round {r}: rescale identity broken (implementation bug): refusing")
                sigma, tau, delta, d = rescaled, list(sigma), disc, -d
                rounds.append({"r": r, "case": "replace", "disc": disc, "d": d})
            else:
                sigma = poly_add(list(sigma), poly_scale(tau_tilde, alpha, field), field)
                tau, delta, d = tau_tilde, delta, d + 1
                rounds.append({"r": r, "case": "retain", "disc": disc, "d": d})
        # scale invariant sigma0 * delta == 1 (paper: shifted template has
        # zero constant term, so retain changes neither; replace sets
        # sigma0 <- delta^{-1} while delta <- DELTA)
        if field.mul(sigma[0], delta) != 1:
            raise ValueError(f"round {r}: scale invariant broken (implementation bug): refusing")
        if len(sigma) - 1 > max_degree:
            raise ValueError(f"round {r}: degree {len(sigma)-1} exceeds {max_degree}: refusing")
        active[r - 1] = len(sigma)
    n_replace = sum(1 for x in rounds if x["case"] == "replace")
    n_retain = sum(1 for x in rounds if x["case"] == "retain")
    # worst case (paper Fig dqi-rs(b) dashed line): after r syndromes only
    # coefficients through degree min(r, max_degree) can be active — a count
    # of min(r, max_degree) + 1, growing by one per round.
    worst = [min(r, max_degree) + 1 for r in range(1, len(syndromes) + 1)]
    return {"sigma": sigma, "rounds": rounds, "invariant_holds": True,
            "n_replace": n_replace, "n_retain": n_retain,
            "stack_needed": n_retain, "active_widths": active,
            "worst_widths": worst}


def locator_roots(sigma: list[int], field, gamma: int) -> list[int]:
    """Positions j with sigma(gamma^{-j}) == 0 (error locations)."""
    _check_field(field)
    if field.mul(sigma[0], 1) == 0 and len(sigma) == 1:
        raise ValueError("zero locator has no roots: refusing")
    order = field.size - 1
    ginv = field.inv(gamma)
    roots = []
    pw = 1
    for j in range(order):
        if poly_eval(sigma, pw, field) == 0:
            roots.append(j)
        pw = field.mul(pw, ginv)
    return roots


def key_equation_report(sigma: list[int], syndromes: list[int], field, gamma: int) -> dict:
    """In-place evaluator check: omega = sigma*S, tail must vanish (SI I.1).

    The circuit overwrites the first 35 syndrome words with the evaluator
    coefficients omega = sigma*S mod z^35 and uses the zeroed tail as
    workspace — sound only when the key equation holds (coefficients
    [deg, N) of the product vanish) AND the locator splits fully
    (nroots == deg: distinct error positions give distinct roots). Tail-zero
    alone is necessary, not sufficient (a degree-35 misdecode past capacity
    still satisfies it with 1 root — measured, not assumed). check_decode()
    enforces both; a misdecode refuses (counts against the failure budget,
    never a wrong sign).
    """
    _check_field(field)
    if not sigma or not syndromes:
        raise ValueError("sigma and syndromes must be non-empty: refusing")
    prod = pmul(list(sigma), list(syndromes), field)
    deg = len(sigma) - 1
    n = len(syndromes)
    tail = prod[deg:n]
    tail_zero = all(c == 0 for c in tail)
    roots = locator_roots(sigma, field, gamma)
    return {"omega": prod[:deg], "tail_zero": tail_zero,
            "tail_checked": len(tail), "deg": deg, "roots": roots,
            "nroots": len(roots), "splits_fully": len(roots) == deg}


def check_decode(sigma: list[int], syndromes: list[int], field, gamma: int) -> dict:
    """Fail-closed decode verdict (key equation + full splitting)."""
    rep = key_equation_report(sigma, syndromes, field, gamma)
    if not rep["tail_zero"]:
        raise ValueError("key-equation tail nonzero (misdecode): refusing")
    if not rep["splits_fully"]:
        raise ValueError(
            f"locator degree {rep['deg']} splits into {rep['nroots']} roots "
            "(misdecode past capacity): refusing")
    return rep


def error_value_from_quotient(omega_beta: int, dsigma_beta: int, field) -> int:
    """Forney error value e = omega(beta)/sigma'(beta) (exact division).

    The position factor kappa is 1 in this syndrome convention
    (S_i = sum e_j gamma^{ij}, BM normalization as here) — verified constant
    across positions and error values in tests, never assumed universal.
    Refuses zero denominator (repeated root: multiplicity path, queued).
    """
    from .gf import BinaryField
    if not isinstance(field, BinaryField):
        raise ValueError("field must be BinaryField: refusing")
    for name, v in (("omega_beta", omega_beta), ("dsigma_beta", dsigma_beta)):
        if not isinstance(v, int) or not (0 <= v < field.size):
            raise ValueError(f"{name} outside field: refusing {v!r}")
    if dsigma_beta == 0:
        raise ValueError("sigma'(beta) == 0 (repeated root): refusing")
    return field.mul(omega_beta, field.inv(dsigma_beta))


def contracted_phase_bit(nu: int, omega_beta: int, psi_beta: int, field) -> int:
    """Scan phase bit Tr(nu * omega(beta) * Psi(beta)) without the error.

    With Psi(beta) = 1/sigma'(beta) this equals Tr(nu * e) — the paper's
    bilinear readout that never materializes e_j. All inputs verified in
    range; nonzero checks belong to the division step above.
    """
    from .gf import BinaryField
    if not isinstance(field, BinaryField):
        raise ValueError("field must be BinaryField: refusing")
    for name, v in (("nu", nu), ("omega_beta", omega_beta), ("psi_beta", psi_beta)):
        if not isinstance(v, int) or not (0 <= v < field.size):
            raise ValueError(f"{name} outside field: refusing {v!r}")
    return field.trace(field.mul(nu, field.mul(omega_beta, psi_beta)))


def _chien_masks(field, beta: int, ncoeffs: int) -> list[list[int]]:
    """For each VAL bit t, the sigma-bit ids (k,i) with bit_t(e_i*beta^k)=1.

    Evaluating sigma(beta) = sum_k sigma_k beta^k is F2-linear in the
    coefficient bits: VAL_t = XOR over contributing sigma bits. Returns, per
    t in 0..n-1, the sorted list of flat sigma-bit ids (k*n+i) with n the
    field bit-width. Linearity is what makes paper-scale verification
    feasible: checking every basis vector (ncoeffs*n positions-free field
    ops per beta) proves VAL == sigma(beta) for ALL coefficient vectors,
    with no 2^(n*ncoeffs) enumeration.
    """
    n = _check_scan_field(field)
    masks: list[list[int]] = [[] for _ in range(n)]
    pw = 1
    for k in range(ncoeffs):
        for i in range(n):
            c = field.mul(1 << i, pw)
            for t in range(n):
                if (c >> t) & 1:
                    masks[t].append(k * n + i)
        pw = field.mul(pw, beta)
    for m in masks:
        m.sort()
    return masks


def _check_scan_field(field) -> int:
    """Binary-field shape for the scan (bit-width n, 2^n elements)."""
    _check_field(field)
    n = getattr(field, "n", None)
    size = getattr(field, "size", None)
    if (not isinstance(n, int) or isinstance(n, bool) or n < 2 or
            size != (1 << n)):
        raise ValueError(f"scan needs a binary field with size 2^n, got {field!r}: refusing")
    return n


def _solve_field_system(rows: list[list[int]], rhs: list[int], field) -> list[int]:
    """Exact solve of a square field system (Gauss-Jordan, partial pivot).

    Exact arithmetic throughout (no rounding); a zero pivot column means
    singular and refuses (fail-closed — the caller guarantees
    nonsingularity, as Vandermonde distinctness does). Every solution is
    verified live by residual (a silent wrong answer is worse than none).
    """
    n = len(rows)
    if n == 0 or any(len(r) != n for r in rows) or len(rhs) != n:
        raise ValueError("system must be square and matched: refusing")
    m = [list(r) + [v] for r, v in zip(rows, rhs)]
    for col in range(n):
        piv = next((r for r in range(col, n) if m[r][col] != 0), None)
        if piv is None:
            raise ValueError(f"singular system at column {col}: refusing")
        m[col], m[piv] = m[piv], m[col]
        inv = field.inv(m[col][col])
        m[col] = [field.mul(x, inv) for x in m[col]]
        for r in range(n):
            if r != col and m[r][col] != 0:
                f = m[r][col]
                m[r] = [field.add(a, field.mul(f, b)) for a, b in zip(m[r], m[col])]
    sol = [m[i][n] for i in range(n)]
    for i in range(n):
        acc = 0
        for j in range(n):
            acc = field.add(acc, field.mul(rows[i][j], sol[j]))
        if acc != rhs[i]:
            raise ValueError(f"residual failed at row {i} (implementation bug): refusing")
    return sol


def mask_cancellation(masks: list[int], field, ncancel: int = 70) -> dict:
    """Offline linear-part cancellation of the DQI mask (paper SI RS scan).

    Given the 12-bit mask at every nonzero position, chooses syndrome
    coefficients a_1..a_70 so the residual mu_tilde_j = mu_j - sum_i a_i
    beta_j^{-i} vanishes on the first `ncancel` positions. The INVERSE
    powers are load-bearing: syndromes are S_1-based in gamma^pos
    (paper Eq rs-syndromes) while scan positions are the roots
    beta_j = gamma^{-pos}, so the linear share of Tr(a_i S_i) at error j
    is Tr((sum_i a_i beta_j^{-i}) e_j) — positive powers silently fail
    the composition (caught live at 35 errors, fixed here). Square
    Vandermonde on the inverted points always solves for ncancel <= 70;
    more refuses (only 70 syndromes back the linear part — the paper's
    71st needs another degree of freedom, noted not closed). The linear
    phase sum_i Tr(a_i S_i) is Clifford-applicable; the scan then covers
    the rest. Residual zero on the canceled set is verified live.
    """
    _check_scan_field(field)
    npos = field.size - 1
    if (not isinstance(masks, list) or len(masks) != npos or
            any(not isinstance(c, int) or not (0 <= c < field.size) for c in masks)):
        raise ValueError(f"masks must be {npos} {field.n}-bit ints: refusing")
    if isinstance(ncancel, bool) or not isinstance(ncancel, int) or ncancel < 1:
        raise ValueError(f"ncancel must be a positive int, got {ncancel!r}: refusing")
    if ncancel > 70:
        # only the 70 supplied syndromes back the linear part (paper RS
        # instance); a 71st coefficient would have no syndrome to ride on.
        raise ValueError(f"ncancel capped at the 70 supplied syndromes, got {ncancel}: refusing")
    if ncancel > npos:
        raise ValueError(f"ncancel {ncancel} exceeds the {npos} positions: refusing")
    from .gf import trace_dual
    canceled = list(range(1, ncancel + 1))
    # a(beta^-1) = mu elementwise: with y_j = beta_j^-1, solve the degree
    # <= ncancel-1 interpolant a_tilde(y_j) = mu_j * beta_j (Vandermonde
    # on distinct y_j; 1/beta_j are distinct since beta_j are).
    inv = {b: field.inv(b) for b in canceled}
    rows = [[_ipow(inv[b], k, field) for k in range(ncancel)] for b in canceled]
    rhs = []
    for b in canceled:
        mu = trace_dual([(masks[b - 1] >> t) & 1 for t in range(field.n)], field)
        rhs.append(field.mul(mu, b))
    coeff = _solve_field_system(rows, rhs, field)  # a_tilde_0..69
    a = [0] + coeff  # a_0 unused; a_i multiplies beta^-i
    residual = {}
    for b in range(1, field.size):
        mu = trace_dual([(masks[b - 1] >> t) & 1 for t in range(field.n)], field)
        s = 0
        pw = field.inv(b)
        step = field.inv(b)
        for i in range(1, ncancel + 1):
            s = field.add(s, field.mul(a[i], pw))
            pw = field.mul(pw, step)
        residual[b] = field.add(mu, s)
    for b in canceled:
        if residual[b] != 0:
            raise ValueError(f"residual nonzero at canceled {b} (implementation bug): refusing")
    return {"canceled": canceled, "a": a[1:], "residual": residual, "ncancel": ncancel}


def _ipow(b: int, k: int, field) -> int:
    p = 1
    for _ in range(k):
        p = field.mul(p, b)
    return p


def build_chien_scan(ncoeffs: int = 3, field=None) -> tuple[object, dict]:
    """Chien search over all nonzero positions (toy default, paper scale ready).

    SIGMA holds ncoeffs field elements (degree <= ncoeffs-1; the paper's RS
    instance needs 36 over F2^12, the toy default 3 over F16). Per position
    beta: VAL = sigma(beta) via CNOTs only (linear in coefficient bits —
    0T), zero-test into found_j, uncompute VAL (same CNOTs, self-inverse).
    SIGMA preserved; FOUND pattern out; VAL and chain temps freed
    (dirty-free raises otherwise). Positions enumerate all nonzero field
    elements (order irrelevant to correctness, fixed ascending for tests).

    No degree cap: the old ncoeffs<=3 lock was proof-scope (exhaustive
    sigma-space needs 2^(n*ncoeffs) trajectories), dissolved by the
    linearity argument — VAL is F2-linear, so basis-exhaustive mask checks
    prove it for all coefficient vectors (slice-48 architecture). Cost is
    structural: zero Toffolis in VAL, (2n-3) per position from the
    zero-test, pinned in regs.
    """
    from .gf import F24
    from .kickmix import Builder
    from .packing import emit_zero_test
    if field is None:
        field = F24
    n = _check_scan_field(field)
    if isinstance(ncoeffs, bool) or not isinstance(ncoeffs, int) or ncoeffs < 1:
        raise ValueError(f"ncoeffs must be a positive int, got {ncoeffs!r}: refusing")
    nsig = ncoeffs * n
    npos = field.size - 1
    SIGMA = list(range(nsig))
    FOUND = list(range(nsig, nsig + npos))
    VAL = list(range(nsig + npos, nsig + npos + n))
    ZT = list(range(nsig + npos + n, nsig + npos + n + max(0, n - 2)))
    b = Builder(nsig + npos + n + max(0, n - 2))
    for q in VAL:
        b.alloc(q)
    n_ccx = 0
    for j, beta in enumerate(range(1, field.size)):
        masks = _chien_masks(field, beta, ncoeffs)
        ops: list[tuple] = []
        for t in range(n):
            for q in masks[t]:
                ops.append(("cx", (q, VAL[t])))
        for name, qs in ops:
            b.cx(*qs)
        n_ccx += emit_zero_test(b, VAL, FOUND[j], ZT)
        for name, qs in ops:
            b.cx(*qs)
    for q in VAL:
        b.free(q)
    return b, {"sigma": tuple(SIGMA), "found": tuple(FOUND), "ccx": n_ccx,
               "positions": npos, "ncoeffs": ncoeffs, "nbits": n}


def chien_scan_spec(ncoeffs: int = 3, field=None):
    """TaskSpec: found_j = [sigma(beta_j) == 0] over all nonzero positions."""
    from .gf import F24
    from .kickmix import TaskSpec
    if field is None:
        field = F24
    n = _check_scan_field(field)
    if isinstance(ncoeffs, bool) or not isinstance(ncoeffs, int) or ncoeffs < 1:
        raise ValueError(f"ncoeffs must be a positive int, got {ncoeffs!r}: refusing")

    nsig = ncoeffs * n
    npos = field.size - 1

    def _perm(bits: dict[int, int]) -> dict[int, int]:
        out = dict(bits)
        coeffs = []
        for k in range(ncoeffs):
            c = 0
            for i in range(n):
                c |= (bits.get(k * n + i, 0) & 1) << i
            coeffs.append(c)
        for j, beta in enumerate(range(1, field.size)):
            acc, pp = 0, 1
            for c in coeffs:
                acc ^= field.mul(c, pp)
                pp = field.mul(pp, beta)
            out[nsig + j] = 1 if acc == 0 else 0
        return out

    nval = n + max(0, n - 2)
    return TaskSpec(num_qubits=nsig + npos + nval, inputs=tuple(range(nsig)),
                    outputs=tuple(range(nsig, nsig + npos)),
                    ancillas=tuple(range(nsig + npos, nsig + npos + nval)),
                    target_perm=_perm)
