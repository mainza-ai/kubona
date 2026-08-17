"""Eval/provenance MCP server (Phase 0 stub).

Tools: write_record (append the content-addressed experiment record) and
query_records (read back; Phase 3 makes the ledger an MCP *resource*).
"""

from __future__ import annotations

import asyncio

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

if __name__ == "__main__":
    asyncio.run(mcp.run_stdio_async())
