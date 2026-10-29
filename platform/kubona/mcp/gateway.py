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

import hashlib
import json
import logging
import os

from typing import Any, Callable

from .auth import AuthError, AuthProvider, DevAuthProvider, Principal
from .transport import InProcessTransport, TransportProvider
from kubona.security.policy import PolicyEngine
from kubona.security.tenant import caller_from_principal
from .approval import (
    ApprovalPacket, ApprovalRegistry, elicitation_form, open_state, seal_state,
)

ToolFn = Callable[..., dict]
# PacketBuilder(principal, args) -> ApprovalPacket | None.
# Return None to auto-allow (e.g. simulator backends); raise to refuse.
ApprovalPacketBuilder = Callable[..., ApprovalPacket | None]

logger = logging.getLogger(__name__)


def _accepts_caller(fn: ToolFn) -> bool:
    """Whether a tool declares the server-side `caller` identity parameter."""
    import inspect
    try:
        return "caller" in inspect.signature(fn).parameters
    except (TypeError, ValueError):
        return False


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
        # Tools that declare a `caller` parameter receive the server-side
        # caller identity (sub + tenant claim), injected per call below.
        # Precomputed at mount; unknown mounts fall back to inspection.
        # Tools prefer it over caller-supplied tenant/requester strings and
        # refuse mismatches unless a live cross-tenant grant covers the
        # request (kubona.security.tenant.resolve_tenant consults the grant
        # registry).
        self._caller_tools: set[str] = set()
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
            if _accepts_caller(fn):
                self._caller_tools.add(f"{server_name}/{name}")
        if isinstance(self.transport, InProcessTransport):
            self.transport._registry = self._registry

    def _caller_for(self, principal: Principal | None) -> dict[str, str] | None:
        """Server-side caller identity for tool injection (Phase-1).

        Derived from the VERIFIED principal only — never from client args.
        None when auth is off (local/test): tools fall back to their
        transport-trusted direct-call behavior.
        """
        if principal is None:
            return None
        return caller_from_principal(principal)

    def _inject_caller(self, server: str, tool: str, args: dict[str, Any],
                       caller: dict[str, str] | None) -> dict[str, Any]:
        """Bind the server-side caller identity into tool args (overwrite).

        A client-supplied `caller` is attacker-controlled and is always
        replaced; tools that declare `caller` then prefer it over any
        caller-supplied requester/tenant string and refuse mismatches unless
        a live cross-tenant grant covers the request (see
        kubona.security.tenant).
        """
        if caller is None:
            return args
        key = f"{server}/{tool}"
        if key in self._caller_tools:
            args = dict(args)
            args["caller"] = caller
        else:
            try:
                fn = self._registry[server][tool]
            except KeyError:
                return args
            if _accepts_caller(fn):
                self._caller_tools.add(key)
                args = dict(args)
                args["caller"] = caller
        return args

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
        # Phase-2: bind the verified principal's identity server-side.
        # Injected (overwriting any client-supplied `caller`); tools prefer
        # it over caller-supplied requester/tenant strings and refuse
        # mismatches unless a live cross-tenant grant covers the request.
        # principal_tenant fail-closed: a malformed claim refuses here,
        # before any tool runs.
        caller = self._caller_for(principal)
        args = self._inject_caller(server, tool, dict(args), caller)
        builder = self._approval.get(f"{server}/{tool}")
        if builder is not None:
            sub = principal.sub if principal else "anon"
            if approval_response is None:
                packet = builder(principal, dict(args))
                if packet is None:
                    pass  # auto-allowed below (free-tier: no secret needed)
                else:
                    # Paid path: sealing demands the secret. Free-tier
                    # auto-allows never reach here, so unconfigured
                    # deployments keep working for simulators while paid
                    # dispatches refuse loudly instead of running unsealed.
                    if self._approval_secret is None:
                        raise AuthError("approval gate armed without a state secret: refusing to run")
                    if not packet.args:
                        packet.args = dict(args)
                    return self._request_approval(sub, packet)
            else:
                return self._resolve_approval(sub, server, tool, args, tools[tool],
                                              approval_response, request_state)
        result = tools[tool](**args)
        entry = {
            "server": server, "tool": tool, "args_keys": sorted(args),
            "sub": principal.sub if principal else "anon",
            "tenant": caller["tenant"] if caller is not None else "ambient",
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
        if state.get("args_digest"):
            current_digest = hashlib.sha256(json.dumps(args, sort_keys=True).encode()).hexdigest()
            if current_digest != state["args_digest"]:
                raise AuthError("approval args mismatch: arguments altered between request and resolution: refused")
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
        # Phase-0: the in-function paid-dispatch gate honors this verified
        # round via ambient context (scope + cost cover re-checked there);
        # direct calls never carry it and must present their own approval.
        from .approval import gateway_approved_call
        record = {"server": server, "tool": tool, "sub": sub,
                  "packet_id": state["packet_id"],
                  "cost_max_usd": float(state.get("cost_max_usd", 0.0)),
                  "nonce": state.get("nonce", "")}
        with gateway_approved_call(record):
            result = fn(**args)
        self._audit_event({"server": server, "tool": tool, "args_keys": sorted(args),
                           "sub": sub, "via": "approved:" + state["packet_id"],
                           "packet_id": state["packet_id"], "phase": "execution_completed"})
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
