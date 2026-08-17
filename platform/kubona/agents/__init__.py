"""Agent registry & personas (Phase 6 / §14.3): agent = persona + workflow + scope + budget.

Agent-created agents are gated: a created agent can never hold capabilities its
creator does not hold (no privilege escalation through creation).
"""

from .registry import Agent, AgentRegistry, AgentCreationError

__all__ = ["Agent", "AgentRegistry", "AgentCreationError"]
