"""Scholarly & data integrations (Phase 6 / §14.5) — governed, provenance-first.

Each source: an AbstractSource (license, freshness, fetch) that writes a
provenance record (source URL, license/TOU flag, fetch time, freshness score)
so every claim in a report traces to a citation. Offline-tolerant (tests and
development never break on network absence). Tier-1 first: arXiv, OpenAlex.
"""

from .sources import ARXIV, OPENALEX, DATA_SOURCES, ground_literature, AbstractSource

__all__ = ["ARXIV", "OPENALEX", "DATA_SOURCES", "ground_literature", "AbstractSource"]
