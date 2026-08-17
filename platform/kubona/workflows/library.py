"""Workflow library — the compounding asset: saved, reusable, creator-identified."""

from __future__ import annotations

import json
import time
import uuid
from dataclasses import asdict, dataclass, field
from pathlib import Path

from kubona.config import state_path
from typing import Any

from .engine import Workflow, WorkflowError


@dataclass
class WorkflowDef:
    workflow_id: str
    source: dict[str, Any]
    created_by: str
    created_at: float = field(default_factory=time.time)
    version: int = 1

    def to_workflow(self) -> Workflow:
        return Workflow.from_dict({**self.source, "version": self.version})


class WorkflowLibrary:
    def __init__(self, path: str | Path | None = None):
        self.path = Path(path) if path else state_path("workflows.json")
        self.path.parent.mkdir(parents=True, exist_ok=True)
        self._items: list[WorkflowDef] = []
        if self.path.exists():
            self._items = [WorkflowDef(**i) for i in json.loads(self.path.read_text())]

    def save(self, source: dict[str, Any], created_by: str) -> WorkflowDef:
        wf = Workflow.from_dict(source)
        if wf.created_by_agent:
            wf.approved = False  # agent-created requires human approval (§13.4)
        item = WorkflowDef(workflow_id=wf.workflow_id, source=source, created_by=created_by)
        self._items.append(item)
        self._save()
        return item

    def get(self, workflow_id: str) -> WorkflowDef | None:
        for i in self._items:
            if i.workflow_id == workflow_id:
                return i
        return None

    def all(self) -> list[WorkflowDef]:
        return list(self._items)

    def _save(self) -> None:
        self.path.write_text(json.dumps([asdict(i) for i in self._items], indent=2))
