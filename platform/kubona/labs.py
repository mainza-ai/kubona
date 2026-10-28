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

from kubona.config import state_path
from kubona.jsonstore import atomic_write_json, exclusive
from kubona.session import tenant_slug
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
        self.path = Path(path) if path else state_path("labs.json")
        self.path.parent.mkdir(parents=True, exist_ok=True)
        self._labs: list[Lab] = []
        self._load()

    def _load(self) -> None:
        self._labs = ([Lab(**l) for l in json.loads(self.path.read_text())]
                      if self.path.exists() else [])

    def create(self, name: str, owner: str, members: list[str] | None = None) -> Lab:
        # W0.13: tenant derives through the single shared slug rule — the old
        # inline lower/replace let hostile names collapse onto shared
        # namespaces (cross-lab session/budget bleed). Invalid names refuse.
        slug = tenant_slug(name)
        with exclusive(self.path):
            self._load()
            if any(l.tenant == slug for l in self._labs):
                raise ValueError(f"tenant {slug!r} already exists: lab names must not collide")
            lab = Lab(lab_id=uuid.uuid4().hex[:8], name=name, owner=owner,
                      tenant=slug, members=members or [owner])
            self._labs.append(lab)
            self._save()
            return lab

    def all(self) -> list[dict[str, Any]]:
        self._load()
        return [l.to_dict() for l in self._labs]

    def _save(self) -> None:
        atomic_write_json(self.path, [l.to_dict() for l in self._labs])
