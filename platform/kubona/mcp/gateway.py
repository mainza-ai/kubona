"""MCP gateway.

Wires the federated servers behind one entry point with:
  - an AuthProvider (verify bearer -> principal)  [dev for local; real OAuth2
    issuer for production via kubona.run.build_auth]
  - a TransportProvider (swappable)               [in-process / stdio / HTTP]
  - per-call identity-aware authorization         [fine-grained scopes, live
    agent delegation; kubona.security.policy]
  - an audit note per call (in-memory + optional persistent JSONL export)
"""

from __future__ import annotations

import json
import logging
import os

from typing import Any, Callable

from .auth import AuthError, AuthProvider, DevAuthProvider, Principal
from .transport import InProcessTransport, TransportProvider
from kubona.security.policy import PolicyEngine
from .approval import (
    ApprovalPacket, ApprovalRegistry, elicitation_form, open_state, seal_state,
)

ToolFn = Callable[..., dict]
# PacketBuilder(principal, args) -> ApprovalPacket | None.
# Return None to auto-allow (e.g. simulator backends); raise to refuse.
ApprovalPacketBuilder = Callable[..., ApprovalPacket | None]

logger = logging.getLogger(__name__)


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
        self._approval: dict[str, ApprovalPacketBuilder] = {}
        self._approval_secret: bytes | None = None
        self._nonces = ApprovalRegistry()
        self._audit: list[dict] = []
        self._audit_path = audit_path  # optional persistent JSONL audit export
        # Loud-failure counter (W2.7): audit/ledger sinks must never fail
        # silently. Every swallowed sink error increments this and logs at
        # warning — the call path still never breaks, but ops can alert on it.
        self._audit_failures: int = 0

    @property
    def audit_failures(self) -> int:
        """Sink errors swallowed so far (audit JSONL + ledger appends)."""
        return self._audit_failures

    def _sink_failed(self, what: str, exc: Exception) -> None:
        self._audit_failures += 1
        logger.warning("kubona gateway %s failed (call path unaffected): %s",
                       what, exc)

    def mount(self, server_name: str, tools: dict[str, ToolFn]) -> None:
        """Mount-merge tools under `server_name` (W2.7).

        Mounting a new server name creates it; mounting into an existing
        server merges. A name collision with a DIFFERENT function refuses
        (fail-closed) instead of silently overwriting — re-mounting the
        identical function object is idempotent and allowed.
        """
        existing = self._registry.setdefault(server_name, {})
        for name, fn in tools.items():
            if name in existing and existing[name] is not fn:
                raise ValueError(
                    f"mount conflict: {server_name}/{name} is already mounted "
                    "(refusing silent overwrite — unmount or rename first)")
            existing[name] = fn
        if isinstance(self.transport, InProcessTransport):
            self.transport._registry = self._registry

    def _authorize(self, principal: Principal, server: str, tool: str) -> None:
        """Per-call identity-aware authorization (Phase 3): fine-grained scopes,
        agent delegation checked live (revocation immediate)."""
        decision = self.policy.decide(principal, f"{server}/{tool}")
        if not decision.allowed:
            raise AuthError(decision.reason)

    def require_approval(self, server: str, tool: str, builder: ApprovalPacketBuilder,
                         secret: bytes | None = None) -> None:
        """Gate a tool behind human approval (Q-11/MRTR). builder(principal,
        args) returns a packet, None to auto-allow, or raises to refuse.
        secret seals requestState; without it approval flows refuse to arm."""
        if secret is not None:
            self._approval_secret = secret
        self._approval[f"{server}/{tool}"] = builder

    def call(self, server: str, tool: str, args: dict[str, Any], token: str | None = None,
             approval_response: dict[str, Any] | None = None,
             request_state: str | None = None) -> dict:
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
        builder = self._approval.get(f"{server}/{tool}")
        if builder is not None:
            if self._approval_secret is None:
                raise AuthError("approval gate armed without a state secret: refusing to run")
            sub = principal.sub if principal else "anon"
            if approval_response is None:
                packet = builder(principal, dict(args))
                if packet is None:
                    pass  # auto-allowed below
                else:
                    return self._request_approval(sub, packet)
            else:
                return self._resolve_approval(sub, server, tool, args, tools[tool],
                                              approval_response, request_state)
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
            except Exception as e:  # audit must never break the call path (W2.7: loudly, not silently)
                self._sink_failed("audit export", e)
        if self._ledger is not None:  # immutable, content-addressed audit (doubles as cost ledger)
            try:
                self._ledger.append("audit", entry)
            except Exception as e:  # audit must never break the call path (W2.7: loudly, not silently)
                self._sink_failed("ledger append", e)
        return result

    def _request_approval(self, sub: str, packet: ApprovalPacket) -> dict:
        """First MRTR round: no execution, no side effects — only the ask."""
        from .approval import elicitation_form, seal_state
        form = elicitation_form(packet)
        state = seal_state(packet, sub, self._approval_secret)
        self._audit_event({"packet_id": packet.packet_id, "phase": "approval_requested",
                           "server": packet.server, "tool": packet.tool, "sub": sub,
                           "cost_max_usd": packet.cost_max_usd})
        return {"resultType": "input_required",
                "inputRequests": {"approval": form},
                "requestState": state}

    def _resolve_approval(self, sub: str, server: str, tool: str, args: dict[str, Any],
                          fn: ToolFn, response: dict[str, Any],
                          request_state: str | None) -> dict:
        """Second MRTR round: verify everything, then execute exactly once."""
        from .approval import open_state
        if not request_state:
            raise AuthError("approval retry without requestState: refused")
        try:
            state = open_state(request_state, self._approval_secret)
        except ValueError as e:
            raise AuthError(f"approval state invalid: {e}")
        if state["sub"] != sub:
            raise AuthError("approval principal mismatch: refused")
        if state["server"] != server or state["tool"] != tool:
            raise AuthError("approval target mismatch: refused")
        try:
            self._nonces.consume(state["nonce"])
        except ValueError as e:
            raise AuthError(f"approval replay refused: {e}")
        action = response.get("action", "cancel")
        if action != "accept":
            reason = str(response.get("reason", response.get("content", "")))[:280]
            self._audit_event({"packet_id": state["packet_id"], "phase": "approval_denied",
                               "server": server, "tool": tool, "sub": sub, "reason": reason})
            return {"resultType": "refused", "ok": False,
                    "rejection_message": reason or "human denied the action"}
        content = response.get("content", {}) or {}
        if content.get("decision") not in ("approve_once", "approve_up_to_max"):
            raise AuthError("approval decision not affirmative: refused")
        try:
            ack = float(content.get("cost_ack_usd"))
        except (TypeError, ValueError):
            raise AuthError("cost acknowledgement missing or invalid: refused")
        if abs(ack - float(state["cost_max_usd"])) > 1e-9:
            raise AuthError("cost acknowledgement mismatch: refused")
        self._audit_event({"packet_id": state["packet_id"], "phase": "approval_granted",
                           "server": server, "tool": tool, "sub": sub,
                           "decision": content["decision"]})
        result = fn(**args)
        self._audit.append({"server": server, "tool": tool, "args_keys": sorted(args),
                            "sub": sub, "via": "approved:" + state["packet_id"]})
        return result

    def _audit_event(self, entry: dict[str, Any]) -> None:
        self._audit.append(entry)
        if self._ledger is not None:
            try:
                self._ledger.append("approval", entry)
            except Exception as e:  # W2.7: loud, never silent
                self._sink_failed("ledger append", e)

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

    def mounted_tools(self) -> dict[str, dict[str, ToolFn]]:
        """Mounted surface snapshot (server → {tool: fn}), copies.

        Measurement (bench/surface.py) and the future MCP catalog (L5)
        build on this — one accessor, never registry internals elsewhere.
        """
        return {s: dict(ts) for s, ts in self._registry.items()}
