"""L2 bench pins: the served MCP surface stays within budget, and the
bench task file is valid (keys compile, required fields present)."""

import json
import re
import sys
from pathlib import Path

REPO = Path(__file__).resolve().parents[2]
BENCH = REPO / "bench"

sys.path.insert(0, str(REPO))  # bench/ lives at repo root, beside platform/
sys.path.insert(0, str(REPO / "platform"))

from bench.surface import SURFACE_BUDGET_BYTES, measure  # noqa: E402


def test_mcp_surface_within_budget():
    entries, total = measure()
    assert len(entries) == 36, f"tool count changed: {len(entries)} (review per L5)"
    assert total <= SURFACE_BUDGET_BYTES, (
        f"surface {total} bytes exceeds {SURFACE_BUDGET_BYTES}: "
        "justify per tool (L5) or raise deliberately"
    )


def test_tasks_valid():
    tasks = json.loads((BENCH / "tasks.json").read_text())
    assert tasks, "no bench tasks"
    seen = set()
    for t in tasks:
        assert t["id"] not in seen, f"duplicate task {t['id']}"
        seen.add(t["id"])
        assert t.get("keys"), f"task {t['id']} has no grading keys"
        for name, pattern in t["keys"].items():
            re.compile(pattern)  # raises on invalid regex
        kind = t.get("kind", "gateway")
        assert kind in ("gateway", "gateway-steps", "mandate", "ledger"), kind
        if kind.startswith("gateway"):
            steps = t.get("steps", [t])
            for s in steps:
                assert "server" in s and "tool" in s, f"task {t['id']} step lacks server/tool"
