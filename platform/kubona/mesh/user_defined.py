"""User-Defined & Standard Agent Registry mesh module re-export.

Provides alias/shim access to `kubona.agents.user_defined` for the mesh subsystem.
"""

from __future__ import annotations

from kubona.agents.user_defined import (
    STANDARD_SYSTEM_AGENTS,
    AgentRegistry,
    UserDefinedAgentManifest,
)

__all__ = [
    "UserDefinedAgentManifest",
    "AgentRegistry",
    "STANDARD_SYSTEM_AGENTS",
]
