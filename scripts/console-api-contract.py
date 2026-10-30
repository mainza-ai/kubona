#!/usr/bin/env python3
"""Route-level contract check: every console API path must be served by a
registered Go route (or an explicit same-origin Next route).

This is intentionally not a response-schema generator. It catches the most
common drift class cheaply: a frontend path whose backend route was renamed or
removed. Response-shape validation remains a separate, larger migration.
"""

from __future__ import annotations

import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
GO = ROOT / "platform" / "core" / "pkg" / "api"
CONSOLE = ROOT / "platform" / "console" / "src"
NEXT_ROUTES = {"/api/config", "/api/lens-image"}


def backend_routes() -> set[str]:
    out: set[str] = set()
    for p in GO.glob("*.go"):
        if p.name.endswith("_test.go"):
            continue
        text = p.read_text(encoding="utf-8", errors="ignore")
        out.update(re.findall(r's\.route\("([^"]+)"', text))
    return out


def frontend_paths() -> set[str]:
    out: set[str] = set()
    for p in CONSOLE.rglob("*"):
        if not p.is_file() or p.suffix not in {".ts", ".tsx"} or p.name.endswith((".test.ts", ".test.tsx")):
            continue
        text = p.read_text(encoding="utf-8", errors="ignore")
        for m in re.finditer(r'\$\{API_BASE\}(/api/[^`"\s]*)', text):
            raw = m.group(1).split("?")[0]
            raw = re.sub(r'\$\{[^}]*\}?', ':id', raw)
            out.add(raw)
        for m in re.finditer(r'"(/api/[^"]+)"', text):
            out.add(m.group(1).split("?")[0])
    return out


def normalize(path: str) -> str:
    path = re.sub(r"\$\{[^}]*\}?", ":id", path)
    path = re.sub(r"//+", "/", path)
    # `${qs}` appended without a query separator becomes :id; strip it when
    # it is not a real path segment.
    path = re.sub(r"(?<!/):id", "", path)
    return path.rstrip("/") if not path.endswith("/") else path


def matches(path: str, routes: set[str]) -> bool:
    if path in NEXT_ROUTES:
        return True
    for route in routes:
        if path == route:
            return True
        if route.endswith("/") and path.startswith(route):
            return True
    return False


def main() -> int:
    routes = {normalize(r) for r in backend_routes()}
    paths = {normalize(f) for f in frontend_paths()}
    paths = {p for p in paths if p.startswith("/api/")}
    orphans = sorted(p for p in paths if not matches(p, routes))
    if orphans:
        print("console-api-contract: frontend paths with no registered backend route", file=sys.stderr)
        for p in orphans:
            print(f"  {p}", file=sys.stderr)
        return 1
    print(f"console-api-contract: {len(paths)} frontend paths covered")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
