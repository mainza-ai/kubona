"""Provenance-backed algorithm library (§11.4).

Entries carry: harness, artifact, metric, cost, ledger reference, and validation
status. Doubles as the monetizable resource (agent-resource-economy) and the
training data for the §13.3 GQE flywheel.
"""

from __future__ import annotations

import json
import time
import uuid
from dataclasses import asdict, dataclass, field
from pathlib import Path
from typing import Any

from kubona.config import state_path


@dataclass
class LibraryEntry:
    entry_id: str
    harness: str
    artifact: Any
    metric: float
    cost_usd: float
    ledger_ref: str
    validated: bool = True
    ts: float = field(default_factory=time.time)
    meta: dict[str, Any] = field(default_factory=dict)


class AlgorithmLibrary:
    def __init__(self, path: str | Path | None = None):
        self.path = Path(path) if path else state_path("library.json")
        self.path.parent.mkdir(parents=True, exist_ok=True)
        self._entries: list[LibraryEntry] = []
        if self.path.exists():
            self._entries = [LibraryEntry(**e) for e in json.loads(self.path.read_text())]

    def add(self, harness: str, artifact: Any, metric: float, cost_usd: float,
            ledger_ref: str, validated: bool = True, **meta: Any) -> LibraryEntry:
        entry = LibraryEntry(entry_id=uuid.uuid4().hex[:16], harness=harness,
                             artifact=artifact, metric=metric, cost_usd=cost_usd,
                             ledger_ref=ledger_ref, validated=validated, meta=meta)
        self._entries.append(entry)
        self._save()
        return entry

    def best(self, harness: str | None = None) -> LibraryEntry | None:
        pool = [e for e in self._entries if (harness is None or e.harness == harness) and e.validated]
        return max(pool, key=lambda e: e.metric) if pool else None

    def all(self) -> list[LibraryEntry]:
        return list(self._entries)

    def _save(self) -> None:
        self.path.write_text(json.dumps([asdict(e) for e in self._entries], indent=2))

    def __len__(self) -> int:
        return len(self._entries)
