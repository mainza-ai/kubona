"""Verification primitives for quantum-advantage claims (fail-closed).

Two structural diagnostics plus a claim-intake checklist. Rationale: the
fastest kills of recent advantage claims were structural, not computational
— mirror (U...U†) constructions that uncompute themselves, and quasi-1D
geometries that transverse contraction eats. A verifier that does not check
structure first will certify its own spoofing.

  - mirror_fraction: how much of the circuit's second half inverts the
    first (gate-wise inverse, reversed order). High values mean the
    "peak" may be an uncomputation artifact — demand contraction-width
    analysis before any advantage reading.
  - min_entanglement_cut: fewest two-qubit gates crossing any qubit
    bipartition (exact for n <= 12, reported unmeasured above — an
    uncomputed diagnostic is a gap, never a pass).
  - intake_checklist: three-task separation. Sample-only statistics
    (XEB/HOG/anticoncentration without fidelity) can NEVER witness
    advantage — such claims refuse outright. Mirrored or quasi-1D
    circuits flag for width analysis. Everything else passes to
    cost-estimated review.

Thresholds are documented heuristics (v1), not constants of nature:
QUASI_1D_CUT < n_qubits / 2; MIRROR_FLAG >= 0.5.
"""

from __future__ import annotations

import itertools
import math
from typing import Any

from kubona.circuits.model import Circuit, Gate

MIRROR_FLAG_THRESHOLD = 0.5

_SELF_INVERSE_1Q = frozenset({"id", "x", "y", "z", "h", "cx", "cz", "swap"})
_INVERSE_PAIR = {"t": "tdg", "tdg": "t", "s": "sdg", "sdg": "s"}
_PARAM_INVERSE = frozenset({"rx", "ry", "rz", "cp"})


def _is_inverse(a: Gate, b: Gate) -> bool:
    """True when b inverts a (same qubits, inverse operation)."""
    if a.qubits != b.qubits:
        return False
    if a.name == b.name:
        if a.name in _SELF_INVERSE_1Q:
            return True
        if a.name in _PARAM_INVERSE:
            pa = a.params[0] if a.params else 0.0
            pb = b.params[0] if b.params else 0.0
            return math.isclose((pa + pb) % (2 * math.pi), 0.0, abs_tol=1e-6)
        return False
    if _INVERSE_PAIR.get(a.name) == b.name:
        return True
    return False


def mirror_fraction(circuit: Circuit) -> dict[str, Any]:
    """Fraction of the second half that inverts the first, reversed.

    A perfect U...U† mirror scores 1.0 (matched pairs / half length).
    Short circuits (< 4 gates) report unmeasured — too little structure
    to judge, and a verdict either way would be invented.
    """
    gates = circuit.gates
    half = len(gates) // 2
    if half < 2:
        return {"mirror_fraction": None, "measured": False,
                "reason": "fewer than 4 gates: no mirror structure to assess"}
    first, second = gates[:half], gates[half:half * 2]
    matched = sum(1 for g1, g2 in zip(first, reversed(second))
                  if _is_inverse(g1, g2))
    return {"mirror_fraction": round(matched / half, 4), "measured": True,
            "matched_pairs": matched, "half_length": half}


def min_entanglement_cut(circuit: Circuit) -> dict[str, Any]:
    """Fewest two-qubit gates crossing any qubit bipartition.

    Exact by subset enumeration for n <= 12; above that the diagnostic
    reports unmeasured (fail-closed: an uncomputed width is a gap, and a
    heuristic number would launder guesswork into the verdict).
    """
    n = circuit.num_qubits
    pairs = [(g.qubits[0], g.qubits[1]) for g in circuit.gates
             if len(g.qubits) == 2]
    if n > 12:
        return {"min_cut": None, "measured": False,
                "reason": f"n={n} > 12: subset enumeration refused"}
    if not pairs:
        return {"min_cut": 0, "measured": True, "partition": [[], list(range(n))],
                "note": "no two-qubit gates: trivially separable"}
    best: tuple[int, list[int]] = (len(pairs) + 1, [])
    for r in range(1, n):
        for left in itertools.combinations(range(n), r):
            inside = set(left)
            cut = sum(1 for a, b in pairs if (a in inside) != (b in inside))
            if cut < best[0]:
                best = (cut, sorted(inside))
    width, left = best
    right = sorted(set(range(n)) - set(left))
    return {"min_cut": width, "measured": True,
            "partition": [left, right]}


def quasi_1d(cut_report: dict[str, Any], n_qubits: int) -> bool | None:
    """Narrow-cut flag (v1 heuristic: min_cut < n/2). None when unmeasured."""
    if not cut_report.get("measured"):
        return None
    return bool(cut_report["min_cut"] < n_qubits / 2)


def intake_checklist(circuit: Circuit, claim_kind: str,
                     fidelity: float | None = None) -> dict[str, Any]:
    """Three-task separation for incoming advantage claims.

    claim_kind: "exact_amplitude" | "bounded_fidelity_sampling" | "score_only".
    - score_only (XEB/HOG/anticoncentration with no fidelity): REFUSE.
      No sample-only statistic can witness advantage.
    - mirrored (fraction >= threshold) or quasi-1D: FLAG for
      contraction-width analysis before any reading.
    - otherwise: PASS to cost-estimated review (not approval — the cost
      estimator and classical anchor still decide).
    """
    valid = ("exact_amplitude", "bounded_fidelity_sampling", "score_only")
    if claim_kind not in valid:
        return {"verdict": "REFUSE", "reasons": [f"unknown claim kind {claim_kind!r}"]}
    reasons: list[str] = []
    if claim_kind == "score_only":
        return {"verdict": "REFUSE",
                "reasons": ["sample-only statistics (XEB/HOG/anticoncentration) "
                            "without fidelity cannot witness advantage"]}

    mirror = mirror_fraction(circuit)
    if mirror["measured"] and mirror["mirror_fraction"] >= MIRROR_FLAG_THRESHOLD:
        reasons.append(f"mirror structure {mirror['mirror_fraction']}: possible "
                       "uncomputation artifact — require contraction-width analysis")
    cut = min_entanglement_cut(circuit)
    narrow = quasi_1d(cut, circuit.num_qubits)
    if narrow is None:
        reasons.append("entanglement width unmeasured: geometry risk unevaluated")
    elif narrow:
        reasons.append(f"narrow entanglement cut ({cut['min_cut']} gates): "
                       "transverse-contraction risk — require width analysis")
    if claim_kind == "bounded_fidelity_sampling" and fidelity is None:
        reasons.append("bounded-fidelity claim without a fidelity number")
    if not reasons:
        return {"verdict": "PASS", "reasons": ["no structural spoofing signals"],
                "mirror": mirror, "cut": cut}
    return {"verdict": "FLAG", "reasons": reasons,
            "mirror": mirror, "cut": cut}
