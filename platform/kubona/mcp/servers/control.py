"""Control-plane MCP server (Phase 2).

Tools: list_loops / loop_status / pause_loop / resume_loop / kill_loop — the
backend the Dart console's live loop inspector talks to (watch + interrupt).
"""

from __future__ import annotations

import asyncio

from mcp.server.mcpserver import MCPServer

from kubona.control import LoopSupervisor
from kubona.session import SessionStore

mcp = MCPServer("kubona-control")

_supervisor = LoopSupervisor(SessionStore())


def register_loop(name: str, session_id: str, total_steps: int = 100) -> dict:
    """A running loop announces itself to the control plane (the harness calls this)."""
    h = _supervisor.register(name, session_id, total_steps)
    return {"ok": True, "loop_id": h.loop_id, "status": h.status}


def step_loop(loop_id: str, step_desc: str) -> dict:
    """A loop reports progress; returns should_run=False when paused/killed."""
    try:
        should = _supervisor.step(loop_id, step_desc)
        return {"ok": True, "should_run": should, "loop_id": loop_id,
                "status": _supervisor.get(loop_id).status,
                "step": _supervisor.get(loop_id).step}
    except KeyError:
        return {"ok": False, "error": f"unknown loop_id {loop_id!r}"}


def list_loops() -> dict:
    return {"ok": True, "loops": _supervisor.list()}


def loop_status(loop_id: str) -> dict:
    try:
        return {"ok": True, **_supervisor.get(loop_id).to_dict()}
    except KeyError:
        return {"ok": False, "error": f"unknown loop_id {loop_id!r}"}


def pause_loop(loop_id: str) -> dict:
    try:
        h = _supervisor.pause(loop_id)
        return {"ok": True, "loop_id": loop_id, "status": h.status}
    except KeyError:
        return {"ok": False, "error": f"unknown loop_id {loop_id!r}"}


def resume_loop(loop_id: str) -> dict:
    try:
        h = _supervisor.resume(loop_id)
        return {"ok": True, "loop_id": loop_id, "status": h.status}
    except KeyError:
        return {"ok": False, "error": f"unknown loop_id {loop_id!r}"}


def kill_loop(loop_id: str) -> dict:
    try:
        h = _supervisor.kill(loop_id)
        return {"ok": True, "loop_id": loop_id, "status": h.status}
    except KeyError:
        return {"ok": False, "error": f"unknown loop_id {loop_id!r}"}


for fn, desc in [
    (register_loop, "A running loop announces itself to the control plane (loop-harness interface)."),
    (step_loop, "A loop reports progress; should_run=False when paused/killed."),
    (list_loops, "List in-flight loops (the console's live loop inspector feed)."),
    (loop_status, "Status of one loop: step, heartbeat, 'what is the agent doing'."),
    (pause_loop, "Pause an in-flight loop (operator override)."),
    (resume_loop, "Resume a paused loop."),
    (kill_loop, "Kill an in-flight loop (operator interrupt)."),
]:
    mcp.add_tool(fn, name=fn.__name__, description=desc)

if __name__ == "__main__":
    asyncio.run(mcp.run_stdio_async())
