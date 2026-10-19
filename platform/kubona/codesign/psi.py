"""Half-degree reciprocal Psi via the square-root split (SI I.1, exact toy).

The paper's scan needs Psi with Psi*sigma' == 1 at every root of sigma
(then Forney division becomes pointwise products). Direct EEA on
(sigma, sigma') runs at degree <= 35. The characteristic-2 split halves it:

  sigma(z) = E(z)^2 + z*O(z)^2,  E_i = sqrt(sigma_{2i}), O_i = sqrt(sigma_{2i+1})
  (squaring is Frobenius-linear: (a+b)^2 = a^2+b^2, so each group is a square;
  sqrt is Frobenius-inverse, exact here, proved element-wise below).
  sigma' = O^2 (squares have zero derivative: (f^2)' = 0 in char 2).
  EEA on the half-degree pair (E,O) gives A*E + B*O = 1; then
  Psi = B^2 + z*A^2 satisfies Psi*sigma' == 1 (mod sigma), i.e. pointwise at
  every root (verified exhaustively at toy scale, never asserted blindly).

Toy caps (stated): binary field n <= 4 (F16), sigma degree <= 4, E/O degree
<= 2. The paper's 35 -> 17 halving is the same program shape one degree up.
"""

from __future__ import annotations


def _check_field(field) -> None:
    for m in ("add", "mul", "inv", "sqr", "n", "size"):
        if not hasattr(field, m):
            raise ValueError(f"field lacks {m} (BinaryField expected): refusing")


def fsqrt(a: int, field) -> int:
    """Frobenius-inverse square root (exact: sqr applied n-1 times)."""
    _check_field(field)
    if not isinstance(a, int) or not (0 <= a < field.size):
        raise ValueError(f"element {a!r} outside field: refusing")
    if a == 0:
        return 0
    x = a
    for _ in range(field.n - 1):
        x = field.sqr(x)
    if field.sqr(x) != a:
        raise ValueError("sqrt round-trip failed (implementation bug): refusing")
    return x


def _norm(p: list[int]) -> list[int]:
    p = list(p)
    while len(p) > 1 and p[-1] == 0:
        p.pop()
    return p if p else [0]


def padd(a: list[int], b: list[int], field) -> list[int]:
    _check_field(field)
    n = max(len(a), len(b))
    return _norm([field.add(a[i] if i < len(a) else 0, b[i] if i < len(b) else 0) for i in range(n)])


def pmul(a: list[int], b: list[int], field) -> list[int]:
    _check_field(field)
    if a == [0] or b == [0]:
        return [0]
    out = [0] * (len(a) + len(b) - 1)
    for i, ca in enumerate(a):
        for j, cb in enumerate(b):
            out[i + j] = field.add(out[i + j], field.mul(ca, cb))
    return _norm(out)


def pscale(p: list[int], s: int, field) -> list[int]:
    _check_field(field)
    if s == 0:
        return [0]
    return _norm([field.mul(c, s) for c in p])


def pdeg(p: list[int]) -> int:
    p = _norm(p)
    return len(p) - 1 if not (len(p) == 1 and p[0] == 0) else -1


def pmod(a: list[int], b: list[int], field) -> list[int]:
    """Remainder of a upon b (leading-term elimination, field divisions)."""
    _check_field(field)
    if pdeg(b) < 0:
        raise ValueError("division by zero polynomial: refusing")
    r = _norm(list(a))
    db = pdeg(b)
    lb = b[db]
    while pdeg(r) >= db and pdeg(r) >= 0:
        dr = pdeg(r)
        coeff = field.mul(r[dr], field.inv(lb))
        shift = dr - db
        for i in range(db + 1):
            r[shift + i] = field.add(r[shift + i], field.mul(coeff, b[i]))
        r = _norm(r)
    return r


def pdivmod(a: list[int], b: list[int], field) -> tuple[list[int], list[int]]:
    _check_field(field)
    if pdeg(b) < 0:
        raise ValueError("division by zero polynomial: refusing")
    r = _norm(list(a))
    db = pdeg(b)
    lb = b[db]
    q = [0] * max(0, pdeg(a) - db + 1)
    while pdeg(r) >= db and pdeg(r) >= 0:
        dr = pdeg(r)
        coeff = field.mul(r[dr], field.inv(lb))
        shift = dr - db
        q[shift] = field.add(q[shift], coeff)
        for i in range(db + 1):
            r[shift + i] = field.add(r[shift + i], field.mul(coeff, b[i]))
        r = _norm(r)
    return _norm(q), _norm(r)


def eea(a: list[int], b: list[int], field) -> tuple[list[int], list[int], list[int]]:
    """Extended Euclid: (g, u, v) with u*a + v*b = g = gcd(a,b).

    Degree-general (the old <= 4 lock was toy proof-scope). The Bezout
    gate verifies every returned triple live, at every degree — a broken
    EEA step cannot slip through scaled-up. Zero pair refuses.
    """
    _check_field(field)
    if pdeg(a) < 0 and pdeg(b) < 0:
        raise ValueError("eea(0,0) undefined: refusing")
    r0, r1 = _norm(list(a)), _norm(list(b))
    u0: list[int] = [1]
    u1: list[int] = [0]
    v0: list[int] = [0]
    v1: list[int] = [1]
    while pdeg(r1) >= 0:
        q, r2 = pdivmod(r0, r1, field)
        r0, r1 = r1, r2
        u0, u1 = u1, padd(u0, pmul(q, u1, field), field)
        v0, v1 = v1, padd(v0, pmul(q, v1, field), field)
    # verify Bezout (load-bearing: never return an unchecked triple)
    if padd(pmul(u0, a, field), pmul(v0, b, field), field) != _norm(r0):
        raise ValueError("Bezout check failed (implementation bug): refusing")
    return _norm(r0), _norm(u0), _norm(v0)


def pderivative(p: list[int], field) -> list[int]:
    """Formal derivative in characteristic 2 (odd terms shift down)."""
    _check_field(field)
    if len(p) <= 1:
        return [0]
    out = [0] * (len(p) - 1)
    for i in range(1, len(p)):
        if i % 2 == 1:
            out[i - 1] = p[i]
    return _norm(out)


def psqr_poly(p: list[int], field) -> list[int]:
    """Frobenius square of a polynomial: (sum c_i z^i)^2 = sum c_i^2 z^{2i}."""
    _check_field(field)
    if p == [0]:
        return [0]
    out = [0] * (2 * len(p) - 1)
    for i, c in enumerate(p):
        out[2 * i] = field.sqr(c)
    return _norm(out)


def peval(p: list[int], x: int, field) -> int:
    acc = 0
    for c in reversed(_norm(p)):
        acc = field.add(field.mul(acc, x), c)
    return acc


def split_sigma(sigma: list[int], field) -> tuple[list[int], list[int]]:
    """Even/odd sqrt split: sigma = E^2 + z*O^2 (any degree >= 1).

    The split is degree-general (Frobenius linearity); the old 1..4 lock
    was toy proof-scope. The round-trip gate proves every degree live:
    E^2 + z*O^2 must rebuild sigma exactly. Separable sigma always splits
    coprimely (a common factor of E,O would divide both sigma and
    sigma' = O^2), so the EEA step downstream is available for every
    locator that passes check_decode — verified per instance anyway.
    """
    _check_field(field)
    sigma = _norm(list(sigma))
    if pdeg(sigma) < 1:
        raise ValueError(f"sigma needs degree >= 1, got {pdeg(sigma)}: refusing")
    evens = [sigma[i] if i < len(sigma) else 0 for i in range(0, len(sigma), 2)]
    odds = [sigma[i] if i < len(sigma) else 0 for i in range(1, len(sigma), 2)]
    E = _norm([fsqrt(c, field) for c in evens])
    O = _norm([fsqrt(c, field) for c in odds])
    # round-trip gate: E^2 + z*O^2 must rebuild sigma exactly
    rebuilt = padd(psqr_poly(E, field), [0] + psqr_poly(O, field), field)
    if _norm(rebuilt) != sigma:
        raise ValueError("split round-trip failed (implementation bug): refusing")
    return E, O


def reciprocal_psi(sigma: list[int], field) -> dict:
    """Half-degree Psi with exhaustive root-wise verification.

    Returns {E, O, A, B, psi, deg_sigma, deg_eea, roots}. Refuses when E,O
    share a factor (Psi undefined there — honest miss, never a fake Psi).
    """
    E, O = split_sigma(sigma, field)
    g, A, B = eea(E, O, field)
    if pdeg(g) != 0:
        raise ValueError("E,O share a factor (Psi undefined): refusing")
    # normalize g to 1 (nonzero constant scale)
    s = g[0]
    A, B = pscale(A, field.inv(s), field), pscale(B, field.inv(s), field)
    psi = padd(psqr_poly(B, field), [0] + psqr_poly(A, field), field)
    dpsi = pderivative(sigma, field)
    roots = [x for x in range(1, field.size) if peval(sigma, x, field) == 0]
    if not roots:
        raise ValueError("sigma has no nonzero roots (nothing to invert at): refusing")
    for beta in roots:
        if field.mul(peval(psi, beta, field), peval(dpsi, beta, field)) != 1:
            raise ValueError(f"Psi*sigma' != 1 at root {beta} (implementation bug): refusing")
    return {"E": E, "O": O, "A": A, "B": B, "psi": psi,
            "deg_sigma": pdeg(sigma), "deg_eea": max(pdeg(E), pdeg(O)),
            "roots": roots}
