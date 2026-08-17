"""Console REST API (Phase 2) — the read/approve-only surface the Flutter console talks to.

Per ui-ux-vision's two hard rules:
  1. READ/APPROVE ONLY — this API never submits jobs; job submission stays
     exclusively behind the MCP-governed flow.
  2. Fixed, audited surfaces — approve/deny, pause/kill only.

Auth: bearer token via the same DevAuthProvider (OAuth 2.1 hardens this in Phase 3).
"""

from __future__ import annotations

import os
from typing import Any

from starlette.applications import Starlette
from starlette.middleware import Middleware
from starlette.middleware.cors import CORSMiddleware
from starlette.requests import Request
from starlette.responses import JSONResponse
from starlette.routing import Route

from kubona.control import LoopSupervisor
from kubona.cost.governance import CostGovernanceService
from kubona.mcp.auth import DevAuthProvider
from kubona.providers import list_providers
from kubona.session import SessionStore

_sessions = SessionStore()
_gov = CostGovernanceService(_sessions)
_loops = LoopSupervisor(_sessions)
_auth = DevAuthProvider()

DEV_TOKEN = os.environ.get("KUBONA_CONSOLE_TOKEN", "dev-token")


def _authed(request: Request) -> bool:
    if not request.headers.get("authorization", "").startswith("Bearer "):
        return False
    token = request.headers["authorization"][len("Bearer "):]
    try:
        _auth.verify(token)
        return True
    except Exception:
        return False


def _auth_error() -> JSONResponse:
    return JSONResponse({"ok": False, "error": "unauthorized: use Bearer <token>"}, status_code=401)


def _json(data: dict[str, Any], status: int = 200) -> JSONResponse:
    return JSONResponse(data, status_code=status)


async def health(request: Request) -> JSONResponse:
    if not _authed(request):
        return _auth_error()
    return _json({"ok": True, "service": "kubona-console-api", "version": "0.2.0"})


async def providers(request: Request) -> JSONResponse:
    if not _authed(request):
        return _auth_error()
    reg = {}
    for name, cap in list_providers().items():
        reg[name] = {"modality": cap.modality.value, "status": cap.status, "pricing": cap.pricing.model}
    return _json({"ok": True, "providers": reg})


async def sessions(request: Request) -> JSONResponse:
    if not _authed(request):
        return _auth_error()
    # SessionStore is append-only snapshots; rebuild the latest per id
    latest: dict[str, dict] = {}
    for sid in _all_session_ids():
        latest[sid] = _session_payload(sid)
    return _json({"ok": True, "sessions": list(latest.values())})


def _all_session_ids() -> list[str]:
    ids: list[str] = []
    if _sessions.path.exists():
        for line in _sessions.path.read_text().splitlines():
            if line.strip():
                import json as _j
                sid = _j.loads(line)["session_id"]
                if sid not in ids:
                    ids.append(sid)
    return ids


def _session_payload(sid: str) -> dict:
    s = _sessions.get(sid)
    br = _gov.burn_rate(sid)
    return {
        "session_id": s.session_id, "name": s.name, "owner": s.owner,
        "spent_usd": s.spent_usd, "budget_ceiling_usd": s.budget_ceiling_usd,
        "remaining_usd": s.remaining(), "status": s.status,
        "burn_rate_usd_per_s": br["burn_rate_usd_per_s"],
        "jobs": s.jobs, "approvals": s.approvals,
    }


async def session_detail(request: Request) -> JSONResponse:
    if not _authed(request):
        return _auth_error()
    try:
        return _json({"ok": True, "session": _session_payload(request.path_params["sid"])})
    except KeyError:
        return _json({"ok": False, "error": "unknown session"}, 404)


async def approvals(request: Request) -> JSONResponse:
    if not _authed(request):
        return _auth_error()
    state = request.query_params.get("state")
    out = []
    for a in _gov._load_approvals().values():
        if state is None or a.state == state:
            out.append({"approval_id": a.approval_id, "session_id": a.session_id, "state": a.state,
                        "amount_requested_usd": a.amount_requested_usd, "reason": a.reason,
                        "requested_by": a.requested_by})
    return _json({"ok": True, "approvals": out})


async def approval_action(request: Request) -> JSONResponse:
    if not _authed(request):
        return _auth_error()
    aid = request.path_params["aid"]
    action = request.path_params["action"]
    reviewer = request.query_params.get("reviewer", "operator")
    try:
        if action == "hold":
            a = _gov.hold_approval(aid, reviewer)
        elif action == "approve":
            a = _gov.approve(aid, reviewer, note=request.query_params.get("note"))
        elif action == "reject":
            a = _gov.reject(aid, reviewer, note=request.query_params.get("note") or "rejected from console")
        else:
            return _json({"ok": False, "error": f"unknown action {action}"}, 400)
        return _json({"ok": True, "approval_id": a.approval_id, "state": a.state})
    except KeyError:
        return _json({"ok": False, "error": "unknown approval"}, 404)
    except Exception as e:
        return _json({"ok": False, "error": str(e)}, 400)


async def provenance(request: Request) -> JSONResponse:
    if not _authed(request):
        return _auth_error()
    from kubona.provenance import Ledger

    ledger = Ledger()
    records = [r for r in ledger if r.kind in ("experiment", "vqe_step", "canary")][-50:]
    by_hash: dict[str, dict] = {}
    for r in records:
        key = r.payload.get("circuit_hash") or r.payload.get("provider") or "unknown"
        agg = by_hash.setdefault(key, {"count": 0, "providers": set()})
        agg["count"] += 1
        if r.payload.get("provider"):
            agg["providers"].add(r.payload["provider"])
    return _json({"ok": True, "recent": [r.to_json() for r in records],
                  "aggregate": {k: {"count": v["count"], "providers": sorted(v["providers"])}
                                for k, v in by_hash.items()}})


async def fleet(request: Request) -> JSONResponse:
    if not _authed(request):
        return _auth_error()
    from kubona.canary import canary_check
    from kubona.providers import list_providers

    reg = {}
    for name, cap in list_providers().items():
        entry = {"modality": cap.modality.value, "status": cap.status, "pricing": cap.pricing.model,
                 "canary": None}
        if cap.execution_plugin:
            try:
                entry["canary"] = canary_check(name, shots=1024)["ok"]
            except Exception as e:  # noqa: BLE001 — fleet view flags unreachable providers
                entry["canary"] = f"unreachable: {e}"
        reg[name] = entry
    return _json({"ok": True, "fleet": reg})


async def reports(request: Request) -> JSONResponse:
    if not _authed(request):
        return _auth_error()
    from kubona.reports.notebook import ReportStore

    return _json({"ok": True, "reports": ReportStore().all()})


async def workflows(request: Request) -> JSONResponse:
    if not _authed(request):
        return _auth_error()
    from kubona.workflows.library import WorkflowLibrary

    lib = WorkflowLibrary()
    return _json({"ok": True, "workflows": [{"workflow_id": w.workflow_id,
                                             "name": w.source.get("name"),
                                             "created_by": w.created_by,
                                             "created_by_agent": w.source.get("created_by_agent", False)}
                                            for w in lib.all()]})


async def agents(request: Request) -> JSONResponse:
    if not _authed(request):
        return _auth_error()
    from kubona.agents import AgentRegistry

    return _json({"ok": True, "agents": [a.to_dict() for a in AgentRegistry().all()]})


async def labs(request: Request) -> JSONResponse:
    if not _authed(request):
        return _auth_error()
    from kubona.labs import LabRegistry

    return _json({"ok": True, "labs": LabRegistry().all()})


async def loops(request: Request) -> JSONResponse:
    if not _authed(request):
        return _auth_error()
    return _json({"ok": True, "loops": _loops.list()})


async def loop_action(request: Request) -> JSONResponse:
    if not _authed(request):
        return _auth_error()
    lid = request.path_params["lid"]
    action = request.path_params["action"]
    try:
        if action == "pause":
            h = _loops.pause(lid)
        elif action == "resume":
            h = _loops.resume(lid)
        elif action == "kill":
            h = _loops.kill(lid)
        else:
            return _json({"ok": False, "error": f"unknown action {action}"}, 400)
        return _json({"ok": True, "loop_id": lid, "status": h.status})
    except KeyError:
        return _json({"ok": False, "error": "unknown loop"}, 404)


app = Starlette(
    middleware=[Middleware(CORSMiddleware,
                          allow_origins=["*"], allow_methods=["*"], allow_headers=["*"])],
    routes=[
        Route("/api/health", health),
        Route("/api/providers", providers),
        Route("/api/sessions", sessions),
        Route("/api/sessions/{sid}", session_detail),
        Route("/api/approvals", approvals),
        Route("/api/approvals/{aid}/{action}", approval_action, methods=["POST"]),
        Route("/api/provenance", provenance),
        Route("/api/fleet", fleet),
        Route("/api/reports", reports),
        Route("/api/workflows", workflows),
        Route("/api/agents", agents),
        Route("/api/labs", labs),
        Route("/api/loops", loops),
        Route("/api/loops/{lid}/{action}", loop_action, methods=["POST"]),
    ],
)


def main() -> None:
    import uvicorn

    port = int(os.environ.get("KUBONA_CONSOLE_PORT", "8000"))
    uvicorn.run(app, host="127.0.0.1", port=port, log_level="warning")


if __name__ == "__main__":
    main()
