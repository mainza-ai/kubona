"""Measure the served MCP surface (leanrea I1): every mounted tool as
{name, description, inputSchema} from live signatures, summed bytes.
Single source for bench/ctx.sh and the CI budget gate — one measurer,
never two copies that rot apart."""

from __future__ import annotations

import inspect
import json
import sys

# Surface budget: measured ~8 KB (2026-10-17) → 16 KB headroom.
# Growth past this fails CI (test_mcp_surface.py). Raise deliberately,
# with a reviewed per-tool justification (L5), never by drift.
SURFACE_BUDGET_BYTES = 16 * 1024


def _schema(fn) -> dict:
    sig = inspect.signature(fn)
    props, req = {}, []
    for n, p in sig.parameters.items():
        t = "string"
        ann = str(p.annotation)
        if p.annotation in (int,) or "int" in ann:
            t = "integer"
        elif p.annotation in (float,) or "float" in ann:
            t = "number"
        elif p.annotation in (bool,) or "bool" in ann:
            t = "boolean"
        props[n] = {"type": t}
        if p.default is inspect.Parameter.empty:
            req.append(n)
    return {"type": "object", "properties": props, "required": req}


def measure() -> tuple[list[dict], int]:
    """(tool_entries, total_bytes) over the production mount table.

    Derives from the LIVE gateway registry (never a hardcoded tool
    list — a hardcoded list undercounts new tools, failing open).
    """
    from kubona.mcp.servers._mount import build_gateway

    gw = build_gateway(require_auth=False)
    mounted = gw.mounted_tools()
    entries = []
    for server in sorted(mounted):
        for name in sorted(mounted[server]):
            fn = mounted[server][name]
            entries.append({
                "server": server,
                "name": name,
                "description": fn.__doc__ or "",
                "inputSchema": _schema(fn),
            })
    total = sum(len(json.dumps(e)) for e in entries)
    return entries, total


def main() -> int:
    budget = SURFACE_BUDGET_BYTES
    if "--budget" in sys.argv:
        budget = int(sys.argv[sys.argv.index("--budget") + 1])
    entries, total = measure()
    by_server: dict[str, int] = {}
    for e in entries:
        by_server[e["server"]] = by_server.get(e["server"], 0) + 1
    print(f"tools: {len(entries)} {by_server}")
    print(f"surface: {total} bytes (~{total // 4} tokens), budget: {budget}")
    if total > budget:
        print(f"OVER BUDGET by {total - budget} bytes: justify per tool (L5) or raise deliberately")
        return 1
    print("within budget")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
