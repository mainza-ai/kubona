"""Transport provider interface — swappable from day one (cryptographic/transport agility).

Phase 0 ships InProcessTransport for tests and the MCP stdio transport for real
servers (each server runs via `mcp.run(transport="stdio")`). Streamable HTTP +
OAuth 2.1 bearer arrives with the gateway hardening phase.
"""

from __future__ import annotations

from abc import ABC, abstractmethod
from typing import Any


class TransportProvider(ABC):
    @abstractmethod
    def open(self, server_name: str) -> Any:
        """Open a transport handle for a named server."""

    @abstractmethod
    def close(self) -> None:
        ...


class InProcessTransport(TransportProvider):
    """Direct function dispatch — used by the P0 gate test to drive the chain."""

    def __init__(self, registry: dict[str, dict[str, Any]]):
        self._registry = registry

    def open(self, server_name: str) -> Any:
        return self._registry[server_name]

    def close(self) -> None:
        pass
