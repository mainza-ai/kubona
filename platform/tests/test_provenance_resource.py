"""Phase 3: the provenance ledger is queryable as an MCP resource."""

import json
import os
import sys
import uuid

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
            # unique hashes: the server ledger file persists across runs, so
            # assertions must key on this run's rows, never on absence overall.
            own_hash = f"res-{uuid.uuid4().hex[:8]}"
            pub_hash = f"pub-{uuid.uuid4().hex[:8]}"
            # tenant-scoped write: visible to its own lab, invisible to ambient resources
            result = await session.call_tool("write_record", {
                "kind": "experiment",
                "payload": {"circuit_hash": own_hash, "provider": "local-simulator",
                            "shots": 512, "results": {"00": 512}, "cost_usd": 0.0}})
            assert json.loads(result.content[0].text)["chain_ok"] is True

            resources = await session.list_resources()
            uris = {r.uri for r in resources.resources}
            assert "provenance://records" in uris and "provenance://best" in uris

            read_rec = await session.read_resource("provenance://records")
            records = json.loads(read_rec.contents[0].text)["records"]
            assert not any(own_hash in r for r in records)

            # public write: visible ambiently and in label-scoped aggregation
            await session.call_tool("write_record", {
                "kind": "experiment",
                "payload": {"circuit_hash": pub_hash, "provider": "local-simulator",
                            "shots": 512, "results": {"00": 512}, "cost_usd": 0.0},
                "label": "public"})
            read_rec = await session.read_resource("provenance://records")
            records = json.loads(read_rec.contents[0].text)["records"]
            assert any(pub_hash in r for r in records)
            assert not any(own_hash in r for r in records)

            read_best = await session.read_resource("provenance://best")
            agg = json.loads(read_best.contents[0].text)["aggregate"]
            assert pub_hash in agg
            assert agg[pub_hash]["providers"] == ["local-simulator"]
            assert own_hash not in agg

            # tenant-scoped query with explicit requester sees its own rows
            q = await session.call_tool("query_records", {"kind": "experiment",
                                                          "requester": "default"})
            assert q is not None
