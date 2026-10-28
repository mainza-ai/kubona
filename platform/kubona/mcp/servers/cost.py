"""Cost-governance MCP server — Phase 1.

Tools: create_session, estimate_cost (pre-flight), request_approval /
hold_approval / approve_approval / reject_approval (two-stage gate),
session_burn_rate (real-time telemetry), quote_payment / verify_payment
(x402-compatible payment interface).
"""

from __future__ import annotations

import asyncio
import threading

from mcp.server.mcpserver import MCPServer

from kubona.circuits import parse_qasm2
from kubona.cost.payment import payments_for
from kubona.schema import load_provider
from kubona.mcp.servers._stores import GOVERNANCE as _gov, SESSIONS as _sessions

mcp = MCPServer("kubona-cost")


# Stored quotes (W0.16): verify_payment checks the proof against the amount
# OWED (the stored quote for this session), never a caller-supplied amount —
# the old code re-quoted from caller input, so a $100 job verified with a
# $0.01 proof. Mismatched or missing stored quotes refuse.
_quotes: dict[str, float] = {}
_quotes_lock = threading.Lock()


def create_session(name: str, owner: str, budget_ceiling_usd: float | None = None) -> dict:
    """Open a session with a hard budget ceiling (None = free-tier only enforced)."""
    s = _sessions.create(name=name, owner=owner, budget_ceiling_usd=budget_ceiling_usd)
    return {"ok": True, "session_id": s.session_id, "budget_ceiling_usd": s.budget_ceiling_usd}


def estimate_cost(circuit_qasm: str, provider: str = "local-simulator", shots: int = 1024,
                  session_id: str | None = None) -> dict:
    """Estimate cost + pre-flight verdict against a session's ceiling."""
    circuit = parse_qasm2(circuit_qasm)
    capability = load_provider(provider)
    if session_id:
        session = _sessions.get(session_id)
        pf = _gov.preflight(capability, circuit, shots, session)
        return {"provider": provider, "session_id": session_id,
                "preflight_ok": pf["ok"], "reason": pf["reason"], "estimate": pf["estimate"]}
    est = capability.cost_estimate(shots, circuit)
    return {"provider": provider, "preflight_ok": est["estimated_usd"] == 0.0,
            "reason": "free tier" if est["estimated_usd"] == 0.0 else "session required for paid providers",
            "estimate": est}


def request_approval(session_id: str, amount_usd: float, reason: str) -> dict:
    a = _gov.request_approval(session_id, amount_usd, reason)
    return {"ok": True, "approval_id": a.approval_id, "state": a.state}


def hold_approval(approval_id: str, reviewer: str = "operator") -> dict:
    a = _gov.hold_approval(approval_id, reviewer)
    return {"ok": True, "approval_id": a.approval_id, "state": a.state}


def approve_approval(approval_id: str, reviewer: str = "operator", note: str | None = None,
                     tenant_id: str = "", job_hash: str = "") -> dict:
    """Approve a held/requested approval. tenant_id + job_hash are required
    (the mandate binds them); the operator key comes from
    KUBONA_OPERATOR_PRIVKEY_HEX — unset refuses instead of minting
    unsigned (W0.9)."""
    from kubona.cost.mandate import operator_key_from_env
    try:
        key = operator_key_from_env()
    except ValueError as e:
        return {"ok": False, "error": str(e)}
    try:
        a = _gov.approve(approval_id, reviewer, note,
                         tenant_id=tenant_id, job_hash=job_hash, signing_key=key)
    except RuntimeError as e:
        return {"ok": False, "error": str(e)}
    return {"ok": True, "approval_id": a.approval_id, "state": a.state}


def reject_approval(approval_id: str, reviewer: str = "operator", note: str = "") -> dict:
    a = _gov.reject(approval_id, reviewer, note)
    return {"ok": True, "approval_id": a.approval_id, "state": a.state}


def session_burn_rate(session_id: str) -> dict:
    try:
        br = _gov.burn_rate(session_id)
        return {"ok": True, **br}
    except KeyError:
        return {"ok": False, "error": f"unknown session_id {session_id!r}"}


def quote_payment(session_id: str, amount_usd: float) -> dict:
    """x402-compatible quote: the request IS the transaction (HTTP 402)."""
    try:
        _sessions.get(session_id)
    except KeyError:
        return {"ok": False, "error": f"unknown session_id {session_id!r}"}
    try:
        pay = payments_for(amount_usd)
    except ValueError as e:
        return {"ok": False, "error": str(e)}
    quote = pay.quote(job_ref=session_id, amount_usd=amount_usd)
    with _quotes_lock:
        _quotes[session_id] = quote.amount_usd
    return {"ok": True, **quote.to_dict()}


def verify_payment(session_id: str, proof: str, amount_usd: float) -> dict:
    """Verify a proof against the STORED quote for this session (W0.16)."""
    try:
        _sessions.get(session_id)
    except KeyError:
        return {"ok": False, "error": f"unknown session_id {session_id!r}"}
    with _quotes_lock:
        owed = _quotes.get(session_id)
    if owed is None:
        return {"ok": False, "error": "no stored quote for this session: quote first, then verify"}
    if amount_usd != owed:
        return {"ok": False,
                "error": f"verify amount ${amount_usd} does not match quoted ${owed}: refusing"}
    pay = payments_for(owed)
    quote = pay.quote(job_ref=session_id, amount_usd=owed)
    return pay.verify(proof, quote)


for fn, desc in [
    (create_session, "Open a session with a hard budget ceiling."),
    (estimate_cost, "Estimate cost + pre-flight verdict for a circuit on a provider."),
    (request_approval, "Request spend approval (stage 1: REVIEW)."),
    (hold_approval, "Hold an approval for review (stage 2: HOLD)."),
    (approve_approval, "Approve a held/requested approval."),
    (reject_approval, "Reject an approval with a note."),
    (session_burn_rate, "Real-time burn-rate telemetry for a session."),
    (quote_payment, "x402-compatible price quote (the request is the transaction)."),
    (verify_payment, "Verify an x402 payment proof."),
]:
    mcp.add_tool(fn, name=fn.__name__, description=desc)

if __name__ == "__main__":
    asyncio.run(mcp.run_stdio_async())
