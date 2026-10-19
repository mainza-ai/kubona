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


def _bm_walk(syndromes: list[int], field, max_rounds: int, max_degree: int,
              harvest: list | None = None) -> dict:
    """... (harvest: optional list; each round appends pre/post register
    snapshots + case for circuit-proof harvesting. Default None: no-op.)"""
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
        if harvest is not None:
            harvest.append({"r": r, "pre": (list(sigma), list(tau), delta, d),
                            "syn": list(syndromes)})
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
        if harvest is not None:
            harvest[-1]["post"] = (list(sigma), list(tau), delta, d)
            harvest[-1]["case"] = rounds[-1]["case"]
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


def _binary_rank_factor(rows: list[int], n: int) -> tuple[list[int], list[int]]:
    """Exact rank factorization of a binary matrix (RREF, deterministic).

    Rows are n-bit masks; returns (Acols, Brows) with sum_k A_k B_k^T == M
    over F2 and len == rank(M) (pivot columns of the original against the
    nonzero RREF rows — textbook identity, verified per-instance by the
    bilinear basis check downstream). Column scan order is fixed, so the
    factorization is deterministic.
    """
    if (not isinstance(n, int) or isinstance(n, bool) or n < 1 or
            not isinstance(rows, list) or len(rows) != n or
            any(not isinstance(r, int) or r < 0 or r >= (1 << n) for r in rows)):
        raise ValueError("need n n-bit row masks: refusing")
    mat = list(rows)
    pivots: list[int] = []
    r = 0
    for c in range(n):
        piv = next((i for i in range(r, n) if (mat[i] >> c) & 1), None)
        if piv is None:
            continue
        mat[r], mat[piv] = mat[piv], mat[r]
        for i in range(n):
            if i != r and (mat[i] >> c) & 1:
                mat[i] ^= mat[r]
        pivots.append(c)
        r += 1
    acols = []
    for c in pivots:
        m = 0
        for i in range(n):
            if (rows[i] >> c) & 1:
                m |= 1 << i
        acols.append(m)
    return acols, mat[:r]


def bilinear_terms(nu: int, field) -> list[tuple[int, int]]:
    """Rank-factored bilinear readout form for one scan position (exact).

    Tr(nu*w*psi) = sum_{i,j} M_ij w_i psi_j with M_ij = Tr(nu*2^i*2^j);
    returns the rank(M) <= n mask pairs (a_k, b_k) whose XOR-AND sum is
    M. Each pair is one product (one CCZ in emission, counted one
    Toffoli like the paper). Rank varies with nu (0 for nu=0 — the
    position contributes nothing); verified in-test by agreement on all
    n*n basis (w, psi) unit pairs (linearity spans the rest).
    """
    n = _check_scan_field(field)
    if not isinstance(nu, int) or isinstance(nu, bool) or not (0 <= nu < field.size):
        raise ValueError(f"nu outside field: refusing {nu!r}")
    rows = []
    for i in range(n):
        m = 0
        for j in range(n):
            if field.trace(field.mul(nu, field.mul(1 << i, 1 << j))):
                m |= 1 << j
        rows.append(m)
    acols, brows = _binary_rank_factor(rows, n)
    return list(zip(acols, brows))


def _check_scan_inputs(field, ncoeffs: int, positions, nus) -> tuple[int, int, int]:
    """Single home for phase-scan layout validation (builder + spec share it).

    Returns (n, nsig, npos). Positions must be distinct nonzero field
    elements; nus must key exactly those positions with in-field values
    (a missing/extra constant would silently mistarget the phase).
    """
    n = _check_scan_field(field)
    if isinstance(ncoeffs, bool) or not isinstance(ncoeffs, int) or ncoeffs < 1:
        raise ValueError(f"ncoeffs must be a positive int, got {ncoeffs!r}: refusing")
    if not isinstance(positions, list) or not positions:
        raise ValueError("positions must be a non-empty list: refusing")
    for b in positions:
        if not isinstance(b, int) or isinstance(b, bool) or not (1 <= b < field.size):
            raise ValueError(f"position {b!r} outside nonzero field elements: refusing")
    if len(set(positions)) != len(positions):
        raise ValueError("duplicate positions would double-apply phase: refusing")
    if (not isinstance(nus, dict) or set(nus.keys()) != set(positions) or
            any(not isinstance(v, int) or isinstance(v, bool) or not (0 <= v < field.size)
                for v in nus.values())):
        raise ValueError("nus must map exactly the positions to field values: refusing")
    return n, ncoeffs * n, len(positions)


def build_phase_scan(field, ncoeffs: int, positions: list[int], nus: dict) -> tuple[object, dict]:
    """Root-gated bilinear phase scan (paper SI RS scan, our construction).

    Registers SIGMA/OMEGA/PSI hold ncoeffs field elements each (36 at
    paper scale: locator, evaluator, reciprocal). Per scanned position:
    VAL finds sigma(beta) (CNOTs), zero-test sets found (21T), VALs find
    omega(beta)/psi(beta) (CNOTs), the nu-factored parities fan out
    (CNOTs), one CCZ per product applies the phase iff found, then
    everything uncomputes: parities, VALs (CNOT mirrors, free), and the
    found flag via a second zero-test call (21T — reuses the proven
    primitive instead of forking a mirror). Structural cost per position
    is 42T + rank(nu) (12 at full rank): the bilinear products match the
    paper Toffoli-for-Toffoli; the gap to its 23T/position is the root
    test (ours 42T incl. cleanup vs their hand-built 11T), stated not
    closed. Inputs preserved; every temp returns to |0> (dirty-free);
    the only observable is the trajectory phase, proven downstream.
    """
    from .kickmix import Builder
    from .packing import emit_zero_test
    n, nsig, npos = _check_scan_inputs(field, ncoeffs, positions, nus)
    SIGMA = list(range(nsig))
    OMEGA = list(range(nsig, 2 * nsig))
    PSI = list(range(2 * nsig, 3 * nsig))
    VALW = list(range(3 * nsig, 3 * nsig + n))
    VALP = list(range(3 * nsig + n, 3 * nsig + 2 * n))
    ZT = list(range(3 * nsig + 2 * n, 3 * nsig + 3 * n - 2))
    FOUND = [3 * nsig + 3 * n - 2]
    PARI = list(range(3 * nsig + 3 * n - 1, 3 * nsig + 5 * n - 1))
    ntemps = 5 * n - 1
    b = Builder(3 * nsig + ntemps)
    # ZT chain temps belong to emit_zero_test (allocs+frees per call):
    # reserve the ids, never touch them here.
    for q in VALW + VALP + FOUND + PARI:
        b.alloc(q)
    n_ccx = [0]
    ranks = []
    for beta in positions:
        masks = _chien_masks(field, beta, ncoeffs)
        for t in range(n):
            for q in masks[t]:
                b.cx(SIGMA[q], VALW[t])
        n_ccx[0] += emit_zero_test(b, VALW, FOUND[0], ZT)
        for t in range(n):
            for q in masks[t]:
                b.cx(SIGMA[q], VALW[t])
        terms = bilinear_terms(nus[beta], field)
        ranks.append(len(terms))
        for t in range(n):
            for q in masks[t]:
                b.cx(OMEGA[q], VALW[t])
        for k, (am, _bm) in enumerate(terms):
            for i in range(n):
                if (am >> i) & 1:
                    b.cx(VALW[i], PARI[k])
        for t in range(n):
            for q in masks[t]:
                b.cx(PSI[q], VALP[t])
        for k, (_am, bm) in enumerate(terms):
            for j in range(n):
                if (bm >> j) & 1:
                    b.cx(VALP[j], PARI[n + k])
        for k in range(len(terms)):
            b.ccz(PARI[k], PARI[n + k], FOUND[0])
            n_ccx[0] += 1
        for k, (_am, bm) in reversed(list(enumerate(terms))):
            for j in range(n):
                if (bm >> j) & 1:
                    b.cx(VALP[j], PARI[n + k])
        for k, (am, _bm) in reversed(list(enumerate(terms))):
            for i in range(n):
                if (am >> i) & 1:
                    b.cx(VALW[i], PARI[k])
        for t in range(n):
            for q in reversed(masks[t]):
                b.cx(PSI[q], VALP[t])
        for t in range(n):
            for q in reversed(masks[t]):
                b.cx(OMEGA[q], VALW[t])
        for t in range(n):
            for q in masks[t]:
                b.cx(SIGMA[q], VALW[t])
        n_ccx[0] += emit_zero_test(b, VALW, FOUND[0], ZT)
        for t in range(n):
            for q in masks[t]:
                b.cx(SIGMA[q], VALW[t])
    for q in VALW + VALP + FOUND + PARI:
        b.free(q)
    return b, {"inputs": tuple(range(3 * nsig)), "temps": ntemps,
               "ccx": n_ccx[0], "positions": tuple(positions),
               "ranks": ranks, "ncoeffs": ncoeffs, "nbits": n,
               "kind": "phase-scan"}


def phase_scan_spec(field, ncoeffs: int, positions: list[int], nus: dict):
    """TaskSpec: pure-phase scan — identity on bits, math phase on roots.

    No bit outputs (everything uncomputes); the phase is the Layer-1 scan
    sum over roots found from the SIGMA register. Planted truth enters
    downstream (composition), here the circuit is held to the formula.
    """
    from .kickmix import TaskSpec
    from .psi import peval
    n, nsig, npos = _check_scan_inputs(field, ncoeffs, positions, nus)

    def _coeffs(bits: dict[int, int], base: int) -> list[int]:
        return [sum((bits.get(base + k * n + i, 0) & 1) << i for i in range(n))
                for k in range(ncoeffs)]

    def _phase(bits: dict[int, int]) -> int:
        sig = _coeffs(bits, 0)
        om = _coeffs(bits, nsig)
        ps = _coeffs(bits, 2 * nsig)
        ph = 0
        for beta in positions:
            if peval(sig, beta, field) != 0:
                continue
            w = peval(om, beta, field)
            p = peval(ps, beta, field)
            ph ^= field.trace(field.mul(nus[beta], field.mul(w, p)))
        return ph & 1

    return TaskSpec(num_qubits=3 * nsig + (5 * n - 1), inputs=tuple(range(3 * nsig)),
                    outputs=(), ancillas=tuple(range(3 * nsig, 3 * nsig + 5 * n - 1)),
                    target_perm=None, target_phase=_phase)


def _emit_incr5(b, n_ccx: list[int], d: list[int], carry: list[int], dagger: bool = False) -> None:
    """In-place +1 on a 5-bit two's complement register (ripple, 6T).

    d LSB-first; carry is 4 zeroed temps, returned clean. Carry-out is
    dropped (two's-complement wrap) — the caller guarantees no overflow
    (BM's |d| <= rounds <= 8, far inside 5 bits; out-of-scope inputs
    would wrap silently, so the round scope is model-gated like the
    divstep harvested states). Top-down apply/uncompute interleaving:
    each carry is cleared while its source bits are still intact.
    dagger=True emits the exact reverse sequence (uncompute); forward
    then dagger is the identity, tested exhaustively both ways.
    """
    if len(d) != 5 or len(carry) != 4:
        raise ValueError("need 5-bit d and 4 carry bits: refusing")
    if not dagger:
        b.cx(d[0], carry[0])
        for i in (1, 2, 3):
            b.ccx(d[i], carry[i - 1], carry[i])
            n_ccx[0] += 1
        b.x(d[0])
        for i in (4, 3, 2):
            b.cx(carry[i - 1], d[i])
            b.ccx(d[i - 1], carry[i - 2], carry[i - 1])
            n_ccx[0] += 1
        b.cx(carry[0], d[1])
        b.cx(d[0], carry[0])
        b.x(carry[0])
    else:
        b.x(carry[0])
        b.cx(d[0], carry[0])
        b.cx(carry[0], d[1])
        for i in (2, 3, 4):
            b.ccx(d[i - 1], carry[i - 2], carry[i - 1])
            n_ccx[0] += 1
            b.cx(carry[i - 1], d[i])
        b.x(d[0])
        for i in (3, 2, 1):
            b.ccx(d[i], carry[i - 1], carry[i])
            n_ccx[0] += 1
        b.cx(d[0], carry[0])


def _emit_neg5(b, n_ccx: list[int], d: list[int], carry: list[int], dagger: bool = False) -> None:
    """In-place two's-complement negate (NOTs + increment, 6T; dagger mirrors)."""
    if len(d) != 5 or len(carry) != 4:
        raise ValueError("need 5-bit d and 4 carry bits: refusing")
    if not dagger:
        for q in d:
            b.x(q)
        _emit_incr5(b, n_ccx, d, carry)
    else:
        _emit_incr5(b, n_ccx, d, carry, dagger=True)
        for q in d:
            b.x(q)


def _bm_round_layout(base: int) -> dict:
    """Single canonical bank layout for one BM round (offsets from base).

    Inputs SIGMA/TAU/DELTA/IDELTA/D/W, temps, then outputs. Base 0
    reproduces the original fixed layout exactly (single-round builder
    below is byte-identical post-refactor, verified by the unchanged
    slice-52 pins). The walk builder reuses this per round with fresh
    output banks and a shared temp block.
    """
    o = [base]

    def take(n: int) -> list[int]:
        r = list(range(o[0], o[0] + n))
        o[0] += n
        return r

    SIGMA = take(16)
    TAU = take(36)
    DELTA = take(4)
    IDELTA = take(4)
    D = take(5)
    W = take(16)
    P = [take(4) for _ in range(4)]
    DISC = take(4)
    FZ, FR, FT = take(1), take(1), take(1)
    ALPHA = take(4)
    AINV = take(4)
    NID = take(4)
    REPL = take(16)
    RET = take(16)
    TINCR = take(5)
    TNEG = take(5)
    CARRY = take(4)
    IT = take(16)
    ZT = take(2)
    RD = take(16)
    TD = take(36)
    DD = take(4)
    IDD = take(4)
    DDD = take(5)
    ntemps = o[0] - base - 81
    OSIG = take(16)
    OTAU = take(36)
    ODELTA = take(4)
    OIDELTA = take(4)
    OD = take(5)
    allocs = [q for grp in (P[0], P[1], P[2], P[3], DISC, FZ, FR, FT, ALPHA,
                            AINV, NID, REPL, RET, TINCR, TNEG, CARRY, IT,
                            RD, TD, DD, IDD, DDD) for q in grp]
    return {"SIGMA": SIGMA, "TAU": TAU, "DELTA": DELTA, "IDELTA": IDELTA,
            "D": D, "W": W, "P": P, "DISC": DISC, "FZ": FZ, "FR": FR,
            "FT": FT, "ALPHA": ALPHA, "AINV": AINV, "NID": NID,
            "REPL": REPL, "RET": RET, "TINCR": TINCR, "TNEG": TNEG,
            "CARRY": CARRY, "IT": IT, "ZT": ZT, "RD": RD, "TD": TD,
            "DD": DD, "IDD": IDD, "DDD": DDD, "OSIG": OSIG, "OTAU": OTAU,
            "ODELTA": ODELTA, "OIDELTA": OIDELTA, "OD": OD, "nin": 81,
            "ntemps": ntemps, "nqubits": o[0] - base, "allocs": allocs}


def _emit_bm_round(b, n_ccx: list[int], L: dict) -> None:
    """One BM round into caller-managed banks (shared emitter, no allocs).

    All qubit ids come from L (single layout home above): reads the
    input bank, writes the output bank, borrows the temp block (left
    clean or dirty-free fires). Deterministic gate order; the walk
    builder calls this per round with fresh output banks.
    """
    from .f24 import emit_fermat_inv, emit_mul
    from .packing import emit_zero_test
    SIGMA = L["SIGMA"]
    TAU = L["TAU"]
    DELTA = L["DELTA"]
    IDELTA = L["IDELTA"]
    D = L["D"]
    W = L["W"]
    P = L["P"]
    DISC = L["DISC"]
    FZ, FR, FT = L["FZ"], L["FR"], L["FT"]
    ALPHA = L["ALPHA"]
    AINV = L["AINV"]
    NID = L["NID"]
    REPL = L["REPL"]
    RET = L["RET"]
    TINCR = L["TINCR"]
    TNEG = L["TNEG"]
    CARRY = L["CARRY"]
    IT = L["IT"]
    ZT = L["ZT"]
    RD = L["RD"]
    TD = L["TD"]
    DD = L["DD"]
    IDD = L["IDD"]
    DDD = L["DDD"]
    OSIG = L["OSIG"]
    OTAU = L["OTAU"]
    ODELTA = L["ODELTA"]
    OIDELTA = L["OIDELTA"]
    OD = L["OD"]
    # outputs start at |0>: SIGMA' = SIGMA, TAU' = shifted TAU
    for i in range(16):
        b.cx(SIGMA[i], OSIG[i])
    for j in range(1, 9):
        for t in range(4):
            b.cx(TAU[4 * (j - 1) + t], OTAU[4 * j + t])
    for i in range(4):
        b.cx(DELTA[i], ODELTA[i])
        b.cx(IDELTA[i], OIDELTA[i])
    # discrepancy disc = sum sigma_i * W_i
    for i in range(4):
        n_ccx[0] += emit_mul(b, [SIGMA[4 * i + t] for t in range(4)],
                             [W[4 * i + t] for t in range(4)], P[i])
    for i in range(4):
        for t in range(4):
            b.cx(P[i][t], DISC[t])
    n_ccx[0] += emit_zero_test(b, DISC, FZ[0], ZT)
    # flags: fr = !fz & (d>=0); ft = !fz & (d<0)
    b.x(FZ[0])
    b.x(D[4])
    b.ccx(FZ[0], D[4], FR[0])
    n_ccx[0] += 1
    b.x(D[4])
    b.x(FZ[0])
    b.x(FZ[0])
    b.ccx(FZ[0], D[4], FT[0])
    n_ccx[0] += 1
    b.x(FZ[0])
    # alpha = disc * IDELTA; ainv = inv(alpha)
    n_ccx[0] += emit_mul(b, DISC, IDELTA, ALPHA)
    n_ccx[0] += emit_fermat_inv(b, ALPHA, AINV, IT)
    # new_IDELTA = ainv * IDELTA (so next round needs no fresh inversion)
    n_ccx[0] += emit_mul(b, AINV, IDELTA, NID)
    # repl_scale = ainv * sigma; ret_scale = alpha * tau_tilde[0..3].
    # tau_tilde is the shifted template (coeff 0 is zero, coeff i>=1 is
    # old tau[i-1]): the retain multiplier reads the shifted coeffs, so
    # coeff 0 stays zero and coeff i>=1 uses TAU[i-1]. (An unshifted draft
    # broke retain rounds while replace rounds passed — caught live.)
    for i in range(4):
        n_ccx[0] += emit_mul(b, AINV, [SIGMA[4 * i + t] for t in range(4)],
                             [REPL[4 * i + t] for t in range(4)])
    for i in range(1, 4):
        n_ccx[0] += emit_mul(b, ALPHA, [TAU[4 * (i - 1) + t] for t in range(4)],
                             [RET[4 * i + t] for t in range(4)])
    # d+1 and -d into temp words
    for i in range(5):
        b.cx(D[i], TINCR[i])
    _emit_incr5(b, n_ccx, TINCR, CARRY)
    for i in range(5):
        b.cx(D[i], TNEG[i])
    _emit_neg5(b, n_ccx, TNEG, CARRY)
    # mux deltas (CNOT-built) and gate into outputs
    for idx in range(16):
        if idx >= 4:
            b.cx(TAU[idx - 4], RD[idx])
        b.cx(REPL[idx], RD[idx])
        b.cx(SIGMA[idx], RD[idx])
        b.ccx(FR[0], RD[idx], OSIG[idx])
        n_ccx[0] += 1
        b.ccx(FT[0], RET[idx], OSIG[idx])
        n_ccx[0] += 1
    for j in range(36):
        if j < 16:
            b.cx(SIGMA[j], TD[j])
        if j >= 4:
            b.cx(TAU[j - 4], TD[j])
        b.ccx(FR[0], TD[j], OTAU[j])
        n_ccx[0] += 1
    for i in range(4):
        b.cx(DISC[i], DD[i])
        b.cx(DELTA[i], DD[i])
        b.ccx(FR[0], DD[i], ODELTA[i])
        n_ccx[0] += 1
        b.cx(NID[i], IDD[i])
        b.cx(IDELTA[i], IDD[i])
        b.ccx(FR[0], IDD[i], OIDELTA[i])
        n_ccx[0] += 1
    for i in range(5):
        b.cx(TNEG[i], DDD[i])
        b.cx(TINCR[i], DDD[i])
    for i in range(5):
        b.cx(TINCR[i], OD[i])
    for i in range(5):
        b.ccx(FR[0], DDD[i], OD[i])
        n_ccx[0] += 1
    # full mirror uncompute (reverse dependency order; output bases are
    # results, never uncomputed — only temp-built deltas mirror back)
    for i in range(5):
        b.cx(TNEG[i], DDD[i])
        b.cx(TINCR[i], DDD[i])
    for i in range(4):
        b.cx(DISC[i], DD[i])
        b.cx(DELTA[i], DD[i])
        b.cx(NID[i], IDD[i])
        b.cx(IDELTA[i], IDD[i])
    _emit_neg5(b, n_ccx, TNEG, CARRY, dagger=True)
    for i in range(5):
        b.cx(D[i], TNEG[i])
    _emit_incr5(b, n_ccx, TINCR, CARRY, dagger=True)
    for i in range(5):
        b.cx(D[i], TINCR[i])
    for j in range(36):
        if j >= 4:
            b.cx(TAU[j - 4], TD[j])
        if j < 16:
            b.cx(SIGMA[j], TD[j])
    for idx in range(16):
        b.cx(SIGMA[idx], RD[idx])
        b.cx(REPL[idx], RD[idx])
        if idx >= 4:
            b.cx(TAU[idx - 4], RD[idx])
    for i in range(3, 0, -1):
        n_ccx[0] += emit_mul(b, ALPHA, [TAU[4 * (i - 1) + t] for t in range(4)],
                             [RET[4 * i + t] for t in range(4)])
    for i in range(3, -1, -1):
        n_ccx[0] += emit_mul(b, AINV, [SIGMA[4 * i + t] for t in range(4)],
                             [REPL[4 * i + t] for t in range(4)])
    n_ccx[0] += emit_mul(b, AINV, IDELTA, NID)
    n_ccx[0] += emit_fermat_inv(b, ALPHA, AINV, IT)
    n_ccx[0] += emit_mul(b, DISC, IDELTA, ALPHA)
    b.x(FZ[0])
    b.ccx(FZ[0], D[4], FT[0])
    n_ccx[0] += 1
    b.x(FZ[0])
    b.x(FZ[0])
    b.x(D[4])
    b.ccx(FZ[0], D[4], FR[0])
    n_ccx[0] += 1
    b.x(D[4])
    b.x(FZ[0])
    n_ccx[0] += emit_zero_test(b, DISC, FZ[0], ZT)
    for i in range(4):
        for t in range(4):
            b.cx(P[i][t], DISC[t])
    for i in range(3, -1, -1):
        n_ccx[0] += emit_mul(b, [SIGMA[4 * i + t] for t in range(4)],
                             [W[4 * i + t] for t in range(4)], P[i])


def build_bm_round() -> tuple[object, dict]:
    """One reversible BM round at toy scale (uniform circuit, no parameters).

    Thin wrapper over the shared emitter (single bank at base 0 —
    byte-identical to the pre-refactor circuit, verified by the
    unchanged slice-52 pins). See _emit_bm_round for the round design.
    """
    from .kickmix import Builder
    L = _bm_round_layout(0)
    b = Builder(L["nqubits"])
    for q in L["allocs"]:
        b.alloc(q)
    n_ccx = [0]
    _emit_bm_round(b, n_ccx, L)
    for q in L["allocs"]:
        b.free(q)
    outs = list(L["OSIG"]) + list(L["OTAU"]) + list(L["ODELTA"]) + list(L["OIDELTA"]) + list(L["OD"])
    return b, {"inputs": tuple(range(L["nin"])), "outputs": tuple(outs),
               "temps": L["ntemps"], "ccx": n_ccx[0], "kind": "bm-round"}


def build_bm_walk(n_rounds: int) -> tuple[object, dict]:
    """Full toy BM walk: n_rounds chained uniform rounds (fresh banks).

    S_1..S_N are inputs (N*4 bits); initial sigma/tau/delta/IDELTA are
    constants (X gates where 1), d starts zero. Each round reads the
    previous bank and writes a fresh 65-bit bank (divstep-bank style);
    one shared temp block is alloc'd once and reused across rounds
    (every round uncomputes it clean). Wiring per round r takes
    S_r..S_{r-3} straight from the S bank (plus one shared ZERO
    qubit for padding) — no W registers, no round index anywhere in the
    circuit. Cost is structural: N x single-round (uniform emission).
    Proven over harvested full walks (same harvested-state discipline,
    chained end to end).
    """
    from .kickmix import Builder
    if isinstance(n_rounds, bool) or not isinstance(n_rounds, int):
        raise ValueError(f"n_rounds must be an int, got {n_rounds!r}: refusing")
    if not 1 <= n_rounds <= 8:
        raise ValueError(f"walk covers 1..8 rounds (toy BM cap), got {n_rounds}: refusing")
    N = n_rounds
    S = list(range(4 * N))
    bank0 = 4 * N
    NB = 65
    temps0 = bank0 + NB * (N + 1)
    ntemps = 164
    ZERO = [temps0 + ntemps]

    def bank(r: int) -> dict:
        b0 = bank0 + NB * r
        return {"SIGMA": list(range(b0, b0 + 16)),
                "TAU": list(range(b0 + 16, b0 + 52)),
                "DELTA": list(range(b0 + 52, b0 + 56)),
                "IDELTA": list(range(b0 + 56, b0 + 60)),
                "D": list(range(b0 + 60, b0 + 65))}

    def tempblock() -> dict:
        t = temps0
        P = [list(range(t + 4 * i, t + 4 * (i + 1))) for i in range(4)]
        t += 16
        DISC = list(range(t, t + 4)); t += 4
        FZ, FR, FT = [t], [t + 1], [t + 2]; t += 3
        ALPHA = list(range(t, t + 4)); t += 4
        AINV = list(range(t, t + 4)); t += 4
        NID = list(range(t, t + 4)); t += 4
        REPL = list(range(t, t + 16)); t += 16
        RET = list(range(t, t + 16)); t += 16
        TINCR = list(range(t, t + 5)); t += 5
        TNEG = list(range(t, t + 5)); t += 5
        CARRY = list(range(t, t + 4)); t += 4
        IT = list(range(t, t + 16)); t += 16
        ZT = list(range(t, t + 2)); t += 2
        RD = list(range(t, t + 16)); t += 16
        TD = list(range(t, t + 36)); t += 36
        DD = list(range(t, t + 4)); t += 4
        IDD = list(range(t, t + 4)); t += 4
        DDD = list(range(t, t + 5)); t += 5
        assert t == temps0 + 164
        return {"P": P, "DISC": DISC, "FZ": FZ, "FR": FR, "FT": FT,
                "ALPHA": ALPHA, "AINV": AINV, "NID": NID, "REPL": REPL,
                "RET": RET, "TINCR": TINCR, "TNEG": TNEG, "CARRY": CARRY,
                "IT": IT, "ZT": ZT, "RD": RD, "TD": TD, "DD": DD,
                "IDD": IDD, "DDD": DDD}

    nqubits = temps0 + ntemps + 1
    T = tempblock()
    # structural guard: S, every bank, the temp block, and ZERO are
    # pairwise disjoint (temps are intentionally shared across rounds;
    # banks must never alias — the divstep pool-collision class).
    regions = [set(S)] + [set(sum([bank(r)[k] for k in
                                   ("SIGMA", "TAU", "DELTA", "IDELTA", "D")], []))
                          for r in range(N + 1)]
    pool = set()
    for rgn in regions:
        assert not (pool & rgn), "bank collision (implementation bug): refusing"
        pool |= rgn
    tset = set()
    for k in ("DISC", "FZ", "FR", "FT", "ALPHA", "AINV", "NID", "REPL",
              "RET", "TINCR", "TNEG", "CARRY", "IT", "RD", "TD", "DD",
              "IDD", "DDD"):
        tset.update(T[k])
    for w in T["P"]:
        tset.update(w)
    assert len(tset) == 162 and not (pool & tset), "temp collision: refusing"
    assert ZERO[0] not in pool | tset
    b = Builder(nqubits)
    for q in ZERO:
        b.alloc(q)
    temps = sorted(tset)  # deterministic alloc order
    for q in temps:
        b.alloc(q)
    n_ccx = [0]
    # initial bank: sigma=[1], tau=[1], delta=1, idelta=1, d=0
    B0 = bank(0)
    b.x(B0["SIGMA"][0])
    b.x(B0["TAU"][0])
    b.x(B0["DELTA"][0])
    b.x(B0["IDELTA"][0])
    for r in range(1, N + 1):
        prev, cur = bank(r - 1), bank(r)
        W = []
        for i in range(4):
            k = r - i
            if k < 1:
                W.extend([ZERO[0]] * 4)
            else:
                W.extend(S[4 * (k - 1) + t] for t in range(4))
        L = dict(T)
        L.update({"SIGMA": prev["SIGMA"], "TAU": prev["TAU"],
                  "DELTA": prev["DELTA"], "IDELTA": prev["IDELTA"],
                  "D": prev["D"], "W": W,
                  "OSIG": cur["SIGMA"], "OTAU": cur["TAU"],
                  "ODELTA": cur["DELTA"], "OIDELTA": cur["IDELTA"],
                  "OD": cur["D"]})
        _emit_bm_round(b, n_ccx, L)
    for q in temps:
        b.free(q)
    for q in ZERO:
        b.free(q)
    final = bank(N)
    outs = final["SIGMA"] + final["TAU"] + final["DELTA"] + final["IDELTA"] + final["D"]
    return b, {"inputs": tuple(S), "outputs": tuple(outs), "temps": ntemps + 1,
               "ccx": n_ccx[0], "rounds": N, "kind": "bm-walk"}


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
