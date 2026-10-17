"""Control-plane MCP server (Phase 2).

Tools: list_loops / loop_status / pause_loop / resume_loop / kill_loop — the
live loop inspector feed (watch + interrupt).

Tenant isolation: loops carry an owning-tenant label (set at register);
reads and overrides take a requester and see/touch only that tenant's
loops. The gateway capability gate (control:read/manage/override)
authorizes WHO may call; the label scopes WHICH loops. step_loop is the
exception: the loop reporting to itself, authorized by possession of the
unguessable loop_id (no requester threaded — documented, not omitted).
"""

from __future__ import annotations

import asyncio

from mcp.server.mcpserver import MCPServer

from kubona.control import LoopSupervisor
from kubona.session import SessionStore

mcp = MCPServer("kubona-control")

_supervisor = LoopSupervisor(SessionStore())


def _refused(loop_id: str) -> dict:
    return {"ok": False, "error": f"loop {loop_id!r} belongs to another lab tenant: refusing"}


def register_loop(name: str, session_id: str, total_steps: int = 100,
                  tenant: str = "default") -> dict:
    """A running loop announces itself to the control plane (the harness calls this)."""
    h = _supervisor.register(name, session_id, total_steps, tenant=tenant or "default")
    return {"ok": True, "loop_id": h.loop_id, "status": h.status, "tenant": h.tenant}


def step_loop(loop_id: str, step_desc: str) -> dict:
    """A loop reports progress; returns should_run=False when paused/killed."""
    try:
        should = _supervisor.step(loop_id, step_desc)
        return {"ok": True, "should_run": should, "loop_id": loop_id,
                "status": _supervisor.get(loop_id).status,
                "step": _supervisor.get(loop_id).step}
    except KeyError:
        return {"ok": False, "error": f"unknown loop_id {loop_id!r}"}


def list_loops(requester: str | None = "default") -> dict:
    return {"ok": True, "loops": _supervisor.list_for(requester)}


def loop_status(loop_id: str, requester: str | None = "default") -> dict:
    try:
        return {"ok": True, **_supervisor.get_for(loop_id, requester).to_dict()}
    except KeyError:
        return {"ok": False, "error": f"unknown loop_id {loop_id!r}"}
    except PermissionError:
        return _refused(loop_id)


def pause_loop(loop_id: str, requester: str | None = "default") -> dict:
    try:
        _supervisor.get_for(loop_id, requester)
        h = _supervisor.pause(loop_id)
        return {"ok": True, "loop_id": loop_id, "status": h.status}
    except KeyError:
        return {"ok": False, "error": f"unknown loop_id {loop_id!r}"}
    except PermissionError:
        return _refused(loop_id)


def resume_loop(loop_id: str, requester: str | None = "default") -> dict:
    try:
        _supervisor.get_for(loop_id, requester)
        h = _supervisor.resume(loop_id)
        return {"ok": True, "loop_id": loop_id, "status": h.status}
    except KeyError:
        return {"ok": False, "error": f"unknown loop_id {loop_id!r}"}
    except PermissionError:
        return _refused(loop_id)


def kill_loop(loop_id: str, requester: str | None = "default") -> dict:
    try:
        _supervisor.get_for(loop_id, requester)
        h = _supervisor.kill(loop_id)
        return {"ok": True, "loop_id": loop_id, "status": h.status}
    except KeyError:
        return {"ok": False, "error": f"unknown loop_id {loop_id!r}"}
    except PermissionError:
        return _refused(loop_id)


for fn, desc in [
    (register_loop, "A running loop announces itself to the control plane (loop-harness interface)."),
    (step_loop, "A loop reports progress; should_run=False when paused/killed."),
    (list_loops, "List in-flight loops for the requesting tenant (the console's live loop inspector feed)."),
    (loop_status, "Status of one loop: step, heartbeat, 'what is the agent doing'."),
    (pause_loop, "Pause an in-flight loop (operator override)."),
    (resume_loop, "Resume a paused loop."),
    (kill_loop, "Kill an in-flight loop (operator interrupt)."),
]:
    mcp.add_tool(fn, name=fn.__name__, description=desc)

if __name__ == "__main__":
    asyncio.run(mcp.run_stdio_async())
