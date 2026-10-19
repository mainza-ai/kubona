"""Hermitian-curve machinery (SI I.2, exact over F64 — no floating anything).

The HOPI code lives on y^8 = x^9 + y (char 2: y^8 + y + x^9 = 0) with
monomials x^a y^b (0 <= b <= 7) ordered by weighted degree 8a + 9b.
Paper anchors reproduced here as machine-checked counts (never cited):
512 affine curve points, 123 monomials of weighted degree <= 150 (syndrome
basis), 48 of degree <= 75 (locator span).

Contents (all exact field arithmetic):
- curve_points(): the 512 positions (enumerated, count-gated).
- monomials(max_wdeg): basis monomials (count-gated at 150/75).
- padd/pmul/reduce/eval_at: bivariate polynomials reduced by the curve
  relation (y^8 -> x^9 + y); reduction soundness is asserted live
  (reduce-then-eval == eval at every curve point on probe inputs).
- syndrome(errors, psis): S(psi) = sum_j e_j psi(P_j) for ANY function
  (the generalized syndrome; verification side needs no 123-limit).
- curve_derivative(sigma): sigma' = dx sigma + x^8 dy sigma (formal
  characteristic-2 partials). Transverse locators read nonzero (simple
  zeros admit the residue formula); repeated zeros give sigma' == 0 and
  the residue honestly refuses (use error_value_local instead).
- locator_recurrence_holds(sigma_coeffs, errors, basis, kprime): the
  nullspace foundation 0 = sum_k sigma_k S(psi_k psi_k') per k'.
- local_expansion(poly, pt, field, n): first n coefficients along the
  uniformizer t = x - x0 (x is a uniformizer at every affine point:
  df = x^8 dx + dy never kills dx). y(t) solved order by order from the
  curve equation (Frobenius + binomial parity, exact). n <= 4.
- error_value_local(sigma, omega, pt, field): the paper's rule — ratio
  omega(P)/sigma'(P) at simple points (== residue), power-series quotient
  at repeated zeros (multiplicity <= 3, else refuse; pole refuses; extra
  vanishing reads 0, "contributes nothing"). Recovers planted values
  exactly; old residue() refuses precisely where this takes over.
- omega_series(syndromes, sigma, basis, field): the RS-analog evaluator
  omega = sigma * S_hat with S_hat = sum s_i psi_i (bilinear in locator
  and syndromes). This is the TRUNCATED syndrome polynomial, not the
  paper's true series-at-infinity: with it, error points read 0 (pinned
  in-test as the documented boundary — the recovery identity needs the
  true series + residue theorem, queued, never claimed here).
- scan_direct / scan_stripes: per-point local evaluation over all 512
  points, whole vs 16-point stripes (64 omega-symbols max per stripe —
  4 orders x 16 points; the paper holds 48 in its local-coordinate
  choice — never a global omega in the striped path). Striped == direct
  proven; storage shapes machine-checked (direct holds > 100 symbols,
  striped bounded by a point-count-independent constant — the paper's
  stated bounded-vs-global tradeoff shape); mult counts measured both
  ways, no winner declared.

Scope (stated): locator FINDING via the scheduled F64 walk lands in
hlanczos (ScheduledFieldWalker: freeze + scheduled repair rounds, ending
test, three-case routing, history, reverse). The true syndrome series +
fraction replacement h_s, eval-interp, and the phase-bit mu-tilde
machinery stay queued, as does the xi-register incremental accumulation;
this module proves the geometry, the local-coefficient rule, and the
stripe batching discipline.
"""

from __future__ import annotations


def _check_field(field) -> None:
    for m in ("add", "mul", "n", "size"):
        if not hasattr(field, m):
            raise ValueError(f"field lacks {m}: refusing")


Poly = dict  # (a, b) -> nonzero field element


def curve_points(field) -> list[tuple[int, int]]:
    """All affine (x, y) with y^8 + y + x^9 == 0 (count must be 512)."""
    _check_field(field)
    if field.n != 6:
        raise ValueError(f"Hermitian curve needs F64, got n={field.n}: refusing")

    def _pow(a: int, e: int) -> int:
        r = 1
        for _ in range(e):
            r = field.mul(r, a)
        return r

    pts = []
    for x in range(field.size):
        x9 = _pow(x, 9)
        for y in range(field.size):
            if field.add(field.add(_pow(y, 8), y), x9) == 0:
                pts.append((x, y))
    if len(pts) != 512:
        raise ValueError(f"curve has {len(pts)} points, want 512 (implementation bug): refusing")
    return pts


def monomials(max_wdeg: int) -> list[tuple[int, int]]:
    """(a, b) with 0 <= b <= 7 and 8a + 9b <= max_wdeg (ordered by degree)."""
    if not isinstance(max_wdeg, int) or max_wdeg < 0 or max_wdeg > 150:
        raise ValueError(f"max_wdeg must be in [0,150], got {max_wdeg!r}: refusing")
    out = [(a, b) for b in range(8) for a in range(max_wdeg // 8 + 1)
           if 8 * a + 9 * b <= max_wdeg]
    out.sort(key=lambda t: (8 * t[0] + 9 * t[1], t[0], t[1]))
    if max_wdeg == 150 and len(out) != 123:
        raise ValueError(f"syndrome basis has {len(out)} monomials, want 123: refusing")
    if max_wdeg == 75 and len(out) != 48:
        raise ValueError(f"locator span has {len(out)} monomials, want 48: refusing")
    return out


def hadd(p: Poly, q: Poly, field) -> Poly:
    _check_field(field)
    out = dict(p)
    for k, v in q.items():
        out[k] = field.add(out.get(k, 0), v)
        if out[k] == 0:
            del out[k]
    return out


def hmul(p: Poly, q: Poly, field) -> Poly:
    _check_field(field)
    out: Poly = {}
    for (a1, b1), c1 in p.items():
        for (a2, b2), c2 in q.items():
            k = (a1 + a2, b1 + b2)
            out[k] = field.add(out.get(k, 0), field.mul(c1, c2))
            if out[k] == 0:
                del out[k]
    return out


def hreduce(p: Poly, field) -> Poly:
    """Reduce by y^8 = x^9 + y until all b <= 7 (terminates: b drops)."""
    _check_field(field)
    out = dict(p)
    while True:
        target = next(((a, b) for (a, b) in out if b >= 8), None)
        if target is None:
            return out
        a, b = target
        c = out.pop(target)
        # y^8 -> x^9 + y: c x^a y^b = c x^(a+9) y^(b-8) + c x^a y^(b-7)
        for k in ((a + 9, b - 8), (a, b - 7)):
            out[k] = field.add(out.get(k, 0), c)
            if out[k] == 0:
                del out[k]


def heval_at(p: Poly, pt: tuple[int, int], field) -> int:
    """Evaluate at a point (caller reduces first if it wants the normal form)."""
    _check_field(field)
    x, y = pt
    acc = 0
    xpows = {0: 1}
    ypows = {0: 1}
    for (a, b), c in p.items():
        while len(xpows) <= a:
            xpows[len(xpows)] = field.mul(xpows[len(xpows) - 1], x)
        while len(ypows) <= b:
            ypows[len(ypows)] = field.mul(ypows[len(ypows) - 1], y)
        acc = field.add(acc, field.mul(c, field.mul(xpows[a], ypows[b])))
    return acc


def _partial(p: Poly, var: str, field) -> Poly:
    """Formal characteristic-2 partial (odd exponents survive, shifted down)."""
    out: Poly = {}
    for (a, b), c in p.items():
        if var == "x" and a % 2 == 1:
            out[(a - 1, b)] = c
        if var == "y" and b % 2 == 1:
            out[(a, b - 1)] = c
    return out


def curve_derivative(sigma: Poly, field) -> Poly:
    """sigma' = dx sigma + x^8 dy sigma along the curve (paper Eq residue)."""
    _check_field(field)
    dx = _partial(sigma, "x", field)
    dy = _partial(sigma, "y", field)
    x8 = {(8, 0): 1}
    return hreduce(hadd(dx, hmul(x8, dy, field), field), field)


def syndrome(errors: list[tuple[tuple[int, int], int]], psi: Poly, field) -> int:
    """Generalized syndrome S(psi) = sum_j e_j psi(P_j) (any function)."""
    _check_field(field)
    acc = 0
    for pt, e in errors:
        if not (0 < e < field.size):
            raise ValueError(f"error value {e!r} outside field: refusing")
        acc = field.add(acc, field.mul(e, heval_at(psi, pt, field)))
    return acc


def locator_recurrence_holds(sigma: Poly, errors: list[tuple[tuple[int, int], int]],
                             basis: list[tuple[int, int]], field) -> bool:
    """0 = sum_k sigma_k S(psi_k psi_k') for every k' (nullspace foundation).

    sigma as {(a,b): coeff} over the basis monomials psi_k = x^a y^b.
    Products reduce by the curve relation; syndromes extend linearly.
    """
    _check_field(field)
    monos = {(a, b): 1 for (a, b) in basis}
    for kprime in basis:
        acc = 0
        kp = {(kprime[0], kprime[1]): 1}
        for k, ck in sigma.items():
            if k not in monos:
                raise ValueError(f"sigma term {k!r} outside basis: refusing")
            prod = hreduce(hmul({k: ck}, kp, field), field)
            acc = field.add(acc, syndrome(errors, prod, field))
        if acc != 0:
            return False
    return True


def residue(sigma: Poly, omega_val: int, pt: tuple[int, int], field) -> int:
    """Simple-zero error value e = omega(P)/sigma'(P) (exact division).

    Refuses when sigma'(P) == 0 (repeated zero — use error_value_local,
    which takes over precisely there via the series quotient).
    """
    _check_field(field)
    d = heval_at(curve_derivative(sigma, field), pt, field)
    if d == 0:
        raise ValueError("sigma'(P) == 0 (repeated zero): refusing division")
    return field.mul(omega_val, field.inv(d))


def _fpow(field, a: int, e: int) -> int:
    """a^e by repeated squaring (exact, counted as mults like everything)."""
    r, base, exp = 1, a, e
    while exp:
        if exp & 1:
            r = field.mul(r, base)
        base = field.mul(base, base)
        exp >>= 1
    return r


def _y_series(x0: int, y0: int, field, K: int) -> list[int]:
    """y(t) = y0 + a1 t + ... + aK t^K along the curve at (x0, y0).

    From y^8 + y + x^9 = 0 with x(t) = x0 + t: Frobenius kills cross
    terms (y(t)^8 = sum a_j^8 t^{8j}), binomial parity kills most of
    x(t)^9 (C(9,k) odd iff k submask of 9, i.e. k in {0,1,8,9}), so
    a_k = [x^9]_k + (a_{k/8}^8 if 8 | k else 0) with a_0 = y0.
    In particular a1 = x0^8 (dy = x^8 dx) and a2 = a3 = 0 always.
    """
    _check_field(field)
    a = [y0]  # a_0
    for k in range(1, K + 1):
        # [x^9]_k = C(9,k) x0^{9-k}, nonzero mod 2 iff k submask of 9
        binom = _fpow(field, x0, 9 - k) if (k & ~9) == 0 else 0
        frob = _fpow(field, a[k // 8], 8) if k % 8 == 0 else 0
        a.append(field.add(binom, frob))
    return a[1:]


def _trunc_mul(A: list[int], B: list[int], field, n: int) -> list[int]:
    """Polynomial product mod t^n (exact, small n)."""
    out = [0] * n
    for i, x in enumerate(A):
        if x == 0:
            continue
        for j, y in enumerate(B):
            if i + j >= n or y == 0:
                continue
            out[i + j] = field.add(out[i + j], field.mul(x, y))
    return out


def _x_pow_series(x0: int, a: int, field, n: int) -> list[int]:
    """(x0 + t)^a mod t^n (binomial parity: C(a,k) odd iff (k & a) == k)."""
    if a < 0:
        raise ValueError(f"negative x-exponent {a}: refusing")
    out = [0] * n
    for k in range(min(a, n - 1) + 1):
        if (k & a) == k:
            out[k] = _fpow(field, x0, a - k)
    return out


def _poly_pow_series(Y: list[int], b: int, field, n: int) -> list[int]:
    """Y(t)^b mod t^n by repeated squaring (b <= 7 after reduction)."""
    if b < 0:
        raise ValueError(f"negative y-exponent {b}: refusing")
    r, base, exp = [1] + [0] * (n - 1), list(Y), b
    while exp:
        if exp & 1:
            r = _trunc_mul(r, base, field, n)
        base = _trunc_mul(base, base, field, n)
        exp >>= 1
    return r


def local_expansion(poly: Poly, pt: tuple[int, int], field, n: int) -> list[int]:
    """First n coefficients of a curve function along t = x - x0.

    c0 is the value at pt; c1 is sigma'(P) (== curve_derivative eval,
    pinned over all 512 points in-test). Reduction first (same function,
    bounded y-degree). n in 1..4: multiplicity support caps at 3 per the
    paper, and order 3 is the deepest quotient needed. Field-generic over
    any BinaryField (the curve relation is characteristic-2 throughout);
    only the point/basis counts are F64-specific.
    """
    _check_field(field)
    if isinstance(n, bool) or not isinstance(n, int) or not (1 <= n <= 4):
        raise ValueError(f"n must be in [1,4], got {n!r}: refusing")
    x0, y0 = pt
    red = hreduce(dict(poly), field)
    a = _y_series(x0, y0, field, n - 1)
    Y = ([y0] + a + [0] * n)[:n]
    out = [0] * n
    for (ap, bp), c in red.items():
        term = _trunc_mul(_x_pow_series(x0, ap, field, n),
                          _poly_pow_series(Y, bp, field, n), field, n)
        for k in range(n):
            if term[k]:
                out[k] = field.add(out[k], field.mul(c, term[k]))
    return out


def _value_from_expansions(s: list[int], w: list[int], field) -> int:
    """One logic home for the paper's rule (ratio at simple, quotient at
    repeated). s, w are order-4 local expansions of sigma, omega."""
    if s[1] != 0:
        return field.mul(w[0], field.inv(s[1]))  # paper Eq residue
    if s[0] != 0:
        raise ValueError("sigma'(P) == 0 at a non-zero (singular): refusing")
    for m in (2, 3):  # repeated zero: cancel the common zero, then read
        if s[m] != 0:
            if any(v != 0 for v in w[:m]):
                raise ValueError("omega vanishes less than sigma (pole): refusing")
            for j in range(m, 4):
                if w[j] != 0:
                    return field.mul(w[j], field.inv(s[m])) if j == m else 0
            return 0  # extra vanishing: contributes nothing (paper)
    raise ValueError("sigma vanishes to order >= 4 (beyond multiplicity 3): refusing")


def error_value_local(sigma: Poly, omega: Poly, pt: tuple[int, int], field) -> int:
    """Error value from local coefficients (paper 0/0 rule).

    Simple points take the ratio (== residue()); repeated zeros take the
    series quotient (multiplicity <= 3); poles, singular ratios, and
    order >= 4 refuse fail-closed.
    """
    _check_field(field)
    return _value_from_expansions(local_expansion(sigma, pt, field, 4),
                                  local_expansion(omega, pt, field, 4), field)


def omega_series(syndromes: list[int], sigma: Poly, basis: list[tuple[int, int]],
                 field) -> Poly:
    """Evaluator omega = sigma * S_hat (RS-analog, bilinear).

    S_hat = sum_i s_i psi_i over the 123 syndrome monomials — the
    truncated syndrome polynomial, counterpart of RS S(z). Bilinear in
    (locator, syndromes) as the paper states for the true evaluator.
    """
    _check_field(field)
    if len(syndromes) != len(basis):
        raise ValueError(f"need one syndrome per basis monomial "
                         f"({len(syndromes)} vs {len(basis)}): refusing")
    span48 = set(monomials(75))
    for k in sigma:
        if k not in span48:
            raise ValueError(f"sigma term {k!r} outside 48-span: refusing")
    shat: Poly = {}
    for (a, b), s in zip(basis, syndromes):
        if s % field.size != s or not (0 <= s < field.size):
            raise ValueError(f"syndrome {s!r} outside field: refusing")
        if s:
            shat[(a, b)] = field.add(shat.get((a, b), 0), s)
    return hreduce(hmul(dict(sigma), shat, field), field)


def _scan_value(sigma: Poly, omega: Poly, pt: tuple[int, int], field) -> int:
    """Per-point scan value (shared by direct and striped paths)."""
    return error_value_local(sigma, omega, pt, field)


def scan_direct(syndromes: list[int], sigma: Poly, points: list[tuple[int, int]],
                field) -> tuple[dict, dict]:
    """Whole-evaluator scan: form omega once, evaluate at every point.

    Returns (values, info) with info["omega_terms"] = symbols held through
    the whole scan (the paper's "> 100 field symbols" — reproduced as a
    machine-checked count, never cited).
    """
    basis = monomials(150)
    omega = omega_series(syndromes, sigma, basis, field)
    return ({pt: _scan_value(sigma, omega, pt, field) for pt in points},
            {"omega_terms": len(omega)})


def scan_stripes(syndromes: list[int], sigma: Poly, points: list[tuple[int, int]],
                 field, stripe: int = 16) -> tuple[dict, dict]:
    """Striped scan: 16-point batches, 48 omega-symbols max per stripe.

    Per point, omega's local coefficients come from sigma(t)*S_hat(t) mod
    t^4 — the global omega is never formed (no omega_series call on this
    path). Each stripe's omega-symbols are used and dropped before the
    next stripe (the paper's batch/erase discipline at toy scale).
    Four orders are kept (order 3 feeds the multiplicity-3 quotient);
    the paper keeps three in its coordinate choice — same bounded shape.
    Returns (values, info) with the max stripe-held omega-symbol count.
    """
    _check_field(field)
    if isinstance(stripe, bool) or not isinstance(stripe, int) or stripe < 1:
        raise ValueError(f"stripe must be a positive int, got {stripe!r}: refusing")
    basis = monomials(150)
    if len(syndromes) != len(basis):
        raise ValueError(f"need {len(basis)} syndromes, got {len(syndromes)}: refusing")
    shat: Poly = {(a, b): s for (a, b), s in zip(basis, syndromes) if s}
    values: dict = {}
    held = 0
    for lo in range(0, len(points), stripe):
        batch = points[lo:lo + stripe]
        wlocs = []
        for pt in batch:
            sig_t = local_expansion(sigma, pt, field, 4)
            shat_t = local_expansion(shat, pt, field, 4)
            w_t = [0] * 4
            for i, x in enumerate(sig_t):
                if x == 0:
                    continue
                for j, y in enumerate(shat_t):
                    if i + j < 4 and y != 0:
                        w_t[i + j] = field.add(w_t[i + j], field.mul(x, y))
            wlocs.append((pt, sig_t, w_t))
        held = max(held, 4 * len(batch))
        for pt, sig_t, w_t in wlocs:
            values[pt] = _value_from_expansions(sig_t, w_t, field)
    return values, {"stripe_omega_symbols": held,
                    "nstripes": (len(points) + stripe - 1) // stripe}
