#!/usr/bin/env python3
"""Fail if a known placeholder/result literal returns to production console code.

The audit's Phase-3 regression gate. Tests may assert these literals are absent;
production component/app/lib code may not contain them. Empty states and real
receipts are allowed; a hardcoded illustrative figure is not.
"""
from __future__ import annotations

import sys
from pathlib import Path

ROOTS = [
    Path("platform/console/src/components"),
    Path("platform/console/src/app"),
    Path("platform/console/src/lib"),
]
FORBIDDEN = [
    "14.8x",
    "99.2%",
    "4.2x",
    "4.218",
    "0.042",
    "-1.842",
    "-1.137",
    "12 : 288",
    "98.4",
    "0.00047",
    "4.0x",
    "synthetic demo",
    "UNVERIFIED DRAFT",
    "PROJECTED DRAFT",
]
SKIP_PARTS = {"node_modules", "generated", ".next"}


def production_files() -> list[Path]:
    files: list[Path] = []
    for root in ROOTS:
        if not root.exists():
            continue
        for path in root.rglob("*"):
            if not path.is_file():
                continue
            if path.suffix not in {".ts", ".tsx", ".js", ".jsx", ".css"}:
                continue
            if path.name.endswith((".test.ts", ".test.tsx", ".spec.ts", ".spec.tsx")):
                continue
            if any(part in SKIP_PARTS for part in path.parts):
                continue
            files.append(path)
    return sorted(files)


def main() -> int:
    hits: list[str] = []
    for path in production_files():
        text = path.read_text(encoding="utf-8", errors="replace")
        for literal in FORBIDDEN:
            if literal in text:
                for line_no, line in enumerate(text.splitlines(), 1):
                    if literal in line:
                        hits.append(f"{path}:{line_no}: forbidden placeholder literal {literal!r}")
    if hits:
        print("placeholder literal gate FAILED", file=sys.stderr)
        print("\n".join(hits), file=sys.stderr)
        return 1
    print(f"placeholder literal gate clean ({len(production_files())} production files)")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
