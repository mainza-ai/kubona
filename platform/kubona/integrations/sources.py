"""Governed scholarly sources (arXiv, OpenAlex) with provenance-first fetching.

A fetch writes a ledger record {source, query, license, fetched_at, freshness,
results} so the discovery loop can cite it. Offline/network-failure degenerates
gracefully to a provenance record noting 'unavailable offline' — the campaign
still completes with a citation needing confirmation (never a silent fake).
"""

from __future__ import annotations

import time
from dataclasses import asdict, dataclass
from typing import Any, Callable

from kubona.provenance import Ledger


@dataclass
class SourceResult:
    source: str
    query: str
    items: list[dict[str, str]]      # {title, url}
    license: str
    fetched_at: float
    offline: bool = False

    def to_dict(self) -> dict[str, Any]:
        return asdict(self)


class AbstractSource:
    name: str = "abstract"
    license: str = "unknown"

    def fetch(self, query: str, _http: Callable | None = None) -> SourceResult:
        """Return items; on network failure return an offline (empty) result."""
        raise NotImplementedError


# arXiv TOU (info.arxiv.org/help/api/tou.html, verified 2026-10-10): at most
# one request every three seconds across ALL machines under our control, one
# connection at a time. Enforced client-side so autonomous loops cannot
# violate it by concurrency.
_ARXIV_MIN_INTERVAL_S = 3.0
_arxiv_last_call: float = 0.0

import threading as _threading

_arxiv_lock = _threading.Lock()


def _respect_arxiv_rate_limit(now: float | None = None,
                              sleep: Callable[[float], None] | None = None) -> None:
    """Block until 3s elapsed since the last arXiv call (process-wide)."""
    import time as _time
    global _arxiv_last_call
    with _arxiv_lock:
        t = now if now is not None else _time.time()
        wait = _ARXIV_MIN_INTERVAL_S - (t - _arxiv_last_call)
        if wait > 0:
            (sleep or _time.sleep)(wait)
            t = t + wait
        _arxiv_last_call = t


def _contact_email() -> str | None:
    """Optional polite-pool contact (OpenAlex mailto). Set KUBONA_CONTACT_EMAIL
    in deployment; never invent an address."""
    import os
    v = (os.environ.get("KUBONA_CONTACT_EMAIL") or "").strip()
    return v or None


class _HTTPArxivSource(AbstractSource):
    name = "arxiv"
    # Metadata CC0; e-prints mostly default-license: link back, never serve.
    license = "CC0 metadata; e-print redistribution requires holder permission"

    def fetch(self, query: str, _http=None) -> SourceResult:
        out: list[dict[str, str]] = []
        offline = False
        try:
            import httpx

            _respect_arxiv_rate_limit()
            client = _http or httpx.Client(timeout=10)
            r = client.get("https://export.arxiv.org/api/query",
                           params={"search_query": f"all:{query}", "max_results": 3})
            r.raise_for_status()
            import xml.etree.ElementTree as ET

            ns = {"a": "http://www.w3.org/2005/Atom"}
            root = ET.fromstring(r.text)
            for e in root.findall("a:entry", ns):
                title = (e.findtext("a:title", default="", namespaces=ns) or "").strip()
                link = (e.find("a:id", ns).text or "") if e.find("a:id", ns) is not None else ""
                out.append({"title": title[:120], "url": link})
        except Exception:  # noqa: BLE001 — offline/dev must not crash the campaign
            offline = True
        return SourceResult(source=self.name, query=query, items=out,
                            license=self.license, fetched_at=time.time(), offline=offline)


class _HTTPOpenAlexSource(AbstractSource):
    name = "openalex"
    license = "CC0 (metadata)"

    def fetch(self, query: str, _http=None) -> SourceResult:
        out: list[dict[str, str]] = []
        offline = False
        try:
            import httpx

            params: dict[str, Any] = {"search": query, "per-page": 3}
            contact = _contact_email()
            if contact:
                params["mailto"] = contact  # polite pool: 10/s vs 1/s anonymous
            client = _http or httpx.Client(timeout=10)
            r = client.get("https://api.openalex.org/works", params=params)
            r.raise_for_status()
            for w in r.json().get("results", []):
                out.append({"title": (w.get("display_name") or "")[:120],
                            "url": w.get("doi") or w.get("id") or ""})
        except Exception:  # noqa: BLE001
            offline = True
        return SourceResult(source=self.name, query=query, items=out,
                            license=self.license, fetched_at=time.time(), offline=offline)


class _MockSource(AbstractSource):
    """Deterministic in-repo source for offline/dev — a well-known reference."""

    name = "mock-papers"
    license = "CC0 (reference corpus)"

    def fetch(self, query: str, _http=None) -> SourceResult:
        # offline=True: static fixture, never a live fetch. Nothing may
        # present these two papers as searched results.
        return SourceResult(source=self.name, query=query,
                            items=[{"title": "VQE: theory and practice", "url": "mock://vqe"},
                                   {"title": "Error mitigation survey", "url": "mock://mirg"}],
                            license=self.license, fetched_at=time.time(), offline=True)


ARXIV = _HTTPArxivSource()
OPENALEX = _HTTPOpenAlexSource()
MOCK = _MockSource()
DATA_SOURCES: dict[str, AbstractSource] = {"arxiv": ARXIV, "openalex": OPENALEX, "mock": MOCK}


def ground_literature(query: str, source: str = "arxiv", ledger: Ledger | None = None,
                      _http=None) -> dict[str, Any]:
    """Fetch + write provenance (license flag, freshness, fetch time, offline note)."""
    src = DATA_SOURCES[source]
    result = src.fetch(query, _http=_http)
    freshness = 1.0 if not result.offline else 0.0
    record = None
    if ledger is not None:
        record = ledger.append("literature", {
            "source": result.source, "query": query, "license": result.license,
            "fetched_at": result.fetched_at, "freshness": freshness,
            "offline": result.offline, "n_results": len(result.items)})
    return {"source": result.source, "items": result.items, "license": result.license,
            "offline": result.offline, "freshness": freshness,
            "ledger_ref": record.record_id if record else None,
            "citations": [f"{i['title']} <{i['url']}>" for i in result.items]}
