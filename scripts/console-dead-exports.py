#!/usr/bin/env python3
"""Fail on new unreferenced function exports from the console API client.

The 2026-10-29 audit found 14 exports in `platform/console/src/lib/api.ts`
with no consumer. Some are deliberately deferred features; the rest are
debt. This check makes the distinction explicit: a new dead export fails
CI, and an allowlisted entry that becomes used fails too, so the list
cannot rot.

Scope: `export function` / `export async function` declarations. It does
not try to prove `export const` values dead; those are mostly shared
constants with a different usage shape.
"""

from __future__ import annotations

import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
API = ROOT / "platform" / "console" / "src" / "lib" / "api.ts"
SRC = ROOT / "platform" / "console" / "src"

# Every entry needs a reason and a decision owner. Remove an entry when the
# export is wired or deleted; the check fails on a stale allowlist entry.
ALLOWED = {
    "resumeTribunal": "tribunal resume UI not mounted; runTribunal is the live path",
    "evaluateDiscoveryPipeline": "discovery curation UI deferred",
    "fetchAutoresearchHarnesses": "autoresearch operator UI deferred (remaining-work)",
    "startAutoresearchLoop": "autoresearch operator UI deferred (remaining-work)",
    "fetchAutoresearchLoops": "autoresearch operator UI deferred (remaining-work)",
    "stepAutoresearchLoop": "autoresearch operator UI deferred (remaining-work)",
    "pauseAutoresearchLoop": "autoresearch operator UI deferred (remaining-work)",
    "resumeAutoresearchLoop": "autoresearch operator UI deferred (remaining-work)",
    "stopAutoresearchLoop": "autoresearch operator UI deferred (remaining-work)",
    "planWorkflow": "workflow console UI pending; Go endpoints are agent-facing",
    "runWorkflow": "workflow console UI pending; Go endpoints are agent-facing",
    "requestServerSignedMandate": "server-signed mandate console path declined; client-mint is the console path",
    "resetDedupForTests": "test-only helper for inflight GET coalescing",
}


def exports(text: str) -> list[str]:
    return re.findall(r"^export\s+(?:async\s+)?function\s+(\w+)", text, re.M)


def main() -> int:
    if not API.exists():
        print(f"console-dead-exports: missing {API.relative_to(ROOT)}", file=sys.stderr)
        return 2
    api_text = API.read_text(encoding="utf-8")
    names = exports(api_text)
    sources = [
        p
        for p in SRC.rglob("*")
        if p.suffix in {".ts", ".tsx"} and not p.name.endswith((".test.ts", ".test.tsx"))
    ]
    texts = {p: p.read_text(encoding="utf-8") for p in sources}
    dead: list[str] = []
    for name in names:
        used = False
        pat = re.compile(r"^export\s+(?:async\s+)?function\s+" + re.escape(name) + r"\b", re.M)
        ref = re.compile(r"\b" + re.escape(name) + r"\b")
        for text in texts.values():
            if ref.search(pat.sub("", text)):
                used = True
                break
        if not used:
            dead.append(name)

    new = [n for n in dead if n not in ALLOWED]
    stale = [n for n in ALLOWED if n not in dead]
    if new:
        print("console-dead-exports: new unreferenced exports", file=sys.stderr)
        for name in new:
            print(f"  {name}: wire it, delete it, or add a dated reason to ALLOWED", file=sys.stderr)
    if stale:
        print("console-dead-exports: stale allowlist entries (now referenced; remove them)", file=sys.stderr)
        for name in stale:
            print(f"  {name}", file=sys.stderr)
    if new or stale:
        return 1
    print(f"console-dead-exports: {len(dead)} known deferred, 0 new")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
