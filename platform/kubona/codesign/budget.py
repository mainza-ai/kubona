"""Failure-budget ledgerrows(SI Table-S4 style).

Each truncation/stack/window cut carries {source, per_use_rate, uses};
the union bound is min(1, sum(rate*uses)). Rates are upper bounds from
distribution sampling (cf. 400M BM sims, 200M walks); uses are counts in
one benchmark invocation. NaN/Inf/negative refuse — a budget that cannot
be stated fail-closed is not a budget.
"""

from __future__ import annotations

import math
from dataclasses import dataclass


@dataclass(frozen=True)
class BudgetEntry:
    source: str
    per_use_rate: float
    uses: int

    def __post_init__(self) -> None:
        if not isinstance(self.source, str) or not self.source.strip():
            raise ValueError("source must be a non-blank string: refusing")
        r = self.per_use_rate
        if not isinstance(r, (int, float)) or isinstance(r, bool) \
                or not math.isfinite(float(r)) or not (0.0 <= float(r) <= 1.0):
            raise ValueError(f"per_use_rate must be finite in [0,1], got {r!r}: refusing")
        if isinstance(self.uses, bool) or not isinstance(self.uses, int) or self.uses < 0:
            raise ValueError(f"uses must be non-negative int, got {self.uses!r}: refusing")

    def contribution(self) -> float:
        return float(self.per_use_rate) * self.uses


def union_bound(entries: list[BudgetEntry]) -> float:
    total = 0.0
    for e in entries:
        if not isinstance(e, BudgetEntry):
            raise ValueError(f"entries must be BudgetEntry, got {e!r}: refusing")
        total += e.contribution()
    return min(1.0, total)


def specialized_euclid_rows() -> list[BudgetEntry]:
    """Paper SI II.2 empirical failure ledger (inline estimates, labeled).

    10M sampled walks: 72 outside fitted windows + 258 missing test-free
    positions (~6.6e-5 combined); schedule cut 1.3e-5; truncated multipliers
    at most 2e-6; total about 8e-5 per point addition (4/100k tests fail).
    The paper says "about" throughout — rows carry the stated values with
    the same honesty (estimates, not bounds).
    """
    return [
        BudgetEntry("fitted-window-escape", 6.6e-5, 1),
        BudgetEntry("schedule-cut-tail", 1.3e-5, 1),
        BudgetEntry("truncated-multipliers", 2e-6, 1),
    ]


def divstep_s4_rows() -> list[BudgetEntry]:
    """Paper Table-S4 point-addition failure ledger (measured contributions).

    Values transcribed verbatim (labels + upper bounds); the union is
    1.599e-4. The paper publishes per-point-addition CONTRIBUTIONS (two
    inversions each; walk limits from 200M sampled walks), not the
    rate/uses split — so each row is stored as rate=contribution x uses=1.
    That folding is declared here, not hidden: do not read a per-use rate
    out of these rows.
    """
    return [
        BudgetEntry("multiplier-carry-and-comparison", 7.07e-5, 1),
        BudgetEntry("stopping-time-and-integer-size", 7.56e-5, 1),
        BudgetEntry("early-stopping-and-z-register", 6.18e-6, 1),
        BudgetEntry("branch-history-storage", 6.8e-6, 1),
        BudgetEntry("fused-square-carry", 6e-7, 1),
    ]


def ledger_rows(entries: list[BudgetEntry]) -> list[dict]:
    total = union_bound(entries)
    rows = [{
        "source": e.source,
        "per_use_rate": float(e.per_use_rate),
        "uses": e.uses,
        "contribution": e.contribution(),
    } for e in entries]
    rows.append({"source": "union_bound", "per_use_rate": None,
                 "uses": None, "contribution": total})
    return rows
