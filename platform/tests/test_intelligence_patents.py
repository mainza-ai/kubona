"""EPO OPS connector: stub-driven tests (no network, no credentials)."""

import pytest

from kubona.intelligence import patents as ops
from kubona.provenance import Ledger


SEARCH_XML = """<?xml version="1.0"?>
<ops:world-patent-data xmlns:ops="http://ops.epo.org">
 <ops:biblio-search><ops:search-result>
  <ops:publication-reference>
   <ops:document-id><ops:country>EP</ops:country><ops:doc-number>4000000</ops:doc-number><ops:kind>B1</ops:kind></ops:document-id>
  </ops:publication-reference>
 </ops:search-result></ops:biblio-search></ops:world-patent-data>"""


class FakeResp:
    def __init__(self, text="", payload=None):
        self.text = text
        self.content = text.encode()
        self._payload = payload or {}

    def raise_for_status(self):
        pass

    def json(self):
        return self._payload


class FakeHTTP:
    def __init__(self):
        self.calls = []

    def post(self, url, headers=None, content=None):
        self.calls.append(("post", url))
        return FakeResp(payload={"access_token": "tok-test"})

    def get(self, url, params=None, headers=None):
        self.calls.append(("get", url, params))
        assert headers["Authorization"] == "Bearer tok-test"
        return FakeResp(text=SEARCH_XML)


@pytest.fixture(autouse=True)
def _env(monkeypatch):
    monkeypatch.setenv(ops.KEY_ENV, "k-test")
    monkeypatch.setenv(ops.SECRET_ENV, "s-test")
    ops.reset_accounting_for_tests()
    yield


def test_search_parses_hits_and_logs_ledger(tmp_path):
    hits = ops.search_patents("ti=quantum", _http=FakeHTTP(),
                              ledger=Ledger(path=tmp_path / "ops.jsonl"))
    assert len(hits) == 1 and hits[0].number == "EP4000000B1"
    assert "espacenet" in hits[0].source_url
    assert (tmp_path / "ops.jsonl").exists()


def test_missing_creds_refuse_before_network(monkeypatch):
    monkeypatch.delenv(ops.KEY_ENV, raising=False)
    with pytest.raises(ops.OpsError, match="not set"):
        ops.search_patents("ti=quantum", _http=FakeHTTP())


def test_free_threshold_refuse(tmp_path, monkeypatch):
    monkeypatch.setattr(ops, "FREE_WEEKLY_BYTES", 10)
    with pytest.raises(ops.OpsError, match="free weekly threshold"):
        ops.search_patents("ti=quantum", _http=FakeHTTP())


def test_unparseable_response_refuses(tmp_path):
    class Bad(FakeHTTP):
        def get(self, url, params=None, headers=None):
            return FakeResp(text="<not xml")
    with pytest.raises(ops.OpsError, match="unparseable"):
        ops.search_patents("ti=quantum", _http=Bad())
