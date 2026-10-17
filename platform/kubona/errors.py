"""R5 — typed errors, Python rail.

Same closed code set as the Go rail (platform/contracts/error-catalog.csv,
loaded directly — no mirror to drift). KubonaError subclasses ValueError
so existing handlers keep working; the code + retryable travel with it
for MCP/REST callers implementing retry-vs-escalate from the catalog.
"""

from __future__ import annotations

import csv
import os
from pathlib import Path
from typing import Any


def _catalog_path() -> Path:
    override = os.environ.get("KUBONA_ERROR_CATALOG")
    if override:
        return Path(override)
    return Path(__file__).resolve().parents[1] / "contracts" / "error-catalog.csv"


def _load_catalog() -> dict[str, dict[str, Any]]:
    path = _catalog_path()
    try:
        rows = list(csv.DictReader(path.read_text().splitlines()))
    except OSError as e:
        raise RuntimeError(
            f"error catalog missing at {path}: refusing to run without "
            f"the closed code set ({e})"
        ) from e
    out = {}
    for r in rows:
        out[r["code"].strip()] = {
            "category": r["category"].strip(),
            "http_status": int(r["http_status"]),
            "retryable": r["retryable"].strip().lower() == "true",
            "remediation": r["remediation"].strip(),
        }
    return out


ERROR_CATALOG: dict[str, dict[str, Any]] = _load_catalog()


class KubonaError(ValueError):
    """Typed refusal: code (closed set) + message + retryable + details."""

    def __init__(self, code: str, message: str, details: dict[str, Any] | None = None):
        if code not in ERROR_CATALOG:
            raise ValueError(
                f"uncatalogued error code {code!r}: register it in "
                "contracts/error-catalog.csv first"
            )
        spec = ERROR_CATALOG[code]
        super().__init__(f"[{code}] {message} (remediation: {spec['remediation']})")
        self.code = code
        self.category = spec["category"]
        self.http_status = spec["http_status"]
        self.retryable = spec["retryable"]
        self.details = details or {}

    def to_dict(self) -> dict[str, Any]:
        return {
            "code": self.code,
            "category": self.category,
            "message": str(self),
            "retryable": self.retryable,
            "remediation": ERROR_CATALOG[self.code]["remediation"],
            "details": self.details,
        }
