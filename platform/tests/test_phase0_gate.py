"""THE PHASE 0 GATE — end-to-end over real MCP (stdio transport).

Gate P0 (roadmap): "an agent submits a simulator job end-to-end through the MCP
chain and gets results back with a provenance record written, using only
open-source dependencies."

This test drives the real MCP servers as subprocesses over the stdio transport,
as a real agent would: cost server -> execution server -> provenance server.
"""

import asyncio
import json
import os
import sys

import pytest

from mcp import ClientSession, StdioServerParameters
from mcp.client.stdio import stdio_client

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
BELL = "OPENQASM 2.0;\nqreg q[2];\nh q[0];\ncx q[0],q[1];"


async def _session(server_module: str):
    """Open one stdio MCP session to a server (an agent holds a session per server).

    NOTE: the execution server's job store is in-memory per process, so submit and
    get_result must share ONE session — the realistic agent pattern, and the reason
    the plan threads `job_id` through the session abstraction.
    """
    params = StdioServerParameters(
        command=sys.executable,
        args=["-m", server_module],
        cwd=ROOT,
        env={**os.environ, "PYTHONPATH": ROOT},
    )
    client = stdio_client(params)
    read, write = await client.__aenter__()
    session = ClientSession(read, write)
    await session.__aenter__()
    await session.initialize()
    return session, (client, session)


async def _call(session, tool: str, args: dict) -> dict:
    result = await session.call_tool(tool, args)
    text = result.content[0].text
    return json.loads(text)


async def _close(session, handles):
    client, sess = handles
    await sess.__aexit__(None, None, None)
    await client.__aexit__(None, None, None)


@pytest.mark.asyncio
async def test_phase0_gate_over_mcp():
    # 1. cost pre-flight via the cost server (one-shot)
    cost_s, cost_h = await _session("kubona.mcp.servers.cost")
    try:
        cost = await _call(cost_s, "estimate_cost",
                           {"circuit_qasm": BELL, "provider": "local-simulator", "shots": 2048})
        assert cost["preflight_ok"] is True
        assert cost["estimate"]["estimated_usd"] == 0.0
    finally:
        await _close(cost_s, cost_h)

    # 2. submit + result via ONE execution-server session (job state is per-process)
    exec_s, exec_h = await _session("kubona.mcp.servers.execution")
    try:
        job = await _call(exec_s, "submit_job",
                          {"circuit_qasm": BELL, "provider": "local-simulator", "shots": 2048, "seed": 7})
        assert job["ok"] is True and job["status"] == "completed"
        assert "job_id" in job and "circuit_hash" in job

        result = await _call(exec_s, "get_result", {"job_id": job["job_id"]})
        assert result["ok"] is True
        counts = result["result"]["counts"]
        assert set(counts) <= {"00", "11"} and sum(counts.values()) == 2048
        assert abs(counts.get("00", 0) - counts.get("11", 0)) < 200  # entangled ~50/50
    finally:
        await _close(exec_s, exec_h)

    # 3+4. provenance write + read-back via one provenance-server session (file-backed ledger)
    prov_s, prov_h = await _session("kubona.mcp.servers.provenance")
    try:
        rec = await _call(prov_s, "write_record", {
            "kind": "experiment",
            "payload": {
                "job_id": job["job_id"],
                "circuit_hash": job["circuit_hash"],
                "provider": "local-simulator",
                "shots": 2048,
                "results": counts,
                "cost_usd": 0.0,
            },
        })
        assert rec["ok"] is True and rec["chain_ok"] is True
        assert rec["record_hash"] and rec["content_hash"]

        q = await _call(prov_s, "query_records", {"kind": "experiment"})
        assert q["count"] >= 1
        assert "local-simulator" in q["records"][-1]
    finally:
        await _close(prov_s, prov_h)


def test_phase0_gate_local_chain():
    """Same chain in-process (fast path; no subprocess) — used as CI smoke."""
    from kubona.mcp.servers._mount import build_gateway

    gw = build_gateway()
    token = "dev-token"
    compiled = gw.call("compiler", "transpile_circuit", {"circuit_qasm": BELL}, token=token)
    assert compiled["num_qubits"] == 2
    job = gw.call("execution", "submit_job",
                  {"circuit_qasm": BELL, "provider": "local-simulator", "shots": 2048, "seed": 7}, token=token)
    assert job["status"] == "completed"
    res = gw.call("execution", "get_result", {"job_id": job["job_id"]}, token=token)
    assert set(res["result"]["counts"]) <= {"00", "11"}
    rec = gw.call("provenance", "write_record", {
        "kind": "experiment",
        "payload": {"job_id": job["job_id"], "circuit_hash": compiled["circuit_hash"],
                    "provider": "local-simulator", "shots": 2048,
                    "results": res["result"]["counts"], "cost_usd": 0.0},
    }, token=token)
    assert rec["chain_ok"] is True
