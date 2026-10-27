"""L5 MCP catalog pins: served surface byte-identical (drift = red),
every tool reviewed (verdict present), error codes ⊆ error catalog,
every mounted tool scope-mapped. Regen (never hand-edit surface):
  KUBONA_UPDATE_CATALOG=1 pytest platform/tests/test_mcp_catalog.py
then fill any null verdicts the merge reports, then re-run green."""

import json
import os
from pathlib import Path

from kubona.mcp import catalog
from kubona.mcp.servers._mount import build_gateway
from kubona.security.policy import TOOL_SCOPES

REPO = Path(__file__).resolve().parents[2]


def _checked():
    path = catalog.catalog_path()
    assert path.exists(), (
        "checked-in MCP catalog missing: regen with "
        "KUBONA_UPDATE_CATALOG=1 pytest platform/tests/test_mcp_catalog.py"
    )
    return json.loads(path.read_text())


def _maybe_update():
    if catalog.update_mode():
        catalog.write_catalog()
        return True
    return False


def test_mounted_tools_all_scope_mapped():
    """Every mounted tool resolves to a scope (L5 review finding:
    compile_circuit shipped mounted but unscoped — default-deny made it
    dead surface. This guard fails the next such gap at introduction)."""
    gw = build_gateway(require_auth=False)
    missing = [f"{s}/{t}" for s, ts in gw.mounted_tools().items()
               for t in ts if f"{s}/{t}" not in TOOL_SCOPES]
    assert not missing, f"mounted but unscoped (dead under default-deny): {missing}"


def test_catalog_matches_live_surface():
    if _maybe_update():
        return
    doc = _checked()
    live = catalog.served_entries()
    assert doc["digest"] == catalog.digest(live), (
        "MCP catalog drift: live surface disagrees with the checked-in "
        "file — regen with KUBONA_UPDATE_CATALOG=1 after intentional "
        "tool changes, never hand-edit surface fields"
    )
    live_keys = {(e["server"], e["name"]) for e in live}
    file_keys = {(t.get("server"), t.get("name")) for t in doc["tools"]}
    assert live_keys == file_keys, (
        f"surface/tool-set skew live={sorted(live_keys - file_keys)} "
        f"file={sorted(file_keys - live_keys)}"
    )
    for t in doc["tools"]:
        live_one = next(e for e in live
                        if (e["server"], e["name"]) == (t["server"], t["name"]))
        for field in ("description", "inputSchema"):
            assert t.get(field) == live_one.get(field), (
                f"{t['server']}/{t['name']}: checked-in {field} drifts "
                "from served bytes — regen, never hand-edit"
            )


def test_catalog_review_complete():
    if catalog.update_mode():
        return
    doc = _checked()
    codes = catalog.error_catalog_codes()
    assert doc["tools"], "catalog has no tools"
    for t in doc["tools"]:
        key = f"{t.get('server')}/{t.get('name')}"
        assert t.get("verdict") in ("keep", "flag"), (
            f"{key}: unreviewed (null verdict) — fill verdict + "
            "trust_property, then re-run green"
        )
        assert t.get("trust_property"), f"{key}: trust_property missing"
        assert isinstance(t.get("error_codes"), list) and t["error_codes"], (
            f"{key}: error_codes missing — retry-vs-escalate needs them")
        unknown = [c for c in t["error_codes"] if c not in codes]
        assert not unknown, f"{key}: uncatalogued codes {unknown}"
