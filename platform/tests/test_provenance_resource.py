"""Phase 3: the provenance ledger is queryable as an MCP resource."""

import json
import os
import sys

import pytest

from mcp import ClientSession, StdioServerParameters
from mcp.client.stdio import stdio_client

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))


@pytest.mark.asyncio
async def test_provenance_resource_readable():
    params = StdioServerParameters(command=sys.executable, args=["-m", "kubona.mcp.servers.provenance"],
                                   cwd=ROOT, env={**os.environ, "PYTHONPATH": ROOT})
    async with stdio_client(params) as (read, write):
        async with ClientSession(read, write) as session:
            await session.initialize()
            # write one record via the tool, then read it back via the RESOURCE
            result = await session.call_tool("write_record", {
                "kind": "experiment",
                "payload": {"circuit_hash": "res-abc", "provider": "local-simulator",
                            "shots": 512, "results": {"00": 512}, "cost_usd": 0.0}})
            assert json.loads(result.content[0].text)["chain_ok"] is True

            resources = await session.list_resources()
            uris = {r.uri for r in resources.resources}
            assert "provenance://records" in uris and "provenance://best" in uris

            read_rec = await session.read_resource("provenance://records")
            records = json.loads(read_rec.contents[0].text)["records"]
            assert any("res-abc" in r for r in records)

            read_best = await session.read_resource("provenance://best")
            agg = json.loads(read_best.contents[0].text)["aggregate"]
            assert "res-abc" in agg
            assert agg["res-abc"]["providers"] == ["local-simulator"]
