"""Laboratories (Phase 6 / §14.2): the multi-user workspace primitive.

A lab names an owner + members and scopes a tenant (the Phase-5 tenant key that
shards sessions/ledgers). Membership/identity live in the identity registry; this
is the grouping surface the console's Labs view renders.
"""

from __future__ import annotations

import json
import time
import uuid
from dataclasses import asdict, dataclass, field
from pathlib import Path
from typing import Any


@dataclass
class Lab:
    lab_id: str
    name: str
    owner: str
    tenant: str
    members: list[str] = field(default_factory=list)
    created_at: float = field(default_factory=time.time)

    def to_dict(self) -> dict[str, Any]:
        return asdict(self)


class LabRegistry:
    def __init__(self, path: str | Path | None = None):
        self.path = Path(path) if path else Path("/tmp/kubona-state/labs.json")
        self.path.parent.mkdir(parents=True, exist_ok=True)
        self._labs: list[Lab] = []
        if self.path.exists():
            self._labs = [Lab(**l) for l in json.loads(self.path.read_text())]

    def create(self, name: str, owner: str, members: list[str] | None = None) -> Lab:
        lab = Lab(lab_id=uuid.uuid4().hex[:8], name=name, owner=owner,
                  tenant=name.lower().replace(" ", "-"),
                  members=members or [owner])
        self._labs.append(lab)
        self._save()
        return lab

    def all(self) -> list[dict[str, Any]]:
        return [l.to_dict() for l in self._labs]

    def _save(self) -> None:
        self.path.write_text(json.dumps([l.to_dict() for l in self._labs], indent=2))
