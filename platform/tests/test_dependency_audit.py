"""The dependency gate refuses a bad waiver and a lost registry.

No network. The scanners themselves are the CI job.
"""
from __future__ import annotations

import datetime as dt
import importlib.util
import json
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
SCRIPT = ROOT / "scripts" / "dependency_audit.py"


def load():
    spec = importlib.util.spec_from_file_location("dependency_audit", SCRIPT)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


audit = load()
TODAY = dt.date(2026, 11, 12)


def row(**overrides):
    base = {
        "id": "GO-2026-6809",
        "ecosystem": "go",
        "owner": "lab-operator",
        "expires": "2027-03-01",
        "reason": "The fixed Go release is not in the image yet; this waiver ends with that bump.",
    }
    base.update(overrides)
    return base


def test_empty_exceptions_file_is_valid():
    problems = audit.validate_exceptions([], TODAY)
    assert problems == []


def test_expiry_beyond_120_days_is_refused():
    problems = audit.validate_exceptions([row(expires="2027-03-13")], TODAY)
    assert any("more than 120 days" in problem for problem in problems)


def test_expiry_on_the_120th_day_is_allowed():
    problems = audit.validate_exceptions([row(expires="2027-03-12")], TODAY)
    assert problems == []


def test_expired_waiver_is_refused():
    problems = audit.validate_exceptions([row(expires="2026-11-11")], TODAY)
    assert any("expired" in problem for problem in problems)


def test_placeholder_owner_and_short_reason_are_refused():
    problems = audit.validate_exceptions([row(owner="todo", reason="later")])
    assert any("placeholder" in problem for problem in problems)
    assert any("reason" in problem for problem in problems)


def test_reachable_go_advisory_is_kept_and_an_uncalled_one_is_not():
    report = """
    {"finding": {"osv": "GO-2026-6809", "trace": [
      {"module": "stdlib"}, {"module": "github.com/mainza-ai/kubona/core", "function": "Serve"}]}}
    {"finding": {"osv": "GO-2026-5932", "trace": [{"module": "golang.org/x/crypto"}]}}
    """
    found = audit.reachable_go_advisories(report, "github.com/mainza-ai/kubona/core")
    assert found == {"GO-2026-6809"}


def test_pnpm_count_without_an_id_is_a_failure():
    report = json.dumps({
        "advisories": {},
        "metadata": {"vulnerabilities": {"info": 0, "low": 0, "moderate": 1, "high": 0, "critical": 0}},
    })
    try:
        audit.npm_advisories(report)
    except audit.AuditFailure as exc:
        assert "no advisory id" in str(exc)
    else:
        raise AssertionError("a count without an id was treated as clean")


def test_unused_exception_is_not_a_silent_waiver():
    waived = {"GO-2026-6809": row()}
    keep, unused = audit.unwaived(set(), waived)
    assert keep == []
    assert unused == ["GO-2026-6809"]


def test_checked_in_exceptions_file_matches_the_validator():
    rows = audit.load_exceptions()
    assert audit.validate_exceptions(rows) == []
