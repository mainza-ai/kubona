"""L1 skill pins: the skill exists, stays lean, keeps its sections, and
every path:line anchor resolves into the tree (dead traps fail here)."""

import re
from pathlib import Path

REPO = Path(__file__).resolve().parents[2]
SKILL = REPO / "skills" / "kubona" / "SKILL.md"

REQUIRED_HEADINGS = ["## Rules", "## Router", "## First move", "## Speed patterns", "## Traps"]
MAX_BYTES = 12 * 1024

ANCHOR_RE = re.compile(
    r"`((?:platform|bench|skills|scripts)/[\w\-/]*\.(?:py|go|json|md|sh|yaml|toml)):(\d+)`"
)


def _text():
    assert SKILL.exists(), f"skill missing at {SKILL}"
    return SKILL.read_text()


def test_skill_exists_and_lean():
    assert SKILL.exists()
    size = SKILL.stat().st_size
    assert size <= MAX_BYTES, f"skill grew past leanness budget: {size} > {MAX_BYTES}"


def test_skill_sections():
    text = _text()
    for heading in REQUIRED_HEADINGS:
        assert heading in text, f"skill missing section {heading}"


def test_skill_anchors_resolve():
    text = _text()
    anchors = ANCHOR_RE.findall(text)
    assert anchors, "skill carries no verifiable anchors"
    for path, line in anchors:
        target = REPO / path
        assert target.exists(), f"anchor target missing: {path}"
        total = sum(1 for _ in target.open())
        assert 1 <= int(line) <= total, f"anchor line out of range: {path}:{line}"
