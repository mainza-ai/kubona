"""Architectural fitness: compute packages must not import the MCP edge (Q-28).

Capability functions accept and return native Python structures uncoupled from
MCP SDK types. This test parses imports with ast (no execution) and fails on
any `kubona.mcp.*` or MCP-SDK import inside compute packages. Adapters may
import inward; compute never imports outward.
"""

import ast
from pathlib import Path

COMPUTE_DIRS = [
    "circuits",
    "anchors",
    "epistemics",
    "science",
    "canary",
    "providers",
    "fidelity",
    "schema",
]

FORBIDDEN_TOP_LEVELS = {"mcp"}
FORBIDDEN_DOTTED = {"kubona.mcp"}


def _imported_modules(path: Path) -> list[str]:
    tree = ast.parse(path.read_text(), filename=str(path))
    mods: list[str] = []
    for node in ast.walk(tree):
        if isinstance(node, ast.Import):
            mods.extend(a.name for a in node.names)
        elif isinstance(node, ast.ImportFrom):
            if node.module:
                mods.append(node.module)
    return mods


def test_compute_packages_do_not_import_mcp_edge():
    root = Path(__file__).parent.parent / "kubona"
    violations = []
    for dirname in COMPUTE_DIRS:
        d = root / dirname
        assert d.is_dir(), f"compute package missing: {d}"
        for path in sorted(d.rglob("*.py")):
            if "__pycache__" in path.parts:
                continue
            for mod in _imported_modules(path):
                top = mod.split(".")[0]
                if top in FORBIDDEN_TOP_LEVELS or any(
                    mod == f or mod.startswith(f + ".") for f in FORBIDDEN_DOTTED
                ):
                    violations.append(f"{path.relative_to(root)} imports {mod}")
    assert not violations, "architectural violations:\n" + "\n".join(violations)
