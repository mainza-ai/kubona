"""THE PHASE 2 GATE — a human can watch and interrupt an in-flight loop.

Gate P2 (roadmap): "a human operator can watch, and interrupt, an in-flight
multi-hour hybrid loop from the console." The console talks to the control MCP
server; this gate proves the backend contract the Dart console renders:
register a loop, watch it (list/status), interrupt it (pause/kill), and verify
the loop's step gate honors the override — plus session/budget context.
"""

import json
import os
import sys

import pytest

from mcp import ClientSession, StdioServerParameters
from mcp.client.stdio import stdio_client

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))


async def _session(server_module: str):
    """One MCP session to a server (the control supervisor is per-process state)."""
    params = StdioServerParameters(command=sys.executable, args=["-m", server_module],
                                   cwd=ROOT, env={**os.environ, "PYTHONPATH": ROOT})
    client = stdio_client(params)
    read, write = await client.__aenter__()
    session = ClientSession(read, write)
    await session.__aenter__()
    await session.initialize()
    return session, (client, session)


async def _call(session, tool: str, args: dict) -> dict:
    result = await session.call_tool(tool, args)
    return json.loads(result.content[0].text)


@pytest.mark.asyncio
async def test_phase2_gate_watch_and_interrupt():
    sess, handles = await _session("kubona.mcp.servers.control")
    try:
        # the running loop announces itself to the control plane (over MCP)
        reg = await _call(sess, "register_loop",
                          {"name": "vqe-loop", "session_id": "s1", "total_steps": 100})
        assert reg["ok"] and reg["status"] == "running"
        loop_id = reg["loop_id"]

        # the loop reports progress; the console watches via MCP
        step = await _call(sess, "step_loop",
                           {"loop_id": loop_id, "step_desc": "step 1/100: measure energy"})
        assert step["ok"] and step["should_run"] is True and step["step"] == 1

        listed = await _call(sess, "list_loops", {})
        assert listed["ok"] and len(listed["loops"]) == 1
        assert listed["loops"][0]["name"] == "vqe-loop"

        # console inspects status ("what is the agent doing now and why")
        status = await _call(sess, "loop_status", {"loop_id": loop_id})
        assert status["ok"] and status["current_step_desc"] == "step 1/100: measure energy"
        assert status["status"] == "running"

        # operator interrupts from the console; the loop honors the override
        paused = await _call(sess, "pause_loop", {"loop_id": loop_id})
        assert paused["status"] == "paused"
        step2 = await _call(sess, "step_loop", {"loop_id": loop_id, "step_desc": "step 2"})
        assert step2["should_run"] is False

        killed = await _call(sess, "kill_loop", {"loop_id": loop_id})
        assert killed["status"] == "killed"
        final = await _call(sess, "loop_status", {"loop_id": loop_id})
        assert final["current_step_desc"] == "interrupted by operator"
    finally:
        client, sess_h = handles
        await sess_h.__aexit__(None, None, None)
        await client.__aexit__(None, None, None)
