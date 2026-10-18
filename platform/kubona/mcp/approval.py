"""MRTR approval retry for high-friction tool calls (Q-11).

When a tool is approval-gated, the first `call()` does NOT execute. It
returns an `input_required` envelope carrying an `elicitation/create`
form-mode request (MCP 2026-07-28) plus an opaque `requestState`. The client
(human console card or agent relay) gathers the decision and retries the same
method with `inputResponses` + echoed state; the gateway verifies (HMAC, TTL,
principal match, single-use nonce, cost acknowledgement) and only then
executes. Decline/cancel never executes.

The full approval packet lives wherever the human reads it (console card);
only the decision transits MCP — form mode MUST NOT carry secrets, URLs, or
payment credentials (URL mode would, but approvals need no secrets).
"""

from __future__ import annotations

import hashlib
import hmac
import json
import secrets
import time
from dataclasses import asdict, dataclass, field
from typing import Any, Callable


APPROVAL_METHOD = "elicitation/create"
DEFAULT_APPROVAL_TTL_S = 900  # 15 minutes: approvals rot fast
MAX_PENDING_NONCES = 10000


@dataclass
class ApprovalPacket:
    """What a competent approver needs: reconstructible without five tabs."""

    packet_id: str
    agent_id: str
    server: str
    tool: str
    backend: str
    program_hash: str
    qubits: int = 0
    shots: int = 0
    cost_max_usd: float = 0.0
    budget_remaining_usd: float = 0.0
    cost_basis: str = ""
    blast_radius: str = ""
    reversibility: str = ""
    evidence: dict[str, Any] = field(default_factory=dict)
    alternatives: dict[str, Any] = field(default_factory=dict)
    expires_at: float = 0.0
    nonce: str = ""
    requestor: str = ""
    args: dict[str, Any] = field(default_factory=dict)

    def to_dict(self) -> dict[str, Any]:
        return asdict(self)


def build_packet(*, agent_id: str, server: str, tool: str, backend: str,
                 program_hash: str, cost_max_usd: float,
                 budget_remaining_usd: float = 0.0, cost_basis: str = "",
                 qubits: int = 0, shots: int = 0,
                 blast_radius: str = "spend only; no data egress",
                 reversibility: str = "irreversible spend once dequeued",
                 evidence: dict[str, Any] | None = None,
                 alternatives: dict[str, Any] | None = None,
                 ttl_s: int = DEFAULT_APPROVAL_TTL_S,
                 requestor: str = "",
                 args: dict[str, Any] | None = None) -> ApprovalPacket:
    """Build a packet. All spend fields required; no silent defaults for money."""
    if cost_max_usd <= 0:
        raise ValueError("cost_max_usd must be positive: approvals never authorize unbounded spend")
    return ApprovalPacket(
        packet_id=f"appr-{secrets.token_hex(8)}",
        agent_id=agent_id, server=server, tool=tool, backend=backend,
        program_hash=program_hash, qubits=qubits, shots=shots,
        cost_max_usd=round(float(cost_max_usd), 6),
        budget_remaining_usd=round(float(budget_remaining_usd), 6),
        cost_basis=cost_basis, blast_radius=blast_radius,
        reversibility=reversibility, evidence=evidence or {},
        alternatives=alternatives or {},
        expires_at=time.time() + ttl_s, nonce=secrets.token_hex(16),
        requestor=requestor,
        args=args or {},
    )


def elicitation_form(packet: ApprovalPacket) -> dict[str, Any]:
    """Flat form-mode elicitation params (spec: flat primitives only)."""
    return {
        "method": APPROVAL_METHOD,
        "params": {
            "mode": "form",
            "message": (
                f"{packet.tool} on {packet.backend}: "
                f"spend up to ${packet.cost_max_usd:.2f} "
                f"(${packet.budget_remaining_usd:.2f} budget left). "
                f"{packet.reversibility}. Expires "
                f"{time.strftime('%Y-%m-%dT%H:%M:%SZ', time.gmtime(packet.expires_at))} "
                f"nonce {packet.nonce[:8]}. See console card {packet.packet_id} for evidence."
            ),
            "requestedSchema": {
                "type": "object",
                "properties": {
                    "decision": {
                        "type": "string",
                        "enum": ["approve_once", "approve_up_to_max", "deny"],
                        "description": "Approve this exact job only, up to the stated max, or deny",
                    },
                    "cost_ack_usd": {
                        "type": "number",
                        "minimum": packet.cost_max_usd,
                        "maximum": packet.cost_max_usd,
                        "description": "Re-type the maximum spend to prove you read it",
                    },
                    "reason": {
                        "type": "string",
                        "maxLength": 280,
                        "description": "Required when denying; returned to the agent",
                    },
                },
                "required": ["decision", "cost_ack_usd"],
            },
        },
    }


def _canonical_state(packet: ApprovalPacket, sub: str) -> bytes:
    args_digest = hashlib.sha256(json.dumps(packet.args, sort_keys=True).encode()).hexdigest()
    return json.dumps({
        "args_digest": args_digest,
        "cost_max_usd": packet.cost_max_usd,
        "exp": packet.expires_at,
        "nonce": packet.nonce,
        "packet_id": packet.packet_id,
        "server": packet.server,
        "sub": sub,
        "tool": packet.tool,
    }, sort_keys=True).encode()


def seal_state(packet: ApprovalPacket, sub: str, secret: bytes) -> str:
    """Opaque requestState: HMAC-bound, echoed verbatim, never parsed by clients."""
    body = _canonical_state(packet, sub)
    sig = hmac.new(secret, body, hashlib.sha256).hexdigest()
    return sig + "." + body.decode()


def open_state(state: str, secret: bytes, now: float | None = None) -> dict[str, Any]:
    """Verify and decode requestState. Raises ValueError on any failure."""
    now = time.time() if now is None else now
    try:
        sig, body = state.split(".", 1)
    except ValueError:
        raise ValueError("malformed request state")
    expect = hmac.new(secret, body.encode(), hashlib.sha256).hexdigest()
    if not hmac.compare_digest(expect, sig):
        raise ValueError("request state signature mismatch")
    try:
        data = json.loads(body)
    except json.JSONDecodeError:
        raise ValueError("request state undecodable")
    if not isinstance(data, dict) or data.get("exp", 0) <= now:
        raise ValueError("approval expired")
    for key in ("packet_id", "sub", "server", "tool", "nonce"):
        if not data.get(key):
            raise ValueError(f"request state missing {key}")
    return data


class ApprovalRegistry:
    """Single-use nonce tracking with TTL sweep and bounded size."""

    def __init__(self) -> None:
        self._consumed: dict[str, float] = {}

    def _sweep(self, now: float) -> None:
        expired = [n for n, exp in self._consumed.items() if exp <= now]
        for n in expired:
            del self._consumed[n]
        while len(self._consumed) > MAX_PENDING_NONCES:
            oldest = min(self._consumed, key=self._consumed.get)
            del self._consumed[oldest]

    def consume(self, nonce: str, ttl_s: int = DEFAULT_APPROVAL_TTL_S,
                now: float | None = None) -> None:
        """Mark a nonce used. Raises on replay."""
        now = time.time() if now is None else now
        self._sweep(now)
        if nonce in self._consumed:
            raise ValueError("approval already consumed (replay refused)")
        self._consumed[nonce] = now + ttl_s


PacketBuilder = Callable[..., ApprovalPacket | None]
