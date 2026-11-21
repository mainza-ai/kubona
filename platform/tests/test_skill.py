"""L1 skill pins: all external skills under skills/*/SKILL.md stay lean,
carry valid frontmatter and anchors, and match Kubona's live MCP, OpenAPI,
and error-catalog contracts (via scripts/skill-lint.py)."""

import importlib.util
from pathlib import Path

REPO = Path(__file__).resolve().parents[2]
SKILLS_DIR = REPO / "skills"
ROUTER_SKILL = SKILLS_DIR / "kubona" / "SKILL.md"

EXPECTED_SKILLS = {
    "kubona",
    "kubona-connect",
    "kubona-experiments",
    "kubona-governance",
    "kubona-research",
    "kubona-ops",
}


def _load_linter():
    spec = importlib.util.spec_from_file_location("skill_lint", REPO / "scripts" / "skill-lint.py")
    assert spec and spec.loader
    mod = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(mod)
    return mod


def test_expected_skill_suite_present():
    present = {p.name for p in SKILLS_DIR.iterdir() if p.is_dir() and (p / "SKILL.md").exists()}
    missing = EXPECTED_SKILLS - present
    assert not missing, f"missing expected external skills: {sorted(missing)}"


def test_router_skill_lean():
    assert ROUTER_SKILL.exists()
    assert ROUTER_SKILL.stat().st_size <= 6 * 1024


def test_skill_lint_clean():
    linter = _load_linter()
    errors = linter.lint_skills()
    assert not errors, "skill-lint failures:\n" + "\n".join(errors)
