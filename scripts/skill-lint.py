#!/usr/bin/env python3
"""Machine-verifiable drift gate for external agent skills under skills/*/SKILL.md.

Checks every skill against Kubona's live contracts so skills can never rot
silently:
  - Valid YAML frontmatter (name matches dir, single-line description)
  - Size budgets (router <= 6 KiB, domain skills <= 12 KiB)
  - No symlinked skill dirs/files and no dev-docs/ references
  - Every `path:line` anchor resolves in the repository
  - Every referenced `kubona-*` skill exists under skills/
  - Every backticked MCP tool name that looks like a Kubona tool exists in
    platform/contracts/mcp-catalog.json
  - Every backticked `/api/...` route exists in console-openapi.json or
    platform/core/pkg/api/route_policy.go
  - Every backticked error code in error-table rows exists in
    platform/contracts/error-catalog.csv

Usage:
  python3 scripts/skill-lint.py [--strict]
"""

from __future__ import annotations

import csv
import json
import re
import sys
from pathlib import Path

REPO = Path(__file__).resolve().parents[1]
SKILLS_DIR = REPO / "skills"
CONTRACTS_DIR = REPO / "platform" / "contracts"

NAME_RE = re.compile(r"^[a-z0-9](?:[a-z0-9-]{0,62}[a-z0-9])?$")
ANCHOR_RE = re.compile(
    r"`((?:platform|bench|skills|scripts|docs|wiki)/[\w\-./]*\.(?:py|go|json|md|sh|yaml|yml|toml|csv)):(\d+)`"
)
SKILL_REF_RE = re.compile(r"`(kubona-[a-z0-9-]+)`")
API_ROUTE_RE = re.compile(r"`(?:GET|POST|PUT|PATCH|DELETE)?\s*(/api/[a-zA-Z0-9_{}/-]+)`")
ERR_TABLE_RE = re.compile(r"^\|\s*`([a-z][a-z0-9_]+)`\s*\|", re.MULTILINE)

MAX_SKILL_BYTES = 12 * 1024
MAX_ROUTER_BYTES = 6 * 1024
ROUTER_REQUIRED_HEADINGS = ["## Rules", "## Router", "## First move", "## Speed patterns", "## Traps"]


def _load_contracts() -> tuple[set[str], set[str], set[str]]:
    mcp_doc = json.loads((CONTRACTS_DIR / "mcp-catalog.json").read_text(encoding="utf-8"))
    mcp_tools = {t["name"] for t in mcp_doc.get("tools", [])}

    openapi = json.loads((CONTRACTS_DIR / "console-openapi.json").read_text(encoding="utf-8"))
    routes = set(openapi.get("paths", {}).keys())

    # Include non-console Go API routes declared in route_policy.go (e.g. /api/a2a/v1, /api/openapi.json, /api/chemistry/anchor)
    rp_path = REPO / "platform" / "core" / "pkg" / "api" / "route_policy.go"
    if rp_path.exists():
        for m in re.finditer(r'Pattern:\s*"(/[^"]+)"', rp_path.read_text(encoding="utf-8")):
            routes.add(m.group(1))

    error_codes: set[str] = set()
    with open(CONTRACTS_DIR / "error-catalog.csv", encoding="utf-8") as f:
        for row in csv.DictReader(f):
            error_codes.add(row["code"].strip())

    return mcp_tools, routes, error_codes


def _normalize_route(path: str) -> str:
    # Normalize `{sessionId}`, `{id}`, `{name}`, etc. to `{}` for structural matching
    return re.sub(r"\{[^}]+\}", "{}", path.rstrip("/"))


def lint_skills() -> list[str]:
    errors: list[str] = []
    if not SKILLS_DIR.exists():
        return [f"skills directory missing: {SKILLS_DIR}"]

    mcp_tools, routes, error_codes = _load_contracts()
    norm_routes = {_normalize_route(r) for r in routes}
    prefix_routes = [r for r in routes if r.endswith("/")]

    skill_dirs = sorted(p for p in SKILLS_DIR.iterdir() if p.is_dir())
    if not skill_dirs:
        return ["no skills found under skills/"]

    known_skills = {p.name for p in skill_dirs}

    for sdir in skill_dirs:
        name = sdir.name
        if sdir.is_symlink():
            errors.append(f"{name}: skill directory is a symlink (refused)")
            continue
        if not NAME_RE.match(name):
            errors.append(f"{name}: invalid skill directory name")
            continue
        sfile = sdir / "SKILL.md"
        if not sfile.exists():
            errors.append(f"{name}: SKILL.md missing")
            continue
        if sfile.is_symlink():
            errors.append(f"{name}: SKILL.md is a symlink (refused)")
            continue

        size = sfile.stat().st_size
        limit = MAX_ROUTER_BYTES if name == "kubona" else MAX_SKILL_BYTES
        if size > limit:
            errors.append(f"{name}: SKILL.md exceeds size budget ({size} > {limit} bytes)")

        text = sfile.read_text(encoding="utf-8")
        if "dev-docs/" in text:
            errors.append(f"{name}: references scratch path dev-docs/ (forbidden)")

        # Frontmatter validation
        if not text.startswith("---\n"):
            errors.append(f"{name}: missing YAML frontmatter")
            continue
        end = text.find("\n---\n", 4)
        if end < 0:
            errors.append(f"{name}: unclosed YAML frontmatter")
            continue
        fm_lines = [ln.strip() for ln in text[4:end].splitlines() if ln.strip()]
        fm: dict[str, str] = {}
        for ln in fm_lines:
            if ":" not in ln:
                errors.append(f"{name}: invalid frontmatter line {ln!r}")
                continue
            k, v = ln.split(":", 1)
            fm[k.strip()] = v.strip()
        if fm.get("name") != name:
            errors.append(f"{name}: frontmatter name {fm.get('name')!r} != dir name {name!r}")
        desc = fm.get("description", "")
        if not desc or len(desc) > 240:
            errors.append(f"{name}: description missing or > 240 chars ({len(desc)})")

        if name == "kubona":
            for heading in ROUTER_REQUIRED_HEADINGS:
                if heading not in text:
                    errors.append(f"{name}: router skill missing heading {heading!r}")

        # path:line anchors
        for rel_path, line_s in ANCHOR_RE.findall(text):
            target = REPO / rel_path
            if not target.exists():
                errors.append(f"{name}: dead anchor target {rel_path}:{line_s}")
                continue
            total = sum(1 for _ in target.open(encoding="utf-8", errors="replace"))
            line_no = int(line_s)
            if not (1 <= line_no <= total):
                errors.append(f"{name}: anchor line out of range {rel_path}:{line_no} (max {total})")

        # Cross-skill refs
        for ref in SKILL_REF_RE.findall(text):
            if ref not in known_skills:
                errors.append(f"{name}: references unknown skill `{ref}`")

        # HTTP route refs
        for route in API_ROUTE_RE.findall(text):
            if "..." in route:
                continue
            nr = _normalize_route(route)
            if nr not in norm_routes and not any(route.startswith(p) for p in prefix_routes):
                errors.append(f"{name}: references unknown HTTP route `{route}`")

        # Error code table rows
        for code in ERR_TABLE_RE.findall(text):
            if code not in error_codes:
                errors.append(f"{name}: error table cites uncatalogued error code `{code}`")

        # Check explicit MCP tool call patterns (e.g., server/tool)
        for srv_tool in re.findall(r"`([a-z]+)/([a-z][a-z0-9_]+)`", text):
            srv, tname = srv_tool
            if srv in {"compiler", "control", "cost", "execution", "provenance", "science", "world"}:
                if tname not in mcp_tools:
                    errors.append(f"{name}: references unknown MCP tool `{srv}/{tname}`")

    return errors


def main() -> int:
    errs = lint_skills()
    if errs:
        for e in errs:
            print(f"ERROR: {e}", file=sys.stderr)
        print(f"skill-lint: {len(errs)} error(s)", file=sys.stderr)
        return 1
    count = len([p for p in SKILLS_DIR.iterdir() if p.is_dir() and (p / "SKILL.md").exists()])
    print(f"skill-lint: OK ({count} external skills verified against live contracts)")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
