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


def codesign_rerun(
    session_id: str,
    spec_kind: str = "and2",
    budget_steps: int = 4,
    seed: int = 7,
    num_tests: int = 64,
    max_failures: int = 0,
    kappa: int = 1,
    max_tokens_per_step: int = 512,
    max_usd: float = 0.0,
    request_id: str | None = None,
) -> dict:
    """Run the governed circuit-proposal rerun through the Go API.

    This is the scoped agent rail for C5b: the MCP server never holds an LLM
    key and never calls a model directly. It forwards to
    `POST /api/codesign/rerun` with the operator bearer; the Go core owns the
    role/settings, BYOK estimate, bounds, and sealed receipt.

    `request_id` is returned (generated if absent) so a caller can retry the
    same receipt after a transient failure instead of double-spending.
    """
    import json
    import os
    import urllib.error
    import urllib.request
    import uuid

    token = os.environ.get("KUBONA_API_TOKEN", "").strip()
    if not token:
        return {"ok": False, "error": "KUBONA_API_TOKEN absent: rerun requires operator auth (no anonymous spend)"}
    if not session_id.strip():
        return {"ok": False, "error": "session_id is required: rerun is spend-attributed"}
    if not 1 <= budget_steps <= 12:
        return {"ok": False, "error": "budget_steps must be in [1,12]: refusing"}
    if not 1 <= max_tokens_per_step <= 1024:
        return {"ok": False, "error": "max_tokens_per_step must be in [1,1024]: refusing"}
    if not 1 <= num_tests <= 512:
        return {"ok": False, "error": "num_tests must be in [1,512]: refusing"}
    if not 0 <= max_failures <= num_tests:
        return {"ok": False, "error": "max_failures must be in [0,num_tests]: refusing"}
    if not 1 <= kappa <= 3:
        return {"ok": False, "error": "kappa must be in [1,3]: refusing"}
    if seed < 0:
        return {"ok": False, "error": "seed must be >= 0: refusing"}
    if max_usd < 0:
        return {"ok": False, "error": "max_usd must be >= 0: refusing"}
    rid = request_id or uuid.uuid4().hex
    body = {
        "session_id": session_id,
        "request_id": rid,
        "spec_kind": spec_kind,
        "budget_steps": budget_steps,
        "seed": seed,
        "num_tests": num_tests,
        "max_failures": max_failures,
        "kappa": kappa,
        "max_tokens_per_step": max_tokens_per_step,
        "max_usd": max_usd,
    }
    base = os.environ.get("KUBONA_API_URL", "http://127.0.0.1:8080").rstrip("/")
    req = urllib.request.Request(
        f"{base}/api/codesign/rerun",
        data=json.dumps(body).encode("utf-8"),
        headers={
            "Content-Type": "application/json",
            "Authorization": f"Bearer {token}",
        },
        method="POST",
    )
    try:
        with urllib.request.urlopen(req, timeout=660) as resp:
            out = json.loads(resp.read().decode("utf-8"))
    except urllib.error.HTTPError as e:
        raw = e.read().decode("utf-8", "replace")
        try:
            err = json.loads(raw)
            msg = err.get("message") or err.get("error") or raw[:280]
        except Exception:
            msg = raw[:280]
        return {"ok": False, "error": str(msg)[:280], "request_id": rid, "http_status": e.code}
    except Exception as e:  # noqa: BLE001 — transport refusal, never a fake success
        return {"ok": False, "error": f"rerun API unreachable: {e}"[:280], "request_id": rid}
    if not isinstance(out, dict) or out.get("ok") is not True:
        msg = out.get("message") or out.get("error") if isinstance(out, dict) else "unparseable response"
        return {"ok": False, "error": str(msg)[:280], "request_id": rid}
    return out


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
    (codesign_rerun, "Run the governed circuit-proposal rerun via the Go API (BYOK estimate)."),
]:
    mcp.add_tool(fn, name=fn.__name__, description=desc)

if __name__ == "__main__":
    asyncio.run(mcp.run_stdio_async())
