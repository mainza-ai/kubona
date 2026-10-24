#!/usr/bin/env python3
"""QPU provision check (advisory — exit 0 always so keyless CI stays green).

Reads, never values:
  1. Credential presence/shape per known provider key (KUBONA_* only).
     Malformed values fail this check loudly but the script still exits 0;
     the failure that matters (submit-time refusal) already exists.
  2. Pricing provenance per capability JSON: unverified (null) or older
     than 90 days is a WARN, not a failure.

Usage: python3 scripts/qpu-provision-check.py [--strict]
  --strict exits 1 on malformed credentials (provisioning gate for a
  hardware-enabled daemon). Default is advisory.
"""
import csv
import json
import os
import re
import sys
from datetime import date
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
SCHEMA_DIR = ROOT / "platform" / "kubona" / "schema" / "providers"

# provider, key, shape-hint (prefix or pattern; never a value)
KNOWN_KEYS = [
    ("ibm-qiskit", "token", r"^([A-Za-z0-9_\-.]{8,}|eyJ[A-Za-z0-9_\-.]+)$"),
    ("rigetti", "qcs_token", r"^[A-Za-z0-9_\-.]{8,}$"),
    ("quantinuum", "api_key", r"^[A-Za-z0-9_\-.]{8,}$"),
    ("dwave-annealing", "leap_token", r"^[A-Za-z0-9_\-.]{8,}$"),
]

STALE_DAYS = 90
warns: list[str] = []
strict_fail = False


def env_key(provider: str, key: str) -> str:
    p = re.sub(r"[^A-Z0-9]+", "_", provider.upper()).strip("_")
    k = re.sub(r"[^A-Z0-9]+", "_", key.upper()).strip("_")
    return f"KUBONA_{p}_{k}"


def check_credentials() -> None:
    global strict_fail
    for provider, key, shape in KNOWN_KEYS:
        name = env_key(provider, key)
        val = os.environ.get(name, "")
        if not val:
            print(f"ABSENT  {name} ({provider}: credentialless until set)")
            continue
        if not re.match(shape, val):
            print(f"MALFORMED {name} ({provider}: value fails the shape hint — check for truncation/paste errors)")
            warns.append(f"{name} malformed")
            strict_fail = True
        else:
            print(f"PRESENT {name} ({provider}: shape ok, {len(val)} chars — value never shown)")


def check_pricing() -> None:
    global strict_fail
    today = date.today()
    for path in sorted(SCHEMA_DIR.glob("*.json")):
        try:
            d = json.loads(path.read_text())
        except Exception as e:  # noqa: BLE001 — advisory script, report and continue
            print(f"UNREADABLE {path.name}: {e}")
            continue
        p = d.get("pricing", {})
        if p.get("model") == "free":
            continue
        verified = p.get("verified")
        if not verified:
            print(f"UNVERIFIED {d.get('provider')}: no rate verification date/source "
                  f"(zero-rate hardware refuses estimation; fill before billing)")
            warns.append(f"{d.get('provider')} unverified")
            continue
        try:
            age = (today - date.fromisoformat(verified)).days
        except ValueError:
            print(f"BAD-DATE {d.get('provider')}: verified={verified!r} is not ISO")
            warns.append(f"{d.get('provider')} bad date")
            strict_fail = True
            continue
        if age > STALE_DAYS:
            print(f"STALE {d.get('provider')}: verified {verified} ({age}d ago)")
            warns.append(f"{d.get('provider')} stale")


def main() -> int:
    print("== credentials ==")
    check_credentials()
    print("== pricing provenance ==")
    check_pricing()
    if warns:
        print(f"-- {len(warns)} advisory warning(s) --")
        for w in warns:
            print(f"  WARN: {w}")
    else:
        print("-- clean: keys shaped, rates verified --")
    if "--strict" in sys.argv[1:] and strict_fail:
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
