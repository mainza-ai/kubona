"""Every paper-scale slice file must run in some CI workflow.

The fast lane ignores all `test_codesign_slice*.py` files. If a new slice
does not match a slow-shard or nightly job it is silently never run. This
guard parses every workflow (no duplicated shard list) and fails when any
slice file is uncovered.
"""

from __future__ import annotations

import fnmatch
import re
from pathlib import Path

import yaml

REPO = Path(__file__).resolve().parents[2]
WORKFLOWS = sorted((REPO / ".github" / "workflows").glob("*.yml"))
TESTS = Path(__file__).resolve().parent


def _slow_shard_patterns() -> list[str]:
    """Slice tokens from every workflow, excluding the fast-lane ignore."""
    patterns: list[str] = []
    for wf in WORKFLOWS:
        doc = yaml.safe_load(wf.read_text()) or {}
        for job in doc.get("jobs", {}).values():
            for step in job.get("steps", []):
                run = step.get("run", "")
                if "--ignore-glob" in run:
                    continue
                for token in re.findall(r"tests/test_codesign_slice[^\s\"']+", run):
                    patterns.append(token.split("::", 1)[0])
    return patterns


def test_every_slice_file_rides_a_slow_shard():
    slices = sorted(p.name for p in TESTS.glob("test_codesign_slice*.py"))
    assert slices, "no slice files found (test discovery broke)"
    patterns = _slow_shard_patterns()
    assert patterns, "no workflow runs paper-scale slice files"
    missing = [
        name for name in slices
        if not any(fnmatch.fnmatch(f"tests/{name}", pat) for pat in patterns)
    ]
    assert not missing, (
        "slice files match no python-slow shard (they are ignored by the fast "
        f"lane and will never run in CI): {missing}"
    )
