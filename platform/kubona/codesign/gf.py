"""Exact binary-field arithmetic GF(2^n): the decoders' ground layer.

Implements carryless multiplication with modular reduction, squaring,
Fermat inversion, and the field trace — the operations behind the
Reed-Solomon (F2^12), Hermitian (F64), and F2^4-inversion tasks.

Moduli are PINNED and self-verifying: every constructor re-proves
irreducibility (no factor of degree <= n/2) and primitivity (x has full
multiplicative order 2^n - 1) on the spot. A wrong modulus refuses
instead of silently defining a non-field.
"""

from __future__ import annotations


def _deg(u: int) -> int:
    if u <= 0:
        raise ValueError("degree undefined for 0: refusing")
    return u.bit_length() - 1


def _poly_mod(a: int, mod: int) -> int:
    """Remainder of binary-polynomial a upon division by mod."""
    if mod <= 1:
        raise ValueError(f"modulus must exceed 1, got {mod!r}: refusing")
    dm = _deg(mod)
    while a.bit_length() - 1 >= dm and a:
        a ^= mod << (a.bit_length() - 1 - dm)
    return a


def _poly_mul(a: int, b: int) -> int:
    out = 0
    while b:
        if b & 1:
            out ^= a
        a <<= 1
        b >>= 1
    return out


def _is_irreducible(mod: int, n: int) -> bool:
    """No monic factor of degree 1..n//2 (trial division, exact for toy n)."""
    if _deg(mod) != n or not (mod & 1):
        return False
    # linear factors: mod(0) != 0 and mod(1) != 0  <=> odd and odd-weight
    if bin(mod).count("1") % 2 == 0:
        return False
    for d in range(2, n // 2 + 1):
        for cand in range((1 << d) + 1, 1 << (d + 1), 2):
            if _deg(cand) != d:
                continue
            if _poly_mod(mod, cand) == 0:
                return False
    return True


def _order_of_x(mod: int, n: int) -> int | None:
    """Multiplicative order of x mod `mod` (None when not a unit power)."""
    size = (1 << n) - 1
    x = 2  # the polynomial X
    if _poly_mod(_poly_mul(x, 1), mod) == 0:
        return None
    # factor the group order, then strip prime factors while power == 1
    order = size
    m, p = size, 2
    factors: dict[int, int] = {}
    while p * p <= m:
        while m % p == 0:
            factors[p] = factors.get(p, 0) + 1
            m //= p
        p += 1 if p == 2 else 2
    if m > 1:
        factors[m] = factors.get(m, 0) + 1
    for prime in factors:
        while order % prime == 0 and _powmod(x, order // prime, mod, n) == 1:
            order //= prime
    return order if _powmod(x, order, mod, n) == 1 else None


def _powmod(a: int, e: int, mod: int, n: int) -> int:
    if e < 0:
        raise ValueError("negative exponent: refusing")
    out = 1
    base = _poly_mod(a, mod)
    while e:
        if e & 1:
            out = _poly_mod(_poly_mul(out, base), mod)
        base = _poly_mod(_poly_mul(base, base), mod)
        e >>= 1
    return out


class BinaryField:
    """GF(2^n) with pinned irreducible primitive modulus."""

    def __init__(self, n: int, modulus: int):
        if not isinstance(n, int) or n < 1 or n > 12:
            raise ValueError(f"n must be in [1,12], got {n!r}: refusing")
        if not isinstance(modulus, int) or _deg(modulus) != n:
            raise ValueError(f"modulus must be degree {n}, got {modulus!r}: refusing")
        if not _is_irreducible(modulus, n):
            raise ValueError(f"modulus {modulus:#x} is reducible (not a field): refusing")
        order = _order_of_x(modulus, n)
        if order != (1 << n) - 1:
            raise ValueError(
                f"modulus {modulus:#x} not primitive (order(x)={order}): refusing")
        self.n = n
        self.modulus = modulus
        self.size = 1 << n

    def add(self, a: int, b: int) -> int:
        self._check(a)
        self._check(b)
        return a ^ b

    def mul(self, a: int, b: int) -> int:
        self._check(a)
        self._check(b)
        return _poly_mod(_poly_mul(a, b), self.modulus)

    def sqr(self, a: int) -> int:
        self._check(a)
        return self.mul(a, a)

    def inv(self, a: int) -> int:
        """Fermat inversion; inv(0) refuses (callers encode 0^-1 = 0 explicitly)."""
        self._check(a)
        if a == 0:
            raise ValueError("inv(0) undefined: refusing (encode 0^-1 = 0 at the task layer)")
        return _powmod(a, self.size - 2, self.modulus, self.n)

    def trace(self, a: int) -> int:
        """Tr(x) = sum_{r=0}^{n-1} x^{2^r} (always 0 or 1)."""
        self._check(a)
        t = 0
        power = a
        for _ in range(self.n):
            t ^= power
            power = self.sqr(power)
        if t not in (0, 1):
            raise ValueError(f"trace escaped GF(2) (implementation bug): refusing {t!r}")
        return t

    def _check(self, a: int) -> None:
        if isinstance(a, bool) or not isinstance(a, int) or not (0 <= a < self.size):
            raise ValueError(f"field element must be in [0,{self.size}), got {a!r}: refusing")


#: F2^4 with the paper's polynomial X^4 + X + 1 (Extended-Data inversion task).
F24 = BinaryField(4, 0b10011)

#: F64 with primitive X^6 + X + 1 (Hermitian-code symbols; verified below).
F64 = BinaryField(6, 0b1000011)

#: F2^12 with primitive X^12 + X^6 + X^4 + X + 1 (Reed-Solomon symbols).
#: Machine-found (first odd-weight degree-12 candidate in binary order),
#: self-proved primitive by the constructor (irreducible + ord(x) == 4095).
F2_12 = BinaryField(12, 0x1053)


def f24_inversion_table() -> dict[int, int]:
    """The paper's F2^4 map x -> x^-1 with 0^-1 = 0 (16 entries, verified)."""
    table = {}
    for x in range(16):
        table[x] = 0 if x == 0 else F24.inv(x)
    for x in range(1, 16):
        if F24.mul(x, table[x]) != 1:
            raise ValueError(f"F24 table violates x*x^-1 == 1 at {x} (implementation bug)")
    return table


def trace_dual(mask_bits: list[int], field: BinaryField) -> int:
    """Mask-to-field map: the unique mu with Tr(mu * 2^j) == mask_bits[j].

    The paper's phase step turns twelve mask bits c_j into mu with
    <c,e>_2 = Tr(mu*e) (fixed binary basis). The trace pairing is
    nondegenerate, so the n x n system always solves; the solution is
    verified equation-by-equation (a residual is an implementation bug,
    refused — never an approximate mu).
    """
    if not isinstance(field, BinaryField):
        raise ValueError(f"field must be BinaryField, got {type(field).__name__}: refusing")
    n = field.n
    if not isinstance(mask_bits, list) or len(mask_bits) != n \
            or any(b not in (0, 1) for b in mask_bits):
        raise ValueError(f"mask_bits must be {n} bits 0/1: refusing")
    basis = [1 << j for j in range(n)]
    # M[row j][col i] = Tr(2^i * 2^j); solve M mu = mask over GF(2).
    rows = []
    for j in range(n):
        mask = 0
        for i in range(n):
            if field.trace(field.mul(basis[i], basis[j])):
                mask |= 1 << i
        rows.append(mask)
    rhs = sum((b & 1) << j for j, b in enumerate(mask_bits))
    # Gauss-Jordan with augmented column tracked alongside.
    mat = list(rows)
    aug = [(rhs >> j) & 1 for j in range(n)]
    where = [-1] * n
    row = 0
    for col in range(n):
        piv = next((i for i in range(row, n) if (mat[i] >> col) & 1), None)
        if piv is None:
            continue
        mat[row], mat[piv] = mat[piv], mat[row]
        aug[row], aug[piv] = aug[piv], aug[row]
        where[col] = row
        for i in range(n):
            if i != row and ((mat[i] >> col) & 1):
                mat[i] ^= mat[row]
                aug[i] ^= aug[row]
        row += 1
    for i in range(row, n):
        if mat[i] == 0 and aug[i] == 1:
            raise ValueError("trace system inconsistent (implementation bug): refusing")
    mu = 0
    for col in range(n):
        if where[col] != -1 and aug[where[col]]:
            mu |= 1 << col
    for j in range(n):
        if field.trace(field.mul(mu, basis[j])) != mask_bits[j]:
            raise ValueError(f"dual residual at bit {j} (implementation bug): refusing")
    return mu
