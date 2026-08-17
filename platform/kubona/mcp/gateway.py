"""MCP gateway skeleton (Phase 0).

Wires the four federated servers behind one entry point with:
  - an AuthProvider (verify bearer -> principal)       [OAuth 2.1 stub / dev]
  - a TransportProvider (swappable)                    [in-process / stdio]
  - per-call identity-aware authorization placeholder  [Phase 3 hardens]
  - an audit note per call                             [doubles as ledger input]
"""

from __future__ import annotations

from typing import Any, Callable

from .auth import AuthError, AuthProvider, DevAuthProvider, Principal
from .transport import InProcessTransport, TransportProvider

ToolFn = Callable[..., dict]


class KubonaGateway:
    def __init__(
        self,
        auth: AuthProvider | None = None,
        transport: TransportProvider | None = None,
        require_auth: bool = True,
    ):
        self.auth = auth or DevAuthProvider()
        self.transport = transport or InProcessTransport({})
        self.require_auth = require_auth
        self._registry: dict[str, dict[str, ToolFn]] = {}
        self._audit: list[dict] = []

    def mount(self, server_name: str, tools: dict[str, ToolFn]) -> None:
        self._registry[server_name] = tools
        if isinstance(self.transport, InProcessTransport):
            self.transport._registry = self._registry

    # -- per-call authorization (placeholder; Phase 3 replaces with identity-aware RBAC)
    def _authorize(self, principal: Principal, server: str, tool: str) -> None:
        if principal.scopes != ("*",) and tool not in principal.scopes:
            raise AuthError(f"principal {principal.sub} not authorized for {server}/{tool}")

    def call(self, server: str, tool: str, args: dict[str, Any], token: str | None = None) -> dict:
        principal: Principal | None = None
        if self.require_auth:
            if token is None:
                raise AuthError("missing bearer token")
            principal = self.auth.verify(token)
        tools = self._registry.get(server)
        if tools is None or tool not in tools:
            raise KeyError(f"unknown server/tool: {server}/{tool}")
        self._authorize(principal, server, tool)
        result = tools[tool](**args)
        self._audit.append({
            "server": server, "tool": tool, "args_keys": sorted(args), "sub": principal.sub if principal else "anon",
        })
        return result

    def audit_log(self) -> list[dict]:
        return list(self._audit)

    def servers(self) -> list[str]:
        return sorted(self._registry)
