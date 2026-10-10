"""Bridge literature grounding: real year/citation passthrough, honest cache flag.

Regression cover for hardcoded publication_year=2026 / citation_count=10 /
cached=True: live metadata must be real, and cache status must be measured.
Network is stubbed; the SQLite cache is redirected to tmp (never ~/.kubona).
"""

import importlib.util
import sys
from pathlib import Path

import pytest

BRIDGE = Path(__file__).resolve().parent.parent / "bridge"


def _load(name, filename):
    if str(BRIDGE) not in sys.path:
        sys.path.insert(0, str(BRIDGE))
    spec = importlib.util.spec_from_file_location(name, BRIDGE / filename)
    mod = importlib.util.module_from_spec(spec)
    sys.modules[name] = mod
    spec.loader.exec_module(mod)
    return mod


@pytest.fixture()
def lit(tmp_path, monkeypatch):
    mod = _load("bridge_literature_under_test", "literature.py")
    monkeypatch.setattr(mod, "CACHE_DB_PATH", tmp_path / "lit.db")
    return mod


ARXIV_XML = """<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
<entry><id>http://arxiv.org/abs/2101.00001v1</id>
<title>  Test VQE paper
</title><summary>Abstract here.</summary>
<published>2021-01-04T00:00:00Z</published>
<author><name>Jane Researcher</name></author>
</entry></feed>"""


class _Resp:
    def __init__(self, status_code=200, text="", payload=None):
        self.status_code = status_code
        self.text = text
        self._payload = payload

    def json(self):
        return self._payload


def test_arxiv_parses_real_year_and_zero_citations(lit, monkeypatch):
    monkeypatch.setattr(lit.httpx, "get", lambda url, timeout: _Resp(200, ARXIV_XML))
    out = lit.search_arxiv("vqe test", max_results=1)
    assert len(out) == 1
    rec = out[0]
    assert rec["year"] == 2021, rec
    assert rec["cited_by_count"] == 0  # arXiv exposes none: 0 means no data
    assert rec["cached"] is False
    assert "Jane Researcher" in rec["authors"]


def test_arxiv_cache_hit_labels_cached(lit, monkeypatch):
    monkeypatch.setattr(lit.httpx, "get", lambda url, timeout: _Resp(200, ARXIV_XML))
    assert lit.search_arxiv("vqe test", max_results=1)[0]["cached"] is False
    calls = []
    monkeypatch.setattr(lit.httpx, "get",
                        lambda url, timeout: calls.append(url) or _Resp(500, ""))
    second = lit.search_arxiv("vqe test", max_results=1)
    assert calls == []  # served from cache: zero network
    assert second[0]["cached"] is True
    assert second[0]["year"] == 2021


def test_openalex_passes_year_and_citations(lit, monkeypatch):
    payload = {"results": [{
        "display_name": "OpenAlex VQE paper",
        "doi": "https://doi.org/10.0000/test",
        "authorships": [{"author": {"display_name": "A. Uthor"}}],
        "open_access": {"oa_status": "gold"},
        "publication_year": 2019,
        "cited_by_count": 42,
    }]}
    monkeypatch.setattr(lit.httpx, "get", lambda url, timeout: _Resp(200, "", payload))
    out = lit.search_openalex("vqe test", max_results=1)
    assert out[0]["year"] == 2019
    assert out[0]["cited_by_count"] == 42
    assert out[0]["cached"] is False


def test_server_maps_real_values_not_placeholders(lit):
    # Pure mapping (no grpc/proto import): the handler builds pb messages
    # from to_records dicts, so this locks the no-placeholder rule hermetically.
    recs = lit.to_records([{
        "id": "http://arxiv.org/abs/2101.00001v1", "title": "T",
        "abstract": "A", "authors": ["J. R."],
        "year": 2021, "cited_by_count": 0, "cached": False,
    }])
    assert len(recs) == 1
    rec = recs[0]
    assert rec["year"] == 2021
    assert rec["cited_by_count"] == 0
    assert rec["cached"] is False
    assert rec["authors"] == "J. R."
    # Defaults are 0 (no data), never invented.
    bare = lit.to_records([{"id": "x"}])[0]
    assert bare["year"] == 0 and bare["cited_by_count"] == 0
    assert bare["authors"] == "Unknown"
