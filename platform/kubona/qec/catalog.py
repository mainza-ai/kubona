"""Known QEC code catalog and structural novelty lookup (Gate 3).

Ships verified Bravyi et al. (Nature 2024) Bivariate Bicycle fixtures and a
loader for qiskit-community/qcode-discovery JSON catalogs.
"""

from __future__ import annotations

import json
from dataclasses import dataclass
from pathlib import Path
from typing import Any, Sequence

import numpy as np

from kubona.qec.canon import canonical_fingerprint, is_lc_equivalent_css
from kubona.qec.codes import KNOWN_BB_SPECS, build_bb_code
from kubona.qec.gf2 import css_validate


@dataclass(frozen=True)
class CatalogEntry:
    name: str
    n: int
    k: int
    d: int | None
    d_kind: str
    source: str
    hx: np.ndarray
    hz: np.ndarray
    fingerprint: str
    swapped_fingerprint: str


def build_builtin_catalog() -> list[CatalogEntry]:
    """Build the in-memory catalog of verified published BB codes."""
    entries: list[CatalogEntry] = []
    for spec in KNOWN_BB_SPECS:
        hx, hz = build_bb_code(spec.ell, spec.m, spec.a_terms, spec.b_terms)
        entries.append(
            CatalogEntry(
                name=spec.name,
                n=spec.n,
                k=spec.k,
                d=spec.d,
                d_kind="proved",
                source=spec.source,
                hx=hx,
                hz=hz,
                fingerprint=canonical_fingerprint(hx, hz),
                swapped_fingerprint=canonical_fingerprint(hz, hx),
            )
        )
    return entries


def load_qcode_discovery_json(path: str | Path) -> list[dict[str, Any]]:
    """Load a qiskit-community/qcode-discovery JSON catalog file."""
    raw = json.loads(Path(path).read_text())
    if isinstance(raw, list):
        return [r for r in raw if isinstance(r, dict)]
    if isinstance(raw, dict):
        for key in ("codes", "results", "proposals"):
            if isinstance(raw.get(key), list):
                return [r for r in raw[key] if isinstance(r, dict)]
    raise ValueError(f"unrecognized qcode-discovery catalog format in {path}")


def lookup_catalog(
    hx: np.ndarray,
    hz: np.ndarray,
    catalog: Sequence[CatalogEntry] | None = None,
) -> dict[str, Any]:
    """Check whether (Hx, Hz) matches any known catalog code up to relabelling or LC swap."""
    Hx, Hz = css_validate(hx, hz)
    fp = canonical_fingerprint(Hx, Hz)
    entries = list(catalog) if catalog is not None else build_builtin_catalog()

    for entry in entries:
        if entry.n != Hx.shape[1]:
            continue
        if fp != entry.fingerprint and fp != entry.swapped_fingerprint:
            continue
        eq, relation = is_lc_equivalent_css(Hx, Hz, entry.hx, entry.hz)
        if eq:
            return {
                "known": True,
                "matched_name": entry.name,
                "relation": relation,
                "source": entry.source,
                "canonical_hash": fp,
            }

    return {
        "known": False,
        "matched_name": None,
        "relation": "novel_to_catalog",
        "source": None,
        "canonical_hash": fp,
    }
