#!/usr/bin/env python3
"""Wiki lint — enforces the Kubona blueprint's structural invariants.

The wiki is a *blueprint*: its value depends on every page being findable, linked,
classifiable, and traceable to a source. These are the checks a human forgets to run
after the twentieth edit, so they run here instead. See AGENTS.md §Page conventions.

Usage:  python3 scripts/wiki-lint.py [--strict]
Exit:   0 clean · 1 findings (with --strict) · 0 findings otherwise (warn-only by default
        so it can be added to CI before the backlog is zero).
"""
from __future__ import annotations

import argparse
import collections
import glob
import os
import re
import sys

VOCAB = {"settled", "adopted", "open-question", "candidate",
         "tracked", "reference", "source", "living"}
SOURCE_KEYS = {"source-architecture-paper", "source-gap-analysis-plan",
               "source-cloudflare-economy", "source-frontier-repo-audit-2026",
               "source-agent-protocol-landscape-2026", "source-openai-math-corpus",
               "source-leanrea-investigation-2026",
               "source-llm-circuit-codesign-2026",
               "source-magic-state-cultivation-2026",
               "source-world-labs-atlas-2026"}
META_PAGES = {"index", "log"}
REQUIRED = ("type", "tags", "date", "updated", "status")
WIKI = os.path.join(os.path.dirname(os.path.dirname(os.path.abspath(__file__))), "wiki")

FM = re.compile(r"^---\n(.*?)\n---\n", re.S)


def check(pages: list[str]) -> dict[str, list[str]]:
    findings: dict[str, list[str]] = collections.defaultdict(list)
    basenames = {os.path.basename(p)[:-3] for p in pages}
    index = open(os.path.join(WIKI, "index.md"), encoding="utf-8").read()
    questions = open(os.path.join(WIKI, "questions.md"), encoding="utf-8").read()

    for path in pages:
        rel = os.path.relpath(path, WIKI)
        stem = os.path.basename(path)[:-3]
        text = open(path, encoding="utf-8").read()
        fm = FM.match(text)
        if not fm:
            findings["missing frontmatter"].append(rel)
            continue
        head = fm.group(1)

        def field(name: str) -> str | None:
            m = re.search(rf"^{name}:\s*(.*)$", head, re.M)
            return m.group(1).strip() if m else None

        for f in REQUIRED:
            if field(f) is None:
                findings["missing required frontmatter field"].append(f"{rel}: {f}")

        # source pages are exempt from `sources:` but must say where they came from
        if field("type") != "source" and stem not in META_PAGES and field("sources") is None:
            findings["missing required frontmatter field"].append(f"{rel}: sources")
        if field("type") == "source" and not (field("raw") or field("origin")):
            findings["source page without raw:/origin: provenance"].append(rel)

        status = field("status")
        if status and status not in VOCAB:
            findings["status outside controlled vocabulary"].append(f"{rel}: {status}")

        for key in re.findall(r"^sources:\s*\[(.*)\]$", head, re.M):
            for token in (t.strip() for t in key.split(",")):
                if token and token not in SOURCE_KEYS:
                    findings["non-canonical source key"].append(f"{rel}: {token}")

        for target in re.findall(r"\[\[([^\]|#]+)", text):
            if target.strip() not in basenames:
                findings["dangling wikilink"].append(f"{rel} -> {target.strip()}")

        if stem not in META_PAGES and stem not in index and stem not in questions:
            findings["page absent from index.md and questions.md"].append(rel)

        # Q-32: every open-question page must have a row in questions.md.
        # Presence in index.md is not enough — the work-list is questions.md.
        # (Substring match is deliberately lenient: a [[link]] reference
        # counts. What it catches is pages with zero mention at all.)
        if status == "open-question" and stem not in questions:
            findings["open-question page without a questions.md row"].append(rel)

        # The invariant that matters: a `path:line` anchor must resolve against the live GitHub
        # URL, never a `dev-docs/` scratch clone (deleted when development ends). Historical prose
        # *about* dev-docs is fine; an anchor pointing into it is not. log.md is an append-only
        # record and is exempt.
        if stem != "log" and rel.startswith(("concepts/", "entities/")):
            for anchor in re.findall(r"dev-docs/[^\s`|)]*:\d+", text):
                findings["path:line anchor resolving into dev-docs/ scratch"].append(f"{rel}: {anchor}")

    return findings


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--strict", action="store_true", help="exit 1 on any finding")
    args = ap.parse_args()

    pages = sorted(glob.glob(os.path.join(WIKI, "**", "*.md"), recursive=True))
    census = collections.Counter(
        re.search(r"^status:\s*(.+)$", FM.match(open(p, encoding="utf-8").read()).group(1), re.M)
        .group(1).strip()
        for p in pages if FM.match(open(p, encoding="utf-8").read()))

    print(f"wiki lint · {len(pages)} pages")
    print("status census: " + " · ".join(f"{k}={v}" for k, v in sorted(census.items())))
    findings = check(pages)
    if not findings:
        print("\n\u2705 clean: frontmatter, status vocabulary, source keys, wikilinks, index coverage, dev-docs discipline")
        return 0
    total = 0
    for kind, items in sorted(findings.items()):
        total += len(items)
        print(f"\n{kind} ({len(items)}):")
        for i in items[:15]:
            print(f"   {i}")
        if len(items) > 15:
            print(f"   … {len(items) - 15} more")
    print(f"\n{total} finding(s)")
    return 1 if args.strict else 0


if __name__ == "__main__":
    sys.exit(main())
