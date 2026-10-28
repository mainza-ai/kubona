"""Refresh one device's calibration from a live provider snapshot.

The snapshot comes from the provider plugin (e.g. IBM backend properties).
This tool validates it, stamps provenance, and atomically updates
platform/contracts/fidelity-devices.json. Dry-run is the default.

A missing/unmeasurable snapshot refuses: an unchanged timestamp would keep
stale calibration looking fresh, which is exactly the failure this prevents.
"""

from __future__ import annotations

import argparse
import json
import os
import sys
from datetime import datetime, timezone
from pathlib import Path
from typing import Any

CONTRACT = Path(__file__).resolve().parents[2] / "contracts" / "fidelity-devices.json"
_REQUIRED = ("t1_us", "t2_us", "single_qubit_error", "two_qubit_error", "readout_error")


class CalibrationError(ValueError):
    pass


def _number(snapshot: dict[str, Any], key: str, *, allow_zero: bool = False) -> float:
    value = snapshot.get(key)
    if isinstance(value, bool) or not isinstance(value, (int, float)) or value < 0 or (not allow_zero and value == 0):
        raise CalibrationError(f"{key} must be a measured {'non-negative' if allow_zero else 'positive'} number")
    return float(value)


def apply_snapshot(device: str, snapshot: dict[str, Any] | None, *,
                   contract: Path | None = None, write: bool = False,
                   now: datetime | None = None) -> dict[str, Any]:
    if not snapshot:
        raise CalibrationError("provider returned no calibration snapshot (unmeasurable)")
    values = {k: _number(snapshot, k, allow_zero=k.endswith("_error")) for k in _REQUIRED}
    if values["t2_us"] > 2.0 * values["t1_us"] + 1e-6:
        raise CalibrationError("physically unsound calibration: t2 > 2*t1")
    path = contract or CONTRACT
    doc = json.loads(path.read_text(encoding="utf-8"))
    target = next((d for d in doc.get("devices", []) if d.get("name") == device), None)
    if target is None:
        raise CalibrationError(f"unknown device {device!r}")
    stamp = (now or datetime.now(timezone.utc)).astimezone(timezone.utc).replace(microsecond=0).isoformat()
    target.update(values)
    target["calibrated_at"] = stamp
    target["version"] = "live-" + stamp
    target["source"] = str(snapshot.get("source") or "provider calibration snapshot")
    if write:
        tmp = path.with_suffix(".json.tmp")
        with open(tmp, "w", encoding="utf-8") as fh:
            fh.write(json.dumps(doc, indent=2) + "\n")
            fh.flush()
            os.fsync(fh.fileno())
        os.replace(tmp, path)
    return target


def _snapshot_from_provider(provider: str) -> dict[str, Any] | None:
    from kubona.providers.base import get_provider
    return get_provider(provider).calibration_snapshot()


def main(argv: list[str] | None = None) -> int:
    ap = argparse.ArgumentParser(description="Refresh device calibration from a live provider")
    ap.add_argument("--device", required=True)
    ap.add_argument("--provider", help="read live calibration from this provider plugin")
    ap.add_argument("--input", help="read a snapshot JSON file instead of a live provider")
    ap.add_argument("--write", action="store_true", help="update the contract (default: dry run)")
    args = ap.parse_args(argv)
    try:
        if args.input:
            snapshot = json.loads(Path(args.input).read_text(encoding="utf-8"))
        elif args.provider:
            snapshot = _snapshot_from_provider(args.provider)
        else:
            raise CalibrationError("one of --provider or --input is required")
        target = apply_snapshot(args.device, snapshot, write=args.write)
    except (CalibrationError, OSError, json.JSONDecodeError) as exc:
        print(f"refused: {exc}", file=sys.stderr)
        return 2
    print(json.dumps({"device": args.device, "calibration": target, "written": args.write}, indent=2))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
