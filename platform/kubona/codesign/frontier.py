"""MAP-Elites (T,Q) frontier and multi-objective Pareto frontier (Phase 3).

Paper: divide the (T,Q) plane into cells, keep lowest TQ per cell so a
space-efficient circuit survives a worse TQ. Phase 3 adds ``ParetoFrontier``
over ``(tq, toffoli_depth, qubits)`` with explicit non-dominance so a circuit
that improves TQ while worsening T-depth is never silently discarded.
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
    toffoli_depth: int = 0


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

    def offer(
        self,
        artifact: Any,
        toffoli: int,
        qubits: int,
        meta: dict | None = None,
        *,
        toffoli_depth: int = 0,
    ) -> bool:
        tq = tq_cost(toffoli, qubits, self.kappa)
        c = self.cell(toffoli, qubits)
        cur = self._cells.get(c)
        if cur is None or tq < cur.tq:
            self._cells[c] = FrontierEntry(artifact, toffoli, qubits, tq, dict(meta or {}), int(toffoli_depth))
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
                "toffoli_depth": best.toffoli_depth,
            },
            "entries": [
                {
                    "toffoli": e.toffoli,
                    "qubits": e.qubits,
                    "tq": e.tq,
                    "toffoli_depth": e.toffoli_depth,
                    "meta": e.meta,
                }
                for e in self.entries()
            ],
        }


class ParetoFrontier:
    """Multi-objective non-dominated frontier over ``(tq, toffoli_depth, qubits)`` (Phase 3).

    Entry A dominates B iff ``A.tq <= B.tq``, ``A.toffoli_depth <= B.toffoli_depth``,
    ``A.qubits <= B.qubits``, and at least one inequality is strict.
    """

    def __init__(self, kappa: int = TQ_KAPPA_DEFAULT):
        self.kappa = kappa
        self._entries: list[FrontierEntry] = []

    @staticmethod
    def dominates(a: FrontierEntry, b: FrontierEntry) -> bool:
        le_all = (a.tq <= b.tq) and (a.toffoli_depth <= b.toffoli_depth) and (a.qubits <= b.qubits)
        lt_any = (a.tq < b.tq) or (a.toffoli_depth < b.toffoli_depth) or (a.qubits < b.qubits)
        return le_all and lt_any

    def offer(
        self,
        artifact: Any,
        toffoli: int,
        qubits: int,
        toffoli_depth: int,
        meta: dict | None = None,
    ) -> bool:
        cand = FrontierEntry(
            artifact=artifact,
            toffoli=int(toffoli),
            qubits=int(qubits),
            tq=tq_cost(int(toffoli), int(qubits), self.kappa),
            meta=dict(meta or {}),
            toffoli_depth=int(toffoli_depth),
        )
        for existing in self._entries:
            if self.dominates(existing, cand):
                return False
            if (
                existing.tq == cand.tq
                and existing.toffoli_depth == cand.toffoli_depth
                and existing.qubits == cand.qubits
            ):
                return False
        self._entries = [e for e in self._entries if not self.dominates(cand, e)]
        self._entries.append(cand)
        return True

    def entries(self) -> list[FrontierEntry]:
        return sorted(self._entries, key=lambda e: (e.tq, e.toffoli_depth, e.qubits))

    def to_dict(self) -> dict[str, Any]:
        return {
            "kappa": self.kappa,
            "objectives": ["tq", "toffoli_depth", "qubits"],
            "size": len(self._entries),
            "entries": [
                {
                    "toffoli": e.toffoli,
                    "qubits": e.qubits,
                    "tq": e.tq,
                    "toffoli_depth": e.toffoli_depth,
                    "meta": e.meta,
                }
                for e in self.entries()
            ],
        }
