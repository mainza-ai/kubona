"""Workflow library — the compounding asset: saved, reusable, creator-identified.

Identity discipline (W2.6): a saved workflow keeps its `workflow_id` across
`to_workflow` (no forked uuid) and across re-saves — re-saving a workflow_id
that already exists bumps `version` in place instead of forking a twin.
Every store writes atomically (tmp-file + fsync + rename) so a crash never
leaves a half-written library.
"""

from __future__ import annotations

import json
import time
from dataclasses import asdict, dataclass, field
from pathlib import Path

from kubona.config import atomic_write_text, state_path
from typing import Any

from .engine import Workflow


@dataclass
class WorkflowDef:
    workflow_id: str
    source: dict[str, Any]
    created_by: str
    created_at: float = field(default_factory=time.time)
    version: int = 1
    # Fork pointer: when a workflow is deliberately duplicated under a NEW id
    # (a true fork, not a re-save), the child records the parent id here so
    # the lineage stays reviewable. Same-id re-saves bump `version` instead.
    supersedes: str | None = None

    def to_workflow(self) -> Workflow:
        """Rehydrate WITHOUT forking identity (W2.6): the workflow keeps the
        library's workflow_id and version."""
        wf = Workflow.from_dict({**self.source, "version": self.version,
                                 "workflow_id": self.workflow_id})
        wf.workflow_id = self.workflow_id
        return wf


def _atomic_write_json(path: Path, payload: Any) -> None:
    """Tmp-file + fsync + rename (W2.6): readers never see a half-write.
    Single shared implementation lives in kubona.config.atomic_write_text."""
    atomic_write_text(path, json.dumps(payload, indent=2))


class WorkflowLibrary:
    def __init__(self, path: str | Path | None = None):
        self.path = Path(path) if path else state_path("workflows.json")
        self.path.parent.mkdir(parents=True, exist_ok=True)
        self._items: list[WorkflowDef] = []
        if self.path.exists():
            raw = self.path.read_text()
            if raw.strip():
                for i in json.loads(raw):
                    # Back-compat: libraries written before `supersedes`
                    # existed carry no such key.
                    i.setdefault("supersedes", None)
                    self._items.append(WorkflowDef(**i))

    def save(self, source: dict[str, Any], created_by: str) -> WorkflowDef:
        """Save a workflow. New workflow_id -> version 1 record; an existing
        workflow_id -> version bump in place (W2.6, no forked twins)."""
        wf = Workflow.from_dict(source)
        if wf.created_by_agent:
            wf.approved = False  # agent-created requires human approval (§13.4)
        wid = source.get("workflow_id") or wf.workflow_id
        existing = self.get(wid)
        if existing is not None:
            existing.source = {**source, "workflow_id": wid}
            existing.version += 1
            existing.created_by = created_by
            self._save()
            return existing
        item = WorkflowDef(workflow_id=wid, source={**source, "workflow_id": wid},
                           created_by=created_by,
                           version=int(source.get("version", 1) or 1))
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
        _atomic_write_json(self.path, [asdict(i) for i in self._items])
