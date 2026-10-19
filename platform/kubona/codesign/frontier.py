"""MAP-Elites (T,Q) frontier: keep min-TQ per cell.

Paper: divide the (T,Q) plane into cells, keep lowest TQ per cell so a
space-efficient circuit survives a worse TQ. Progress need not be monotonic.
"""

from __future__ import annotations

from dataclasses import dataclass, field
from typing import Any

from .tq import TQ_KAPPA_DEFAULT, tq_cost


@dataclass
class FrontierEntry:
    artifact: Any
    toffoli: int
    qubits: int
    tq: int
    meta: dict[str, Any] = field(default_factory=dict)


class MapElitesFrontier:
    """Grid over (T,Q) log-buckets keeping the min-TQ entry per cell."""

    def __init__(self, t_buckets: int = 12, q_buckets: int = 12, kappa: int = TQ_KAPPA_DEFAULT):
        if t_buckets <= 0 or q_buckets <= 0:
            raise ValueError("bucket counts must be positive")
        self.t_buckets = t_buckets
        self.q_buckets = q_buckets
        self.kappa = kappa
        self._cells: dict[tuple[int, int], FrontierEntry] = {}

    @staticmethod
    def _bucket(v: int, n: int) -> int:
        # log2 buckets: 0->0, 1->1, 2-3->2, 4-7->3, ...
        if v <= 0:
            return 0
        b = v.bit_length()
        return min(b, n - 1)

    def cell(self, t: int, q: int) -> tuple[int, int]:
        return (self._bucket(t, self.t_buckets), self._bucket(q, self.q_buckets))

    def offer(self, artifact: Any, toffoli: int, qubits: int, meta: dict | None = None) -> bool:
        tq = tq_cost(toffoli, qubits, self.kappa)
        c = self.cell(toffoli, qubits)
        cur = self._cells.get(c)
        if cur is None or tq < cur.tq:
            self._cells[c] = FrontierEntry(artifact, toffoli, qubits, tq, dict(meta or {}))
            return True
        return False

    def best(self) -> FrontierEntry | None:
        if not self._cells:
            return None
        return min(self._cells.values(), key=lambda e: e.tq)

    def entries(self) -> list[FrontierEntry]:
        return sorted(self._cells.values(), key=lambda e: e.tq)

    def to_dict(self) -> dict:
        best = self.best()
        return {
            "kappa": self.kappa,
            "cells": len(self._cells),
            "best": None if best is None else {
                "toffoli": best.toffoli,
                "qubits": best.qubits,
                "tq": best.tq,
            },
            "entries": [
                {"toffoli": e.toffoli, "qubits": e.qubits, "tq": e.tq, "meta": e.meta}
                for e in self.entries()
            ],
        }
