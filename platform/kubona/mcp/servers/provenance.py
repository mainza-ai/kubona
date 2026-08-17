"""Eval/provenance MCP server.

Exposes the content-addressed, hash-chained provenance ledger as an
agent-queryable MCP resource: write_record / query_records (+ best aggregation).
"""

from __future__ import annotations

import asyncio
import json

from mcp.server.mcpserver import MCPServer

from kubona.provenance import Ledger

mcp = MCPServer("kubona-provenance")

_ledger = Ledger()


def write_record(kind: str, payload: dict) -> dict:
    """Append an immutable, content-addressed record to the provenance ledger."""
    record = _ledger.append(kind, payload)
    return {"ok": True, "record_id": record.record_id, "record_hash": record.record_hash,
            "content_hash": record.content_hash, "chain_ok": _ledger.validate_chain()[0]}


def query_records(kind: str | None = None) -> dict:
    records = [r for r in _ledger if kind is None or r.kind == kind]
    return {"count": len(records), "records": [r.to_json() for r in records]}


mcp.add_tool(write_record, name="write_record",
             description="Append an immutable, content-addressed record to the provenance ledger.")
mcp.add_tool(query_records, name="query_records",
             description="Return ledger records, optionally filtered by kind.")


def _aggregate(kind: str | None = None) -> dict:
    """'What worked best historically': per circuit-hash aggregation."""
    records = [r for r in _ledger if kind is None or r.kind == kind]
    by_hash: dict[str, dict] = {}
    for r in records:
        p = r.payload
        key = p.get("circuit_hash") or p.get("provider") or "unknown"
        agg = by_hash.setdefault(key, {"count": 0, "providers": set(), "costs": []})
        agg["count"] += 1
        if p.get("provider"):
            agg["providers"].add(p["provider"])
        if p.get("cost_usd") is not None:
            agg["costs"].append(p["cost_usd"])
    out = {}
    for key, agg in by_hash.items():
        out[key] = {"count": agg["count"],
                    "providers": sorted(agg["providers"]),
                    "avg_cost_usd": round(sum(agg["costs"]) / len(agg["costs"]), 6) if agg["costs"] else None}
    return out


@mcp.resource("provenance://records", name="provenance-records",
              description="Immutable experiment/audit records from the content-addressed ledger.",
              mime_type="application/json")
def records_resource() -> str:
    return json.dumps({"records": [r.to_json() for r in _ledger]})


@mcp.resource("provenance://best", name="provenance-best",
              description="Historical aggregation — what worked best per circuit hash (Phase 3: ledger as MCP resource).",
              mime_type="application/json")
def best_resource() -> str:
    return json.dumps({"aggregate": _aggregate()})

if __name__ == "__main__":
    asyncio.run(mcp.run_stdio_async())
