"""Generated MCP catalog (L5, closes P2-13): the served tool surface,
digest-pinned, with a reviewed verdict per tool.

Single source is the LIVE gateway registry (`mounted_tools`) — never a
hardcoded tool list (a hardcoded list undercounts new tools, failing
open). Canonical bytes (sorted keys, compact) feed the digest; the
checked-in file carries review fields (verdict, trust_property,
error_codes) that generation MERGES by (server, name): new tools arrive
with null verdicts, and the test fails until a human fills them.

Regen after intentional tool changes (never hand-edit surface fields):
  KUBONA_UPDATE_CATALOG=1 pytest platform/tests/test_mcp_catalog.py
"""

from __future__ import annotations

import asyncio
import csv
import hashlib
import json
import os
from pathlib import Path


def _repo_root() -> Path:
    return Path(__file__).resolve().parents[3]


def catalog_path() -> Path:
    return _repo_root() / "platform" / "contracts" / "mcp-catalog.json"


def error_catalog_codes() -> set[str]:
    codes = set()
    with open(_repo_root() / "platform" / "contracts" / "error-catalog.csv") as f:
        for row in csv.DictReader(f):
            codes.add(row["code"].strip())
    return codes


def served_entries() -> list[dict]:
    """Live served surface, canonical order, review fields blank.

    Served shape comes from the SDK's OWN list_tools() serialization
    (full by_alias dump, Nones dropped) — the same bytes a client
    receives — so the catalog cannot disagree with the wire on
    algorithm, only on staleness. A future annotations change
    (readOnlyHint etc.) trips the digest by design: any served-surface
    change forces review.
    """
    from kubona.mcp.servers import compiler, control, cost, execution, provenance, world

    entries = []
    for server, mod in [("compiler", compiler), ("control", control), ("cost", cost),
                        ("execution", execution), ("provenance", provenance),
                        ("world", world)]:
        for tool in asyncio.run(mod.mcp.list_tools()):
            d = tool.model_dump(by_alias=True, exclude_none=True)
            d["server"] = server
            d["verdict"] = None
            d["trust_property"] = None
            d["error_codes"] = []
            entries.append(d)
    entries.sort(key=lambda e: (e["server"], e["name"]))
    return entries


REVIEW_FIELDS = ("verdict", "trust_property", "error_codes")


def surface_of(entry: dict) -> dict:
    """Served-shape subset (review fields excluded): filling a verdict
    must never invalidate the digest — the digest pins the surface,
    review diffs are reviewed as review."""
    return {k: v for k, v in entry.items() if k not in REVIEW_FIELDS}


def canonical_bytes(entries: list[dict]) -> bytes:
    return json.dumps([surface_of(e) for e in entries],
                      sort_keys=True, separators=(",", ":")).encode()


def digest(entries: list[dict]) -> str:
    return hashlib.sha256(canonical_bytes(entries)).hexdigest()


def build_catalog() -> dict:
    """Assemble the catalog doc, merging prior review fields by key."""
    entries = served_entries()
    prior: dict[tuple[str, str], dict] = {}
    path = catalog_path()
    if path.exists():
        doc = json.loads(path.read_text())
        for t in doc.get("tools", []):
            prior[(t.get("server"), t.get("name"))] = t
    for e in entries:
        old = prior.get((e["server"], e["name"]), {})
        for field in ("verdict", "trust_property", "error_codes"):
            if field in old:
                e[field] = old[field]
    return {
        "generated_by": "KUBONA_UPDATE_CATALOG=1 pytest platform/tests/test_mcp_catalog.py",
        "digest": digest(entries),
        "tools": entries,
    }


def write_catalog() -> Path:
    path = catalog_path()
    path.write_text(json.dumps(build_catalog(), indent=2) + "\n")
    return path


def update_mode() -> bool:
    return os.environ.get("KUBONA_UPDATE_CATALOG") == "1"
