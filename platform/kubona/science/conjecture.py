"""Exact finite-domain mathematical law discovery.

This is deliberately not a theorem prover. Given a finite sequence it searches
for the simplest exact polynomial or affine linear recurrence that reproduces a
training prefix, then tests it on a held-out suffix. A surviving candidate is a
**conjecture with a finite-scope certificate**, never a proof and never a
novelty claim.

The value is falsifiability: every candidate carries predictions and an
explicit falsifier, and a candidate that misses its holdout is rejected rather
than reported as a discovered law.
"""

from __future__ import annotations

import hashlib
import math
from dataclasses import dataclass
from fractions import Fraction
from typing import Any, Iterable


MAX_SEQUENCE_TERMS = 200
MAX_ORDER = 6
MAX_DEGREE = 6


def _fraction(value: Any) -> Fraction:
    if isinstance(value, bool):
        raise ValueError("booleans are not sequence terms")
    if isinstance(value, Fraction):
        return value
    if isinstance(value, int):
        return Fraction(value)
    if isinstance(value, float):
        if not math.isfinite(value):
            raise ValueError("sequence terms must be finite")
        return Fraction(str(value))
    if isinstance(value, str):
        text = value.strip()
        if not text:
            raise ValueError("empty sequence term")
        try:
            return Fraction(text)
        except ValueError as exc:
            raise ValueError(f"sequence term {value!r} is not an exact number") from exc
    raise ValueError(f"unsupported sequence term type {type(value).__name__}")


def _solve_linear(matrix: list[list[Fraction]], vector: list[Fraction]) -> list[Fraction] | None:
    n = len(matrix)
    if n == 0 or any(len(row) != n for row in matrix):
        return None
    a = [list(row) + [vector[i]] for i, row in enumerate(matrix)]
    for col in range(n):
        pivot = None
        for row in range(col, n):
            if a[row][col] != 0:
                pivot = row
                break
        if pivot is None:
            return None
        a[col], a[pivot] = a[pivot], a[col]
        scale = a[col][col]
        a[col] = [value / scale for value in a[col]]
        for row in range(n):
            if row == col or a[row][col] == 0:
                continue
            factor = a[row][col]
            a[row] = [a[row][j] - factor * a[col][j] for j in range(n + 1)]
    return [a[i][n] for i in range(n)]


def _poly_eval(coeffs: list[Fraction], n: int) -> Fraction:
    total = Fraction(0)
    for coeff in reversed(coeffs):
        total = total * n + coeff
    return total


def _fit_polynomial(values: list[Fraction], degree: int) -> list[Fraction] | None:
    if degree < 0 or len(values) < degree + 1:
        return None
    matrix = []
    vector = []
    for n in range(degree + 1):
        matrix.append([Fraction(n) ** power for power in range(degree + 1)])
        vector.append(values[n])
    return _solve_linear(matrix, vector)


def _fit_recurrence(values: list[Fraction], order: int) -> list[Fraction] | None:
    if order < 1 or len(values) < 2 * order + 1:
        return None
    rows = []
    vector = []
    for n in range(order, 2 * order + 1):
        rows.append([values[n - lag] for lag in range(1, order + 1)] + [Fraction(1)])
        vector.append(values[n])
    return _solve_linear(rows, vector)


def _format_fraction(value: Fraction) -> str:
    if value.denominator == 1:
        return str(value.numerator)
    return f"{value.numerator}/{value.denominator}"


def _format_polynomial(coeffs: list[Fraction]) -> str:
    parts: list[str] = []
    for power in range(len(coeffs) - 1, -1, -1):
        coeff = coeffs[power]
        if coeff == 0:
            continue
        term = _format_fraction(abs(coeff))
        if power == 1:
            term += "*n"
        elif power > 1:
            term += f"*n^{power}"
        if not parts:
            parts.append(("-" if coeff < 0 else "") + term)
        else:
            parts.append(("- " if coeff < 0 else "+ ") + term)
    return " ".join(parts) if parts else "0"


def _format_recurrence(coeffs: list[Fraction], order: int) -> str:
    parts = []
    for lag, coeff in enumerate(coeffs[:order], start=1):
        if coeff == 0:
            continue
        term = f"{_format_fraction(abs(coeff))}*a(n-{lag})"
        if not parts:
            parts.append(("-" if coeff < 0 else "") + term)
        else:
            parts.append(("- " if coeff < 0 else "+ ") + term)
    constant = coeffs[order]
    if constant != 0 or not parts:
        term = _format_fraction(abs(constant))
        if not parts:
            parts.append(("-" if constant < 0 else "") + term)
        else:
            parts.append(("- " if constant < 0 else "+ ") + term)
    return " ".join(parts)


def _candidate_id(kind: str, law: str, values: list[Fraction]) -> str:
    digest = hashlib.sha256()
    digest.update(kind.encode())
    digest.update(b"\0")
    digest.update(law.encode())
    for value in values:
        digest.update(b"\0")
        digest.update(_format_fraction(value).encode())
    return "cand-" + digest.hexdigest()[:20]


@dataclass(frozen=True)
class ConjectureCandidate:
    candidate_id: str
    kind: str
    law: str
    coefficients: list[str]
    train_terms: int
    holdout_terms: int
    holdout_verified: bool
    predictions: list[str]
    status: str

    def to_dict(self) -> dict[str, Any]:
        return {
            "candidate_id": self.candidate_id,
            "kind": self.kind,
            "law": self.law,
            "coefficients": self.coefficients,
            "train_terms": self.train_terms,
            "holdout_terms": self.holdout_terms,
            "holdout_verified": self.holdout_verified,
            "predictions": self.predictions,
            "status": self.status,
            "proof_status": "not-a-proof",
            "falsifier": (
                "Any next term that differs from the first prediction refutes "
                "this finite-scope conjecture."
            ),
            "certificate": {
                "method": "exact rational arithmetic",
                "residual": "0",
                "scope": f"first {self.train_terms + self.holdout_terms} terms",
                "counterexample": None,
            },
            "limitations": (
                "Finite-prefix extrapolation; a holdout match is not a proof "
                "and does not establish novelty."
            ),
        }


def discover_sequence_laws(
    values: Iterable[Any],
    *,
    holdout: int | None = None,
    max_order: int = MAX_ORDER,
    max_degree: int = MAX_DEGREE,
) -> dict[str, Any]:
    seq = [_fraction(value) for value in values]
    if len(seq) < 3:
        raise ValueError("at least 3 sequence terms are required")
    if len(seq) > MAX_SEQUENCE_TERMS:
        raise ValueError(f"at most {MAX_SEQUENCE_TERMS} sequence terms are accepted")
    order_cap = int(max_order)
    degree_cap = int(max_degree)
    if order_cap < 1 or order_cap > MAX_ORDER:
        raise ValueError(f"max_order must be in [1,{MAX_ORDER}]")
    if degree_cap < 0 or degree_cap > MAX_DEGREE:
        raise ValueError(f"max_degree must be in [0,{MAX_DEGREE}]")
    if holdout is None:
        holdout_n = max(1, min(5, len(seq) // 4))
    else:
        holdout_n = int(holdout)
    if holdout_n < 1 or holdout_n >= len(seq) - 2:
        raise ValueError("holdout must leave at least 3 training terms")
    train = seq[:-holdout_n]
    test = seq[-holdout_n:]
    candidates: list[ConjectureCandidate] = []

    for degree in range(degree_cap + 1):
        coeffs = _fit_polynomial(train, degree)
        if coeffs is None:
            continue
        if any(_poly_eval(coeffs, n) != train[n] for n in range(len(train))):
            continue
        holdout_ok = all(_poly_eval(coeffs, len(train) + i) == value for i, value in enumerate(test))
        if not holdout_ok:
            continue
        predictions = [_format_fraction(_poly_eval(coeffs, len(seq) + i)) for i in range(5)]
        law = f"a(n) = {_format_polynomial(coeffs)}"
        candidates.append(ConjectureCandidate(
            candidate_id=_candidate_id("polynomial", law, seq),
            kind="polynomial", law=law,
            coefficients=[_format_fraction(c) for c in coeffs],
            train_terms=len(train), holdout_terms=len(test),
            holdout_verified=True, predictions=predictions, status="conjecture",
        ))

    for order in range(1, order_cap + 1):
        coeffs = _fit_recurrence(train, order)
        if coeffs is None:
            continue
        def eval_rec(n: int) -> Fraction:
            if n < len(train):
                return train[n]
            total = coeffs[order]
            for lag, coeff in enumerate(coeffs[:order], start=1):
                total += coeff * eval_rec(n - lag)
            return total
        if any(eval_rec(n) != train[n] for n in range(len(train))):
            continue
        holdout_ok = all(eval_rec(len(train) + i) == value for i, value in enumerate(test))
        if not holdout_ok:
            continue
        predictions = [_format_fraction(eval_rec(len(seq) + i)) for i in range(5)]
        law = f"a(n) = {_format_recurrence(coeffs, order)}"
        candidates.append(ConjectureCandidate(
            candidate_id=_candidate_id("linear_recurrence", law, seq),
            kind="linear_recurrence", law=law,
            coefficients=[_format_fraction(c) for c in coeffs],
            train_terms=len(train), holdout_terms=len(test),
            holdout_verified=True, predictions=predictions, status="conjecture",
        ))

    unique: list[ConjectureCandidate] = []
    seen: set[tuple[str, str]] = set()
    for candidate in sorted(candidates, key=lambda c: (
        0 if c.kind == "polynomial" else 1,
        len(c.coefficients),
        c.law,
    )):
        key = (candidate.kind, candidate.law)
        if key in seen:
            continue
        seen.add(key)
        unique.append(candidate)
    candidates = unique
    return {
        "success": True,
        "kind": "conjecture",
        "result_version": "experiment-result-v1",
        "input_terms": [_format_fraction(v) for v in seq],
        "train_terms": len(train),
        "holdout_terms": len(test),
        "candidates": [c.to_dict() for c in candidates],
        "candidate_count": len(candidates),
        "status": "conjecture" if candidates else "no_law_verified",
        "measured": False,
        "hardware": False,
        "claimable": False,
        "backend": "exact-rational-cpu",
        "provider": "kubona-exact-arithmetic",
        "message": (
            "A candidate is a finite-scope conjecture with a holdout check, "
            "not a theorem or a novelty claim."
            if candidates else
            "No polynomial or affine linear recurrence survived the held-out terms."
        ),
        "error_message": "",
    }
