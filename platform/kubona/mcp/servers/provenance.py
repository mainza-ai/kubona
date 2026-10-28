"""Eval/provenance MCP server.

Exposes the content-addressed, hash-chained provenance ledger as an
agent-queryable MCP resource: write_record / query_records (+ best aggregation).
"""

from __future__ import annotations

import asyncio
import json
from collections import deque

from mcp.server.mcpserver import MCPServer

from kubona.provenance import Ledger
from kubona.security.tenant import CrossTenantRefusal, resolve_tenant
from kubona.session import strict_tenant_slug

mcp = MCPServer("kubona-provenance")

_ledger = Ledger()

MAX_QUERY_RECORDS = 1000
MAX_SCAN_RECORDS = 10000


from kubona.provenance import Ledger, readable


def _resolve_write_label(label: str | None, caller: dict | None) -> str | None:
    """Label for a new record (injected identity wins, fail-closed).

    - With an injected caller: omitted labels bind to the principal's own
      tenant file-label; "public" stays public; an explicit "tenant/<other>"
      refuses (label spoofing across tenants). Malformed labels refuse.
    - Without one (direct in-process / stdio callers): legacy behavior —
      omitted stays omitted (the ledger defaults to its file tenant), but an
      explicit "tenant/<id>" is strictly validated (refuse-don't-collapse).

    Residual: the module-global `_ledger` is the default-tenant file, so a
    non-default caller tenant is recorded as a `tenant/<T>` label inside the
    default file. Read isolation holds via the label check, but physical
    sharding per tenant (a Ledger registry keyed by validated tenant) is a
    follow-up for the ledger owner.
    """
    if caller is not None:
        mine = caller["tenant"]
        if label is None or label == "":
            return f"tenant/{mine}"
        if label == "public":
            return "public"
        if label.startswith("tenant/"):
            want = label[len("tenant/"):]
            try:
                strict_tenant_slug(want)
            except ValueError as e:
                raise CrossTenantRefusal(f"malformed tenant label {label!r}: refusing ({e})")
            if want != mine:
                raise CrossTenantRefusal(
                    f"label {label!r} claims another lab tenant (principal is {mine!r}): "
                    "refusing — cross-tenant writes belong to the caller's own tenant file "
                    "(no cross-tenant grant mechanism exists yet)")
            return label
        raise CrossTenantRefusal(f"unknown label shape {label!r}: refusing")
    if label is not None and label.startswith("tenant/"):
        strict_tenant_slug(label[len("tenant/"):])  # refuse-don't-collapse, fail-closed
    return label


def write_record(kind: str, payload: dict, label: str | None = None,
                 caller: dict | None = None) -> dict:
    """Append an immutable, content-addressed record to the provenance ledger.

    `label` is "public" or omitted (defaults to the ledger's lab tenant).
    Cross-tenant writes belong to the caller's own tenant file.
    Gateway-injected `caller` binds the label; spoofs refuse.
    """
    try:
        resolved = _resolve_write_label(label, caller)
    except CrossTenantRefusal as e:
        return {"ok": False, "error": f"cross-tenant write refused: {e}"}
    except ValueError as e:
        return {"ok": False, "error": str(e)}
    record = _ledger.append(kind, payload, label=resolved)
    return {"ok": True, "record_id": record.record_id, "record_hash": record.record_hash,
            "content_hash": record.content_hash, "chain_ok": _ledger.validate_chain()[0],
            "label": record.label}


def query_records(kind: str | None = None, requester: str | None = None,
                   limit: int = 100, offset: int = 0,
                   caller: dict | None = None) -> dict:
    """Query ledger records with pre-serialization readership enforcement.

    Scoped to the injected `caller` identity when present (a differing
    requester refuses); without one, omitted requester = ambient (public
    rows only) and explicit requesters are strictly validated. Bounded
    scan + page.
    """
    try:
        if caller is not None:
            effective = resolve_tenant(supplied=requester, caller=caller)
        elif requester is None:
            effective = None  # ambient: public rows only (never a default-tenant scope)
        else:
            effective = strict_tenant_slug(requester)
    except CrossTenantRefusal as e:
        return {"ok": False, "error": f"cross-tenant read refused: {e}",
                "count": 0, "returned": 0, "offset": offset if isinstance(offset, int) else 0,
                "limit": limit if isinstance(limit, int) else 100, "truncated": False, "records": []}
    except ValueError as e:
        return {"ok": False, "error": str(e),
                "count": 0, "returned": 0, "offset": offset if isinstance(offset, int) else 0,
                "limit": limit if isinstance(limit, int) else 100, "truncated": False, "records": []}
    # Readership is enforced BEFORE serialization (never after flooding):
    # tenant rows are visible inside their own lab tenant only. Ambient
    # callers (requester None) see public rows. Both the scan and the page are bounded.
    if not isinstance(limit, int) or isinstance(limit, bool):
        limit = 100
    limit = min(max(limit, 1), MAX_QUERY_RECORDS)
    if not isinstance(offset, int) or isinstance(offset, bool) or offset < 0:
        offset = 0
    matched = 0
    page: list[dict] = []
    truncated = False
    scanned = 0
    for r in _ledger:
        scanned += 1
        if scanned > MAX_SCAN_RECORDS:
            truncated = True
            break
        if kind is not None and r.kind != kind:
            continue
        if not readable(r, effective, _ledger.tenant or "default"):
            continue
        matched += 1
        if matched > offset and len(page) < limit:
            page.append(r.to_json())
    if matched > offset + len(page):
        truncated = True
    return {"count": matched, "returned": len(page), "offset": offset,
            "limit": limit, "truncated": truncated, "records": page}


mcp.add_tool(write_record, name="write_record",
             description="Append an immutable, content-addressed record to the provenance ledger.")
mcp.add_tool(query_records, name="query_records",
             description="Return ledger records, optionally filtered by kind.")


def _aggregate(kind: str | None = None, requester: str | None = None) -> dict:
    """'What worked best historically': per circuit-hash aggregation.

    Aggregates are label-joined: only records readable by the requester feed
    the counts, so dashboards are label-scoped by construction — never
    global-with-redaction (existence is information).
    """
    records = []
    truncated = False
    for i, r in enumerate(_ledger):
        if i >= MAX_SCAN_RECORDS:
            truncated = True
            break
        if (kind is None or r.kind == kind) and readable(r, requester, _ledger.tenant or "default"):
            records.append(r)
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
    out["__truncated__"] = truncated
    return out


@mcp.resource("provenance://records", name="provenance-records",
              description="Immutable experiment/audit records from the content-addressed ledger.",
              mime_type="application/json")
def records_resource() -> str:
    # Ambient resource context carries no requester: public rows only
    # (legitimately public — MCP resources carry no identity; this is the
    # documented ambient path, alongside best_resource's public-only
    # aggregation and control step_loop's loop-id possession).
    # Tenant rows are reachable via query_records under an authenticated
    # (gateway-bound) identity.
    records: deque[dict] = deque(maxlen=MAX_QUERY_RECORDS)
    truncated = False
    for r in _ledger:
        if readable(r, None, _ledger.tenant or "default"):
            if len(records) == MAX_QUERY_RECORDS:
                truncated = True
            records.append(r.to_json())
    return json.dumps({"records": list(records), "returned": len(records), "truncated": truncated})


@mcp.resource("provenance://best", name="provenance-best",
              description="Historical aggregation — what worked best per circuit hash (Phase 3: ledger as MCP resource).",
              mime_type="application/json")
def best_resource() -> str:
    return json.dumps({"aggregate": _aggregate()})

if __name__ == "__main__":
    asyncio.run(mcp.run_stdio_async())
