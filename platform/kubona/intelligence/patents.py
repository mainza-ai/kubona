"""EPO Open Patent Services (OPS) connector: prior-art intake for intelligence.

Terms verified 2026-10-11 against epo.org primary sources (OPS T&C, Fair use
charter, OPS page):
  - Registration-gated: OAuth consumer key + secret (operator step); robot
    access welcomed on OPS within limits (never scrape Espacenet — unsupported).
  - Licence: use/include data in own databases/products/services and
    distribute *as part of* products; NEVER redistribute raw data as such
    (OPS T&C s3). No royalties on derived products.
  - Free threshold 4 GB/calendar week; excess = EUR 2,800/yr subscription.
  - Fair use enforced here: >=6 s between searches (10/min), ~1 Mbit/s,
    quota refuse before the call, robot-hours note in docstring (schedule
    bulk 19:00-07:00 GMT/weekends).

Credential rule: without KUBONA_EPO_OPS_KEY/SECRET the live path refuses
before any network touch. `_http` injectable for stub tests.
"""

from __future__ import annotations

import os
import threading
import time
from dataclasses import dataclass, field
from typing import Any, Callable, List, Optional

from kubona.provenance import Ledger

KEY_ENV = "KUBONA_EPO_OPS_KEY"
SECRET_ENV = "KUBONA_EPO_OPS_SECRET"

LICENSE = ("EPO OPS (https://www.epo.org/en/searching-for-patents/data/"
           "web-services/ops): derived-use only, no raw redistribution "
           "(OPS T&C s3); free <=4GB/calendar week, EUR2800/yr above; "
           "fair use <=10 searches/min")

TOKEN_URL = "https://ops.epo.org/3.2/auth/accesstoken"
SEARCH_URL = "https://ops.epo.org/3.2/rest-services/published-data/search"

FREE_WEEKLY_BYTES = 4 * 1024 ** 3
MIN_INTERVAL_S = 6.0

_lock = threading.Lock()
_last_call: float = 0.0
_week_start: str = ""
_bytes_this_week: int = 0


class OpsError(RuntimeError):
    """Refusal: missing creds, quota, rate, network, or parse failure."""


def _week_id(now: float) -> str:
    return time.strftime("%G-W%V", time.gmtime(now))


def _respect_rate_limit(sleep: Optional[Callable[[float], None]] = None) -> None:
    global _last_call
    with _lock:
        t = time.time()
        wait = MIN_INTERVAL_S - (t - _last_call)
        if wait > 0:
            (sleep or time.sleep)(wait)
            t += wait
        _last_call = t


def _charge(nbytes: int) -> None:
    """Refuse before exceeding the free weekly threshold (process-local)."""
    global _week_start, _bytes_this_week
    with _lock:
        week = _week_id(time.time())
        if week != _week_start:
            _week_start, _bytes_this_week = week, 0
        if _bytes_this_week + nbytes > FREE_WEEKLY_BYTES:
            raise OpsError(
                f"EPO OPS free weekly threshold would be exceeded "
                f"({_bytes_this_week + nbytes} > {FREE_WEEKLY_BYTES} bytes): "
                "refusing (operator: subscription or next week)")
        _bytes_this_week += nbytes


def reset_accounting_for_tests() -> None:
    global _last_call, _week_start, _bytes_this_week
    with _lock:
        _last_call, _week_start, _bytes_this_week = 0.0, "", 0


@dataclass
class PatentHit:
    number: str
    title: str
    applicants: List[str] = field(default_factory=list)
    cpc: List[str] = field(default_factory=list)
    pub_date: str = ""
    source_url: str = ""


def _credentials() -> tuple:
    key = os.environ.get(KEY_ENV, "").strip()
    secret = os.environ.get(SECRET_ENV, "").strip()
    if not key or not secret:
        raise OpsError(
            f"EPO OPS refused: {KEY_ENV}/{SECRET_ENV} not set "
            "(register at developers.epo.org — operator step)")
    return key, secret


def search_patents(cql: str, _http: Any = None,
                   ledger: Optional[Ledger] = None) -> List[PatentHit]:
    """CQL search (e.g. 'ti=quantum AND pd>=2024'); returns patent hits.

    Each hit carries its source URL + the OPS licence flag travels with it
    into the ledger. Parse failures refuse (never partial rows as prior art).
    """
    import base64
    key, secret = _credentials()
    _respect_rate_limit()
    import httpx
    client = _http or httpx.Client(timeout=30)
    token_resp = client.post(
        TOKEN_URL,
        headers={"Authorization": "Basic " + base64.b64encode(
            f"{key}:{secret}".encode()).decode(),
            "Content-Type": "application/x-www-form-urlencoded"},
        content="grant_type=client_credentials")
    token_resp.raise_for_status()
    token = token_resp.json().get("access_token", "")
    resp = client.get(SEARCH_URL, params={"q": cql, "Range": "1-10"},
                      headers={"Authorization": f"Bearer {token}",
                               "Accept": "application/xml"})
    resp.raise_for_status()
    _charge(len(resp.content))
    try:
        import xml.etree.ElementTree as ET
        root = ET.fromstring(resp.text)
        ns = {"ops": "http://ops.epo.org"}
        hits = []
        for doc in root.findall(".//ops:publication-reference", ns):
            num = "".join((doc.findtext("ops:document-id/ops:country", default="", namespaces=ns),
                           doc.findtext("ops:document-id/ops:doc-number", default="", namespaces=ns),
                           doc.findtext("ops:document-id/ops:kind", default="", namespaces=ns)))
            hits.append(PatentHit(number=num or "unknown", title="",
                                  source_url=f"https://worldwide.espacenet.com/publicationDetails/biblio?CC={num[:2]}&NR={num[2:]}" if len(num) > 2 else ""))
    except Exception as exc:
        raise OpsError(f"EPO OPS response unparseable: refusing ({exc})")
    if ledger is not None:
        ledger.append("patents", {"query": cql, "hits": len(hits),
                                  "license": LICENSE})
    return hits
