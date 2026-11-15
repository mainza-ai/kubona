#!/usr/bin/env python3
"""Dependency vulnerability gate.

One command for four ecosystems. A missing tool, an unreadable report, or a
registry that cannot be fetched is a failure — never a clean scan. An
exception suppresses one advisory only when it names the ecosystem, an
owner, a reason, and an expiry no more than 120 days out.

  python3 scripts/dependency_audit.py --check-exceptions
  python3 scripts/dependency_audit.py --audit go|python|npm|rust
"""
from __future__ import annotations

import argparse
import datetime as dt
import json
import re
import shutil
import subprocess
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
EXCEPTIONS_PATH = ROOT / "platform" / "contracts" / "dependency-audit-exceptions.json"
CORE = ROOT / "platform" / "core"
CONSOLE = ROOT / "platform" / "console"
CRATES = ROOT / "platform" / "crates"

MAX_DAYS = 120
ECOSYSTEMS = ("go", "python", "npm", "rust")
OWNERS_REFUSED = {"todo", "tbd", "unknown", "n/a", "none", "xxx", "fixme", "owner"}
ID_RE = re.compile(
    r"^(?:GO-\d{4}-\d+|CVE-\d{4}-\d+|GHSA-[a-z0-9]{4}-[a-z0-9]{4}-[a-z0-9]{4}"
    r"|PYSEC-\d{4}-\d+|RUSTSEC-\d{4}-\d+)$"
)
ENTRY_KEYS = {"id", "ecosystem", "owner", "expires", "reason"}


class AuditFailure(Exception):
    """The gate cannot honestly say the tree is clean."""


def today_utc() -> dt.date:
    return dt.datetime.now(dt.timezone.utc).date()


def load_exceptions(path: Path = EXCEPTIONS_PATH) -> list[dict]:
    try:
        raw = json.loads(path.read_text(encoding="utf-8"))
    except FileNotFoundError as exc:
        raise AuditFailure(f"missing exceptions file: {path}") from exc
    except json.JSONDecodeError as exc:
        raise AuditFailure(f"exceptions file is not JSON: {exc}") from exc
    if not isinstance(raw, dict) or set(raw) != {"exceptions"}:
        raise AuditFailure("exceptions file must contain only an 'exceptions' array")
    rows = raw["exceptions"]
    if not isinstance(rows, list):
        raise AuditFailure("'exceptions' must be an array")
    return rows


def validate_exceptions(rows: list[dict], today: dt.date | None = None) -> list[str]:
    """Return human-readable problems. An empty list means the file is valid."""
    today = today or today_utc()
    problems: list[str] = []
    seen: set[tuple[str, str]] = set()
    for index, row in enumerate(rows):
        where = f"exceptions[{index}]"
        if not isinstance(row, dict) or set(row) != ENTRY_KEYS:
            problems.append(f"{where} must have exactly id, ecosystem, owner, expires, reason")
            continue
        advisory = str(row["id"]).strip()
        ecosystem = str(row["ecosystem"]).strip()
        owner = str(row["owner"]).strip()
        reason = str(row["reason"]).strip()
        expires_raw = str(row["expires"]).strip()
        if not ID_RE.match(advisory):
            problems.append(f"{where} id {advisory!r} is not a GO/CVE/GHSA/PYSEC/RUSTSEC id")
        if ecosystem not in ECOSYSTEMS:
            problems.append(f"{where} ecosystem {ecosystem!r} is not one of {', '.join(ECOSYSTEMS)}")
        if len(owner) < 2 or owner.casefold() in OWNERS_REFUSED:
            problems.append(f"{where} owner must name a person, not a placeholder")
        if len(reason) < 40:
            problems.append(f"{where} reason must say why this cannot be fixed yet (at least 40 characters)")
        try:
            expires = dt.date.fromisoformat(expires_raw)
        except ValueError:
            problems.append(f"{where} expires must be YYYY-MM-DD")
            continue
        age = (expires - today).days
        if age < 0:
            problems.append(f"{where} expired on {expires.isoformat()}")
        elif age > MAX_DAYS:
            problems.append(f"{where} expires {expires.isoformat()} is more than {MAX_DAYS} days out")
        key = (ecosystem, advisory)
        if key in seen:
            problems.append(f"{where} repeats {ecosystem} {advisory}")
        seen.add(key)
    return problems


def active_exceptions(rows: list[dict], ecosystem: str, today: dt.date) -> dict[str, dict]:
    out: dict[str, dict] = {}
    for row in rows:
        if row["ecosystem"] != ecosystem:
            continue
        expires = dt.date.fromisoformat(row["expires"])
        if (expires - today).days < 0:
            continue
        out[row["id"]] = row
    return out


def unwaived(found: set[str], waived: dict[str, dict]) -> tuple[list[str], list[str]]:
    keep = sorted(advisory for advisory in found if advisory not in waived)
    unused = sorted(advisory for advisory in waived if advisory not in found)
    return keep, unused


def decode_json_stream(text: str) -> list[dict]:
    decoder = json.JSONDecoder()
    values: list[dict] = []
    index = 0
    length = len(text)
    while index < length:
        while index < length and text[index].isspace():
            index += 1
        if index >= length:
            break
        try:
            value, index = decoder.raw_decode(text, index)
        except json.JSONDecodeError as exc:
            raise AuditFailure(f"tool report is not JSON: {exc}") from exc
        if not isinstance(value, dict):
            raise AuditFailure("tool report contained a non-object JSON value")
        values.append(value)
    return values


def reachable_go_advisories(report: str, module: str) -> set[str]:
    """Advisories whose trace reaches this module. Imported-but-uncalled stay out."""
    found: set[str] = set()
    for value in decode_json_stream(report):
        finding = value.get("finding")
        if not isinstance(finding, dict):
            continue
        advisory = finding.get("osv")
        trace = finding.get("trace") or []
        if not isinstance(advisory, str) or not isinstance(trace, list):
            raise AuditFailure("govulncheck finding is missing osv or trace")
        if any(isinstance(frame, dict) and frame.get("module") == module for frame in trace):
            found.add(advisory)
    return found


def python_advisories(report: str) -> set[str]:
    try:
        payload = json.loads(report)
    except json.JSONDecodeError as exc:
        raise AuditFailure(f"pip-audit report is not JSON: {exc}") from exc
    deps = payload.get("dependencies")
    if not isinstance(deps, list):
        raise AuditFailure("pip-audit report has no dependencies array")
    found: set[str] = set()
    for dep in deps:
        if not isinstance(dep, dict):
            raise AuditFailure("pip-audit dependency is not an object")
        for vuln in dep.get("vulns") or []:
            if not isinstance(vuln, dict) or not isinstance(vuln.get("id"), str):
                raise AuditFailure("pip-audit vulnerability is missing an id")
            found.add(vuln["id"])
    return found


def npm_advisories(report: str) -> set[str]:
    try:
        payload = json.loads(report)
    except json.JSONDecodeError as exc:
        raise AuditFailure(f"pnpm audit report is not JSON: {exc}") from exc
    advisories = payload.get("advisories") or {}
    if not isinstance(advisories, dict):
        raise AuditFailure("pnpm audit advisories is not an object")
    found: set[str] = set()
    for key, advisory in advisories.items():
        if isinstance(advisory, dict):
            ghsa = advisory.get("github_advisory_id")
            cves = advisory.get("cves") or []
            if isinstance(ghsa, str) and ghsa:
                found.add(ghsa)
            elif isinstance(cves, list) and cves and isinstance(cves[0], str):
                found.add(cves[0])
            else:
                raise AuditFailure(f"pnpm advisory {key} has no GHSA or CVE id")
        else:
            raise AuditFailure(f"pnpm advisory {key} is not an object")
    counts = (payload.get("metadata") or {}).get("vulnerabilities") or {}
    total = 0
    if isinstance(counts, dict):
        for name in ("info", "low", "moderate", "high", "critical"):
            value = counts.get(name, 0) or 0
            if not isinstance(value, int):
                raise AuditFailure("pnpm vulnerability count is not an integer")
            total += value
    if total and not found:
        raise AuditFailure("pnpm reported vulnerabilities but no advisory id was parsed")
    return found


def rust_advisories(report: str) -> set[str]:
    found = set(re.findall(r"RUSTSEC-\d{4}-\d+", report))
    if not report.strip():
        return found
    # A successful empty report is `{`…`advisories: []` or diagnostic text
    # with no ids. A non-JSON failure still counts ids if they were printed,
    # and the caller treats a non-zero exit with no ids as a tool failure.
    return found


def go_module(core: Path) -> str:
    proc = subprocess.run(
        ["go", "list", "-m"], cwd=core, text=True, capture_output=True, check=False,
    )
    if proc.returncode != 0 or not proc.stdout.strip():
        raise AuditFailure(f"go list -m failed: {proc.stderr.strip() or proc.stdout.strip()}")
    return proc.stdout.strip().splitlines()[0].strip()


def run_command(argv: list[str], cwd: Path) -> subprocess.CompletedProcess[str]:
    if shutil.which(argv[0]) is None:
        raise AuditFailure(f"{argv[0]} is not installed; refusing to treat the ecosystem as clean")
    return subprocess.run(argv, cwd=cwd, text=True, capture_output=True, check=False)


def require_clean(ecosystem: str, found: set[str], rows: list[dict], today: dt.date) -> None:
    waived = active_exceptions(rows, ecosystem, today)
    keep, unused = unwaived(found, waived)
    if unused:
        raise AuditFailure(
            f"{ecosystem}: exception does not match a current advisory (remove it): {', '.join(unused)}"
        )
    if keep:
        raise AuditFailure(f"{ecosystem}: unexcepted advisories: {', '.join(keep)}")
    print(f"{ecosystem}: clean ({len(waived)} active exception(s))")


def audit_go(rows: list[dict], today: dt.date) -> None:
    module = go_module(CORE)
    proc = run_command(["govulncheck", "-json", "./..."], CORE)
    if proc.returncode not in (0, 3):
        raise AuditFailure(
            "govulncheck failed before it could report advisories "
            f"(exit {proc.returncode}): {(proc.stderr or proc.stdout).strip()[:500]}"
        )
    found = reachable_go_advisories(proc.stdout, module)
    if proc.returncode == 3 and not found:
        raise AuditFailure("govulncheck exit 3 but no reachable advisory was parsed")
    if proc.returncode == 0 and found:
        raise AuditFailure(f"govulncheck exit 0 disagreed with the report: {', '.join(sorted(found))}")
    require_clean("go", found, rows, today)


def audit_python(rows: list[dict], today: dt.date) -> None:
    proc = run_command(
        ["pip-audit", "--format", "json", "--progress-spinner", "off", "--local"],
        ROOT / "platform",
    )
    if proc.returncode not in (0, 1) or not proc.stdout.strip():
        raise AuditFailure(
            "pip-audit failed before it could report advisories "
            f"(exit {proc.returncode}): {(proc.stderr or proc.stdout).strip()[:500]}"
        )
    found = python_advisories(proc.stdout)
    if proc.returncode == 1 and not found:
        raise AuditFailure("pip-audit exit 1 but no advisory was parsed")
    if proc.returncode == 0 and found:
        raise AuditFailure(f"pip-audit exit 0 disagreed with the report: {', '.join(sorted(found))}")
    require_clean("python", found, rows, today)


def audit_npm(rows: list[dict], today: dt.date) -> None:
    proc = run_command(["pnpm", "audit", "--json"], CONSOLE)
    # pnpm prints the JSON report on stdout for both clean and vulnerable trees.
    if not proc.stdout.strip():
        raise AuditFailure(
            "pnpm audit produced no report "
            f"(exit {proc.returncode}): {proc.stderr.strip()[:500]}"
        )
    found = npm_advisories(proc.stdout)
    if proc.returncode not in (0, 1):
        raise AuditFailure(
            f"pnpm audit failed (exit {proc.returncode}): {proc.stderr.strip()[:500]}"
        )
    if proc.returncode == 1 and not found:
        raise AuditFailure("pnpm audit exit 1 but no advisory was parsed")
    if proc.returncode == 0 and found:
        raise AuditFailure(f"pnpm audit exit 0 disagreed with the report: {', '.join(sorted(found))}")
    require_clean("npm", found, rows, today)


def audit_rust(rows: list[dict], today: dt.date) -> None:
    if shutil.which("cargo-deny") is None and shutil.which("cargo") is None:
        raise AuditFailure("cargo-deny is not installed; refusing to treat Rust as clean")
    locks = sorted(CRATES.glob("*/Cargo.lock"))
    if not locks:
        raise AuditFailure(f"no Cargo.lock under {CRATES}")
    found: set[str] = set()
    for lock in locks:
        manifest = lock.parent / "Cargo.toml"
        proc = run_command(
            [
                "cargo-deny",
                "--manifest-path", str(manifest),
                "check",
                "--config", str(CRATES / "deny.toml"),
                "advisories",
            ],
            CRATES,
        )
        report = (proc.stdout or "") + "\n" + (proc.stderr or "")
        ids = rust_advisories(report)
        if proc.returncode != 0 and not ids:
            raise AuditFailure(
                f"cargo-deny failed for {manifest.parent.name} before reporting an advisory "
                f"(exit {proc.returncode}): {report.strip()[:500]}"
            )
        found.update(ids)
    require_clean("rust", found, rows, today)


AUDITS = {"go": audit_go, "python": audit_python, "npm": audit_npm, "rust": audit_rust}


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description="Kubona dependency vulnerability gate")
    parser.add_argument("--check-exceptions", action="store_true")
    parser.add_argument("--audit", choices=ECOSYSTEMS)
    args = parser.parse_args(argv)
    if not args.check_exceptions and not args.audit:
        parser.error("pass --check-exceptions and/or --audit")
    try:
        rows = load_exceptions()
        problems = validate_exceptions(rows)
        if problems:
            raise AuditFailure("exceptions file refused:\n- " + "\n- ".join(problems))
        if args.check_exceptions and not args.audit:
            print(f"exceptions: {len(rows)} row(s), none expired or over {MAX_DAYS} days")
        if args.audit:
            AUDITS[args.audit](rows, today_utc())
    except AuditFailure as exc:
        print(f"dependency-audit: {exc}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
