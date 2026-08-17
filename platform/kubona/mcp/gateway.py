"""MCP gateway skeleton (Phase 0).

Wires the four federated servers behind one entry point with:
  - an AuthProvider (verify bearer -> principal)       [OAuth 2.1 stub / dev]
  - a TransportProvider (swappable)                    [in-process / stdio]
  - per-call identity-aware authorization placeholder  [Phase 3 hardens]
  - an audit note per call                             [doubles as ledger input]
"""

from __future__ import annotations

import json
import os

from typing import Any, Callable

from .auth import AuthError, AuthProvider, DevAuthProvider, Principal
from .transport import InProcessTransport, TransportProvider
from kubona.security.policy import PolicyEngine

ToolFn = Callable[..., dict]


class KubonaGateway:
    def __init__(
        self,
        auth: AuthProvider | None = None,
        transport: TransportProvider | None = None,
        require_auth: bool = True,
        policy: PolicyEngine | None = None,
        ledger=None,
        audit_path=None,
    ):
        self.auth = auth or DevAuthProvider()
        self.transport = transport or InProcessTransport({})
        self.require_auth = require_auth
        self.policy = policy or PolicyEngine()
        self._ledger = ledger  # optional immutable audit sink (hash-chained)
        self._registry: dict[str, dict[str, ToolFn]] = {}
        self._audit: list[dict] = []
        self._audit_path = audit_path  # optional persistent JSONL audit export

    def mount(self, server_name: str, tools: dict[str, ToolFn]) -> None:
        self._registry[server_name] = tools
        if isinstance(self.transport, InProcessTransport):
            self.transport._registry = self._registry

    def _authorize(self, principal: Principal, server: str, tool: str) -> None:
        """Per-call identity-aware authorization (Phase 3): fine-grained scopes,
        agent delegation checked live (revocation immediate)."""
        decision = self.policy.decide(principal, f"{server}/{tool}")
        if not decision.allowed:
            raise AuthError(decision.reason)

    def call(self, server: str, tool: str, args: dict[str, Any], token: str | None = None) -> dict:
        principal: Principal | None = None
        if self.require_auth:
            if token is None:
                raise AuthError("missing bearer token")
            principal = self.auth.verify(token)
        tools = self._registry.get(server)
        if tools is None or tool not in tools:
            raise KeyError(f"unknown server/tool: {server}/{tool}")
        if self.require_auth:  # authorization is meaningless when auth is off (local/test)
            self._authorize(principal, server, tool)
        result = tools[tool](**args)
        entry = {
            "server": server, "tool": tool, "args_keys": sorted(args),
            "sub": principal.sub if principal else "anon",
        }
        self._audit.append(entry)
        if self._audit_path is not None:  # persistent audit export (JSONL, append-only)
            try:
                with open(self._audit_path, "a") as f:
                    f.write(json.dumps(entry, sort_keys=True) + "\n")
            except Exception:  # audit must never break the call path
                pass
        if self._ledger is not None:  # immutable, content-addressed audit (doubles as cost ledger)
            try:
                self._ledger.append("audit", entry)
            except Exception:  # audit must never break the call path
                pass
        return result

    def audit_log(self) -> list[dict]:
        return list(self._audit)

    def audit_export(self) -> list[dict]:
        """All audit entries, including any persisted across restarts."""
        if self._audit_path is not None and hasattr(self, "_loaded_audit"):
            return self._loaded_audit + self._audit
        saved = []
        if self._audit_path is not None and os.path.exists(self._audit_path):
            for line in open(self._audit_path).read().splitlines():
                if line.strip():
                    saved.append(json.loads(line))
        return saved + self._audit

    def servers(self) -> list[str]:
        return sorted(self._registry)
