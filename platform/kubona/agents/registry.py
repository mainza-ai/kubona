"""Agent personas + registry with no-escalation creation."""

from __future__ import annotations

import fcntl
import json
import math
import time
import uuid
from contextlib import contextmanager
from dataclasses import asdict, dataclass, field
from pathlib import Path
from typing import Any, Iterator

from kubona.config import state_path
from kubona.identity import IdentityRegistry
import threading


class AgentCreationError(RuntimeError):
    pass


@dataclass
class Agent:
    agent_id: str
    persona: str                       # a role label ("ansatz-specialist-reviewer")
    owner: str | None = None
    scopes: tuple[str, ...] = ()
    budget_ceiling_usd: float | None = None
    workflow_ids: list[str] = field(default_factory=list)
    status: str = "active"             # active | disabled
    created_at: float = field(default_factory=time.time)
    created_by: str | None = None      # set when an agent created this agent

    def to_dict(self) -> dict[str, Any]:
        return {**asdict(self), "scopes": list(self.scopes)}


class AgentRegistry:
    def __init__(self, path: str | Path | None = None, identities: IdentityRegistry | None = None):
        self.path = Path(path) if path else state_path("agents.json")
        self.path.parent.mkdir(parents=True, exist_ok=True)
        self.identities = identities or IdentityRegistry()
        self._thread_lock = threading.Lock()
        self._lock_path = self.path.parent / (self.path.name + ".lock")
        self._agents: dict[str, Agent] = {}
        self._load()

    @contextmanager
    def _exclusive(self) -> Iterator[None]:
        with self._thread_lock:
            with open(self._lock_path, "a+b") as lf:
                fcntl.flock(lf.fileno(), fcntl.LOCK_EX)
                try:
                    yield
                finally:
                    fcntl.flock(lf.fileno(), fcntl.LOCK_UN)

    def _load(self) -> None:
        if not self.path.exists():
            return
        try:
            raw = json.loads(self.path.read_text())
        except (ValueError, OSError):
            return  # corrupt store reads empty, never crashes boot
        if not isinstance(raw, list):
            return
        for a in raw:
            try:
                d = dict(a)
                d["scopes"] = tuple(d.get("scopes", ()))
                self._agents[d["agent_id"]] = Agent(**{k: v for k, v in d.items()
                                                       if k in Agent.__dataclass_fields__})
            except (TypeError, KeyError):
                continue  # drifted row skipped, never fatal

    def _check_new(self, agent_id: str, persona: str, scopes: tuple[str, ...],
                   budget_ceiling_usd: float | None) -> tuple[str, ...]:
        if not agent_id or not agent_id.strip():
            raise AgentCreationError("agent_id is required")
        if agent_id in self._agents:
            raise AgentCreationError(f"agent {agent_id} already exists")
        if not persona or not persona.strip():
            raise AgentCreationError("persona is required")
        if not scopes:
            raise AgentCreationError("scopes are required: default-deny, never open")
        clean = []
        for s in scopes:
            if not isinstance(s, str) or not s.strip():
                raise AgentCreationError(f"scope {s!r} is not a valid scope string")
            clean.append(s)
        if budget_ceiling_usd is not None and (
            not isinstance(budget_ceiling_usd, (int, float))
            or not math.isfinite(budget_ceiling_usd) or budget_ceiling_usd <= 0
        ):
            raise AgentCreationError(f"budget_ceiling_usd must be positive finite or None, got {budget_ceiling_usd!r}")
        return tuple(clean)

    def create_agent(self, creator_id: str, creator_scopes: tuple[str, ...],
                     agent_id: str, persona: str, scopes: tuple[str, ...],
                     budget_ceiling_usd: float | None = None) -> Agent:
        """Single creation path (W0.15): every agent is created BY a creator
        whose scopes are stated explicitly, and the new scopes must be a
        SUBSET — no privilege escalation through entrypoint choice (the old
        proof-less create()/register() is gone). The creator is an existing
        agent id (scopes AND ceiling read from its record — caller-stated
        creator state refuses) or an "operator:<sub>" identity (scopes stated
        by the authenticated caller; the gateway layer derives them from the
        principal in W0.16). A child ceiling may not exceed the creator's
        recorded ceiling when both are set."""
        clean = self._check_new(agent_id, persona, scopes, budget_ceiling_usd)
        if creator_id.startswith("operator:"):
            if not creator_scopes:
                raise AgentCreationError("operator bootstrap requires explicit creator_scopes")
            allowed = set(creator_scopes)
        else:
            creator = self._agents.get(creator_id)
            if creator is None:
                raise AgentCreationError(f"creator agent {creator_id} unknown")
            if creator_scopes:
                raise AgentCreationError(
                    "creator scopes come from the creator's record, never the caller: refusing")
            allowed = set(creator.scopes)
            if creator.budget_ceiling_usd is not None and budget_ceiling_usd is not None \
                    and budget_ceiling_usd > creator.budget_ceiling_usd:
                raise AgentCreationError(
                    f"child ceiling {budget_ceiling_usd} exceeds creator ceiling "
                    f"{creator.budget_ceiling_usd}: refusing")
        if not set(clean) <= allowed:
            raise AgentCreationError(
                f"no escalation: {sorted(clean)} not a subset of creator's {sorted(allowed)}")
        with self._exclusive():
            if agent_id in self._agents:
                raise AgentCreationError(f"agent {agent_id} already exists")
            a = Agent(agent_id=agent_id, persona=persona, scopes=clean,
                      budget_ceiling_usd=budget_ceiling_usd, created_by=creator_id)
            self._agents[a.agent_id] = a
            self._save()
            return a

    def usable(self, agent_id: str) -> Agent:
        """Return the agent iff it exists and is not disabled. Administrative
        listing status is advisory; spend/execution gates live in the gateway
        policy (scopes), consulted per call — this states the boundary so no
        caller mistakes a label for a lock."""
        a = self._agents.get(agent_id)
        if a is None:
            raise AgentCreationError(f"agent {agent_id} unknown")
        if a.status == "disabled":
            raise AgentCreationError(f"agent {agent_id} is disabled")
        return a

    def get(self, agent_id: str) -> Agent | None:
        return self._agents.get(agent_id)

    def disable(self, agent_id: str) -> Agent:
        with self._exclusive():
            a = self._agents.get(agent_id)
            if a is None:
                raise AgentCreationError(f"agent {agent_id} unknown: refusing to disable ghosts")
            a.status = "disabled"
            self._save()
            return a

    def link_workflow(self, agent_id: str, workflow_id: str) -> Agent:
        if not workflow_id or not workflow_id.strip():
            raise AgentCreationError("workflow_id is required")
        with self._exclusive():
            a = self._agents.get(agent_id)
            if a is None:
                raise AgentCreationError(f"agent {agent_id} unknown: refusing to link ghosts")
            a.workflow_ids.append(workflow_id)
            self._save()
            return a

    def all(self) -> list[Agent]:
        return list(self._agents.values())

    def _save(self) -> None:
        tmp = self.path.with_suffix(".json.tmp")
        tmp.write_text(json.dumps([a.to_dict() for a in self._agents.values()], indent=2))
        tmp.replace(self.path)
