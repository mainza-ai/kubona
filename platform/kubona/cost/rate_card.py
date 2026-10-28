"""Validated, atomic provider rate-card refresh.

Vendor pricing is not exposed as a stable machine API, so the platform does
not scrape pricing pages. An operator supplies the verified numbers (date +
source URL) and this tool validates and applies them to the provider schema.
Dry-run is the default; ``--write`` is explicit.

The zero-rate refusal in ``ProviderCapability.cost_estimate`` is unchanged:
a card with zero hardware rates is rejected here rather than saved.
"""

from __future__ import annotations

import argparse
import json
import os
import sys
from datetime import date
from pathlib import Path
from typing import Any

SCHEMA_DIR = Path(__file__).resolve().parents[1] / "schema" / "providers"
_MODELS = {"per_shot", "per_task_plus_per_shot", "per_second", "token_consumption", "free"}


class RateCardError(ValueError):
    pass


def validate_rate_card(provider: str, card: dict[str, Any]) -> dict[str, Any]:
    """Validate one verified card and return the normalized pricing object."""
    if not isinstance(card, dict):
        raise RateCardError("rate card must be a JSON object")
    model = card.get("model")
    if model not in _MODELS:
        raise RateCardError(f"unknown pricing model {model!r}")
    if card.get("currency", "USD") != "USD":
        raise RateCardError("only USD rate cards are supported")
    for key in ("per_shot_usd", "per_task_usd", "per_second_usd"):
        value = card.get(key, 0.0)
        if isinstance(value, bool) or not isinstance(value, (int, float)) or value < 0:
            raise RateCardError(f"{key} must be a non-negative number")
    if model == "per_shot" and float(card.get("per_shot_usd", 0)) <= 0:
        raise RateCardError("per_shot card must carry a positive per_shot_usd")
    if model == "per_task_plus_per_shot" and float(card.get("per_task_usd", 0)) <= 0 and float(card.get("per_shot_usd", 0)) <= 0:
        raise RateCardError("per_task_plus_per_shot card must carry a positive rate")
    if model == "per_second" and float(card.get("per_second_usd", 0)) <= 0:
        raise RateCardError("per_second card must carry a positive per_second_usd")
    verified = str(card.get("verified", "")).strip()
    try:
        date.fromisoformat(verified)
    except ValueError as exc:
        raise RateCardError("verified must be an ISO date (YYYY-MM-DD)") from exc
    source = str(card.get("source", "")).strip()
    if not source.startswith(("https://", "http://")):
        raise RateCardError("source must be the vendor pricing URL")
    notes = str(card.get("notes", "")).strip()
    if not notes:
        raise RateCardError("notes must state the plan/billing basis")
    out = dict(card)
    out["model"] = model
    out["currency"] = "USD"
    out["per_shot_usd"] = float(card.get("per_shot_usd", 0.0))
    out["per_task_usd"] = float(card.get("per_task_usd", 0.0))
    out["per_second_usd"] = float(card.get("per_second_usd", 0.0))
    out["verified"] = verified
    out["source"] = source
    out["notes"] = notes
    return out


def apply_rate_card(provider: str, card: dict[str, Any], *, write: bool = False,
                    schema_dir: Path | None = None) -> dict[str, Any]:
    """Merge a validated card into the provider schema; write atomically."""
    root = schema_dir or SCHEMA_DIR
    path = root / f"{provider}.json"
    if not path.exists():
        raise RateCardError(f"unknown provider schema {path}")
    doc = json.loads(path.read_text(encoding="utf-8"))
    if doc.get("provider") != provider:
        raise RateCardError(f"schema provider mismatch: {doc.get('provider')!r}")
    pricing = validate_rate_card(provider, card)
    doc["pricing"] = pricing
    if write:
        tmp = path.with_suffix(".json.tmp")
        with open(tmp, "w", encoding="utf-8") as fh:
            fh.write(json.dumps(doc, indent=2) + "\n")
            fh.flush()
            os.fsync(fh.fileno())
        os.replace(tmp, path)
    return doc


def main(argv: list[str] | None = None) -> int:
    ap = argparse.ArgumentParser(description="Validate/apply a verified provider rate card")
    ap.add_argument("--provider", required=True)
    ap.add_argument("--input", required=True, help="rate-card JSON file")
    ap.add_argument("--write", action="store_true", help="write the provider schema (default: dry run)")
    args = ap.parse_args(argv)
    card = json.loads(Path(args.input).read_text(encoding="utf-8"))
    try:
        doc = apply_rate_card(args.provider, card, write=args.write)
    except RateCardError as exc:
        print(f"refused: {exc}", file=sys.stderr)
        return 2
    print(json.dumps({"provider": args.provider, "pricing": doc["pricing"], "written": args.write}, indent=2))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
