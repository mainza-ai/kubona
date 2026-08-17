"""Agent personas + registry with no-escalation creation."""

from __future__ import annotations

import json
import time
import uuid
from dataclasses import asdict, dataclass, field
from pathlib import Path
from typing import Any

from kubona.identity import IdentityRegistry


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
        self.path = Path(path) if path else Path("/tmp/kubona-state/agents.json")
        self.path.parent.mkdir(parents=True, exist_ok=True)
        self.identities = identities or IdentityRegistry()
        self._agents: dict[str, Agent] = {}
        if self.path.exists():
            for a in json.loads(self.path.read_text()):
                d = dict(a); d["scopes"] = tuple(d["scopes"])
                self._agents[d["agent_id"]] = Agent(**d)

    def register(self, agent_id: str, persona: str, scopes: tuple[str, ...],
                 budget_ceiling_usd: float | None = None) -> Agent:
        if agent_id in self._agents:
            raise AgentCreationError(f"agent {agent_id} already exists")
        a = Agent(agent_id=agent_id, persona=persona, scopes=scopes,
                  budget_ceiling_usd=budget_ceiling_usd)
        self._agents[a.agent_id] = a
        self._save()
        return a

    def create(self, agent_id: str, persona: str, scopes: tuple[str, ...],
               budget_ceiling_usd: float | None = None) -> Agent:
        return self.register(agent_id, persona, scopes, budget_ceiling_usd)

    def create_as_agent(self, creator_id: str, agent_id: str, persona: str,
                        scopes: tuple[str, ...], budget_ceiling_usd: float | None = None) -> Agent:
        """No privilege escalation: a created agent's scopes are a SUBSET of the
        creator's effective scopes (an agent cannot grant powers it does not hold)."""
        creator = self._agents.get(creator_id)
        if creator is None:
            raise AgentCreationError(f"creator agent {creator_id} unknown")
        creator_scopes = set(creator.scopes)
        if not set(scopes) <= creator_scopes:
            raise AgentCreationError(
                f"no escalation: {scopes} not a subset of creator's {sorted(creator_scopes)}")
        a = Agent(agent_id=agent_id, persona=persona, scopes=scopes,
                  budget_ceiling_usd=budget_ceiling_usd, created_by=creator_id)
        self._agents[a.agent_id] = a
        self._save()
        return a

    def get(self, agent_id: str) -> Agent | None:
        return self._agents.get(agent_id)

    def disable(self, agent_id: str) -> Agent:
        a = self._agents[agent_id]
        a.status = "disabled"
        self._save()
        return a

    def link_workflow(self, agent_id: str, workflow_id: str) -> Agent:
        a = self._agents[agent_id]
        a.workflow_ids.append(workflow_id)
        self._save()
        return a

    def all(self) -> list[Agent]:
        return list(self._agents.values())

    def _save(self) -> None:
        self.path.write_text(json.dumps([a.to_dict() for a in self._agents.values()], indent=2))
