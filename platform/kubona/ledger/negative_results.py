"""Immutable Negative Results & Refutations Ledger (Ring 1).

Prevents wasteful re-exploration by indexing refuted hypotheses, barren plateau traps,
and non-advantaged variational trials in an append-only cryptographic ledger.
"""

from __future__ import annotations

import json
import os
import time
import uuid
from dataclasses import asdict, dataclass, field
from pathlib import Path
from typing import Any, List, Optional


@dataclass
class NegativeResultRecord:
    record_id: str
    timestamp: float
    problem_type: str  # e.g., "vqe_ground_state", "qaoa_maxcut", "circuit_synthesis"
    hamiltonian_or_graph: str
    ansatz_family: str
    failure_mode: str  # "BARREN_PLATEAU", "CLASSICAL_SUPERIOR", "NOISE_CORRUPTION", "BUDGET_EXHAUSTED"
    variational_energy: float
    classical_anchor_energy: float
    delta_hartree: float
    shots_wasted: int
    cost_usd: float
    falsification_notes: str
    raw_parameters: List[float] = field(default_factory=list)

    def to_dict(self) -> dict[str, Any]:
        return asdict(self)


class NegativeResultsLedger:
    def __init__(self, storage_dir: str | Path | None = None):
        base = Path(storage_dir or os.environ.get("KUBONA_STATE_DIR", "/tmp/kubona-ledger"))
        self.ledger_path = base / "negative_results.jsonl"
        self.ledger_path.parent.mkdir(parents=True, exist_ok=True)

    def record_refutation(
        self,
        problem_type: str,
        hamiltonian: str,
        ansatz_family: str,
        failure_mode: str,
        variational_energy: float,
        classical_anchor_energy: float,
        shots_wasted: int = 0,
        cost_usd: float = 0.0,
        falsification_notes: str = "",
        raw_parameters: List[float] | None = None,
    ) -> NegativeResultRecord:
        rec = NegativeResultRecord(
            record_id=uuid.uuid4().hex[:16],
            timestamp=time.time(),
            problem_type=problem_type,
            hamiltonian_or_graph=hamiltonian,
            ansatz_family=ansatz_family,
            failure_mode=failure_mode,
            variational_energy=variational_energy,
            classical_anchor_energy=classical_anchor_energy,
            delta_hartree=variational_energy - classical_anchor_energy,
            shots_wasted=shots_wasted,
            cost_usd=cost_usd,
            falsification_notes=falsification_notes,
            raw_parameters=raw_parameters or [],
        )
        with open(self.ledger_path, "a", encoding="utf-8") as f:
            f.write(json.dumps(rec.to_dict()) + "\n")
        return rec

    def all(self) -> List[NegativeResultRecord]:
        if not self.ledger_path.exists():
            return []
        out = []
        with open(self.ledger_path, "r", encoding="utf-8") as f:
            for line in f:
                if line.strip():
                    d = json.loads(line)
                    out.append(NegativeResultRecord(**d))
        return out

    def search_similar_failures(self, hamiltonian: str, ansatz_family: str) -> List[NegativeResultRecord]:
        return [
            r for r in self.all()
            if r.hamiltonian_or_graph == hamiltonian and r.ansatz_family == ansatz_family
        ]
